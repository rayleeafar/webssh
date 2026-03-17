package sftp

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/pkg/sftp"
	"github.com/webssh/manager/internal/middleware"
	"github.com/webssh/manager/pkg/crypto"
	"golang.org/x/crypto/ssh"
)

type SFTPHandler struct {
	db *sql.DB
}

func NewSFTPHandler(db *sql.DB) *SFTPHandler {
	return &SFTPHandler{db: db}
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

	sftpClient, cleanup, err := h.getSFTPClient(nodeID, user.ID, user.EncryptionKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cleanup()

	files, err := sftpClient.ReadDir(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list directory: %v", err), http.StatusInternalServerError)
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

	sftpClient, cleanup, err := h.getSFTPClient(nodeID, user.ID, user.EncryptionKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cleanup()

	file, err := sftpClient.Open(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to open file: %v", err), http.StatusInternalServerError)
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

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	sftpClient, cleanup, err := h.getSFTPClient(nodeID, user.ID, user.EncryptionKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cleanup()

	remotePath := path + "/" + header.Filename
	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create remote file: %v", err), http.StatusInternalServerError)
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

	sftpClient, cleanup, err := h.getSFTPClient(nodeID, user.ID, user.EncryptionKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cleanup()

	if err := sftpClient.Remove(path); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete: %v", err), http.StatusInternalServerError)
		return
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

	sftpClient, cleanup, err := h.getSFTPClient(req.NodeID, user.ID, user.EncryptionKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cleanup()

	if err := sftpClient.Mkdir(req.Path); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create directory: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *SFTPHandler) getSFTPClient(nodeID, userID int, encryptionKey string) (*sftp.Client, func(), error) {
	var host, nodeUsername, encryptedCreds string
	var port, ownerID int

	err := h.db.QueryRow(`
		SELECT host, port, username, encrypted_credentials, user_id
		FROM nodes
		WHERE id = ?
	`, nodeID).Scan(&host, &port, &nodeUsername, &encryptedCreds, &ownerID)
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

	sshConfig := &ssh.ClientConfig{
		User: nodeUsername,
		Auth: []ssh.AuthMethod{
			ssh.Password(credentials),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	sshClient, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), sshConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("SSH connection failed: %v", err)
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

	return sftpClient, cleanup, nil
}
