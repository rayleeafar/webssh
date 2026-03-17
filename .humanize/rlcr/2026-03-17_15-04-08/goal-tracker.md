# Goal Tracker

<!--
This file tracks the ultimate goal, acceptance criteria, and plan evolution.
It prevents goal drift by maintaining a persistent anchor across all rounds.

RULES:
- IMMUTABLE SECTION: Do not modify after initialization
- MUTABLE SECTION: Update each round, but document all changes
- Every task must be in one of: Active, Completed, or Deferred
- Deferred items require explicit justification
-->

## IMMUTABLE SECTION
<!-- Do not modify after initialization -->

### Ultimate Goal

Build a self-hosted web application (WebSSH Manager) that provides browser-based SSH terminal access and SFTP file management for remote servers. The system consists of a Go backend providing SSH/SFTP connectivity and API services, and a Next.js 15 frontend delivering the user interface. Users authenticate to WebSSH Manager, manage their remote server nodes, open web-based SSH terminals, and perform file operations through an SFTP interface—all accessible from any modern browser without plugins or client-side installations.

### Acceptance Criteria
<!-- Each criterion must be independently verifiable -->
<!-- Claude must extract or define these in Round 0 -->

- **AC-1**: User Authentication and Session Management - Users can register, log in with credentials, receive session tokens, and access protected endpoints. Invalid credentials and expired tokens are properly rejected.

- **AC-2**: SSH Node Management - Users can create, list, update, and delete SSH nodes with host, port, and username. Credentials are stored encrypted in SQLite. Users cannot access other users' nodes.

- **AC-3**: Real-time Web Terminal via SSH - Users can initiate SSH connections to configured nodes via WebSocket, see terminal output with ANSI escape codes, send input in real-time, handle window resize events, and disconnect cleanly. Invalid credentials and unreachable hosts fail gracefully.

- **AC-4**: SFTP File Manager - Users can browse remote directories, upload/download files, delete files/directories, create directories, and view file permissions. Operations fail gracefully with appropriate errors for invalid credentials or paths.

- **AC-5**: Secure Communication - Application serves content over HTTPS when configured, redirects HTTP to HTTPS, includes HSTS headers, and upgrades WebSocket to WSS. HTTP-only mode is available for development.

- **AC-6**: Database and Credential Storage - SQLite database is created automatically with proper schema. User passwords are hashed (bcrypt/argon2). SSH credentials are encrypted with AES-256-GCM using user-derived keys. No plaintext passwords in database.

- **AC-7**: Multi-Node Support - Users can manage multiple SSH nodes and open concurrent terminal sessions to different nodes. Each session maintains independent state. No arbitrary caps on node count.

- **AC-8**: Error Handling and Resilience - SSH connection failures, network interruptions, and SFTP operation failures display user-friendly error messages. Application does not crash on timeouts or malformed responses. No sensitive information in error messages.

---

## MUTABLE SECTION
<!-- Update each round with justification for changes -->

### Plan Version: 1 (Updated: Round 0)

#### Plan Evolution Log
<!-- Document any changes to the plan with justification -->
| Round | Change | Reason | Impact on AC |
|-------|--------|--------|--------------|
| 0 | Initial plan | - | - |
| 1 | Rejected Claude's completion/deferment request; kept incomplete work active and added blocking implementation issues discovered during review | Core plan tasks remain unfinished or partially implemented despite claimed completion | AC-1, AC-4, AC-5, AC-6, AC-8 remain incomplete |

