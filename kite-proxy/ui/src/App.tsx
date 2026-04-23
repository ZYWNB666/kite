import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route, NavLink } from 'react-router-dom'
import { Toaster } from 'sonner'
import { Settings, Server, BookOpen } from 'lucide-react'
import ConfigPage from './pages/ConfigPage'
import ClustersPage from './pages/ClustersPage'
import UsagePage from './pages/UsagePage'
import { getConfig, getStatus } from './api/client'
import type { Config, Status } from './types'

function Nav({ configured }: { configured: boolean }) {
  const linkClass = ({ isActive }: { isActive: boolean }) =>
    `flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
      isActive
        ? 'bg-blue-50 text-blue-700'
        : 'text-gray-600 hover:bg-gray-100'
    }`

  return (
    <nav className="space-y-1">
      <NavLink to="/ui/config" className={linkClass}>
        <Settings size={16} />
        Configuration
      </NavLink>
      <NavLink to="/ui/clusters" className={({ isActive }) =>
        linkClass({ isActive }) + (!configured ? ' opacity-40 pointer-events-none' : '')
      }>
        <Server size={16} />
        Clusters
      </NavLink>
      <NavLink to="/ui/usage" className={({ isActive }) =>
        linkClass({ isActive }) + (!configured ? ' opacity-40 pointer-events-none' : '')
      }>
        <BookOpen size={16} />
        Usage
      </NavLink>
    </nav>
  )
}

export default function App() {
  const [config, setConfig] = useState<Config | null>(null)
  const [status, setStatus] = useState<Status | null>(null)

  async function loadConfig() {
    try {
      const cfg = await getConfig()
      setConfig(cfg)
    } catch {
      // not configured yet
    }
  }

  async function loadStatus() {
    try {
      const s = await getStatus()
      setStatus(s)
    } catch {
      // ignore
    }
  }

  useEffect(() => {
    void loadConfig()
    void loadStatus()
  }, [])

  const configured = config?.configured ?? false

  return (
    <BrowserRouter>
      <Toaster richColors position="top-right" />
      <div className="min-h-screen bg-gray-50 flex flex-col">
        {/* Header */}
        <header className="bg-white border-b px-6 py-4 flex items-center justify-between shadow-sm">
          <div className="flex items-center gap-3">
            <div className="font-bold text-lg text-blue-700 tracking-tight">kite-proxy</div>
            <span className="text-xs text-gray-400 hidden sm:inline">
              Kubernetes API Forwarding Proxy
            </span>
          </div>
          <div className="flex items-center gap-2 text-xs text-gray-500">
            <span
              className={`inline-block h-2 w-2 rounded-full ${
                configured ? 'bg-green-500' : 'bg-gray-300'
              }`}
            />
            {configured ? 'Connected' : 'Not configured'}
            {status?.cachedClusters && status.cachedClusters.length > 0 && (
              <span className="ml-2 rounded-full bg-blue-100 text-blue-700 px-2 py-0.5">
                {status.cachedClusters.length} cached
              </span>
            )}
          </div>
        </header>

        <div className="flex flex-1 overflow-hidden">
          {/* Sidebar */}
          <aside className="w-52 bg-white border-r px-4 py-6 hidden sm:block">
            <Nav configured={configured} />
          </aside>

          {/* Main content */}
          <main className="flex-1 overflow-auto p-8">
            <Routes>
              <Route path="/" element={<ConfigPage config={config} onSaved={() => { void loadConfig() }} />} />
              <Route path="/ui" element={<ConfigPage config={config} onSaved={() => { void loadConfig() }} />} />
              <Route path="/ui/" element={<ConfigPage config={config} onSaved={() => { void loadConfig() }} />} />
              <Route path="/ui/config" element={<ConfigPage config={config} onSaved={() => { void loadConfig() }} />} />
              <Route path="/ui/clusters" element={<ClustersPage />} />
              <Route path="/ui/usage" element={<UsagePage />} />
            </Routes>
          </main>
        </div>
      </div>
    </BrowserRouter>
  )
}
