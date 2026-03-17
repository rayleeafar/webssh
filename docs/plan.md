# WebSSH Manager Implementation Plan

## Goal Description

Build a self-hosted web application that provides browser-based SSH terminal access and SFTP file management for remote servers. The system consists of a Go backend providing SSH/SFTP connectivity and API services, and a Next.js 15 frontend delivering the user interface. Users authenticate to WebSSH Manager, manage their remote server nodes, open web-based SSH terminals, and perform file operations through an SFTP interface—all accessible from any modern browser without plugins or client-side installations.

## Acceptance Criteria

Following TDD philosophy, each criterion includes positive and negative tests for deterministic verification.

- AC-1: User Authentication and Session Management
  - Positive Tests (expected to PASS):
    - User can register with valid username and password
    - User can log in with correct credentials and receive a session token
    - Authenticated user can access protected API endpoints
    - Session persists across page refreshes until logout or expiration
  - Negative Tests (expected to FAIL):
    - Registration with duplicate username is rejected
    - Login with incorrect password is rejected
    - Unauthenticated requests to protected endpoints return 401
    - Expired or invalid session tokens are rejected

- AC-2: SSH Node Management
  - Positive Tests (expected to PASS):
    - User can create a new SSH node with host, port, username
    - User can list all their configured SSH nodes
    - User can update node details (name, host, port, username)
    - User can delete a node they own
    - Node credentials (password or private key) are stored encrypted in SQLite
  - Negative Tests (expected to FAIL):
    - User cannot view or modify nodes belonging to other users
    - Creating a node with invalid host format is rejected
    - Creating a node with port outside valid range (1-65535) is rejected
    - Deleting a non-existent node returns appropriate error

- AC-3: Real-time Web Terminal via SSH
  - Positive Tests (expected to PASS):
    - User can initiate SSH connection to a configured node via WebSocket
    - Terminal displays SSH server's welcome message and prompt
    - User input is transmitted to SSH server and output is displayed in real-time
    - Terminal supports ANSI escape codes (colors, cursor positioning)
    - Terminal handles resize events (window size changes propagate to SSH session)
    - User can disconnect terminal session cleanly
  - Negative Tests (expected to FAIL):
    - Connection attempt with invalid credentials fails gracefully with error message
    - Connection to unreachable host times out with appropriate error
    - WebSocket connection without valid session token is rejected
    - Malformed terminal input is handled without crashing the session

- AC-4: SFTP File Manager
  - Positive Tests (expected to PASS):
    - User can browse remote directory structure via SFTP
    - User can upload files to remote server
    - User can download files from remote server
    - User can delete remote files and directories
    - User can create new directories on remote server
    - File permissions and metadata are displayed correctly
  - Negative Tests (expected to FAIL):
    - SFTP operations without valid SSH credentials fail with error
    - Attempting to access paths outside user's permissions is rejected by server
    - Upload of excessively large files (if size limit implemented) is rejected
    - Operations on non-existent paths return appropriate errors

- AC-5: Secure Communication
  - Positive Tests (expected to PASS):
    - Application serves content over HTTPS when TLS is configured
    - HTTP requests are redirected to HTTPS when redirect is enabled
    - HSTS header is present in HTTPS responses when configured
    - WebSocket connections upgrade to WSS (secure WebSocket)
  - Negative Tests (expected to FAIL):
    - Application can run in HTTP-only mode for development/reverse proxy scenarios (this is allowed per user clarification)
    - Invalid TLS certificates are rejected by clients (browser behavior, not application test)

- AC-6: Database and Credential Storage
  - Positive Tests (expected to PASS):
    - SQLite database is created automatically on first run
    - User passwords are hashed (bcrypt or similar) before storage
    - SSH credentials are encrypted using key derived from user password
    - Database schema includes tables for users, nodes, and credentials
    - Encrypted credentials can be successfully decrypted when user authenticates
  - Negative Tests (expected to FAIL):
    - Raw passwords are never stored in plaintext in database
    - Credentials cannot be decrypted without correct user password
    - Database queries with SQL injection attempts are safely handled

