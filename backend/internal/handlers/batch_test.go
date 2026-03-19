package handlers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/webssh/manager/internal/database"
	"github.com/webssh/manager/pkg/crypto"
	gossh "golang.org/x/crypto/ssh"
)

// TestBatchExec_RejectsEmptyNodeIds verifies that the handler returns 400 when
// nodeIds is an empty array.
func TestBatchExec_RejectsEmptyNodeIds(t *testing.T) {
	h := NewBatchHandler(nil)
	body := strings.NewReader(`{"nodeIds":[],"command":"id","timeout":5}`)
	req := httptest.NewRequest(http.MethodPost, "/api/batch/exec", body)
	req = addUserToContext(req, 1, "alice", "dGVzdGtleQ==")
	w := httptest.NewRecorder()
	h.Exec(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestBatchExec_RejectsEmptyCommand verifies that the handler returns 400 when
// command is an empty string.
func TestBatchExec_RejectsEmptyCommand(t *testing.T) {
	h := NewBatchHandler(nil)
	body := strings.NewReader(`{"nodeIds":[1],"command":"","timeout":5}`)
	req := httptest.NewRequest(http.MethodPost, "/api/batch/exec", body)
	req = addUserToContext(req, 1, "alice", "dGVzdGtleQ==")
	w := httptest.NewRecorder()
	h.Exec(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestBatchExec_JumpProxyAuthTypeIsolation is the key regression test for the
// bug where runCommandOnNode used nc.authType (the TARGET node's auth type)
// instead of nc.proxyAuthType (the jump credential's own auth type) when
// building JumpSSHConfig.
//
// The test wires up a fake SSH server that only accepts password
// "jump-secret-pass".  The target node and its jump host are given separate
// credentials with different passwords.  If the old bug were present the
// handler would forward the target's password to the jump server — causing an
// authentication error.  With the fix in place the jump handshake succeeds and
// the only error is about being unable to reach the (deliberately
// unreachable) target.
func TestBatchExec_JumpProxyAuthTypeIsolation(t *testing.T) {
	// Encryption key: 32 ASCII bytes, distinct from the all-zero testEncKeyB64.
	encKey := []byte("0123456789abcdef0123456789abcdef")
	encKeyB64 := base64.StdEncoding.EncodeToString(encKey)

	// Start a fake jump SSH server that only accepts the jump password.
	jumpHost, jumpPort := startFakeSSHServer(t, "jump-secret-pass")

	// Full-schema in-memory DB.
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	aliceID := createTestUser(t, db, "alice", "pass")

	// Target credential — password that the jump server must NOT accept.
	encTargetPass, err := crypto.Encrypt("target-pass", encKey)
	if err != nil {
		t.Fatalf("encrypt target cred: %v", err)
	}
	res, err := db.Exec(
		"INSERT INTO credentials (user_id, auth_type, encrypted_value) VALUES (?, 'password', ?)",
		aliceID, encTargetPass,
	)
	if err != nil {
		t.Fatalf("insert target credential: %v", err)
	}
	targetCredID, _ := res.LastInsertId()

	// Jump credential — the password the fake server accepts.
	encJumpPass, err := crypto.Encrypt("jump-secret-pass", encKey)
	if err != nil {
		t.Fatalf("encrypt jump cred: %v", err)
	}
	res, err = db.Exec(
		"INSERT INTO credentials (user_id, auth_type, encrypted_value) VALUES (?, 'password', ?)",
		aliceID, encJumpPass,
	)
	if err != nil {
		t.Fatalf("insert jump credential: %v", err)
	}
	jumpCredID, _ := res.LastInsertId()

	// Node: target port 1 so the final dial to the target is refused; proxy
	// points at the fake SSH server.
	res, err = db.Exec(`
		INSERT INTO nodes
			(user_id, name, host, port, username, credential_id,
			 proxy_type, proxy_host, proxy_port, proxy_username, proxy_credential_id)
		VALUES (?, 'testnode', '127.0.0.1', 1, 'root', ?,
		        'jump', ?, ?, 'jumpuser', ?)`,
		aliceID, targetCredID,
		jumpHost, jumpPort, jumpCredID,
	)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID, _ := res.LastInsertId()

	// Build and send the batch exec request.
	reqBody := fmt.Sprintf(`{"nodeIds":[%d],"command":"id","timeout":5}`, nodeID)
	req := httptest.NewRequest(http.MethodPost, "/api/batch/exec", strings.NewReader(reqBody))
	req = addUserToContext(req, aliceID, "alice", encKeyB64)
	w := httptest.NewRecorder()
	NewBatchHandler(db).Exec(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var results []BatchNodeResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	errMsg := strings.ToLower(results[0].Error)

	// The jump authentication must have SUCCEEDED.  Any "authentication" or
	// "unable to authenticate" string indicates the old bug is present.
	if strings.Contains(errMsg, "authentication") || strings.Contains(errMsg, "unable to authenticate") {
		t.Errorf("jump authentication failed — proxyAuthType bug may be present: %s", results[0].Error)
	}

	// We must have gotten past the jump host: the error should reflect a failure
	// to reach the actual target, not a jump-level problem.
	if results[0].Error == "" {
		t.Errorf("expected an error (target is unreachable) but got none")
	}

	hasTargetFailure := strings.Contains(errMsg, "connect") ||
		strings.Contains(errMsg, "refused") ||
		strings.Contains(errMsg, "dial") ||
		strings.Contains(errMsg, "i/o timeout") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "connection") ||
		strings.Contains(errMsg, "ssh")
	if !hasTargetFailure {
		t.Errorf("expected a target-connection error, got: %s", results[0].Error)
	}
}

// startFakeSSHServer starts a minimal in-process SSH server on a random
// loopback port that accepts only the given password.  It returns the host and
// port to dial.  The server is shut down via t.Cleanup.
func startFakeSSHServer(t *testing.T, acceptedPassword string) (host string, port int) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}

	cfg := &gossh.ServerConfig{
		PasswordCallback: func(_ gossh.ConnMetadata, pass []byte) (*gossh.Permissions, error) {
			if string(pass) == acceptedPassword {
				return nil, nil
			}
			return nil, fmt.Errorf("bad password")
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go func(c net.Conn) {
				sshConn, chans, reqs, err := gossh.NewServerConn(c, cfg)
				if err != nil {
					c.Close()
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
			}(conn)
		}
	}()

	h, p, _ := net.SplitHostPort(ln.Addr().String())
	portNum, _ := strconv.Atoi(p)
	return h, portNum
}
