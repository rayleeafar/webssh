package ssh

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/webssh/manager/internal/auth"
	"github.com/webssh/manager/internal/middleware"
	"github.com/webssh/manager/pkg/crypto"
	"golang.org/x/crypto/ssh"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
		if allowedOrigin == "" {
			return true // Allow all in development
		}
		return r.Header.Get("Origin") == allowedOrigin
	},
}

// sshSession is the interface for an SSH session used by HandleWebSocket.
type sshSession interface {
	RequestPty(term string, h, w int, modes ssh.TerminalModes) error
	Shell() error
	StdinPipe() (io.WriteCloser, error)
	StdoutPipe() (io.Reader, error)
	StderrPipe() (io.Reader, error)
	WindowChange(h, w int) error
	Close() error
}

// sshClientConn is the interface for an SSH client used by HandleWebSocket.
type sshClientConn interface {
	NewSession() (sshSession, error)
	Close() error
}

// defaultSSHClientConn wraps *ssh.Client to implement sshClientConn.
type defaultSSHClientConn struct{ c *ssh.Client }

func (d *defaultSSHClientConn) NewSession() (sshSession, error) { return d.c.NewSession() }
func (d *defaultSSHClientConn) Close() error                    { return d.c.Close() }

// sshDialFunc is the injectable function for dialing an SSH server.
type sshDialFunc func(network, addr string, config *ssh.ClientConfig) (sshClientConn, error)

func defaultSSHDial(network, addr string, config *ssh.ClientConfig) (sshClientConn, error) {
	c, err := ssh.Dial(network, addr, config)
	if err != nil {
		return nil, err
	}
	return &defaultSSHClientConn{c: c}, nil
}

// TerminalSession represents a persistent terminal session (local shell or remote SSH).
type TerminalSession struct {
	ID         string
	NodeID     int
	UserID     int
	Host       string
	db         *sql.DB
	decKey     []byte

	// Local shell process
	localCmd   *exec.Cmd
	ptyFile    *os.File

	// Remote SSH connection
	sshClient  sshClientConn
	sshSession sshSession

	// Common I/O
	mu         sync.Mutex
	stdin      io.WriteCloser

	// Active websocket connection
	connMu     sync.Mutex
	conn       *websocket.Conn

	// Reconnection & retry state
	reconnectMu sync.Mutex
	reconnecting bool
	retries      int
	status       string // "connected", "disconnected", "failed"

	// Output history buffer
	historyMu  sync.Mutex
	history    []byte

	// Closing
	Closed     bool
	once       sync.Once
}

func (s *TerminalSession) appendHistory(data []byte) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.history = append(s.history, data...)
	const maxHistory = 150 * 1024 // 150KB backlog
	if len(s.history) > maxHistory {
		s.history = s.history[len(s.history)-maxHistory:]
	}
}

func (s *TerminalSession) getHistory() []byte {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	cpy := make([]byte, len(s.history))
	copy(cpy, s.history)
	return cpy
}

func (s *TerminalSession) Broadcast(messageType int, data []byte) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if s.conn != nil {
		_ = s.conn.WriteMessage(messageType, data)
	}
}

func (s *TerminalSession) Attach(conn *websocket.Conn) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.conn = conn

	// Replay history backlog
	s.historyMu.Lock()
	backlog := make([]byte, len(s.history))
	copy(backlog, s.history)
	s.historyMu.Unlock()

	if len(backlog) > 0 {
		_ = conn.WriteMessage(websocket.BinaryMessage, backlog)
	}
}

func (s *TerminalSession) Close() {
	s.once.Do(func() {
		s.mu.Lock()
		s.Closed = true
		s.connMu.Lock()
		if s.conn != nil {
			_ = s.conn.Close()
		}
		s.connMu.Unlock()
		if s.stdin != nil {
			_ = s.stdin.Close()
		}
		if s.ptyFile != nil {
			_ = s.ptyFile.Close()
		}
		if s.localCmd != nil && s.localCmd.Process != nil {
			_ = s.localCmd.Process.Kill()
		}
		if s.sshSession != nil {
			_ = s.sshSession.Close()
		}
		if s.sshClient != nil {
			_ = s.sshClient.Close()
		}
		s.mu.Unlock()

		globalSessionManager.Remove(s.ID)
	})
}

