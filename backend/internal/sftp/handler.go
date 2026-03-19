package sftp

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/pkg/sftp"
	sshutil "github.com/webssh/manager/internal/ssh"
	"github.com/webssh/manager/internal/middleware"
	"github.com/webssh/manager/pkg/crypto"
	"golang.org/x/crypto/ssh"
)

// sftpReadFile is the interface for an SFTP file opened for reading.
// Using an interface allows tests to inject in-memory fakes.
type sftpReadFile interface {
	io.Reader
	io.Closer
	Stat() (os.FileInfo, error)
}

// sftpWriteFile is the interface for an SFTP file opened for writing.
type sftpWriteFile interface {
	io.Writer
	io.Closer
}

// sftpClientIF wraps the *sftp.Client methods used by handlers.
type sftpClientIF interface {
	ReadDir(p string) ([]os.FileInfo, error)
	Open(path string) (sftpReadFile, error)
	Create(path string) (sftpWriteFile, error)
	Stat(p string) (os.FileInfo, error)
	Remove(path string) error
	RemoveDirectory(path string) error
	Mkdir(path string) error
}

// realSFTPClient wraps *sftp.Client to satisfy sftpClientIF.
type realSFTPClient struct{ c *sftp.Client }

func (r *realSFTPClient) ReadDir(p string) ([]os.FileInfo, error) { return r.c.ReadDir(p) }
func (r *realSFTPClient) Open(path string) (sftpReadFile, error)  { return r.c.Open(path) }
func (r *realSFTPClient) Create(path string) (sftpWriteFile, error) {
	return r.c.Create(path)
}
func (r *realSFTPClient) Stat(p string) (os.FileInfo, error)    { return r.c.Stat(p) }
func (r *realSFTPClient) Remove(path string) error              { return r.c.Remove(path) }
func (r *realSFTPClient) RemoveDirectory(path string) error     { return r.c.RemoveDirectory(path) }
func (r *realSFTPClient) Mkdir(path string) error               { return r.c.Mkdir(path) }

// sftpClientFactory is the injectable function for obtaining an SFTP client.
// Tests replace this with a factory that returns an in-memory fake.
type sftpClientFactory func(nodeID, userID int, encryptionKey string) (sftpClientIF, func(), error)

type SFTPHandler struct {
	db        *sql.DB
	newClient sftpClientFactory // injectable; defaults to getSFTPClient
}

func NewSFTPHandler(db *sql.DB) *SFTPHandler {
	h := &SFTPHandler{db: db}
	h.newClient = h.getSFTPClient
	return h
}

type FileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
	IsDir   bool   `json:"is_dir"`
}

