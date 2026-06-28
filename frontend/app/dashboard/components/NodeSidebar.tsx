'use client'

import { useState } from 'react'

interface Node {
  id: number
  name: string
  host: string
  port: number
  username: string
  created_at: string
}

interface NodeSidebarProps {
  nodes: Node[]
  activeNodeIds: Set<number>
  onNodeClick: (node: Node) => void
  onAddNode: () => void
  onEditNode: (node: Node) => void
  onDeleteNode: (id: number) => void
  isMobile?: boolean
  onCloseMobileSidebar?: () => void
}

export default function NodeSidebar({
  nodes,
  activeNodeIds,
  onNodeClick,
  onAddNode,
  onEditNode,
  onDeleteNode,
  isMobile = false,
  onCloseMobileSidebar,
}: NodeSidebarProps) {
  const [hoveredId, setHoveredId] = useState<number | null>(null)

  return (
    <div
      style={{
        width: isMobile ? '100%' : 280,
        position: isMobile ? 'absolute' : 'relative',
        left: 0,
        top: 0,
        bottom: 0,
        zIndex: 30,
        flexShrink: 0,
        background: '#0a0a14',
        borderRight: '1px solid rgba(0,255,255,0.12)',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}
    >
      {/* Header */}
      <div
        style={{
          padding: '14px 16px 10px',
          borderBottom: '1px solid rgba(0,255,255,0.08)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexShrink: 0,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {isMobile && onCloseMobileSidebar && (
            <button
              onClick={onCloseMobileSidebar}
              style={{
                background: 'transparent',
                border: 'none',
                color: '#6070a0',
                cursor: 'pointer',
                fontSize: 16,
                padding: '0 4px',
                display: 'flex',
                alignItems: 'center',
              }}
            >
              ←
            </button>
          )}
          <span
            style={{
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 10,
              letterSpacing: '0.3em',
              color: '#6070a0',
            }}
          >
            NODES
          </span>
        </div>
        <span
          style={{
            background: 'rgba(0,255,255,0.12)',
            border: '1px solid rgba(0,255,255,0.25)',
            color: '#00ffff',
            fontFamily: "'Orbitron', sans-serif",
            fontSize: 9,
            padding: '2px 8px',
            letterSpacing: '0.1em',
          }}
        >
          {nodes.length}
        </span>
      </div>

      {/* Node list */}
      <div style={{ flex: 1, overflowY: 'auto', padding: '8px 0' }}>
        {nodes.length === 0 && (
          <div
            style={{
              padding: '24px 16px',
              textAlign: 'center',
              color: '#303060',
              fontSize: 12,
              fontFamily: "'Rajdhani', sans-serif",
              letterSpacing: '0.05em',
            }}
          >
            No nodes configured
          </div>
        )}
        {nodes.map((node) => {
          const isActive = activeNodeIds.has(node.id)
          const isHovered = hoveredId === node.id

          return (
            <div
              key={node.id}
              onClick={() => onNodeClick(node)}
              onMouseEnter={() => setHoveredId(node.id)}
              onMouseLeave={() => setHoveredId(null)}
              style={{
                padding: '10px 16px 10px 14px',
                cursor: 'pointer',
                borderLeft: `2px solid ${isActive || isHovered ? '#00ffff' : 'transparent'}`,
                background: isActive
                  ? 'rgba(0,255,255,0.06)'
                  : isHovered
                  ? 'rgba(0,255,255,0.03)'
                  : 'transparent',
                boxShadow: isActive || isHovered ? 'inset 4px 0 8px rgba(0,255,255,0.05)' : 'none',
                transition: 'border-color 0.15s, background 0.15s',
                position: 'relative',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                gap: 8,
              }}
            >
              <div style={{ flex: 1, minWidth: 0 }}>
                {/* Status dot + name */}
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 3 }}>
                  <span
                    style={{
                      width: 6,
                      height: 6,
                      borderRadius: '50%',
                      background: isActive ? '#00ff88' : '#303060',
                      flexShrink: 0,
                      boxShadow: isActive ? '0 0 6px rgba(0,255,136,0.8)' : 'none',
                      transition: 'background 0.2s, box-shadow 0.2s',
                    }}
                  />
                  <span
                    style={{
                      fontFamily: "'Rajdhani', sans-serif",
                      fontWeight: 700,
                      fontSize: 14,
                      color: isActive ? '#c8d8f0' : '#8090b8',
                      letterSpacing: '0.03em',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      flex: 1,
                    }}
                  >
                    {node.name}
                  </span>
                </div>
                {/* Host:port */}
                <div
                  style={{
                    fontFamily: "'JetBrains Mono', monospace",
                    fontSize: 11,
                    color: '#404070',
                    paddingLeft: 14,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {node.username}@{node.host}:{node.port}
                </div>
              </div>

              {/* Edit/Delete — show on hover */}
              {isHovered && (
                <div
                  style={{ display: 'flex', gap: 4, flexShrink: 0 }}
                  onClick={(e) => e.stopPropagation()}
                >
                  <button
                    onClick={() => onEditNode(node)}
                    title="Edit node"
                    style={{
                      background: 'transparent',
                      border: '1px solid rgba(0,128,255,0.4)',
                      color: '#0080ff',
                      width: 24,
                      height: 24,
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      cursor: 'pointer',
                      fontSize: 12,
                      padding: 0,
                      transition: 'border-color 0.15s, box-shadow 0.15s',
                    }}
                    onMouseEnter={(e) => {
                      e.currentTarget.style.boxShadow = '0 0 8px rgba(0,128,255,0.4)'
                      e.currentTarget.style.borderColor = 'rgba(0,128,255,0.8)'
                    }}
                    onMouseLeave={(e) => {
                      e.currentTarget.style.boxShadow = 'none'
                      e.currentTarget.style.borderColor = 'rgba(0,128,255,0.4)'
                    }}
                  >
                    ✎
                  </button>
                  <button
                    onClick={() => onDeleteNode(node.id)}
                    title="Delete node"
                    style={{
                      background: 'transparent',
                      border: '1px solid rgba(255,48,96,0.4)',
                      color: '#ff3060',
                      width: 24,
                      height: 24,
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      cursor: 'pointer',
                      fontSize: 12,
                      padding: 0,
                      transition: 'border-color 0.15s, box-shadow 0.15s',
                    }}
                    onMouseEnter={(e) => {
                      e.currentTarget.style.boxShadow = '0 0 8px rgba(255,48,96,0.4)'
                      e.currentTarget.style.borderColor = 'rgba(255,48,96,0.8)'
                    }}
                    onMouseLeave={(e) => {
                      e.currentTarget.style.boxShadow = 'none'
                      e.currentTarget.style.borderColor = 'rgba(255,48,96,0.4)'
                    }}
                  >
                    ✕
                  </button>
                </div>
              )}
            </div>
          )
        })}
      </div>

      {/* Add Node button */}
      <div style={{ padding: '12px 16px', flexShrink: 0, borderTop: '1px solid rgba(0,255,255,0.08)' }}>
        <button
          onClick={onAddNode}
          style={{
            width: '100%',
            padding: '10px',
            background: 'transparent',
            border: '1px solid rgba(0,255,255,0.3)',
            color: '#00ffff',
            fontFamily: "'Orbitron', sans-serif",
            fontSize: 10,
            letterSpacing: '0.2em',
            cursor: 'pointer',
            transition: 'border-color 0.2s, box-shadow 0.2s, background 0.2s',
          }}
          onMouseEnter={(e) => {
            const t = e.currentTarget
            t.style.borderColor = 'rgba(0,255,255,0.7)'
            t.style.boxShadow = '0 0 12px rgba(0,255,255,0.25)'
            t.style.background = 'rgba(0,255,255,0.04)'
          }}
          onMouseLeave={(e) => {
            const t = e.currentTarget
            t.style.borderColor = 'rgba(0,255,255,0.3)'
            t.style.boxShadow = 'none'
            t.style.background = 'transparent'
          }}
        >
          + ADD NODE
        </button>
      </div>
    </div>
  )
}
