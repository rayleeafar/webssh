package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webssh/manager/internal/auth"
	"github.com/webssh/manager/internal/database"
	"github.com/webssh/manager/internal/middleware"
)

// testMasterKey is a fixed 32-byte key used in all auth handler tests.
var testMasterKey = []byte("test-master-key-for-unit-tests!!")

func setupAuthTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatalf("failed to setup test DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestAuthService(db *sql.DB) *auth.Service {
	return auth.NewService(db, testMasterKey)
}

func registerUser(t *testing.T, svc *auth.Service, username, password string) {
	t.Helper()
	if _, err := svc.Register(username, password); err != nil {
		t.Fatalf("register failed: %v", err)
	}
}

// --- Method-not-allowed guards (keep existing coverage) ---

func TestRegisterHandlerMethodNotAllowed(t *testing.T) {
	handler := &AuthHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/register", nil)
	w := httptest.NewRecorder()
	handler.Register(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestLoginHandlerMethodNotAllowed(t *testing.T) {
	handler := &AuthHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
	w := httptest.NewRecorder()
	handler.Login(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestLogoutHandlerMethodNotAllowed(t *testing.T) {
	handler := &AuthHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	handler.Logout(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestMeHandlerMethodNotAllowed(t *testing.T) {
	handler := &AuthHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/me", nil)
	w := httptest.NewRecorder()
	handler.Me(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// --- AC-1: register success ---

func TestRegisterSuccess(t *testing.T) {
	db := setupAuthTestDB(t)
	svc := newTestAuthService(db)
	h := NewAuthHandler(svc, false)

	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegisterDuplicateUsername(t *testing.T) {
	db := setupAuthTestDB(t)
	svc := newTestAuthService(db)
	h := NewAuthHandler(svc, false)
	registerUser(t, svc, "alice", "secret")

	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "other"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

// --- AC-1: login success / failure ---

func TestLoginSuccess(t *testing.T) {
	db := setupAuthTestDB(t)
	svc := newTestAuthService(db)
	h := NewAuthHandler(svc, false)
	registerUser(t, svc, "alice", "secret")

	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// session_token cookie must be set and HttpOnly
	var sessionCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session_token" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session_token cookie")
	}
	if !sessionCookie.HttpOnly {
		t.Error("session_token cookie must be HttpOnly")
	}

	// csrf_token must be in response body
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	csrfToken, ok := resp["csrf_token"].(string)
	if !ok || csrfToken == "" {
		t.Error("expected non-empty csrf_token in login response")
	}
}

func TestLoginFailure(t *testing.T) {
	db := setupAuthTestDB(t)
	svc := newTestAuthService(db)
	h := NewAuthHandler(svc, false)

	body, _ := json.Marshal(map[string]string{"username": "nouser", "password": "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// --- AC-1: /api/auth/me ---

func TestMeUnauthenticated(t *testing.T) {
	db := setupAuthTestDB(t)
	svc := newTestAuthService(db)
	h := NewAuthHandler(svc, false)
	authMW := middleware.AuthMiddleware(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	w := httptest.NewRecorder()

	authMW(http.HandlerFunc(h.Me)).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMeWithValidSession(t *testing.T) {
	db := setupAuthTestDB(t)
	svc := newTestAuthService(db)
	h := NewAuthHandler(svc, false)
	authMW := middleware.AuthMiddleware(svc)
	registerUser(t, svc, "alice", "secret")
	session, err := svc.Login("alice", "secret")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: session.Token})
	w := httptest.NewRecorder()

	authMW(http.HandlerFunc(h.Me)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["username"] != "alice" {
		t.Errorf("expected username 'alice', got %v", resp["username"])
	}
	if csrf, ok := resp["csrf_token"].(string); !ok || csrf == "" {
		t.Error("expected non-empty csrf_token in /api/auth/me response")
	}
}

// --- AC-1: logout revokes session ---

func TestLogoutRevokesSession(t *testing.T) {
	db := setupAuthTestDB(t)
	svc := newTestAuthService(db)
	h := NewAuthHandler(svc, false)
	authMW := middleware.AuthMiddleware(svc)
	csrfMW := middleware.CSRFMiddleware
	registerUser(t, svc, "alice", "secret")

	session, err := svc.Login("alice", "secret")
	if err != nil {
		t.Fatal(err)
	}

	// Logout with correct CSRF token
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: "session_token", Value: session.Token})
	logoutReq.Header.Set("X-CSRF-Token", session.CSRFToken)
	logoutW := httptest.NewRecorder()

	authMW(csrfMW(http.HandlerFunc(h.Logout))).ServeHTTP(logoutW, logoutReq)

	if logoutW.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", logoutW.Code, logoutW.Body.String())
	}

	// Reusing the old session token must now be rejected
	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(&http.Cookie{Name: "session_token", Value: session.Token})
	meW := httptest.NewRecorder()

	authMW(http.HandlerFunc(h.Me)).ServeHTTP(meW, meReq)

	if meW.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 after logout, got %d", meW.Code)
	}
}

// --- CSRF: session-bound validation ---

func TestCSRFRejectsInvalidToken(t *testing.T) {
	db := setupAuthTestDB(t)
	svc := newTestAuthService(db)
	authMW := middleware.AuthMiddleware(svc)
	csrfMW := middleware.CSRFMiddleware
	registerUser(t, svc, "alice", "secret")
	session, _ := svc.Login("alice", "secret")

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: session.Token})
	req.Header.Set("X-CSRF-Token", "definitely-wrong-token")
	w := httptest.NewRecorder()

	authMW(csrfMW(handler)).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("handler must not be called with invalid CSRF token")
	}
}

func TestCSRFRejectsMissingToken(t *testing.T) {
	db := setupAuthTestDB(t)
	svc := newTestAuthService(db)
	authMW := middleware.AuthMiddleware(svc)
	csrfMW := middleware.CSRFMiddleware
	registerUser(t, svc, "alice", "secret")
	session, _ := svc.Login("alice", "secret")

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: session.Token})
	// No X-CSRF-Token header
	w := httptest.NewRecorder()

	authMW(csrfMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCSRFAcceptsSessionBoundToken(t *testing.T) {
	db := setupAuthTestDB(t)
	svc := newTestAuthService(db)
	authMW := middleware.AuthMiddleware(svc)
	csrfMW := middleware.CSRFMiddleware
	registerUser(t, svc, "alice", "secret")
	session, _ := svc.Login("alice", "secret")

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: session.Token})
	req.Header.Set("X-CSRF-Token", session.CSRFToken)
	w := httptest.NewRecorder()

	authMW(csrfMW(handler)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !called {
		t.Error("handler must be called with valid CSRF token")
	}
}

func TestCSRFTokenIsBoundToSession(t *testing.T) {
	// A token from session A must not work for session B
	db := setupAuthTestDB(t)
	svc := newTestAuthService(db)
	authMW := middleware.AuthMiddleware(svc)
	csrfMW := middleware.CSRFMiddleware
	registerUser(t, svc, "alice", "secret")
	registerUser(t, svc, "bob", "secret2")

	sessionA, _ := svc.Login("alice", "secret")
	sessionB, _ := svc.Login("bob", "secret2")

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: sessionB.Token})
	req.Header.Set("X-CSRF-Token", sessionA.CSRFToken) // token from a different session
	w := httptest.NewRecorder()

	authMW(csrfMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 when using another session's CSRF token, got %d", w.Code)
	}
}