- AC-7: Multi-Node Support
  - Positive Tests (expected to PASS):
    - User can configure and manage multiple SSH nodes
    - User can open multiple concurrent terminal sessions to different nodes
    - Each terminal session maintains independent state
    - System handles resource-dependent number of nodes without artificial limits
  - Negative Tests (expected to FAIL):
    - System does not impose arbitrary caps on node count (best effort based on resources)
    - Concurrent sessions do not interfere with each other's I/O

- AC-8: Error Handling and Resilience
  - Positive Tests (expected to PASS):
    - SSH connection failures display user-friendly error messages
    - Network interruptions during terminal sessions are detected and reported
    - SFTP operation failures provide actionable error information
    - Application logs errors appropriately for debugging
  - Negative Tests (expected to FAIL):
    - Application does not crash on SSH connection timeout
    - Application does not crash on malformed SSH server responses
    - Application does not expose sensitive information in error messages

## Path Boundaries

Path boundaries define the acceptable range of implementation quality and choices.

### Upper Bound (Maximum Acceptable Scope)

The implementation includes all three core modules (SSH Node Management, Real-time Web Terminal, SFTP File Manager) with full user authentication, encrypted credential storage, WebSocket-based terminal with xterm.js, a complete SFTP browser with upload/download/edit/delete/mkdir capabilities, HTTPS with TLS and HTTP redirect, HSTS headers, comprehensive error handling, and test coverage for both Go backend and Next.js frontend. The Go backend exposes a clean REST API plus WebSocket endpoints. The Next.js frontend provides a polished, responsive UI with App Router and React Server Components.

### Lower Bound (Minimum Acceptable Scope)

The implementation includes a Go backend with REST API for node CRUD and WebSocket proxy for SSH sessions, SQLite storage with encrypted credentials, basic user authentication with session tokens, a Next.js 15 frontend with a functional terminal view (xterm.js) and basic SFTP file listing with upload/download. HTTPS support is present but HTTP-only mode is available for development. Error messages are functional but may not cover every edge case.

### Allowed Choices

- Can use:
  - Go standard library and `golang.org/x/crypto/ssh` for SSH
  - `pkg/sftp` for SFTP operations
  - `mattn/go-sqlite3` or `modernc.org/sqlite` for SQLite driver
  - `gorilla/websocket` or `nhooyr.io/websocket` for WebSocket handling
  - `xterm.js` for browser terminal emulation
  - `bcrypt` or `argon2` for password hashing
  - AES-256-GCM for credential encryption
  - Any Go HTTP router (chi, gin, standard mux)
  - Next.js 15 App Router with TypeScript
  - Tailwind CSS or any CSS framework for styling
  - Docker for containerized deployment
- Cannot use:
  - External database systems (PostgreSQL, MySQL, Redis)
  - External authentication services (OAuth providers) — local auth only for v1.0
  - Paid or proprietary dependencies

## Feasibility Hints and Suggestions

> **Note**: This section is for reference and understanding only. These are conceptual suggestions, not prescriptive requirements.

### Conceptual Approach

**Architecture Overview:**

```
┌─────────────────────────────────────────────────────────────┐
│                         Browser                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │           Next.js 15 Frontend (Port 3000)            │  │
│  │  - React Server Components + App Router              │  │
│  │  - xterm.js for terminal rendering                   │  │
│  │  - SFTP file browser UI                              │  │
│  │  - WebSocket client for terminal I/O                 │  │
│  └──────────────────────────────────────────────────────┘  │
│                          │                                   │
│                    HTTPS/WSS                                 │
└──────────────────────────┼──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│              Go Backend (Port 8080)                          │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  HTTP/WebSocket Server                                 │ │
│  │  - REST API: /api/auth, /api/nodes, /api/sftp         │ │
│  │  - WebSocket: /ws/terminal                             │ │
│  │  - TLS termination (optional, can be reverse proxy)    │ │
│  └────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  Business Logic Layer                                  │ │
│  │  - User authentication & session management            │ │
│  │  - Node CRUD operations                                │ │
│  │  - SSH connection pooling                              │ │
│  │  - SFTP operations proxy                               │ │
│  └────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  Data Access Layer                                     │ │
│  │  - SQLite database (users, nodes, credentials)         │ │
│  │  - Credential encryption/decryption                    │ │
│  └────────────────────────────────────────────────────────┘ │
└──────────────────────────┬──────────────────────────────────┘
                           │
                      SSH/SFTP
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                  Remote SSH Servers                          │
│  - Server 1 (192.168.1.10:22)                               │
│  - Server 2 (example.com:22)                                │
│  - Server N (...)                                            │
└─────────────────────────────────────────────────────────────┘
```

