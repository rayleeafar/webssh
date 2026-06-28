import AsyncStorage from '@react-native-async-storage/async-storage';

let serverUrl: string | null = null;
let sessionToken: string | null = null;
let csrfToken: string | null = null;

// Initialize from storage
export async function initApiConfig(): Promise<{ serverUrl: string | null; token: string | null }> {
  try {
    serverUrl = await AsyncStorage.getItem('server_url');
    sessionToken = await AsyncStorage.getItem('session_token');
    csrfToken = await AsyncStorage.getItem('csrf_token');
  } catch {
    // ignore storage errors
  }
  return { serverUrl, token: sessionToken };
}

export async function setServerUrl(url: string): Promise<void> {
  serverUrl = url.replace(/\/$/, ''); // strip trailing slash
  await AsyncStorage.setItem('server_url', serverUrl);
}

export async function getServerUrl(): Promise<string | null> {
  if (!serverUrl) {
    serverUrl = await AsyncStorage.getItem('server_url');
  }
  return serverUrl;
}

export async function setSession(token: string, csrf: string): Promise<void> {
  sessionToken = token;
  csrfToken = csrf;
  await AsyncStorage.setItem('session_token', token);
  await AsyncStorage.setItem('csrf_token', csrf);
}

export async function clearSession(): Promise<void> {
  sessionToken = null;
  csrfToken = null;
  await AsyncStorage.removeItem('session_token');
  await AsyncStorage.removeItem('csrf_token');
}

export async function fetchCSRFToken(): Promise<string> {
  if (csrfToken) return csrfToken;

  const res = await fetch(`${serverUrl}/api/auth/me`, {
    headers: {
      'Authorization': `Bearer ${sessionToken}`,
    },
  });

  if (!res.ok) {
    throw new Error('Not authenticated');
  }

  const data = await res.json();
  csrfToken = data.csrf_token;
  if (csrfToken) {
    await AsyncStorage.setItem('csrf_token', csrfToken);
  }
  return csrfToken || '';
}

export async function apiRequest(url: string, options: RequestInit = {}): Promise<Response> {
  const sUrl = await getServerUrl();
  if (!sUrl) {
    throw new Error('Server URL not configured');
  }

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  if (sessionToken) {
    headers['Authorization'] = `Bearer ${sessionToken}`;
  }

  if (options.headers) {
    Object.assign(headers, options.headers);
  }

  // Add CSRF token for state-changing methods
  const skipCsrf = ['/api/auth/login', '/api/auth/register', '/api/auth/logout', '/api/auth/2fa/verify'];
  const needsCsrf =
    options.method &&
    ['POST', 'PUT', 'DELETE', 'PATCH'].includes(options.method) &&
    !skipCsrf.some((p) => url.startsWith(p));
    
  if (needsCsrf) {
    try {
      const csrf = await fetchCSRFToken();
      headers['X-CSRF-Token'] = csrf;
    } catch {
      // ignore or let it fail at server side
    }
  }

  return fetch(`${sUrl}${url}`, {
    ...options,
    headers,
  });
}

export async function apiGet(url: string): Promise<Response> {
  return apiRequest(url, { method: 'GET' });
}

export async function apiPost(url: string, body?: unknown): Promise<Response> {
  return apiRequest(url, {
    method: 'POST',
    body: body ? JSON.stringify(body) : undefined,
  });
}

export async function apiPut(url: string, body?: unknown): Promise<Response> {
  return apiRequest(url, {
    method: 'PUT',
    body: body ? JSON.stringify(body) : undefined,
  });
}

export async function apiDelete(url: string): Promise<Response> {
  return apiRequest(url, { method: 'DELETE' });
}

export async function getWSTicket(): Promise<string> {
  const res = await apiGet('/api/auth/ws-ticket');
  if (!res.ok) throw new Error('Failed to get WebSocket ticket');
  const data = await res.json();
  return data.ticket as string;
}

export async function getWebSocketBase(): Promise<string> {
  const sUrl = await getServerUrl();
  if (!sUrl) return 'ws://localhost:8080';
  return sUrl.replace(/^https/, 'wss').replace(/^http/, 'ws');
}
