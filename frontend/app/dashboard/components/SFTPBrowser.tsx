'use client'

import { useState, useEffect, useCallback } from 'react'
import { apiGet, apiPost, apiDelete, apiRequest } from '@/lib/api'

interface FileInfo {
  name: string
  size: number
  mode: string
  mod_time: string
  is_dir: boolean
}

interface SFTPBrowserProps {
  nodeId: number | null
  initialPath?: string
}

function formatSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
}

export default function SFTPBrowser({ nodeId, initialPath }: SFTPBrowserProps) {
  const [files, setFiles] = useState<FileInfo[]>([])
  const [currentPath, setCurrentPath] = useState('.')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [showMkdirForm, setShowMkdirForm] = useState(false)
  const [newDirName, setNewDirName] = useState('')
  const [uploading, setUploading] = useState(false)
  const [hoveredRow, setHoveredRow] = useState<string | null>(null)

  const fetchFiles = useCallback(
    async (path: string) => {
      if (!nodeId) return
      setLoading(true)
      setError('')
      try {
        const res = await apiGet(
          `/api/sftp/list?nodeId=${nodeId}&path=${encodeURIComponent(path)}`
        )
        if (!res.ok) throw new Error('Failed to list files')
        const data = await res.json()
        setFiles(data || [])
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load files')
      } finally {
        setLoading(false)
      }
    },
    [nodeId]
  )

  useEffect(() => {
    if (nodeId) {
      const path = initialPath || '.'
      setCurrentPath(path)
      fetchFiles(path)
    }
  }, [nodeId, initialPath, fetchFiles])

  const handleNavigate = (name: string, isDir: boolean) => {
    if (!isDir) return
    if (name === '..') {
      const parts = currentPath.split('/').filter((p) => p && p !== '.')
      parts.pop()
      const newPath = parts.length > 0 ? parts.join('/') : '.'
      setCurrentPath(newPath)
      fetchFiles(newPath)
    } else {
      const newPath = currentPath === '.' ? name : `${currentPath}/${name}`
      setCurrentPath(newPath)
      fetchFiles(newPath)
    }
  }

  const handleDownload = async (name: string) => {
    try {
      const filePath = currentPath === '.' ? name : `${currentPath}/${name}`
      const res = await apiGet(
        `/api/sftp/download?nodeId=${nodeId}&path=${encodeURIComponent(filePath)}`
      )
      if (!res.ok) throw new Error('Download failed')
      const blob = await res.blob()
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = name
      a.click()
      window.URL.revokeObjectURL(url)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Download failed')
    }
  }

  const handleDelete = async (name: string) => {
    if (!confirm(`Delete ${name}?`)) return
    try {
      const filePath = currentPath === '.' ? name : `${currentPath}/${name}`
      const res = await apiDelete(
        `/api/sftp/delete?nodeId=${nodeId}&path=${encodeURIComponent(filePath)}`
      )
      if (!res.ok) throw new Error('Delete failed')
      fetchFiles(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
    }
  }

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file || !nodeId) return
    setUploading(true)
    setError('')
    try {
      const formData = new FormData()
      formData.append('file', file)
      formData.append('nodeId', String(nodeId))
      formData.append(
        'path',
        currentPath === '.' ? file.name : `${currentPath}/${file.name}`
      )
      const res = await apiRequest('/api/sftp/upload', {
        method: 'POST',
        body: formData,
        headers: {},
      })
      if (!res.ok) throw new Error('Upload failed')
      fetchFiles(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Upload failed')
    } finally {
      setUploading(false)
      e.target.value = ''
    }
  }

  const handleMkdir = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newDirName.trim() || !nodeId) return
    setError('')
    try {
      const dirPath =
        currentPath === '.' ? newDirName : `${currentPath}/${newDirName}`
      const res = await apiPost('/api/sftp/mkdir', {
        node_id: nodeId,
        path: dirPath,
      })
      if (!res.ok) throw new Error('Failed to create directory')
      setShowMkdirForm(false)
      setNewDirName('')
      fetchFiles(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create directory')
    }
  }

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
        SELECT A NODE AND OPEN A TERMINAL TO USE SFTP
      </div>
    )
  }

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
          gap: 8,
        }}
      >
        {/* Path breadcrumb */}
        <div
          style={{
            fontFamily: "'JetBrains Mono', monospace",
            fontSize: 11,
            color: '#6070a0',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            flex: 1,
          }}
        >
          <span style={{ color: '#303060' }}>~/</span>
          <span style={{ color: '#8090b8' }}>{currentPath === '.' ? '' : currentPath}</span>
        </div>

        <div style={{ display: 'flex', gap: 6, flexShrink: 0 }}>
          {/* New folder */}
          <button
            onClick={() => setShowMkdirForm(!showMkdirForm)}
            style={toolbarButtonStyle}
            onMouseEnter={(e) => applyBtnHover(e, 'cyan')}
            onMouseLeave={(e) => resetBtnHover(e)}
          >
            {showMkdirForm ? '✕ CANCEL' : '+ FOLDER'}
          </button>

          {/* Upload */}
          <label style={{ ...toolbarButtonStyle, cursor: 'pointer' }}>
            {uploading ? 'UPLOADING...' : '↑ UPLOAD'}
            <input
              type="file"
              onChange={handleUpload}
              disabled={uploading}
              style={{ display: 'none' }}
            />
          </label>

          {/* Refresh */}
          <button
            onClick={() => fetchFiles(currentPath)}
            style={toolbarButtonStyle}
            onMouseEnter={(e) => applyBtnHover(e, 'cyan')}
            onMouseLeave={(e) => resetBtnHover(e)}
          >
            ↺
          </button>
        </div>
      </div>

      {/* Mkdir form */}
      {showMkdirForm && (
        <div
          style={{
            padding: '8px 12px',
            borderBottom: '1px solid rgba(0,255,255,0.08)',
            background: 'rgba(0,255,255,0.03)',
            flexShrink: 0,
          }}
        >
          <form onSubmit={handleMkdir} style={{ display: 'flex', gap: 8 }}>
            <input
              type="text"
              value={newDirName}
              onChange={(e) => setNewDirName(e.target.value)}
              placeholder="Directory name"
              required
              style={{
                flex: 1,
                background: 'rgba(5,5,8,0.8)',
                border: 'none',
                borderBottom: '1px solid rgba(0,255,255,0.3)',
                color: '#c8d8f0',
                fontFamily: "'JetBrains Mono', monospace",
                fontSize: 12,
                padding: '4px 8px',
                outline: 'none',
              }}
              autoFocus
            />
            <button type="submit" style={toolbarButtonStyle}>
              CREATE
            </button>
          </form>
        </div>
      )}

      {/* Error */}
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

      {/* File table */}
      {loading ? (
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
          LOADING...
        </div>
      ) : (
        <div style={{ flex: 1, overflowY: 'auto' }}>
          <table
            style={{
              width: '100%',
              borderCollapse: 'collapse',
              fontFamily: "'Rajdhani', sans-serif",
              fontSize: 13,
            }}
          >
            <thead>
              <tr
                style={{
                  background: '#0a0a14',
                  borderBottom: '1px solid rgba(0,255,255,0.1)',
                }}
              >
                {['NAME', 'MODE', 'SIZE', 'MODIFIED', 'ACTIONS'].map((col) => (
                  <th
                    key={col}
                    className={col === 'MODE' || col === 'MODIFIED' ? 'mobile-hide' : ''}
                    style={{
                      padding: '6px 12px',
                      textAlign: 'left',
                      fontFamily: "'Orbitron', sans-serif",
                      fontSize: 9,
                      letterSpacing: '0.2em',
                      color: '#404070',
                      fontWeight: 400,
                    }}
                  >
                    {col}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {currentPath !== '.' && (
                <tr
                  style={{
                    borderBottom: '1px solid rgba(0,255,255,0.04)',
                    background: hoveredRow === '..' ? 'rgba(0,255,255,0.04)' : 'transparent',
                    cursor: 'pointer',
                    transition: 'background 0.1s',
                  }}
                  onClick={() => handleNavigate('..', true)}
                  onMouseEnter={() => setHoveredRow('..')}
                  onMouseLeave={() => setHoveredRow(null)}
                >
                  <td style={{ padding: '6px 12px', color: '#00ffff' }}>
                    <span style={{ marginRight: 6, opacity: 0.7 }}>▶</span>..
                  </td>
                  <td className="mobile-hide" style={{ padding: '6px 12px', color: '#303060' }}>-</td>
                  <td style={{ padding: '6px 12px', color: '#303060' }}>-</td>
                  <td className="mobile-hide" style={{ padding: '6px 12px', color: '#303060' }}>-</td>
                  <td style={{ padding: '6px 12px' }} />
                </tr>
              )}
              {files.map((file) => (
                <tr
                  key={file.name}
                  style={{
                    borderBottom: '1px solid rgba(0,255,255,0.03)',
                    background:
                      hoveredRow === file.name
                        ? 'rgba(0,255,255,0.03)'
                        : 'transparent',
                    transition: 'background 0.1s',
                  }}
                  onMouseEnter={() => setHoveredRow(file.name)}
                  onMouseLeave={() => setHoveredRow(null)}
                >
                  <td
                    style={{
                      padding: '6px 12px',
                      cursor: file.is_dir ? 'pointer' : 'default',
                      color: file.is_dir ? '#00ffff' : '#8090b8',
                    }}
                    onClick={() => handleNavigate(file.name, file.is_dir)}
                  >
                    <span style={{ marginRight: 6 }}>
                      {file.is_dir ? '▶' : '·'}
                    </span>
                    {file.name}
                  </td>
                  <td
                    className="mobile-hide"
                    style={{
                      padding: '6px 12px',
                      fontFamily: "'JetBrains Mono', monospace",
                      fontSize: 11,
                      color: '#404070',
                    }}
                  >
                    {file.mode}
                  </td>
                  <td
                    style={{
                      padding: '6px 12px',
                      fontFamily: "'JetBrains Mono', monospace",
                      fontSize: 11,
                      color: '#404070',
                    }}
                  >
                    {file.is_dir ? '-' : formatSize(file.size)}
                  </td>
                  <td
                    className="mobile-hide"
                    style={{
                      padding: '6px 12px',
                      fontSize: 11,
                      color: '#404070',
                      fontFamily: "'JetBrains Mono', monospace",
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {file.mod_time}
                  </td>
                  <td style={{ padding: '6px 12px' }}>
                    <div style={{ display: 'flex', gap: 8 }}>
                      {!file.is_dir && (
                        <button
                          onClick={() => handleDownload(file.name)}
                          style={actionButtonStyle('#0080ff')}
                          onMouseEnter={(e) => {
                            e.currentTarget.style.color = '#4499ff'
                            e.currentTarget.style.textShadow = '0 0 8px rgba(0,128,255,0.6)'
                          }}
                          onMouseLeave={(e) => {
                            e.currentTarget.style.color = '#0080ff'
                            e.currentTarget.style.textShadow = 'none'
                          }}
                        >
                          ↓ DL
                        </button>
                      )}
                      <button
                        onClick={() => handleDelete(file.name)}
                        style={actionButtonStyle('#ff3060')}
                        onMouseEnter={(e) => {
                          e.currentTarget.style.color = '#ff6080'
                          e.currentTarget.style.textShadow = '0 0 8px rgba(255,48,96,0.6)'
                        }}
                        onMouseLeave={(e) => {
                          e.currentTarget.style.color = '#ff3060'
                          e.currentTarget.style.textShadow = 'none'
                        }}
                      >
                        ✕ DEL
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
              {files.length === 0 && (
                <tr>
                  <td
                    colSpan={5}
                    style={{
                      padding: '24px',
                      textAlign: 'center',
                      color: '#252545',
                      fontFamily: "'Orbitron', sans-serif",
                      fontSize: 10,
                      letterSpacing: '0.2em',
                    }}
                  >
                    EMPTY DIRECTORY
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

const toolbarButtonStyle: React.CSSProperties = {
  background: 'transparent',
  border: '1px solid rgba(0,255,255,0.2)',
  color: '#6070a0',
  fontFamily: "'Orbitron', sans-serif",
  fontSize: 9,
  letterSpacing: '0.15em',
  padding: '4px 10px',
  cursor: 'pointer',
  transition: 'color 0.15s, border-color 0.15s, box-shadow 0.15s',
  display: 'inline-flex',
  alignItems: 'center',
}

function applyBtnHover(e: React.MouseEvent<HTMLButtonElement>, _color: string) {
  const t = e.currentTarget
  t.style.color = '#00ffff'
  t.style.borderColor = 'rgba(0,255,255,0.5)'
  t.style.boxShadow = '0 0 8px rgba(0,255,255,0.15)'
}

function resetBtnHover(e: React.MouseEvent<HTMLButtonElement>) {
  const t = e.currentTarget
  t.style.color = '#6070a0'
  t.style.borderColor = 'rgba(0,255,255,0.2)'
  t.style.boxShadow = 'none'
}

function actionButtonStyle(color: string): React.CSSProperties {
  return {
    background: 'transparent',
    border: 'none',
    color,
    fontFamily: "'Orbitron', sans-serif",
    fontSize: 9,
    letterSpacing: '0.1em',
    cursor: 'pointer',
    padding: '2px 4px',
    transition: 'color 0.15s, text-shadow 0.15s',
  }
}
