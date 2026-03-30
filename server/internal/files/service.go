package files

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"remoteaccess/server/internal/devices"
	"remoteaccess/server/internal/models"
	"remoteaccess/server/internal/sessions"
	"remoteaccess/server/internal/storage"
)

var (
	ErrTransferNotFound = errors.New("file transfer not found")
	ErrFileTooLarge     = errors.New("file exceeds allowed size")
)

type Notifier interface {
	NotifyDevice(deviceID string, payload any) error
}

type Service struct {
	db             *sql.DB
	sessions       *sessions.Service
	devices        *devices.Service
	notifier       Notifier
	maxFileBytes   int64
	storageDirPath string
}

func NewService(store *storage.Store, sessionsSvc *sessions.Service, devicesSvc *devices.Service, maxFileBytes int64, storageDirPath string) *Service {
	return &Service{
		db:             store.DB,
		sessions:       sessionsSvc,
		devices:        devicesSvc,
		maxFileBytes:   maxFileBytes,
		storageDirPath: storageDirPath,
	}
}

func (s *Service) SetNotifier(notifier Notifier) {
	s.notifier = notifier
}

type UploadInput struct {
	UserID       string
	FromDeviceID string
	ToDeviceID   string
	SessionID    string
	SessionToken string
	Header       *multipart.FileHeader
	Reader       multipart.File
}

func (s *Service) Upload(ctx context.Context, input UploadInput) (models.FileTransfer, error) {
	defer input.Reader.Close()

	if input.Header == nil {
		return models.FileTransfer{}, fmt.Errorf("missing file")
	}
	if input.Header.Size > s.maxFileBytes {
		return models.FileTransfer{}, ErrFileTooLarge
	}

	_, peerDeviceID, err := s.sessions.ValidateSessionParticipant(ctx, input.SessionID, input.SessionToken, input.FromDeviceID)
	if err != nil {
		return models.FileTransfer{}, err
	}
	if input.ToDeviceID != peerDeviceID {
		return models.FileTransfer{}, sessions.ErrSessionUnauthorized
	}

	fromOwned, err := s.devices.BelongsToUser(ctx, input.FromDeviceID, input.UserID)
	if err != nil {
		return models.FileTransfer{}, err
	}
	if !fromOwned {
		return models.FileTransfer{}, sessions.ErrSessionUnauthorized
	}

	if err := os.MkdirAll(s.storageDirPath, 0o755); err != nil {
		return models.FileTransfer{}, err
	}

	transferID := storage.NewID("flt")
	safeName := sanitizeFilename(input.Header.Filename)
	storagePath := filepath.Join(s.storageDirPath, transferID+"_"+safeName)

	targetFile, err := os.Create(storagePath)
	if err != nil {
		return models.FileTransfer{}, err
	}
	defer targetFile.Close()

	written, err := io.Copy(targetFile, io.LimitReader(input.Reader, s.maxFileBytes+1))
	if err != nil {
		return models.FileTransfer{}, err
	}
	if written > s.maxFileBytes {
		_ = os.Remove(storagePath)
		return models.FileTransfer{}, ErrFileTooLarge
	}

	now := storage.NowUTC()
	transfer := models.FileTransfer{
		ID:                 transferID,
		SessionID:          input.SessionID,
		Filename:           safeName,
		SizeBytes:          written,
		Status:             "uploaded",
		StoragePath:        storagePath,
		UploadedByDeviceID: input.FromDeviceID,
		TargetDeviceID:     input.ToDeviceID,
		CreatedAt:          now,
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO file_transfers (
			id, session_id, filename, size_bytes, status, storage_path, uploaded_by_device_id, target_device_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		transfer.ID,
		transfer.SessionID,
		transfer.Filename,
		transfer.SizeBytes,
		transfer.Status,
		transfer.StoragePath,
		transfer.UploadedByDeviceID,
		transfer.TargetDeviceID,
		transfer.CreatedAt,
	)
	if err != nil {
		_ = os.Remove(storagePath)
		return models.FileTransfer{}, err
	}

	s.logAudit(ctx, input.UserID, input.FromDeviceID, "file_uploaded", map[string]any{
		"transfer_id":  transfer.ID,
		"session_id":   transfer.SessionID,
		"filename":     transfer.Filename,
		"size_bytes":   transfer.SizeBytes,
		"to_device_id": transfer.TargetDeviceID,
	})

	if s.notifier != nil {
		_ = s.notifier.NotifyDevice(input.ToDeviceID, map[string]any{
			"type": "file_available",
			"file": map[string]any{
				"id":             transfer.ID,
				"session_id":     transfer.SessionID,
				"filename":       transfer.Filename,
				"size_bytes":     transfer.SizeBytes,
				"from_device_id": input.FromDeviceID,
			},
		})
	}

	return transfer, nil
}

func (s *Service) Download(ctx context.Context, userID, transferID string) (models.FileTransfer, *os.File, error) {
	transfer, err := s.getByID(ctx, transferID)
	if err != nil {
		return models.FileTransfer{}, nil, err
	}

	fromOwned, err := s.devices.BelongsToUser(ctx, transfer.UploadedByDeviceID, userID)
	if err != nil {
		return models.FileTransfer{}, nil, err
	}
	toOwned, err := s.devices.BelongsToUser(ctx, transfer.TargetDeviceID, userID)
	if err != nil {
		return models.FileTransfer{}, nil, err
	}
	if !fromOwned && !toOwned {
		return models.FileTransfer{}, nil, sessions.ErrSessionUnauthorized
	}

	f, err := os.Open(transfer.StoragePath)
	if err != nil {
		return models.FileTransfer{}, nil, err
	}

	_, _ = s.db.ExecContext(ctx, `UPDATE file_transfers SET status = 'downloaded' WHERE id = ?`, transfer.ID)
	return transfer, f, nil
}

func (s *Service) getByID(ctx context.Context, transferID string) (models.FileTransfer, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, session_id, filename, size_bytes, status, storage_path, uploaded_by_device_id, target_device_id, created_at
		FROM file_transfers WHERE id = ?`, transferID)

	var transfer models.FileTransfer
	if err := row.Scan(
		&transfer.ID,
		&transfer.SessionID,
		&transfer.Filename,
		&transfer.SizeBytes,
		&transfer.Status,
		&transfer.StoragePath,
		&transfer.UploadedByDeviceID,
		&transfer.TargetDeviceID,
		&transfer.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.FileTransfer{}, ErrTransferNotFound
		}
		return models.FileTransfer{}, err
	}
	return transfer, nil
}

func (s *Service) logAudit(ctx context.Context, userID, deviceID, action string, metadata map[string]any) {
	jsonBytes, _ := json.Marshal(metadata)
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, user_id, device_id, action, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		storage.NewID("adt"),
		userID,
		deviceID,
		action,
		string(jsonBytes),
		storage.NowUTC(),
	)
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "..", "")
	if name == "" {
		return "file.bin"
	}
	return name
}
