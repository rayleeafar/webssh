package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

type CSRFStore struct {
	tokens map[string]time.Time
	mu     sync.RWMutex
}

func NewCSRFStore() *CSRFStore {
	store := &CSRFStore{
		tokens: make(map[string]time.Time),
	}
	go store.cleanup()
	return store
}

func (s *CSRFStore) Generate() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(b)

	s.mu.Lock()
	s.tokens[token] = time.Now().Add(1 * time.Hour)
	s.mu.Unlock()

	return token, nil
}

func (s *CSRFStore) Validate(token string) bool {
	s.mu.RLock()
	expiry, exists := s.tokens[token]
	s.mu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().After(expiry) {
		s.mu.Lock()
		delete(s.tokens, token)
		s.mu.Unlock()
		return false
	}

	return true
}

func (s *CSRFStore) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for token, expiry := range s.tokens {
			if now.After(expiry) {
				delete(s.tokens, token)
			}
		}
		s.mu.Unlock()
	}
}

func CSRFMiddleware(store *CSRFStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			token := r.Header.Get("X-CSRF-Token")
			if token == "" {
				http.Error(w, "CSRF token required", http.StatusForbidden)
				return
			}

			if !store.Validate(token) {
				http.Error(w, "Invalid CSRF token", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
