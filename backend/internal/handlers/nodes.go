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
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	PrivateKey  string `json:"private_key,omitempty"`
}

type UpdateNodeRequest struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	PrivateKey  string `json:"private_key,omitempty"`
}

type NodeResponse struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

	credentials := req.Password
	if req.PrivateKey != "" {
		credentials = req.PrivateKey
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

	result, err := h.db.Exec(`
		INSERT INTO nodes (user_id, name, host, port, username, encrypted_credentials)
		VALUES (?, ?, ?, ?, ?, ?)
	`, user.ID, req.Name, req.Host, req.Port, req.Username, encryptedCreds)
	if err != nil {
		http.Error(w, "Failed to create node", http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(NodeResponse{
		ID:        int(id),
		Name:      req.Name,
		Host:      req.Host,
		Port:      req.Port,
		Username:  req.Username,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
		SELECT id, name, host, port, username, created_at, updated_at
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
		if err := rows.Scan(&node.ID, &node.Name, &node.Host, &node.Port, &node.Username, &node.CreatedAt, &node.UpdatedAt); err != nil {
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

	var existingUserID int
	err := h.db.QueryRow("SELECT user_id FROM nodes WHERE id = ?", nodeID).Scan(&existingUserID)
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

	query := "UPDATE nodes SET name = ?, host = ?, port = ?, username = ?, updated_at = CURRENT_TIMESTAMP"
	args := []interface{}{req.Name, req.Host, req.Port, req.Username}

	if req.Password != "" || req.PrivateKey != "" {
		credentials := req.Password
		if req.PrivateKey != "" {
			credentials = req.PrivateKey
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

		query += ", encrypted_credentials = ?"
		args = append(args, encryptedCreds)
	}

	query += " WHERE id = ?"
	args = append(args, nodeID)

	_, err = h.db.Exec(query, args...)
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
