# WebSSH Manager

> SSH + SFTP in your browser — anywhere, secure, lightweight

WebSSH Manager is a **self-hosted, zero-dependency**, lightweight web application that turns any modern browser into a full-featured SSH terminal and SFTP file manager for your remote servers. 

Designed for developers, DevOps engineers, system administrators, and homelab enthusiasts who want a clean, private, browser-only SSH portal without installing anything on their machines.

## ✨ Key Features

- **Unlimited Remote Nodes**: Manage unlimited remote servers with ease.
- **Instant Web Terminal**: Real-time SSH terminal in your browser powered by xterm.js and WebSockets.
- **Built-in SFTP Manager**: Browse, upload, download, create directories, and delete files directly from the UI.
- **Self-Hosted & Secure**: 
  - 100% self-hosted, meaning your credentials never leave your server.
  - SSH credentials encrypted at rest with AES-256-GCM.
  - User passwords hashed with bcrypt.
- **Zero-Dependency**: 
  - Backend built in Go (Golang) for performance, small memory footprint, and concurrency.
  - Runs on a single embedded SQLite database (zero-configuration).
  - Can be compiled into a single unified binary (frontend embedded natively via Go).
- **Responsive UI**: Modern Next.js 16 frontend styled with TailwindCSS 3.

## 🚀 Quick Start (Docker)

The easiest way to get started is using Docker Compose.

1. Clone the repository:
   ```bash
   git clone https://github.com/your-username/webssh-manager.git
   cd webssh-manager
   ```

2. Set your master secret for encryption:
   ```bash
   export MASTER_SECRET="your-super-secret-key-change-me"
   ```

3. Start the application:
   ```bash
   docker-compose up -d
   ```

4. Open your browser and navigate to `http://localhost:3000`.

## 🛠 Building from Source (Single Binary)

You can compile the entire application (Next.js frontend + Go backend) into a single, standalone executable binary without any external dependencies.

**Requirements:**
- Go 1.25+
- Node.js 18+ & npm

```bash
# Build the Next.js frontend, copy assets, and compile the Go backend
make build

# Run the unified standalone binary
export MASTER_SECRET="your-super-secret-key-change-me"
./webssh
```

## 🗺 Roadmap & Upcoming Features

We are actively working on improving WebSSH Manager. Planned features include:

- [x] **Proxy & Jump Server Support**: SSH connection support via SOCKS5, HTTP/HTTPS proxies, or through jump servers.
- [x] **System Dashboard**: Fetch basic system info (CPU, Memory, Disk) when connecting to a node and display on a dashboard tab.
- [x] **Batch Execution**: Select multiple nodes to run a command simultaneously and retrieve aggregated results.
- [x] **Multi-Tab Sessions**: Open multiple terminals to the same node in separate tabs.
- [x] **Two-Factor Authentication (2FA)**: Enhance login security with TOTP.
- [x] **Full HTTPS/TLS integration**: Streamlined configuration for secure connections.

## 🏗 Architecture

- **Backend**: Go (Golang) + SQLite 
- **Frontend**: Next.js 16 (App Router) + React Server Components + TypeScript + Tailwind CSS
- **Communication**: REST API for management endpoints, WebSockets for realtime terminal streams

### Project Layout
```text
.
├── backend/          # Go backend service (auth, ssh, sftp, sqlite)
├── frontend/         # Next.js frontend application (React, xterm.js)
├── Makefile          # Build scripts for unified binary
└── docker-compose.yml# Easy deployment configuration
```

## 🔐 Security Details

- Encryption keys derived from user passwords using PBKDF2 (100,000 iterations).
- SSH credentials encrypted with AES-256-GCM.
- Parameterized SQL queries to prevent SQL injection.
- Node ownership checks strictly enforced on backend endpoints.
- WebSocket connections require valid authentication sessions.

## ⚙️ Configuration (Environment Variables)

When running the backend directly or via Docker, you can configure the app using the following variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `MASTER_SECRET` | **Required.** Secret used for credential encryption | |
| `DB_PATH` | Path to SQLite database file | `/data/webssh.db` |
| `LISTEN_ADDR` | Server listen address (backend) | `0.0.0.0:8080` |
| `ENABLE_TLS` | Enable HTTPS | `false` |
| `TLS_CERT` | TLS certificate path | |
| `TLS_KEY` | TLS private key path | |
| `NEXT_PUBLIC_WS_URL`| WebSocket address for frontend | `ws://localhost:8080` |

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the project
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## 📄 License

Distributed under the MIT License.