**Key Flows:**

1. **User Authentication:**
   - User submits credentials to `/api/auth/login`
   - Backend validates against SQLite, generates session token (JWT or UUID)
   - Token stored in HTTP-only cookie or returned to client
   - Subsequent requests include token for authorization

2. **Terminal Session:**
   - User selects node, frontend opens WebSocket to `/ws/terminal?nodeId=X`
   - Backend authenticates WebSocket connection via token
   - Backend retrieves encrypted credentials from SQLite, decrypts using user's key
   - Backend establishes SSH connection to remote server
   - Backend proxies bidirectional data: Browser ↔ WebSocket ↔ SSH ↔ Remote Server
   - xterm.js renders terminal output in browser

3. **SFTP Operations:**
   - User navigates to SFTP view, frontend calls `/api/sftp/list?nodeId=X&path=/home`
   - Backend establishes SFTP session using stored credentials
   - Backend returns directory listing as JSON
   - For uploads: frontend sends file via multipart POST to `/api/sftp/upload`
   - For downloads: frontend requests `/api/sftp/download?nodeId=X&path=/file.txt`

4. **Credential Encryption:**
   - User password → PBKDF2/Argon2 → Encryption Key (32 bytes)
   - SSH credentials → AES-256-GCM(plaintext, key) → Ciphertext stored in SQLite
   - On retrieval: Ciphertext → AES-256-GCM-Decrypt(ciphertext, key) → Plaintext credentials

**Pseudocode Example (Go Backend - Terminal WebSocket Handler):**

```go
func HandleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
    // 1. Authenticate user from session token
    user := authenticateRequest(r)
    if user == nil {
        http.Error(w, "Unauthorized", 401)
        return
    }

    // 2. Get node ID from query params
    nodeID := r.URL.Query().Get("nodeId")
    node := getNodeByID(nodeID, user.ID)
    if node == nil {
        http.Error(w, "Node not found", 404)
        return
    }

    // 3. Decrypt SSH credentials
    credentials := decryptCredentials(node.EncryptedCreds, user.EncryptionKey)

    // 4. Upgrade HTTP to WebSocket
    conn, _ := upgrader.Upgrade(w, r, nil)
    defer conn.Close()

    // 5. Establish SSH connection
    sshClient, _ := ssh.Dial("tcp", node.Host+":"+node.Port, &ssh.ClientConfig{
        User: node.Username,
        Auth: []ssh.AuthMethod{ssh.Password(credentials.Password)},
    })
    defer sshClient.Close()

    // 6. Create SSH session
    session, _ := sshClient.NewSession()
    defer session.Close()

    // 7. Set up PTY
    session.RequestPty("xterm-256color", 80, 24, ssh.TerminalModes{})

    // 8. Pipe SSH I/O to WebSocket
    stdin, _ := session.StdinPipe()
    stdout, _ := session.StdoutPipe()

    session.Start("/bin/bash")

    // Goroutine: WebSocket → SSH
    go func() {
        for {
            _, msg, _ := conn.ReadMessage()
            stdin.Write(msg)
        }
    }()

    // Main: SSH → WebSocket
    buf := make([]byte, 1024)
    for {
        n, _ := stdout.Read(buf)
        conn.WriteMessage(websocket.TextMessage, buf[:n])
    }
}
```

### Relevant References

