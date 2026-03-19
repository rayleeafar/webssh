package handlers

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/webssh/manager/internal/middleware"
	"github.com/webssh/manager/pkg/crypto"
)

type NodeHandler struct {
	db *sql.DB
}

func NewNodeHandler(db *sql.DB) *NodeHandler {
	return &NodeHandler{db: db}
}

type CreateNodeRequest struct {
	Name                  string `json:"name"`
	Host                  string `json:"host"`
	Port                  int    `json:"port"`
	Username              string `json:"username"`
	Password              string `json:"password,omitempty"`
	PrivateKey            string `json:"private_key,omitempty"`
	ProxyType             string `json:"proxy_type,omitempty"`
	ProxyHost             string `json:"proxy_host,omitempty"`
	ProxyPort             int    `json:"proxy_port,omitempty"`
	ProxyUsername         string `json:"proxy_username,omitempty"`
	ProxyCredentialID     int    `json:"proxy_credential_id,omitempty"`
	JumpProxyType         string `json:"jump_proxy_type,omitempty"`
	JumpProxyHost         string `json:"jump_proxy_host,omitempty"`
	JumpProxyPort         int    `json:"jump_proxy_port,omitempty"`
	JumpProxyCredentialID int    `json:"jump_proxy_credential_id,omitempty"`
}

type UpdateNodeRequest struct {
	Name                  string `json:"name"`
	Host                  string `json:"host"`
	Port                  int    `json:"port"`
	Username              string `json:"username"`
	Password              string `json:"password,omitempty"`
	PrivateKey            string `json:"private_key,omitempty"`
	ProxyType             string `json:"proxy_type,omitempty"`
	ProxyHost             string `json:"proxy_host,omitempty"`
	ProxyPort             int    `json:"proxy_port,omitempty"`
	ProxyUsername         string `json:"proxy_username,omitempty"`
	ProxyCredentialID     int    `json:"proxy_credential_id,omitempty"`
	JumpProxyType         string `json:"jump_proxy_type,omitempty"`
	JumpProxyHost         string `json:"jump_proxy_host,omitempty"`
	JumpProxyPort         int    `json:"jump_proxy_port,omitempty"`
	JumpProxyCredentialID int    `json:"jump_proxy_credential_id,omitempty"`
}

