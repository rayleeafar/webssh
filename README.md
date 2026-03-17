# WebSSH Manager

A self-hosted web application for browser-based SSH terminal access and SFTP file management.

## Architecture

- **Backend**: Go (Golang) with SQLite database
- **Frontend**: Next.js 15 with TypeScript and Tailwind CSS
- **Communication**: REST API + WebSocket for terminal

## Project Structure

```
.
├── backend/          # Go backend
│   ├── cmd/server/   # Main application entry point
│   ├── internal/     # Internal packages
│   │   ├── auth/     # Authentication service
│   │   ├── config/   # Configuration management
│   │   ├── database/ # Database initialization and migrations
│   │   ├── models/   # Data models
│   │   ├── ssh/      # SSH connection handling
│   │   ├── sftp/     # SFTP operations
│   │   └── websocket/# WebSocket handlers
│   └── pkg/crypto/   # Encryption utilities
│
└── frontend/         # Next.js frontend
    └── app/          # App Router pages

```

## Features

- User authentication with session management
- SSH node management (CRUD operations)
- Real-time web terminal via WebSocket
- SFTP file browser with upload/download
- Encrypted credential storage (AES-256-GCM)
- HTTPS/TLS support

## Development

### Backend

```bash
cd backend
go run cmd/server/main.go
```

### Frontend

```bash
cd frontend
npm run dev
```

## Security

- User passwords hashed with bcrypt
- SSH credentials encrypted with AES-256-GCM
- Encryption keys derived from user passwords using PBKDF2
- Session-based authentication with secure tokens
- Parameterized SQL queries to prevent injection

## Configuration

Environment variables:
- `DB_PATH`: SQLite database file path (default: `./data/webssh.db`)
- `LISTEN_ADDR`: Server listen address (default: `:8080`)
- `ENABLE_TLS`: Enable HTTPS (default: `false`)
- `TLS_CERT`: TLS certificate path
- `TLS_KEY`: TLS key path
