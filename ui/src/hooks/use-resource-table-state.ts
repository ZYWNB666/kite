import { useCallback, useEffect, useState } from 'react'
import {
  ColumnFiltersState,
  PaginationState,
  RowSelectionState,
  SortingState,
} from '@tanstack/react-table'

import { getClusterScopedStorageKey } from '@/lib/current-cluster'

interface UseResourceTableStateOptions {
  resourceName: string
  clusterScope: boolean
  defaultHiddenColumns: string[]
}

function readStoredJSON<T>(storage: Storage, key: string, fallback: T): T {
  const value = storage.getItem(key)
  if (!value) {
    return fallback
  }

  try {
    return JSON.parse(value) as T
  } catch {
    return fallback
  }
}

export function useResourceTableState({
  resourceName,
  clusterScope,
  defaultHiddenColumns,
}: UseResourceTableStateOptions) {
  const [sorting, setSorting] = useState<SortingState>([])
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(() =>
    readStoredJSON(
      sessionStorage,
      getClusterScopedStorageKey(`-${resourceName}-columnFilters`),
      []
    )
  )
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState<string>(() => {
    return (
      sessionStorage.getItem(
        getClusterScopedStorageKey(`-${resourceName}-searchQuery`)
      ) || ''
    )
  })
  const [columnVisibility, setColumnVisibility] = useState<
    Record<string, boolean>
  >(() => {
    const savedVisibility = readStoredJSON<Record<string, boolean> | null>(
      localStorage,
      getClusterScopedStorageKey(`-${resourceName}-columnVisibility`),
      null
    )
    if (savedVisibility) {
      return savedVisibility
    }

    const initialVisibility: Record<string, boolean> = {}
    defaultHiddenColumns.forEach((columnId) => {
      initialVisibility[columnId] = false
    })
    return initialVisibility
  })
  const [pagination, setPagination] = useState<PaginationState>(() => {
    const savedPageSize = sessionStorage.getItem(
      getClusterScopedStorageKey(`-${resourceName}-pageSize`)
    )
    return {
      pageIndex: 0,
      pageSize: savedPageSize ? Number(savedPageSize) : 20,
    }
  })
  const [refreshInterval, setRefreshInterval] = useState(5000)
  const [selectedNamespaces, setSelectedNamespaces] = useState<
    string[]
  >(() => {
    if (clusterScope) return []
    const stored = localStorage.getItem(
      getClusterScopedStorageKey('selectedNamespaces')
    )
    if (stored) {
      try {
        const parsed = JSON.parse(stored)
        if (Array.isArray(parsed) && parsed.length > 0) return parsed
      } catch {
        // fall through to legacy migration
      }
    }
    // Migrate legacy single-value storage
    const legacy = localStorage.getItem(
      getClusterScopedStorageKey('selectedNamespace')
    )
    return legacy ? [legacy] : ['default']
  })
  const [useSSE, setUseSSE] = useState(false)

  // The namespace to send to the API. When exactly one namespace is selected
  // (and it's not _all), we query that specific namespace for efficiency.
  // When multiple or _all is selected, we query _all and filter on the client.
  const effectiveNamespace = clusterScope
    ? undefined
    : selectedNamespaces.length === 1 && selectedNamespaces[0] !== '_all'
      ? selectedNamespaces[0]
      : '_all'

  useEffect(() => {
    if (clusterScope || selectedNamespaces.length > 0) {
      return
    }

    const stored = localStorage.getItem(
      getClusterScopedStorageKey('selectedNamespaces')
    )
    if (stored) {
      try {
        const parsed = JSON.parse(stored)
        if (Array.isArray(parsed) && parsed.length > 0) {
          setSelectedNamespaces(parsed)
          return
        }
      } catch {
        // ignore
      }
    }
    setSelectedNamespaces(['default'])
  }, [clusterScope, selectedNamespaces])

  useEffect(() => {
    const storageKey = getClusterScopedStorageKey(
      `-${resourceName}-searchQuery`
    )
    if (searchQuery) {
      sessionStorage.setItem(storageKey, searchQuery)
      return
    }

    sessionStorage.removeItem(storageKey)
  }, [resourceName, searchQuery])

  useEffect(() => {
    localStorage.setItem(
      getClusterScopedStorageKey(`-${resourceName}-columnVisibility`),
      JSON.stringify(columnVisibility)
    )
  }, [columnVisibility, resourceName])

  useEffect(() => {
    sessionStorage.setItem(
      getClusterScopedStorageKey(`-${resourceName}-pageSize`),
      pagination.pageSize.toString()
    )
  }, [pagination.pageSize, resourceName])

  useEffect(() => {
    const storageKey = getClusterScopedStorageKey(
      `-${resourceName}-columnFilters`
    )
    if (columnFilters.length > 0) {
      sessionStorage.setItem(storageKey, JSON.stringify(columnFilters))
      return
    }

    sessionStorage.removeItem(storageKey)
  }, [columnFilters, resourceName])

  useEffect(() => {
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }, [columnFilters, searchQuery])

  const handleNamespaceChange = useCallback((value: string[]) => {
    const normalized = value.length === 0 ? ['default'] : value
    localStorage.setItem(
      getClusterScopedStorageKey('selectedNamespaces'),
      JSON.stringify(normalized)
    )
    // Also update legacy key for components still reading it
    localStorage.setItem(
      getClusterScopedStorageKey('selectedNamespace'),
      normalized.includes('_all') ? '_all' : normalized[0]
    )
    setSelectedNamespaces(normalized)
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    setSearchQuery('')
    window.dispatchEvent(
      new CustomEvent('kite:namespace-change', {
        detail: { namespaces: normalized },
      })
    )
  }, [])

  const handleUseSSEChange = useCallback((pressed: boolean) => {
    setUseSSE(pressed)
    setRefreshInterval((current) => {
      if (pressed) {
        return 0
      }
      if (current === 0) {
        return 5000
      }
      return current
    })
  }, [])

  const handleRefreshIntervalChange = useCallback((value: number) => {
    setRefreshInterval(value)
    if (value > 0) {
      setUseSSE(false)
    }
  }, [])

  return {
    sorting,
    setSorting,
    columnFilters,
    setColumnFilters,
    rowSelection,
    setRowSelection,
    deleteDialogOpen,
    setDeleteDialogOpen,
    searchQuery,
    setSearchQuery,
    columnVisibility,
    setColumnVisibility,
    pagination,
    setPagination,
    refreshInterval,
    setRefreshInterval,
    selectedNamespaces,
    effectiveNamespace,
    useSSE,
    handleNamespaceChange,
    handleUseSSEChange,
    handleRefreshIntervalChange,
  }
}
