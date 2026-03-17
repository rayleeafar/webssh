package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webssh/manager/internal/middleware"
	"github.com/webssh/manager/internal/models"
	"github.com/webssh/manager/pkg/crypto"
	_ "github.com/mattn/go-sqlite3"
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

// AC Test: User cannot access nodes without authentication
func TestListNodesUnauthorized(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	handler := NewNodeHandler(db)
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	w := httptest.NewRecorder()

	handler.List(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

// AC Test: User can only see their own nodes
func TestListNodesOnlyOwnNodes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create two users
	user1ID := createTestUser(t, db, "user1", "pass1")
	user2ID := createTestUser(t, db, "user2", "pass2")

	// Create nodes for both users
	createTestNode(t, db, user1ID, "user1-node", "192.168.1.1")
	createTestNode(t, db, user2ID, "user2-node", "192.168.1.2")

	handler := NewNodeHandler(db)
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	req = addUserToContext(req, user1ID, "user1", "dGVzdGtleQ==")
	w := httptest.NewRecorder()

	handler.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var nodes []NodeResponse
	json.NewDecoder(w.Body).Decode(&nodes)

	if len(nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(nodes))
	}
	if len(nodes) > 0 && nodes[0].Name != "user1-node" {
		t.Errorf("Expected node name 'user1-node', got '%s'", nodes[0].Name)
	}
}

// AC Test: User cannot update another user's node
func TestUpdateNodeForbidden(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user1ID := createTestUser(t, db, "user1", "pass1")
	user2ID := createTestUser(t, db, "user2", "pass2")
	nodeID := createTestNode(t, db, user1ID, "user1-node", "192.168.1.1")

	handler := NewNodeHandler(db)
	body := bytes.NewBufferString(`{"name":"hacked","host":"192.168.1.1","port":22,"username":"root"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/nodes/1", body)
	req = addUserToContext(req, user2ID, "user2", "dGVzdGtleQ==")
	w := httptest.NewRecorder()

	handler.Update(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	// Verify node was not modified
	var name string
	db.QueryRow("SELECT name FROM nodes WHERE id = ?", nodeID).Scan(&name)
	if name != "user1-node" {
		t.Errorf("Node name should not have changed, got '%s'", name)
	}
}

// AC Test: User cannot delete another user's node
func TestDeleteNodeForbidden(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user1ID := createTestUser(t, db, "user1", "pass1")
	user2ID := createTestUser(t, db, "user2", "pass2")
	nodeID := createTestNode(t, db, user1ID, "user1-node", "192.168.1.1")

	handler := NewNodeHandler(db)
	req := httptest.NewRequest(http.MethodDelete, "/api/nodes/1", nil)
	req = addUserToContext(req, user2ID, "user2", "dGVzdGtleQ==")
	w := httptest.NewRecorder()

	handler.Delete(w, req)

	// Should return 404 because the WHERE clause filters by user_id
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	// Verify node still exists
	var count int
	db.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", nodeID).Scan(&count)
	if count != 1 {
		t.Errorf("Node should still exist, got count %d", count)
	}
}

// Helper functions
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	schema := `
	CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		encryption_key_salt TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		host TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 22,
		username TEXT NOT NULL,
		auth_type TEXT NOT NULL DEFAULT 'password',
		encrypted_credentials TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	return db
}

func createTestUser(t *testing.T, db *sql.DB, username, password string) int {
	result, err := db.Exec(
		"INSERT INTO users (username, password_hash, encryption_key_salt) VALUES (?, ?, ?)",
		username, "hash", "salt",
	)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	id, _ := result.LastInsertId()
	return int(id)
}

func createTestNode(t *testing.T, db *sql.DB, userID int, name, host string) int {
	key := []byte("0123456789abcdef0123456789abcdef")
	encrypted, _ := crypto.Encrypt("password", key)

	result, err := db.Exec(
		"INSERT INTO nodes (user_id, name, host, port, username, auth_type, encrypted_credentials) VALUES (?, ?, ?, ?, ?, ?, ?)",
		userID, name, host, 22, "root", "password", encrypted,
	)
	if err != nil {
		t.Fatalf("Failed to create test node: %v", err)
	}
	id, _ := result.LastInsertId()
	return int(id)
}

func addUserToContext(req *http.Request, userID int, username, encryptionKey string) *http.Request {
	user := &models.User{
		ID:            userID,
		Username:      username,
		EncryptionKey: encryptionKey,
	}
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, user)
	return req.WithContext(ctx)
}