func (s *TerminalSession) startLocalShell(cmd *exec.Cmd, ptyFile *os.File) {
	s.localCmd = cmd
	s.ptyFile = ptyFile
	s.stdin = ptyFile
	s.status = "connected"

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := ptyFile.Read(buf)
			if err != nil {
				s.Close()
				return
			}
			if n > 0 {
				data := buf[:n]
				s.appendHistory(data)
				s.Broadcast(websocket.BinaryMessage, data)
			}
		}
	}()
}

func (s *TerminalSession) startSSHShell(client sshClientConn, sess sshSession, stdin io.WriteCloser, stdout, stderr io.Reader) {
	s.sshClient = client
	s.sshSession = sess
	s.stdin = stdin
	s.status = "connected"

	var wg sync.WaitGroup
	wg.Add(2)

	readPipe := func(r io.Reader) {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := r.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				data := buf[:n]
				s.appendHistory(data)
				s.Broadcast(websocket.BinaryMessage, data)
			}
		}
	}

	go readPipe(stdout)
	go readPipe(stderr)

	go func() {
		wg.Wait()
		s.handleDisconnect()
	}()
}

func (s *TerminalSession) handleDisconnect() {
	s.reconnectMu.Lock()
	if s.Closed || s.reconnecting {
		s.reconnectMu.Unlock()
		return
	}
	s.reconnecting = true
	s.status = "disconnected"
	s.reconnectMu.Unlock()

	defer func() {
		s.reconnectMu.Lock()
		s.reconnecting = false
		s.reconnectMu.Unlock()
	}()

	msg := []byte("\r\n\r\n\x1b[33m[Gateway] Connection lost. Attempting to reconnect (Max 3 retries)...\x1b[0m\r\n")
	s.appendHistory(msg)
	s.Broadcast(websocket.BinaryMessage, msg)

	for s.retries = 1; s.retries <= 3; s.retries++ {
		s.mu.Lock()
		closed := s.Closed
		s.mu.Unlock()
		if closed {
			return
		}

		time.Sleep(5 * time.Second)

		err := s.attemptReconnect()
		if err == nil {
			s.reconnectMu.Lock()
			s.status = "connected"
			s.retries = 0
			s.reconnectMu.Unlock()

			successMsg := []byte("\r\n\x1b[32m[Gateway] Reconnected successfully.\x1b[0m\r\n\r\n")
			s.appendHistory(successMsg)
			s.Broadcast(websocket.BinaryMessage, successMsg)
			return
		}

		retryMsg := []byte(fmt.Sprintf("\x1b[33m[Gateway] Reconnect attempt %d/3 failed: %v\x1b[0m\r\n", s.retries, err))
		s.appendHistory(retryMsg)
		s.Broadcast(websocket.BinaryMessage, retryMsg)
	}

	s.reconnectMu.Lock()
	s.status = "failed"
	s.reconnectMu.Unlock()

	failedMsg := []byte("\r\n\x1b[31m[Gateway] Connection failed permanently. You can close this tab.\x1b[0m\r\n")
	s.appendHistory(failedMsg)
	s.Broadcast(websocket.BinaryMessage, failedMsg)
}

