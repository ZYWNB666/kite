const CURRENT_CLUSTER_SESSION_KEY = 'current-cluster'
const CURRENT_CLUSTER_URL_KEY = 'cluster'
const CURRENT_CLUSTER_HEADER_KEY = 'x-cluster-name'

export function getClusterFromUrl() {
  if (typeof window === 'undefined') {
    return null
  }

  return new URLSearchParams(window.location.search).get(
    CURRENT_CLUSTER_URL_KEY
  )
}

export function getCurrentCluster() {
  const urlCluster = getClusterFromUrl()
  if (urlCluster) {
    return urlCluster
  }

  if (typeof sessionStorage === 'undefined') {
    return null
  }
  return sessionStorage.getItem(CURRENT_CLUSTER_SESSION_KEY)
}

export function setCurrentCluster(clusterName: string) {
  sessionStorage.setItem(CURRENT_CLUSTER_SESSION_KEY, clusterName)
}

export function clearCurrentCluster() {
  sessionStorage.removeItem(CURRENT_CLUSTER_SESSION_KEY)
}

export function appendCurrentClusterParam(
  params: URLSearchParams,
  clusterName?: string | null
) {
  const currentCluster = clusterName ?? getCurrentCluster()
  if (currentCluster) {
    params.set(CURRENT_CLUSTER_HEADER_KEY, currentCluster)
  }
}

export function appendCurrentClusterHeader(
  headers: Record<string, string> | Headers,
  clusterName?: string | null
) {
  const currentCluster = clusterName ?? getCurrentCluster()
  if (currentCluster) {
    if (headers instanceof Headers) {
      headers.set(CURRENT_CLUSTER_HEADER_KEY, currentCluster)
    } else {
      headers[CURRENT_CLUSTER_HEADER_KEY] = currentCluster
    }
  }
}

export function getClusterScopedStorageKey(key: string) {
  const currentCluster = getCurrentCluster()
  return `${currentCluster || ''}${key}`
}

export function getClusterQueryKey(...parts: unknown[]) {
  return ['cluster', getCurrentCluster() || '', ...parts]
}

export function withClusterHref(
  href: string,
  clusterName: string,
  baseHref = window.location.href
) {
  const url = new URL(href, baseHref)
  const baseUrl = new URL(baseHref)

  if (url.origin !== baseUrl.origin) {
    return href
  }

  url.searchParams.set(CURRENT_CLUSTER_URL_KEY, clusterName)
  return `${url.pathname}${url.search}${url.hash}`
}

export function withClusterRequestHref(
  href: string,
  clusterName = getCurrentCluster(),
  baseHref = window.location.href
) {
  if (!clusterName) {
    return href
  }

  const url = new URL(href, baseHref)
  const baseUrl = new URL(baseHref)
  if (url.origin !== baseUrl.origin) {
    return href
  }

  url.searchParams.set(CURRENT_CLUSTER_HEADER_KEY, clusterName)
  return `${url.pathname}${url.search}${url.hash}`
}