type NodeResponse struct {
	ID                    int       `json:"id"`
	Name                  string    `json:"name"`
	Host                  string    `json:"host"`
	Port                  int       `json:"port"`
	Username              string    `json:"username"`
	ProxyType             string    `json:"proxy_type"`
	ProxyHost             string    `json:"proxy_host"`
	ProxyPort             int       `json:"proxy_port"`
	ProxyUsername         string    `json:"proxy_username"`
	ProxyCredentialID     int       `json:"proxy_credential_id"`
	JumpProxyType         string    `json:"jump_proxy_type"`
	JumpProxyHost         string    `json:"jump_proxy_host"`
	JumpProxyPort         int       `json:"jump_proxy_port"`
	JumpProxyCredentialID int       `json:"jump_proxy_credential_id"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func (h *NodeHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := validateNodeRequest(req.Name, req.Host, req.Port, req.Username); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := validateProxyFields(req.ProxyType, req.ProxyHost, req.ProxyPort, req.ProxyUsername, req.ProxyCredentialID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	credentials := req.Password
	authType := "password"
	if req.PrivateKey != "" {
		credentials = req.PrivateKey
		authType = "private_key"
	}

	// Decode the encryption key from the session
	key, err := base64.StdEncoding.DecodeString(user.EncryptionKey)
	if err != nil {
		http.Error(w, "Failed to decode encryption key", http.StatusInternalServerError)
		return
	}

	encryptedCreds, err := crypto.Encrypt(credentials, key)
	if err != nil {
		http.Error(w, "Failed to encrypt credentials", http.StatusInternalServerError)
		return
	}

	// Insert into credentials table first
	credResult, err := h.db.Exec(`
		INSERT INTO credentials (user_id, auth_type, encrypted_value)
		VALUES (?, ?, ?)`,
		user.ID, authType, encryptedCreds,
	)
	if err != nil {
		http.Error(w, "Failed to store credentials", http.StatusInternalServerError)
		return
	}

	credID, err := credResult.LastInsertId()
	if err != nil {
		http.Error(w, "Failed to get credential ID", http.StatusInternalServerError)
		return
	}

	// Insert into nodes table with credential_id and proxy fields
	result, err := h.db.Exec(`
		INSERT INTO nodes (user_id, name, host, port, username, credential_id,
		                   proxy_type, proxy_host, proxy_port, proxy_username, proxy_credential_id,
		                   jump_proxy_type, jump_proxy_host, jump_proxy_port, jump_proxy_credential_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID, req.Name, req.Host, req.Port, req.Username, credID,
		req.ProxyType, req.ProxyHost, req.ProxyPort, req.ProxyUsername, req.ProxyCredentialID,
		req.JumpProxyType, req.JumpProxyHost, req.JumpProxyPort, req.JumpProxyCredentialID,
	)
	if err != nil {
		http.Error(w, "Failed to create node", http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	// Build routing synchronously so the goroutine does not need DB access.
	if routing, rerr := loadNodeSSHRouting(h.db, int(id), user.ID, key); rerr == nil {
		nodeID := int(id)
		go collectAndStoreSysInfo(h.db, nodeID, routing)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(NodeResponse{
		ID:                    int(id),
		Name:                  req.Name,
		Host:                  req.Host,
		Port:                  req.Port,
		Username:              req.Username,
		ProxyType:             req.ProxyType,
		ProxyHost:             req.ProxyHost,
		ProxyPort:             req.ProxyPort,
		ProxyUsername:         req.ProxyUsername,
		ProxyCredentialID:     req.ProxyCredentialID,
		JumpProxyType:         req.JumpProxyType,
		JumpProxyHost:         req.JumpProxyHost,
		JumpProxyPort:         req.JumpProxyPort,
		JumpProxyCredentialID: req.JumpProxyCredentialID,
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	})
}

func (h *NodeHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := h.db.Query(`
		SELECT id, name, host, port, username,
		       proxy_type, proxy_host, proxy_port, proxy_username, proxy_credential_id,
		       COALESCE(jump_proxy_type,''), COALESCE(jump_proxy_host,''),
		       COALESCE(jump_proxy_port,0), COALESCE(jump_proxy_credential_id,0),
		       created_at, updated_at
		FROM nodes
		WHERE user_id = ?
		ORDER BY created_at DESC
	`, user.ID)
	if err != nil {
		http.Error(w, "Failed to list nodes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var nodes []NodeResponse
	for rows.Next() {
		var node NodeResponse
		if err := rows.Scan(
			&node.ID, &node.Name, &node.Host, &node.Port, &node.Username,
			&node.ProxyType, &node.ProxyHost, &node.ProxyPort, &node.ProxyUsername, &node.ProxyCredentialID,
			&node.JumpProxyType, &node.JumpProxyHost, &node.JumpProxyPort, &node.JumpProxyCredentialID,
			&node.CreatedAt, &node.UpdatedAt,
		); err != nil {
			continue
		}
		nodes = append(nodes, node)
	}

	if nodes == nil {
		nodes = []NodeResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

func (h *NodeHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	nodeID := extractNodeID(r.URL.Path)
	if nodeID == 0 {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	var existingUserID, credentialID int
	err := h.db.QueryRow("SELECT user_id, credential_id FROM nodes WHERE id = ?", nodeID).Scan(&existingUserID, &credentialID)
	if err == sql.ErrNoRows {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to check node ownership", http.StatusInternalServerError)
		return
	}

	if existingUserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var req UpdateNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := validateNodeRequest(req.Name, req.Host, req.Port, req.Username); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := validateProxyFields(req.ProxyType, req.ProxyHost, req.ProxyPort, req.ProxyUsername, req.ProxyCredentialID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update credentials if provided
	if req.Password != "" || req.PrivateKey != "" {
		credentials := req.Password
		authType := "password"
		if req.PrivateKey != "" {
			credentials = req.PrivateKey
			authType = "private_key"
		}

		// Decode the encryption key from the session
		key, err := base64.StdEncoding.DecodeString(user.EncryptionKey)
		if err != nil {
			http.Error(w, "Failed to decode encryption key", http.StatusInternalServerError)
			return
		}

		encryptedCreds, err := crypto.Encrypt(credentials, key)
		if err != nil {
			http.Error(w, "Failed to encrypt credentials", http.StatusInternalServerError)
			return
		}

		// Update the credentials table
		_, err = h.db.Exec(`
			UPDATE credentials
			SET auth_type = ?, encrypted_value = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`,
			authType, encryptedCreds, credentialID,
		)
		if err != nil {
			http.Error(w, "Failed to update credentials", http.StatusInternalServerError)
			return
		}
	}

	// Update the nodes table
	_, err = h.db.Exec(`
		UPDATE nodes
		SET name = ?, host = ?, port = ?, username = ?,
		    proxy_type = ?, proxy_host = ?, proxy_port = ?, proxy_username = ?, proxy_credential_id = ?,
		    jump_proxy_type = ?, jump_proxy_host = ?, jump_proxy_port = ?, jump_proxy_credential_id = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		req.Name, req.Host, req.Port, req.Username,
		req.ProxyType, req.ProxyHost, req.ProxyPort, req.ProxyUsername, req.ProxyCredentialID,
		req.JumpProxyType, req.JumpProxyHost, req.JumpProxyPort, req.JumpProxyCredentialID,
		nodeID,
	)
	if err != nil {
		http.Error(w, "Failed to update node", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *NodeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	nodeID := extractNodeID(r.URL.Path)
	if nodeID == 0 {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	result, err := h.db.Exec("DELETE FROM nodes WHERE id = ? AND user_id = ?", nodeID, user.ID)
	if err != nil {
		http.Error(w, "Failed to delete node", http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func validateProxyFields(proxyType, proxyHost string, proxyPort int, proxyUsername string, proxyCredentialID int) error {
	switch proxyType {
	case "", "socks5", "http", "https", "jump":
		// valid
	default:
		return fmt.Errorf("proxy_type must be one of: '', 'socks5', 'http', 'https', 'jump'")
	}
	if proxyType == "jump" {
		if proxyHost == "" {
			return fmt.Errorf("jump host address is required")
		}
		if proxyPort <= 0 {
			return fmt.Errorf("jump host port is required")
		}
		if proxyUsername == "" {
			return fmt.Errorf("jump host username is required")
		}
		if proxyCredentialID <= 0 {
			return fmt.Errorf("jump host credential is required")
		}
	}
	if (proxyType == "socks5" || proxyType == "http" || proxyType == "https") && proxyHost == "" {
		return fmt.Errorf("proxy_host is required when proxy_type is '%s'", proxyType)
	}
	return nil
}

func validateNodeRequest(name, host string, port int, username string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if host == "" {
		return fmt.Errorf("host is required")
	}
	if !isValidHost(host) {
		return fmt.Errorf("invalid host format")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if username == "" {
		return fmt.Errorf("username is required")
	}
	return nil
}

func isValidHost(host string) bool {
	ipv4Pattern := `^(\d{1,3}\.){3}\d{1,3}$`
	domainPattern := `^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`
	localhostPattern := `^localhost$`

	if matched, _ := regexp.MatchString(ipv4Pattern, host); matched {
		return true
	}
	if matched, _ := regexp.MatchString(domainPattern, host); matched {
		return true
	}
	if matched, _ := regexp.MatchString(localhostPattern, host); matched {
		return true
	}
	return false
}

func extractNodeID(path string) int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 {
		return 0
	}
	id, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0
	}
	return id
}
