# WebSSH Manager — Extended Feature Plan

## Goal Description

Extend the WebSSH Manager with four capabilities:

1. **Proxy & Jump Server support** — SSH connections can route through a SOCKS5, HTTP, or HTTPS proxy, or through a jump server (bastion host). Jump servers themselves can also be reached via a proxy.
2. **System Info caching** — When a node is first added, the backend SSHes to it, collects basic system information (hostname, OS, CPU, memory, disk, IP), and stores it in the database. The stored data refreshes when the user clicks the sysinfo tab.
3. **Batch command execution** — A user selects one or more nodes, submits a shell command, and receives results from all selected nodes within a configurable timeout (default 60 s, maximum 600 s).
4. **Multiple SSH sessions per node** — The dashboard allows opening more than one terminal tab connected to the same node simultaneously. Each tab maintains an independent SSH session and PTY. Tabs can be freely opened, closed, and switched between.

---

## Acceptance Criteria

Following TDD philosophy, each criterion includes positive and negative tests for deterministic verification.

### Feature 1 — Proxy & Jump Server

- AC-1: The nodes table stores an optional proxy configuration (type, host, port, credential reference).
  - Positive Tests:
    - A node saved with `proxy_type='socks5'`, `proxy_host`, `proxy_port`, and `proxy_credential_id` can be retrieved with all proxy fields intact.
    - A node saved with no proxy fields has all proxy columns NULL.
  - Negative Tests:
    - Saving a node with `proxy_type='invalid'` is rejected or stored as-is but causes a dial error (not a silent no-op).

- AC-2: An SSH connection to a node configured with a SOCKS5 proxy routes all TCP through the proxy.
  - Positive Tests:
    - Given a node that is only reachable via a SOCKS5 proxy, opening its terminal tab establishes a working SSH session.
    - The `dialSSH` function is called with a custom `net.Conn` obtained through `golang.org/x/net/proxy` SOCKS5 dialer rather than a direct TCP dial.
  - Negative Tests:
    - If the SOCKS5 proxy address is unreachable, the WebSocket receives an error message and the SSH session does not open.

- AC-3: An SSH connection to a node configured with an HTTP or HTTPS proxy routes through an HTTP CONNECT tunnel.
  - Positive Tests:
    - Given a node reachable only through an HTTP CONNECT proxy, opening its terminal tab establishes a working session.
  - Negative Tests:
    - If the proxy returns a non-200 response to CONNECT, the connection fails with a descriptive error.

- AC-4: An SSH connection to a node configured as a jump-server connection SSHes to the jump host first, then dials the target from there.
  - Positive Tests:
    - Given a node whose only network path is through a jump host, opening the terminal establishes a working session via `jumpClient.Dial("tcp", target)`.
    - The jump host itself can have its own proxy configuration (SOCKS5 or HTTP), and the combined chain works end-to-end.
  - Negative Tests:
    - If the jump host SSH authentication fails, the terminal receives an error; the target node is never dialed.

- AC-5: The Node Add/Edit modal exposes proxy fields only when a proxy type is selected.
  - Positive Tests:
    - Selecting proxy type "SOCKS5" reveals host, port, and credential fields.
    - Selecting proxy type "Jump Server" reveals jump-host SSH fields (host, port, username, credential).
    - Selecting "None" hides all proxy fields.
  - Negative Tests:
    - Submitting the form with proxy type "SOCKS5" but an empty proxy host is rejected with a validation error before the request reaches the backend.

### Feature 2 — System Info Caching

- AC-6: When a node is successfully created via `POST /api/nodes`, the backend attempts to SSH into the node, collect system info, and store it in a `node_sysinfo` table.
  - Positive Tests:
    - After creating a node that is reachable, `SELECT * FROM node_sysinfo WHERE node_id = ?` returns a row with non-empty hostname, os, cpu_model, mem_total, disk_total, collected_at.
    - The node creation API response is returned immediately; the sysinfo collection runs asynchronously and does not block the HTTP response.
  - Negative Tests:
    - If the node is unreachable at creation time, no sysinfo row is inserted, but the node itself is still created successfully.
    - Sysinfo collection failure does not cause the `POST /api/nodes` handler to return an error status.

