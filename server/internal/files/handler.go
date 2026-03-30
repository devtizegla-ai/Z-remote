package files

import (
	"errors"
	"net/http"

	apphttp "remoteaccess/server/internal/http"
	"remoteaccess/server/internal/sessions"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	userID, ok := apphttp.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing user context")
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		apphttp.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	sessionID := r.FormValue("session_id")
	sessionToken := r.FormValue("session_token")
	fromDeviceID := r.FormValue("from_device_id")
	toDeviceID := r.FormValue("to_device_id")

	file, header, err := r.FormFile("file")
	if err != nil {
		apphttp.WriteError(w, http.StatusBadRequest, "missing file")
		return
	}

	transfer, err := h.service.Upload(r.Context(), UploadInput{
		UserID:       userID,
		FromDeviceID: fromDeviceID,
		ToDeviceID:   toDeviceID,
		SessionID:    sessionID,
		SessionToken: sessionToken,
		Header:       header,
		Reader:       file,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	apphttp.WriteJSON(w, http.StatusCreated, map[string]any{"file_transfer": transfer})
}

func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	userID, ok := apphttp.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing user context")
		return
	}

	transferID := r.URL.Query().Get("transfer_id")
	if transferID == "" {
		apphttp.WriteError(w, http.StatusBadRequest, "transfer_id is required")
		return
	}

	transfer, file, err := h.service.Download(r.Context(), userID, transferID)
	if err != nil {
		h.handleError(w, err)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+transfer.Filename+"\"")
	http.ServeContent(w, r, transfer.Filename, transfer.CreatedAt, file)
}

func (h *Handler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTransferNotFound):
		apphttp.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrFileTooLarge):
		apphttp.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, sessions.ErrSessionUnauthorized):
		apphttp.WriteError(w, http.StatusForbidden, err.Error())
	default:
		apphttp.WriteError(w, http.StatusBadRequest, err.Error())
	}
}

