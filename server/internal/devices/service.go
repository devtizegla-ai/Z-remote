package devices

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"remoteaccess/server/internal/models"
	"remoteaccess/server/internal/storage"
)

var (
	ErrDeviceNotFound          = errors.New("device not found")
	ErrDeviceOwnershipConflict = errors.New("device already belongs to another user")
	ErrDeviceIdentityMismatch  = errors.New("device identity mismatch for this id")
	ErrDeviceAuthFailed        = errors.New("device authentication failed")
)

type Service struct {
	db *sql.DB
}

func NewService(store *storage.Store) *Service {
	return &Service{db: store.DB}
}

type RegisterInput struct {
	DeviceID    string
	UserID      string
	DeviceName  string
	MachineName string
	MACAddress  string
	Platform    string
	AppVersion  string
	DeviceKey   string
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (models.Device, error) {
	now := storage.NowUTC()

	input.UserID = strings.TrimSpace(input.UserID)
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.DeviceName = strings.TrimSpace(input.DeviceName)
	input.MachineName = strings.TrimSpace(input.MachineName)
	input.MACAddress = normalizeMAC(input.MACAddress)
	input.Platform = strings.TrimSpace(input.Platform)
	input.AppVersion = strings.TrimSpace(input.AppVersion)
	input.DeviceKey = strings.TrimSpace(input.DeviceKey)

	if input.UserID == "" {
		return models.Device{}, fmt.Errorf("user_id is required")
	}
	if input.DeviceKey == "" {
		return models.Device{}, fmt.Errorf("device_key is required")
	}
	if input.DeviceID == "" {
		input.DeviceID = storage.NewID("dev")
	}
	if input.DeviceName == "" {
		input.DeviceName = "Unknown Device"
	}
	if input.MachineName == "" {
		input.MachineName = input.DeviceName
	}
	if input.Platform == "" {
		input.Platform = "unknown"
	}
	if input.AppVersion == "" {
		input.AppVersion = "0.1.0"
	}

	deviceKeyHash := hashDeviceKey(input.DeviceKey)

	existing, err := s.GetByID(ctx, input.DeviceID)
	switch {
	case err == nil:
		if existing.UserID != input.UserID {
			return models.Device{}, ErrDeviceOwnershipConflict
		}
		if existing.DeviceKeyHash != "" {
			if existing.DeviceKeyHash != deviceKeyHash {
				return models.Device{}, ErrDeviceAuthFailed
			}
		} else if !identityCompatible(existing.MachineName, input.MachineName) ||
			!identityCompatible(existing.MACAddress, input.MACAddress) {
			return models.Device{}, ErrDeviceIdentityMismatch
		}

		machineName := fallbackIfEmpty(input.MachineName, existing.MachineName)
		macAddress := fallbackIfEmpty(input.MACAddress, existing.MACAddress)
		if machineName == "" {
			machineName = input.DeviceName
		}

		_, err = s.db.ExecContext(ctx, `
			UPDATE devices SET
				device_name = ?,
				machine_name = ?,
				mac_address = ?,
				platform = ?,
				app_version = ?,
				device_key_hash = ?,
				status = 'online',
				last_seen_at = ?,
				updated_at = ?
			WHERE id = ?`,
			input.DeviceName,
			machineName,
			macAddress,
			input.Platform,
			input.AppVersion,
			deviceKeyHash,
			now,
			now,
			input.DeviceID,
		)
		if err != nil {
			return models.Device{}, err
		}
	case errors.Is(err, ErrDeviceNotFound):
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO devices (
				id, user_id, device_name, machine_name, mac_address, platform, app_version, device_key_hash, status, last_seen_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'online', ?, ?, ?)`,
			input.DeviceID,
			input.UserID,
			input.DeviceName,
			input.MachineName,
			input.MACAddress,
			input.Platform,
			input.AppVersion,
			deviceKeyHash,
			now,
			now,
			now,
		)
		if err != nil {
			return models.Device{}, err
		}
	default:
		return models.Device{}, err
	}

	return s.GetByID(ctx, input.DeviceID)
}

func (s *Service) Authenticate(ctx context.Context, userID, deviceID, deviceKey string) (models.Device, error) {
	userID = strings.TrimSpace(userID)
	deviceID = strings.TrimSpace(deviceID)
	deviceKey = strings.TrimSpace(deviceKey)

	if userID == "" || deviceID == "" || deviceKey == "" {
		return models.Device{}, ErrDeviceAuthFailed
	}

	device, err := s.GetByID(ctx, deviceID)
	if err != nil {
		if errors.Is(err, ErrDeviceNotFound) {
			return models.Device{}, ErrDeviceAuthFailed
		}
		return models.Device{}, err
	}
	if device.UserID != userID {
		return models.Device{}, ErrDeviceAuthFailed
	}

	deviceKeyHash := hashDeviceKey(deviceKey)
	if device.DeviceKeyHash == "" {
		return models.Device{}, ErrDeviceAuthFailed
	}
	if device.DeviceKeyHash != deviceKeyHash {
		return models.Device{}, ErrDeviceAuthFailed
	}

	return device, nil
}

func (s *Service) GetByID(ctx context.Context, deviceID string) (models.Device, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, device_name, machine_name, mac_address, platform, app_version, status, device_key_hash, last_seen_at, created_at, updated_at
		FROM devices WHERE id = ?`, deviceID)
	var device models.Device
	err := row.Scan(
		&device.ID,
		&device.UserID,
		&device.DeviceName,
		&device.MachineName,
		&device.MACAddress,
		&device.Platform,
		&device.AppVersion,
		&device.Status,
		&device.DeviceKeyHash,
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
		SELECT id, user_id, device_name, machine_name, mac_address, platform, app_version, status, device_key_hash, last_seen_at, created_at, updated_at
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
			&d.MachineName,
			&d.MACAddress,
			&d.Platform,
			&d.AppVersion,
			&d.Status,
			&d.DeviceKeyHash,
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
	now := storage.NowUTC()
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE devices SET status = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`,
		status,
		now,
		now,
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

func hashDeviceKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func normalizeMAC(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "-", ":")
	return strings.ToUpper(value)
}

func fallbackIfEmpty(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func identityCompatible(existing, incoming string) bool {
	existing = strings.TrimSpace(existing)
	incoming = strings.TrimSpace(incoming)
	if existing == "" || incoming == "" {
		return true
	}
	return strings.EqualFold(existing, incoming)
}
