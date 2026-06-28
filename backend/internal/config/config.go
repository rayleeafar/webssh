package config

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	DatabasePath string
	ListenAddr   string
	TLSCertPath  string
	TLSKeyPath   string
	EnableTLS    bool
	HTTPAddr     string
	MasterSecret string
}

func Load() *Config {
	envEnableTLS := getEnv("ENABLE_TLS", "false") == "true"
	
	dbPathFlag := flag.String("db-path", getEnv("DB_PATH", "./data/webssh.db"), "Path to SQLite database file")
	
	defaultAddr := ":8080"
	if envEnableTLS {
		defaultAddr = ":8443"
	}
	listenAddrFlag := flag.String("listen-addr", getEnv("LISTEN_ADDR", defaultAddr), "Address to listen on")
	tlsCertFlag := flag.String("tls-cert", getEnv("TLS_CERT", ""), "Path to TLS certificate file")
	tlsKeyFlag := flag.String("tls-key", getEnv("TLS_KEY", ""), "Path to TLS private key file")
	enableTLSFlag := flag.Bool("enable-tls", envEnableTLS, "Enable HTTPS/TLS")
	httpAddrFlag := flag.String("http-addr", getEnv("HTTP_ADDR", ":8080"), "Address for HTTP-to-HTTPS redirect server")
	masterSecretFlag := flag.String("master-secret", getEnv("MASTER_SECRET", ""), "Master secret for credential encryption")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of WebSSH Manager Server:\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\nConfiguration can also be set via Environment Variables:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  DB_PATH       Path to SQLite database file\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  LISTEN_ADDR   Address to listen on\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  TLS_CERT      Path to TLS certificate file\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  TLS_KEY       Path to TLS private key file\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  ENABLE_TLS    Enable HTTPS/TLS (true/false)\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  HTTP_ADDR     Address for HTTP-to-HTTPS redirect server\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  MASTER_SECRET Master secret for credential encryption\n")
	}

	if !flag.Parsed() {
		flag.Parse()
	}

	finalListenAddr := *listenAddrFlag
	if *enableTLSFlag && finalListenAddr == ":8080" {
		finalListenAddr = ":8443"
	}

	return &Config{
		DatabasePath: *dbPathFlag,
		ListenAddr:   finalListenAddr,
		TLSCertPath:  *tlsCertFlag,
		TLSKeyPath:   *tlsKeyFlag,
		EnableTLS:    *enableTLSFlag,
		HTTPAddr:     *httpAddrFlag,
		MasterSecret: *masterSecretFlag,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