func (s *TerminalSession) attemptReconnect() error {
	var username, encryptedCreds, authType string
	var port, ownerID int
	var proxyType, proxyHost, proxyUsername string
	var proxyPort, proxyCredentialID int
	var jumpProxyType, jumpProxyHost string
	var jumpProxyPort, jumpProxyCredentialID int

	err := s.db.QueryRow(`
		SELECT n.port, n.username, c.auth_type, c.encrypted_value, n.user_id,
		       COALESCE(n.proxy_type,''), COALESCE(n.proxy_host,''), COALESCE(n.proxy_port,0),
		       COALESCE(n.proxy_username,''), COALESCE(n.proxy_credential_id,0),
		       COALESCE(n.jump_proxy_type,''), COALESCE(n.jump_proxy_host,''),
		       COALESCE(n.jump_proxy_port,0), COALESCE(n.jump_proxy_credential_id,0)
		FROM nodes n
		JOIN credentials c ON n.credential_id = c.id
		WHERE n.id = ?
	`, s.NodeID).Scan(&port, &username, &authType, &encryptedCreds, &ownerID,
		&proxyType, &proxyHost, &proxyPort, &proxyUsername, &proxyCredentialID,
		&jumpProxyType, &jumpProxyHost, &jumpProxyPort, &jumpProxyCredentialID)
	if err != nil {
		return fmt.Errorf("fetch node: %w", err)
	}

	if ownerID != s.UserID {
		return fmt.Errorf("forbidden")
	}

	credentials, err := crypto.Decrypt(encryptedCreds, s.decKey)
	if err != nil {
		return fmt.Errorf("decrypt credentials: %w", err)
	}

	authMethods, err := BuildAuthMethod(authType, credentials)
	if err != nil {
		return fmt.Errorf("build auth method: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         8 * time.Second,
	}

	var client sshClientConn
	if proxyType != "" {
		var proxyCreds string
		if proxyCredentialID > 0 {
			var proxyAuthType, proxyEncCreds string
			s.db.QueryRow(
				"SELECT auth_type, encrypted_value FROM credentials WHERE id = ? AND user_id = ?",
				proxyCredentialID, s.UserID,
			).Scan(&proxyAuthType, &proxyEncCreds)
			proxyCreds, _ = crypto.Decrypt(proxyEncCreds, s.decKey)
		}

		var jumpSSHConfig *ssh.ClientConfig
		if proxyType == "jump" && proxyCredentialID > 0 {
			var jumpAuthType, jumpEncCreds string
			s.db.QueryRow(
				"SELECT auth_type, encrypted_value FROM credentials WHERE id = ? AND user_id = ?",
				proxyCredentialID, s.UserID,
			).Scan(&jumpAuthType, &jumpEncCreds)
			if jumpEncCreds != "" {
				jumpCreds, _ := crypto.Decrypt(jumpEncCreds, s.decKey)
				jumpAuthMethods, _ := BuildAuthMethod(jumpAuthType, jumpCreds)
				jumpSSHConfig = &ssh.ClientConfig{
					User:            proxyUsername,
					Auth:            jumpAuthMethods,
					HostKeyCallback: ssh.InsecureIgnoreHostKey(),
				}
			}
		}

		var jumpProxyCreds string
		if jumpProxyType != "" && jumpProxyCredentialID > 0 {
			var jpAuthType, jpEncCreds string
			s.db.QueryRow(
				"SELECT auth_type, encrypted_value FROM credentials WHERE id = ? AND user_id = ?",
				jumpProxyCredentialID, s.UserID,
			).Scan(&jpAuthType, &jpEncCreds)
			if jpEncCreds != "" {
				jumpProxyCreds, _ = crypto.Decrypt(jpEncCreds, s.decKey)
			}
		}

		cfg := NodeDialConfig{
			TargetHost:     s.Host,
			TargetPort:     port,
			ProxyType:      proxyType,
			ProxyHost:      proxyHost,
			ProxyPort:      proxyPort,
			ProxyUsername:  proxyUsername,
			ProxyCreds:     proxyCreds,
			JumpSSHConfig:  jumpSSHConfig,
			JumpProxyType:  jumpProxyType,
			JumpProxyHost:  jumpProxyHost,
			JumpProxyPort:  jumpProxyPort,
			JumpProxyCreds: jumpProxyCreds,
		}
		client, err = dialWithProxy(cfg, sshConfig)
	} else {
		client, err = defaultSSHDial("tcp", fmt.Sprintf("%s:%d", s.Host, port), sshConfig)
	}
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return fmt.Errorf("new session: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("stderr pipe: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("request pty: %w", err)
	}

	if err := session.Shell(); err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("start shell: %w", err)
	}

	s.mu.Lock()
	s.sshClient = client
	s.sshSession = session
	s.stdin = stdin
	s.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)

	readPipe := func(r io.Reader) {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := r.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				data := buf[:n]
				s.appendHistory(data)
				s.Broadcast(websocket.BinaryMessage, data)
			}
		}
	}

	go readPipe(stdout)
	go readPipe(stderr)

	go func() {
		wg.Wait()
		s.handleDisconnect()
	}()

	return nil
}

func (s *TerminalSession) readLoop(conn *websocket.Conn) {
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			s.connMu.Lock()
			if s.conn == conn {
				s.conn = nil
			}
			s.connMu.Unlock()
			_ = conn.Close()
			return
		}

		if len(msg) > 0 && msg[0] == 0 {
			if len(msg) >= 5 {
				rows := uint16(msg[1])<<8 | uint16(msg[2])
				cols := uint16(msg[3])<<8 | uint16(msg[4])
				s.Resize(rows, cols)
			}
			continue
		}

		s.mu.Lock()
		stdin := s.stdin
		s.mu.Unlock()

		if stdin != nil {
			_, _ = stdin.Write(msg)
		}
	}
}

