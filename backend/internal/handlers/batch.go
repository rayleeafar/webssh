package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/webssh/manager/internal/middleware"
	sshutil "github.com/webssh/manager/internal/ssh"
	"github.com/webssh/manager/pkg/crypto"
	gossh "golang.org/x/crypto/ssh"
)

// BatchHandler handles concurrent command execution across multiple nodes.
type BatchHandler struct {
	db *sql.DB
}

func NewBatchHandler(db *sql.DB) *BatchHandler {
	return &BatchHandler{db: db}
}

// BatchExecRequest describes a command to run on one or more nodes.
type BatchExecRequest struct {
	NodeIDs []int  `json:"nodeIds"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"` // seconds, 1–600; 0 defaults to 60
}

// BatchNodeResult holds the outcome of running the command on a single node.
type BatchNodeResult struct {
	NodeID   int    `json:"nodeId"`
	Host     string `json:"host"`
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	TimedOut bool   `json:"timedOut"`
	Error    string `json:"error,omitempty"`
}

// nodeCredentials holds the data needed to open an SSH connection to a node.
type nodeCredentials struct {
	nodeID         int
	host           string
	port           int
	username       string
	authType       string
	encryptedCreds string
}

// Exec handles POST /api/batch/exec — runs a command on all requested nodes
// concurrently and returns results as a JSON array.
func (h *BatchHandler) Exec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req BatchExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.NodeIDs) == 0 {
		http.Error(w, "nodeIds must not be empty", http.StatusBadRequest)
		return
	}
	if req.Command == "" {
		http.Error(w, "command must not be empty", http.StatusBadRequest)
		return
	}
	if req.Timeout > 600 {
		http.Error(w, "timeout must not exceed 600 seconds", http.StatusBadRequest)
		return
	}
	if req.Timeout < 1 {
		req.Timeout = 60
	}

	// Decode the user's encryption key once for all nodes.
	encKey, err := base64.StdEncoding.DecodeString(user.EncryptionKey)
	if err != nil {
		http.Error(w, "Key error", http.StatusInternalServerError)
		return
	}

	// Verify that every requested node belongs to the authenticated user and
	// collect the credentials needed to SSH into each one.
	nodeCreds := make([]nodeCredentials, 0, len(req.NodeIDs))
	for _, nid := range req.NodeIDs {
		var nc nodeCredentials
		nc.nodeID = nid
		queryErr := h.db.QueryRow(`
			SELECT n.host, n.port, n.username, c.auth_type, c.encrypted_value
			FROM nodes n
			JOIN credentials c ON n.credential_id = c.id
			WHERE n.id = ? AND n.user_id = ?`, nid, user.ID,
		).Scan(&nc.host, &nc.port, &nc.username, &nc.authType, &nc.encryptedCreds)
		if queryErr == sql.ErrNoRows {
			http.Error(w, fmt.Sprintf("node %d not found or not owned by user", nid), http.StatusForbidden)
			return
		}
		if queryErr != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		nodeCreds = append(nodeCreds, nc)
	}

	// Execute command concurrently on all nodes within the requested timeout.
	results := make([]BatchNodeResult, len(nodeCreds))
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(req.Timeout)*time.Second)
	defer cancel()

	for i, nc := range nodeCreds {
		wg.Add(1)
		go func(idx int, nc nodeCredentials) {
			defer wg.Done()
			results[idx] = runCommandOnNode(ctx, nc, req.Command, encKey)
		}(i, nc)
	}

	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		log.Printf("batch exec: encode response: %v", err)
	}
}

// dialResult carries the outcome of an asynchronous SSH dial.
type dialResult struct {
	client *gossh.Client
	err    error
}

// runCommandOnNode opens an SSH connection to nc, executes cmd, and returns the
// outcome. It respects ctx for timeout/cancellation.
func runCommandOnNode(ctx context.Context, nc nodeCredentials, cmd string, encKey []byte) BatchNodeResult {
	result := BatchNodeResult{
		NodeID: nc.nodeID,
		Host:   nc.host,
	}

	creds, err := crypto.Decrypt(nc.encryptedCreds, encKey)
	if err != nil {
		result.Error = fmt.Sprintf("decrypt credentials: %v", err)
		return result
	}

	authMethods, err := sshutil.BuildAuthMethod(nc.authType, creds)
	if err != nil {
		result.Error = fmt.Sprintf("build auth method: %v", err)
		return result
	}

	// Dial in a separate goroutine so that context cancellation is honoured.
	dialCh := make(chan dialResult, 1)
	go func() {
		c, e := gossh.Dial("tcp", fmt.Sprintf("%s:%d", nc.host, nc.port), &gossh.ClientConfig{
			User:            nc.username,
			Auth:            authMethods,
			HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec
		})
		dialCh <- dialResult{c, e}
	}()

	var client *gossh.Client
	select {
	case <-ctx.Done():
		result.TimedOut = true
		result.Error = "timed out waiting for SSH connection"
		return result
	case res := <-dialCh:
		if res.err != nil {
			result.Error = fmt.Sprintf("SSH dial: %v", res.err)
			return result
		}
		client = res.client
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		result.Error = fmt.Sprintf("open SSH session: %v", err)
		return result
	}

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	// Run the command in a goroutine so that context cancellation can interrupt it.
	runCh := make(chan error, 1)
	go func() {
		runCh <- sess.Run(cmd)
	}()

	select {
	case <-ctx.Done():
		sess.Close()
		result.TimedOut = true
		result.Error = "command timed out"
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()
	case runErr := <-runCh:
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()
		if runErr == nil {
			result.ExitCode = 0
		} else if exitErr, ok := runErr.(*gossh.ExitError); ok {
			result.ExitCode = exitErr.ExitStatus()
		} else {
			result.Error = fmt.Sprintf("run command: %v", runErr)
		}
	}

	return result
}
