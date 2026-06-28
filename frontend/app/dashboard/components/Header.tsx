'use client'

import { apiPost } from '@/lib/api'
import { useRouter } from 'next/navigation'

interface HeaderProps {
  user: { username: string } | null
  onLogout?: () => void
  isMobile?: boolean
  sidebarOpen?: boolean
  onToggleSidebar?: () => void
}

export default function Header({
  user,
  onLogout,
  isMobile = false,
  sidebarOpen = false,
  onToggleSidebar,
}: HeaderProps) {
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
        padding: isMobile ? '0 10px' : '0 20px',
        background: 'rgba(8,8,16,0.98)',
        borderBottom: '1px solid rgba(0,255,255,0.15)',
        flexShrink: 0,
        zIndex: 50,
        position: 'relative',
      }}
    >
      {/* Left: Logo */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        {isMobile && onToggleSidebar && (
          <button
            onClick={onToggleSidebar}
            style={{
              background: 'transparent',
              border: '1px solid rgba(0,255,255,0.3)',
              color: '#00ffff',
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 12,
              padding: '2px 8px',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              height: 24,
            }}
          >
            {sidebarOpen ? '✕' : '☰'}
          </button>
        )}
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
          {isMobile ? 'WEBSSH' : 'WEBSSH MANAGER'}
        </span>
      </div>

      {/* Right: User info + settings + logout */}
      <div style={{ display: 'flex', alignItems: 'center', gap: isMobile ? 8 : 16 }}>
        {user && !isMobile && (
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
        <a
          href="/settings"
          style={{
            fontFamily: "'Orbitron', sans-serif",
            fontSize: 10,
            letterSpacing: '0.2em',
            color: '#6070a0',
            textDecoration: 'none',
            padding: '4px 8px',
            border: '1px solid rgba(0,255,255,0.25)',
            transition: 'color 0.2s, border-color 0.2s',
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.color = '#00ffff'
            e.currentTarget.style.borderColor = 'rgba(0,255,255,0.5)'
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.color = '#6070a0'
            e.currentTarget.style.borderColor = 'rgba(0,255,255,0.25)'
          }}
        >
          {isMobile ? 'SETUP' : 'SETTINGS'}
        </a>
        <button
          onClick={handleLogout}
          style={{
            background: 'transparent',
            border: '1px solid rgba(0,255,255,0.25)',
            color: '#6070a0',
            fontFamily: "'Orbitron', sans-serif",
            fontSize: 10,
            letterSpacing: '0.2em',
            padding: '4px 8px',
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
          {isMobile ? 'EXIT' : 'LOGOUT'}
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
