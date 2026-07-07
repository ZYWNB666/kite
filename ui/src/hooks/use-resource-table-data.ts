import { useMemo } from 'react'

import { ResourceType } from '@/types/api'
import { useResources, useResourcesWatch } from '@/lib/api'

interface UseResourceTableDataOptions {
  resourceName: string
  resourceType?: ResourceType
  namespace?: string
  /** When provided and contains fewer namespaces than the API query (i.e.
   * multiple namespaces selected), the hook fetches `_all` and filters the
   * result client-side to only these namespaces. */
  clientFilterNamespaces?: string[]
  useSSE: boolean
  refreshInterval: number
}

export function useResourceTableData<T>({
  resourceName,
  resourceType,
  namespace,
  clientFilterNamespaces,
  useSSE,
  refreshInterval,
}: UseResourceTableDataOptions) {
  const resolvedResourceType = (resourceType ??
    (resourceName.toLowerCase() as ResourceType)) as ResourceType

  const query = useResources(resolvedResourceType, namespace, {
    refreshInterval: useSSE ? 0 : refreshInterval,
    reduce: true,
    disable: useSSE,
  })

  const watch = useResourcesWatch(resolvedResourceType, namespace, {
    reduce: true,
    enabled: useSSE,
  })

  const rawSSEData = watch.data
  const rawData = useSSE ? rawSSEData : query.data

  // When multiple namespaces are selected we fetch _all and filter here.
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
    isLoading: useSSE ? watch.isLoading : query.isLoading,
    isError: useSSE ? Boolean(watch.error) : query.isError,
    error: (useSSE ? watch.error : query.error) as Error | null,
    refetch: useSSE ? watch.refetch : query.refetch,
    isConnected: watch.isConnected,
  }
}
