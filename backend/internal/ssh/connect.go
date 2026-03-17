package ssh

import (
	"fmt"

	"golang.org/x/crypto/ssh"
)

// BuildAuthMethod creates the appropriate SSH auth method based on auth type.
func BuildAuthMethod(authType, credentials string) ([]ssh.AuthMethod, error) {
	switch authType {
	case "private_key":
		signer, err := ssh.ParsePrivateKey([]byte(credentials))
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	default:
		return []ssh.AuthMethod{ssh.Password(credentials)}, nil
	}
}
