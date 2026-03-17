package config

import (
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
	enableTLS := getEnv("ENABLE_TLS", "false") == "true"
	defaultAddr := ":8080"
	if enableTLS {
		defaultAddr = ":8443"
	}
	return &Config{
		DatabasePath: getEnv("DB_PATH", "./data/webssh.db"),
		ListenAddr:   getEnv("LISTEN_ADDR", defaultAddr),
		TLSCertPath:  getEnv("TLS_CERT", ""),
		TLSKeyPath:   getEnv("TLS_KEY", ""),
		EnableTLS:    enableTLS,
		HTTPAddr:     getEnv("HTTP_ADDR", ":8080"),
		MasterSecret: getEnv("MASTER_SECRET", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
