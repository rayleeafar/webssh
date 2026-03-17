package ssh

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/webssh/manager/internal/auth"
	"github.com/webssh/manager/internal/database"
	"github.com/webssh/manager/internal/middleware"
	"github.com/webssh/manager/internal/models"
	cryptoutil "github.com/webssh/manager/pkg/crypto"
	gossh "golang.org/x/crypto/ssh"
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

// --- In-process SSH test server ---

// windowChangeMsg records a terminal resize event received by the test SSH server.
type windowChangeMsg struct {
	Rows uint32
	Cols uint32
}

// testSSHServer is a minimal in-process SSH server for terminal handler tests.
// It accepts any password credential and records terminal resize events.
type testSSHServer struct {
	listener net.Listener
	config   *gossh.ServerConfig
	mu       sync.Mutex
	resizes  []windowChangeMsg
}

func startTestSSHServer(t *testing.T) *testSSHServer {
	t.Helper()

	hostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &gossh.ServerConfig{
		PasswordCallback: func(_ gossh.ConnMetadata, _ []byte) (*gossh.Permissions, error) {
			return &gossh.Permissions{}, nil
		},
	}
	cfg.AddHostKey(signer)

	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	s := &testSSHServer{listener: l, config: cfg}
	t.Cleanup(func() { l.Close() })
	go s.serve()
	return s
}

func (s *testSSHServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *testSSHServer) RecordedResizes() []windowChangeMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]windowChangeMsg, len(s.resizes))
	copy(out, s.resizes)
	return out
}

func (s *testSSHServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *testSSHServer) handleConn(conn net.Conn) {
	sshConn, chans, reqs, err := gossh.NewServerConn(conn, s.config)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go gossh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(gossh.UnknownChannelType, "")
			continue
		}
		go s.handleSession(newChan)
	}
}

