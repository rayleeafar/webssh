# WebSSH Manager

A self-hosted web application for browser-based SSH terminal access and SFTP file management.

## Architecture

- **Backend**: Go (Golang) with SQLite database
- **Frontend**: Next.js 16 with TypeScript and Tailwind CSS 3
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
│   │   ├── handlers/ # HTTP request handlers
│   │   ├── middleware/ # Authentication middleware
│   │   ├── models/   # Data models
│   │   ├── ssh/      # SSH/terminal WebSocket handling
│   │   └── sftp/     # SFTP operations
│   └── pkg/crypto/   # Encryption utilities
│
└── frontend/         # Next.js frontend
    └── app/          # App Router pages
        ├── login/    # Login page
        ├── register/ # Registration page
        ├── nodes/    # Node management
        ├── terminal/ # Web terminal with xterm.js
        └── sftp/     # SFTP file browser
```

## Features

- ✅ User authentication with session management
- ✅ SSH node management (CRUD operations)
- ✅ Real-time web terminal via WebSocket with xterm.js
- ✅ SFTP file browser with download/delete operations
- ✅ Encrypted credential storage (AES-256-GCM)
- ✅ Host format validation (IPv4, domain, localhost)
- ✅ Port range validation (1-65535)
- ✅ Node ownership enforcement
- ⏳ HTTPS/TLS support (configurable, not yet implemented)
- ⏳ File upload functionality (backend ready, frontend pending)
- ⏳ Directory creation (backend ready, frontend pending)

## Development

### Backend

```bash
cd backend
go run cmd/server/main.go
```

Server starts on `localhost:8080` by default.

### Frontend

```bash
cd frontend
npm install
npm run dev
```

Frontend runs on `localhost:3000` by default.

## API Endpoints

### Authentication
- `POST /api/auth/register` - Register new user
- `POST /api/auth/login` - Login and get session token
- `POST /api/auth/logout` - Logout and clear session
- `GET /api/auth/me` - Get current user (protected)

### Nodes
- `GET /api/nodes` - List user's nodes (protected)
- `POST /api/nodes` - Create new node (protected)
- `PUT /api/nodes/:id` - Update node (protected)
- `DELETE /api/nodes/:id` - Delete node (protected)

### Terminal
- `WS /ws/terminal?nodeId=X` - WebSocket terminal connection (protected)

### SFTP
- `GET /api/sftp/list?nodeId=X&path=Y` - List directory (protected)
- `GET /api/sftp/download?nodeId=X&path=Y` - Download file (protected)
- `POST /api/sftp/upload` - Upload file (protected)
- `DELETE /api/sftp/delete?nodeId=X&path=Y` - Delete file/directory (protected)
- `POST /api/sftp/mkdir` - Create directory (protected)

## Security

- User passwords hashed with bcrypt
- SSH credentials encrypted with AES-256-GCM
- Encryption keys derived from user passwords using PBKDF2 (100,000 iterations)
- Session-based authentication with secure tokens
- Parameterized SQL queries to prevent injection
- Node ownership checks on all operations
- WebSocket connections require authentication

## Configuration

Environment variables:
- `DB_PATH`: SQLite database file path (default: `./data/webssh.db`)
- `LISTEN_ADDR`: Server listen address (default: `:8080`)
- `ENABLE_TLS`: Enable HTTPS (default: `false`)
- `TLS_CERT`: TLS certificate path
- `TLS_KEY`: TLS key path

## Known Limitations

- HTTPS/TLS support is configured but not fully implemented
- No CSRF protection yet
- No rate limiting on authentication endpoints
- No comprehensive error logging framework
- No automated tests
- File upload UI not implemented in frontend
- Directory creation UI not implemented in frontend
- SSH host key verification uses InsecureIgnoreHostKey (should be configurable)
