'use client'

import { useState, useEffect, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { apiGet, apiDelete, setCsrfToken } from '@/lib/api'
import dynamic from 'next/dynamic'
import Header from './components/Header'
import NodeSidebar from './components/NodeSidebar'
import TabBar from './components/TabBar'
import EmptyState from './components/EmptyState'
import BottomPanel from './components/BottomPanel'
import BottomTabBar from './components/BottomTabBar'
import NodeModal from './components/NodeModal'

// xterm uses browser globals (self, window) — must be loaded client-side only
const TerminalPane = dynamic(() => import('./components/TerminalPane'), { ssr: false })

interface Tab {
  id: string
  nodeId: number
  nodeName: string
  host: string
}

interface Node {
  id: number
  name: string
  host: string
  port: number
  username: string
  created_at: string
}

export default function DashboardPage() {
  const [nodes, setNodes] = useState<Node[]>([])
  const [tabs, setTabs] = useState<Tab[]>([])
  const [activeTabId, setActiveTabId] = useState<string | null>(null)
  const [bottomTab, setBottomTab] = useState<'sftp' | 'sysinfo' | null>(null)
  const [bottomHeight, setBottomHeight] = useState(240)
  const [showNodeModal, setShowNodeModal] = useState(false)
  const [editingNode, setEditingNode] = useState<Node | null>(null)
  const [user, setUser] = useState<{ username: string } | null>(null)
  const router = useRouter()

  const fetchNodes = useCallback(async () => {
    try {
      const res = await apiGet('/api/nodes')
      if (res.status === 401) {
        router.push('/login')
        return
      }
      if (!res.ok) throw new Error('Failed to fetch nodes')
      const data = await res.json()
      setNodes(data)
    } catch {
      // ignore fetch errors
    }
  }, [router])

  useEffect(() => {
    const init = async () => {
      try {
        const meRes = await apiGet('/api/auth/me')
        if (meRes.status === 401) {
          router.push('/login')
          return
        }
        if (meRes.ok) {
          const me = await meRes.json()
          if (me.csrf_token) setCsrfToken(me.csrf_token)
          if (me.username) setUser({ username: me.username })
        }
        await fetchNodes()
      } catch {
        router.push('/login')
      }
    }
    init()
  }, [router, fetchNodes])

  const openTab = useCallback((node: Node) => {
    // Generate id outside the updater so it's stable if React calls
    // the updater more than once (concurrent mode).
    const newTabId = crypto.randomUUID()
    let idToActivate = newTabId

    setTabs((prev) => {
      const existing = prev.find((t) => t.nodeId === node.id)
      if (existing) {
        idToActivate = existing.id
        return prev
      }
      return [...prev, { id: newTabId, nodeId: node.id, nodeName: node.name, host: node.host }]
    })

    // Call outside the updater — no side-effects inside state updaters.
    setActiveTabId(idToActivate)
  }, [])

  const closeTab = useCallback((tabId: string) => {
    setTabs((prev) => {
      const idx = prev.findIndex((t) => t.id === tabId)
      const next = prev.filter((t) => t.id !== tabId)
      setActiveTabId((cur) => {
        if (cur !== tabId) return cur
        if (next.length === 0) return null
        // activate adjacent tab
        const newIdx = Math.min(idx, next.length - 1)
        return next[newIdx].id
      })
      return next
    })
  }, [])

  const handleDeleteNode = useCallback(
    async (id: number) => {
      if (!confirm('Delete this node?')) return
      try {
        const res = await apiDelete(`/api/nodes/${id}`)
        if (!res.ok) throw new Error('Failed to delete node')
        // Close any open tab for this node
        setTabs((prev) => {
          const toClose = prev.filter((t) => t.nodeId === id)
          toClose.forEach((t) => closeTab(t.id))
          return prev.filter((t) => t.nodeId !== id)
        })
        fetchNodes()
      } catch (err) {
        alert(err instanceof Error ? err.message : 'Delete failed')
      }
    },
    [fetchNodes, closeTab]
  )

  const activeTab = tabs.find((t) => t.id === activeTabId) ?? null
  const activeNodeId = activeTab?.nodeId ?? null
  const activeNodeIds = new Set(tabs.map((t) => t.nodeId))

  const handleBottomTabClick = (t: 'sftp' | 'sysinfo') => {
    setBottomTab((prev) => (prev === t ? null : t))
  }

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100vh',
        background: '#050508',
        overflow: 'hidden',
      }}
    >
      <Header user={user} />

      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        {/* Sidebar */}
        <NodeSidebar
          nodes={nodes}
          activeNodeIds={activeNodeIds}
          onNodeClick={openTab}
          onAddNode={() => {
            setEditingNode(null)
            setShowNodeModal(true)
          }}
          onEditNode={(node) => {
            setEditingNode(node)
            setShowNodeModal(true)
          }}
          onDeleteNode={handleDeleteNode}
        />

        {/* Main content */}
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            flex: 1,
            minWidth: 0,
            overflow: 'hidden',
          }}
        >
          {/* Tab bar */}
          <TabBar
            tabs={tabs}
            activeTabId={activeTabId}
            onSelect={setActiveTabId}
            onClose={closeTab}
          />

          {/* Terminal area */}
          <div
            style={{
              flex: 1,
              position: 'relative',
              overflow: 'hidden',
              background: '#050508',
            }}
          >
            {tabs.length === 0 && <EmptyState />}
            {tabs.map((tab) => (
              <TerminalPane
                key={tab.id}
                nodeId={tab.nodeId}
                active={tab.id === activeTabId}
              />
            ))}
          </div>

          {/* Bottom panel */}
          {bottomTab && (
            <BottomPanel
              tab={bottomTab}
              onTabChange={setBottomTab}
              onClose={() => setBottomTab(null)}
              height={bottomHeight}
              onHeightChange={setBottomHeight}
              activeNodeId={activeNodeId}
            />
          )}

          {/* Bottom tab bar */}
          <BottomTabBar
            activeTab={bottomTab}
            onTabClick={handleBottomTabClick}
          />
        </div>
      </div>

      {/* Node modal */}
      {showNodeModal && (
        <NodeModal
          node={editingNode}
          onClose={() => {
            setShowNodeModal(false)
            setEditingNode(null)
          }}
          onSave={() => {
            setShowNodeModal(false)
            setEditingNode(null)
            fetchNodes()
          }}
        />
      )}
    </div>
  )
}