func (s *TerminalSession) Resize(rows, cols uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptyFile != nil {
		_ = pty.Setsize(s.ptyFile, &pty.Winsize{Rows: rows, Cols: cols})
	}
	if s.sshSession != nil {
		_ = s.sshSession.WindowChange(int(rows), int(cols))
	}
}

// SessionManager manages persistent terminal sessions.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*TerminalSession
}

var globalSessionManager = &SessionManager{
	sessions: make(map[string]*TerminalSession),
}

func (m *SessionManager) Get(id string) *TerminalSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

func (m *SessionManager) Add(sess *TerminalSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[sess.ID] = sess
}

func (m *SessionManager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

type TerminalHandler struct {
	db          *sql.DB
	authService *auth.Service
	dialSSH     sshDialFunc // nil falls back to defaultSSHDial
}

func NewTerminalHandler(db *sql.DB, authService *auth.Service) *TerminalHandler {
	return &TerminalHandler{db: db, authService: authService, dialSSH: defaultSSHDial}
}

func (h *TerminalHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	nodeIDStr := r.URL.Query().Get("nodeId")
	sessionID := r.URL.Query().Get("sessionId")
	if nodeIDStr == "" {
		http.Error(w, "Node ID required", http.StatusBadRequest)
		return
	}

	nodeID, err := strconv.Atoi(nodeIDStr)
	if err != nil {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	if sessionID == "" {
		sessionID = fmt.Sprintf("fallback-%d", nodeID)
	}

	// 1. Check if session already exists
	if sess := globalSessionManager.Get(sessionID); sess != nil {
		if sess.UserID != user.ID || sess.NodeID != nodeID {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}

		sess.Attach(conn)
		sess.readLoop(conn)
		return
	}

	// 2. Resolve Node details
	var host, username, encryptedCreds, authType string
	var port, ownerID int
	var proxyType, proxyHost, proxyUsername string
	var proxyPort, proxyCredentialID int
	var jumpProxyType, jumpProxyHost string
	var jumpProxyPort, jumpProxyCredentialID int
	err = h.db.QueryRow(`
		SELECT n.host, n.port, n.username, c.auth_type, c.encrypted_value, n.user_id,
		       COALESCE(n.proxy_type,''), COALESCE(n.proxy_host,''), COALESCE(n.proxy_port,0),
		       COALESCE(n.proxy_username,''), COALESCE(n.proxy_credential_id,0),
		       COALESCE(n.jump_proxy_type,''), COALESCE(n.jump_proxy_host,''),
		       COALESCE(n.jump_proxy_port,0), COALESCE(n.jump_proxy_credential_id,0)
		FROM nodes n
		JOIN credentials c ON n.credential_id = c.id
		WHERE n.id = ?
	`, nodeID).Scan(&host, &port, &username, &authType, &encryptedCreds, &ownerID,
		&proxyType, &proxyHost, &proxyPort, &proxyUsername, &proxyCredentialID,
		&jumpProxyType, &jumpProxyHost, &jumpProxyPort, &jumpProxyCredentialID)
	if err == sql.ErrNoRows {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to fetch node", http.StatusInternalServerError)
		return
	}

	if ownerID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Decode the encryption key from the session
	key, err := base64.StdEncoding.DecodeString(user.EncryptionKey)
	if err != nil {
		log.Printf("Failed to decode encryption key: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	sess := &TerminalSession{
		ID:     sessionID,
		NodeID: nodeID,
		UserID: user.ID,
		Host:   host,
		db:     h.db,
		decKey: key,
	}

	// 3. Initialize Shell
	if host == "localhost-shell" {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		cmd := exec.Command(shell)
		if homeDir, err := os.UserHomeDir(); err == nil {
			cmd.Dir = homeDir
		}

		f, err := pty.Start(cmd)
		if err != nil {
			http.Error(w, "Failed to start local shell", http.StatusInternalServerError)
			return
		}

		sess.startLocalShell(cmd, f)
	} else {
		credentials, err := crypto.Decrypt(encryptedCreds, key)
		if err != nil {
			log.Printf("Failed to decrypt credentials for node %d: %v", nodeID, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		authMethods, err := BuildAuthMethod(authType, credentials)
		if err != nil {
			log.Printf("Failed to build auth method for node %d: %v", nodeID, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		sshConfig := &ssh.ClientConfig{
			User:            username,
			Auth:            authMethods,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         8 * time.Second,
		}

		var client sshClientConn
		if proxyType != "" {
			var proxyCreds string
			if proxyCredentialID > 0 {
				var proxyAuthType, proxyEncCreds string
				h.db.QueryRow(
					"SELECT auth_type, encrypted_value FROM credentials WHERE id = ? AND user_id = ?",
					proxyCredentialID, user.ID,
				).Scan(&proxyAuthType, &proxyEncCreds)
				proxyCreds, _ = crypto.Decrypt(proxyEncCreds, key)
			}

			var jumpSSHConfig *ssh.ClientConfig
			if proxyType == "jump" && proxyCredentialID > 0 {
				var jumpAuthType, jumpEncCreds string
				h.db.QueryRow(
					"SELECT auth_type, encrypted_value FROM credentials WHERE id = ? AND user_id = ?",
					proxyCredentialID, user.ID,
				).Scan(&jumpAuthType, &jumpEncCreds)
				if jumpEncCreds != "" {
					jumpCreds, _ := crypto.Decrypt(jumpEncCreds, key)
					jumpAuthMethods, _ := BuildAuthMethod(jumpAuthType, jumpCreds)
					jumpSSHConfig = &ssh.ClientConfig{
						User:            proxyUsername,
						Auth:            jumpAuthMethods,
						HostKeyCallback: ssh.InsecureIgnoreHostKey(),
					}
				}
			}

			var jumpProxyCreds string
			if jumpProxyType != "" && jumpProxyCredentialID > 0 {
				var jpAuthType, jpEncCreds string
				h.db.QueryRow(
					"SELECT auth_type, encrypted_value FROM credentials WHERE id = ? AND user_id = ?",
					jumpProxyCredentialID, user.ID,
				).Scan(&jpAuthType, &jpEncCreds)
				if jpEncCreds != "" {
					jumpProxyCreds, _ = crypto.Decrypt(jpEncCreds, key)
				}
			}

			cfg := NodeDialConfig{
				TargetHost:     host,
				TargetPort:     port,
				ProxyType:      proxyType,
				ProxyHost:      proxyHost,
				ProxyPort:      proxyPort,
				ProxyUsername:  proxyUsername,
				ProxyCreds:     proxyCreds,
				JumpSSHConfig:  jumpSSHConfig,
				JumpProxyType:  jumpProxyType,
				JumpProxyHost:  jumpProxyHost,
				JumpProxyPort:  jumpProxyPort,
				JumpProxyCreds: jumpProxyCreds,
			}
			client, err = dialWithProxy(cfg, sshConfig)
		} else {
			dialFn := h.dialSSH
			if dialFn == nil {
				dialFn = defaultSSHDial
			}
			client, err = dialFn("tcp", fmt.Sprintf("%s:%d", host, port), sshConfig)
		}
		if err != nil {
			log.Printf("SSH connection failed for node %d: %v", nodeID, err)
			conn, err := upgrader.Upgrade(w, r, nil)
			if err == nil {
				_ = conn.WriteMessage(websocket.TextMessage, []byte("SSH connection failed: "+err.Error()+"\r\n"))
				_ = conn.Close()
			}
			return
		}

		session, err := client.NewSession()
		if err != nil {
			client.Close()
			http.Error(w, "Failed to create SSH session", http.StatusInternalServerError)
			return
		}

		stdin, err := session.StdinPipe()
		if err != nil {
			session.Close()
			client.Close()
			http.Error(w, "Failed to initialize stdin pipe", http.StatusInternalServerError)
			return
		}

		stdout, err := session.StdoutPipe()
		if err != nil {
			session.Close()
			client.Close()
			http.Error(w, "Failed to initialize stdout pipe", http.StatusInternalServerError)
			return
		}

		stderr, err := session.StderrPipe()
		if err != nil {
			session.Close()
			client.Close()
			http.Error(w, "Failed to initialize stderr pipe", http.StatusInternalServerError)
			return
		}

		modes := ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}

		if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
			session.Close()
			client.Close()
			http.Error(w, "Failed to request PTY", http.StatusInternalServerError)
			return
		}

		if err := session.Shell(); err != nil {
			session.Close()
			client.Close()
			http.Error(w, "Failed to start shell", http.StatusInternalServerError)
			return
		}

		sess.startSSHShell(client, session, stdin, stdout, stderr)
	}

	globalSessionManager.Add(sess)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		sess.Close()
		return
	}

	sess.Attach(conn)
	sess.readLoop(conn)
}

func (h *TerminalHandler) HandleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	sess := globalSessionManager.Get(sessionID)
	if sess == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if sess.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	sess.Close()
	w.WriteHeader(http.StatusNoContent)
}
