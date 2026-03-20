package sftp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/webssh/manager/internal/auth"
	"github.com/webssh/manager/internal/database"
	"github.com/webssh/manager/internal/middleware"
	"github.com/webssh/manager/internal/models"
	cryptoutil "github.com/webssh/manager/pkg/crypto"
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
	user, _ := svc.ValidateSession(session.Session.Token)

	return NewSFTPHandler(db), svc, user
}

// insertSFTPNode creates a DB credential + node for the given user.
// Tests that exercise the real getSFTPClient path (SSH failure) need a real node row.
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

// --- Fake SFTP client (in-memory, no network sockets) ---

// fakeFileInfo implements os.FileInfo.
type fakeFileInfo struct {
	name  string
	size  int64
	mode  os.FileMode
	isDir bool
}

func (f *fakeFileInfo) Name() string      { return f.name }
func (f *fakeFileInfo) Size() int64       { return f.size }
func (f *fakeFileInfo) Mode() os.FileMode { return f.mode }
func (f *fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f *fakeFileInfo) IsDir() bool       { return f.isDir }
func (f *fakeFileInfo) Sys() interface{}  { return nil }

// fakeReadFile implements sftpReadFile backed by a string.
type fakeReadFile struct {
	*strings.Reader
	info os.FileInfo
}

func (f *fakeReadFile) Stat() (os.FileInfo, error) { return f.info, nil }
func (f *fakeReadFile) Close() error               { return nil }

// fakeWriteFile implements sftpWriteFile backed by a *bytes.Buffer.
type fakeWriteFile struct{ buf *bytes.Buffer }

func (f *fakeWriteFile) Write(p []byte) (int, error) { return f.buf.Write(p) }
func (f *fakeWriteFile) Close() error                { return nil }

// fakeSFTPClient is a configurable in-memory SFTP client for tests.
type fakeSFTPClient struct {
	readDirFn    func(p string) ([]os.FileInfo, error)
	openFn       func(path string) (sftpReadFile, error)
	createFn     func(path string) (sftpWriteFile, error)
	statFn       func(p string) (os.FileInfo, error)
	removeFn     func(path string) error
	removeDirFn  func(path string) error
	mkdirFn      func(path string) error
}

func (c *fakeSFTPClient) ReadDir(p string) ([]os.FileInfo, error) {
	if c.readDirFn != nil {
		return c.readDirFn(p)
	}
	return nil, nil
}
func (c *fakeSFTPClient) Open(path string) (sftpReadFile, error) {
	if c.openFn != nil {
		return c.openFn(path)
	}
	return nil, fmt.Errorf("open: not configured")
}
func (c *fakeSFTPClient) Create(path string) (sftpWriteFile, error) {
	if c.createFn != nil {
		return c.createFn(path)
	}
	return nil, fmt.Errorf("create: not configured")
}
func (c *fakeSFTPClient) Stat(p string) (os.FileInfo, error) {
	if c.statFn != nil {
		return c.statFn(p)
	}
	return nil, fmt.Errorf("stat: not configured")
}
func (c *fakeSFTPClient) Remove(path string) error {
	if c.removeFn != nil {
		return c.removeFn(path)
	}
	return nil
}
func (c *fakeSFTPClient) RemoveDirectory(path string) error {
	if c.removeDirFn != nil {
		return c.removeDirFn(path)
	}
	return nil
}
func (c *fakeSFTPClient) Mkdir(path string) error {
	if c.mkdirFn != nil {
		return c.mkdirFn(path)
	}
	return nil
}

// injectFake replaces the handler's newClient with one that always returns fc.
func injectFake(h *SFTPHandler, fc sftpClientIF) {
	h.newClient = func(_, _ int, _ string) (sftpClientIF, func(), error) {
		return fc, func() {}, nil
	}
}

