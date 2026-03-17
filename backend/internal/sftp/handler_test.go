package sftp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/webssh/manager/internal/auth"
	"github.com/webssh/manager/internal/database"
	"github.com/webssh/manager/internal/middleware"
	"github.com/webssh/manager/internal/models"
	cryptoutil "github.com/webssh/manager/pkg/crypto"
	gossh "golang.org/x/crypto/ssh"

	sftppkg "github.com/pkg/sftp"
)

var sftpTestMasterKey = []byte("sftp-test-master-key-32bytes-ok!")

// withUser injects a user into request context.
func withUser(r *http.Request, user *models.User) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserContextKey, user)
	return r.WithContext(ctx)
}

func setupSFTPTestDB(t *testing.T) (*SFTPHandler, *auth.Service, *models.User) {
	t.Helper()
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	svc := auth.NewService(db, sftpTestMasterKey)
	svc.Register("alice", "pass")
	session, _ := svc.Login("alice", "pass")
	user, _ := svc.ValidateSession(session.Token)

	return NewSFTPHandler(db), svc, user
}

// insertSFTPNode creates a DB credential + node pointing to host:port for the given user.
func insertSFTPNode(t *testing.T, db *sql.DB, user *models.User, host string, port int) int {
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
		"INSERT INTO nodes (user_id, name, host, port, username, credential_id) VALUES (?, 'sftp-node', ?, ?, 'testuser', ?)",
		user.ID, host, port, credID,
	)
	if err != nil {
		t.Fatal(err)
	}
	nodeID, _ := nr.LastInsertId()
	return int(nodeID)
}

// --- In-process SSH/SFTP test server ---

// testSFTPServer is a minimal in-process SSH server that serves the SFTP subsystem
// backed by a real temp directory.
type testSFTPServer struct {
	listener net.Listener
	dir      string
	config   *gossh.ServerConfig
}

func startTestSFTPServer(t *testing.T) *testSFTPServer {
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

	s := &testSFTPServer{
		listener: l,
		dir:      t.TempDir(),
		config:   cfg,
	}
	t.Cleanup(func() { l.Close() })
	go s.serve()
	return s
}

