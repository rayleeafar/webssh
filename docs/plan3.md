# WebSSH Manager — Feature Implementation Plan

**Features**: TOTP-Based Two-Factor Authentication + Single-Binary Build System
**Date**: 2026-03-19
**Status**: Ready for implementation

---

## Table of Contents

1. [Codebase Orientation](#1-codebase-orientation)
2. [Feature 1: TOTP 2FA](#2-feature-1-totp-2fa)
   - [2.1 Backend — Database Migration](#21-backend--database-migration)
   - [2.2 Backend — Models](#22-backend--models)
   - [2.3 Backend — TOTP Service Methods](#23-backend--totp-service-methods)
   - [2.4 Backend — Modified Login Flow](#24-backend--modified-login-flow)
   - [2.5 Backend — New HTTP Handlers](#25-backend--new-http-handlers)
   - [2.6 Backend — Route Registration](#26-backend--route-registration)
   - [2.7 Frontend — Login Page (2FA Step 2)](#27-frontend--login-page-2fa-step-2)
   - [2.8 Frontend — Settings Page](#28-frontend--settings-page)
   - [2.9 Frontend — Header Navigation](#29-frontend--header-navigation)
   - [2.10 Frontend — API Utility Updates](#210-frontend--api-utility-updates)
3. [Feature 2: Single-Binary Build with Makefile](#3-feature-2-single-binary-build-with-makefile)
   - [3.1 Frontend — next.config.ts Static Export](#31-frontend--nextconfigts-static-export)
   - [3.2 Backend — Embedded Static File Server](#32-backend--embedded-static-file-server)
   - [3.3 Backend — Static Placeholder](#33-backend--static-placeholder)
   - [3.4 Backend — main.go Static Handler Registration](#34-backend--maingo-static-handler-registration)
   - [3.5 Makefile](#35-makefile)
4. [Implementation Order](#4-implementation-order)
5. [Gotchas and Cross-Cutting Concerns](#5-gotchas-and-cross-cutting-concerns)
6. [Testing Checklist](#6-testing-checklist)

---

## 1. Codebase Orientation

Before diving into changes, here is the critical structure to have in mind:

```
demo_webssh/
├── backend/
│   ├── cmd/server/main.go          # Entry point, route registration, mux wiring
│   ├── internal/
│   │   ├── auth/auth.go            # Service: Register, Login, ValidateSession, RevokeSession
│   │   ├── config/config.go        # Env-based config struct
│   │   ├── database/database.go    # initialize() + all migrate*() functions
│   │   ├── handlers/
│   │   │   ├── auth.go             # AuthHandler (Register, Login, Logout, Me)
│   │   │   └── auth_test.go        # Handler-level tests using httptest
│   │   ├── middleware/
│   │   │   ├── auth.go             # AuthMiddleware, extractToken, GetUserFromContext
│   │   │   └── csrf.go             # CSRFMiddleware
│   │   └── models/models.go        # User, Node, Session, NodeSysInfo structs
│   ├── pkg/crypto/crypto.go        # GenerateSalt, DeriveKey, Encrypt, Decrypt (AES-256-GCM)
│   └── go.mod                      # module github.com/webssh/manager
├── frontend/
│   ├── app/
│   │   ├── layout.tsx              # Root layout (no nav — each page is standalone)
│   │   ├── globals.css             # Neon Noir design system CSS
│   │   ├── login/page.tsx          # Login form (apiPost → setCsrfToken → push /dashboard)
│   │   └── dashboard/
│   │       ├── page.tsx            # Main dashboard with nodes, terminals, panels
│   │       └── components/
│   │           └── Header.tsx      # Header with username + LOGOUT button
│   ├── lib/api.ts                  # API_BASE, csrf token management, apiRequest/apiPost/etc.
│   ├── next.config.ts              # output: 'standalone', rewrites for /api and /ws
│   └── package.json                # next 16.x, react 19.x, xterm, tailwindcss
└── docs/
```

**Key patterns to maintain throughout all new code:**

- All state-mutating API calls use CSRF middleware: `authMiddleware(csrfMiddleware(handler))`.
- Handler tests live in the same package as handlers (e.g., `internal/handlers/`) and use `:memory:` SQLite databases via `database.Initialize`.
- AES-256-GCM encryption for sensitive data at rest uses `pkg/crypto.Encrypt(plaintext, masterKey)`.
- The neon noir design language: dark backgrounds (`#050508`, `rgba(13,13,26,0.95)`), cyan (`#00ffff`) accent, Orbitron/Rajdhani/JetBrains Mono fonts, CSS classes `.neon-input`, `.corner-tl/tr/bl/br`.
- The login page handles `apiPost` → parse JSON → `setCsrfToken` → `router.push`. The 2FA step must slot into this same pattern.

---

## 2. Feature 1: TOTP 2FA

### 2.1 Backend — Database Migration

**File to modify**: `backend/internal/database/database.go`

Add two new migrate functions at the bottom of the file and call them from `migrate()`.

**Step A — Add `migrateTOTP` function:**

Uses the additive column pattern already established for `migrateSessions` (check column existence via `pragma_table_info`, then `ALTER TABLE ... ADD COLUMN`). Do not drop or recreate the users table.

```
migrateTOTP(db *sql.DB) error:
  - ALTER TABLE users ADD COLUMN totp_secret TEXT NOT NULL DEFAULT ''
    (only if 'totp_secret' column not present in pragma_table_info)
  - ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0
    (only if 'totp_enabled' column not present)
```

**Step B — Add `migrateTempTokens` function:**

```sql
CREATE TABLE IF NOT EXISTS temp_tokens (
    token TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    encrypted_key TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
)
```

The `encrypted_key` column stores the already-wrapped encryption key (same value that would go into `sessions.encryption_key`) so that when the TOTP step completes, the full session can be created without repeating the PBKDF2 derivation.

**Step C — Call both from `migrate()`:**

Add calls after step 4 (migrateSessions) and before step 5 (migrateNodeSysInfo):

```go
// 4b. TOTP columns on users table
if err := migrateTOTP(db); err != nil {
    return fmt.Errorf("failed to migrate totp: %w", err)
}

// 4c. Temp tokens for pending 2FA logins
if err := migrateTempTokens(db); err != nil {
    return fmt.Errorf("failed to migrate temp_tokens: %w", err)
}
```

**Gotcha**: The `migrate()` function calls sub-functions sequentially and all share the same `*sql.DB`. Any function that fails causes the whole migration to fail at startup. Make sure `migrateTOTP` is purely additive (never `DROP` or `ALTER` in a destructive way).

---

### 2.2 Backend — Models

**File to modify**: `backend/internal/models/models.go`

Add fields to `User` struct and add a `TempToken` struct:

```go
// In User struct — add after CreatedAt:
TOTPSecret  string `json:"-"`    // AES-encrypted TOTP secret, empty if not set
TOTPEnabled bool   `json:"-"`

// New struct at the bottom:
type TempToken struct {
    Token        string
    UserID       int
    EncryptedKey string    // the wrapped encryption key, ready to put in sessions
    ExpiresAt    time.Time
    CreatedAt    time.Time
}
```

These fields are never sent to the client (`json:"-"`), which matches the pattern of `PasswordHash`, `EncryptionKey`, etc.

---

### 2.3 Backend — TOTP Service Methods

**File to modify**: `backend/internal/auth/auth.go`

**Step A — Add Go dependency** (run before writing code):

```
cd backend && go get github.com/pquerna/otp/totp && go get github.com/pquerna/otp
```

The `pquerna/otp` package is the canonical Go TOTP library. It implements RFC 6238 and is widely used.

**Step B — Add TOTP methods to `Service`:**

```
GenerateTOTPSecret(userID int) (secret string, otpauthURL string, err error)
```
- Call `totp.Generate(totp.GenerateOpts{Issuer: "WebSSH Manager", AccountName: <username>})`
- The returned `Key.Secret()` is the base32 TOTP secret (store this encrypted)
- The returned `Key.URL()` is the `otpauth://` URI used by authenticator apps
- Do NOT store to DB yet — just return the secret + URL so the caller can show the QR code. Storage happens on confirm (EnableTOTP).

```
EnableTOTP(userID int, plaintextSecret string, code string) error
```
- First call `totp.Validate(code, plaintextSecret)` — if false, return `fmt.Errorf("invalid TOTP code")`
- Encrypt the secret: `encryptedSecret, err := crypto.Encrypt(plaintextSecret, s.masterKey)`
- `UPDATE users SET totp_secret = ?, totp_enabled = 1 WHERE id = ?`

```
DisableTOTP(userID int, code string) error
```
- Fetch `totp_secret` for the user
- `decryptedSecret, err := crypto.Decrypt(totp_secret, s.masterKey)`
- Call `totp.Validate(code, decryptedSecret)` — if false, return error
- `UPDATE users SET totp_secret = '', totp_enabled = 0 WHERE id = ?`

```
ValidateTOTP(userID int, code string) error
```
- Fetch `totp_secret` for the user
- Decrypt secret with master key
- Call `totp.Validate(code, decryptedSecret)` — if false, return error

```
TOTPStatus(userID int) (enabled bool, err error)
```
- `SELECT totp_enabled FROM users WHERE id = ?`
- Used by the settings GET endpoint.

```
CreateTempToken(userID int, wrappedKey string) (string, error)
```
- Generate 32-byte random token (use existing `generateToken()`)
- `INSERT INTO temp_tokens (token, user_id, encrypted_key, expires_at) VALUES (?, ?, ?, ?)`
- `expires_at = time.Now().Add(5 * time.Minute)`
- Return token string

```
ConsumeTempToken(token string) (*models.TempToken, error)
```
- Use a DB transaction:
  - `SELECT token, user_id, encrypted_key, expires_at FROM temp_tokens WHERE token = ? AND expires_at > ?`
  - If not found: `return nil, fmt.Errorf("invalid or expired temp token")`
  - `DELETE FROM temp_tokens WHERE token = ?`
  - Commit
- Return the `TempToken`

---

### 2.4 Backend — Modified Login Flow

**File to modify**: `backend/internal/auth/auth.go`

Add a discriminated result type for the `Login` method:

```go
type LoginResult struct {
    Session     *models.Session
    TempToken   string
    Requires2FA bool
}
```

Change the method signature:
```go
func (s *Service) Login(username, password string) (*LoginResult, error)
```

**Modified `Login` logic:**

```
1. Query user (id, username, password_hash, encryption_key_salt, totp_enabled)
2. bcrypt.CompareHashAndPassword — same as before
3. Derive encryptionKey, wrap it with masterKey — same as before
4. If totp_enabled == 1:
   a. CreateTempToken(user.ID, wrappedKey)
   b. Return &LoginResult{TempToken: token, Requires2FA: true}, nil
5. If totp_enabled == 0:
   a. Generate session token + CSRF token
   b. INSERT INTO sessions
   c. Return &LoginResult{Session: &models.Session{...}}, nil
```

**File to modify**: `backend/internal/handlers/auth.go`

Update `Login` handler to interpret `LoginResult`:

```go
result, err := h.authService.Login(req.Username, req.Password)
if err != nil {
    http.Error(w, "Invalid credentials", http.StatusUnauthorized)
    return
}

if result.Requires2FA {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "requires_2fa": true,
        "temp_token":   result.TempToken,
    })
    return
}

// Full session — set cookie and return csrf_token as before
http.SetCookie(w, &http.Cookie{...})
json.NewEncoder(w).Encode(map[string]interface{}{
    "user_id":    result.Session.UserID,
    "csrf_token": result.Session.CSRFToken,
})
```

**Gotcha**: The frontend currently does `if (data.csrf_token) setCsrfToken(data.csrf_token)` immediately after login. When `requires_2fa: true`, there is no `csrf_token` yet. The frontend must check `data.requires_2fa` before calling `setCsrfToken` — see section 2.7.

---

### 2.5 Backend — New HTTP Handlers

**New file**: `backend/internal/handlers/totp.go`

Create a `TOTPHandler` struct:

```go
type TOTPHandler struct {
    authService  *auth.Service
    secureCookie bool
}

func NewTOTPHandler(authService *auth.Service, secureCookie bool) *TOTPHandler
```

**Handler methods:**

```
GET /api/auth/2fa/setup → SetupTOTP
```
- Requires auth middleware (user in context)
- Get `userID` from `middleware.GetUserFromContext`
- Call `authService.GenerateTOTPSecret(userID)` to get `(secret, otpauthURL, err)`
- Return JSON: `{"secret": "BASE32SECRET", "otpauth_url": "otpauth://..."}`
- The frontend renders the QR code client-side using `qrcode.react` (no PNG encoding needed on backend)
- Do NOT store the secret in DB at this stage.

```
POST /api/auth/2fa/enable → EnableTOTP
```
- Requires auth + CSRF middleware
- Body: `{"secret": "BASE32SECRET", "code": "123456"}`
- The frontend sends back the secret it received from `/setup` along with the OTP code
- Call `authService.EnableTOTP(userID, secret, code)`
- Return 200 JSON `{"message": "2FA enabled"}`

```
POST /api/auth/2fa/disable → DisableTOTP
```
- Requires auth + CSRF middleware
- Body: `{"code": "123456"}`
- Call `authService.DisableTOTP(userID, code)`
- Return 204

```
GET /api/auth/2fa/status → GetTOTPStatus
```
- Requires auth middleware
- Call `authService.TOTPStatus(userID)`
- Return JSON `{"enabled": true/false}`

```
POST /api/auth/2fa/verify → VerifyTOTP
```
- No auth middleware required (user has no session yet)
- No CSRF required (no session exists to bind a CSRF token to)
- Body: `{"temp_token": "...", "code": "123456"}`
- Call `authService.ConsumeTempToken(temp_token)` — gets back `*TempToken` with `UserID`, `EncryptedKey`
- Call `authService.ValidateTOTP(tempToken.UserID, code)` — validate code
- Generate session token + CSRF token
- `INSERT INTO sessions (token, user_id, encryption_key, csrf_token, expires_at)` using `tempToken.EncryptedKey` (no PBKDF2 re-derivation needed)
- Set `session_token` cookie (HttpOnly, Secure=secureCookie, SameSite=Lax, MaxAge=86400)
- Return 200 JSON `{"user_id": ..., "csrf_token": "..."}`

---

### 2.6 Backend — Route Registration

**File to modify**: `backend/cmd/server/main.go`

Add after `authHandler := ...`:

```go
totpHandler := handlers.NewTOTPHandler(authService, cfg.EnableTLS)
```

Add routes to the mux:

```go
// 2FA setup and management (requires full session auth)
mux.Handle("/api/auth/2fa/setup",   authMiddleware(http.HandlerFunc(totpHandler.SetupTOTP)))
mux.Handle("/api/auth/2fa/status",  authMiddleware(http.HandlerFunc(totpHandler.GetTOTPStatus)))
mux.Handle("/api/auth/2fa/enable",  authMiddleware(csrfMiddleware(http.HandlerFunc(totpHandler.EnableTOTP))))
mux.Handle("/api/auth/2fa/disable", authMiddleware(csrfMiddleware(http.HandlerFunc(totpHandler.DisableTOTP))))

// 2FA verification — no auth (pre-session)
mux.HandleFunc("/api/auth/2fa/verify", totpHandler.VerifyTOTP)
```

---

### 2.7 Frontend — Login Page (2FA Step 2)

**File to modify**: `frontend/app/login/page.tsx`

Add state for the two-step flow:

```typescript
const [step, setStep] = useState<'credentials' | '2fa'>('credentials')
const [tempToken, setTempToken] = useState('')
const [totpCode, setTotpCode] = useState('')
```

**Modify `handleSubmit`:**

```typescript
const res = await apiPost('/api/auth/login', { username, password })
if (!res.ok) throw new Error('Invalid credentials')
const data = await res.json()

if (data.requires_2fa) {
  setTempToken(data.temp_token)
  setStep('2fa')
  setLoading(false)
  return
}

if (data.csrf_token) setCsrfToken(data.csrf_token)
router.push('/dashboard')
```

**Add `handleVerify` function** (use `apiPost` since `/api/auth/2fa/verify` is in skipCsrf list — see 2.10):

```typescript
const handleVerify = async (e: React.FormEvent) => {
  e.preventDefault()
  setError('')
  setLoading(true)
  try {
    const res = await apiPost('/api/auth/2fa/verify', {
      temp_token: tempToken,
      code: totpCode,
    })
    if (!res.ok) throw new Error('Invalid authenticator code')
    const data = await res.json()
    if (data.csrf_token) setCsrfToken(data.csrf_token)
    router.push('/dashboard')
  } catch (err) {
    setError(err instanceof Error ? err.message : '2FA verification failed')
  } finally {
    setLoading(false)
  }
}
```

**Render step 2 form** (inside the same outer container/corner div):

```tsx
{step === '2fa' && (
  <form onSubmit={handleVerify}>
    <div style={{textAlign:'center', marginBottom:24}}>
      <div style={{fontFamily:"'Orbitron',sans-serif", fontSize:13, color:'#6070a0', letterSpacing:'0.2em'}}>
        TWO-FACTOR AUTHENTICATION
      </div>
      <div style={{fontSize:12, color:'#404060', marginTop:8}}>
        Enter the 6-digit code from your authenticator app
      </div>
    </div>
    {error && <div className="error-box">{error}</div>}
    <input
      type="text"
      inputMode="numeric"
      pattern="[0-9]{6}"
      maxLength={6}
      required
      value={totpCode}
      onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, ''))}
      className="neon-input"
      autoComplete="one-time-code"
      placeholder="000000"
    />
    <button type="submit" disabled={loading}>
      {loading ? 'VERIFYING...' : 'VERIFY CODE'}
    </button>
    <button type="button" onClick={() => { setStep('credentials'); setError('') }}>
      Back to login
    </button>
  </form>
)}
```

---

### 2.8 Frontend — Settings Page

**New file**: `frontend/app/settings/page.tsx`

This page requires authentication. Follow the same guard pattern as `dashboard/page.tsx`: call `/api/auth/me` in `useEffect`, push to `/login` on 401.

**npm dependency to add** (run before writing the page):

```
cd frontend && npm install qrcode.react
```

`qrcode.react` renders an `otpauth://` URL as a QR code client-side. Import as:
```typescript
import { QRCodeSVG } from 'qrcode.react'
```

**Page state:**

```typescript
type FlowState = 'idle' | 'setup' | 'disabling'

const [totpEnabled, setTotpEnabled] = useState(false)
const [flowState, setFlowState] = useState<FlowState>('idle')
const [setupData, setSetupData] = useState<{secret: string, otpauth_url: string} | null>(null)
const [code, setCode] = useState('')
const [error, setError] = useState('')
const [success, setSuccess] = useState('')
```

**`handleStartSetup`**: `GET /api/auth/2fa/setup` → set `setupData`, set `flowState = 'setup'`

**`handleConfirmEnable`**: `POST /api/auth/2fa/enable` with `{secret: setupData.secret, code}` → on success, set `totpEnabled = true`, reset flow

**`handleStartDisable`**: set `flowState = 'disabling'`

**`handleConfirmDisable`**: `POST /api/auth/2fa/disable` with `{code}` → on success, set `totpEnabled = false`, reset flow

**Render sections** (styled in neon noir):

- **Status row**: Badge showing "2FA: ENABLED" (cyan) or "2FA: DISABLED" (gray). Button "ENABLE 2FA" or "DISABLE 2FA".
- **Setup flow** (`flowState === 'setup'`):
  1. `<QRCodeSVG value={setupData.otpauth_url} size={200} />` with dark background and cyan border
  2. Raw base32 secret in a monospace code block (for manual entry)
  3. 6-digit input + "CONFIRM AND ENABLE" button
  4. Instruction: "Scan the QR code with Google Authenticator or Authy, then enter the code to confirm."
- **Disabling flow** (`flowState === 'disabling'`):
  1. Warning text: "Enter your current TOTP code to disable two-factor authentication."
  2. 6-digit input + "CONFIRM DISABLE" button
- **Back link** to `/dashboard` at the top

---

### 2.9 Frontend — Header Navigation

**File to modify**: `frontend/app/dashboard/components/Header.tsx`

Add a "SETTINGS" link between the username display and the LOGOUT button, styled to match the neon noir aesthetic (Orbitron font, cyan hover, subtle border).

---

### 2.10 Frontend — API Utility Updates

**File to modify**: `frontend/lib/api.ts`

Add `/api/auth/2fa/verify` to the `skipCsrf` list so `apiPost` can be used without triggering a `/api/auth/me` preflight (there is no session yet at that point):

```typescript
const skipCsrf = [
  '/api/auth/login',
  '/api/auth/register',
  '/api/auth/logout',
  '/api/auth/2fa/verify',  // ← add this
]
```

---

## 3. Feature 2: Single-Binary Build with Makefile

### 3.1 Frontend — next.config.ts Static Export

**File to modify**: `frontend/next.config.ts`

Add an environment-variable toggle so the existing Docker behavior is unchanged:

```typescript
const isBuild = process.env.NEXT_STATIC_EXPORT === 'true'

const nextConfig = isBuild
  ? {
      output: 'export',
      trailingSlash: true,
      // No rewrites() — static export does not support runtime rewrites
      // API calls go to the same origin at runtime (Go serves both)
      env: {
        NEXT_PUBLIC_WS_URL: process.env.NEXT_PUBLIC_WS_URL || '',
      },
    }
  : {
      // Existing dev/Docker config unchanged
      reactStrictMode: true,
      output: 'standalone',
      env: {
        NEXT_PUBLIC_WS_URL: process.env.NEXT_PUBLIC_WS_URL || '',
      },
      async rewrites() {
        const backendUrl = process.env.BACKEND_URL || 'http://backend:8080'
        return [
          { source: '/api/:path*', destination: `${backendUrl}/api/:path*` },
          { source: '/ws/:path*', destination: `${backendUrl}/ws/:path*` },
          { source: '/health', destination: `${backendUrl}/health` },
        ]
      },
    }

export default nextConfig
```

The Makefile sets `NEXT_STATIC_EXPORT=true` when invoking `make frontend-build`.

**Critical constraints of `output: 'export'`:**

1. No `getServerSideProps`, no Next.js API routes — this project has neither (uses external Go API).
2. All existing pages use `'use client'` + `useEffect` for data fetching — no SSR — so static export works as-is.
3. `rewrites()` is incompatible with `output: 'export'` — removed in static branch.
4. `trailingSlash: true` ensures `/dashboard` exports as `dashboard/index.html`, which the Go file server handles correctly.
5. `NEXT_PUBLIC_API_URL` defaults to `''` in `lib/api.ts` (same-origin) — no change needed for single-binary production.

---

### 3.2 Backend — Embedded Static File Server

Use build tags to allow the backend to compile without frontend files during development:

**New file**: `backend/internal/static/static_embed.go`

```go
//go:build embed

package static

import (
    "embed"
    "io/fs"
    "net/http"
    "strings"
)

//go:embed all:files
var embeddedFiles embed.FS

// Handler returns an http.Handler that serves embedded static files with
// SPA fallback: unknown paths return index.html.
func Handler() http.Handler {
    fsys, _ := fs.Sub(embeddedFiles, "files")
    fileServer := http.FileServer(http.FS(fsys))
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        path := strings.TrimPrefix(r.URL.Path, "/")
        if path == "" {
            path = "index.html"
        }
        if _, err := fs.Stat(fsys, path); err != nil {
            // SPA fallback — serve index.html
            r2 := *r
            r2.URL.Path = "/"
            fileServer.ServeHTTP(w, &r2)
            return
        }
        fileServer.ServeHTTP(w, r)
    })
}
```

**New file**: `backend/internal/static/static_noembed.go`

```go
//go:build !embed

package static

import "net/http"

// Handler returns nil when built without the embed tag (development mode).
func Handler() http.Handler { return nil }
```

The `//go:embed all:files` directive embeds everything in `backend/internal/static/files/`. The `all:` prefix prevents accidental omission of files whose names start with `.` or `_`.

---

### 3.3 Backend — Static Placeholder

**Files to create:**

- `backend/internal/static/files/.gitkeep` — empty placeholder (the embed directive requires the directory to exist)

**Add to `.gitignore`** (project root):

```
backend/internal/static/files/*
!backend/internal/static/files/.gitkeep
```

This ensures built frontend files are never accidentally committed.

---

### 3.4 Backend — main.go Static Handler Registration

**File to modify**: `backend/cmd/server/main.go`

Add import:

```go
"github.com/webssh/manager/internal/static"
```

Add after all `/api/*` and `/ws/*` routes, at the end of mux setup:

```go
// Serve embedded frontend (only active when built with -tags embed).
// All non-API paths serve the SPA index.html as fallback.
if h := static.Handler(); h != nil {
    mux.Handle("/", h)
}
```

**Routing order**: Go's `net/http.ServeMux` uses longest-match prefix. All explicit `/api/...` and `/ws/...` patterns are registered first, so the `/` catch-all only receives unmatched requests.

---

### 3.5 Makefile

**New file**: `Makefile` (project root)

```makefile
.PHONY: all build frontend-build copy-frontend backend-build clean dev run

BINARY     := webssh
BACKEND    := ./backend
FRONTEND   := ./frontend
STATIC_DIR := $(BACKEND)/internal/static/files
OUT_DIR    := $(FRONTEND)/out

all: build

## Build the complete single binary (frontend embedded in backend)
build: frontend-build copy-frontend backend-build

## Build the Next.js frontend in static export mode
frontend-build:
	NEXT_STATIC_EXPORT=true cd $(FRONTEND) && npm run build

## Copy built frontend files into the Go embed directory
copy-frontend:
	rm -rf $(STATIC_DIR)/*
	cp -r $(OUT_DIR)/. $(STATIC_DIR)/
	@echo "Frontend files copied to $(STATIC_DIR)"

## Compile the Go backend with embedded frontend
backend-build:
	cd $(BACKEND) && go build \
		-tags embed \
		-ldflags "-s -w" \
		-o ../$(BINARY) \
		./cmd/server/

## Remove all build artifacts
clean:
	rm -rf $(OUT_DIR)
	rm -rf $(STATIC_DIR)/*
	touch $(STATIC_DIR)/.gitkeep
	rm -f ./$(BINARY)

## Run both servers in development mode (requires MASTER_SECRET env var)
dev:
	@test -n "$$MASTER_SECRET" || (echo "ERROR: MASTER_SECRET env var must be set"; exit 1)
	@echo "Starting backend on :8080 ..."
	cd $(BACKEND) && go run ./cmd/server/ &
	@echo "Starting frontend dev server on :3000 ..."
	@echo "Tip: Set BACKEND_URL=http://localhost:8080 for API proxying"
	BACKEND_URL=$${BACKEND_URL:-http://localhost:8080} cd $(FRONTEND) && npm run dev

## Run the compiled single binary
run:
	./$(BINARY)
```

**Notes:**

- `frontend-build`: `NEXT_STATIC_EXPORT=true` activates `output: 'export'` in `next.config.ts`; generates `frontend/out/`
- `copy-frontend`: `cp -r out/.` copies directory *contents* (not the directory itself) into `STATIC_DIR`
- `backend-build`: `-tags embed` selects `static_embed.go`; `-ldflags "-s -w"` strips debug info to reduce binary size
- `clean`: removes artifacts and recreates `.gitkeep` placeholder
- `dev`: uses `&` to background the Go process so both servers run concurrently; user terminates with Ctrl-C
- `run`: requires `MASTER_SECRET` (and optionally `DB_PATH`, `LISTEN_ADDR`) as env vars

---

## 4. Implementation Order

Steps are sequenced to respect dependencies and allow incremental testing:

### Phase 1: Database Foundation

1. Add `migrateTOTP()` and `migrateTempTokens()` to `database/database.go`
2. Add `TOTPSecret`, `TOTPEnabled` to `User` struct in `models/models.go`
3. Add `TempToken` struct to `models/models.go`
4. Run: `cd backend && go test ./...` — all existing tests should pass (additive migration only)

### Phase 2: Backend Service Layer

5. Run: `cd backend && go get github.com/pquerna/otp/totp`
6. Add `GenerateTOTPSecret`, `EnableTOTP`, `DisableTOTP`, `ValidateTOTP`, `TOTPStatus` to `auth/auth.go`
7. Add `CreateTempToken`, `ConsumeTempToken` to `auth/auth.go`
8. Modify `Login` in `auth/auth.go` to return `*LoginResult`
9. Write unit tests for TOTP service methods

### Phase 3: Backend Handlers and Routes

10. Create `handlers/totp.go` with all four handlers
11. Update `handlers/auth.go` `Login` handler to interpret `LoginResult`
12. Add routes to `cmd/server/main.go`
13. Write handler tests in `handlers/totp_test.go`
14. Run: `cd backend && go test ./...` — all tests pass

### Phase 4: Frontend 2FA Flow

15. Update `lib/api.ts` skipCsrf list
16. Modify `app/login/page.tsx` for two-step login
17. Test login flow manually end-to-end (dev mode)

### Phase 5: Frontend Settings Page

18. Run: `cd frontend && npm install qrcode.react`
19. Create `app/settings/page.tsx`
20. Modify `dashboard/components/Header.tsx` to add Settings link
21. Test full 2FA enable/disable flow manually

### Phase 6: Static Build Infrastructure

22. Create `backend/internal/static/files/.gitkeep`
23. Create `backend/internal/static/static_embed.go` (with `//go:build embed`)
24. Create `backend/internal/static/static_noembed.go` (with `//go:build !embed`)
25. Modify `frontend/next.config.ts` to support `NEXT_STATIC_EXPORT` toggle
26. Modify `backend/cmd/server/main.go` to import and register static handler

### Phase 7: Makefile and End-to-End Test

27. Create `Makefile` at project root
28. Add static files to `.gitignore`
29. Run: `make build` — verify it produces `./webssh`
30. Run: `MASTER_SECRET=testsecret ./webssh` — verify binary serves both API and frontend on `:8080`
31. Navigate to `http://localhost:8080` — verify frontend loads
32. Test: register, login with 2FA, verify in single-binary mode

---

## 5. Gotchas and Cross-Cutting Concerns

### TOTP Time Skew
`totp.Validate(code, secret)` from `pquerna/otp` uses ±30 second default window. This is standard and acceptable for usability.

### TOTP Secret Encryption Key
TOTP secrets are encrypted with `s.masterKey` (server master key), not the user's per-session derived key. This is intentional: the TOTP secret must be accessible at login time, before a session exists. This is the correct design, consistent with how the master key wraps the session encryption key.

### Temp Token Race Condition
If a user double-submits the verify form, two requests race to `ConsumeTempToken`. The DB transaction ensures only one succeeds — the second gets "invalid or expired temp token." The frontend should disable the verify button while loading (`setLoading(true)` before the request).

### WebSocket in Static Export Mode
`getWebSocketBase()` in `lib/api.ts` falls back to `window.location.host` when `NEXT_PUBLIC_WS_URL` is empty. In single-binary mode, WebSocket paths (`/ws/terminal`) are served by the same Go binary on the same host/port, so this fallback resolves correctly. No env var changes needed for production.

For `make dev` (split servers), set `NEXT_PUBLIC_WS_URL=ws://localhost:8080` since Next.js dev server does not proxy WebSocket upgrades.

### `trailingSlash: true` and Go File Server
With `trailingSlash: true`, Next.js exports `/dashboard` as `dashboard/index.html`. A request for `/dashboard` (no trailing slash) is redirected to `/dashboard/` by `http.FileServer` automatically. Verify this after `make build`.

### SPA Fallback for Client-Side Navigation
Direct navigation to `/dashboard` requests `dashboard/` from Go. With the files structure (`dashboard/index.html` exists), `http.FileServer` serves it. Unknown paths (true 404s) return the SPA `index.html` — standard behavior for single-page apps.

### `qrcode.react` Types
`qrcode.react` v4+ ships its own TypeScript declarations. If using v3 or earlier, install `@types/qrcode.react` separately.

### Makefile `cd` Behavior
Each Makefile recipe line runs in a new shell. Always use `cd dir && command` on a single logical line:
```makefile
# CORRECT
frontend-build:
	NEXT_STATIC_EXPORT=true cd $(FRONTEND) && npm run build

# WRONG — cd does not persist to next line
frontend-build:
	cd $(FRONTEND)
	npm run build
```

### Build Tag: `embed`
The `embed` build tag selects the static file embedding. It is not a reserved Go build tag name. Confirm with `go help buildconstraint` if needed.

### Docker Compose Regression
The `next.config.ts` change must not break `docker-compose up`. Since `NEXT_STATIC_EXPORT` is not set in `docker-compose.yml`, the existing `output: 'standalone'` branch remains active for Docker builds. Verify `docker-compose up --build` still works after the `next.config.ts` change.

---

## 6. Testing Checklist

### 2FA Feature

- [ ] User registers and logs in without 2FA — existing behavior unchanged
- [ ] User enables 2FA from settings page: QR code appears, code confirmed, enabled badge shown
- [ ] User logs out
- [ ] User logs in again: password accepted → 2FA code prompt appears
- [ ] User enters valid TOTP code → full session created → redirected to dashboard
- [ ] User enters invalid TOTP code → error shown, stays on step 2
- [ ] Temp token expires (wait 5+ minutes): verify returns 401
- [ ] User disables 2FA from settings → subsequent logins skip 2FA step
- [ ] Backend tests: `cd backend && go test ./internal/auth/... ./internal/handlers/...`

### Single-Binary Build

- [ ] `make build` completes without errors
- [ ] `MASTER_SECRET=test DB_PATH=./data/test.db ./webssh` starts on `:8080`
- [ ] `GET http://localhost:8080/` returns Next.js index.html (200)
- [ ] `GET http://localhost:8080/dashboard/` returns dashboard HTML (200)
- [ ] `GET http://localhost:8080/settings/` returns settings HTML (200)
- [ ] `GET http://localhost:8080/api/auth/me` returns 401 (Go handler, not static file)
- [ ] `GET http://localhost:8080/nonexistent` returns SPA index.html (SPA fallback)
- [ ] `make clean` removes all build artifacts and recreates `.gitkeep`
- [ ] `make dev` starts both servers; API proxied correctly to `:8080`
- [ ] `docker-compose up --build` still works (no regression in Docker build)

---

## Files Summary

### New files to create:
| File | Purpose |
|------|---------|
| `backend/internal/handlers/totp.go` | TOTP HTTP handlers |
| `backend/internal/static/static_embed.go` | Embedded FS serving (build tag: embed) |
| `backend/internal/static/static_noembed.go` | No-op stub for dev builds |
| `backend/internal/static/files/.gitkeep` | Placeholder for embed directory |
| `frontend/app/settings/page.tsx` | 2FA management settings page |
| `Makefile` | Build system (project root) |

### Files to modify:
| File | Changes |
|------|---------|
| `backend/internal/database/database.go` | Add `migrateTOTP`, `migrateTempTokens` |
| `backend/internal/models/models.go` | Add TOTP fields to User, add TempToken struct |
| `backend/internal/auth/auth.go` | Add TOTP service methods, modify Login return type |
| `backend/internal/handlers/auth.go` | Update Login handler for LoginResult |
| `backend/cmd/server/main.go` | Register TOTP routes, import and register static handler |
| `frontend/app/login/page.tsx` | Add 2FA step 2 UI |
| `frontend/app/dashboard/components/Header.tsx` | Add Settings navigation link |
| `frontend/lib/api.ts` | Add `/api/auth/2fa/verify` to skipCsrf list |
| `frontend/next.config.ts` | Add NEXT_STATIC_EXPORT toggle for static output |
| `.gitignore` | Exclude `backend/internal/static/files/*` except `.gitkeep` |
