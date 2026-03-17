package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/webssh/manager/internal/auth"
	"github.com/webssh/manager/internal/config"
	"github.com/webssh/manager/internal/database"
	"github.com/webssh/manager/internal/handlers"
	"github.com/webssh/manager/internal/logging"
	"github.com/webssh/manager/internal/middleware"
	"github.com/webssh/manager/internal/sftp"
	"github.com/webssh/manager/internal/ssh"
)

func main() {
	cfg := config.Load()

	db, err := database.Initialize(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	authService := auth.NewService(db)
	authHandler := handlers.NewAuthHandler(authService, cfg.EnableTLS)
	nodeHandler := handlers.NewNodeHandler(db)
	terminalHandler := ssh.NewTerminalHandler(db, authService)
	sftpHandler := sftp.NewSFTPHandler(db)

	authMiddleware := middleware.AuthMiddleware(authService)
	csrfMiddleware := middleware.CSRFMiddleware

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("/api/auth/register", authHandler.Register)
	mux.HandleFunc("/api/auth/login", authHandler.Login)
	mux.Handle("/api/auth/logout", authMiddleware(csrfMiddleware(http.HandlerFunc(authHandler.Logout))))
	mux.Handle("/api/auth/me", authMiddleware(http.HandlerFunc(authHandler.Me)))

	mux.Handle("/api/nodes", authMiddleware(csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			nodeHandler.List(w, r)
		case http.MethodPost:
			nodeHandler.Create(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))))

	mux.Handle("/api/nodes/", authMiddleware(csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			nodeHandler.Update(w, r)
		case http.MethodDelete:
			nodeHandler.Delete(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))))

	mux.Handle("/ws/terminal", authMiddleware(http.HandlerFunc(terminalHandler.HandleWebSocket)))

	mux.Handle("/api/sftp/list", authMiddleware(http.HandlerFunc(sftpHandler.List)))
	mux.Handle("/api/sftp/download", authMiddleware(http.HandlerFunc(sftpHandler.Download)))
	mux.Handle("/api/sftp/upload", authMiddleware(csrfMiddleware(http.HandlerFunc(sftpHandler.Upload))))
	mux.Handle("/api/sftp/delete", authMiddleware(csrfMiddleware(http.HandlerFunc(sftpHandler.Delete))))
	mux.Handle("/api/sftp/mkdir", authMiddleware(csrfMiddleware(http.HandlerFunc(sftpHandler.Mkdir))))

	addr := cfg.ListenAddr
	handler := logging.LoggingMiddleware(mux)

	if cfg.EnableTLS {
		log.Printf("Starting HTTPS server on %s", addr)

		httpsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			handler.ServeHTTP(w, r)
		})

		go func() {
			httpAddr := cfg.HTTPAddr
			log.Printf("Starting HTTP redirect server on %s", httpAddr)

			// Extract just the port from addr (e.g. "0.0.0.0:8443" → ":8443")
			httpsPort := addr
			if colonIdx := strings.LastIndex(addr, ":"); colonIdx != -1 {
				httpsPort = addr[colonIdx:]
			}

			http.ListenAndServe(httpAddr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				host := r.Host
				// Strip port from incoming Host header
				if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
					host = host[:colonIdx]
				}
				target := "https://" + host + httpsPort + r.RequestURI
				http.Redirect(w, r, target, http.StatusMovedPermanently)
			}))
		}()

		if err := http.ListenAndServeTLS(addr, cfg.TLSCertPath, cfg.TLSKeyPath, httpsHandler); err != nil {
			log.Fatalf("HTTPS server failed: %v", err)
		}
	} else {
		log.Printf("Starting HTTP server on %s", addr)
		if err := http.ListenAndServe(addr, handler); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}
}