// injectFailure replaces the handler's newClient with one that always errors.
func injectFailure(h *SFTPHandler, msg string) {
	h.newClient = func(_, _ int, _ string) (sftpClientIF, func(), error) {
		return nil, nil, fmt.Errorf("%s", msg)
	}
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

// --- SSH connection failure (real getSFTPClient path via DB, port 1 → refused) ---

func TestSFTPList_SSHConnectionFailure(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	svc := auth.NewService(db, sftpTestMasterKey)
	svc.Register("alice", "pass")
	session, _ := svc.Login("alice", "pass")
	user, _ := svc.ValidateSession(session.Session.Token)

	nodeID := insertSFTPNode(t, db, user, "127.0.0.1", 1) // port 1 → connection refused

	h := NewSFTPHandler(db)
	req := httptest.NewRequest(http.MethodGet,
		"/api/sftp/list?nodeId="+intS(nodeID)+"&path=.", nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for SSH connection failure, got %d", w.Code)
	}
	if w.Body.String() == "" {
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
	user, _ := svc.ValidateSession(session.Session.Token)

	nodeID := insertSFTPNode(t, db, user, "127.0.0.1", 1)

	h := NewSFTPHandler(db)
	req := httptest.NewRequest(http.MethodGet,
		"/api/sftp/download?nodeId="+intS(nodeID)+"&path=file.txt", nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.Download(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for SSH connection failure, got %d", w.Code)
	}
}

// --- Happy-path tests via in-memory fake SFTP client (no network sockets) ---

func TestSFTPList_Success(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)
	fake := &fakeSFTPClient{
		readDirFn: func(p string) ([]os.FileInfo, error) {
			return []os.FileInfo{
				&fakeFileInfo{name: "hello.txt", size: 5, mode: 0644},
				&fakeFileInfo{name: "subdir", isDir: true, mode: os.ModeDir | 0755},
			}, nil
		},
	}
	injectFake(h, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/sftp/list?nodeId=1&path=/some/dir", nil)
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
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
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
	content := "file content"
	fake := &fakeSFTPClient{
		openFn: func(path string) (sftpReadFile, error) {
			info := &fakeFileInfo{name: "download.txt", size: int64(len(content))}
			return &fakeReadFile{Reader: strings.NewReader(content), info: info}, nil
		},
	}
	injectFake(h, fake)

	req := httptest.NewRequest(http.MethodGet,
		"/api/sftp/download?nodeId=1&path=/remote/download.txt", nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.Download(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != content {
		t.Errorf("expected body %q, got %q", content, got)
	}
}

func TestSFTPUpload_Success(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)
	var written bytes.Buffer
	fake := &fakeSFTPClient{
		createFn: func(path string) (sftpWriteFile, error) {
			return &fakeWriteFile{buf: &written}, nil
		},
	}
	injectFake(h, fake)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("nodeId", "1")
	mw.WriteField("path", "/remote/uploaded.txt")
	fw, _ := mw.CreateFormFile("file", "uploaded.txt")
	io.WriteString(fw, "uploaded content")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/sftp/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.Upload(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if got := written.String(); got != "uploaded content" {
		t.Errorf("expected uploaded content %q, got %q", "uploaded content", got)
	}
}

func TestSFTPDelete_Success(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)
	var removed string
	fake := &fakeSFTPClient{
		statFn: func(p string) (os.FileInfo, error) {
			return &fakeFileInfo{name: "todelete.txt", mode: 0644}, nil
		},
		removeFn: func(path string) error {
			removed = path
			return nil
		},
	}
	injectFake(h, fake)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/sftp/delete?nodeId=1&path=/remote/todelete.txt", nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if removed != "/remote/todelete.txt" {
		t.Errorf("expected Remove(%q), got %q", "/remote/todelete.txt", removed)
	}
}

func TestSFTPMkdir_Success(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)
	var created string
	fake := &fakeSFTPClient{
		mkdirFn: func(path string) error {
			created = path
			return nil
		},
	}
	injectFake(h, fake)

	body := strings.NewReader(`{"node_id":1,"path":"/remote/newdir"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/sftp/mkdir", body)
	req.Header.Set("Content-Type", "application/json")
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.Mkdir(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if created != "/remote/newdir" {
		t.Errorf("expected Mkdir(%q), got %q", "/remote/newdir", created)
	}
}

func TestSFTPList_NonExistentPath(t *testing.T) {
	h, _, user := setupSFTPTestDB(t)
	fake := &fakeSFTPClient{
		readDirFn: func(p string) ([]os.FileInfo, error) {
			return nil, fmt.Errorf("file does not exist")
		},
	}
	injectFake(h, fake)

	req := httptest.NewRequest(http.MethodGet,
		"/api/sftp/list?nodeId=1&path=/nonexistent/path", nil)
	req = withUser(req, user)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for non-existent path, got %d", w.Code)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty error message")
	}
	// Error response must not contain Go internal details
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
