'use client'

import { useState, useEffect } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { apiGet, apiPost, apiDelete, apiRequest, setCsrfToken } from '@/lib/api'


interface FileInfo {
  name: string
  size: number
  mode: string
  mod_time: string
  is_dir: boolean
}

export default function SFTPPage() {
  const searchParams = useSearchParams()
  const nodeId = searchParams.get('nodeId') || ''
  const router = useRouter()
  const [files, setFiles] = useState<FileInfo[]>([])
  const [currentPath, setCurrentPath] = useState('.')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showMkdirForm, setShowMkdirForm] = useState(false)
  const [newDirName, setNewDirName] = useState('')
  const [uploading, setUploading] = useState(false)

  useEffect(() => {
    checkAuth()
  }, [])

  useEffect(() => {
    if (!loading) fetchFiles(currentPath)
  }, [currentPath])

  const checkAuth = async () => {
    try {
      const res = await apiGet('/api/auth/me')
      if (res.status === 401) {
        router.push('/login')
        return
      }
      if (res.ok) {
        const me = await res.json()
        if (me.csrf_token) setCsrfToken(me.csrf_token)
      }
      await fetchFiles(currentPath)
    } catch {
      router.push('/login')
    }
  }

  const fetchFiles = async (path: string) => {
    setLoading(true)
    setError('')

    try {
      const res = await apiGet(
        `/api/sftp/list?nodeId=${nodeId}&path=${encodeURIComponent(path)}`
      )

      if (res.status === 401) {
        router.push('/login')
        return
      }

      if (!res.ok) throw new Error('Failed to list files')

      const data = await res.json()
      setFiles(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load files')
    } finally {
      setLoading(false)
    }
  }

  const handleNavigate = (name: string, isDir: boolean) => {
    if (!isDir) return

    if (name === '..') {
      const parts = currentPath.split('/').filter(p => p && p !== '.')
      parts.pop()
      setCurrentPath(parts.length > 0 ? parts.join('/') : '.')
    } else {
      const newPath = currentPath === '.' ? name : `${currentPath}/${name}`
      setCurrentPath(newPath)
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
    if (!file) return

    setUploading(true)
    setError('')

    try {
      const formData = new FormData()
      formData.append('file', file)
      formData.append('nodeId', nodeId as string)
      formData.append('path', currentPath === '.' ? file.name : `${currentPath}/${file.name}`)

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
    if (!newDirName.trim()) return

    setError('')

    try {
      const dirPath = currentPath === '.' ? newDirName : `${currentPath}/${newDirName}`

      const res = await apiPost('/api/sftp/mkdir', {
        node_id: parseInt(nodeId as string),
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

  return (
    <div className="min-h-screen bg-gray-50 p-8">
      <div className="max-w-6xl mx-auto">
        <div className="flex justify-between items-center mb-8">
          <div>
            <a href="/nodes" className="text-blue-600 hover:text-blue-700 mb-2 inline-block">
              ← Back to Nodes
            </a>
            <h1 className="text-3xl font-bold">SFTP Browser</h1>
            <p className="text-gray-600 mt-1">Current path: {currentPath}</p>
          </div>
          <div className="flex gap-2">
            <label className="px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700 cursor-pointer">
              {uploading ? 'Uploading...' : 'Upload File'}
              <input
                type="file"
                onChange={handleUpload}
                disabled={uploading}
                className="hidden"
              />
            </label>
            <button
              onClick={() => setShowMkdirForm(!showMkdirForm)}
              className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"
            >
              {showMkdirForm ? 'Cancel' : 'New Folder'}
            </button>
          </div>
        </div>

        {showMkdirForm && (
          <div className="mb-4 bg-white p-4 rounded-lg shadow">
            <form onSubmit={handleMkdir} className="flex gap-2">
              <input
                type="text"
                value={newDirName}
                onChange={(e) => setNewDirName(e.target.value)}
                placeholder="Directory name"
                required
                className="flex-1 px-3 py-2 border border-gray-300 rounded-md"
              />
              <button
                type="submit"
                className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"
              >
                Create
              </button>
            </form>
          </div>
        )}

        {error && (
          <div className="mb-4 bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded">
            {error}
          </div>
        )}

        {loading ? (
          <div className="text-center py-12">Loading...</div>
        ) : (
          <div className="bg-white rounded-lg shadow overflow-hidden">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Permissions</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Size</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Modified</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {currentPath !== '.' && (
                  <tr className="hover:bg-gray-50 cursor-pointer" onClick={() => handleNavigate('..', true)}>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className="text-blue-600">📁 ..</span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">-</td>
                    <td className="px-6 py-4 whitespace-nowrap">-</td>
                    <td className="px-6 py-4 whitespace-nowrap">-</td>
                    <td className="px-6 py-4 whitespace-nowrap">-</td>
                  </tr>
                )}
                {files.map((file) => (
                  <tr key={file.name} className="hover:bg-gray-50">
                    <td
                      className="px-6 py-4 whitespace-nowrap cursor-pointer"
                      onClick={() => handleNavigate(file.name, file.is_dir)}
                    >
                      <span className={file.is_dir ? 'text-blue-600' : ''}>
                        {file.is_dir ? '📁' : '📄'} {file.name}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 font-mono">
                      {file.mode}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {file.is_dir ? '-' : `${(file.size / 1024).toFixed(2)} KB`}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {file.mod_time}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm">
                      {!file.is_dir && (
                        <>
                          <button
                            onClick={() => handleDownload(file.name)}
                            className="text-blue-600 hover:text-blue-700 mr-4"
                          >
                            Download
                          </button>
                          <button
                            onClick={() => handleDelete(file.name)}
                            className="text-red-600 hover:text-red-700"
                          >
                            Delete
                          </button>
                        </>
                      )}
                      {file.is_dir && (
                        <button
                          onClick={() => handleDelete(file.name)}
                          className="text-red-600 hover:text-red-700"
                        >
                          Delete
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

