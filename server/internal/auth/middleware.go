package auth

import (
	"context"
	"net/http"
	"strings"

	apphttp "remoteaccess/server/internal/http"
)

func Middleware(tokens *TokenManager) apphttp.Middleware {
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
			if deviceID := r.Header.Get("X-Device-ID"); deviceID != "" {
				ctx = context.WithValue(ctx, apphttp.ContextDeviceID, deviceID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

