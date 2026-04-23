/** API types for kite-proxy */

export interface Config {
  kiteURL: string
  apiKeyMasked: string
  configured: boolean
}

export interface ClusterInfo {
  name: string
  cached: boolean
}

export interface Status {
  status: string
  configured: boolean
  cachedClusters: string[]
}
