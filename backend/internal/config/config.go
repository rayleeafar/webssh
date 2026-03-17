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
}

func Load() *Config {
	return &Config{
		DatabasePath: getEnv("DB_PATH", "./data/webssh.db"),
		ListenAddr:   getEnv("LISTEN_ADDR", ":8080"),
		TLSCertPath:  getEnv("TLS_CERT", ""),
		TLSKeyPath:   getEnv("TLS_KEY", ""),
		EnableTLS:    getEnv("ENABLE_TLS", "false") == "true",
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