Since this is a new project, there are no existing code paths to reference. However, the following external libraries and documentation will be useful:

- `golang.org/x/crypto/ssh` - Go SSH client library
- `github.com/pkg/sftp` - SFTP client for Go
- `github.com/gorilla/websocket` - WebSocket library for Go
- `github.com/mattn/go-sqlite3` - SQLite driver for Go
- `xterm.js` - Terminal emulator for the browser (https://xtermjs.org/)
- Next.js 15 App Router documentation (https://nextjs.org/docs)
- WebSocket API (MDN) for frontend WebSocket client

## Dependencies and Sequence

### Milestones

1. **Foundation and Infrastructure**: Establish project structure, database schema, and core authentication
   - Phase A: Initialize Go project with module structure, set up SQLite database with schema for users, nodes, and credentials
   - Phase B: Implement user registration and login endpoints with password hashing and session token generation
   - Phase C: Implement credential encryption/decryption utilities using AES-256-GCM with user-derived keys
   - Phase D: Initialize Next.js 15 project with App Router, TypeScript, and basic layout structure

2. **SSH Node Management**: Enable users to configure and manage remote server connections
   - Phase A: Implement Go backend API endpoints for node CRUD operations (create, read, update, delete)
   - Phase B: Build Next.js frontend pages for node management UI (list, add, edit, delete nodes)
   - Phase C: Integrate frontend with backend API, handle authentication tokens in requests
   - Phase D: Add validation for node configuration (host format, port range, required fields)

3. **Real-time Web Terminal**: Establish SSH connectivity and browser-based terminal interface
   - Phase A: Implement WebSocket handler in Go backend for terminal connections
   - Phase B: Establish SSH client connections using golang.org/x/crypto/ssh with stored credentials
   - Phase C: Proxy bidirectional data between WebSocket and SSH session (stdin/stdout)
   - Phase D: Integrate xterm.js in Next.js frontend, connect to backend WebSocket endpoint
   - Phase E: Handle terminal resize events and PTY size negotiation
   - Phase F: Implement connection error handling and graceful disconnection

4. **SFTP File Manager**: Provide file browsing and transfer capabilities
   - Phase A: Implement SFTP session management in Go backend using pkg/sftp
   - Phase B: Create REST API endpoints for SFTP operations (list, upload, download, delete, mkdir)
   - Phase C: Build Next.js frontend file browser UI with directory navigation
   - Phase D: Implement file upload functionality with multipart form handling
   - Phase E: Implement file download functionality with proper content-type headers
   - Phase F: Add file deletion and directory creation operations

5. **Security and Production Readiness**: Harden security, add TLS support, and prepare for deployment
   - Phase A: Implement HTTPS/TLS support in Go backend with certificate configuration
   - Phase B: Add HTTP to HTTPS redirect and HSTS headers (configurable for development)
   - Phase C: Implement CSRF protection for state-changing API endpoints
   - Phase D: Add comprehensive error handling and logging throughout the application
   - Phase E: Create Docker configuration for containerized deployment
   - Phase F: Write tests for critical backend functions (authentication, encryption, SSH connection)

**Dependencies:**

- Milestone 1 must complete before all others (foundation required)
- Milestone 2 depends on Milestone 1 (node management requires authentication)
- Milestone 3 depends on Milestones 1 and 2 (terminal requires auth and node configuration)
- Milestone 4 depends on Milestones 1 and 2 (SFTP requires auth and node configuration)
- Milestone 5 can begin after Milestones 3 and 4 are functional (security hardening applied to working features)

## Implementation Notes

### Code Style Requirements
- Implementation code and comments must NOT contain plan-specific terminology such as "AC-", "Milestone", "Step", "Phase", or similar workflow markers
- These terms are for plan documentation only, not for the resulting codebase
- Use descriptive, domain-appropriate naming in code instead

### Architecture Clarifications (from User Input)

Based on user clarifications during plan generation:

1. **Architecture Model**: The system will run as separate Next.js and Go processes (not a single embedded binary). The Go backend provides APIs and WebSocket endpoints, while Next.js serves the frontend independently.

2. **Authentication**: Username/password authentication with local database storage. User accounts are stored in SQLite with hashed passwords.

3. **Credential Storage**: SSH credentials (passwords and private keys) are encrypted in SQLite using a key derived from the user's password. Credentials are decrypted when the user authenticates.

4. **Terminal Protocol**: WebSocket (bidirectional) for real-time terminal communication between browser and Go backend.

5. **HTTPS/TLS**: While the draft specifies "100% HTTPS + TLS only," HTTP fallback is allowed for development environments or when deployed behind a TLS-terminating reverse proxy. Production deployments should enforce HTTPS.

6. **Node Limits**: The "unlimited remote nodes" claim is a best-effort goal. The system should not impose artificial limits, but practical limits based on system resources (memory, file descriptors, etc.) are acceptable.

### Security Considerations

- **Password Hashing**: Use bcrypt or argon2 for user password hashing before storage
- **Credential Encryption**: Use AES-256-GCM for encrypting SSH credentials at rest
- **Key Derivation**: Use PBKDF2 or Argon2 to derive encryption keys from user passwords
- **Session Management**: Use secure, HTTP-only cookies or bearer tokens with appropriate expiration
- **SQL Injection**: Use parameterized queries for all database operations
- **XSS Prevention**: Sanitize terminal output if rendering HTML; xterm.js handles this by default
- **CSRF Protection**: Implement CSRF tokens for state-changing operations
- **Rate Limiting**: Consider rate limiting on authentication endpoints to prevent brute force attacks

### Configuration Management

The application should support configuration via:
- Configuration file (e.g., `config.yaml` or `config.json`)
- Environment variables (for Docker/container deployments)
- Command-line flags (for development)

Key configuration parameters:
- Database file path
- HTTP/HTTPS listen address and port
- TLS certificate and key paths
- Session timeout duration
- Maximum concurrent SSH connections (resource limit)

### Deployment Options

1. **Standalone**: Run Go backend and Next.js frontend as separate processes on the same host
2. **Docker Compose**: Multi-container setup with Go backend and Next.js frontend in separate containers
3. **Reverse Proxy**: Deploy behind nginx or Caddy for TLS termination and load balancing

### Testing Strategy

- **Unit Tests**: Test critical Go functions (encryption, authentication, database operations)
- **Integration Tests**: Test API endpoints with mock database
- **Manual Testing**: Test SSH/SFTP connectivity with real remote servers
- **Frontend Tests**: Test React components with Jest and React Testing Library (optional for v1.0)

--- Original Design Draft Start ---

# Product Draft: WebSSH Manager

**Version:** 1.0 (Draft)  
**Date:** March 2026  
**Product Name:** WebSSH Manager  
**Tagline:** "SSH + SFTP in your browser — anywhere, secure, lightweight"

## 1. Overview

WebSSH Manager is a **self-hosted, zero-dependency**, lightweight web application that turns any modern browser into a full-featured SSH terminal and SFTP file manager for your remote servers.

Users can:
- Manage unlimited remote nodes (servers)
- Open instant SSH terminals
- Browse, upload, download, edit files via SFTP
- Access everything from any device with a browser

**Core promise:**  
- Only HTTPS + TLS communication  
- No browser plugins  
- No heavy dependencies  
- Runs on a single binary (or tiny Docker image)

**Target users:** Developers, DevOps engineers, system administrators, homelab enthusiasts who want a clean, private, browser-only SSH portal without installing anything on their machines.

## 2. Key Requirements (User-Provided + Perfected)

- **Backend:** Go (Golang) – chosen for performance, small binary, excellent SSH libraries
- **Frontend:** Next.js 15 (App Router + React Server Components + TypeScript)
- **Database:** SQLite (embedded, zero-config, single file)
- **Communication:** 100% HTTPS + TLS only (HTTP redirect + HSTS enforced)
- **Core Modules:**
  - SSH Node Management
  - Real-time Web Terminal
  - SFTP File Manager

--- Original Design Draft End ---
