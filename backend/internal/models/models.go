package models

import "time"

type User struct {
	ID                int       `json:"id"`
	Username          string    `json:"username"`
	PasswordHash      string    `json:"-"`
	EncryptionKeySalt string    `json:"-"`
	EncryptionKey     string    `json:"-"`
	CSRFToken         string    `json:"-"`
	CreatedAt         time.Time `json:"created_at"`
}

type Node struct {
	ID                   int       `json:"id"`
	UserID               int       `json:"user_id"`
	Name                 string    `json:"name"`
	Host                 string    `json:"host"`
	Port                 int       `json:"port"`
	Username             string    `json:"username"`
	EncryptedCredentials string    `json:"-"`
	CredentialID         int       `json:"credential_id"`
	// Proxy / jump-server configuration
	ProxyType         string `json:"proxy_type"`
	ProxyHost         string `json:"proxy_host"`
	ProxyPort         int    `json:"proxy_port"`
	ProxyUsername     string `json:"proxy_username"`
	ProxyCredentialID int    `json:"proxy_credential_id"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type NodeSysInfo struct {
	NodeID      int       `json:"node_id"`
	Hostname    string    `json:"hostname"`
	OS          string    `json:"os"`
	Kernel      string    `json:"kernel"`
	Uptime      string    `json:"uptime"`
	CPUModel    string    `json:"cpu_model"`
	CPUCores    int       `json:"cpu_cores"`
	LoadAvg     string    `json:"load_avg"`
	MemTotal    int64     `json:"mem_total"`
	MemUsed     int64     `json:"mem_used"`
	DiskTotal   string    `json:"disk_total"`
	DiskUsed    string    `json:"disk_used"`
	DiskPct     string    `json:"disk_pct"`
	IPAddr      string    `json:"ip_addr"`
	CollectedAt time.Time `json:"collected_at"`
}

type Session struct {
	Token     string    `json:"token"`
	CSRFToken string    `json:"csrf_token"`
	UserID    int       `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}
