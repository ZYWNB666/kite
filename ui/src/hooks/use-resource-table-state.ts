import { useCallback, useEffect, useRef, useState } from 'react'
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
import {
  areResourceFiltersEqual,
  hasResourceFilters,
  readResourceFilters,
  RESOURCE_SEARCH_MODE_KEY,
  RESOURCE_SEARCH_MODE_REGEX,
  RESOURCE_SEARCH_QUERY_KEY,
  setResourceFiltersInSearchParams,
  setResourceQueryInSearchParams,
  setResourceSearchModeInSearchParams,
} from '@/lib/resource-table-url-state'

interface UseResourceTableStateOptions {
  resourceName: string
  clusterScope: boolean
  defaultHiddenColumns: string[]
  watchSupported: boolean
  filterableColumnIds: string[]
}

type ColumnFiltersUpdater =
  | ColumnFiltersState
  | ((current: ColumnFiltersState) => ColumnFiltersState)

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
  filterableColumnIds,
}: UseResourceTableStateOptions) {
  const [searchParams, setSearchParams] = useSearchParams()
  const searchParamsValue = searchParams.toString()
  const namespaceUrlValue =
    getNamespacesFromUrl(searchParamsValue).join('\u0000')
  const filterableColumnIdsValue = filterableColumnIds.join('\u0000')
  const [sorting, setSorting] = useState<SortingState>([])
  const [columnFilters, setColumnFiltersState] = useState<ColumnFiltersState>(
    () => {
      if (hasResourceFilters(searchParams)) {
        return readResourceFilters(searchParams, filterableColumnIds)
      }
      return readStoredJSON(
        sessionStorage,
        getClusterScopedStorageKey(`-${resourceName}-columnFilters`),
        []
      )
    }
  )
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [searchQuery, setSearchQueryState] = useState<string>(() => {
    if (searchParams.has(RESOURCE_SEARCH_QUERY_KEY)) {
      return searchParams.get(RESOURCE_SEARCH_QUERY_KEY) ?? ''
    }
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
    const urlNamespaces = namespaceUrlValue
      ? namespaceUrlValue.split('\u0000')
      : []
    if (urlNamespaces.length > 0) return urlNamespaces
    return getCurrentNamespaces()
  })
  const [useSSE, setUseSSE] = useState(watchSupported)
  const [useRegex, setUseRegex] = useState(
    searchParams.get(RESOURCE_SEARCH_MODE_KEY) === RESOURCE_SEARCH_MODE_REGEX
  )
  const resourceSearchInitialized = useRef(false)
  const initialSearchQuery = useRef(searchQuery)
  const initialColumnFilters = useRef(columnFilters)

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

    const urlNamespaces = namespaceUrlValue
      ? namespaceUrlValue.split('\u0000')
      : []
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
  }, [clusterScope, namespaceUrlValue, selectedNamespaces, setSearchParams])

  useEffect(() => {
    const currentParams = new URLSearchParams(searchParamsValue)
    const allowedColumnIds = filterableColumnIdsValue
      ? filterableColumnIdsValue.split('\u0000')
      : []

    if (!resourceSearchInitialized.current) {
      resourceSearchInitialized.current = true
      let shouldCanonicalize = false

      if (
        !currentParams.has(RESOURCE_SEARCH_QUERY_KEY) &&
        initialSearchQuery.current
      ) {
        setResourceQueryInSearchParams(
          currentParams,
          initialSearchQuery.current
        )
        shouldCanonicalize = true
      }
      if (
        !hasResourceFilters(currentParams) &&
        initialColumnFilters.current.length > 0
      ) {
        setResourceFiltersInSearchParams(
          currentParams,
          initialColumnFilters.current,
          allowedColumnIds
        )
        shouldCanonicalize = true
      }

      if (shouldCanonicalize) {
        setSearchParams(currentParams, { replace: true })
      }
      return
    }

    const urlQuery = currentParams.get(RESOURCE_SEARCH_QUERY_KEY) ?? ''
    setSearchQueryState((current) =>
      current === urlQuery ? current : urlQuery
    )

    const urlUseRegex =
      currentParams.get(RESOURCE_SEARCH_MODE_KEY) === RESOURCE_SEARCH_MODE_REGEX
    setUseRegex((current) => (current === urlUseRegex ? current : urlUseRegex))

    const urlFilters = hasResourceFilters(currentParams)
      ? readResourceFilters(currentParams, allowedColumnIds)
      : []
    setColumnFiltersState((current) =>
      areResourceFiltersEqual(current, urlFilters) ? current : urlFilters
    )
  }, [filterableColumnIdsValue, searchParamsValue, setSearchParams])

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
      window.dispatchEvent(
        new CustomEvent('kite:namespace-change', {
          detail: { namespaces: normalized },
        })
      )
    },
    [setSearchParams]
  )

  const setSearchQuery = useCallback(
    (value: string) => {
      setSearchQueryState(value)
      setSearchParams(
        (current) => setResourceQueryInSearchParams(current, value),
        { replace: true }
      )
    },
    [setSearchParams]
  )

  const setColumnFilters = useCallback(
    (updater: ColumnFiltersUpdater) => {
      const next =
        typeof updater === 'function' ? updater(columnFilters) : updater
      setColumnFiltersState(next)
      setSearchParams(
        (current) =>
          setResourceFiltersInSearchParams(current, next, filterableColumnIds),
        { replace: true }
      )
    },
    [columnFilters, filterableColumnIds, setSearchParams]
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

  const handleUseRegexChange = useCallback(
    (pressed: boolean) => {
      setUseRegex(pressed)
      setSearchParams(
        (current) => setResourceSearchModeInSearchParams(current, pressed),
        { replace: true }
      )
    },
    [setSearchParams]
  )

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
