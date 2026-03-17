package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateNodeRequest(t *testing.T) {
	tests := []struct {
		name     string
		nodeName string
		host     string
		port     int
		username string
		wantErr  bool
	}{
		{"valid ipv4", "server1", "192.168.1.1", 22, "root", false},
		{"valid domain", "server2", "example.com", 22, "user", false},
		{"valid localhost", "local", "localhost", 22, "admin", false},
		{"empty name", "", "192.168.1.1", 22, "root", true},
		{"empty host", "server", "", 22, "root", true},
		{"invalid host", "server", "invalid..host", 22, "root", true},
		{"port too low", "server", "192.168.1.1", 0, "root", true},
		{"port too high", "server", "192.168.1.1", 70000, "root", true},
		{"empty username", "server", "192.168.1.1", 22, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNodeRequest(tt.nodeName, tt.host, tt.port, tt.username)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNodeRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsValidHost(t *testing.T) {
	tests := []struct {
		host  string
		valid bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"example.com", true},
		{"sub.example.com", true},
		{"localhost", true},
		{"invalid..host", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			result := isValidHost(tt.host)
			if result != tt.valid {
				t.Errorf("isValidHost(%q) = %v, want %v", tt.host, result, tt.valid)
			}
		})
	}
}

func TestCreateNodeMethodNotAllowed(t *testing.T) {
	handler := &NodeHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	w := httptest.NewRecorder()

	handler.Create(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}