func (s *testSFTPServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *testSFTPServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *testSFTPServer) handleConn(conn net.Conn) {
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

func (s *testSFTPServer) handleSession(newChan gossh.NewChannel) {
	ch, reqs, err := newChan.Accept()
	if err != nil {
		return
	}

	for req := range reqs {
		if req.Type != "subsystem" {
			if req.WantReply {
				req.Reply(false, nil)
			}
			continue
		}

		// Parse subsystem name from payload: uint32 length + name bytes
		if len(req.Payload) < 4 {
			if req.WantReply {
				req.Reply(false, nil)
			}
			continue
		}
		nameLen := binary.BigEndian.Uint32(req.Payload[0:4])
		if int(nameLen) > len(req.Payload)-4 {
			if req.WantReply {
				req.Reply(false, nil)
			}
			continue
		}
		name := string(req.Payload[4 : 4+nameLen])

		if name != "sftp" {
			if req.WantReply {
				req.Reply(false, nil)
			}
			continue
		}

		req.Reply(true, nil)
		srv, srvErr := sftppkg.NewServer(ch)
		if srvErr != nil {
			ch.Close()
			return
		}
		srv.Serve()
		ch.Close()
		return
	}

	ch.Close()
}

// --- Auth rejection tests ---

func TestSFTPList_Unauthorized(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := NewSFTPHandler(db)

	req := httptest.NewRequest(http.MethodGet, "/api/sftp/list?nodeId=1&path=.", nil)
	// No user in context
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSFTPDownload_Unauthorized(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := NewSFTPHandler(db)

	req := httptest.NewRequest(http.MethodGet, "/api/sftp/download?nodeId=1&path=file.txt", nil)
	w := httptest.NewRecorder()
	h.Download(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSFTPDelete_Unauthorized(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := NewSFTPHandler(db)

	req := httptest.NewRequest(http.MethodDelete, "/api/sftp/delete?nodeId=1&path=file.txt", nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSFTPMkdir_Unauthorized(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := NewSFTPHandler(db)

	req := httptest.NewRequest(http.MethodPost, "/api/sftp/mkdir", nil)
	w := httptest.NewRecorder()
	h.Mkdir(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// --- Missing / invalid nodeId ---

func TestSFTPList_MissingNodeID(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sftp/list?path=.", nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing nodeId, got %d", w.Code)
	}
}

func TestSFTPDownload_MissingNodeID(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sftp/download?path=file.txt", nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.Download(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing nodeId, got %d", w.Code)
	}
}

// --- SSH connection failure (node points to unreachable host) ---

func TestSFTPList_SSHConnectionFailure(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	svc := auth.NewService(db, sftpTestMasterKey)
	svc.Register("alice", "pass")
	session, _ := svc.Login("alice", "pass")
	user, _ := svc.ValidateSession(session.Token)

	// Insert a node pointing to 127.0.0.1:1 (no SSH server)
	userKey, _ := base64.StdEncoding.DecodeString(user.EncryptionKey)
	encrypted, _ := cryptoutil.Encrypt("sshpass", userKey)
	credResult, err := db.Exec(
		"INSERT INTO credentials (user_id, auth_type, encrypted_value) VALUES (?, 'password', ?)",
		user.ID, encrypted,
	)
	if err != nil {
		t.Fatal(err)
	}
	credID, _ := credResult.LastInsertId()
	nodeResult, err := db.Exec(
		"INSERT INTO nodes (user_id, name, host, port, username, credential_id) VALUES (?, 'bad-node', '127.0.0.1', 1, 'root', ?)",
		user.ID, credID,
	)
	if err != nil {
		t.Fatal(err)
	}
	nodeIDRaw, _ := nodeResult.LastInsertId()
	nodeID := int(nodeIDRaw)

	h := NewSFTPHandler(db)
	req := httptest.NewRequest(http.MethodGet, "/api/sftp/list?nodeId="+intS(nodeID)+"&path=.", nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for SSH connection failure, got %d", w.Code)
	}
	// Response must not expose internal details
	body := w.Body.String()
	if body == "" {
		t.Error("expected user-friendly error message in response body")
	}
}

func TestSFTPDownload_SSHConnectionFailure(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	svc := auth.NewService(db, sftpTestMasterKey)
	svc.Register("alice", "pass")
	session, _ := svc.Login("alice", "pass")
	user, _ := svc.ValidateSession(session.Token)

	userKey, _ := base64.StdEncoding.DecodeString(user.EncryptionKey)
	encrypted, _ := cryptoutil.Encrypt("sshpass", userKey)
	credResult, _ := db.Exec(
		"INSERT INTO credentials (user_id, auth_type, encrypted_value) VALUES (?, 'password', ?)",
		user.ID, encrypted,
	)
	credID, _ := credResult.LastInsertId()
	nodeResult, _ := db.Exec(
		"INSERT INTO nodes (user_id, name, host, port, username, credential_id) VALUES (?, 'bad-node', '127.0.0.1', 1, 'root', ?)",
		user.ID, credID,
	)
	nodeIDRaw, _ := nodeResult.LastInsertId()
	nodeID := int(nodeIDRaw)

	h := NewSFTPHandler(db)
	req := httptest.NewRequest(http.MethodGet, "/api/sftp/download?nodeId="+intS(nodeID)+"&path=file.txt", nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.Download(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for SSH connection failure, got %d", w.Code)
	}
}

// --- Happy-path tests via in-process SFTP server ---

func TestSFTPList_Success(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)
	srv := startTestSFTPServer(t)
	host, portStr, _ := net.SplitHostPort(srv.Addr())
	port, _ := strconv.Atoi(portStr)
	nodeID := insertSFTPNode(t, h.db, user, host, port)

	// Create a file in the server's temp dir
	if err := os.WriteFile(filepath.Join(srv.dir, "hello.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet,
		"/api/sftp/list?nodeId="+intS(nodeID)+"&path="+url.QueryEscape(srv.dir), nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var files []FileInfo
	if err := json.Unmarshal(w.Body.Bytes(), &files); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}
	found := false
	for _, f := range files {
		if f.Name == "hello.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("hello.txt not found in listing: %+v", files)
	}
}

func TestSFTPDownload_Success(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)
	srv := startTestSFTPServer(t)
	host, portStr, _ := net.SplitHostPort(srv.Addr())
	port, _ := strconv.Atoi(portStr)
	nodeID := insertSFTPNode(t, h.db, user, host, port)

	filePath := filepath.Join(srv.dir, "download.txt")
	if err := os.WriteFile(filePath, []byte("file content"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet,
		"/api/sftp/download?nodeId="+intS(nodeID)+"&path="+url.QueryEscape(filePath), nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.Download(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "file content" {
		t.Errorf("expected body %q, got %q", "file content", got)
	}
}

func TestSFTPUpload_Success(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)
	srv := startTestSFTPServer(t)
	host, portStr, _ := net.SplitHostPort(srv.Addr())
	port, _ := strconv.Atoi(portStr)
	nodeID := insertSFTPNode(t, h.db, user, host, port)

	destPath := filepath.Join(srv.dir, "uploaded.txt")

	// Build multipart/form-data body
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("nodeId", intS(nodeID))
	mw.WriteField("path", destPath)
	fw, _ := mw.CreateFormFile("file", "uploaded.txt")
	io.WriteString(fw, "uploaded content")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/sftp/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.Upload(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("uploaded file not found on disk: %v", err)
	}
	if string(got) != "uploaded content" {
		t.Errorf("expected %q, got %q", "uploaded content", string(got))
	}
}

func TestSFTPDelete_Success(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)
	srv := startTestSFTPServer(t)
	host, portStr, _ := net.SplitHostPort(srv.Addr())
	port, _ := strconv.Atoi(portStr)
	nodeID := insertSFTPNode(t, h.db, user, host, port)

	filePath := filepath.Join(srv.dir, "todelete.txt")
	if err := os.WriteFile(filePath, []byte("bye"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete,
		"/api/sftp/delete?nodeId="+intS(nodeID)+"&path="+url.QueryEscape(filePath), nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted, but it still exists")
	}
}

func TestSFTPMkdir_Success(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)
	srv := startTestSFTPServer(t)
	host, portStr, _ := net.SplitHostPort(srv.Addr())
	port, _ := strconv.Atoi(portStr)
	nodeID := insertSFTPNode(t, h.db, user, host, port)

	newDir := filepath.Join(srv.dir, "newsubdir")

	body := strings.NewReader(`{"node_id":` + intS(nodeID) + `,"path":"` + newDir + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/sftp/mkdir", body)
	req.Header.Set("Content-Type", "application/json")
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.Mkdir(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatalf("expected directory to be created, stat failed: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected %s to be a directory", newDir)
	}
}

func TestSFTPList_NonExistentPath(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)
	srv := startTestSFTPServer(t)
	host, portStr, _ := net.SplitHostPort(srv.Addr())
	port, _ := strconv.Atoi(portStr)
	nodeID := insertSFTPNode(t, h.db, user, host, port)

	req := httptest.NewRequest(http.MethodGet,
		"/api/sftp/list?nodeId="+intS(nodeID)+"&path="+url.QueryEscape("/nonexistent/path/that/does/not/exist"), nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for non-existent path, got %d", w.Code)
	}
	// Error message must be user-facing and not expose internal stack trace
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty error message")
	}
	// Must not contain raw filesystem paths or Go error internals
	if strings.Contains(body, "os.PathError") || strings.Contains(body, "syscall") {
		t.Errorf("response leaks internal error details: %q", body)
	}
}

func intS(i int) string {
	if i <= 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
