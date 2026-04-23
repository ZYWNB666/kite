import type { Config, ClusterInfo, Status } from '../types'

const BASE = '/api'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...init?.headers },
    ...init,
  })
  if (!res.ok) {
    const text = await res.text()
    let msg = text
    try {
      msg = JSON.parse(text)?.error ?? text
    } catch {
      /* empty */
    }
    throw new Error(msg || `HTTP ${res.status}`)
  }
  return res.json() as Promise<T>
}

export async function getConfig(): Promise<Config> {
  return request<Config>('/config')
}

export async function saveConfig(kiteURL: string, apiKey: string): Promise<void> {
  await request('/config', {
    method: 'POST',
    body: JSON.stringify({ kiteURL, apiKey }),
  })
}

export async function listClusters(): Promise<{ clusters: ClusterInfo[] }> {
  return request<{ clusters: ClusterInfo[] }>('/clusters')
}

export async function getStatus(): Promise<Status> {
  return request<Status>('/status')
}

export async function clearCache(): Promise<void> {
  await request('/cache', { method: 'DELETE' })
}

export async function prewarmCluster(name: string): Promise<void> {
  await request(`/cache/${encodeURIComponent(name)}`, { method: 'POST' })
}

export async function downloadKubeconfig(): Promise<string> {
  const res = await fetch(`${BASE}/kubeconfig`)
  if (!res.ok) {
    const text = await res.text()
    let msg = text
    try {
      msg = JSON.parse(text)?.error ?? text
    } catch {
      /* empty */
    }
    throw new Error(msg || `HTTP ${res.status}`)
  }
  return res.text()
}
