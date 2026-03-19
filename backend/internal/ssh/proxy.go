package ssh

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"

	"golang.org/x/net/proxy"

	gossh "golang.org/x/crypto/ssh"
)

// NodeDialConfig holds the proxy and target parameters needed to establish
// a network connection to an SSH server, optionally through a proxy or jump host.
type NodeDialConfig struct {
	// Target SSH server
	TargetHost string
	TargetPort int
	// Proxy settings
	ProxyType     string // "", "socks5", "http", "https", "jump"
	ProxyHost     string
	ProxyPort     int
	ProxyUsername string
	ProxyCreds    string // decrypted proxy credentials (password or private key)
	// Jump server SSH auth (only used when ProxyType == "jump")
	JumpSSHConfig *gossh.ClientConfig
}

// DialTarget is the exported entry point for the sftp package and other consumers.
// It delegates to dialTarget.
func DialTarget(cfg NodeDialConfig) (net.Conn, error) {
	return dialTarget(cfg)
}

// dialTarget returns a net.Conn to the target SSH address using the configured
// proxy or jump-server routing.
func dialTarget(cfg NodeDialConfig) (net.Conn, error) {
	targetAddr := fmt.Sprintf("%s:%d", cfg.TargetHost, cfg.TargetPort)

	switch cfg.ProxyType {
	case "socks5":
		var auth *proxy.Auth
		if cfg.ProxyCreds != "" {
			auth = &proxy.Auth{User: cfg.ProxyUsername, Password: cfg.ProxyCreds}
		}
		proxyAddr := fmt.Sprintf("%s:%d", cfg.ProxyHost, cfg.ProxyPort)
		dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("socks5 dialer: %w", err)
		}
		return dialer.Dial("tcp", targetAddr)

	case "http", "https":
		return httpConnectDial(cfg.ProxyHost, cfg.ProxyPort, cfg.ProxyUsername, cfg.ProxyCreds, cfg.TargetHost, cfg.TargetPort)

	case "jump":
		if cfg.JumpSSHConfig == nil {
			return nil, fmt.Errorf("jump host SSH config is required")
		}
		jumpAddr := fmt.Sprintf("%s:%d", cfg.ProxyHost, cfg.ProxyPort)
		jumpClient, err := gossh.Dial("tcp", jumpAddr, cfg.JumpSSHConfig)
		if err != nil {
			return nil, fmt.Errorf("jump host connection failed: %w", err)
		}
		// Dial the target through the jump host's TCP channel
		conn, err := jumpClient.Dial("tcp", targetAddr)
		if err != nil {
			jumpClient.Close()
			return nil, fmt.Errorf("jump host dial to target failed: %w", err)
		}
		// Wrap conn so closing it also closes the jump client
		return &jumpConn{Conn: conn, jumpClient: jumpClient}, nil

	default:
		return net.Dial("tcp", targetAddr)
	}
}

// httpConnectDial sends an HTTP CONNECT request through an HTTP/HTTPS proxy.
func httpConnectDial(proxyHost string, proxyPort int, proxyUser, proxyCreds, targetHost string, targetPort int) (net.Conn, error) {
	proxyAddr := fmt.Sprintf("%s:%d", proxyHost, proxyPort)
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("proxy connect failed: %w", err)
	}

	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetAddr, targetAddr)
	if proxyUser != "" {
		creds := base64.StdEncoding.EncodeToString([]byte(proxyUser + ":" + proxyCreds))
		req += "Proxy-Authorization: Basic " + creds + "\r\n"
	}
	req += "\r\n"

	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("proxy CONNECT write failed: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("proxy CONNECT response error: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("proxy CONNECT rejected with status %d %s", resp.StatusCode, resp.Status)
	}
	return conn, nil
}

// jumpConn wraps a net.Conn through a jump host so that closing the conn
// also tears down the jump host SSH client.
type jumpConn struct {
	net.Conn
	jumpClient *gossh.Client
}

func (j *jumpConn) Close() error {
	err := j.Conn.Close()
	j.jumpClient.Close()
	return err
}

// dialWithProxy establishes an SSH client connection to the target through
// the proxy described in cfg, returning a sshClientConn.
func dialWithProxy(cfg NodeDialConfig, sshConfig *gossh.ClientConfig) (sshClientConn, error) {
	conn, err := dialTarget(cfg)
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("%s:%d", cfg.TargetHost, cfg.TargetPort)
	c, chans, reqs, err := gossh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &defaultSSHClientConn{c: gossh.NewClient(c, chans, reqs)}, nil
}

