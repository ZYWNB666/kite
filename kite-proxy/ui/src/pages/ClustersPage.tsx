import { useState, useEffect } from 'react'
import { toast } from 'sonner'
import { RefreshCw, Zap, Trash2 } from 'lucide-react'
import { listClusters, prewarmCluster, clearCache } from '../api/client'
import type { ClusterInfo } from '../types'

export default function ClustersPage() {
  const [clusters, setClusters] = useState<ClusterInfo[]>([])
  const [loading, setLoading] = useState(false)
  const [actionCluster, setActionCluster] = useState<string | null>(null)

  async function load() {
    setLoading(true)
    try {
      const res = await listClusters()
      setClusters(res.clusters ?? [])
    } catch (err) {
      toast.error(String(err))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  async function handlePrewarm(name: string) {
    setActionCluster(name)
    try {
      await prewarmCluster(name)
      toast.success(`Kubeconfig for "${name}" loaded into memory`)
      await load()
    } catch (err) {
      toast.error(String(err))
    } finally {
      setActionCluster(null)
    }
  }

  async function handleClearCache() {
    try {
      await clearCache()
      toast.success('All kubeconfigs cleared from memory')
      await load()
    } catch (err) {
      toast.error(String(err))
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">Clusters</h1>
          <p className="text-sm text-gray-500 mt-1">
            Clusters you have proxy access to via kite. Kubeconfigs are stored
            only in memory.
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => { void load() }}
            disabled={loading}
            className="inline-flex items-center gap-1 rounded-md border px-3 py-1.5 text-sm hover:bg-gray-50 disabled:opacity-50"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
            Refresh
          </button>
          <button
            onClick={() => { void handleClearCache() }}
            className="inline-flex items-center gap-1 rounded-md border border-red-200 px-3 py-1.5 text-sm text-red-600 hover:bg-red-50"
          >
            <Trash2 size={14} />
            Clear Cache
          </button>
        </div>
      </div>

      {clusters.length === 0 && !loading && (
        <div className="rounded-md border border-dashed p-8 text-center text-sm text-gray-400">
          No clusters found. Make sure kite is configured and you have proxy
          permissions.
        </div>
      )}

      <div className="space-y-3">
        {clusters.map((cl) => (
          <div
            key={cl.name}
            className="flex items-center justify-between rounded-lg border bg-white px-4 py-3 shadow-sm"
          >
            <div className="flex items-center gap-3">
              <span
                className={`inline-block h-2.5 w-2.5 rounded-full ${
                  cl.cached ? 'bg-green-500' : 'bg-gray-300'
                }`}
              />
              <div>
                <p className="font-medium text-sm">{cl.name}</p>
                <p className="text-xs text-gray-400">
                  {cl.cached ? 'Kubeconfig cached in memory' : 'Not cached – will be fetched on first request'}
                </p>
              </div>
            </div>
            <button
              onClick={() => { void handlePrewarm(cl.name) }}
              disabled={actionCluster === cl.name}
              className="inline-flex items-center gap-1 rounded-md border px-3 py-1.5 text-xs hover:bg-gray-50 disabled:opacity-50"
            >
              <Zap size={12} />
              {actionCluster === cl.name ? 'Loading…' : cl.cached ? 'Reload' : 'Prewarm'}
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}
