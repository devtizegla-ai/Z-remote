package devices

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"remoteaccess/server/internal/models"
	"remoteaccess/server/internal/storage"
)

var ErrDeviceNotFound = errors.New("device not found")

type Service struct {
	db *sql.DB
}

func NewService(store *storage.Store) *Service {
	return &Service{db: store.DB}
}

type RegisterInput struct {
	DeviceID   string
	UserID     string
	DeviceName string
	Platform   string
	AppVersion string
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (models.Device, error) {
	now := storage.NowUTC()
	if strings.TrimSpace(input.DeviceID) == "" {
		input.DeviceID = storage.NewID("dev")
	}
	if strings.TrimSpace(input.DeviceName) == "" {
		input.DeviceName = "Unknown Device"
	}
	if strings.TrimSpace(input.Platform) == "" {
		input.Platform = "unknown"
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO devices (id, user_id, device_name, platform, app_version, status, last_seen_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'online', ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			user_id = excluded.user_id,
			device_name = excluded.device_name,
			platform = excluded.platform,
			app_version = excluded.app_version,
			status = 'online',
			last_seen_at = excluded.last_seen_at,
			updated_at = excluded.updated_at`,
		input.DeviceID,
		input.UserID,
		input.DeviceName,
		input.Platform,
		input.AppVersion,
		now,
		now,
		now,
	)
	if err != nil {
		return models.Device{}, err
	}

	return s.GetByID(ctx, input.DeviceID)
}

func (s *Service) GetByID(ctx context.Context, deviceID string) (models.Device, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, device_name, platform, app_version, status, last_seen_at, created_at, updated_at
		FROM devices WHERE id = ?`, deviceID)
	var device models.Device
	err := row.Scan(
		&device.ID,
		&device.UserID,
		&device.DeviceName,
		&device.Platform,
		&device.AppVersion,
		&device.Status,
		&device.LastSeenAt,
		&device.CreatedAt,
		&device.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Device{}, ErrDeviceNotFound
		}
		return models.Device{}, err
	}
	return device, nil
}

func (s *Service) ListByUser(ctx context.Context, userID string, onlineOnly bool) ([]models.Device, error) {
	query := `
		SELECT id, user_id, device_name, platform, app_version, status, last_seen_at, created_at, updated_at
		FROM devices WHERE user_id = ?`
	args := []any{userID}
	if onlineOnly {
		query += " AND status = 'online'"
	}
	query += " ORDER BY last_seen_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	devices := []models.Device{}
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(
			&d.ID,
			&d.UserID,
			&d.DeviceName,
			&d.Platform,
			&d.AppVersion,
			&d.Status,
			&d.LastSeenAt,
			&d.CreatedAt,
			&d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *Service) SetStatus(ctx context.Context, deviceID, status string) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE devices SET status = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`,
		status,
		storage.NowUTC(),
		storage.NowUTC(),
		deviceID,
	)
	return err
}

func (s *Service) BelongsToUser(ctx context.Context, deviceID, userID string) (bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM devices WHERE id = ? AND user_id = ?`, deviceID, userID)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

