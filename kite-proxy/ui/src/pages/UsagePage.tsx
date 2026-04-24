import { useState } from 'react'
import { toast } from 'sonner'
import { Download, Copy, Terminal } from 'lucide-react'
import { downloadKubeconfig } from '../api/client'

export default function UsagePage() {
  const [kubeconfig, setKubeconfig] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const port = window.location.port || '8090'

  async function handleDownload() {
    setLoading(true)
    try {
      const yaml = await downloadKubeconfig()
      setKubeconfig(yaml)
      // Also trigger browser download.
      const blob = new Blob([yaml], { type: 'application/x-yaml' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'kubeconfig-kite-proxy.yaml'
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      toast.error(String(err))
    } finally {
      setLoading(false)
    }
  }

  async function handleCopy() {
    if (!kubeconfig) return
    try {
      await navigator.clipboard.writeText(kubeconfig)
      toast.success('Kubeconfig copied to clipboard')
    } catch {
      toast.error('Failed to copy')
    }
  }

  return (
    <div>
      <h1 className="text-2xl font-bold mb-2">Usage</h1>
      <p className="text-sm text-gray-500 mb-6">
        Use kite-proxy as a transparent Kubernetes API endpoint without exposing your kubeconfigs.
      </p>

      {/* How it works */}
      <section className="mb-8">
        <h2 className="text-lg font-semibold mb-3">How it works</h2>
        <ol className="list-decimal list-inside space-y-2 text-sm text-gray-700">
          <li>Configure kite-proxy with your kite server URL and API key.</li>
          <li>kite-proxy fetches kubeconfigs from kite (kept <strong>only in memory</strong>).</li>
          <li>Generate a local kubeconfig that points <code>kubectl</code> at kite-proxy.</li>
          <li>Run <code>kubectl</code> commands normally – kite-proxy forwards them securely.</li>
        </ol>
      </section>

      {/* Generated kubeconfig */}
      <section className="mb-8">
        <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
          <Terminal size={18} />
          Generate Local Kubeconfig
        </h2>
        <p className="text-sm text-gray-500 mb-4">
          Download a kubeconfig that configures <code>kubectl</code> to use this
          kite-proxy instance (port&nbsp;{port}).
        </p>
        <div className="flex gap-3">
          <button
            onClick={() => { void handleDownload() }}
            disabled={loading}
            className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow hover:bg-blue-700 disabled:opacity-50"
          >
            <Download size={16} />
            {loading ? 'Generating…' : 'Download kubeconfig'}
          </button>
          {kubeconfig && (
            <button
              onClick={() => { void handleCopy() }}
              className="inline-flex items-center gap-2 rounded-md border px-4 py-2 text-sm shadow hover:bg-gray-50"
            >
              <Copy size={16} />
              Copy to clipboard
            </button>
          )}
        </div>

        {kubeconfig && (
          <pre className="mt-4 rounded-md bg-gray-900 text-green-400 text-xs p-4 overflow-auto max-h-64">
            {kubeconfig}
          </pre>
        )}
      </section>

      {/* CLI examples */}
      <section>
        <h2 className="text-lg font-semibold mb-3">kubectl Examples</h2>
        <div className="space-y-3 text-sm">
          <CodeBlock label="Use generated kubeconfig">
            {`export KUBECONFIG=kubeconfig-kite-proxy.yaml
kubectl get pods -A`}
          </CodeBlock>
          <CodeBlock label="Direct proxy URL (single cluster)">
            {`kubectl --server=http://localhost:${port}/proxy/<cluster-name> \\
        --insecure-skip-tls-verify \\
        get pods -A`}
          </CodeBlock>
        </div>
      </section>
    </div>
  )
}

function CodeBlock({ label, children }: { label: string; children: string }) {
  function copy() {
    void navigator.clipboard.writeText(children).then(
      () => toast.success('Copied'),
      () => toast.error('Failed to copy'),
    )
  }

  return (
    <div>
      <p className="text-xs font-medium text-gray-500 mb-1">{label}</p>
      <div className="relative group">
        <pre className="rounded-md bg-gray-900 text-green-400 text-xs p-4 overflow-auto">{children}</pre>
        <button
          onClick={copy}
          className="absolute top-2 right-2 hidden group-hover:inline-flex items-center gap-1 rounded bg-white/10 px-2 py-1 text-xs text-white hover:bg-white/20"
        >
          <Copy size={12} />
          Copy
        </button>
      </div>
    </div>
  )
}
