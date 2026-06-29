'use client'

import { useEffect, useRef, useState } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { apiGet, getWebSocketBase, setCsrfToken, getWSTicket } from '@/lib/api'


export default function TerminalClient() {
  const searchParams = useSearchParams()
  const nodeId = (searchParams.get('nodeId') || '') as string
  const router = useRouter()
  const terminalRef = useRef<HTMLDivElement>(null)
  const [error, setError] = useState('')
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    // cancelled flag prevents stale async continuations after cleanup
    // (React Strict Mode mounts→unmounts→mounts in dev)
    let cancelled = false
    let ws: WebSocket | null = null
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    let term: any = null
    let resizeObserver: ResizeObserver | null = null

    const init = async () => {
      // Import xterm lazily inside useEffect so it is never evaluated server-side.
      // xterm accesses browser-only globals (self, window) at module load time.
      const { Terminal } = await import('xterm')
      const { FitAddon } = await import('@xterm/addon-fit')

      const meRes = await apiGet('/api/auth/me')
      if (cancelled) return
      if (meRes.status === 401) {
        router.push('/login')
        return
      }
      if (meRes.ok) {
        const me = await meRes.json()
        if (me.csrf_token) setCsrfToken(me.csrf_token)
      }

      // nodeId is already defined above
      term = new Terminal({
        cursorBlink: true,
        fontSize: 14,
        fontFamily: "'MesloLGS NF', 'Meslo LGS NF', 'MesloLGS Nerd Font', 'JetBrainsMono Nerd Font', 'JetBrains Mono Nerd Font', 'FiraCode Nerd Font', 'Fira Code Nerd Font', 'Hack Nerd Font', 'Symbols Nerd Font Mono', 'JetBrains Mono', 'Menlo', 'Monaco', 'Courier New', monospace",
        theme: { background: '#1e1e1e', foreground: '#d4d4d4' },
      })

      const fitAddon = new FitAddon()
      term.loadAddon(fitAddon)

      if (terminalRef.current) {
        term.open(terminalRef.current)
        fitAddon.fit()
      }

      const ticket = await getWSTicket()
      const wsBase = getWebSocketBase()
      ws = new WebSocket(`${wsBase}/ws/terminal?nodeId=${nodeId}&ticket=${encodeURIComponent(ticket)}`)
      // Receive binary frames as ArrayBuffer so we can write synchronously
      // without an extra async Blob→ArrayBuffer conversion that can reorder writes.
      ws.binaryType = 'arraybuffer'

      ws.onopen = () => {
        if (cancelled) { ws?.close(); return }
        setConnected(true)
        setError('')

        // Wrap fit() in rAF to break the synchronous ResizeObserver→fit()→resize→
        // ResizeObserver loop that causes a rapid stream of resize WebSocket frames.
        resizeObserver = new ResizeObserver(() => {
          requestAnimationFrame(() => {
            if (!fitAddon || cancelled) return
            fitAddon.fit()
            const dims = fitAddon.proposeDimensions()
            if (dims && ws && ws.readyState === WebSocket.OPEN) {
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
        if (terminalRef.current) resizeObserver.observe(terminalRef.current)

        term!.onData((data: string) => {
          if (ws && ws.readyState === WebSocket.OPEN) ws.send(data)
        })
      }

      ws.onmessage = (event) => {
        if (event.data instanceof ArrayBuffer) {
          term?.write(new Uint8Array(event.data))
        } else if (typeof event.data === 'string') {
          term?.write(event.data)
        }
      }

      ws.onerror = () => {
        setError('WebSocket connection error')
        setConnected(false)
      }

      ws.onclose = () => {
        setConnected(false)
        term?.write('\r\n\r\nConnection closed.\r\n')
      }
    }

    init().catch(() => { if (!cancelled) router.push('/login') })

    return () => {
      cancelled = true
      resizeObserver?.disconnect()
      ws?.close()
      term?.dispose()
    }
  }, [nodeId, router])

  return (
    <div className="h-screen flex flex-col bg-gray-900">
      <div className="bg-gray-800 px-4 py-2 flex justify-between items-center">
        <div className="flex items-center gap-4">
          <a href="/nodes" className="text-white hover:text-gray-300">
            ← Back to Nodes
          </a>
          <div className="flex items-center gap-2">
            <div className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`} />
            <span className="text-white text-sm">
              {connected ? 'Connected' : 'Disconnected'}
            </span>
          </div>
        </div>
      </div>
      {error && (
        <div className="bg-red-600 text-white px-4 py-2">{error}</div>
      )}
      <div ref={terminalRef} className="flex-1 overflow-hidden" />
    </div>
  )
}
