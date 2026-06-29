package ssh

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
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
	user, _ := svc.ValidateSession(session.Session.Token)

	req := httptest.NewRequest(http.MethodGet, "/ws/terminal", nil)
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
	user, _ := svc.ValidateSession(session.Session.Token)

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
	user, _ := svc.ValidateSession(session.Session.Token)

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

	svc.Register("alice", "pass1")
	svc.Register("bob", "pass2")
	sessionAlice, _ := svc.Login("alice", "pass1")
	sessionBob, _ := svc.Login("bob", "pass2")
	userAlice, _ := svc.ValidateSession(sessionAlice.Session.Token)
	userBob, _ := svc.ValidateSession(sessionBob.Session.Token)

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

	h := NewTerminalHandler(db, svc)
	req := httptest.NewRequest(http.MethodGet, "/ws/terminal?nodeId="+intToStr(int(nodeID)), nil)
	req = requestWithUser(req, userBob)
	w := httptest.NewRecorder()
	h.HandleWebSocket(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for ownership violation, got %d", w.Code)
	}
}

// --- Fake SSH session implementation (no real network sockets) ---

// fakeSSHSession is an in-memory sshSession for testing.
// It writes "$ " to stdout (then EOF) and records WindowChange calls.
type fakeSSHSession struct {
	mu      sync.Mutex
	resizes []windowSizeRecord
}

type windowSizeRecord struct{ rows, cols int }

// nopWriteCloser discards writes and is a no-op on Close.
type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }

func (s *fakeSSHSession) RequestPty(_ string, _, _ int, _ gossh.TerminalModes) error {
	return nil
}
func (s *fakeSSHSession) Shell() error { return nil }
func (s *fakeSSHSession) StdinPipe() (io.WriteCloser, error) {
	return nopWriteCloser{}, nil
}
func (s *fakeSSHSession) StdoutPipe() (io.Reader, error) {
	// Returns "$ " followed by EOF so the handler goroutine can read and then exit cleanly.
	return strings.NewReader("$ "), nil
}
func (s *fakeSSHSession) StderrPipe() (io.Reader, error) {
	return strings.NewReader(""), nil // immediate EOF
}
func (s *fakeSSHSession) WindowChange(rows, cols int) error {
	s.mu.Lock()
	s.resizes = append(s.resizes, windowSizeRecord{rows, cols})
	s.mu.Unlock()
	return nil
}
func (s *fakeSSHSession) Close() error { return nil }
func (s *fakeSSHSession) Setenv(name, value string) error { return nil }

func (s *fakeSSHSession) recordedResizes() []windowSizeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]windowSizeRecord, len(s.resizes))
	copy(cp, s.resizes)
	return cp
}

// fakeSSHClientConn wraps a fakeSSHSession to implement sshClientConn.
type fakeSSHClientConn struct{ sess *fakeSSHSession }

func (c *fakeSSHClientConn) NewSession() (sshSession, error) { return c.sess, nil }
func (c *fakeSSHClientConn) Close() error                    { return nil }

// fakeDial returns a dialer that immediately returns the given client.
func fakeDial(client sshClientConn) sshDialFunc {
	return func(_, _ string, _ *gossh.ClientConfig) (sshClientConn, error) {
		return client, nil
	}
}

// failDial returns a dialer that always fails with the given error.
func failDial(msg string) sshDialFunc {
	return func(_, _ string, _ *gossh.ClientConfig) (sshClientConn, error) {
		return nil, fmt.Errorf("%s", msg)
	}
}

// insertNode creates a DB node pointing to any host:port for the given user.
// Used for tests that need a valid node row but don't make real SSH connections.
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

// --- SSH dial failure via real WebSocket (uses httptest.NewServer, falls back to IPv6) ---

func TestHandleWebSocket_SSHDialFailure(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := auth.NewService(db, termTestMasterKey)

	svc.Register("alice", "pass1")
	session, _ := svc.Login("alice", "pass1")
	user, _ := svc.ValidateSession(session.Session.Token)

	nodeID := insertNode(t, db, user, "127.0.0.1", 22)

	h := NewTerminalHandler(db, svc)
	h.dialSSH = failDial("connection refused")

	authMW := middleware.AuthMiddleware(svc)
	srv := httptest.NewServer(authMW(http.HandlerFunc(h.HandleWebSocket)))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") +
		"/ws/terminal?nodeId=" + intToStr(nodeID)
	header := http.Header{"Cookie": []string{"session_token=" + session.Session.Token}}

	conn, resp, dialErr := websocket.DefaultDialer.Dial(wsURL, header)
	if dialErr != nil {
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			t.Logf("WebSocket dial got HTTP %d (SSH failure before upgrade)", resp.StatusCode)
			return
		}
		t.Fatalf("unexpected dial error: %v", dialErr)
	}
	defer conn.Close()

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read from WebSocket failed: %v", err)
	}
	if !strings.Contains(string(msg), "SSH connection failed") &&
		!strings.Contains(string(msg), "failed") {
		t.Errorf("expected SSH failure message, got: %q", string(msg))
	}
}

