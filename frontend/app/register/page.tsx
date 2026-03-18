'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { apiPost } from '@/lib/api'

export default function RegisterPage() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const router = useRouter()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await apiPost('/api/auth/register', { username, password })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || 'Registration failed')
      }
      router.push('/login')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Registration failed')
    } finally {
      setLoading(false)
    }
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
          top: '25%',
          right: '10%',
          width: 360,
          height: 360,
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(255,0,255,0.04) 0%, transparent 70%)',
          pointerEvents: 'none',
        }}
      />
      <div
        style={{
          position: 'fixed',
          bottom: '15%',
          left: '10%',
          width: 280,
          height: 280,
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(0,128,255,0.04) 0%, transparent 70%)',
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
              fontSize: 22,
              fontWeight: 900,
              color: '#00ffff',
              letterSpacing: '0.12em',
              textShadow: '0 0 20px rgba(0,255,255,0.8), 0 0 40px rgba(0,255,255,0.3)',
              marginBottom: 8,
            }}
          >
            CREATE ACCOUNT
          </div>
          <div
            style={{
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 11,
              color: '#6070a0',
              letterSpacing: '0.3em',
            }}
          >
            NEW OPERATOR REGISTRATION
          </div>
        </div>

        <form onSubmit={handleSubmit}>
          {error && (
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
          )}

          <div style={{ marginBottom: 28 }}>
            <label
              style={{
                display: 'block',
                fontSize: 11,
                letterSpacing: '0.2em',
                color: '#6070a0',
                marginBottom: 8,
                fontFamily: "'Orbitron', sans-serif",
              }}
            >
              USERNAME
            </label>
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
            <label
              style={{
                display: 'block',
                fontSize: 11,
                letterSpacing: '0.2em',
                color: '#6070a0',
                marginBottom: 8,
                fontFamily: "'Orbitron', sans-serif",
              }}
            >
              PASSWORD
            </label>
            <input
              type="password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="neon-input"
              autoComplete="new-password"
            />
          </div>

          <button
            type="submit"
            disabled={loading}
            style={{
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
            }}
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
            {loading ? 'CREATING...' : 'REGISTER OPERATOR'}
          </button>

          <div style={{ textAlign: 'center', marginTop: 24 }}>
            <a
              href="/login"
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
              Already registered? <span style={{ color: '#00ffff' }}>SIGN IN</span>
            </a>
          </div>
        </form>

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
