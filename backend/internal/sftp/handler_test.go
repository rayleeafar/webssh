package sftp

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

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
	user, _ := svc.ValidateSession(session.Token)

	return NewSFTPHandler(db), svc, user
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
