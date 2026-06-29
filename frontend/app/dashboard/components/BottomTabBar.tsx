'use client'

interface BottomTabBarProps {
  activeTab: 'sftp' | 'sysinfo' | 'batch' | null
  onTabClick: (tab: 'sftp' | 'sysinfo' | 'batch') => void
  onCollapse: () => void
}

export default function BottomTabBar({ activeTab, onTabClick, onCollapse }: BottomTabBarProps) {
  const tabs: { id: 'sftp' | 'sysinfo' | 'batch'; label: string }[] = [
    { id: 'sftp', label: 'SFTP' },
    { id: 'sysinfo', label: 'SYSINFO' },
    { id: 'batch', label: 'BATCH' },
  ]

  return (
    <div
      onClick={(e) => {
        if (e.target === e.currentTarget && activeTab) {
          onCollapse()
        }
      }}
      style={{
        height: 32,
        background: '#080810',
        borderTop: '1px solid rgba(0,255,255,0.1)',
        display: 'flex',
        alignItems: 'stretch',
        flexShrink: 0,
      }}
    >
      {tabs.map((tab) => {
        const isActive = activeTab === tab.id
        return (
          <button
            key={tab.id}
            onClick={() => onTabClick(tab.id)}
            style={{
              background: isActive ? 'rgba(0,255,255,0.08)' : 'transparent',
              border: 'none',
              borderRight: '1px solid rgba(0,255,255,0.08)',
              borderBottom: isActive ? '2px solid #00ffff' : '2px solid transparent',
              color: isActive ? '#00ffff' : '#404070',
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 9,
              letterSpacing: '0.2em',
              padding: '0 20px',
              cursor: 'pointer',
              transition: 'color 0.15s, background 0.15s, border-color 0.15s',
              textShadow: isActive ? '0 0 8px rgba(0,255,255,0.5)' : 'none',
            }}
            onMouseEnter={(e) => {
              if (!isActive) {
                e.currentTarget.style.color = '#8090c0'
                e.currentTarget.style.background = 'rgba(0,255,255,0.02)'
              }
            }}
            onMouseLeave={(e) => {
              if (!isActive) {
                e.currentTarget.style.color = '#404070'
                e.currentTarget.style.background = 'transparent'
              }
            }}
          >
            {tab.label}
          </button>
        )
      })}
      {/* Spacer — clicking here collapses the active panel */}
      <div
        onClick={() => {
          if (activeTab) onCollapse()
        }}
        style={{
          flex: 1,
          cursor: activeTab ? 'pointer' : 'default',
        }}
      />
    </div>
  )
}
