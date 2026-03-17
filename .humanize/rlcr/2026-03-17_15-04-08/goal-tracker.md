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

### Plan Version: 1 (Updated: Round 4)

#### Plan Evolution Log
<!-- Document any changes to the plan with justification -->
| Round | Change | Reason | Impact on AC |
|-------|--------|--------|--------------|
| 0 | Initial plan | - | - |
| 1 | Rejected Claude's completion/deferment request; kept incomplete work active and added blocking implementation issues discovered during review | Core plan tasks remain unfinished or partially implemented despite claimed completion | AC-1, AC-4, AC-5, AC-6, AC-8 remain incomplete |
| 4 | Moved the verified schema migration, frontend auth/CSRF wiring, SFTP UI flows, and HTTPS redirect fix out of active work; resolved the matching open issues; kept broader AC completion blocked on missing SFTP permissions display and incomplete AC-driven coverage for nodes, terminal, SFTP, TLS/HSTS, and concurrency | Round 4 materially fixed the Round 3 blockers, but the original plan still requires additional implementation and verification beyond those fixes | AC-2, AC-4, AC-5, AC-6, AC-7, AC-8 remain incomplete |

#### Active Tasks
<!-- Map each task to its target Acceptance Criterion -->
| Task | Target AC | Status | Notes |
|------|-----------|--------|-------|
| Initialize Go project with module structure | AC-1, AC-6 | completed | Backend foundation created |
| Implement user registration endpoint | AC-1 | completed | With bcrypt password hashing |
| Implement user login endpoint | AC-1 | completed | With session token generation |
| Implement credential encryption/decryption utilities | AC-2, AC-6 | completed | AES-256-GCM with PBKDF2 |
| Initialize Next.js 15 project with App Router | AC-1, AC-2, AC-3, AC-4 | completed | Frontend foundation created |
| Implement Go backend API for node CRUD | AC-2 | completed | Handlers and routes exist and are now wired through session-cookie auth plus CSRF protection |
| Add validation for node configuration | AC-2 | completed | Host format and port range validation exist in backend handlers |
| Implement WebSocket handler for terminal | AC-3 | completed | WebSocket endpoint and SSH session setup exist |
| Establish SSH client connections | AC-3 | completed | Password and private-key SSH auth paths exist |
| Proxy bidirectional data (WebSocket ↔ SSH) | AC-3 | completed | stdin/stdout/stderr proxying exists |
| Integrate xterm.js in frontend | AC-3 | completed | Terminal page uses xterm.js and fit addon |
| Handle terminal resize events | AC-3 | completed | Resize messages are sent and mapped to `WindowChange` |
| Implement terminal error handling | AC-3, AC-8 | pending | Frontend transport now derives from runtime origin, but terminal failure/reconnect behavior is still not covered by dedicated tests |
| Implement SFTP session management | AC-4 | completed | SFTP session creation exists |
| Create REST API for SFTP operations | AC-4 | completed | Backend list/download/upload/delete/mkdir handlers exist |
| Build Next.js SFTP file browser UI | AC-4 | completed | Browse/upload/download/delete/mkdir UI exists, but permissions are still not displayed |
| Implement file download functionality | AC-4 | completed | Backend and frontend download paths exist |
| Implement HTTPS/TLS support | AC-5 | pending | TLS serving and HSTS exist, but HTTPS/WSS behavior is not yet covered by dedicated tests |
| Add comprehensive error handling | AC-8 | pending | Auth/session bootstrap improved, but terminal, SFTP, and resilience/error-path coverage remain incomplete |
| Create Docker configuration | AC-5 | pending | Containerized deployment |
| Write tests for critical functions | AC-1, AC-3, AC-6 | pending | Auth and DB migration now have stronger coverage, but node CRUD, SSH/WebSocket, SFTP, TLS/HSTS, and concurrency tests are still missing |

