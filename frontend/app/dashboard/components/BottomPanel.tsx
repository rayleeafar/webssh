'use client'

import { useRef, useCallback } from 'react'
import SFTPBrowser from './SFTPBrowser'
import SystemInfo from './SystemInfo'
import BatchExec from './BatchExec'

interface Node {
  id: number
  name: string
  host: string
}

interface BottomPanelProps {
  tab: 'sftp' | 'sysinfo' | 'batch'
  onTabChange: (tab: 'sftp' | 'sysinfo' | 'batch') => void
  onClose: () => void
  height: number
  onHeightChange: (h: number) => void
  activeNodeId: number | null
  nodes: Node[]
  activeTabCwd?: string
}

export default function BottomPanel({
  tab,
  onClose,
  height,
  onHeightChange,
  activeNodeId,
  nodes,
  activeTabCwd,
}: BottomPanelProps) {
  const dragStartY = useRef<number>(0)
  const dragStartH = useRef<number>(0)

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      dragStartY.current = e.clientY
      dragStartH.current = height

      const onMouseMove = (ev: MouseEvent) => {
        const delta = dragStartY.current - ev.clientY
        const newH = Math.max(100, Math.min(600, dragStartH.current + delta))
        onHeightChange(newH)
      }

      const onMouseUp = () => {
        document.removeEventListener('mousemove', onMouseMove)
        document.removeEventListener('mouseup', onMouseUp)
      }

      document.addEventListener('mousemove', onMouseMove)
      document.addEventListener('mouseup', onMouseUp)
    },
    [height, onHeightChange]
  )

  return (
    <div
      style={{
        height,
        flexShrink: 0,
        display: 'flex',
        flexDirection: 'column',
        borderTop: '1px solid rgba(0,255,255,0.15)',
        background: '#080810',
        position: 'relative',
      }}
    >
      {/* Drag handle */}
      <div
        onMouseDown={handleMouseDown}
        style={{
          height: 6,
          cursor: 'row-resize',
          background: 'transparent',
          flexShrink: 0,
          position: 'relative',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
        onMouseEnter={(e) => {
          const handle = e.currentTarget.querySelector('div') as HTMLDivElement
          if (handle) {
            handle.style.background = 'rgba(0,255,255,0.5)'
            handle.style.boxShadow = '0 0 6px rgba(0,255,255,0.4)'
          }
        }}
        onMouseLeave={(e) => {
          const handle = e.currentTarget.querySelector('div') as HTMLDivElement
          if (handle) {
            handle.style.background = 'rgba(0,255,255,0.2)'
            handle.style.boxShadow = 'none'
          }
        }}
      >
        <div
          style={{
            width: 40,
            height: 3,
            background: 'rgba(0,255,255,0.2)',
            borderRadius: 2,
            transition: 'background 0.15s, box-shadow 0.15s',
            pointerEvents: 'none',
          }}
        />
      </div>

      {/* Content */}
      <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        {tab === 'batch' ? (
          <BatchExec nodes={nodes} />
        ) : !activeNodeId ? (
          <div
            style={{
              flex: 1,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#303060',
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 11,
              letterSpacing: '0.2em',
              textAlign: 'center',
              padding: 20,
            }}
          >
            SELECT A NODE AND OPEN A TERMINAL TO USE THIS PANEL
          </div>
        ) : tab === 'sftp' ? (
          <SFTPBrowser nodeId={activeNodeId} initialPath={activeTabCwd} />
        ) : (
          <SystemInfo nodeId={activeNodeId} />
        )}
      </div>

      {/* Close button removed — collapse is now handled by clicking the tab bar empty space */}
    </div>
  )
}
