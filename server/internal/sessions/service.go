package sessions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"remoteaccess/server/internal/devices"
	"remoteaccess/server/internal/models"
	"remoteaccess/server/internal/storage"
)

var (
	ErrSessionRequestNotFound = errors.New("session request not found")
	ErrInvalidSessionRequest  = errors.New("invalid session request")
	ErrSessionNotFound        = errors.New("remote session not found")
	ErrSessionUnauthorized    = errors.New("not authorized for this session")
	ErrSessionExpired         = errors.New("session expired")
)

type Notifier interface {
	NotifyDevice(deviceID string, payload any) error
}

type Service struct {
	db              *sql.DB
	devicesService  *devices.Service
	notifier        Notifier
	sessionTokenTTL time.Duration
}

func NewService(store *storage.Store, devicesService *devices.Service, sessionTokenTTL time.Duration) *Service {
	return &Service{
		db:              store.DB,
		devicesService:  devicesService,
		sessionTokenTTL: sessionTokenTTL,
	}
}

func (s *Service) SetNotifier(notifier Notifier) {
	s.notifier = notifier
}

func (s *Service) Request(ctx context.Context, userID, requesterDeviceID, targetDeviceID string) (models.SessionRequest, error) {
	if requesterDeviceID == "" || targetDeviceID == "" {
		return models.SessionRequest{}, fmt.Errorf("requester_device_id and target_device_id are required")
	}
	if requesterDeviceID == targetDeviceID {
		return models.SessionRequest{}, fmt.Errorf("cannot request session with same device")
	}

	requesterOwned, err := s.devicesService.BelongsToUser(ctx, requesterDeviceID, userID)
	if err != nil {
		return models.SessionRequest{}, err
	}
	if !requesterOwned {
		return models.SessionRequest{}, ErrSessionUnauthorized
	}

	targetDevice, err := s.devicesService.GetByID(ctx, targetDeviceID)
	if err != nil {
		return models.SessionRequest{}, err
	}
	if targetDevice.Status != "online" {
		return models.SessionRequest{}, fmt.Errorf("target device is offline")
	}

	now := storage.NowUTC()
	request := models.SessionRequest{
		ID:                storage.NewID("srq"),
		RequesterDeviceID: requesterDeviceID,
		TargetDeviceID:    targetDeviceID,
		Status:            "pending",
		CreatedAt:         now,
	}

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO session_requests (id, requester_device_id, target_device_id, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		request.ID,
		request.RequesterDeviceID,
		request.TargetDeviceID,
		request.Status,
		request.CreatedAt,
	)
	if err != nil {
		return models.SessionRequest{}, err
	}

	s.logAudit(ctx, userID, requesterDeviceID, "session_request_created", map[string]any{
		"request_id":       request.ID,
		"target_device_id": targetDeviceID,
	})

	if s.notifier != nil {
		_ = s.notifier.NotifyDevice(targetDeviceID, map[string]any{
			"type":    "session_request",
			"request": request,
		})
	}

	return request, nil
}

func (s *Service) Respond(ctx context.Context, userID, requestID, targetDeviceID string, accept bool) (models.SessionRequest, error) {
	request, err := s.GetRequestByID(ctx, requestID)
	if err != nil {
		return models.SessionRequest{}, err
	}
	if request.TargetDeviceID != targetDeviceID {
		return models.SessionRequest{}, ErrSessionUnauthorized
	}

	owned, err := s.devicesService.BelongsToUser(ctx, targetDeviceID, userID)
	if err != nil {
		return models.SessionRequest{}, err
	}
	if !owned {
		return models.SessionRequest{}, ErrSessionUnauthorized
	}
	if request.Status != "pending" {
		return models.SessionRequest{}, ErrInvalidSessionRequest
	}

	now := storage.NowUTC()
	status := "rejected"
	if accept {
		status = "accepted"
	}

	_, err = s.db.ExecContext(
		ctx,
		`UPDATE session_requests SET status = ?, responded_at = ? WHERE id = ?`,
		status,
		now,
		requestID,
	)
	if err != nil {
		return models.SessionRequest{}, err
	}
	request.Status = status
	request.RespondedAt = &now

	s.logAudit(ctx, userID, targetDeviceID, "session_request_responded", map[string]any{
		"request_id": requestID,
		"status":     status,
	})

	if s.notifier != nil {
		_ = s.notifier.NotifyDevice(request.RequesterDeviceID, map[string]any{
			"type":    "session_response",
			"request": request,
		})
	}

	return request, nil
}

