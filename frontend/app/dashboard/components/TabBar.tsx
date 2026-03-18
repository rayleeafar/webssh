'use client'

import { useState } from 'react'

interface Tab {
  id: string
  nodeId: number
  nodeName: string
  host: string
}

interface TabBarProps {
  tabs: Tab[]
  activeTabId: string | null
  onSelect: (id: string) => void
  onClose: (id: string) => void
}

export default function TabBar({ tabs, activeTabId, onSelect, onClose }: TabBarProps) {
  const [hoveredTabId, setHoveredTabId] = useState<string | null>(null)

  return (
    <div
      style={{
        height: 40,
        background: '#080810',
        borderBottom: '1px solid rgba(0,255,255,0.1)',
        display: 'flex',
        alignItems: 'stretch',
        overflowX: 'auto',
        overflowY: 'hidden',
        flexShrink: 0,
      }}
    >
      {tabs.length === 0 ? (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            padding: '0 16px',
            color: '#303060',
            fontFamily: "'Orbitron', sans-serif",
            fontSize: 10,
            letterSpacing: '0.2em',
          }}
        >
          NO ACTIVE TERMINALS
        </div>
      ) : (
        tabs.map((tab) => {
          const isActive = tab.id === activeTabId
          const isHovered = hoveredTabId === tab.id

          return (
            <div
              key={tab.id}
              onClick={() => onSelect(tab.id)}
              onMouseEnter={() => setHoveredTabId(tab.id)}
              onMouseLeave={() => setHoveredTabId(null)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                padding: '0 14px 0 12px',
                cursor: 'pointer',
                borderRight: '1px solid rgba(0,255,255,0.08)',
                borderBottom: isActive
                  ? '2px solid #00ffff'
                  : '2px solid transparent',
                background: isActive
                  ? 'rgba(0,255,255,0.06)'
                  : isHovered
                  ? 'rgba(0,255,255,0.02)'
                  : 'transparent',
                flexShrink: 0,
                transition: 'background 0.15s, border-color 0.15s',
                maxWidth: 200,
                minWidth: 0,
                position: 'relative',
              }}
            >
              {/* Status dot */}
              <span
                style={{
                  width: 6,
                  height: 6,
                  borderRadius: '50%',
                  background: '#00ff88',
                  boxShadow: '0 0 4px rgba(0,255,136,0.8)',
                  flexShrink: 0,
                }}
              />
              <span
                style={{
                  fontFamily: "'Rajdhani', sans-serif",
                  fontSize: 13,
                  fontWeight: 600,
                  color: isActive ? '#c8d8f0' : '#6070a0',
                  letterSpacing: '0.03em',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  flex: 1,
                  minWidth: 0,
                }}
              >
                {tab.nodeName}
              </span>
              <button
                onClick={(e) => {
                  e.stopPropagation()
                  onClose(tab.id)
                }}
                style={{
                  background: 'transparent',
                  border: 'none',
                  color: isHovered ? '#ff3060' : '#404070',
                  cursor: 'pointer',
                  fontSize: 14,
                  lineHeight: 1,
                  padding: '0 2px',
                  flexShrink: 0,
                  transition: 'color 0.15s',
                }}
                onMouseEnter={(e) => { e.currentTarget.style.color = '#ff3060' }}
                onMouseLeave={(e) => { e.currentTarget.style.color = isHovered ? '#ff3060' : '#404070' }}
              >
                ×
              </button>
            </div>
          )
        })
      )}
    </div>
  )
}
