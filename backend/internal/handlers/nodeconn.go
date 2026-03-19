package handlers

import (
	"database/sql"
	"fmt"

	sshutil "github.com/webssh/manager/internal/ssh"
	"github.com/webssh/manager/pkg/crypto"
	gossh "golang.org/x/crypto/ssh"
)

// nodeSSHRouting contains everything required to open an SSH connection to a
// node, including the proxy/jump dial configuration and the target SSH client
// config built from decrypted credentials.
type nodeSSHRouting struct {
	DialConfig sshutil.NodeDialConfig
	SSHConfig  *gossh.ClientConfig
}

// loadNodeSSHRouting queries the database for a node's full SSH routing
// configuration and decrypts all required credentials using encKey.
// Returns sql.ErrNoRows if the node is not found, or an error if the caller
// does not own the node.
func loadNodeSSHRouting(db *sql.DB, nodeID, userID int, encKey []byte) (*nodeSSHRouting, error) {
	var host, username, encryptedCreds, authType string
	var port, ownerID int
	var proxyType, proxyHost, proxyUsername string
	var proxyPort, proxyCredentialID int
	var jumpProxyType, jumpProxyHost string
	var jumpProxyPort, jumpProxyCredentialID int

	err := db.QueryRow(`
		SELECT n.host, n.port, n.username, c.auth_type, c.encrypted_value, n.user_id,
		       COALESCE(n.proxy_type,''), COALESCE(n.proxy_host,''), COALESCE(n.proxy_port,0),
		       COALESCE(n.proxy_username,''), COALESCE(n.proxy_credential_id,0),
		       COALESCE(n.jump_proxy_type,''), COALESCE(n.jump_proxy_host,''), COALESCE(n.jump_proxy_port,0),
		       COALESCE(n.jump_proxy_credential_id,0)
		FROM nodes n
		JOIN credentials c ON n.credential_id = c.id
		WHERE n.id = ?`, nodeID,
	).Scan(
		&host, &port, &username, &authType, &encryptedCreds, &ownerID,
		&proxyType, &proxyHost, &proxyPort, &proxyUsername, &proxyCredentialID,
		&jumpProxyType, &jumpProxyHost, &jumpProxyPort, &jumpProxyCredentialID,
	)
	if err != nil {
		return nil, err // sql.ErrNoRows propagates as-is
	}
	if ownerID != userID {
		return nil, fmt.Errorf("node %d: access denied", nodeID)
	}

	// Decrypt target SSH credentials.
	targetCreds, err := crypto.Decrypt(encryptedCreds, encKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt target credentials: %w", err)
	}
	targetAuth, err := sshutil.BuildAuthMethod(authType, targetCreds)
	if err != nil {
		return nil, fmt.Errorf("build target auth method: %w", err)
	}

	routing := &nodeSSHRouting{
		SSHConfig: &gossh.ClientConfig{
			User:            username,
			Auth:            targetAuth,
			HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec
		},
		DialConfig: sshutil.NodeDialConfig{
			TargetHost:    host,
			TargetPort:    port,
			ProxyType:     proxyType,
			ProxyHost:     proxyHost,
			ProxyPort:     proxyPort,
			ProxyUsername: proxyUsername,
			JumpProxyType: jumpProxyType,
			JumpProxyHost: jumpProxyHost,
			JumpProxyPort: jumpProxyPort,
		},
	}

	// Decrypt the proxy/jump credential (if any).
	if proxyCredentialID > 0 {
		var proxyAuthType, proxyEncCreds string
		if e := db.QueryRow(
			"SELECT auth_type, encrypted_value FROM credentials WHERE id = ? AND user_id = ?",
			proxyCredentialID, userID,
		).Scan(&proxyAuthType, &proxyEncCreds); e == nil && proxyEncCreds != "" {
			decrypted, _ := crypto.Decrypt(proxyEncCreds, encKey)
			if proxyType == "jump" {
				// For jump: the proxy credential is the jump host's SSH credential.
				jumpAuth, _ := sshutil.BuildAuthMethod(proxyAuthType, decrypted)
				routing.DialConfig.JumpSSHConfig = &gossh.ClientConfig{
					User:            proxyUsername,
					Auth:            jumpAuth,
					HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec
				}
			} else {
				routing.DialConfig.ProxyCreds = decrypted
			}
		}
	}

	// Decrypt the jump host's upstream proxy credential (for jump-behind-proxy).
	if jumpProxyCredentialID > 0 {
		var jpAuthType, jpEncCreds string
		if e := db.QueryRow(
			"SELECT auth_type, encrypted_value FROM credentials WHERE id = ? AND user_id = ?",
			jumpProxyCredentialID, userID,
		).Scan(&jpAuthType, &jpEncCreds); e == nil && jpEncCreds != "" {
			decrypted, _ := crypto.Decrypt(jpEncCreds, encKey)
			routing.DialConfig.JumpProxyCreds = decrypted
		}
	}

	return routing, nil
}

// dialNodeSSH opens a fully-routed SSH client connection to the node described
// by routing, using dialTarget for proxy/jump resolution.
func dialNodeSSH(routing *nodeSSHRouting) (*gossh.Client, error) {
	conn, err := sshutil.DialTarget(routing.DialConfig)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	addr := fmt.Sprintf("%s:%d", routing.DialConfig.TargetHost, routing.DialConfig.TargetPort)
	c, chans, reqs, err := gossh.NewClientConn(conn, addr, routing.SSHConfig)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SSH handshake: %w", err)
	}
	return gossh.NewClient(c, chans, reqs), nil
}
