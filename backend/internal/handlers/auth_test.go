package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterHandlerMethodNotAllowed(t *testing.T) {
	handler := &AuthHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/register", nil)
	w := httptest.NewRecorder()

	handler.Register(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestLoginHandlerMethodNotAllowed(t *testing.T) {
	handler := &AuthHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
	w := httptest.NewRecorder()

	handler.Login(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestLogoutHandlerMethodNotAllowed(t *testing.T) {
	handler := &AuthHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/logout", nil)
	w := httptest.NewRecorder()

	handler.Logout(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestMeHandlerMethodNotAllowed(t *testing.T) {
	handler := &AuthHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/me", nil)
	w := httptest.NewRecorder()

	handler.Me(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

