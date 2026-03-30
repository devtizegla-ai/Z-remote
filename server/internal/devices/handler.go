package devices

import (
	"encoding/json"
	"errors"
	"net/http"

	apphttp "remoteaccess/server/internal/http"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type registerRequest struct {
	DeviceID    string `json:"device_id"`
	DeviceName  string `json:"device_name"`
	MachineName string `json:"machine_name"`
	MACAddress  string `json:"mac_address"`
	Platform    string `json:"platform"`
	AppVersion  string `json:"app_version"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	userID, ok := apphttp.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing user context")
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apphttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	deviceID := req.DeviceID
	if ctxDeviceID, ok := apphttp.DeviceIDFromContext(r.Context()); ok && ctxDeviceID != "" {
		if deviceID != "" && deviceID != ctxDeviceID {
			apphttp.WriteError(w, http.StatusBadRequest, "device_id in body does not match authenticated device")
			return
		}
		deviceID = ctxDeviceID
	}

	device, err := h.service.Register(r.Context(), RegisterInput{
		DeviceID:    deviceID,
		UserID:      userID,
		DeviceName:  req.DeviceName,
		MachineName: req.MachineName,
		MACAddress:  req.MACAddress,
		Platform:    req.Platform,
		AppVersion:  req.AppVersion,
		DeviceKey:   r.Header.Get("X-Device-Key"),
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrDeviceOwnershipConflict), errors.Is(err, ErrDeviceIdentityMismatch), errors.Is(err, ErrDeviceAuthFailed):
			apphttp.WriteError(w, http.StatusForbidden, err.Error())
		default:
			apphttp.WriteError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	apphttp.WriteJSON(w, http.StatusOK, device)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := apphttp.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing user context")
		return
	}

	onlineOnly := r.URL.Query().Get("all") != "true"
	devices, err := h.service.ListByUser(r.Context(), userID, onlineOnly)
	if err != nil {
		apphttp.WriteError(w, http.StatusInternalServerError, "failed to list devices")
		return
	}

	apphttp.WriteJSON(w, http.StatusOK, map[string]any{"devices": devices})
}
