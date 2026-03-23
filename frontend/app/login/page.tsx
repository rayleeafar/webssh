'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { apiPost, setCsrfToken } from '@/lib/api'

export default function LoginPage() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [step, setStep] = useState<'credentials' | '2fa'>('credentials')
  const [tempToken, setTempToken] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const router = useRouter()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
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
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    } finally {
      setLoading(false)
    }
  }

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

  const labelStyle: React.CSSProperties = {
    display: 'block',
    fontSize: 11,
    letterSpacing: '0.2em',
    color: '#6070a0',
    marginBottom: 8,
    fontFamily: "'Orbitron', sans-serif",
  }

  const errorBox = error ? (
    <div
      style={{
        marginBottom: 20,
        padding: '10px 14px',
        background: 'rgba(255,48,96,0.1)',
        border: '1px solid rgba(255,48,96,0.3)',
        color: '#ff3060',
        fontSize: 13,
        letterSpacing: '0.02em',
      }}
    >
      {error}
    </div>
  ) : null

  const submitButtonStyle: React.CSSProperties = {
    width: '100%',
    padding: '12px',
    background: 'transparent',
    border: '1px solid rgba(0,255,255,0.6)',
    color: '#00ffff',
    fontFamily: "'Orbitron', sans-serif",
    fontSize: 12,
    letterSpacing: '0.25em',
    cursor: loading ? 'not-allowed' : 'pointer',
    opacity: loading ? 0.6 : 1,
    boxShadow: '0 0 12px rgba(0,255,255,0.2), inset 0 0 12px rgba(0,255,255,0.03)',
    transition: 'box-shadow 0.2s, background 0.2s',
  }

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: '#050508',
        fontFamily: "'Rajdhani', sans-serif",
      }}
    >
      {/* Ambient glow blobs */}
      <div
        style={{
          position: 'fixed',
          top: '20%',
          left: '15%',
          width: 400,
          height: 400,
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(0,255,255,0.04) 0%, transparent 70%)',
          pointerEvents: 'none',
        }}
      />
      <div
        style={{
          position: 'fixed',
          bottom: '20%',
          right: '15%',
          width: 300,
          height: 300,
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(255,0,255,0.04) 0%, transparent 70%)',
          pointerEvents: 'none',
        }}
      />

      <div
        style={{
          position: 'relative',
          width: '100%',
          maxWidth: 400,
          padding: '48px 40px',
          background: 'rgba(13,13,26,0.95)',
          border: '1px solid rgba(0,255,255,0.2)',
          boxShadow: '0 0 40px rgba(0,255,255,0.08), inset 0 0 60px rgba(0,0,0,0.3)',
        }}
        className="corner-tl corner-tr corner-bl corner-br"
      >
        {/* Header */}
        <div style={{ textAlign: 'center', marginBottom: 40 }}>
          <div
            style={{
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 28,
              fontWeight: 900,
              color: '#00ffff',
              letterSpacing: '0.15em',
              textShadow: '0 0 20px rgba(0,255,255,0.8), 0 0 40px rgba(0,255,255,0.3)',
              marginBottom: 8,
            }}
          >
            WEBSSH
          </div>
          <div
            style={{
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 11,
              color: '#6070a0',
              letterSpacing: '0.3em',
            }}
          >
            {step === 'credentials' ? 'SECURE TERMINAL ACCESS' : 'TWO-FACTOR AUTHENTICATION'}
          </div>
        </div>

        {step === 'credentials' ? (
          <form onSubmit={handleSubmit}>
            {errorBox}

            <div style={{ marginBottom: 28 }}>
              <label style={labelStyle}>USERNAME</label>
              <input
                type="text"
                required
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="neon-input"
                autoComplete="username"
              />
            </div>

            <div style={{ marginBottom: 36 }}>
              <label style={labelStyle}>PASSWORD</label>
              <input
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="neon-input"
                autoComplete="current-password"
              />
            </div>

            <button
              type="submit"
              disabled={loading}
              style={submitButtonStyle}
              onMouseEnter={(e) => {
                if (!loading) {
                  const t = e.currentTarget
                  t.style.boxShadow = '0 0 20px rgba(0,255,255,0.5), inset 0 0 20px rgba(0,255,255,0.08)'
                  t.style.background = 'rgba(0,255,255,0.05)'
                }
              }}
              onMouseLeave={(e) => {
                const t = e.currentTarget
                t.style.boxShadow = '0 0 12px rgba(0,255,255,0.2), inset 0 0 12px rgba(0,255,255,0.03)'
                t.style.background = 'transparent'
              }}
            >
              {loading ? 'AUTHENTICATING...' : 'INITIATE SESSION'}
            </button>

            <div style={{ textAlign: 'center', marginTop: 24 }}>
              <Link
                href="/register"
                style={{
                  fontSize: 12,
                  color: '#6070a0',
                  letterSpacing: '0.1em',
                  textDecoration: 'none',
                  transition: 'color 0.2s',
                }}
                onMouseEnter={(e) => { (e.currentTarget as HTMLAnchorElement).style.color = '#00ffff' }}
                onMouseLeave={(e) => { (e.currentTarget as HTMLAnchorElement).style.color = '#6070a0' }}
              >
                No account? <span style={{ color: '#00ffff' }}>REGISTER</span>
              </Link>
            </div>
          </form>
        ) : (
          <form onSubmit={handleVerify}>
            <div style={{ textAlign: 'center', marginBottom: 28 }}>
              <div style={{ fontSize: 12, color: '#6070a0', letterSpacing: '0.05em', lineHeight: 1.6 }}>
                Enter the 6-digit code from your authenticator app
              </div>
            </div>

            {errorBox}

            <div style={{ marginBottom: 36 }}>
              <label style={labelStyle}>AUTHENTICATOR CODE</label>
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
                style={{ textAlign: 'center', letterSpacing: '0.4em', fontSize: 20 }}
              />
            </div>

            <button
              type="submit"
              disabled={loading}
              style={submitButtonStyle}
              onMouseEnter={(e) => {
                if (!loading) {
                  const t = e.currentTarget
                  t.style.boxShadow = '0 0 20px rgba(0,255,255,0.5), inset 0 0 20px rgba(0,255,255,0.08)'
                  t.style.background = 'rgba(0,255,255,0.05)'
                }
              }}
              onMouseLeave={(e) => {
                const t = e.currentTarget
                t.style.boxShadow = '0 0 12px rgba(0,255,255,0.2), inset 0 0 12px rgba(0,255,255,0.03)'
                t.style.background = 'transparent'
              }}
            >
              {loading ? 'VERIFYING...' : 'VERIFY CODE'}
            </button>

            <div style={{ textAlign: 'center', marginTop: 16 }}>
              <button
                type="button"
                onClick={() => { setStep('credentials'); setError(''); setTotpCode('') }}
                style={{
                  fontSize: 11,
                  color: '#6070a0',
                  background: 'none',
                  border: 'none',
                  cursor: 'pointer',
                  letterSpacing: '0.1em',
                  fontFamily: "'Rajdhani', sans-serif",
                }}
                onMouseEnter={(e) => { e.currentTarget.style.color = '#00ffff' }}
                onMouseLeave={(e) => { e.currentTarget.style.color = '#6070a0' }}
              >
                ← Back to login
              </button>
            </div>
          </form>
        )}

        {/* Decorative bottom bar */}
        <div
          style={{
            position: 'absolute',
            bottom: 0,
            left: '10%',
            width: '80%',
            height: 1,
            background: 'linear-gradient(90deg, transparent, rgba(0,255,255,0.4), transparent)',
          }}
        />
      </div>
    </div>
  )
}