- AC-7: `GET /api/nodes/{id}/sysinfo` refreshes the stored record by re-collecting from the live node and returns the updated data.
  - Positive Tests:
    - The response JSON matches the fields in the `node_sysinfo` table after the request completes.
    - Calling the endpoint twice in sequence returns data with an updated `collected_at` timestamp on the second call (assuming at least 1 second apart).
  - Negative Tests:
    - If the SSH connection fails during refresh, the endpoint returns the last successfully cached row (not an error), along with a `stale: true` flag or equivalent indicator.
    - Requesting sysinfo for a node that belongs to a different user returns 403.

- AC-8: The frontend sysinfo tab displays cached data immediately on open, then refreshes via `GET /api/nodes/{id}/sysinfo`.
  - Positive Tests:
    - Opening the sysinfo tab renders previously cached values without waiting for a live SSH round-trip.
    - After the refresh request completes, displayed values update to reflect any changes.
  - Negative Tests:
    - If the refresh call fails, the previously cached values remain visible (no blank/error screen).

### Feature 3 — Batch Command Execution

- AC-9: `POST /api/batch/exec` accepts a list of node IDs, a command string, and a timeout (1–600 s, default 60 s), executes the command concurrently on all nodes, and returns aggregated results.
  - Positive Tests:
    - Sending `{nodeIds: [1,2], command: "hostname", timeout: 30}` returns a result entry for each node containing `nodeId`, `host`, `exitCode`, `stdout`, and `stderr`.
    - All nodes that respond within the timeout have `timedOut: false` and a non-empty `stdout`.
    - The total response time is bounded by `timeout + overhead`, not the sum of all node response times.
  - Negative Tests:
    - A node whose SSH connection fails returns an entry with `error` set and `stdout` empty; other nodes' results are unaffected.
    - A timeout value greater than 600 is rejected with 400 Bad Request.
    - A request with an empty `nodeIds` list is rejected with 400 Bad Request.
    - Requesting batch exec with a node that belongs to a different user returns 403 for that node's result entry (or the whole request is rejected).

- AC-10: Commands that do not complete within the timeout are cancelled and reported as timed-out.
  - Positive Tests:
    - A command of `sleep 120` with timeout 5 s returns `timedOut: true` for the affected node within ~5 s.
    - The SSH session for the timed-out node is closed after the timeout, not left open.
  - Negative Tests:
    - A command that produces no output before timing out still returns a result entry (not a missing entry).

- AC-11: The dashboard provides a Batch Exec UI: node multi-select, command input, submit, and a results table.
  - Positive Tests:
    - Selecting two nodes, entering a command, and clicking Run shows a results row for each node with stdout/stderr content.
    - Timed-out nodes are visually distinguished (e.g., a warning icon or "timed out" label).
  - Negative Tests:
    - Clicking Run with no nodes selected shows a validation message and does not send the request.
    - Clicking Run with an empty command field shows a validation message.

### Feature 4 — Multiple Sessions Per Node

- AC-12: Clicking a node in the sidebar always opens a new terminal tab, even if a tab for that node already exists.
  - Positive Tests:
    - Clicking node "A" twice creates two separate tabs, each with an independent WebSocket connection and PTY.
    - Both tabs to the same node show independent shell prompts; a command run in tab 1 does not appear in tab 2.
  - Negative Tests:
    - Closing one tab to node "A" does not close the other tab's SSH session.

- AC-13: Tabs can be freely switched; inactive tabs maintain their SSH sessions and terminal scroll buffers.
  - Positive Tests:
    - Switching away from a tab and back does not reconnect the WebSocket or reset the terminal buffer.
    - Output generated in a background tab is visible when the user switches back to it.
  - Negative Tests:
    - Switching tabs does not trigger a new `GET /api/auth/me` call or a new WebSocket handshake.

---

## Path Boundaries