// --- Resize propagation test (uses fake SSH session — no net.Listen) ---

func TestHandleWebSocket_ResizePropagation(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := auth.NewService(db, termTestMasterKey)

	svc.Register("alice", "pass1")
	session, _ := svc.Login("alice", "pass1")
	user, _ := svc.ValidateSession(session.Session.Token)

	nodeID := insertNode(t, db, user, "127.0.0.1", 22)

	fakeSess := &fakeSSHSession{}
	h := NewTerminalHandler(db, svc)
	h.dialSSH = fakeDial(&fakeSSHClientConn{sess: fakeSess})

	authMW := middleware.AuthMiddleware(svc)
	srv := httptest.NewServer(authMW(http.HandlerFunc(h.HandleWebSocket)))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/terminal?nodeId=" + intToStr(nodeID)
	header := http.Header{"Cookie": []string{"session_token=" + session.Session.Token}}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	// Read the initial "$ " prompt forwarded from fake stdout
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read initial prompt: %v", err)
	}
	conn.SetReadDeadline(time.Time{})

	// Send terminal resize message: [0x00, rowsHi, rowsLo, colsHi, colsLo]
	rows, cols := 40, 120
	resizeMsg := []byte{0, byte(rows >> 8), byte(rows), byte(cols >> 8), byte(cols)}
	if err := conn.WriteMessage(websocket.BinaryMessage, resizeMsg); err != nil {
		t.Fatalf("failed to send resize message: %v", err)
	}

	// Allow time for the message to flow through the handler goroutine
	time.Sleep(100 * time.Millisecond)

	resizes := fakeSess.recordedResizes()
	if len(resizes) == 0 {
		t.Fatal("expected at least one resize event, got none")
	}
	last := resizes[len(resizes)-1]
	if last.rows != rows || last.cols != cols {
		t.Errorf("resize: got rows=%d cols=%d, want rows=%d cols=%d",
			last.rows, last.cols, rows, cols)
	}
}

// --- Concurrent sessions test (uses fake SSH session — no net.Listen) ---

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
	userAlice, _ := svc.ValidateSession(sessAlice.Session.Token)
	userBob, _ := svc.ValidateSession(sessBob.Session.Token)

	nodeAlice := insertNode(t, db, userAlice, "127.0.0.1", 22)
	nodeBob := insertNode(t, db, userBob, "127.0.0.1", 22)

	h := NewTerminalHandler(db, svc)
	// Each dial call creates a fresh fakeSSHSession so sessions are independent.
	h.dialSSH = func(_, _ string, _ *gossh.ClientConfig) (sshClientConn, error) {
		return &fakeSSHClientConn{sess: &fakeSSHSession{}}, nil
	}

	authMW := middleware.AuthMiddleware(svc)
	srv := httptest.NewServer(authMW(http.HandlerFunc(h.HandleWebSocket)))
	t.Cleanup(srv.Close)

	baseURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/terminal?nodeId="

	// Connect Alice's session
	hdrAlice := http.Header{"Cookie": []string{"session_token=" + sessAlice.Session.Token}}
	connAlice, _, err := websocket.DefaultDialer.Dial(baseURL+intToStr(nodeAlice), hdrAlice)
	if err != nil {
		t.Fatalf("Alice dial failed: %v", err)
	}
	defer connAlice.Close()

	// Connect Bob's session while Alice is still connected
	hdrBob := http.Header{"Cookie": []string{"session_token=" + sessBob.Session.Token}}
	connBob, _, err := websocket.DefaultDialer.Dial(baseURL+intToStr(nodeBob), hdrBob)
	if err != nil {
		t.Fatalf("Bob dial failed: %v", err)
	}
	defer connBob.Close()

	// Both sessions should independently receive the fake "$ " initial message
	type result struct {
		name string
		msg  []byte
		err  error
	}
	results := make(chan result, 2)

	var wg sync.WaitGroup
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

// --- Proxy / dialTarget tests ---

