'use client'

import { apiPost } from '@/lib/api'
import { useRouter } from 'next/navigation'

interface HeaderProps {
  user: { username: string } | null
  onLogout?: () => void
}

export default function Header({ user, onLogout }: HeaderProps) {
  const router = useRouter()

  const handleLogout = async () => {
    try {
      await apiPost('/api/auth/logout')
    } catch {
      // ignore
    }
    if (onLogout) onLogout()
    router.push('/login')
  }

  return (
    <header
      style={{
        height: 48,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '0 20px',
        background: 'rgba(8,8,16,0.98)',
        borderBottom: '1px solid rgba(0,255,255,0.15)',
        flexShrink: 0,
        zIndex: 50,
        position: 'relative',
      }}
    >
      {/* Left: Logo */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <span
          style={{
            color: '#00ffff',
            fontSize: 20,
            textShadow: '0 0 10px rgba(0,255,255,0.9)',
            lineHeight: 1,
          }}
        >
          ◈
        </span>
        <span
          style={{
            fontFamily: "'Orbitron', sans-serif",
            fontSize: 14,
            fontWeight: 700,
            color: '#00ffff',
            letterSpacing: '0.2em',
            textShadow: '0 0 12px rgba(0,255,255,0.6)',
          }}
        >
          WEBSSH MANAGER
        </span>
      </div>

      {/* Right: User info + logout */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
        {user && (
          <span
            style={{
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 11,
              color: '#6070a0',
              letterSpacing: '0.15em',
            }}
          >
            {user.username.toUpperCase()}
          </span>
        )}
        <button
          onClick={handleLogout}
          style={{
            background: 'transparent',
            border: '1px solid rgba(0,255,255,0.25)',
            color: '#6070a0',
            fontFamily: "'Orbitron', sans-serif",
            fontSize: 10,
            letterSpacing: '0.2em',
            padding: '4px 12px',
            cursor: 'pointer',
            transition: 'color 0.2s, border-color 0.2s, box-shadow 0.2s',
          }}
          onMouseEnter={(e) => {
            const t = e.currentTarget
            t.style.color = '#ff3060'
            t.style.borderColor = 'rgba(255,48,96,0.5)'
            t.style.boxShadow = '0 0 8px rgba(255,48,96,0.25)'
          }}
          onMouseLeave={(e) => {
            const t = e.currentTarget
            t.style.color = '#6070a0'
            t.style.borderColor = 'rgba(0,255,255,0.25)'
            t.style.boxShadow = 'none'
          }}
        >
          LOGOUT
        </button>
      </div>

      {/* Bottom glow line */}
      <div
        style={{
          position: 'absolute',
          bottom: 0,
          left: 0,
          right: 0,
          height: 1,
          background: 'linear-gradient(90deg, transparent 0%, rgba(0,255,255,0.3) 30%, rgba(0,255,255,0.3) 70%, transparent 100%)',
        }}
      />
    </header>
  )
}