### Upper Bound (Maximum Acceptable Scope)
All four features are implemented with full backend handler test coverage (table-driven tests using injectable fakes), frontend form validation, the `node_sysinfo` DB table with migration, a dedicated batch exec handler with per-node timeout cancellation via `context.WithTimeout`, and a polished dashboard Batch Exec panel with node multi-select and results table in Neon Noir style. The proxy dial chain supports SOCKS5, HTTP CONNECT, and jump-host-via-proxy combinations. Sysinfo collection on node creation is asynchronous (goroutine) and does not block the API response.

### Lower Bound (Minimum Acceptable Scope)
All four features satisfy their acceptance criteria: proxy/jump fields exist in the DB and are used when dialing; sysinfo is collected on node creation and stored in DB and is refreshed on demand; batch exec endpoint accepts node IDs + command + timeout and returns per-node results; multiple tabs to the same node each have independent SSH sessions. Frontend covers the required interactions (proxy fields in node modal, batch exec UI, multi-tab opening).

### Allowed Choices
- Can use: `golang.org/x/net/proxy` for SOCKS5 dialing; standard `net/http` transport for HTTP CONNECT proxy; `golang.org/x/crypto/ssh` `Client.Dial` for jump host chaining; `context.WithTimeout` + goroutines for batch concurrency; SQLite `ALTER TABLE` migration for new columns/tables.
- Can use: either a new `node_sysinfo` table or a JSON column on `nodes` for caching sysinfo — both are acceptable.
- Cannot use: external process spawning (no `exec.Command` for SSH) — all SSH must go through `golang.org/x/crypto/ssh`.
- Cannot use: storing proxy passwords in plaintext — must reuse the existing `credentials` table encryption path.
- Cannot use: blocking the `POST /api/nodes` response on sysinfo collection.

---

## Feasibility Hints and Suggestions

> **Note**: This section is for reference and understanding only. These are conceptual suggestions, not prescriptive requirements.

### Conceptual Approach

**Proxy/Jump Dial chain (Feature 1)**

```
func buildDialer(node Node, credsMap map[int]string) (net.Conn, error) {
    switch node.ProxyType {
    case "socks5":
        dialer, _ := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
        return dialer.Dial("tcp", targetAddr)
    case "http":
        return httpConnectDial(proxyAddr, targetAddr, proxyCreds)
    case "jump":
        jumpClient, _ := ssh.Dial("tcp", jumpAddr, jumpConfig)  // jump may itself use a proxy
        conn, _ := jumpClient.Dial("tcp", targetAddr)
        return conn, nil
    default:
        return net.Dial("tcp", targetAddr)
    }
}
```

The existing `sshDialFunc` field on `TerminalHandler` and `SFTPHandler` can be extended to accept the full `Node` record so the dialer can build the appropriate chain.

**Sysinfo on node creation (Feature 2)**

```go
// in nodes POST handler, after INSERT succeeds:
go func() {
    info, err := sysinfo.Collect(node, decryptedCreds)
    if err != nil { return }
    db.Exec(`INSERT OR REPLACE INTO node_sysinfo (...) VALUES (...)`, ...)
}()
```

**Batch exec (Feature 3)**

```go
results := make(chan NodeResult, len(nodeIDs))
for _, id := range nodeIDs {
    go func(nodeID int) {
        ctx, cancel := context.WithTimeout(r.Context(), timeout)
        defer cancel()
        out, err := runCommand(ctx, node, cmd)
        results <- NodeResult{NodeID: nodeID, Stdout: out, Err: err}
    }(id)
}
// collect all results
```

**Multi-tab per node (Feature 4)**

Remove the deduplication check in `openTab` in `frontend/app/dashboard/page.tsx` so every click always appends a new tab entry with a fresh `crypto.randomUUID()`. The tab `id` (UUID) is already the React key, so each `TerminalPane` is a fully independent component instance with its own WebSocket.

