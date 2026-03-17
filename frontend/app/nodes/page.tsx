'use client'

import { useState, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { apiGet, apiPost, apiPut, apiDelete, setCsrfToken } from '@/lib/api'

interface Node {
  id: number
  name: string
  host: string
  port: number
  username: string
  created_at: string
}

export default function NodesPage() {
  const [nodes, setNodes] = useState<Node[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [editingNode, setEditingNode] = useState<Node | null>(null)
  const [formData, setFormData] = useState({
    name: '',
    host: '',
    port: 22,
    username: '',
    password: '',
    private_key: '',
  })
  const [error, setError] = useState('')
  const router = useRouter()

  useEffect(() => {
    checkAuthAndFetch()
  }, [])

  const checkAuthAndFetch = async () => {
    try {
      // Bootstrap auth state from the session cookie
      const meRes = await apiGet('/api/auth/me')
      if (meRes.status === 401) {
        router.push('/login')
        return
      }
      if (meRes.ok) {
        const me = await meRes.json()
        if (me.csrf_token) setCsrfToken(me.csrf_token)
      }
      await fetchNodes()
    } catch {
      router.push('/login')
    }
  }

  const fetchNodes = async () => {
    try {
      const res = await apiGet('/api/nodes')

      if (res.status === 401) {
        router.push('/login')
        return
      }

      if (!res.ok) throw new Error('Failed to fetch nodes')

      const data = await res.json()
      setNodes(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load nodes')
    } finally {
      setLoading(false)
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    try {
      if (editingNode) {
        const res = await apiPut(`/api/nodes/${editingNode.id}`, formData)
        if (!res.ok) {
          const text = await res.text()
          throw new Error(text || 'Failed to update node')
        }
      } else {
        const res = await apiPost('/api/nodes', formData)
        if (!res.ok) {
          const text = await res.text()
          throw new Error(text || 'Failed to create node')
        }
      }

      setShowForm(false)
      setEditingNode(null)
      setFormData({ name: '', host: '', port: 22, username: '', password: '', private_key: '' })
      fetchNodes()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save node')
    }
  }

  const handleEdit = (node: Node) => {
    setEditingNode(node)
    setFormData({
      name: node.name,
      host: node.host,
      port: node.port,
      username: node.username,
      password: '',
      private_key: '',
    })
    setShowForm(true)
  }

  const handleCancelEdit = () => {
    setShowForm(false)
    setEditingNode(null)
    setFormData({ name: '', host: '', port: 22, username: '', password: '', private_key: '' })
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Are you sure you want to delete this node?')) return

    try {
      const res = await apiDelete(`/api/nodes/${id}`)
      if (!res.ok) throw new Error('Failed to delete node')
      fetchNodes()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete node')
    }
  }

  if (loading) {
    return <div className="min-h-screen flex items-center justify-center">Loading...</div>
  }

  return (
    <div className="min-h-screen bg-gray-50 p-8">
      <div className="max-w-6xl mx-auto">
        <div className="flex justify-between items-center mb-8">
          <h1 className="text-3xl font-bold">SSH Nodes</h1>
          <button
            onClick={() => {
              if (showForm && !editingNode) {
                handleCancelEdit()
              } else {
                setShowForm(!showForm)
                setEditingNode(null)
                setFormData({ name: '', host: '', port: 22, username: '', password: '', private_key: '' })
              }
            }}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"
          >
            {showForm && !editingNode ? 'Cancel' : 'Add Node'}
          </button>
        </div>

        {error && (
          <div className="mb-4 bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded">
            {error}
          </div>
        )}

        {showForm && (
          <div className="mb-8 bg-white p-6 rounded-lg shadow">
            <h2 className="text-xl font-bold mb-4">
              {editingNode ? 'Edit Node' : 'Add New Node'}
            </h2>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700">Name</label>
                <input
                  type="text"
                  required
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">Host</label>
                <input
                  type="text"
                  required
                  value={formData.host}
                  onChange={(e) => setFormData({ ...formData, host: e.target.value })}
                  className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">Port</label>
                <input
                  type="number"
                  required
                  value={formData.port}
                  onChange={(e) => setFormData({ ...formData, port: parseInt(e.target.value) })}
                  className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">Username</label>
                <input
                  type="text"
                  required
                  value={formData.username}
                  onChange={(e) => setFormData({ ...formData, username: e.target.value })}
                  className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">
                  Password {editingNode && '(leave blank to keep current)'}
                </label>
                <input
                  type="password"
                  required={!editingNode && !formData.private_key}
                  value={formData.password}
                  onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                  className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">
                  Private Key (optional, use instead of password)
                </label>
                <textarea
                  value={formData.private_key}
                  onChange={(e) => setFormData({ ...formData, private_key: e.target.value })}
                  placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                  rows={4}
                  className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md font-mono text-sm"
                />
              </div>
              <div className="flex gap-2">
                <button
                  type="submit"
                  className="flex-1 px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"
                >
                  {editingNode ? 'Update Node' : 'Create Node'}
                </button>
                {editingNode && (
                  <button
                    type="button"
                    onClick={handleCancelEdit}
                    className="px-4 py-2 bg-gray-600 text-white rounded hover:bg-gray-700"
                  >
                    Cancel
                  </button>
                )}
              </div>
            </form>
          </div>
        )}

        <div className="grid gap-4">
          {nodes.length === 0 ? (
            <div className="text-center py-12 text-gray-500">
              No nodes configured. Add your first SSH node to get started.
            </div>
          ) : (
            nodes.map((node) => (
              <div key={node.id} className="bg-white p-6 rounded-lg shadow">
                <div className="flex justify-between items-start">
                  <div>
                    <h3 className="text-xl font-bold">{node.name}</h3>
                    <p className="text-gray-600 mt-1">
                      {node.username}@{node.host}:{node.port}
                    </p>
                  </div>
                  <div className="flex gap-2">
                    <a
                      href={`/terminal/${node.id}`}
                      className="px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700"
                    >
                      Terminal
                    </a>
                    <a
                      href={`/sftp/${node.id}`}
                      className="px-4 py-2 bg-purple-600 text-white rounded hover:bg-purple-700"
                    >
                      SFTP
                    </a>
                    <button
                      onClick={() => handleEdit(node)}
                      className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"
                    >
                      Edit
                    </button>
                    <button
                      onClick={() => handleDelete(node.id)}
                      className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700"
                    >
                      Delete
                    </button>
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
