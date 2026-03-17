'use client'

import { useEffect, useRef, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import { Terminal } from 'xterm'
import { FitAddon } from '@xterm/addon-fit'
import 'xterm/css/xterm.css'

export default function TerminalPage() {
  const params = useParams()
  const router = useRouter()
  const terminalRef = useRef<HTMLDivElement>(null)
  const [error, setError] = useState('')
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    const token = localStorage.getItem('session_token')
    if (!token) {
      router.push('/login')
      return
    }

    const nodeId = params.nodeId as string
    const term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
      },
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)

    if (terminalRef.current) {
      term.open(terminalRef.current)
      fitAddon.fit()
    }

    const ws = new WebSocket(`ws://localhost:8080/ws/terminal?nodeId=${nodeId}`)

    ws.onopen = () => {
      setConnected(true)
      setError('')

      const resizeObserver = new ResizeObserver(() => {
        fitAddon.fit()
        const dims = term.getDimensions()
        if (dims) {
          const msg = new Uint8Array(5)
          msg[0] = 0
          msg[1] = (dims.rows >> 8) & 0xff
          msg[2] = dims.rows & 0xff
          msg[3] = (dims.cols >> 8) & 0xff
          msg[4] = dims.cols & 0xff
          ws.send(msg)
        }
      })

      if (terminalRef.current) {
        resizeObserver.observe(terminalRef.current)
      }

      term.onData((data) => {
        ws.send(data)
      })
    }

    ws.onmessage = (event) => {
      if (event.data instanceof Blob) {
        event.data.arrayBuffer().then((buffer) => {
          term.write(new Uint8Array(buffer))
        })
      } else if (typeof event.data === 'string') {
        term.write(event.data)
      }
    }

    ws.onerror = () => {
      setError('WebSocket connection error')
      setConnected(false)
    }

    ws.onclose = () => {
      setConnected(false)
      term.write('\r\n\r\nConnection closed.\r\n')
    }

    return () => {
      ws.close()
      term.dispose()
    }
  }, [params.nodeId, router])

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
        <div className="bg-red-600 text-white px-4 py-2">
          {error}
        </div>
      )}
      <div ref={terminalRef} className="flex-1" />
    </div>
  )
}
