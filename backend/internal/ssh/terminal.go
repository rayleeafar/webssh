package ssh

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

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

type TerminalHandler struct {
	db          *sql.DB
	authService *auth.Service
}

func NewTerminalHandler(db *sql.DB, authService *auth.Service) *TerminalHandler {
	return &TerminalHandler{db: db, authService: authService}
}

func (h *TerminalHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	nodeIDStr := r.URL.Query().Get("nodeId")
	if nodeIDStr == "" {
		http.Error(w, "Node ID required", http.StatusBadRequest)
		return
	}

	nodeID, err := strconv.Atoi(nodeIDStr)
	if err != nil {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	var host, username, encryptedCreds, authType string
	var port, ownerID int
	err = h.db.QueryRow(`
		SELECT n.host, n.port, n.username, c.auth_type, c.encrypted_value, n.user_id
		FROM nodes n
		JOIN credentials c ON n.credential_id = c.id
		WHERE n.id = ?
	`, nodeID).Scan(&host, &port, &username, &authType, &encryptedCreds, &ownerID)
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

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	sshConfig := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	sshClient, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), sshConfig)
	if err != nil {
		log.Printf("SSH connection failed for node %d: %v", nodeID, err)
		conn.WriteMessage(websocket.TextMessage, []byte("SSH connection failed. Check node credentials and connectivity.\r\n"))
		return
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to create SSH session.\r\n"))
		return
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to initialize terminal.\r\n"))
		return
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to initialize terminal.\r\n"))
		return
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to initialize terminal.\r\n"))
		return
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to request terminal.\r\n"))
		return
	}

	if err := session.Shell(); err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to start shell.\r\n"))
		return
	}

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}

			if len(msg) > 0 && msg[0] == 0 {
				if len(msg) >= 5 {
					rows := int(msg[1])<<8 | int(msg[2])
					cols := int(msg[3])<<8 | int(msg[4])
					session.WindowChange(rows, cols)
				}
				continue
			}

			if _, err := stdin.Write(msg); err != nil {
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("stdout read error: %v", err)
				}
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("stderr read error: %v", err)
				}
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	wg.Wait()
}