func (h *SFTPHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	nodeIDStr := r.URL.Query().Get("nodeId")
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}

	nodeID, err := strconv.Atoi(nodeIDStr)
	if err != nil {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	sftpClient, cleanup, err := h.newClient(nodeID, user.ID, user.EncryptionKey)
	if err != nil {
		log.Printf("SFTP connection failed for node %d: %v", nodeID, err)
		http.Error(w, "Failed to connect to server", http.StatusInternalServerError)
		return
	}
	defer cleanup()

	files, err := sftpClient.ReadDir(path)
	if err != nil {
		log.Printf("Failed to list directory %q on node %d: %v", path, nodeID, err)
		http.Error(w, "Failed to list directory", http.StatusInternalServerError)
		return
	}

	var fileInfos []FileInfo
	for _, file := range files {
		fileInfos = append(fileInfos, FileInfo{
			Name:    file.Name(),
			Size:    file.Size(),
			Mode:    file.Mode().String(),
			ModTime: file.ModTime().Format("2006-01-02 15:04:05"),
			IsDir:   file.IsDir(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fileInfos)
}

func (h *SFTPHandler) Download(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	nodeIDStr := r.URL.Query().Get("nodeId")
	path := r.URL.Query().Get("path")

	nodeID, err := strconv.Atoi(nodeIDStr)
	if err != nil {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	sftpClient, cleanup, err := h.newClient(nodeID, user.ID, user.EncryptionKey)
	if err != nil {
		log.Printf("SFTP connection failed for node %d: %v", nodeID, err)
		http.Error(w, "Failed to connect to server", http.StatusInternalServerError)
		return
	}
	defer cleanup()

	file, err := sftpClient.Open(path)
	if err != nil {
		log.Printf("Failed to open file %q on node %d: %v", path, nodeID, err)
		http.Error(w, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "Failed to stat file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", stat.Name()))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))

	io.Copy(w, file)
}

func (h *SFTPHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	nodeIDStr := r.FormValue("nodeId")
	path := r.FormValue("path")

	nodeID, err := strconv.Atoi(nodeIDStr)
	if err != nil {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	sftpClient, cleanup, err := h.newClient(nodeID, user.ID, user.EncryptionKey)
	if err != nil {
		log.Printf("SFTP connection failed for node %d: %v", nodeID, err)
		http.Error(w, "Failed to connect to server", http.StatusInternalServerError)
		return
	}
	defer cleanup()

	// path already includes the filename from the frontend
	remoteFile, err := sftpClient.Create(path)
	if err != nil {
		log.Printf("Failed to create remote file %q on node %d: %v", path, nodeID, err)
		http.Error(w, "Failed to create remote file", http.StatusInternalServerError)
		return
	}
	defer remoteFile.Close()

	if _, err := io.Copy(remoteFile, file); err != nil {
		http.Error(w, "Failed to upload file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *SFTPHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	nodeIDStr := r.URL.Query().Get("nodeId")
	path := r.URL.Query().Get("path")

	nodeID, err := strconv.Atoi(nodeIDStr)
	if err != nil {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	sftpClient, cleanup, err := h.newClient(nodeID, user.ID, user.EncryptionKey)
	if err != nil {
		log.Printf("SFTP connection failed for node %d: %v", nodeID, err)
		http.Error(w, "Failed to connect to server", http.StatusInternalServerError)
		return
	}
	defer cleanup()

	// Check if path is a directory
	info, err := sftpClient.Stat(path)
	if err != nil {
		log.Printf("Failed to stat %q on node %d: %v", path, nodeID, err)
		http.Error(w, "Failed to access path", http.StatusInternalServerError)
		return
	}

	if info.IsDir() {
		// For directories, use RemoveDirectory
		if err := sftpClient.RemoveDirectory(path); err != nil {
			log.Printf("Failed to delete directory %q on node %d: %v", path, nodeID, err)
			http.Error(w, "Failed to delete directory", http.StatusInternalServerError)
			return
		}
	} else {
		// For files, use Remove
		if err := sftpClient.Remove(path); err != nil {
			log.Printf("Failed to delete file %q on node %d: %v", path, nodeID, err)
			http.Error(w, "Failed to delete file", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *SFTPHandler) Mkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		NodeID int    `json:"node_id"`
		Path   string `json:"path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	sftpClient, cleanup, err := h.newClient(req.NodeID, user.ID, user.EncryptionKey)
	if err != nil {
		log.Printf("SFTP connection failed for node %d: %v", req.NodeID, err)
		http.Error(w, "Failed to connect to server", http.StatusInternalServerError)
		return
	}
	defer cleanup()

	if err := sftpClient.Mkdir(req.Path); err != nil {
		log.Printf("Failed to create directory %q on node %d: %v", req.Path, req.NodeID, err)
		http.Error(w, "Failed to create directory", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *SFTPHandler) getSFTPClient(nodeID, userID int, encryptionKey string) (sftpClientIF, func(), error) {
	var host, nodeUsername, encryptedCreds, authType string
	var port, ownerID int
	var proxyType, proxyHost, proxyUsername string
	var proxyPort, proxyCredentialID int
	var jumpProxyType, jumpProxyHost string
	var jumpProxyPort, jumpProxyCredentialID int

	err := h.db.QueryRow(`
		SELECT n.host, n.port, n.username, c.auth_type, c.encrypted_value, n.user_id,
		       COALESCE(n.proxy_type,''), COALESCE(n.proxy_host,''), COALESCE(n.proxy_port,0),
		       COALESCE(n.proxy_username,''), COALESCE(n.proxy_credential_id,0),
		       COALESCE(n.jump_proxy_type,''), COALESCE(n.jump_proxy_host,''),
		       COALESCE(n.jump_proxy_port,0), COALESCE(n.jump_proxy_credential_id,0)
		FROM nodes n
		JOIN credentials c ON n.credential_id = c.id
		WHERE n.id = ?
	`, nodeID).Scan(&host, &port, &nodeUsername, &authType, &encryptedCreds, &ownerID,
		&proxyType, &proxyHost, &proxyPort, &proxyUsername, &proxyCredentialID,
		&jumpProxyType, &jumpProxyHost, &jumpProxyPort, &jumpProxyCredentialID)
	if err == sql.ErrNoRows {
		return nil, nil, fmt.Errorf("node not found")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch node")
	}

	if ownerID != userID {
		return nil, nil, fmt.Errorf("forbidden")
	}

	// Decode the encryption key from the session
	key, err := base64.StdEncoding.DecodeString(encryptionKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode encryption key")
	}

	credentials, err := crypto.Decrypt(encryptedCreds, key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decrypt credentials")
	}

	authMethods, err := sshutil.BuildAuthMethod(authType, credentials)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build auth method: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            nodeUsername,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	var sshClient *ssh.Client
	if proxyType != "" {
		// Load proxy credentials if a proxy credential ID is configured.
		var proxyCreds string
		if proxyCredentialID > 0 {
			var proxyAuthType, proxyEncCreds string
			h.db.QueryRow(
				"SELECT auth_type, encrypted_value FROM credentials WHERE id = ? AND user_id = ?",
				proxyCredentialID, userID,
			).Scan(&proxyAuthType, &proxyEncCreds)
			proxyCreds, _ = crypto.Decrypt(proxyEncCreds, key)
		}

		// Build JumpSSHConfig when proxy type is "jump".
		var jumpSSHConfig *ssh.ClientConfig
		if proxyType == "jump" && proxyCredentialID > 0 {
			var jumpAuthType, jumpEncCreds string
			h.db.QueryRow(
				"SELECT auth_type, encrypted_value FROM credentials WHERE id = ? AND user_id = ?",
				proxyCredentialID, userID,
			).Scan(&jumpAuthType, &jumpEncCreds)
			if jumpEncCreds != "" {
				jumpCreds, _ := crypto.Decrypt(jumpEncCreds, key)
				jumpAuthMethods, _ := sshutil.BuildAuthMethod(jumpAuthType, jumpCreds)
				jumpSSHConfig = &ssh.ClientConfig{
					User:            proxyUsername,
					Auth:            jumpAuthMethods,
					HostKeyCallback: ssh.InsecureIgnoreHostKey(),
				}
			}
		}

		// Fetch jump proxy credentials if the jump host has its own upstream proxy.
		var jumpProxyCreds string
		if jumpProxyType != "" && jumpProxyCredentialID > 0 {
			var jpAuthType, jpEncCreds string
			h.db.QueryRow(
				"SELECT auth_type, encrypted_value FROM credentials WHERE id = ? AND user_id = ?",
				jumpProxyCredentialID, userID,
			).Scan(&jpAuthType, &jpEncCreds)
			if jpEncCreds != "" {
				jumpProxyCreds, _ = crypto.Decrypt(jpEncCreds, key)
			}
		}

		cfg := sshutil.NodeDialConfig{
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
		conn, err := sshutil.DialTarget(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("SSH proxy connection failed: %v", err)
		}
		addr := fmt.Sprintf("%s:%d", host, port)
		c, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
		if err != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("SSH handshake failed: %v", err)
		}
		sshClient = ssh.NewClient(c, chans, reqs)
	} else {
		sshClient, err = ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), sshConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("SSH connection failed: %v", err)
		}
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, nil, fmt.Errorf("SFTP client creation failed: %v", err)
	}

	cleanup := func() {
		sftpClient.Close()
		sshClient.Close()
	}

	return &realSFTPClient{c: sftpClient}, cleanup, nil
}
