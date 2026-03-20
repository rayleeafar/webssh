'use client'

import { useState, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { apiGet, apiPost, setCsrfToken } from '@/lib/api'
import { QRCodeSVG } from 'qrcode.react'

type FlowState = 'idle' | 'setup' | 'disabling'

export default function SettingsPage() {
  const [user, setUser] = useState<{ id: number; username: string } | null>(null)
  const [totpEnabled, setTotpEnabled] = useState(false)
  const [flowState, setFlowState] = useState<FlowState>('idle')
  const [setupData, setSetupData] = useState<{ secret: string; otpauth_url: string } | null>(null)
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [loading, setLoading] = useState(false)
  const router = useRouter()

  useEffect(() => {
    const init = async () => {
      const meRes = await apiGet('/api/auth/me')
      if (meRes.status === 401) { router.push('/login'); return }
      const me = await meRes.json()
      if (me.csrf_token) setCsrfToken(me.csrf_token)
      setUser({ id: me.id, username: me.username })

      const statusRes = await apiGet('/api/auth/2fa/status')
      if (statusRes.ok) {
        const st = await statusRes.json()
        setTotpEnabled(st.enabled)
      }
    }
    init()
  }, [router])

  const handleStartSetup = async () => {
    setError('')
    setSuccess('')
    setLoading(true)
    try {
      const res = await apiGet('/api/auth/2fa/setup')
      if (!res.ok) throw new Error('Failed to generate 2FA setup')
      const data = await res.json()
      setSetupData(data)
      setFlowState('setup')
      setCode('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Setup failed')
    } finally {
      setLoading(false)
    }
  }

  const handleConfirmEnable = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await apiPost('/api/auth/2fa/enable', {
        secret: setupData?.secret,
        code,
      })
      if (!res.ok) throw new Error('Invalid code — try again')
      setTotpEnabled(true)
      setFlowState('idle')
      setSetupData(null)
      setCode('')
      setSuccess('Two-factor authentication has been enabled.')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Enable failed')
    } finally {
      setLoading(false)
    }
  }

  const handleStartDisable = () => {
    setFlowState('disabling')
    setError('')
    setSuccess('')
    setCode('')
  }

  const handleConfirmDisable = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await apiPost('/api/auth/2fa/disable', { code })
      if (!res.ok) throw new Error('Invalid code — try again')
      setTotpEnabled(false)
      setFlowState('idle')
      setCode('')
      setSuccess('Two-factor authentication has been disabled.')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Disable failed')
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

  const codeInputStyle: React.CSSProperties = {
    textAlign: 'center',
    letterSpacing: '0.4em',
    fontSize: 20,
  }

  return (
    <div
      style={{
        minHeight: '100vh',
        background: '#050508',
        fontFamily: "'Rajdhani', sans-serif",
        color: '#c0c8e0',
      }}
    >
      {/* Ambient glow */}
      <div style={{ position: 'fixed', top: '20%', left: '15%', width: 400, height: 400, borderRadius: '50%', background: 'radial-gradient(circle, rgba(0,255,255,0.03) 0%, transparent 70%)', pointerEvents: 'none' }} />

      {/* Header */}
      <header style={{ height: 48, display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '0 20px', background: 'rgba(8,8,16,0.98)', borderBottom: '1px solid rgba(0,255,255,0.15)', position: 'relative' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <span style={{ color: '#00ffff', fontSize: 20, textShadow: '0 0 10px rgba(0,255,255,0.9)', lineHeight: 1 }}>◈</span>
          <span style={{ fontFamily: "'Orbitron', sans-serif", fontSize: 14, fontWeight: 700, color: '#00ffff', letterSpacing: '0.2em', textShadow: '0 0 12px rgba(0,255,255,0.6)' }}>
            WEBSSH MANAGER
          </span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
          {user && (
            <span style={{ fontFamily: "'Orbitron', sans-serif", fontSize: 11, color: '#6070a0', letterSpacing: '0.15em' }}>
              {user.username.toUpperCase()}
            </span>
          )}
          <a
            href="/dashboard"
            style={{ fontFamily: "'Orbitron', sans-serif", fontSize: 10, letterSpacing: '0.2em', color: '#6070a0', textDecoration: 'none', padding: '4px 12px', border: '1px solid rgba(0,255,255,0.25)', transition: 'color 0.2s, border-color 0.2s' }}
            onMouseEnter={(e) => { e.currentTarget.style.color = '#00ffff'; e.currentTarget.style.borderColor = 'rgba(0,255,255,0.5)' }}
            onMouseLeave={(e) => { e.currentTarget.style.color = '#6070a0'; e.currentTarget.style.borderColor = 'rgba(0,255,255,0.25)' }}
          >
            ← DASHBOARD
          </a>
        </div>
        <div style={{ position: 'absolute', bottom: 0, left: 0, right: 0, height: 1, background: 'linear-gradient(90deg, transparent 0%, rgba(0,255,255,0.3) 30%, rgba(0,255,255,0.3) 70%, transparent 100%)' }} />
      </header>

      {/* Content */}
      <div style={{ maxWidth: 600, margin: '60px auto', padding: '0 20px' }}>
        <div style={{ fontFamily: "'Orbitron', sans-serif", fontSize: 18, fontWeight: 700, color: '#00ffff', letterSpacing: '0.15em', marginBottom: 8 }}>
          SECURITY SETTINGS
        </div>
        <div style={{ fontSize: 12, color: '#404060', letterSpacing: '0.05em', marginBottom: 40 }}>
          Manage two-factor authentication and account security
        </div>

        {/* 2FA section */}
        <div
          style={{ background: 'rgba(13,13,26,0.95)', border: '1px solid rgba(0,255,255,0.15)', padding: '32px' }}
          className="corner-tl corner-tr corner-bl corner-br"
        >
          {/* Section title + status */}
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 24 }}>
            <div>
              <div style={{ fontFamily: "'Orbitron', sans-serif", fontSize: 13, color: '#a0b0d0', letterSpacing: '0.15em', marginBottom: 4 }}>
                TWO-FACTOR AUTHENTICATION
              </div>
              <div style={{ fontSize: 12, color: '#404060' }}>
                Protect your account with a TOTP authenticator app
              </div>
            </div>
            <div style={{
              padding: '4px 12px',
              border: `1px solid ${totpEnabled ? 'rgba(0,255,128,0.5)' : 'rgba(96,112,160,0.3)'}`,
              color: totpEnabled ? '#00ff80' : '#6070a0',
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 10,
              letterSpacing: '0.2em',
              background: totpEnabled ? 'rgba(0,255,128,0.05)' : 'transparent',
            }}>
              {totpEnabled ? 'ENABLED' : 'DISABLED'}
            </div>
          </div>

          {/* Success / error banners */}
          {success && (
            <div style={{ marginBottom: 24, padding: '10px 14px', background: 'rgba(0,255,128,0.08)', border: '1px solid rgba(0,255,128,0.3)', color: '#00ff80', fontSize: 13 }}>
              {success}
            </div>
          )}
          {error && (
            <div style={{ marginBottom: 24, padding: '10px 14px', background: 'rgba(255,48,96,0.1)', border: '1px solid rgba(255,48,96,0.3)', color: '#ff3060', fontSize: 13 }}>
              {error}
            </div>
          )}

          {/* Idle state */}
          {flowState === 'idle' && (
            <div>
              {!totpEnabled ? (
                <button
                  onClick={handleStartSetup}
                  disabled={loading}
                  style={{ padding: '10px 24px', background: 'transparent', border: '1px solid rgba(0,255,255,0.6)', color: '#00ffff', fontFamily: "'Orbitron', sans-serif", fontSize: 11, letterSpacing: '0.2em', cursor: loading ? 'not-allowed' : 'pointer', opacity: loading ? 0.6 : 1, transition: 'all 0.2s' }}
                  onMouseEnter={(e) => { e.currentTarget.style.background = 'rgba(0,255,255,0.05)' }}
                  onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                >
                  {loading ? 'LOADING...' : 'ENABLE 2FA'}
                </button>
              ) : (
                <button
                  onClick={handleStartDisable}
                  style={{ padding: '10px 24px', background: 'transparent', border: '1px solid rgba(255,48,96,0.5)', color: '#ff3060', fontFamily: "'Orbitron', sans-serif", fontSize: 11, letterSpacing: '0.2em', cursor: 'pointer', transition: 'all 0.2s' }}
                  onMouseEnter={(e) => { e.currentTarget.style.background = 'rgba(255,48,96,0.05)' }}
                  onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                >
                  DISABLE 2FA
                </button>
              )}
            </div>
          )}

          {/* Setup flow */}
          {flowState === 'setup' && setupData && (
            <div>
              <div style={{ marginBottom: 24, fontSize: 13, color: '#8090b0', lineHeight: 1.7 }}>
                Scan the QR code with <strong style={{ color: '#a0b0d0' }}>Google Authenticator</strong>, <strong style={{ color: '#a0b0d0' }}>Authy</strong>, or any TOTP app, then enter the 6-digit code to confirm.
              </div>

              {/* QR Code */}
              <div style={{ display: 'flex', justifyContent: 'center', marginBottom: 24 }}>
                <div style={{ padding: 16, background: '#ffffff', border: '2px solid rgba(0,255,255,0.4)', display: 'inline-block' }}>
                  <QRCodeSVG value={setupData.otpauth_url} size={180} />
                </div>
              </div>

              {/* Manual entry secret */}
              <div style={{ marginBottom: 28 }}>
                <div style={{ fontSize: 11, color: '#6070a0', letterSpacing: '0.1em', marginBottom: 8, fontFamily: "'Orbitron', sans-serif" }}>
                  MANUAL ENTRY KEY
                </div>
                <div style={{ fontFamily: "'JetBrains Mono', 'Courier New', monospace", fontSize: 13, color: '#00ffff', background: 'rgba(0,255,255,0.04)', border: '1px solid rgba(0,255,255,0.15)', padding: '10px 14px', letterSpacing: '0.15em', wordBreak: 'break-all' }}>
                  {setupData.secret}
                </div>
              </div>

              <form onSubmit={handleConfirmEnable}>
                <div style={{ marginBottom: 24 }}>
                  <label style={labelStyle}>CONFIRM CODE</label>
                  <input
                    type="text"
                    inputMode="numeric"
                    pattern="[0-9]{6}"
                    maxLength={6}
                    required
                    value={code}
                    onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))}
                    className="neon-input"
                    autoComplete="one-time-code"
                    placeholder="000000"
                    style={codeInputStyle}
                  />
                </div>

                <div style={{ display: 'flex', gap: 12 }}>
                  <button
                    type="submit"
                    disabled={loading}
                    style={{ flex: 1, padding: '10px', background: 'transparent', border: '1px solid rgba(0,255,255,0.6)', color: '#00ffff', fontFamily: "'Orbitron', sans-serif", fontSize: 11, letterSpacing: '0.2em', cursor: loading ? 'not-allowed' : 'pointer', opacity: loading ? 0.6 : 1, transition: 'all 0.2s' }}
                    onMouseEnter={(e) => { if (!loading) e.currentTarget.style.background = 'rgba(0,255,255,0.05)' }}
                    onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                  >
                    {loading ? 'VERIFYING...' : 'CONFIRM & ENABLE'}
                  </button>
                  <button
                    type="button"
                    onClick={() => { setFlowState('idle'); setError(''); setCode('') }}
                    style={{ padding: '10px 20px', background: 'transparent', border: '1px solid rgba(96,112,160,0.3)', color: '#6070a0', fontFamily: "'Orbitron', sans-serif", fontSize: 11, letterSpacing: '0.2em', cursor: 'pointer', transition: 'all 0.2s' }}
                    onMouseEnter={(e) => { e.currentTarget.style.color = '#a0b0d0'; e.currentTarget.style.borderColor = 'rgba(96,112,160,0.6)' }}
                    onMouseLeave={(e) => { e.currentTarget.style.color = '#6070a0'; e.currentTarget.style.borderColor = 'rgba(96,112,160,0.3)' }}
                  >
                    CANCEL
                  </button>
                </div>
              </form>
            </div>
          )}

          {/* Disable flow */}
          {flowState === 'disabling' && (
            <div>
              <div style={{ marginBottom: 24, fontSize: 13, color: '#8090b0', lineHeight: 1.7 }}>
                Enter your current authenticator code to disable two-factor authentication.
              </div>

              <form onSubmit={handleConfirmDisable}>
                <div style={{ marginBottom: 24 }}>
                  <label style={labelStyle}>AUTHENTICATOR CODE</label>
                  <input
                    type="text"
                    inputMode="numeric"
                    pattern="[0-9]{6}"
                    maxLength={6}
                    required
                    value={code}
                    onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))}
                    className="neon-input"
                    autoComplete="one-time-code"
                    placeholder="000000"
                    style={codeInputStyle}
                  />
                </div>

                <div style={{ display: 'flex', gap: 12 }}>
                  <button
                    type="submit"
                    disabled={loading}
                    style={{ flex: 1, padding: '10px', background: 'transparent', border: '1px solid rgba(255,48,96,0.6)', color: '#ff3060', fontFamily: "'Orbitron', sans-serif", fontSize: 11, letterSpacing: '0.2em', cursor: loading ? 'not-allowed' : 'pointer', opacity: loading ? 0.6 : 1, transition: 'all 0.2s' }}
                    onMouseEnter={(e) => { if (!loading) e.currentTarget.style.background = 'rgba(255,48,96,0.05)' }}
                    onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                  >
                    {loading ? 'VERIFYING...' : 'CONFIRM DISABLE'}
                  </button>
                  <button
                    type="button"
                    onClick={() => { setFlowState('idle'); setError(''); setCode('') }}
                    style={{ padding: '10px 20px', background: 'transparent', border: '1px solid rgba(96,112,160,0.3)', color: '#6070a0', fontFamily: "'Orbitron', sans-serif", fontSize: 11, letterSpacing: '0.2em', cursor: 'pointer', transition: 'all 0.2s' }}
                    onMouseEnter={(e) => { e.currentTarget.style.color = '#a0b0d0'; e.currentTarget.style.borderColor = 'rgba(96,112,160,0.6)' }}
                    onMouseLeave={(e) => { e.currentTarget.style.color = '#6070a0'; e.currentTarget.style.borderColor = 'rgba(96,112,160,0.3)' }}
                  >
                    CANCEL
                  </button>
                </div>
              </form>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
