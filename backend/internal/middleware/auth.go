package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/webssh/manager/internal/auth"
	"github.com/webssh/manager/internal/models"
)

type contextKey string

const UserContextKey contextKey = "user"

func AuthMiddleware(authService *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Primary: session cookie or Bearer token
			if token := extractToken(r); token != "" {
				if user, err := authService.ValidateSession(token); err == nil {
					ctx := context.WithValue(r.Context(), UserContextKey, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Fallback: short-lived WS ticket in query param (used by WebSocket
			// connections where cookie delivery is unreliable in some browsers).
			if ticket := r.URL.Query().Get("ticket"); ticket != "" {
				if user, err := authService.ValidateWSTicket(ticket); err == nil {
					ctx := context.WithValue(r.Context(), UserContextKey, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		})
	}
}

func extractToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			return parts[1]
		}
	}

	cookie, err := r.Cookie("session_token")
	if err == nil {
		return cookie.Value
	}

	return ""
}

func GetUserFromContext(ctx context.Context) (*models.User, bool) {
	user, ok := ctx.Value(UserContextKey).(*models.User)
	return user, ok
}
