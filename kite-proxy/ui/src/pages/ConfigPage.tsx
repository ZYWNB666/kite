import { useState } from 'react'
import { toast } from 'sonner'
import { Save, Eye, EyeOff } from 'lucide-react'
import { saveConfig, listClusters } from '../api/client'
import type { Config } from '../types'

interface Props {
  config: Config | null
  onSaved: () => void
}

export default function ConfigPage({ config, onSaved }: Props) {
  const [kiteURL, setKiteURL] = useState(config?.kiteURL ?? '')
  const [apiKey, setApiKey] = useState('')
  const [showKey, setShowKey] = useState(false)
  const [saving, setSaving] = useState(false)

  async function handleSave(e: React.FormEvent) {
    e.preventDefault()
    if (!kiteURL.trim() || !apiKey.trim()) {
      toast.error('Please fill in all fields')
      return
    }
    setSaving(true)
    try {
      await saveConfig(kiteURL.trim(), apiKey.trim())
      toast.success('Configuration saved')
      onSaved()
    } catch (err) {
      toast.error(String(err))
    } finally {
      setSaving(false)
    }
  }

  async function handleTest() {
    if (!kiteURL.trim() || !apiKey.trim()) {
      toast.error('Please fill in Kite URL and API key first')
      return
    }
    setSaving(true)
    try {
      // Temporarily save then test by fetching clusters.
      await saveConfig(kiteURL.trim(), apiKey.trim())
      const result = await listClusters()
      toast.success(`Connection successful – ${result.clusters.length} cluster(s) available`)
      onSaved()
    } catch (err) {
      toast.error(`Connection failed: ${String(err)}`)
    } finally {
      setSaving(false)
    }
  }

  // Reload current config from server on mount.
  // (config prop is already loaded by the parent.)

  return (
    <div className="max-w-xl mx-auto">
      <h1 className="text-2xl font-bold mb-2">Configuration</h1>
      <p className="text-gray-500 mb-6 text-sm">
        Connect kite-proxy to your kite server. The API key is stored only in
        memory and is never written to disk.
      </p>

      {config?.configured && (
        <div className="mb-4 rounded-md bg-green-50 border border-green-200 px-4 py-3 text-sm text-green-700 flex items-center gap-2">
          <span className="inline-block h-2 w-2 rounded-full bg-green-500" />
          Connected to <strong>{config.kiteURL}</strong>
        </div>
      )}

      <form onSubmit={(e) => { void handleSave(e) }} className="space-y-4">
        <div>
          <label className="block text-sm font-medium mb-1">Kite Server URL</label>
          <input
            type="url"
            required
            placeholder="https://kite.example.com"
            value={kiteURL}
            onChange={(e) => setKiteURL(e.target.value)}
            className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>

        <div>
          <label className="block text-sm font-medium mb-1">API Key</label>
          <div className="relative">
            <input
              type={showKey ? 'text' : 'password'}
              required
              placeholder={config?.apiKeyMasked ? `Current: ${config.apiKeyMasked}` : 'kite…'}
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              className="w-full rounded-md border border-gray-300 px-3 py-2 pr-10 text-sm shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
            <button
              type="button"
              onClick={() => setShowKey(!showKey)}
              className="absolute right-2 top-2 text-gray-400 hover:text-gray-600"
            >
              {showKey ? <EyeOff size={16} /> : <Eye size={16} />}
            </button>
          </div>
          <p className="text-xs text-gray-400 mt-1">
            Leave blank to keep the existing key.
          </p>
        </div>

        <div className="flex gap-3">
          <button
            type="submit"
            disabled={saving}
            className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow hover:bg-blue-700 disabled:opacity-50"
          >
            <Save size={16} />
            {saving ? 'Saving…' : 'Save'}
          </button>
          <button
            type="button"
            onClick={() => { void handleTest() }}
            disabled={saving}
            className="inline-flex items-center gap-2 rounded-md border border-gray-300 px-4 py-2 text-sm font-medium shadow hover:bg-gray-50 disabled:opacity-50"
          >
            Test Connection
          </button>
        </div>
      </form>
    </div>
  )
}
