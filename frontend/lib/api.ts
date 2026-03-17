// API utility with CSRF token handling

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

let csrfToken: string | null = null

export async function fetchCSRFToken(): Promise<string> {
  if (csrfToken) {
    return csrfToken
  }

  const res = await fetch(`${API_BASE}/api/csrf-token`, {
    credentials: 'include',
  })

  if (!res.ok) {
    throw new Error('Failed to fetch CSRF token')
  }

  const data = await res.json()
  csrfToken = data.token
  return data.token
}

export function getAuthToken(): string | null {
  if (typeof window === 'undefined') return null
  return localStorage.getItem('session_token')
}

export async function apiRequest(
  url: string,
  options: RequestInit = {}
): Promise<Response> {
  const token = getAuthToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }

  // Merge existing headers
  if (options.headers) {
    const existingHeaders = new Headers(options.headers)
    existingHeaders.forEach((value, key) => {
      headers[key] = value
    })
  }

  if (token) {
    headers['Authorization'] = `Bearer ${token}`
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

export async function apiPost(url: string, body?: any): Promise<Response> {
  return apiRequest(url, {
    method: 'POST',
    body: body ? JSON.stringify(body) : undefined,
  })
}

export async function apiPut(url: string, body?: any): Promise<Response> {
  return apiRequest(url, {
    method: 'PUT',
    body: body ? JSON.stringify(body) : undefined,
  })
}

export async function apiDelete(url: string): Promise<Response> {
  return apiRequest(url, { method: 'DELETE' })
}
