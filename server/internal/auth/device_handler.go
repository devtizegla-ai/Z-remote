package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"remoteaccess/server/internal/devices"
	apphttp "remoteaccess/server/internal/http"
	"remoteaccess/server/internal/storage"
)

type DeviceHandler struct {
	service        *Service
	devicesService *devices.Service
}

func NewDeviceHandler(service *Service, devicesService *devices.Service) *DeviceHandler {
	return &DeviceHandler{
		service:        service,
		devicesService: devicesService,
	}
}

type deviceLoginRequest struct {
	DeviceID    string `json:"device_id"`
	DeviceName  string `json:"device_name"`
	MachineName string `json:"machine_name"`
	MACAddress  string `json:"mac_address"`
	Platform    string `json:"platform"`
	AppVersion  string `json:"app_version"`
}

func (h *DeviceHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req deviceLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apphttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		deviceID = strings.TrimSpace(r.Header.Get("X-Device-ID"))
	}
	deviceKey := strings.TrimSpace(r.Header.Get("X-Device-Key"))
	if deviceID == "" || deviceKey == "" {
		apphttp.WriteError(w, http.StatusBadRequest, "device_id and X-Device-Key are required")
		return
	}

	userID, err := h.resolveOrCreateUserForDevice(r.Context(), req, deviceID, deviceKey)
	if err != nil {
		switch {
		case errors.Is(err, devices.ErrDeviceAuthFailed),
			errors.Is(err, devices.ErrDeviceOwnershipConflict),
			errors.Is(err, devices.ErrDeviceIdentityMismatch):
			apphttp.WriteError(w, http.StatusForbidden, err.Error())
		default:
			apphttp.WriteError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	device, err := h.devicesService.Register(r.Context(), devices.RegisterInput{
		DeviceID:    deviceID,
		UserID:      userID,
		DeviceName:  req.DeviceName,
		MachineName: req.MachineName,
		MACAddress:  req.MACAddress,
		Platform:    req.Platform,
		AppVersion:  req.AppVersion,
		DeviceKey:   deviceKey,
	})
	if err != nil {
		switch {
		case errors.Is(err, devices.ErrDeviceAuthFailed),
			errors.Is(err, devices.ErrDeviceOwnershipConflict),
			errors.Is(err, devices.ErrDeviceIdentityMismatch):
			apphttp.WriteError(w, http.StatusForbidden, err.Error())
		default:
			apphttp.WriteError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	user, err := h.service.Me(r.Context(), userID)
	if err != nil {
		apphttp.WriteError(w, http.StatusInternalServerError, "failed to load device profile")
		return
	}

	accessToken, err := h.service.tokens.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		apphttp.WriteError(w, http.StatusInternalServerError, "failed generating access token")
		return
	}
	refreshToken, err := h.service.tokens.GenerateRefreshToken(user.ID, user.Email)
	if err != nil {
		apphttp.WriteError(w, http.StatusInternalServerError, "failed generating refresh token")
		return
	}

	apphttp.WriteJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":         user.ID,
			"name":       user.Name,
			"email":      user.Email,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		},
		"device":        device,
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

func (h *DeviceHandler) resolveOrCreateUserForDevice(
	ctx context.Context,
	req deviceLoginRequest,
	deviceID string,
	deviceKey string,
) (userID string, err error) {
	// fast path: device already exists and key matches
	existing, getErr := h.devicesService.GetByID(ctx, deviceID)
	if getErr == nil {
		if _, authErr := h.devicesService.Authenticate(ctx, existing.UserID, deviceID, deviceKey); authErr != nil {
			return "", authErr
		}
		user, userErr := h.service.Me(ctx, existing.UserID)
		if userErr != nil {
			return "", userErr
		}
		return user.ID, nil
	}
	if !errors.Is(getErr, devices.ErrDeviceNotFound) {
		return "", getErr
	}

	name := strings.TrimSpace(req.DeviceName)
	if name == "" {
		name = "Device " + shortLabel(deviceID)
	}
	email := fmt.Sprintf("%s@device.zremote.local", sanitizedDeviceEmailKey(deviceID))
	passwordHash, hashErr := HashPassword(storage.NewID("dpass"))
	if hashErr != nil {
		return "", hashErr
	}

	now := storage.NowUTC()
	newUserID := storage.NewID("usr")
	_, execErr := h.service.db.ExecContext(ctx, `
		INSERT INTO users (id, name, email, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		newUserID,
		name,
		email,
		passwordHash,
		now,
		now,
	)
	if execErr != nil {
		// collision fallback if email already exists
		if strings.Contains(strings.ToLower(execErr.Error()), "unique") {
			var existingUserID string
			err = h.service.db.QueryRowContext(ctx, `SELECT id FROM users WHERE email = ?`, email).Scan(&existingUserID)
			if err == nil {
				return existingUserID, nil
			}
			if errors.Is(err, sql.ErrNoRows) {
				return "", fmt.Errorf("failed creating device profile")
			}
		}
		return "", execErr
	}

	return newUserID, nil
}

func sanitizedDeviceEmailKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	builder := strings.Builder{}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	out := builder.String()
	if out == "" {
		out = storage.NewID("device")
	}
	return out
}

func shortLabel(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return value
	}
	return value[len(value)-8:]
}
