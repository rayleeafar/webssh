package ssh

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/webssh/manager/internal/auth"
	"github.com/webssh/manager/internal/database"
	"github.com/webssh/manager/internal/middleware"
	"github.com/webssh/manager/internal/models"
	cryptoutil "github.com/webssh/manager/pkg/crypto"
)

var termTestMasterKey = []byte("terminal-test-master-key-32bytes")

func setupTerminalTestDB(t *testing.T) (*TerminalHandler, *auth.Service) {
	t.Helper()
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	svc := auth.NewService(db, termTestMasterKey)
	return NewTerminalHandler(db, svc), svc
}

// requestWithUser injects a user into context for direct handler tests.
func requestWithUser(r *http.Request, user *models.User) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserContextKey, user)
	return r.WithContext(ctx)
}

// --- Rejection tests (before WebSocket upgrade) ---

func TestHandleWebSocket_MissingNodeID(t *testing.T) {
	h, svc := setupTerminalTestDB(t)
	// Register and login to get a real user
	svc.Register("alice", "pass")
	session, _ := svc.Login("alice", "pass")
	user, _ := svc.ValidateSession(session.Token)

	req := httptest.NewRequest(http.MethodGet, "/ws/terminal", nil)
	// No nodeId param
	req = requestWithUser(req, user)
	w := httptest.NewRecorder()

	h.HandleWebSocket(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing nodeId, got %d", w.Code)
	}
}

func TestHandleWebSocket_InvalidNodeID(t *testing.T) {
	h, svc := setupTerminalTestDB(t)
	svc.Register("alice", "pass")
	session, _ := svc.Login("alice", "pass")
	user, _ := svc.ValidateSession(session.Token)

	req := httptest.NewRequest(http.MethodGet, "/ws/terminal?nodeId=notanumber", nil)
	req = requestWithUser(req, user)
	w := httptest.NewRecorder()

	h.HandleWebSocket(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid nodeId, got %d", w.Code)
	}
}

func TestHandleWebSocket_NodeNotFound(t *testing.T) {
	h, svc := setupTerminalTestDB(t)
	svc.Register("alice", "pass")
	session, _ := svc.Login("alice", "pass")
	user, _ := svc.ValidateSession(session.Token)

	req := httptest.NewRequest(http.MethodGet, "/ws/terminal?nodeId=99999", nil)
	req = requestWithUser(req, user)
	w := httptest.NewRecorder()

	h.HandleWebSocket(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent nodeId, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWebSocket_OwnershipViolation(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := auth.NewService(db, termTestMasterKey)

	// Register two users
	svc.Register("alice", "pass1")
	svc.Register("bob", "pass2")

	sessionAlice, _ := svc.Login("alice", "pass1")
	sessionBob, _ := svc.Login("bob", "pass2")
	userAlice, _ := svc.ValidateSession(sessionAlice.Token)
	userBob, _ := svc.ValidateSession(sessionBob.Token)

	// Create a node owned by Alice
	aliceKey, _ := base64.StdEncoding.DecodeString(userAlice.EncryptionKey)
	encrypted, _ := cryptoutil.Encrypt("sshpass", aliceKey)
	credResult, _ := db.Exec(
		"INSERT INTO credentials (user_id, auth_type, encrypted_value) VALUES (?, 'password', ?)",
		userAlice.ID, encrypted,
	)
	credID, _ := credResult.LastInsertId()
	nodeResult, _ := db.Exec(
		"INSERT INTO nodes (user_id, name, host, port, username, credential_id) VALUES (?, 'alice-node', '127.0.0.1', 22, 'root', ?)",
		userAlice.ID, credID,
	)
	nodeID, _ := nodeResult.LastInsertId()

	// Bob tries to access Alice's node
	h := NewTerminalHandler(db, svc)
	req := httptest.NewRequest(http.MethodGet, "/ws/terminal?nodeId="+intToStr(int(nodeID)), nil)
	req = requestWithUser(req, userBob)
	w := httptest.NewRecorder()

	h.HandleWebSocket(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for ownership violation, got %d", w.Code)
	}
}

// --- SSH dial failure via real WebSocket ---

func TestHandleWebSocket_SSHDialFailure(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := auth.NewService(db, termTestMasterKey)

	svc.Register("alice", "pass1")
	session, _ := svc.Login("alice", "pass1")
	user, _ := svc.ValidateSession(session.Token)

	// Create a node pointing to a port that has no SSH server
	userKey, _ := base64.StdEncoding.DecodeString(user.EncryptionKey)
	encrypted, _ := cryptoutil.Encrypt("sshpass", userKey)
	credResult, _ := db.Exec(
		"INSERT INTO credentials (user_id, auth_type, encrypted_value) VALUES (?, 'password', ?)",
		user.ID, encrypted,
	)
	credID, _ := credResult.LastInsertId()
	nodeResult, _ := db.Exec(
		// Port 1 is reserved and almost never has a service
		"INSERT INTO nodes (user_id, name, host, port, username, credential_id) VALUES (?, 'no-ssh', '127.0.0.1', 1, 'root', ?)",
		user.ID, credID,
	)
	nodeID, _ := nodeResult.LastInsertId()

	// Build a test HTTP server with auth middleware + terminal handler
	h := NewTerminalHandler(db, svc)
	authMW := middleware.AuthMiddleware(svc)
	srv := httptest.NewServer(authMW(http.HandlerFunc(h.HandleWebSocket)))
	t.Cleanup(srv.Close)

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") +
		"/ws/terminal?nodeId=" + intToStr(int(nodeID))

	// Connect with session cookie
	header := http.Header{}
	header.Add("Cookie", "session_token="+session.Token)
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		// Some SSH failures may cause a non-101 response; that's acceptable
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			t.Logf("WebSocket dial got HTTP %d (SSH failure before upgrade may occur)", resp.StatusCode)
			return
		}
		t.Fatalf("unexpected dial error: %v", err)
	}
	defer conn.Close()

	// The handler dials SSH and writes a failure message to the WebSocket
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read from WebSocket failed: %v", err)
	}
	if !strings.Contains(string(msg), "SSH connection failed") &&
		!strings.Contains(string(msg), "connection refused") &&
		!strings.Contains(string(msg), "failed") {
		t.Errorf("expected SSH failure message, got: %q", string(msg))
	}
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