#### Active Tasks
<!-- Map each task to its target Acceptance Criterion -->
| Task | Target AC | Status | Notes |
|------|-----------|--------|-------|
| Initialize Go project with module structure | AC-1, AC-6 | completed | Backend foundation created |
| Set up SQLite database schema (users, nodes, credentials) | AC-1, AC-2, AC-6 | completed | Schema with auto-migration |
| Implement user registration endpoint | AC-1 | completed | With bcrypt password hashing |
| Implement user login endpoint | AC-1 | completed | With session token generation |
| Implement credential encryption/decryption utilities | AC-2, AC-6 | completed | AES-256-GCM with PBKDF2 |
| Initialize Next.js 15 project with App Router | AC-1, AC-2, AC-3, AC-4 | completed | Frontend foundation created |
| Implement Go backend API for node CRUD | AC-2 | completed | Handlers and routes exist, but end-to-end UX is still blocked by missing CSRF integration |
| Build Next.js frontend for node management UI | AC-2 | pending | List/add/delete UI exists, but edit is missing and state-changing actions do not send CSRF tokens |
| Integrate frontend with backend API | AC-2 | pending | Handle auth tokens |
| Add validation for node configuration | AC-2 | completed | Host format and port range validation exist in backend handlers |
| Implement WebSocket handler for terminal | AC-3 | completed | WebSocket endpoint and SSH session setup exist |
| Establish SSH client connections | AC-3 | completed | Password-based SSH dial path exists; private-key auth still missing |
| Proxy bidirectional data (WebSocket ↔ SSH) | AC-3 | completed | stdin/stdout/stderr proxying exists |
| Integrate xterm.js in frontend | AC-3 | completed | Terminal page uses xterm.js and fit addon |
| Handle terminal resize events | AC-3 | completed | Resize messages are sent and mapped to `WindowChange` |
| Implement terminal error handling | AC-3, AC-8 | pending | Errors are raw/unsanitized and secure deployment still uses hardcoded `ws://` |
| Implement SFTP session management | AC-4 | completed | SFTP session creation exists |
| Create REST API for SFTP operations | AC-4 | completed | Backend list/download/upload/delete/mkdir handlers exist |
| Build Next.js SFTP file browser UI | AC-4 | pending | Browse/download/delete UI exists, but upload and mkdir flows are missing |
| Implement file upload functionality | AC-4 | pending | Multipart form handling |
| Implement file download functionality | AC-4 | completed | Backend and frontend download paths exist |
| Add file deletion and mkdir operations | AC-4 | pending | Complete SFTP operations |
| Implement HTTPS/TLS support | AC-5 | pending | Certificate configuration |
| Add HTTP to HTTPS redirect | AC-5 | pending | Configurable for development |
| Implement CSRF protection | AC-5, AC-8 | pending | State-changing endpoints |
| Add comprehensive error handling | AC-8 | pending | Throughout application |
| Create Docker configuration | AC-5 | pending | Containerized deployment |
| Write tests for critical functions | AC-1, AC-3, AC-6 | pending | Auth, encryption, SSH |

### Completed and Verified
<!-- Only move tasks here after Codex verification -->
| AC | Task | Completed Round | Verified Round | Evidence |
|----|------|-----------------|----------------|----------|
| AC-1, AC-6 | Initialize Go project with module structure | 0 | 1 | `backend/cmd/server/main.go` builds with `go build ./cmd/server` during review |
| AC-1, AC-6 | Set up SQLite database schema | 0 | 1 | `backend/internal/database/database.go` initializes `users`, `nodes`, and `sessions` tables automatically |
| AC-1 | Implement user registration endpoint | 1 | 1 | `backend/internal/handlers/auth.go` exposes `POST /api/auth/register` and backend tests pass |
| AC-1 | Implement user login endpoint | 1 | 1 | `backend/internal/handlers/auth.go` exposes `POST /api/auth/login` and backend tests pass |
| AC-2, AC-6 | Implement credential encryption/decryption utilities | 0 | 1 | `backend/pkg/crypto/crypto.go` and `go test ./...` verified the crypto package |
| AC-1, AC-2, AC-3, AC-4 | Initialize Next.js project with App Router | 0 | 1 | App Router pages exist under `frontend/app`, though feature completeness is still pending |

### Explicitly Deferred
<!-- Items here require strong justification -->
| Task | Original AC | Deferred Since | Justification | When to Reconsider |
|------|-------------|----------------|---------------|-------------------|

### Open Issues
<!-- Issues discovered during implementation -->
| Issue | Discovered Round | Blocking AC | Resolution Path |
|-------|-----------------|-------------|-----------------|
| Frontend does not fetch/send CSRF tokens, so node create/delete and SFTP delete/upload/mkdir requests fail with 403 | 1 | AC-2, AC-4, AC-5, AC-8 | Add session-bound CSRF issuance/validation and wire the frontend to include the token on every state-changing request |
| Credential encryption is derived from username plus salt rather than the user's password, and schema lacks a dedicated `credentials` table from the plan | 1 | AC-2, AC-6 | Rework key derivation around authenticated password material/session secret and split encrypted credentials into their own table |
| Logout only clears the browser cookie and does not revoke the server-side session token | 1 | AC-1 | Delete the session from SQLite during logout and verify rejected reuse of the old token |
| TLS support is incomplete: redirect is hardcoded to `:8080`, not configurable, and secure frontend transport still hardcodes `http://` and `ws://` URLs | 1 | AC-5 | Add explicit HTTPS/redirect config and derive API/WebSocket origins from runtime environment so HTTPS deployments use HTTPS/WSS |
| SFTP and node management UIs are incomplete relative to the plan | 1 | AC-2, AC-4 | Add node edit UI plus SFTP upload and mkdir flows, and verify them end to end |
| Existing tests mostly cover helper functions and method guards, not the acceptance-criteria success/failure cases | 1 | AC-1, AC-2, AC-3, AC-4, AC-5, AC-6, AC-8 | Add integration-style handler tests for auth, node CRUD, CSRF, logout revocation, and SFTP/terminal failure paths |
