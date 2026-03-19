'use client'

import { useState } from 'react'
import { apiPost } from '@/lib/api'

interface Node {
  id: number
  name: string
  host: string
}

interface BatchResult {
  nodeId: number
  host: string
  exitCode: number
  stdout: string
  stderr: string
  timedOut: boolean
  error?: string
}

interface Props {
  nodes: Node[]
}

export default function BatchExec({ nodes }: Props) {
  const [selectedNodes, setSelectedNodes] = useState<Set<number>>(new Set())
  const [command, setCommand] = useState('')
  const [timeoutSec, setTimeoutSec] = useState(60)
  const [results, setResults] = useState<BatchResult[]>([])
  const [running, setRunning] = useState(false)
  const [validationError, setValidationError] = useState('')

  const toggleNode = (id: number) => {
    setSelectedNodes(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const toggleAll = () => {
    if (selectedNodes.size === nodes.length) {
      setSelectedNodes(new Set())
    } else {
      setSelectedNodes(new Set(nodes.map(n => n.id)))
    }
  }

  const run = async () => {
    setValidationError('')
    if (selectedNodes.size === 0) {
      setValidationError('Select at least one node')
      return
    }
    if (!command.trim()) {
      setValidationError('Enter a command')
      return
    }
    setRunning(true)
    try {
      const res = await apiPost('/api/batch/exec', {
        nodeIds: Array.from(selectedNodes),
        command: command.trim(),
        timeout: timeoutSec,
      })
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      setResults(data)
    } catch (e) {
      setValidationError(e instanceof Error ? e.message : 'Request failed')
    } finally {
      setRunning(false)
    }
  }

  const labelStyle: React.CSSProperties = {
    fontFamily: "'Orbitron', sans-serif",
    fontSize: 9,
    letterSpacing: '0.2em',
    color: '#404070',
    marginBottom: 6,
    display: 'block',
  }

  const inputStyle: React.CSSProperties = {
    width: '100%',
    background: 'rgba(8,8,16,0.8)',
    border: 'none',
    borderBottom: '1px solid rgba(157,78,221,0.3)',
    color: '#e0e0e0',
    fontFamily: "'JetBrains Mono', monospace",
    fontSize: 12,
    padding: '7px 4px',
    outline: 'none',
    boxSizing: 'border-box',
  }

  return (
    <div style={{ display: 'flex', height: '100%', color: '#e0e0e0', fontSize: 13 }}>
      {/* Left: node selector */}
      <div
        style={{
          width: 220,
          flexShrink: 0,
          borderRight: '1px solid rgba(157,78,221,0.15)',
          display: 'flex',
          flexDirection: 'column',
          background: '#0d0d14',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            padding: '8px 12px',
            borderBottom: '1px solid rgba(157,78,221,0.15)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            flexShrink: 0,
          }}
        >
          <span
            style={{
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 9,
              letterSpacing: '0.2em',
              color: '#404070',
            }}
          >
            NODES
          </span>
          <button
            onClick={toggleAll}
            style={{
              background: 'transparent',
              border: '1px solid rgba(157,78,221,0.3)',
              color: '#9d4edd',
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 8,
              letterSpacing: '0.1em',
              padding: '2px 8px',
              cursor: 'pointer',
            }}
          >
            {selectedNodes.size === nodes.length && nodes.length > 0 ? 'NONE' : 'ALL'}
          </button>
        </div>
        <div style={{ flex: 1, overflowY: 'auto' }}>
          {nodes.length === 0 ? (
            <div
              style={{
                padding: '20px 12px',
                color: '#303060',
                fontFamily: "'Orbitron', sans-serif",
                fontSize: 9,
                letterSpacing: '0.15em',
                textAlign: 'center',
              }}
            >
              NO NODES
            </div>
          ) : (
            nodes.map(node => {
              const checked = selectedNodes.has(node.id)
              return (
                <div
                  key={node.id}
                  onClick={() => toggleNode(node.id)}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 8,
                    padding: '7px 12px',
                    cursor: 'pointer',
                    borderBottom: '1px solid rgba(255,255,255,0.03)',
                    background: checked ? 'rgba(157,78,221,0.08)' : 'transparent',
                    transition: 'background 0.15s',
                  }}
                >
                  <div
                    style={{
                      width: 12,
                      height: 12,
                      border: `1px solid ${checked ? '#9d4edd' : 'rgba(255,255,255,0.2)'}`,
                      background: checked ? '#9d4edd' : 'transparent',
                      flexShrink: 0,
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontSize: 9,
                      color: '#fff',
                      transition: 'background 0.15s, border-color 0.15s',
                    }}
                  >
                    {checked ? '✓' : ''}
                  </div>
                  <div style={{ minWidth: 0 }}>
                    <div
                      style={{
                        fontFamily: "'Rajdhani', sans-serif",
                        fontSize: 12,
                        fontWeight: 600,
                        color: checked ? '#e0e0e0' : '#6070a0',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {node.name}
                    </div>
                    <div
                      style={{
                        fontFamily: "'JetBrains Mono', monospace",
                        fontSize: 10,
                        color: '#303060',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {node.host}
                    </div>
                  </div>
                </div>
              )
            })
          )}
        </div>
      </div>

      {/* Right: command input + results */}
      <div
        style={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          background: '#0d0d14',
          minWidth: 0,
          overflow: 'hidden',
        }}
      >
        {/* Command area */}
        <div
          style={{
            padding: '10px 14px',
            borderBottom: '1px solid rgba(157,78,221,0.15)',
            background: '#1a1a2e',
            flexShrink: 0,
          }}
        >
          {validationError && (
            <div
              style={{
                marginBottom: 8,
                padding: '6px 10px',
                background: 'rgba(239,68,68,0.1)',
                border: '1px solid rgba(239,68,68,0.3)',
                color: '#ef4444',
                fontSize: 12,
                fontFamily: "'Rajdhani', sans-serif",
              }}
            >
              {validationError}
            </div>
          )}
          <div style={{ display: 'flex', gap: 10, alignItems: 'flex-end' }}>
            <div style={{ flex: 1 }}>
              <label style={labelStyle}>COMMAND</label>
              <input
                type="text"
                value={command}
                onChange={e => setCommand(e.target.value)}
                placeholder="uptime && df -h"
                onKeyDown={e => { if (e.key === 'Enter' && !running) run() }}
                style={inputStyle}
              />
            </div>
            <div style={{ width: 80 }}>
              <label style={labelStyle}>TIMEOUT (s)</label>
              <input
                type="number"
                value={timeoutSec}
                onChange={e => setTimeoutSec(parseInt(e.target.value) || 60)}
                min={1}
                max={300}
                style={inputStyle}
              />
            </div>
            <button
              onClick={run}
              disabled={running}
              style={{
                padding: '8px 18px',
                background: running ? 'rgba(157,78,221,0.1)' : 'transparent',
                border: '1px solid rgba(157,78,221,0.6)',
                color: running ? '#6030a0' : '#9d4edd',
                fontFamily: "'Orbitron', sans-serif",
                fontSize: 10,
                letterSpacing: '0.15em',
                cursor: running ? 'not-allowed' : 'pointer',
                flexShrink: 0,
                transition: 'background 0.15s, color 0.15s',
                alignSelf: 'flex-end',
                marginBottom: 0,
                height: 32,
              }}
            >
              {running ? 'RUNNING...' : 'RUN'}
            </button>
          </div>
        </div>

        {/* Results */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '10px 14px' }}>
          {results.length === 0 && !running && (
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                height: '100%',
                color: '#303060',
                fontFamily: "'Orbitron', sans-serif",
                fontSize: 10,
                letterSpacing: '0.2em',
              }}
            >
              RUN A COMMAND TO SEE RESULTS
            </div>
          )}
          {results.map((result, i) => {
            const hasError = !!result.error
            const rowColor = result.timedOut
              ? '#f59e0b'
              : hasError
              ? '#ef4444'
              : result.exitCode !== 0
              ? '#ef4444'
              : '#00ff88'

            return (
              <div
                key={i}
                style={{
                  marginBottom: 10,
                  border: `1px solid ${rowColor}33`,
                  background: result.timedOut
                    ? 'rgba(245,158,11,0.05)'
                    : hasError || result.exitCode !== 0
                    ? 'rgba(239,68,68,0.05)'
                    : 'rgba(0,255,136,0.03)',
                }}
              >
                {/* Result header */}
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 10,
                    padding: '6px 10px',
                    borderBottom: `1px solid ${rowColor}22`,
                    background: `${rowColor}0a`,
                  }}
                >
                  <span
                    style={{
                      width: 8,
                      height: 8,
                      borderRadius: '50%',
                      background: rowColor,
                      boxShadow: `0 0 4px ${rowColor}88`,
                      flexShrink: 0,
                    }}
                  />
                  <span
                    style={{
                      fontFamily: "'Rajdhani', sans-serif",
                      fontSize: 13,
                      fontWeight: 600,
                      color: '#e0e0e0',
                    }}
                  >
                    {result.host}
                  </span>
                  <span
                    style={{
                      fontFamily: "'JetBrains Mono', monospace",
                      fontSize: 10,
                      color: rowColor,
                      marginLeft: 'auto',
                    }}
                  >
                    {result.timedOut
                      ? 'TIMED OUT'
                      : hasError
                      ? result.error
                      : `exit ${result.exitCode}`}
                  </span>
                </div>

                {/* stdout */}
                {result.stdout && (
                  <pre
                    style={{
                      margin: 0,
                      padding: '6px 10px',
                      fontFamily: "'JetBrains Mono', monospace",
                      fontSize: 11,
                      color: '#c8d8f0',
                      whiteSpace: 'pre-wrap',
                      wordBreak: 'break-all',
                      borderBottom: result.stderr ? `1px solid ${rowColor}11` : 'none',
                    }}
                  >
                    {result.stdout}
                  </pre>
                )}

                {/* stderr */}
                {result.stderr && (
                  <pre
                    style={{
                      margin: 0,
                      padding: '6px 10px',
                      fontFamily: "'JetBrains Mono', monospace",
                      fontSize: 11,
                      color: '#ef4444',
                      whiteSpace: 'pre-wrap',
                      wordBreak: 'break-all',
                    }}
                  >
                    {result.stderr}
                  </pre>
                )}
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
