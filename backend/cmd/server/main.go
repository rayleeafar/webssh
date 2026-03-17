package main

import (
	"log"
	"net/http"

	"github.com/webssh/manager/internal/auth"
	"github.com/webssh/manager/internal/config"
	"github.com/webssh/manager/internal/database"
	"github.com/webssh/manager/internal/handlers"
	"github.com/webssh/manager/internal/middleware"
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
	authHandler := handlers.NewAuthHandler(authService)
	nodeHandler := handlers.NewNodeHandler(db)
	terminalHandler := ssh.NewTerminalHandler(db, authService)

	authMiddleware := middleware.AuthMiddleware(authService)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("/api/auth/register", authHandler.Register)
	mux.HandleFunc("/api/auth/login", authHandler.Login)
	mux.HandleFunc("/api/auth/logout", authHandler.Logout)
	mux.Handle("/api/auth/me", authMiddleware(http.HandlerFunc(authHandler.Me)))

	mux.Handle("/api/nodes", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			nodeHandler.List(w, r)
		case http.MethodPost:
			nodeHandler.Create(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))

	mux.Handle("/api/nodes/", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			nodeHandler.Update(w, r)
		case http.MethodDelete:
			nodeHandler.Delete(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))

	mux.Handle("/ws/terminal", authMiddleware(http.HandlerFunc(terminalHandler.HandleWebSocket)))

	addr := cfg.ListenAddr
	log.Printf("Starting server on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
