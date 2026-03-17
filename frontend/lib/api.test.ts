import { getWebSocketBase } from './api'

// Store original env so we can restore it
const originalEnv = process.env

afterEach(() => {
  process.env = { ...originalEnv }
  // Reset window.location between tests
  delete (global as Record<string, unknown>).window
})

describe('getWebSocketBase', () => {
  it('upgrades https:// NEXT_PUBLIC_API_URL to wss://', () => {
    process.env.NEXT_PUBLIC_API_URL = 'https://example.com:8443'
    expect(getWebSocketBase()).toBe('wss://example.com:8443')
  })

  it('upgrades http:// NEXT_PUBLIC_API_URL to ws://', () => {
    process.env.NEXT_PUBLIC_API_URL = 'http://example.com:8080'
    expect(getWebSocketBase()).toBe('ws://example.com:8080')
  })

  it('derives wss:// from window.location when page is https: and no env var', () => {
    delete process.env.NEXT_PUBLIC_API_URL
    ;(global as Record<string, unknown>).window = {
      location: { protocol: 'https:', host: 'myapp.example.com' },
    }
    expect(getWebSocketBase()).toBe('wss://myapp.example.com')
  })

  it('derives ws:// from window.location when page is http: and no env var', () => {
    delete process.env.NEXT_PUBLIC_API_URL
    ;(global as Record<string, unknown>).window = {
      location: { protocol: 'http:', host: 'localhost:3000' },
    }
    expect(getWebSocketBase()).toBe('ws://localhost:3000')
  })

  it('falls back to ws://localhost:8080 when window is undefined and no env var', () => {
    delete process.env.NEXT_PUBLIC_API_URL
    // In Node.js test environment without jsdom window, typeof window === 'undefined'
    // We simulate SSR by deleting the global window
    const origWindow = global.window
    // @ts-expect-error intentional deletion to test SSR branch
    delete global.window
    try {
      expect(getWebSocketBase()).toBe('ws://localhost:8080')
    } finally {
      global.window = origWindow
    }
  })
})