### Completed and Verified
<!-- Only move tasks here after Codex verification -->
| AC | Task | Completed Round | Verified Round | Evidence |
|----|------|-----------------|----------------|----------|
| AC-1, AC-6 | Initialize Go project with module structure | 0 | 1 | `backend/cmd/server/main.go` builds with `go build ./cmd/server` during review |
| AC-1, AC-2, AC-6 | Set up SQLite database schema (users, nodes, credentials) | 0 | 4 | `backend/internal/database/database.go` creates the target schema and migrates legacy `nodes.encrypted_credentials` databases safely; `backend/internal/database/database_test.go` now covers fresh install, legacy migration, and idempotence |
| AC-1 | Implement user registration endpoint | 1 | 1 | `backend/internal/handlers/auth.go` exposes `POST /api/auth/register` and backend tests pass |
| AC-1 | Implement user login endpoint | 1 | 1 | `backend/internal/handlers/auth.go` exposes `POST /api/auth/login` and backend tests pass |
| AC-2, AC-6 | Implement credential encryption/decryption utilities | 0 | 1 | `backend/pkg/crypto/crypto.go` and `go test ./...` verified the crypto package |
| AC-1, AC-2, AC-3, AC-4 | Initialize Next.js project with App Router | 0 | 1 | App Router pages exist under `frontend/app`, though feature completeness is still pending |
| AC-2 | Build Next.js frontend for node management UI | 4 | 4 | `frontend/app/nodes/page.tsx` now supports list/add/edit/delete and bootstraps session state from `/api/auth/me` |
| AC-2 | Integrate frontend with backend API | 4 | 4 | `frontend/lib/api.ts` now uses cookie-based auth, session-bound CSRF headers, and same-origin defaults instead of `localStorage` bearer tokens |
| AC-4 | Build Next.js SFTP file browser UI | 4 | 4 | `frontend/app/sftp/[nodeId]/page.tsx` now supports browse/upload/download/delete/mkdir flows and session bootstrap from `/api/auth/me` |
| AC-4 | Implement file upload functionality | 4 | 4 | `frontend/app/sftp/[nodeId]/page.tsx` posts multipart uploads to `backend/internal/sftp/handler.go` |
| AC-4 | Add file deletion and mkdir operations | 4 | 4 | `frontend/app/sftp/[nodeId]/page.tsx` and `backend/internal/sftp/handler.go` now implement delete and mkdir flows |
| AC-5 | Add HTTP to HTTPS redirect | 4 | 4 | `backend/cmd/server/main.go` now derives the HTTPS port from `LISTEN_ADDR` and rebuilds the redirect host safely |
| AC-5, AC-8 | Implement CSRF protection | 4 | 4 | Session rows now store `csrf_token`, `backend/internal/middleware/csrf.go` validates it against the authenticated session, and `backend/internal/handlers/auth_test.go` covers accept/reject/reuse cases |

### Explicitly Deferred
<!-- Items here require strong justification -->
| Task | Original AC | Deferred Since | Justification | When to Reconsider |
|------|-------------|----------------|---------------|-------------------|

### Open Issues
<!-- Issues discovered during implementation -->
| Issue | Discovered Round | Blocking AC | Resolution Path |
|-------|-----------------|-------------|-----------------|
| The password-derived encryption key is still stored in the `sessions` table, so active-session credentials can be decrypted from the database without re-supplying the user's password | 4 | AC-6 | Stop persisting the unwrap/decryption key directly in SQLite sessions, or wrap it with a server-held secret; add a regression test proving DB data alone is insufficient to decrypt credentials |
| TLS support is partially improved, but HTTPS redirect/HSTS/WSS behavior is still not verified by dedicated tests | 1 | AC-5 | Add targeted tests for redirect target correctness, HSTS header behavior, and secure WebSocket/API transport under HTTPS configuration |
| Acceptance-criteria coverage is still incomplete outside auth/database migration and a few node ownership checks | 1 | AC-2, AC-3, AC-4, AC-5, AC-6, AC-7, AC-8 | Add AC-driven tests for node CRUD success paths, terminal auth/failure/resize behavior, SFTP happy/failure paths, credential decryption failure cases, TLS/HSTS, and multi-session concurrency |
| SFTP UI does not display file permissions even though the backend returns them in `mode` | 4 | AC-4 | Surface `mode` in `frontend/app/sftp/[nodeId]/page.tsx` and verify permissions/metadata rendering end to end |