func (s *testSSHServer) handleSession(newChan gossh.NewChannel) {
	ch, reqs, err := newChan.Accept()
	if err != nil {
		return
	}
	defer ch.Close()

	for req := range reqs {
		switch req.Type {
		case "pty-req":
			req.Reply(true, nil)
		case "window-change":
			// RFC 4254 / golang.org/x/crypto/ssh ptyWindowChangeMsg layout:
			// [0:4] Columns (uint32 BE), [4:8] Rows (uint32 BE)
			if len(req.Payload) >= 8 {
				cols := binary.BigEndian.Uint32(req.Payload[0:4])
				rows := binary.BigEndian.Uint32(req.Payload[4:8])
				s.mu.Lock()
				s.resizes = append(s.resizes, windowChangeMsg{Rows: rows, Cols: cols})
				s.mu.Unlock()
			}
			if req.WantReply {
				req.Reply(true, nil)
			}
		case "shell":
			req.Reply(true, nil)
			// Write an initial prompt so the WebSocket client has something to read.
			go func() {
				_, _ = ch.Write([]byte("$ "))
				io.Copy(io.Discard, ch)
			}()
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

// startIPv4Server creates an httptest.Server bound explicitly to 127.0.0.1 (IPv4).
func startIPv4Server(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = l
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// insertNode creates a DB node pointing to host:port for the given user.
func insertNode(t *testing.T, db *sql.DB, user *models.User, host string, port int) int {
	t.Helper()
	key, _ := base64.StdEncoding.DecodeString(user.EncryptionKey)
	enc, _ := cryptoutil.Encrypt("testpass", key)
	cr, err := db.Exec(
		"INSERT INTO credentials (user_id, auth_type, encrypted_value) VALUES (?, 'password', ?)",
		user.ID, enc,
	)
	if err != nil {
		t.Fatal(err)
	}
	credID, _ := cr.LastInsertId()
	nr, err := db.Exec(
		"INSERT INTO nodes (user_id, name, host, port, username, credential_id) VALUES (?, 'test-node', ?, ?, 'testuser', ?)",
		user.ID, host, port, credID,
	)
	if err != nil {
		t.Fatal(err)
	}
	nodeID, _ := nr.LastInsertId()
	return int(nodeID)
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

	h := NewTerminalHandler(db, svc)
	authMW := middleware.AuthMiddleware(svc)
	// Use IPv4-only listener to avoid IPv6 loopback binding in constrained environments.
	srv := startIPv4Server(t, authMW(http.HandlerFunc(h.HandleWebSocket)))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") +
		"/ws/terminal?nodeId=" + intToStr(int(nodeID))

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

// --- Resize propagation test ---

func TestHandleWebSocket_ResizePropagation(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := auth.NewService(db, termTestMasterKey)

	svc.Register("alice", "pass1")
	session, _ := svc.Login("alice", "pass1")
	user, _ := svc.ValidateSession(session.Token)

	sshSrv := startTestSSHServer(t)
	host, portStr, _ := net.SplitHostPort(sshSrv.Addr())
	port, _ := strconv.Atoi(portStr)

	nodeID := insertNode(t, db, user, host, port)

	h := NewTerminalHandler(db, svc)
	authMW := middleware.AuthMiddleware(svc)
	srv := startIPv4Server(t, authMW(http.HandlerFunc(h.HandleWebSocket)))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/terminal?nodeId=" + intToStr(nodeID)
	header := http.Header{"Cookie": []string{"session_token=" + session.Token}}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	// Read the initial shell prompt written by the test SSH server
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read initial prompt: %v", err)
	}
	conn.SetReadDeadline(time.Time{}) // reset

	// Send a terminal resize message: [0x00, rowsHi, rowsLo, colsHi, colsLo]
	// The terminal handler decodes: rows = msg[1]<<8|msg[2], cols = msg[3]<<8|msg[4]
	rows, cols := 40, 120
	resizeMsg := []byte{0, byte(rows >> 8), byte(rows), byte(cols >> 8), byte(cols)}
	if err := conn.WriteMessage(websocket.BinaryMessage, resizeMsg); err != nil {
		t.Fatalf("failed to send resize message: %v", err)
	}

	// Allow time for the message to propagate through the SSH connection
	time.Sleep(300 * time.Millisecond)

	resizes := sshSrv.RecordedResizes()
	if len(resizes) == 0 {
		t.Fatal("expected at least one resize event on the SSH server, got none")
	}
	last := resizes[len(resizes)-1]
	if int(last.Rows) != rows || int(last.Cols) != cols {
		t.Errorf("resize: got rows=%d cols=%d, want rows=%d cols=%d", last.Rows, last.Cols, rows, cols)
	}
}

// --- Concurrent sessions test ---

// TestHandleWebSocket_ConcurrentSessions verifies that two simultaneous WebSocket
// terminal sessions for different users operate independently without interference.
func TestHandleWebSocket_ConcurrentSessions(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := auth.NewService(db, termTestMasterKey)

	svc.Register("alice", "pass1")
	svc.Register("bob", "pass2")
	sessAlice, _ := svc.Login("alice", "pass1")
	sessBob, _ := svc.Login("bob", "pass2")
	userAlice, _ := svc.ValidateSession(sessAlice.Token)
	userBob, _ := svc.ValidateSession(sessBob.Token)

	sshSrv := startTestSSHServer(t)
	host, portStr, _ := net.SplitHostPort(sshSrv.Addr())
	port, _ := strconv.Atoi(portStr)

	nodeAlice := insertNode(t, db, userAlice, host, port)
	nodeBob := insertNode(t, db, userBob, host, port)

	h := NewTerminalHandler(db, svc)
	authMW := middleware.AuthMiddleware(svc)
	srv := startIPv4Server(t, authMW(http.HandlerFunc(h.HandleWebSocket)))

	baseURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/terminal?nodeId="

	// Connect Alice's session
	hdrAlice := http.Header{"Cookie": []string{"session_token=" + sessAlice.Token}}
	connAlice, respAlice, dialErr := websocket.DefaultDialer.Dial(baseURL+intToStr(nodeAlice), hdrAlice)
	if dialErr != nil {
		if respAlice != nil {
			t.Fatalf("Alice dial failed with HTTP %d: %v", respAlice.StatusCode, dialErr)
		}
		t.Fatalf("Alice dial failed: %v", dialErr)
	}
	defer connAlice.Close()

	// Connect Bob's session while Alice is still connected
	hdrBob := http.Header{"Cookie": []string{"session_token=" + sessBob.Token}}
	connBob, respBob, dialErr := websocket.DefaultDialer.Dial(baseURL+intToStr(nodeBob), hdrBob)
	if dialErr != nil {
		if respBob != nil {
			t.Fatalf("Bob dial failed with HTTP %d: %v", respBob.StatusCode, dialErr)
		}
		t.Fatalf("Bob dial failed: %v", dialErr)
	}
	defer connBob.Close()

	// Both sessions should receive an initial message from the SSH server independently
	var wg sync.WaitGroup
	type result struct {
		name string
		msg  []byte
		err  error
	}
	results := make(chan result, 2)

	readOne := func(name string, conn *websocket.Conn) {
		defer wg.Done()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, msg, err := conn.ReadMessage()
		results <- result{name: name, msg: msg, err: err}
	}

	wg.Add(2)
	go readOne("alice", connAlice)
	go readOne("bob", connBob)
	wg.Wait()
	close(results)

	for r := range results {
		if r.err != nil {
			t.Errorf("%s: read failed: %v", r.name, r.err)
		} else if len(r.msg) == 0 {
			t.Errorf("%s: expected non-empty initial SSH message", r.name)
		}
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
