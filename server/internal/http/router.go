package http

import (
	nethttp "net/http"

	"remoteaccess/server/internal/config"
)

type RouteHandlers struct {
	Health          nethttp.Handler
	AuthRegister    nethttp.Handler
	AuthLogin       nethttp.Handler
	AuthDeviceLogin nethttp.Handler
	Me              nethttp.Handler
	DevicesRegister nethttp.Handler
	DevicesList     nethttp.Handler
	SessionsRequest nethttp.Handler
	SessionsRespond nethttp.Handler
	SessionsStart   nethttp.Handler
	SessionsEnd     nethttp.Handler
	SessionsList    nethttp.Handler
	FilesUpload     nethttp.Handler
	FilesDownload   nethttp.Handler
	WS              nethttp.Handler
}

func NewRouter(cfg config.Config, handlers RouteHandlers) nethttp.Handler {
	mux := nethttp.NewServeMux()
	mux.Handle("/health", methodHandler(nethttp.MethodGet, handlers.Health))

	mux.Handle("/api/auth/register", methodHandler(nethttp.MethodPost, handlers.AuthRegister))
	mux.Handle("/api/auth/login", methodHandler(nethttp.MethodPost, handlers.AuthLogin))
	mux.Handle("/api/auth/device-login", methodHandler(nethttp.MethodPost, handlers.AuthDeviceLogin))

	mux.Handle("/api/me", methodHandler(nethttp.MethodGet, handlers.Me))
	mux.Handle("/api/devices/register", methodHandler(nethttp.MethodPost, handlers.DevicesRegister))
	mux.Handle("/api/devices", methodHandler(nethttp.MethodGet, handlers.DevicesList))

	mux.Handle("/api/sessions/request", methodHandler(nethttp.MethodPost, handlers.SessionsRequest))
	mux.Handle("/api/sessions/respond", methodHandler(nethttp.MethodPost, handlers.SessionsRespond))
	mux.Handle("/api/sessions/start", methodHandler(nethttp.MethodPost, handlers.SessionsStart))
	mux.Handle("/api/sessions/end", methodHandler(nethttp.MethodPost, handlers.SessionsEnd))
	mux.Handle("/api/sessions", methodHandler(nethttp.MethodGet, handlers.SessionsList))

	mux.Handle("/api/files/upload", methodHandler(nethttp.MethodPost, handlers.FilesUpload))
	mux.Handle("/api/files/download", methodHandler(nethttp.MethodGet, handlers.FilesDownload))

	mux.Handle("/ws", handlers.WS)

	return Chain(
		mux,
		CORSMiddleware(cfg.CORSAllowedOrigins),
		RecoverMiddleware,
		LoggingMiddleware,
	)
}

func methodHandler(expected string, next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != expected {
			WriteError(w, nethttp.StatusMethodNotAllowed, "method not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}
