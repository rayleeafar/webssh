// API utility — authentication via HTTP-only session cookie, CSRF token in memory

const API_BASE = process.env.NEXT_PUBLIC_API_URL || ''

let csrfToken: string | null = null

export function setCsrfToken(token: string): void {
  csrfToken = token
}

export function invalidateCSRFToken(): void {
  csrfToken = null
}

/**
 * Fetch the CSRF token bound to the current session.
 * Uses /api/auth/me so the token is always session-specific.
 */
export async function fetchCSRFToken(): Promise<string> {
  if (csrfToken) return csrfToken

  const res = await fetch(`${API_BASE}/api/auth/me`, {
    credentials: 'include',
  })

  if (!res.ok) {
    throw new Error('Not authenticated')
  }

  const data = await res.json()
  csrfToken = data.csrf_token
  return data.csrf_token
}

export async function apiRequest(
  url: string,
  options: RequestInit = {}
): Promise<Response> {
  const headers: Record<string, string> = {}

  // Don't set Content-Type for FormData — browser sets it with boundary
  if (!(options.body instanceof FormData)) {
    headers['Content-Type'] = 'application/json'
  }

  if (options.headers) {
    const existingHeaders = new Headers(options.headers)
    existingHeaders.forEach((value, key) => {
      headers[key] = value
    })
  }

  // Add CSRF token for state-changing methods, but not for the auth
  // endpoints that establish a session (no cookie yet → /api/auth/me 401).
  const skipCsrf = ['/api/auth/login', '/api/auth/register', '/api/auth/logout', '/api/auth/2fa/verify']
  const needsCsrf =
    options.method &&
    ['POST', 'PUT', 'DELETE', 'PATCH'].includes(options.method) &&
    !skipCsrf.some((p) => url.startsWith(p))
  if (needsCsrf) {
    const csrf = await fetchCSRFToken()
    headers['X-CSRF-Token'] = csrf
  }

  return fetch(`${API_BASE}${url}`, {
    ...options,
    headers,
    credentials: 'include',
  })
}

export async function apiGet(url: string): Promise<Response> {
  return apiRequest(url, { method: 'GET' })
}

export async function apiPost(url: string, body?: unknown): Promise<Response> {
  return apiRequest(url, {
    method: 'POST',
    body: body ? JSON.stringify(body) : undefined,
  })
}

export async function apiPut(url: string, body?: unknown): Promise<Response> {
  return apiRequest(url, {
    method: 'PUT',
    body: body ? JSON.stringify(body) : undefined,
  })
}

export async function apiDelete(url: string): Promise<Response> {
  return apiRequest(url, { method: 'DELETE' })
}

/**
 * Request a short-lived (60 s) WebSocket authentication ticket.
 * Use this instead of the session cookie for WebSocket connections,
 * as cookie delivery during WS upgrades is unreliable in some browsers.
 */
export async function getWSTicket(): Promise<string> {
  const res = await apiGet('/api/auth/ws-ticket')
  if (!res.ok) throw new Error('Failed to get WebSocket ticket')
  const data = await res.json()
  return data.ticket as string
}

/**
 * Derive the WebSocket base URL.
 *
 * Priority order:
 * 1. NEXT_PUBLIC_WS_URL — explicit WebSocket base (e.g. ws://localhost:8080)
 * 2. NEXT_PUBLIC_API_URL — derive wss/ws from the API base URL
 * 3. Current page origin — only works if a reverse proxy handles /ws/* upgrades
 *
 * Next.js rewrites() do not proxy WebSocket upgrade connections, so in
 * development or plain Docker deployments set NEXT_PUBLIC_WS_URL to the
 * backend address (e.g. ws://localhost:8080) to bypass the Next.js proxy.
 */
export function getWebSocketBase(): string {
  if (typeof window !== 'undefined' && window.location.port !== '3000') {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${window.location.host}`
  }

  const wsUrl = process.env.NEXT_PUBLIC_WS_URL || ''
  if (wsUrl) return wsUrl

  const apiBase = process.env.NEXT_PUBLIC_API_URL || ''
  if (apiBase) {
    return apiBase.replace(/^https/, 'wss').replace(/^http/, 'ws')
  }
  return 'ws://localhost:8080'
}
