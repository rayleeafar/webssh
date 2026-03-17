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

  // Add CSRF token for state-changing methods
  if (options.method && ['POST', 'PUT', 'DELETE', 'PATCH'].includes(options.method)) {
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
 * Derive the WebSocket base URL from the current page origin,
 * upgrading to wss:// when the page is served over HTTPS.
 */
export function getWebSocketBase(): string {
  const apiBase = process.env.NEXT_PUBLIC_API_URL || ''
  if (apiBase) {
    return apiBase.replace(/^https/, 'wss').replace(/^http/, 'ws')
  }
  if (typeof window === 'undefined') return 'ws://localhost:8080'
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${window.location.host}`
}
