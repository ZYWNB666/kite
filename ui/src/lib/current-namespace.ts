import { getClusterScopedStorageKey } from './current-cluster'

const CURRENT_NAMESPACES_SESSION_SUFFIX = 'selectedNamespaces'
const LEGACY_CURRENT_NAMESPACE_SESSION_SUFFIX = 'selectedNamespace'
export const CURRENT_NAMESPACE_URL_KEY = 'namespace'

function normalizeNamespaces(namespaces: string[]) {
  const normalized = [...new Set(namespaces.filter(Boolean))]
  return normalized.includes('_all') ? ['_all'] : normalized
}

function readStoredNamespaces(storage: Storage) {
  const stored = storage.getItem(
    getClusterScopedStorageKey(CURRENT_NAMESPACES_SESSION_SUFFIX)
  )
  if (stored) {
    try {
      const parsed = JSON.parse(stored)
      if (Array.isArray(parsed)) {
        const normalized = normalizeNamespaces(
          parsed.filter((value): value is string => typeof value === 'string')
        )
        if (normalized.length > 0) {
          return normalized
        }
      }
    } catch {
      // Fall through to the legacy single-value key.
    }
  }

  const legacy = storage.getItem(
    getClusterScopedStorageKey(LEGACY_CURRENT_NAMESPACE_SESSION_SUFFIX)
  )
  return legacy ? normalizeNamespaces([legacy]) : []
}

export function getNamespacesFromUrl(search = window.location.search) {
  return normalizeNamespaces(
    new URLSearchParams(search).getAll(CURRENT_NAMESPACE_URL_KEY)
  )
}

export function getCurrentNamespaces() {
  const urlNamespaces = getNamespacesFromUrl()
  if (urlNamespaces.length > 0) {
    return urlNamespaces
  }

  const sessionNamespaces = readStoredNamespaces(sessionStorage)
  if (sessionNamespaces.length > 0) {
    return sessionNamespaces
  }

  // One-way compatibility read for versions that stored namespace selection
  // in shared localStorage. New selections are persisted only per tab.
  const legacyNamespaces = readStoredNamespaces(localStorage)
  return legacyNamespaces.length > 0 ? legacyNamespaces : ['default']
}

export function setCurrentNamespaces(namespaces: string[]) {
  const normalized = normalizeNamespaces(namespaces)
  const current = normalized.length > 0 ? normalized : ['default']
  sessionStorage.setItem(
    getClusterScopedStorageKey(CURRENT_NAMESPACES_SESSION_SUFFIX),
    JSON.stringify(current)
  )
  sessionStorage.setItem(
    getClusterScopedStorageKey(LEGACY_CURRENT_NAMESPACE_SESSION_SUFFIX),
    current.includes('_all') ? '_all' : current[0]
  )
  return current
}

export function setNamespacesInSearchParams(
  searchParams: URLSearchParams,
  namespaces: string[]
) {
  const normalized = normalizeNamespaces(namespaces)
  const current = normalized.length > 0 ? normalized : ['default']
  searchParams.delete(CURRENT_NAMESPACE_URL_KEY)
  current.forEach((namespace) => {
    searchParams.append(CURRENT_NAMESPACE_URL_KEY, namespace)
  })
  return searchParams
}

export function withCurrentNamespacesHref(
  href: string,
  namespaces = getCurrentNamespaces(),
  baseHref = window.location.href
) {
  const url = new URL(href, baseHref)
  const baseUrl = new URL(baseHref)
  if (url.origin !== baseUrl.origin || url.pathname.includes('/api/v1/')) {
    return href
  }

  setNamespacesInSearchParams(url.searchParams, namespaces)
  return `${url.pathname}${url.search}${url.hash}`
}
