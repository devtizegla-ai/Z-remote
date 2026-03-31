package files

import (
	"errors"
	"net/http"

	apphttp "remoteaccess/server/internal/http"
	"remoteaccess/server/internal/sessions"
)

const (
	multipartMemoryLimit int64 = 32 << 20
	requestOverheadBytes int64 = 1 << 20
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
	deviceID, ok := apphttp.DeviceIDFromContext(r.Context())
	if !ok || deviceID == "" {
		apphttp.WriteError(w, http.StatusUnauthorized, "missing device context")
		return
	}

	maxRequestBytes := h.service.MaxFileBytes() + requestOverheadBytes
	if maxRequestBytes < requestOverheadBytes {
		maxRequestBytes = requestOverheadBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)

	if err := r.ParseMultipartForm(multipartMemoryLimit); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			apphttp.WriteError(w, http.StatusRequestEntityTooLarge, ErrFileTooLarge.Error())
			return
		}
		apphttp.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	sessionID := r.FormValue("session_id")
	sessionToken := r.FormValue("session_token")
	fromDeviceID := r.FormValue("from_device_id")
	toDeviceID := r.FormValue("to_device_id")
	targetSavePath := r.FormValue("target_save_path")
	if fromDeviceID != "" && fromDeviceID != deviceID {
		apphttp.WriteError(w, http.StatusBadRequest, "from_device_id does not match authenticated device")
		return
	}
	fromDeviceID = deviceID

	file, header, err := r.FormFile("file")
	if err != nil {
		apphttp.WriteError(w, http.StatusBadRequest, "missing file")
		return
	}

	transfer, err := h.service.Upload(r.Context(), UploadInput{
		UserID:         userID,
		FromDeviceID:   fromDeviceID,
		ToDeviceID:     toDeviceID,
		SessionID:      sessionID,
		SessionToken:   sessionToken,
		TargetSavePath: targetSavePath,
		Header:         header,
		Reader:         file,
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
		apphttp.WriteError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, sessions.ErrSessionUnauthorized):
		apphttp.WriteError(w, http.StatusForbidden, err.Error())
	default:
		apphttp.WriteError(w, http.StatusBadRequest, err.Error())
	}
}
