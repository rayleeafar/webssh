'use client'

import { useState, useEffect } from 'react'
import { apiPost, apiPut } from '@/lib/api'

interface Node {
  id: number
  name: string
  host: string
  port: number
  username: string
  created_at: string
}

interface NodeModalProps {
  node: Node | null
  onClose: () => void
  onSave: () => void
}

type AuthTab = 'password' | 'private_key'

export default function NodeModal({ node, onClose, onSave }: NodeModalProps) {
  const [name, setName] = useState(node?.name ?? '')
  const [host, setHost] = useState(node?.host ?? '')
  const [port, setPort] = useState(node?.port ?? 22)
  const [username, setUsername] = useState(node?.username ?? '')
  const [password, setPassword] = useState('')
  const [privateKey, setPrivateKey] = useState('')
  const [authTab, setAuthTab] = useState<AuthTab>('password')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const isEditing = !!node

  useEffect(() => {
    if (node) {
      setName(node.name)
      setHost(node.host)
      setPort(node.port)
      setUsername(node.username)
    }
  }, [node])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    const payload = {
      name,
      host,
      port,
      username,
      password: authTab === 'password' ? password : '',
      private_key: authTab === 'private_key' ? privateKey : '',
    }

    try {
      let res: Response
      if (isEditing) {
        res = await apiPut(`/api/nodes/${node!.id}`, payload)
      } else {
        res = await apiPost('/api/nodes', payload)
      }

      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || (isEditing ? 'Failed to update node' : 'Failed to create node'))
      }

      onSave()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed')
    } finally {
      setLoading(false)
    }
  }

  // Close on backdrop click
  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) onClose()
  }

  // Close on Escape
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  return (
    <div
      onClick={handleBackdropClick}
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(5,5,8,0.85)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 100,
        backdropFilter: 'blur(2px)',
      }}
    >
      <div
        style={{
          position: 'relative',
          width: '100%',
          maxWidth: 480,
          maxHeight: '90vh',
          overflowY: 'auto',
          background: '#0d0d1a',
          border: '1px solid rgba(0,255,255,0.3)',
          boxShadow: '0 0 60px rgba(0,255,255,0.12), 0 0 120px rgba(0,255,255,0.05)',
          padding: '32px 32px 28px',
        }}
        className="corner-tl corner-tr corner-bl corner-br"
      >
        {/* Title */}
        <div style={{ marginBottom: 28 }}>
          <h2
            style={{
              fontFamily: "'Orbitron', sans-serif",
              fontSize: 16,
              fontWeight: 700,
              color: '#00ffff',
              letterSpacing: '0.15em',
              textShadow: '0 0 12px rgba(0,255,255,0.5)',
              margin: 0,
              marginBottom: 4,
            }}
          >
            {isEditing ? 'EDIT NODE' : 'ADD NODE'}
          </h2>
          <div
            style={{
              width: 40,
              height: 1,
              background: 'linear-gradient(90deg, #00ffff, transparent)',
            }}
          />
        </div>

        {error && (
          <div
            style={{
              marginBottom: 20,
              padding: '10px 14px',
              background: 'rgba(255,48,96,0.1)',
              border: '1px solid rgba(255,48,96,0.3)',
              color: '#ff3060',
              fontSize: 13,
            }}
          >
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          {/* Basic fields */}
          <div style={{ marginBottom: 20 }}>
            <FormField
              label="NODE NAME"
              type="text"
              value={name}
              onChange={setName}
              required
              placeholder="production-web-01"
            />
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 100px', gap: 12, marginBottom: 20 }}>
            <FormField
              label="HOST"
              type="text"
              value={host}
              onChange={setHost}
              required
              placeholder="192.168.1.100"
            />
            <FormField
              label="PORT"
              type="number"
              value={String(port)}
              onChange={(v) => setPort(parseInt(v) || 22)}
              required
              placeholder="22"
            />
          </div>

          <div style={{ marginBottom: 24 }}>
            <FormField
              label="USERNAME"
              type="text"
              value={username}
              onChange={setUsername}
              required
              placeholder="root"
            />
          </div>

          {/* Auth type tabs */}
          <div style={{ marginBottom: 16 }}>
            <div
              style={{
                fontFamily: "'Orbitron', sans-serif",
                fontSize: 9,
                letterSpacing: '0.2em',
                color: '#404070',
                marginBottom: 10,
              }}
            >
              AUTHENTICATION
            </div>
            <div
              style={{
                display: 'flex',
                borderBottom: '1px solid rgba(0,255,255,0.1)',
                marginBottom: 16,
              }}
            >
              {(['password', 'private_key'] as AuthTab[]).map((t) => (
                <button
                  key={t}
                  type="button"
                  onClick={() => setAuthTab(t)}
                  style={{
                    background: 'transparent',
                    border: 'none',
                    borderBottom: authTab === t ? '2px solid #00ffff' : '2px solid transparent',
                    color: authTab === t ? '#00ffff' : '#404070',
                    fontFamily: "'Orbitron', sans-serif",
                    fontSize: 9,
                    letterSpacing: '0.15em',
                    padding: '6px 16px 8px',
                    cursor: 'pointer',
                    transition: 'color 0.15s, border-color 0.15s',
                    marginBottom: -1,
                    textShadow: authTab === t ? '0 0 8px rgba(0,255,255,0.4)' : 'none',
                  }}
                >
                  {t === 'password' ? 'PASSWORD' : 'PRIVATE KEY'}
                </button>
              ))}
            </div>

            {authTab === 'password' ? (
              <FormField
                label={isEditing ? 'NEW PASSWORD (leave blank to keep)' : 'PASSWORD'}
                type="password"
                value={password}
                onChange={setPassword}
                required={!isEditing}
                placeholder={isEditing ? '••••••••' : 'Enter password'}
              />
            ) : (
              <div>
                <label
                  style={{
                    display: 'block',
                    fontFamily: "'Orbitron', sans-serif",
                    fontSize: 9,
                    letterSpacing: '0.2em',
                    color: '#404070',
                    marginBottom: 8,
                  }}
                >
                  {isEditing ? 'NEW PRIVATE KEY (leave blank to keep)' : 'PRIVATE KEY'}
                </label>
                <textarea
                  value={privateKey}
                  onChange={(e) => setPrivateKey(e.target.value)}
                  placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                  required={!isEditing && authTab === 'private_key'}
                  rows={5}
                  style={{
                    width: '100%',
                    background: 'rgba(8,8,16,0.8)',
                    border: 'none',
                    borderBottom: '1px solid rgba(0,255,255,0.3)',
                    color: '#c8d8f0',
                    fontFamily: "'JetBrains Mono', monospace",
                    fontSize: 11,
                    padding: '8px 4px',
                    outline: 'none',
                    resize: 'vertical',
                    minHeight: 80,
                    transition: 'border-color 0.2s',
                  }}
                  onFocus={(e) => {
                    e.currentTarget.style.borderBottomColor = '#00ffff'
                  }}
                  onBlur={(e) => {
                    e.currentTarget.style.borderBottomColor = 'rgba(0,255,255,0.3)'
                  }}
                />
              </div>
            )}
          </div>

          {/* Actions */}
          <div style={{ display: 'flex', gap: 10, marginTop: 28 }}>
            <button
              type="submit"
              disabled={loading}
              style={{
                flex: 1,
                padding: '11px',
                background: 'transparent',
                border: '1px solid rgba(0,255,255,0.6)',
                color: '#00ffff',
                fontFamily: "'Orbitron', sans-serif",
                fontSize: 11,
                letterSpacing: '0.2em',
                cursor: loading ? 'not-allowed' : 'pointer',
                opacity: loading ? 0.6 : 1,
                boxShadow: '0 0 12px rgba(0,255,255,0.15)',
                transition: 'box-shadow 0.2s, background 0.2s',
              }}
              onMouseEnter={(e) => {
                if (!loading) {
                  e.currentTarget.style.boxShadow = '0 0 20px rgba(0,255,255,0.4)'
                  e.currentTarget.style.background = 'rgba(0,255,255,0.06)'
                }
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.boxShadow = '0 0 12px rgba(0,255,255,0.15)'
                e.currentTarget.style.background = 'transparent'
              }}
            >
              {loading ? 'SAVING...' : isEditing ? 'UPDATE NODE' : 'CREATE NODE'}
            </button>
            <button
              type="button"
              onClick={onClose}
              style={{
                padding: '11px 20px',
                background: 'transparent',
                border: '1px solid rgba(255,255,255,0.1)',
                color: '#6070a0',
                fontFamily: "'Orbitron', sans-serif",
                fontSize: 11,
                letterSpacing: '0.15em',
                cursor: 'pointer',
                transition: 'border-color 0.2s, color 0.2s',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.color = '#c8d8f0'
                e.currentTarget.style.borderColor = 'rgba(255,255,255,0.25)'
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.color = '#6070a0'
                e.currentTarget.style.borderColor = 'rgba(255,255,255,0.1)'
              }}
            >
              CANCEL
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

interface FormFieldProps {
  label: string
  type: string
  value: string
  onChange: (v: string) => void
  required?: boolean
  placeholder?: string
}

function FormField({ label, type, value, onChange, required, placeholder }: FormFieldProps) {
  return (
    <div>
      <label
        style={{
          display: 'block',
          fontFamily: "'Orbitron', sans-serif",
          fontSize: 9,
          letterSpacing: '0.2em',
          color: '#404070',
          marginBottom: 8,
        }}
      >
        {label}
      </label>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        required={required}
        placeholder={placeholder}
        className="neon-input"
        min={type === 'number' ? 1 : undefined}
        max={type === 'number' ? 65535 : undefined}
      />
    </div>
  )
}
