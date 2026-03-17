package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/webssh/manager/internal/auth"
	"github.com/webssh/manager/internal/database"
	"github.com/webssh/manager/pkg/crypto"
)

// testEncKeyB64 is a valid base64-encoded 32-byte AES-256 key for use in tests.
var testEncKeyB64 = base64.StdEncoding.EncodeToString(make([]byte, 32))

func setupNodeCRUDDB(t *testing.T) (*NodeHandler, int) {
	t.Helper()
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	userID := createTestUser(t, db, "alice", "pass")
	return NewNodeHandler(db), userID
}

// nodeCtxReq creates a request with a 32-byte encryption key in context.
func nodeCtxReq(method, path string, body []byte, userID int) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.RequestURI = path
	return addUserToContext(req, userID, "alice", testEncKeyB64)
}

// --- AC-2: Node CRUD success paths ---

func TestCreateNodeSuccess(t *testing.T) {
	h, userID := setupNodeCRUDDB(t)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "my-server", "host": "192.168.1.1", "port": 22, "username": "root", "password": "sshpass",
	})
	req := nodeCtxReq(http.MethodPost, "/api/nodes", body, userID)
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp NodeResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Name != "my-server" {
		t.Errorf("expected name 'my-server', got '%s'", resp.Name)
	}
	if resp.ID == 0 {
		t.Error("expected non-zero node ID")
	}
}

func TestListNodesAfterCreate(t *testing.T) {
	h, userID := setupNodeCRUDDB(t)

	// Create two nodes
	for _, name := range []string{"srv1", "srv2"} {
		body, _ := json.Marshal(map[string]interface{}{
			"name": name, "host": "192.168.1.1", "port": 22, "username": "root", "password": "x",
		})
		req := nodeCtxReq(http.MethodPost, "/api/nodes", body, userID)
		w := httptest.NewRecorder()
		h.Create(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s failed: %d %s", name, w.Code, w.Body.String())
		}
	}

	// List
	listReq := nodeCtxReq(http.MethodGet, "/api/nodes", nil, userID)
	listW := httptest.NewRecorder()
	h.List(listW, listReq)

	if listW.Code != http.StatusOK {
		t.Fatalf("list failed: %d", listW.Code)
	}
	var nodes []NodeResponse
	json.NewDecoder(listW.Body).Decode(&nodes)
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestUpdateNodeSuccess(t *testing.T) {
	h, userID := setupNodeCRUDDB(t)

	// Create node using raw DB access (createTestNode) then update via handler
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	uid := createTestUser(t, db, "bob", "pass")
	nodeID := createTestNode(t, db, uid, "original", "192.168.1.1")
	handler := NewNodeHandler(db)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "updated", "host": "192.168.1.2", "port": 2222, "username": "admin",
	})
	path := "/api/nodes/" + strconv.Itoa(nodeID)
	req := addUserToContext(
		func() *http.Request {
			r := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.RequestURI = path
			return r
		}(),
		uid, "bob", testEncKeyB64,
	)
	w := httptest.NewRecorder()
	handler.Update(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("update failed: %d %s", w.Code, w.Body.String())
	}

	// Verify the change
	listReq := addUserToContext(httptest.NewRequest(http.MethodGet, "/api/nodes", nil), uid, "bob", testEncKeyB64)
	listW := httptest.NewRecorder()
	handler.List(listW, listReq)
	var nodes []NodeResponse
	json.NewDecoder(listW.Body).Decode(&nodes)
	if len(nodes) != 1 || nodes[0].Name != "updated" {
		t.Errorf("expected updated node, got %+v", nodes)
	}

	_ = h
	_ = userID
}

func TestDeleteNodeSuccess(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	userID := createTestUser(t, db, "alice", "pass")
	nodeID := createTestNode(t, db, userID, "toDelete", "192.168.1.1")
	handler := NewNodeHandler(db)

	path := "/api/nodes/" + strconv.Itoa(nodeID)
	req := addUserToContext(
		func() *http.Request {
			r := httptest.NewRequest(http.MethodDelete, path, nil)
			r.RequestURI = path
			return r
		}(),
		userID, "alice", testEncKeyB64,
	)
	w := httptest.NewRecorder()
	handler.Delete(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("delete failed: %d %s", w.Code, w.Body.String())
	}

	// Confirm gone
	listReq := addUserToContext(httptest.NewRequest(http.MethodGet, "/api/nodes", nil), userID, "alice", testEncKeyB64)
	listW := httptest.NewRecorder()
	handler.List(listW, listReq)
	var nodes []NodeResponse
	json.NewDecoder(listW.Body).Decode(&nodes)
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes after delete, got %d", len(nodes))
	}
}

// --- AC-6 regression: DB contents alone are insufficient to decrypt credentials ---

func TestCredentialEncryptionDBInsufficientForDecrypt(t *testing.T) {
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := auth.NewService(db, testMasterKey)

	if _, err := svc.Register("alice", "mypassword"); err != nil {
		t.Fatal(err)
	}
	session, err := svc.Login("alice", "mypassword")
	if err != nil {
		t.Fatal(err)
	}

	// Read the raw wrapped key from DB
	var rawWrappedKey string
	if err := db.QueryRow("SELECT encryption_key FROM sessions WHERE token = ?",
		session.Token).Scan(&rawWrappedKey); err != nil {
		t.Fatal(err)
	}

	// Get the real (unwrapped) key via ValidateSession
	user, err := svc.ValidateSession(session.Token)
	if err != nil {
		t.Fatal(err)
	}
	realKey, err := base64.StdEncoding.DecodeString(user.EncryptionKey)
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt a test credential with the real key
	ciphertext, err := crypto.Encrypt("super-secret-ssh-password", realKey)
	if err != nil {
		t.Fatal(err)
	}

	// The raw DB value is AES-GCM-encrypted ciphertext, not a valid AES key.
	// Attempting to decrypt the credential using it must fail.
	rawBytes, _ := base64.StdEncoding.DecodeString(rawWrappedKey)
	_, decryptErr := crypto.Decrypt(ciphertext, rawBytes)
	if decryptErr == nil {
		t.Error("expected decryption to fail with raw DB wrapped key — DB alone should be insufficient to decrypt credentials")
	}

	// With the real unwrapped key, decryption must succeed.
	plaintext, err := crypto.Decrypt(ciphertext, realKey)
	if err != nil {
		t.Errorf("decryption with real key must succeed: %v", err)
	}
	if plaintext != "super-secret-ssh-password" {
		t.Errorf("expected 'super-secret-ssh-password', got '%s'", plaintext)
	}

	// Also verify the raw DB key is different from the real key (wrapped ≠ original)
	rawKeyB64 := rawWrappedKey
	if rawKeyB64 == user.EncryptionKey {
		t.Error("sessions.encryption_key must be wrapped (different from the real key)")
	}
}
