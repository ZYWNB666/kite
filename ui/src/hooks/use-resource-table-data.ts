import { useMemo } from 'react'

import { ResourceType } from '@/types/api'
import { useResources, useResourcesWatch } from '@/lib/api'

interface UseResourceTableDataOptions {
  resourceName: string
  resourceType?: ResourceType
  namespace?: string
  /** When multiple namespaces are selected, request only those namespaces
   * from `_all` and retain client-side filtering for backward compatibility. */
  clientFilterNamespaces?: string[]
  currentCluster?: string | null
  useSSE: boolean
  refreshInterval: number
}

export function useResourceTableData<T>({
  resourceName,
  resourceType,
  namespace,
  clientFilterNamespaces,
  currentCluster,
  useSSE,
  refreshInterval,
}: UseResourceTableDataOptions) {
  const resolvedResourceType = (resourceType ??
    (resourceName.toLowerCase() as ResourceType)) as ResourceType

  const watch = useResourcesWatch(resolvedResourceType, namespace, {
    reduce: true,
    enabled: useSSE,
    cluster: currentCluster,
    namespaces: clientFilterNamespaces,
  })

  const useQueryFallback = !useSSE || watch.isUnsupported
  const query = useResources(resolvedResourceType, namespace, {
    refreshInterval: useQueryFallback ? refreshInterval || 5000 : 0,
    reduce: true,
    disable: !useQueryFallback,
    namespaces: clientFilterNamespaces,
  })

  const rawSSEData = watch.data
  // Preserve the last query result while the initial watch snapshot is being
  // established, avoiding an empty-table flash during mode changes.
  const rawData = useSSE
    ? (rawSSEData ?? query.data)
    : (query.data ?? rawSSEData)
  const isUsingWatch = useSSE && !watch.isUnsupported

  // Keep client-side filtering so this UI remains compatible with older APIs.
  const data = useMemo(() => {
    if (!rawData) return undefined
    if (
      !clientFilterNamespaces ||
      clientFilterNamespaces.length === 0 ||
      clientFilterNamespaces.includes('_all')
    ) {
      return rawData as T[] | undefined
    }
    const nsSet = new Set(clientFilterNamespaces)
    return (rawData as T[]).filter((item) => {
      const meta = (item as { metadata?: { namespace?: string } })?.metadata
      return meta?.namespace != null && nsSet.has(meta.namespace)
    }) as T[]
  }, [rawData, clientFilterNamespaces])

  return {
    resourceType: resolvedResourceType,
    data,
    isLoading: isUsingWatch
      ? watch.isLoading
      : query.isLoading && rawData === undefined,
    isError: isUsingWatch
      ? Boolean(
          watch.error && rawSSEData === undefined && query.data === undefined
        )
      : Boolean(query.isError && rawData === undefined),
    error: (isUsingWatch ? watch.error : query.error) as Error | null,
    refetch: isUsingWatch ? watch.refetch : query.refetch,
    isConnected: isUsingWatch && watch.isConnected,
    watchUnavailable: watch.isUnsupported,
  }
}
