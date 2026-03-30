package devices

import (
	"encoding/json"
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
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
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

	device, err := h.service.Register(r.Context(), RegisterInput{
		DeviceID:   req.DeviceID,
		UserID:     userID,
		DeviceName: req.DeviceName,
		Platform:   req.Platform,
		AppVersion: req.AppVersion,
	})
	if err != nil {
		apphttp.WriteError(w, http.StatusBadRequest, err.Error())
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

