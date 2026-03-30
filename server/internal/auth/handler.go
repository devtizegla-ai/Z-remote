package auth

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
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apphttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.service.Register(r.Context(), req.Name, req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailExists):
			apphttp.WriteError(w, http.StatusConflict, err.Error())
		default:
			apphttp.WriteError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	apphttp.WriteJSON(w, http.StatusCreated, map[string]any{
		"user": map[string]any{
			"id":         user.ID,
			"name":       user.Name,
			"email":      user.Email,
			"created_at": user.CreatedAt,
		},
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apphttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			apphttp.WriteError(w, http.StatusUnauthorized, err.Error())
			return
		}
		apphttp.WriteError(w, http.StatusInternalServerError, "failed to login")
		return
	}

	apphttp.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := apphttp.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing user context")
		return
	}

	user, err := h.service.Me(r.Context(), userID)
	if err != nil {
		apphttp.WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	apphttp.WriteJSON(w, http.StatusOK, map[string]any{
		"id":         user.ID,
		"name":       user.Name,
		"email":      user.Email,
		"created_at": user.CreatedAt,
		"updated_at": user.UpdatedAt,
	})
}

