'use client'

import { useEffect, useRef } from 'react'
import { Terminal } from 'xterm'
import { FitAddon } from '@xterm/addon-fit'
import 'xterm/css/xterm.css'
import { apiGet, getWebSocketBase, setCsrfToken } from '@/lib/api'

interface TerminalPaneProps {
  nodeId: number
  active: boolean
}

export default function TerminalPane({ nodeId, active }: TerminalPaneProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const roRef = useRef<ResizeObserver | null>(null)
  const initializedRef = useRef(false)

  // Initialize terminal once on mount
  useEffect(() => {
    if (initializedRef.current) return
    initializedRef.current = true

    let cancelled = false

    const init = async () => {
      // Bootstrap CSRF token
      try {
        const meRes = await apiGet('/api/auth/me')
        if (!cancelled && meRes.ok) {
          const me = await meRes.json()
          if (me.csrf_token) setCsrfToken(me.csrf_token)
        }
      } catch {
        // continue anyway
      }

      if (cancelled) return

      const term = new Terminal({
        cursorBlink: true,
        fontSize: 13,
        fontFamily: "'JetBrains Mono', 'Menlo', 'Monaco', 'Courier New', monospace",
        theme: {
          background: '#050508',
          foreground: '#c8d8f0',
          cursor: '#00ffff',
          cursorAccent: '#050508',
          selectionBackground: 'rgba(0,255,255,0.2)',
          black: '#050508',
          red: '#ff3060',
          green: '#00ff88',
          yellow: '#ffcc00',
          blue: '#0080ff',
          magenta: '#ff00ff',
          cyan: '#00ffff',
          white: '#c8d8f0',
          brightBlack: '#303060',
          brightRed: '#ff6080',
          brightGreen: '#40ffaa',
          brightYellow: '#ffdd44',
          brightBlue: '#4499ff',
          brightMagenta: '#ff44ff',
          brightCyan: '#44ffff',
          brightWhite: '#e8f0ff',
        },
        scrollback: 5000,
        allowTransparency: true,
      })

      const fitAddon = new FitAddon()
      term.loadAddon(fitAddon)
      termRef.current = term
      fitRef.current = fitAddon

      if (containerRef.current) {
        term.open(containerRef.current)
        requestAnimationFrame(() => {
          if (!cancelled && fitAddon) fitAddon.fit()
        })
      }

      const wsBase = getWebSocketBase()
      const ws = new WebSocket(`${wsBase}/ws/terminal?nodeId=${nodeId}`)
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      ws.onopen = () => {
        if (cancelled) { ws.close(); return }

        // Set up resize observer
        const ro = new ResizeObserver(() => {
          requestAnimationFrame(() => {
            if (cancelled || !fitAddon) return
            fitAddon.fit()
            const dims = fitAddon.proposeDimensions()
            if (dims && ws.readyState === WebSocket.OPEN) {
              const msg = new Uint8Array(5)
              msg[0] = 0
              msg[1] = (dims.rows >> 8) & 0xff
              msg[2] = dims.rows & 0xff
              msg[3] = (dims.cols >> 8) & 0xff
              msg[4] = dims.cols & 0xff
              ws.send(msg)
            }
          })
        })
        roRef.current = ro
        if (containerRef.current) ro.observe(containerRef.current)

        term.onData((data) => {
          if (ws.readyState === WebSocket.OPEN) ws.send(data)
        })
      }

      ws.onmessage = (event) => {
        if (event.data instanceof ArrayBuffer) {
          term.write(new Uint8Array(event.data))
        } else if (typeof event.data === 'string') {
          term.write(event.data)
        }
      }

      ws.onclose = () => {
        term.write('\r\n\r\n\x1b[31mConnection closed.\x1b[0m\r\n')
      }

      ws.onerror = () => {
        term.write('\r\n\x1b[31mWebSocket error.\x1b[0m\r\n')
      }
    }

    init().catch(console.error)

    return () => {
      cancelled = true
      roRef.current?.disconnect()
      wsRef.current?.close()
      termRef.current?.dispose()
      termRef.current = null
      fitRef.current = null
      wsRef.current = null
      roRef.current = null
      initializedRef.current = false
    }
  }, [nodeId]) // eslint-disable-line react-hooks/exhaustive-deps

  // When pane becomes active, trigger a fit
  useEffect(() => {
    if (active) {
      requestAnimationFrame(() => {
        fitRef.current?.fit()
        // Send resize to server
        const fit = fitRef.current
        const ws = wsRef.current
        if (fit && ws && ws.readyState === WebSocket.OPEN) {
          const dims = fit.proposeDimensions()
          if (dims) {
            const msg = new Uint8Array(5)
            msg[0] = 0
            msg[1] = (dims.rows >> 8) & 0xff
            msg[2] = dims.rows & 0xff
            msg[3] = (dims.cols >> 8) & 0xff
            msg[4] = dims.cols & 0xff
            ws.send(msg)
          }
        }
      })
    }
  }, [active])

  return (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        display: 'flex',
        flexDirection: 'column',
        background: '#050508',
        // visibility keeps terminal in layout flow so fitAddon always has
        // correct container dimensions; display:none zeroes them out and
        // causes xterm to render a 0×0 viewport on first open.
        visibility: active ? 'visible' : 'hidden',
        pointerEvents: active ? 'auto' : 'none',
      }}
      className="scanlines"
    >
      {/* Terminal container */}
      <div
        ref={containerRef}
        style={{
          flex: 1,
          overflow: 'hidden',
          padding: '4px',
        }}
      />
    </div>
  )
}