func (s *Service) Start(ctx context.Context, userID, requestID, requesterDeviceID string) (models.RemoteSession, error) {
	request, err := s.GetRequestByID(ctx, requestID)
	if err != nil {
		return models.RemoteSession{}, err
	}
	if request.RequesterDeviceID != requesterDeviceID {
		return models.RemoteSession{}, ErrSessionUnauthorized
	}

	owned, err := s.devicesService.BelongsToUser(ctx, requesterDeviceID, userID)
	if err != nil {
		return models.RemoteSession{}, err
	}
	if !owned {
		return models.RemoteSession{}, ErrSessionUnauthorized
	}
	if request.Status != "accepted" {
		return models.RemoteSession{}, fmt.Errorf("session request has not been accepted")
	}

	now := storage.NowUTC()
	tokenExpiry := now.Add(s.sessionTokenTTL)
	session := models.RemoteSession{
		ID:                storage.NewID("rss"),
		RequesterDeviceID: request.RequesterDeviceID,
		TargetDeviceID:    request.TargetDeviceID,
		SessionToken:      storage.NewID("stkn"),
		Status:            "active",
		StartedAt:         &now,
		CreatedAt:         now,
	}

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO remote_sessions (id, requester_device_id, target_device_id, session_token, status, started_at, token_expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.RequesterDeviceID,
		session.TargetDeviceID,
		session.SessionToken,
		session.Status,
		session.StartedAt,
		tokenExpiry,
		session.CreatedAt,
	)
	if err != nil {
		return models.RemoteSession{}, err
	}

	s.logAudit(ctx, userID, requesterDeviceID, "remote_session_started", map[string]any{
		"session_id":         session.ID,
		"target_device_id":   session.TargetDeviceID,
		"token_expires_at":   tokenExpiry,
		"session_request_id": requestID,
	})

	if s.notifier != nil {
		notifyPayload := map[string]any{
			"type": "session_started",
			"session": map[string]any{
				"id":                  session.ID,
				"requester_device_id": session.RequesterDeviceID,
				"target_device_id":    session.TargetDeviceID,
				"session_token":       session.SessionToken,
				"status":              session.Status,
				"started_at":          session.StartedAt,
				"created_at":          session.CreatedAt,
				"token_expires_at":    tokenExpiry,
			},
		}
		_ = s.notifier.NotifyDevice(session.RequesterDeviceID, notifyPayload)
		_ = s.notifier.NotifyDevice(session.TargetDeviceID, notifyPayload)
	}

	return session, nil
}

func (s *Service) ListByUser(ctx context.Context, userID string) ([]models.RemoteSession, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rs.id, rs.requester_device_id, rs.target_device_id, rs.session_token, rs.status, rs.started_at, rs.ended_at, rs.created_at
		FROM remote_sessions rs
		JOIN devices d1 ON d1.id = rs.requester_device_id
		JOIN devices d2 ON d2.id = rs.target_device_id
		WHERE d1.user_id = ? OR d2.user_id = ?
		ORDER BY rs.created_at DESC`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.RemoteSession{}
	for rows.Next() {
		var sItem models.RemoteSession
		var startedAt sql.NullTime
		var endedAt sql.NullTime
		if err := rows.Scan(
			&sItem.ID,
			&sItem.RequesterDeviceID,
			&sItem.TargetDeviceID,
			&sItem.SessionToken,
			&sItem.Status,
			&startedAt,
			&endedAt,
			&sItem.CreatedAt,
		); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			t := startedAt.Time
			sItem.StartedAt = &t
		}
		if endedAt.Valid {
			t := endedAt.Time
			sItem.EndedAt = &t
		}
		items = append(items, sItem)
	}
	return items, rows.Err()
}

func (s *Service) ValidateSessionParticipant(ctx context.Context, sessionID, sessionToken, deviceID string) (models.RemoteSession, string, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, requester_device_id, target_device_id, session_token, status, started_at, ended_at, created_at, token_expires_at
		FROM remote_sessions
		WHERE id = ?`, sessionID)

	var session models.RemoteSession
	var startedAt sql.NullTime
	var endedAt sql.NullTime
	var tokenExpiresAt time.Time
	if err := row.Scan(
		&session.ID,
		&session.RequesterDeviceID,
		&session.TargetDeviceID,
		&session.SessionToken,
		&session.Status,
		&startedAt,
		&endedAt,
		&session.CreatedAt,
		&tokenExpiresAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.RemoteSession{}, "", ErrSessionNotFound
		}
		return models.RemoteSession{}, "", err
	}
	if startedAt.Valid {
		t := startedAt.Time
		session.StartedAt = &t
	}
	if endedAt.Valid {
		t := endedAt.Time
		session.EndedAt = &t
	}

	if session.SessionToken != sessionToken {
		return models.RemoteSession{}, "", ErrSessionUnauthorized
	}
	if session.Status != "active" {
		return models.RemoteSession{}, "", fmt.Errorf("session is not active")
	}
	if storage.NowUTC().After(tokenExpiresAt) {
		return models.RemoteSession{}, "", ErrSessionExpired
	}

	switch deviceID {
	case session.RequesterDeviceID:
		return session, session.TargetDeviceID, nil
	case session.TargetDeviceID:
		return session, session.RequesterDeviceID, nil
	default:
		return models.RemoteSession{}, "", ErrSessionUnauthorized
	}
}

func (s *Service) GetRequestByID(ctx context.Context, requestID string) (models.SessionRequest, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, requester_device_id, target_device_id, status, created_at, responded_at
		FROM session_requests WHERE id = ?`, requestID)

	var request models.SessionRequest
	var respondedAt sql.NullTime
	if err := row.Scan(
		&request.ID,
		&request.RequesterDeviceID,
		&request.TargetDeviceID,
		&request.Status,
		&request.CreatedAt,
		&respondedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.SessionRequest{}, ErrSessionRequestNotFound
		}
		return models.SessionRequest{}, err
	}
	if respondedAt.Valid {
		t := respondedAt.Time
		request.RespondedAt = &t
	}
	return request, nil
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