// TestDialTarget_NoProxy verifies that dialTarget with an empty ProxyType
// attempts a plain TCP connection to the target address (and fails with a
// connection-refused error when nothing is listening, not a proxy error).
func TestDialTarget_NoProxy(t *testing.T) {
	// Pick an ephemeral port that is almost certainly not listening.
	cfg := NodeDialConfig{
		TargetHost: "127.0.0.1",
		TargetPort: 1, // port 1 is never open in tests
		ProxyType:  "",
	}
	conn, err := dialTarget(cfg)
	if err == nil {
		conn.Close()
		t.Fatal("expected an error dialing port 1, got none")
	}
	// The error should mention a connection issue, not a proxy issue.
	errStr := err.Error()
	if contains(errStr, "proxy") || contains(errStr, "socks") {
		t.Errorf("expected plain TCP error, got proxy-related error: %v", err)
	}
}

// TestHTTPConnectDial_Non200 verifies that httpConnectDial returns an error
// when the proxy returns a non-200 status code (e.g. 407 Proxy Auth Required).
func TestHTTPConnectDial_Non200(t *testing.T) {
	// Start a test TCP server that responds to CONNECT with 407.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test listener: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		// Drain the CONNECT request then reply with 407.
		buf := make([]byte, 4096)
		c.Read(buf)
		c.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n"))
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	conn, err := httpConnectDial(host, port, "", "", "10.0.0.1", 22)
	if err == nil {
		conn.Close()
		t.Fatal("expected error for non-200 proxy response, got none")
	}
	if !contains(err.Error(), "407") {
		t.Errorf("expected 407 in error message, got: %v", err)
	}
	<-done
}

// TestDialTarget_JumpNoSSHConfig verifies that dialTarget with ProxyType="jump"
// returns an error containing "jump host SSH config is required" when JumpSSHConfig is nil.
func TestDialTarget_JumpNoSSHConfig(t *testing.T) {
	cfg := NodeDialConfig{
		TargetHost:    "127.0.0.1",
		TargetPort:    22,
		ProxyType:     "jump",
		ProxyHost:     "127.0.0.1",
		ProxyPort:     1, // port 1 — unlikely to be listening
		JumpSSHConfig: nil,
	}
	conn, err := dialTarget(cfg)
	if err == nil {
		conn.Close()
		t.Fatal("expected an error when JumpSSHConfig is nil, got none")
	}
	errStr := err.Error()
	if !contains(errStr, "jump host SSH config is required") && !contains(errStr, "dial jump host") {
		t.Errorf("expected jump-related error, got: %v", err)
	}
}

// TestDialTarget_JumpUnreachable verifies that dialTarget with ProxyType="jump"
// and a real JumpSSHConfig but an unreachable jump address returns a connection error.
func TestDialTarget_JumpUnreachable(t *testing.T) {
	// Build a minimal ssh.ClientConfig for the jump host (it won't actually connect).
	jumpCfg := &gossh.ClientConfig{
		User:            "testuser",
		Auth:            []gossh.AuthMethod{gossh.Password("testpass")},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}
	cfg := NodeDialConfig{
		TargetHost:    "127.0.0.1",
		TargetPort:    22,
		ProxyType:     "jump",
		ProxyHost:     "127.0.0.1",
		ProxyPort:     1, // port 1 — not listening, connection refused
		JumpSSHConfig: jumpCfg,
	}
	conn, err := dialTarget(cfg)
	if err == nil {
		conn.Close()
		t.Fatal("expected an error dialing unreachable jump host, got none")
	}
	// Expect a connection error, not a nil-config error.
	if !contains(err.Error(), "dial jump host") && !contains(err.Error(), "connection refused") &&
		!contains(err.Error(), "connect") {
		t.Errorf("expected a connection error, got: %v", err)
	}
}

