package middleware

import (
	"net/http"
)

// CSRFMiddleware validates X-CSRF-Token against the session-bound CSRF token.
// It must run after AuthMiddleware so that the user (with CSRFToken) is in context.
func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		user, ok := GetUserFromContext(r.Context())
		if !ok || user.CSRFToken == "" {
			http.Error(w, "CSRF validation failed", http.StatusForbidden)
			return
		}

		token := r.Header.Get("X-CSRF-Token")
		if token == "" || token != user.CSRFToken {
			http.Error(w, "CSRF validation failed", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