### Relevant References
- `backend/internal/ssh/terminal.go` — `sshDialFunc`, `defaultSSHDial`, `TerminalHandler`
- `backend/internal/handlers/sysinfo.go` — existing shell command collection pattern to reuse for DB caching
- `backend/internal/database/database.go` — migration pattern for new tables/columns
- `backend/cmd/server/main.go` — route registration pattern
- `frontend/app/dashboard/page.tsx` — `openTab`, `tabs` state, `Tab` interface
- `frontend/app/dashboard/components/NodeModal.tsx` — form structure to extend with proxy fields
- `frontend/app/dashboard/components/SystemInfo.tsx` — existing sysinfo display component

---

## Dependencies and Sequence

### Milestones

1. **Database Schema** — Foundation for Features 1 and 2
   - Add proxy columns to `nodes` table: `proxy_type`, `proxy_host`, `proxy_port`, `proxy_username`, `proxy_credential_id`
   - Create `node_sysinfo` table: `node_id`, `hostname`, `os`, `kernel`, `uptime`, `cpu_model`, `cpu_cores`, `cpu_load`, `mem_total_mb`, `mem_used_mb`, `disk_total_gb`, `disk_used_gb`, `disk_pct`, `ip`, `collected_at`
   - Both changes via a single migration in `database.go`

2. **Backend — Proxy Dial** — Depends on Milestone 1 (proxy columns must exist to be read)
   - Implement `buildProxyDialer` in `backend/internal/ssh/` or a new `proxy` subpackage
   - Thread node proxy config through `TerminalHandler` and `SFTPHandler` dial paths
   - Add `golang.org/x/net/proxy` dependency

3. **Backend — Sysinfo Caching** — Depends on Milestone 1 (node_sysinfo table)
   - Extract the shell-command collection logic in `sysinfo.go` into a reusable `Collect(node, creds)` function
   - Call it asynchronously in the `POST /api/nodes` handler after successful INSERT
   - Update `GET /api/nodes/{id}/sysinfo` to write back to DB and return cached data on SSH failure

4. **Backend — Batch Exec** — Independent of Milestones 1–3; can proceed in parallel
   - Implement `POST /api/batch/exec` handler with goroutine-per-node + `context.WithTimeout`
   - Register route in `main.go`

5. **Frontend — Node Modal Proxy Fields** — Depends on Milestone 2 (proxy columns in API response)
   - Extend `NodeModal.tsx` with proxy type selector and conditional proxy credential fields
   - Update `NodeSidebar.tsx` / nodes API calls to pass proxy fields

6. **Frontend — Multi-Tab Per Node** — Independent; can proceed any time
   - Remove nodeId deduplication in `openTab` in `dashboard/page.tsx`
   - Update tab display (node name + session index, e.g. "web-01 #2") to distinguish multiple tabs for the same node

7. **Frontend — Batch Exec UI** — Depends on Milestone 4 (batch exec endpoint)
   - Add "Batch Exec" tab to the bottom panel (`BottomTabBar`, `BottomPanel`)
   - Node multi-select list, command input, timeout input, Run button, results table

8. **Frontend — Sysinfo Tab Update** — Depends on Milestone 3 (cached sysinfo)
   - Verify `SystemInfo.tsx` handles the `stale` flag and shows cached data on load

---

## Implementation Notes

### Code Style Requirements
- Implementation code and comments must NOT contain plan-specific terminology such as "AC-", "Milestone", "Step", "Phase", or similar workflow markers
- These terms are for plan documentation only, not for the resulting codebase
- Use descriptive, domain-appropriate naming in code instead

--- Original Design Draft Start ---

# make a plan to add below features:
  - 1,node ssh connect method soupport via socks5/http/https proxy or through a jump
  server, jump server also soupport via proxy too
  - 2.add a endpoint to get node system info,backend can get basic info by run some
  shell cmd on remote node server on first add the node then store basic info to
  local db,and update when user click on the sysinfo tab
  - 3.add a batch exec function,user can select nodes to run a cmd and get return
  within a timeout period
  - 4.soupport open a node ssh session in more than one tab,and each tab has its
  own session,and user can switch between tabs to manage different sessions
--- Original Design Draft End ---
