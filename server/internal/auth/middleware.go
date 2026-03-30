package auth

import (
	"context"
	"net/http"
	"strings"

	"remoteaccess/server/internal/devices"
	apphttp "remoteaccess/server/internal/http"
)

func Middleware(tokens *TokenManager, devicesService *devices.Service) apphttp.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				apphttp.WriteError(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				apphttp.WriteError(w, http.StatusUnauthorized, "invalid authorization header")
				return
			}

			claims, err := tokens.Parse(parts[1])
			if err != nil || claims.Type != "access" {
				apphttp.WriteError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), apphttp.ContextUserID, claims.UserID)
			deviceID := strings.TrimSpace(r.Header.Get("X-Device-ID"))
			deviceKey := strings.TrimSpace(r.Header.Get("X-Device-Key"))

			// Device registration is the enrollment endpoint and can run before the device exists.
			if r.URL.Path == "/api/devices/register" {
				if deviceID != "" {
					ctx = context.WithValue(ctx, apphttp.ContextDeviceID, deviceID)
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if r.URL.Path == "/api/me" {
				if deviceID != "" {
					ctx = context.WithValue(ctx, apphttp.ContextDeviceID, deviceID)
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if deviceID == "" || deviceKey == "" {
				apphttp.WriteError(w, http.StatusUnauthorized, "missing device authentication headers")
				return
			}

			if _, err := devicesService.Authenticate(r.Context(), claims.UserID, deviceID, deviceKey); err != nil {
				apphttp.WriteError(w, http.StatusUnauthorized, "invalid device authentication")
				return
			}
			ctx = context.WithValue(ctx, apphttp.ContextDeviceID, deviceID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