// TestDialTarget_JumpSuccess verifies that dialTarget with ProxyType="jump"
// succeeds when both the jump host and target are real (fake) SSH servers.
func TestDialTarget_JumpSuccess(t *testing.T) {
	// Generate an ephemeral host key for both fake SSH servers.
	hostKey := generateTestSigner(t)
	jumpHostKey := generateTestSigner(t)

	// Start a fake target SSH server.
	targetLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("target listen: %v", err)
	}
	defer targetLn.Close()
	go serveFakeSSH(targetLn, hostKey, "targetpass")

	// Start a fake jump SSH server.
	jumpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("jump listen: %v", err)
	}
	defer jumpLn.Close()
	go serveFakeSSH(jumpLn, jumpHostKey, "jumppass")

	jumpHost, jumpPortStr, _ := net.SplitHostPort(jumpLn.Addr().String())
	jumpPort, _ := strconv.Atoi(jumpPortStr)
	targetHost, targetPortStr, _ := net.SplitHostPort(targetLn.Addr().String())
	targetPort, _ := strconv.Atoi(targetPortStr)

	cfg := NodeDialConfig{
		TargetHost:    targetHost,
		TargetPort:    targetPort,
		ProxyType:     "jump",
		ProxyHost:     jumpHost,
		ProxyPort:     jumpPort,
		ProxyUsername: "jumpuser",
		JumpSSHConfig: &gossh.ClientConfig{
			User:            "jumpuser",
			Auth:            []gossh.AuthMethod{gossh.Password("jumppass")},
			HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec
		},
	}

	conn, err := dialTarget(cfg)
	if err != nil {
		t.Fatalf("expected successful jump dial, got error: %v", err)
	}
	conn.Close()
}

// TestDialTarget_JumpWrongCredentials verifies that dialTarget with ProxyType="jump"
// and wrong jump credentials returns an authentication failure error.
func TestDialTarget_JumpWrongCredentials(t *testing.T) {
	// Generate an ephemeral host key for the fake jump SSH server.
	jumpHostKey := generateTestSigner(t)

	// Start a fake jump SSH server that only accepts "correctpass".
	jumpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("jump listen: %v", err)
	}
	defer jumpLn.Close()
	go serveFakeSSH(jumpLn, jumpHostKey, "correctpass")

	jumpHost, jumpPortStr, _ := net.SplitHostPort(jumpLn.Addr().String())
	jumpPort, _ := strconv.Atoi(jumpPortStr)

	cfg := NodeDialConfig{
		TargetHost:    "127.0.0.1",
		TargetPort:    22,
		ProxyType:     "jump",
		ProxyHost:     jumpHost,
		ProxyPort:     jumpPort,
		ProxyUsername: "jumpuser",
		JumpSSHConfig: &gossh.ClientConfig{
			User:            "jumpuser",
			Auth:            []gossh.AuthMethod{gossh.Password("wrongpass")},
			HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec
			Timeout:         5 * time.Second,
		},
	}

	conn, err := dialTarget(cfg)
	if err == nil {
		conn.Close()
		t.Fatal("expected auth failure with wrong jump credentials, got nil error")
	}
	errStr := err.Error()
	if !contains(errStr, "handshake") && !contains(errStr, "auth") &&
		!contains(errStr, "unable to authenticate") && !contains(errStr, "jump host") {
		t.Errorf("expected auth-related error, got: %v", err)
	}
}

// generateTestSigner creates a new ECDSA P-256 private key and wraps it as a
// gossh.Signer, for use as a test SSH server host key.
func generateTestSigner(t *testing.T) gossh.Signer {
	t.Helper()
	priv, err := ecdsaP256Key()
	if err != nil {
		t.Fatalf("generate ecdsa key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	return signer
}

// ecdsaP256Key generates a new ECDSA P-256 private key using crypto/rand.
func ecdsaP256Key() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// serveFakeSSH runs a minimal SSH server on ln that accepts password auth
// with the given acceptedPassword. It handles channel open requests by
// immediately sending EOF so that callers can verify connectivity.
func serveFakeSSH(ln net.Listener, hostKey gossh.Signer, acceptedPassword string) {
	cfg := &gossh.ServerConfig{
		PasswordCallback: func(c gossh.ConnMetadata, pass []byte) (*gossh.Permissions, error) {
			if string(pass) == acceptedPassword {
				return nil, nil
			}
			return nil, fmt.Errorf("bad password")
		},
	}
	cfg.AddHostKey(hostKey)

	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleFakeSSHConn(conn, cfg)
	}
}

// handleFakeSSHConn performs the SSH handshake and drains incoming channel
// requests, responding with success to allow jump-host tunnelling tests.
func handleFakeSSHConn(conn net.Conn, cfg *gossh.ServerConfig) {
	sshConn, chans, reqs, err := gossh.NewServerConn(conn, cfg)
	if err != nil {
		conn.Close()
		return
	}
	defer sshConn.Close()
	go gossh.DiscardRequests(reqs)
	for newChan := range chans {
		ch, chanReqs, err := newChan.Accept()
		if err != nil {
			continue
		}
		go gossh.DiscardRequests(chanReqs)
		ch.Close()
	}
}

// contains is a simple case-sensitive substring check used in tests.
func contains(s, sub string) bool {
	return len(sub) <= len(s) && (sub == "" || func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
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
