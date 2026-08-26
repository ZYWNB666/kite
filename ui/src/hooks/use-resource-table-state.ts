import { useCallback, useEffect, useState } from 'react'
import {
  ColumnFiltersState,
  PaginationState,
  RowSelectionState,
  SortingState,
} from '@tanstack/react-table'
import { useSearchParams } from 'react-router-dom'

import { getClusterScopedStorageKey } from '@/lib/current-cluster'
import {
  getCurrentNamespaces,
  getNamespacesFromUrl,
  setCurrentNamespaces,
  setNamespacesInSearchParams,
} from '@/lib/current-namespace'

interface UseResourceTableStateOptions {
  resourceName: string
  clusterScope: boolean
  defaultHiddenColumns: string[]
  watchSupported: boolean
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
  watchSupported,
}: UseResourceTableStateOptions) {
  const [searchParams, setSearchParams] = useSearchParams()
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
  const [refreshInterval, setRefreshInterval] = useState(
    watchSupported ? 0 : 5000
  )
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>(() => {
    if (clusterScope) return []
    const urlNamespaces = getNamespacesFromUrl(searchParams.toString())
    if (urlNamespaces.length > 0) return urlNamespaces
    return getCurrentNamespaces()
  })
  const [useSSE, setUseSSE] = useState(watchSupported)
  const [useRegex, setUseRegex] = useState(false)

  // The namespace to send to the API. When exactly one namespace is selected
  // (and it's not _all), we query that specific namespace for efficiency.
  // When multiple or _all is selected, we query _all and filter on the client.
  const effectiveNamespace = clusterScope
    ? undefined
    : selectedNamespaces.length === 1 && selectedNamespaces[0] !== '_all'
      ? selectedNamespaces[0]
      : '_all'

  useEffect(() => {
    if (clusterScope) return

    const urlNamespaces = getNamespacesFromUrl(searchParams.toString())
    if (urlNamespaces.length === 0) {
      setSearchParams(
        (current) => setNamespacesInSearchParams(current, selectedNamespaces),
        { replace: true }
      )
      return
    }

    const normalized = setCurrentNamespaces(urlNamespaces)
    setSelectedNamespaces((current) =>
      current.length === normalized.length &&
      current.every((namespace, index) => namespace === normalized[index])
        ? current
        : normalized
    )
    window.dispatchEvent(
      new CustomEvent('kite:namespace-change', {
        detail: { namespaces: normalized },
      })
    )
  }, [clusterScope, searchParams, selectedNamespaces, setSearchParams])

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

  const handleNamespaceChange = useCallback(
    (value: string[]) => {
      const normalized = setCurrentNamespaces(value)
      setSelectedNamespaces(normalized)
      setSearchParams(
        (current) => setNamespacesInSearchParams(current, normalized),
        { replace: true }
      )
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
      setSearchQuery('')
      window.dispatchEvent(
        new CustomEvent('kite:namespace-change', {
          detail: { namespaces: normalized },
        })
      )
    },
    [setSearchParams]
  )

  const handleUseSSEChange = useCallback(
    (pressed: boolean) => {
      const enabled = pressed && watchSupported
      setUseSSE(enabled)
      setRefreshInterval((current) => {
        if (enabled) {
          return 0
        }
        if (current === 0) {
          return 5000
        }
        return current
      })
    },
    [watchSupported]
  )

  const handleUseRegexChange = useCallback((pressed: boolean) => {
    setUseRegex(pressed)
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
    useRegex,
    handleNamespaceChange,
    handleUseSSEChange,
    handleUseRegexChange,
    handleRefreshIntervalChange,
  }
}
