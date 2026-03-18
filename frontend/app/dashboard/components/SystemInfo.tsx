'use client'

import { useState, useEffect, useCallback } from 'react'
import { apiGet } from '@/lib/api'

interface SysInfoData {
  hostname: string
  os: string
  kernel: string
  uptime: string
  cpu_model: string
  cpu_cores: number
  load_avg: string
  mem_total: number
  mem_used: number
  disk_total: string
  disk_used: string
  disk_pct: string
  ip_addr: string
}

interface SystemInfoProps {
  nodeId: number | null
}

function ProgressBar({
  pct,
  color,
  label,
}: {
  pct: number
  color: string
  label: string
}) {
  const clamped = Math.max(0, Math.min(100, pct))
  const warn = clamped > 80
  const activeColor = warn ? '#ff3060' : color

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
      <span
        style={{
          fontFamily: "'Orbitron', sans-serif",
          fontSize: 8,
          letterSpacing: '0.15em',
          color: '#404070',
          width: 28,
          flexShrink: 0,
        }}
      >
        {label}
      </span>
      <div
        style={{
          flex: 1,
          height: 6,
          background: 'rgba(255,255,255,0.05)',
          borderRadius: 3,
          overflow: 'hidden',
          position: 'relative',
        }}
      >
        <div
          style={{
            width: `${clamped}%`,
            height: '100%',
            background: activeColor,
            boxShadow: `0 0 6px ${activeColor}88`,
            borderRadius: 3,
            transition: 'width 0.5s ease',
          }}
        />
      </div>
      <span
        style={{
          fontFamily: "'JetBrains Mono', monospace",
          fontSize: 10,
          color: warn ? '#ff3060' : '#6070a0',
          width: 36,
          textAlign: 'right',
          flexShrink: 0,
        }}
      >
        {clamped.toFixed(0)}%
      </span>
    </div>
  )
}

function InfoCard({
  label,
  value,
  color = '#8090b8',
}: {
  label: string
  value: string
  color?: string
}) {
  return (
    <div
      style={{
        background: 'rgba(5,5,8,0.6)',
        border: '1px solid rgba(0,255,255,0.1)',
        padding: '10px 14px',
      }}
    >
      <div
        style={{
          fontFamily: "'Orbitron', sans-serif",
          fontSize: 8,
          letterSpacing: '0.2em',
          color: '#404070',
          marginBottom: 6,
        }}
      >
        {label}
      </div>
      <div
        style={{
          fontFamily: "'Rajdhani', sans-serif",
          fontSize: 13,
          fontWeight: 600,
          color,
          wordBreak: 'break-all',
        }}
      >
        {value || '—'}
      </div>
    </div>
  )
}

