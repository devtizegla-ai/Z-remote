package sessions

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

type requestBody struct {
	TargetDeviceID string `json:"target_device_id"`
}

type respondBody struct {
	RequestID string `json:"request_id"`
	Accept    bool   `json:"accept"`
}

type startBody struct {
	RequestID string `json:"request_id"`
}

type endBody struct {
	SessionID    string `json:"session_id"`
	SessionToken string `json:"session_token"`
}

func (h *Handler) Request(w http.ResponseWriter, r *http.Request) {
	userID, ok := apphttp.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing user context")
		return
	}
	deviceID, ok := apphttp.DeviceIDFromContext(r.Context())
	if !ok || deviceID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing device context")
		return
	}

	var req requestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apphttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.service.Request(r.Context(), userID, deviceID, req.TargetDeviceID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	apphttp.WriteJSON(w, http.StatusCreated, map[string]any{"session_request": item})
}

func (h *Handler) Respond(w http.ResponseWriter, r *http.Request) {
	userID, ok := apphttp.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing user context")
		return
	}
	deviceID, ok := apphttp.DeviceIDFromContext(r.Context())
	if !ok || deviceID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing device context")
		return
	}

	var req respondBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apphttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.service.Respond(r.Context(), userID, req.RequestID, deviceID, req.Accept)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	apphttp.WriteJSON(w, http.StatusOK, map[string]any{"session_request": item})
}

func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	userID, ok := apphttp.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing user context")
		return
	}
	deviceID, ok := apphttp.DeviceIDFromContext(r.Context())
	if !ok || deviceID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing device context")
		return
	}

	var req startBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apphttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	session, err := h.service.Start(r.Context(), userID, req.RequestID, deviceID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	apphttp.WriteJSON(w, http.StatusCreated, map[string]any{"remote_session": session})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := apphttp.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing user context")
		return
	}

	sessions, err := h.service.ListByUser(r.Context(), userID)
	if err != nil {
		apphttp.WriteError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	apphttp.WriteJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (h *Handler) End(w http.ResponseWriter, r *http.Request) {
	userID, ok := apphttp.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing user context")
		return
	}
	deviceID, ok := apphttp.DeviceIDFromContext(r.Context())
	if !ok || deviceID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing device context")
		return
	}

	var req endBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apphttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	session, err := h.service.End(r.Context(), userID, deviceID, req.SessionID, req.SessionToken)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	apphttp.WriteJSON(w, http.StatusOK, map[string]any{"remote_session": session})
}

func (h *Handler) handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSessionUnauthorized):
		apphttp.WriteError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrSessionRequestNotFound), errors.Is(err, ErrSessionNotFound):
		apphttp.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrSessionExpired), errors.Is(err, ErrInvalidSessionRequest), errors.Is(err, ErrSessionNotActive):
		apphttp.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		apphttp.WriteError(w, http.StatusBadRequest, err.Error())
	}
}