export default function SystemInfo({ nodeId }: SystemInfoProps) {
  const [data, setData] = useState<SysInfoData | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)

  const fetchSysInfo = useCallback(async () => {
    if (!nodeId) return
    setLoading(true)
    setError('')
    try {
      const res = await apiGet(`/api/nodes/${nodeId}/sysinfo`)
      if (!res.ok) throw new Error(`Failed to fetch system info (${res.status})`)
      const json = await res.json()
      setData(json)
      setLastUpdated(new Date())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch system info')
    } finally {
      setLoading(false)
    }
  }, [nodeId])

  useEffect(() => {
    if (!nodeId) return
    fetchSysInfo()
    const interval = setInterval(fetchSysInfo, 30000)
    return () => clearInterval(interval)
  }, [nodeId, fetchSysInfo])

  if (!nodeId) {
    return (
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
        }}
      >
        SELECT A NODE AND OPEN A TERMINAL TO VIEW SYSTEM INFO
      </div>
    )
  }

  const memPct = data && data.mem_total > 0 ? (data.mem_used / data.mem_total) * 100 : 0
  const diskPct = data ? parseInt(data.disk_pct || '0', 10) : 0
  const loadVals = data?.load_avg?.split(' ').map(Number) ?? []
  const cpuLoad = data && data.cpu_cores > 0 && loadVals[0]
    ? Math.min(100, (loadVals[0] / data.cpu_cores) * 100)
    : 0

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        overflow: 'hidden',
        background: '#080810',
      }}
    >
      {/* Toolbar */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '6px 12px',
          borderBottom: '1px solid rgba(0,255,255,0.08)',
          flexShrink: 0,
        }}
      >
        <span
          style={{
            fontFamily: "'Orbitron', sans-serif",
            fontSize: 9,
            letterSpacing: '0.25em',
            color: '#404070',
          }}
        >
          SYSTEM INFORMATION
        </span>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          {lastUpdated && (
            <span
              style={{
                fontFamily: "'JetBrains Mono', monospace",
                fontSize: 10,
                color: '#303060',
              }}
            >
              {lastUpdated.toLocaleTimeString()}
            </span>
          )}
          <button
            onClick={fetchSysInfo}
            disabled={loading}
            style={{
              background: 'transparent',
              border: '1px solid rgba(0,255,255,0.2)',
              color: loading ? '#303060' : '#6070a0',
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 9,
              letterSpacing: '0.15em',
              padding: '3px 10px',
              cursor: loading ? 'not-allowed' : 'pointer',
              transition: 'color 0.15s, border-color 0.15s',
            }}
          >
            {loading ? '...' : '↺ REFRESH'}
          </button>
        </div>
      </div>

      {error && (
        <div
          style={{
            padding: '6px 12px',
            background: 'rgba(255,48,96,0.08)',
            borderBottom: '1px solid rgba(255,48,96,0.2)',
            color: '#ff3060',
            fontSize: 12,
            flexShrink: 0,
          }}
        >
          {error}
        </div>
      )}

      {!data && !loading && !error && (
        <div
          style={{
            flex: 1,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#303060',
            fontFamily: "'Orbitron', sans-serif",
            fontSize: 10,
            letterSpacing: '0.2em',
          }}
        >
          CONNECTING...
        </div>
      )}

      {data && (
        <div style={{ flex: 1, overflowY: 'auto', padding: '12px' }}>
          {/* Identity grid */}
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))',
              gap: 8,
              marginBottom: 16,
            }}
          >
            <InfoCard label="HOSTNAME" value={data.hostname} color="#00ffff" />
            <InfoCard label="IP ADDRESS" value={data.ip_addr} />
            <InfoCard label="OS" value={data.os} />
            <InfoCard label="KERNEL" value={data.kernel} />
            <InfoCard label="UPTIME" value={data.uptime} color="#00ff88" />
          </div>

          {/* CPU section */}
          <div
            style={{
              background: 'rgba(5,5,8,0.6)',
              border: '1px solid rgba(0,255,255,0.1)',
              padding: '12px 14px',
              marginBottom: 8,
            }}
          >
            <div
              style={{
                fontFamily: "'Orbitron', sans-serif",
                fontSize: 9,
                letterSpacing: '0.2em',
                color: '#404070',
                marginBottom: 10,
              }}
            >
              CPU
            </div>
            <div
              style={{
                fontFamily: "'Rajdhani', sans-serif",
                fontSize: 12,
                color: '#6070a0',
                marginBottom: 8,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {data.cpu_model || '—'}
            </div>
            <div
              style={{
                display: 'flex',
                gap: 16,
                marginBottom: 10,
              }}
            >
              <div>
                <span style={{ color: '#303060', fontFamily: "'Orbitron', sans-serif", fontSize: 8, letterSpacing: '0.1em' }}>CORES </span>
                <span style={{ color: '#c8d8f0', fontFamily: "'JetBrains Mono', monospace", fontSize: 13, fontWeight: 500 }}>
                  {data.cpu_cores || '—'}
                </span>
              </div>
              <div>
                <span style={{ color: '#303060', fontFamily: "'Orbitron', sans-serif", fontSize: 8, letterSpacing: '0.1em' }}>LOAD AVG </span>
                <span style={{ color: '#c8d8f0', fontFamily: "'JetBrains Mono', monospace", fontSize: 13, fontWeight: 500 }}>
                  {data.load_avg || '—'}
                </span>
              </div>
            </div>
            <ProgressBar pct={cpuLoad} color="#00ffff" label="CPU" />
          </div>

          {/* Memory */}
          <div
            style={{
              background: 'rgba(5,5,8,0.6)',
              border: '1px solid rgba(0,255,255,0.1)',
              padding: '12px 14px',
              marginBottom: 8,
            }}
          >
            <div
              style={{
                fontFamily: "'Orbitron', sans-serif",
                fontSize: 9,
                letterSpacing: '0.2em',
                color: '#404070',
                marginBottom: 10,
              }}
            >
              MEMORY
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: '#6070a0' }}>
                {data.mem_used} MB used
              </span>
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: '#404070' }}>
                {data.mem_total} MB total
              </span>
            </div>
            <ProgressBar pct={memPct} color="#ff00ff" label="MEM" />
          </div>

          {/* Disk */}
          <div
            style={{
              background: 'rgba(5,5,8,0.6)',
              border: '1px solid rgba(0,255,255,0.1)',
              padding: '12px 14px',
            }}
          >
            <div
              style={{
                fontFamily: "'Orbitron', sans-serif",
                fontSize: 9,
                letterSpacing: '0.2em',
                color: '#404070',
                marginBottom: 10,
              }}
            >
              DISK (/)
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: '#6070a0' }}>
                {data.disk_used} used
              </span>
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: '#404070' }}>
                {data.disk_total} total
              </span>
            </div>
            <ProgressBar pct={diskPct} color="#0080ff" label="DSK" />
          </div>
        </div>
      )}
    </div>
  )
}
