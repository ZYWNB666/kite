import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ColumnDef,
  FilterFn,
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { Box, Database } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ResourceType } from '@/types/api'
import { deleteResource } from '@/lib/api'
import { getResourceMetadata } from '@/lib/resource-catalog'
import { supportsResourceWatch } from '@/lib/resource-watch'
import { useCluster } from '@/hooks/use-cluster'
import { useResourceTableData } from '@/hooks/use-resource-table-data'
import { useResourceTableState } from '@/hooks/use-resource-table-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

import { ErrorMessage } from './error-message'
import { ResourceTableToolbar } from './resource-table-toolbar'
import { ResourceTableView } from './resource-table-view'

// The shared filter is intentionally reusable across unrelated resource shapes.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const multiSelectFilter: FilterFn<any> = (
  row,
  columnId,
  filterValue: string[]
) => {
  if (!filterValue?.length) return true
  return filterValue.includes(row.getValue(columnId) as string)
}
multiSelectFilter.autoRemove = (val: unknown) =>
  !val || (Array.isArray(val) && val.length === 0)

export interface ResourceTableProps<T> {
  resourceName: string
  resourceType?: ResourceType // Optional, used for fetching resources
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  columns: ColumnDef<T, any>[]
  clusterScope?: boolean // If true, don't show namespace selector
  searchQueryFilter?: (item: T, query: string) => boolean // Custom filter function
  showCreateButton?: boolean // If true, show create button
  onCreateClick?: () => void // Callback for create button click
  extraToolbars?: React.ReactNode[] // Additional toolbar components
  renderBatchActions?: (context: {
    selectedRows: T[]
    clearSelection: () => void
    refetch: () => Promise<unknown> | void
  }) => React.ReactNode
  defaultHiddenColumns?: string[] // Columns to hide by default
}

export function ResourceTable<T>({
  resourceName,
  resourceType,
  columns,
  clusterScope = false,
  searchQueryFilter,
  showCreateButton = false,
  onCreateClick,
  extraToolbars = [],
  renderBatchActions,
  defaultHiddenColumns = [],
}: ResourceTableProps<T>) {
  const { t } = useTranslation()
  const { currentCluster } = useCluster()
  const resolvedResourceType = (resourceType ??
    (resourceName.toLowerCase() as ResourceType)) as ResourceType
  const watchSupported = supportsResourceWatch(resolvedResourceType)
  const {
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
  } = useResourceTableState({
    resourceName,
    clusterScope,
    defaultHiddenColumns,
    watchSupported,
  })
  const [isDeleting, setIsDeleting] = useState(false)
  // When multiple namespaces are selected (not _all), fetch _all and filter
  // client-side. When a single namespace or _all is selected, no client filter.
  const clientFilterNamespaces =
    !clusterScope &&
    selectedNamespaces.length > 1 &&
    !selectedNamespaces.includes('_all')
      ? selectedNamespaces
      : undefined
  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
    isConnected,
    watchUnavailable,
  } = useResourceTableData<T>({
    resourceName,
    resourceType,
    namespace: effectiveNamespace,
    clientFilterNamespaces,
    currentCluster,
    useSSE,
    refreshInterval,
  })

  useEffect(() => {
    if (!useSSE || !watchUnavailable) return
    handleUseSSEChange(false)
    toast.warning(
      t('resourceTable.watchUnavailable', {
        defaultValue: 'Watch is unavailable; switched to 5s refresh.',
      }) + (error?.message ? ` (${error.message})` : '')
    )
  }, [error, handleUseSSEChange, t, useSSE, watchUnavailable])
  const displayResourceName = (() => {
    const resource = getResourceMetadata(resolvedResourceType)
    if (!resource) {
      return resourceName
    }
    if (resource.titleKey) {
      return t(resource.titleKey, {
        defaultValue:
          resource.shortLabel || resource.pluralLabel || resourceName,
      })
    }
    return resource.shortLabel || resource.pluralLabel || resourceName
  })()

  // Add namespace column when showing all namespaces
  const enhancedColumns = useMemo(() => {
    const selectColumn: ColumnDef<T> = {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={
            table.getIsAllPageRowsSelected() ||
            (table.getIsSomePageRowsSelected() && 'indeterminate')
          }
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label="Select all"
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label="Select row"
        />
      ),
      enableSorting: false,
      enableHiding: false,
    }

    const baseColumns = [selectColumn, ...columns]

    // Only add namespace column if not cluster scope, showing all namespaces,
    // and there isn't already a namespace column in the provided columns
    if (
      !clusterScope &&
      (selectedNamespaces.includes('_all') || selectedNamespaces.length > 1)
    ) {
      // Check if namespace column already exists in the provided columns
      const hasNamespaceColumn = columns.some((col) => {
        // Check if the column accesses namespace data
        if ('accessorKey' in col && col.accessorKey === 'metadata.namespace') {
          return true
        }
        if ('accessorFn' in col && col.id === 'namespace') {
          return true
        }
        return false
      })

      // Only add namespace column if it doesn't already exist
      if (!hasNamespaceColumn) {
        const namespaceColumn = {
          id: 'namespace',
          header: t('resourceTable.namespace'),
          accessorFn: (row: T) => {
            // Try to get namespace from metadata.namespace
            const metadata = (row as { metadata?: { namespace?: string } })
              ?.metadata
            return metadata?.namespace || '-'
          },
          cell: ({ getValue }: { getValue: () => string }) => (
            <Badge variant="outline" className="ml-2 ">
              {getValue()}
            </Badge>
          ),
        }

        // Insert namespace column after select and first column (typically name)
        const columnsWithNamespace = [...baseColumns]
        columnsWithNamespace.splice(2, 0, namespaceColumn)
        return columnsWithNamespace
      }
    }
    return baseColumns
  }, [columns, clusterScope, selectedNamespaces, t])

  const memoizedData = useMemo(() => (data || []) as T[], [data])

  useEffect(() => {
    if (!useSSE && error) {
      setRefreshInterval(0)
    }
  }, [useSSE, error, setRefreshInterval])

  // Create table instance using TanStack Table
  const table = useReactTable<T>({
    data: memoizedData,
    columns: enhancedColumns,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getFacetedRowModel: getFacetedRowModel(),
    getFacetedUniqueValues: getFacetedUniqueValues(),
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    onRowSelectionChange: setRowSelection,
    onColumnVisibilityChange: setColumnVisibility,
    getRowId: (row) => {
      const metadata = (
        row as {
          metadata?: { name?: string; namespace?: string; uid?: string }
        }
      )?.metadata
      if (!metadata?.name) {
        return `row-${Math.random()}`
      }
      return (
        metadata.uid ||
        (metadata.namespace
          ? `${metadata.namespace}/${metadata.name}`
          : metadata.name)
      )
    },
    state: {
      sorting,
      columnFilters,
      globalFilter: searchQuery,
      pagination,
      rowSelection,
      columnVisibility,
    },
    onPaginationChange: setPagination,
    // Let TanStack Table handle pagination automatically based on filtered data
    manualPagination: false,
    // Improve filtering performance and consistency
    globalFilterFn: (row, _columnId, value) => {
      const searchValue = String(value)

      // Regex mode: match all visible cells against the regex pattern.
      // Falls back to substring match if the pattern is invalid.
      if (useRegex && searchValue) {
        let regex: RegExp
        try {
          regex = new RegExp(searchValue, 'i')
        } catch {
          // Invalid regex — fall back to substring match
          const lower = searchValue.toLowerCase()
          if (searchQueryFilter) {
            return searchQueryFilter(row.original as T, lower)
          }
          return row.getVisibleCells().some((cell) => {
            const cellValue = String(cell.getValue() || '').toLowerCase()
            return cellValue.includes(lower)
          })
        }
        return row.getVisibleCells().some((cell) => {
          const cellValue = String(cell.getValue() || '')
          return regex.test(cellValue)
        })
      }

      // Normal substring mode
      if (searchQueryFilter) {
        return searchQueryFilter(row.original as T, searchValue.toLowerCase())
      }

      const lower = searchValue.toLowerCase()

      // Search across all visible columns
      return row.getVisibleCells().some((cell) => {
        const cellValue = String(cell.getValue() || '').toLowerCase()
        return cellValue.includes(lower)
      })
    },
    // Add this to prevent unnecessary pagination resets
    autoResetPageIndex: false,
    enableRowSelection: true,
  })

  // Handle batch delete - must be after table is defined
  const handleBatchDelete = useCallback(async () => {
    setIsDeleting(true)
    const selectedRows = table
      .getSelectedRowModel()
      .rows.map((row) => row.original)

    const deletePromises = selectedRows.map((row) => {
      const metadata = (
        row as { metadata?: { name?: string; namespace?: string } }
      )?.metadata
      const name = metadata?.name
      const namespace = clusterScope ? undefined : metadata?.namespace

      if (!name) {
        return Promise.resolve()
      }

      return deleteResource(resolvedResourceType, name, namespace)
        .then(() => {
          toast.success(t('resourceTable.deleteSuccess', { name }))
        })
        .catch((error) => {
          console.error(`Failed to delete ${name}:`, error)
          toast.error(
            t('resourceTable.deleteFailed', { name, error: error.message })
          )
          throw error
        })
    })

    try {
      await Promise.allSettled(deletePromises)
      // Reset selection and close dialog
      setRowSelection({})
      setDeleteDialogOpen(false)
      // Refetch data
      if (!useSSE) {
        refetch()
      }
    } finally {
      setIsDeleting(false)
    }
  }, [
    table,
    clusterScope,
    resolvedResourceType,
    t,
    useSSE,
    refetch,
    setRowSelection,
    setDeleteDialogOpen,
  ])
  // Calculate total and filtered row counts
  const totalRowCount = useMemo(
    () => (data as T[] | undefined)?.length || 0,
    [data]
  )
  const filteredRowCount = useMemo(() => {
    if (!data || (data as T[]).length === 0) return 0
    // Force re-computation when filters change
    void searchQuery // Ensure dependency is used
    void columnFilters // Ensure dependency is used
    return table.getFilteredRowModel().rows.length
  }, [table, data, searchQuery, columnFilters])

  // Check if there are active filters
  const hasActiveFilters = useMemo(() => {
    return Boolean(searchQuery) || columnFilters.length > 0
  }, [searchQuery, columnFilters])

  // Render empty state based on condition
  const renderEmptyState = () => {
    // Only show loading state if there's no existing data
    if (isLoading && (!data || (data as T[]).length === 0)) {
      return (
        <div className="h-72 flex flex-col items-center justify-center">
          <div className="mb-4 bg-muted/30 p-6 rounded-full">
            <Database className="h-12 w-12 text-muted-foreground animate-pulse" />
          </div>
          <h3 className="text-lg font-medium mb-1">
            Loading {displayResourceName}...
          </h3>
          <p className="text-muted-foreground">
            Retrieving data
            {!clusterScope && selectedNamespaces.length > 0
              ? ` from ${
                  selectedNamespaces.includes('_all')
                    ? 'All Namespaces'
                    : selectedNamespaces.length === 1
                      ? `namespace ${selectedNamespaces[0]}`
                      : `${selectedNamespaces.length} namespaces`
                }`
              : ''}
          </p>
        </div>
      )
    }

    if (isError) {
      return (
        <ErrorMessage
          resourceName={displayResourceName}
          error={error}
          refetch={refetch}
        />
      )
    }

    if (data && (data as T[]).length === 0) {
      return (
        <div className="h-72 flex flex-col items-center justify-center">
          <div className="mb-4 bg-muted/30 p-6 rounded-full">
            <Box className="h-12 w-12 text-muted-foreground" />
          </div>
          <h3 className="text-lg font-medium mb-1">
            No {displayResourceName} found
          </h3>
          <p className="text-muted-foreground">
            {searchQuery
              ? `No results match your search query: "${searchQuery}"`
              : clusterScope
                ? `There are no ${displayResourceName} found`
                : `There are no ${displayResourceName} in ${
                    selectedNamespaces.includes('_all')
                      ? 'any namespace'
                      : selectedNamespaces.length === 1
                        ? `namespace ${selectedNamespaces[0]}`
                        : `the ${selectedNamespaces.length} selected namespaces`
                  }`}
          </p>
          {searchQuery && (
            <Button
              variant="outline"
              className="mt-4"
              onClick={() => setSearchQuery('')}
            >
              Clear Search
            </Button>
          )}
        </div>
      )
    }

    return null
  }

  const emptyState = renderEmptyState()
  const selectedRows = table
    .getSelectedRowModel()
    .rows.map((row) => row.original)
  const batchActions = renderBatchActions?.({
    selectedRows,
    clearSelection: () => setRowSelection({}),
    refetch,
  })

  return (
    <div className="flex flex-col gap-3">
      <ResourceTableToolbar
        table={table}
        resourceName={displayResourceName}
        clusterScope={clusterScope}
        extraToolbars={extraToolbars}
        showCreateButton={showCreateButton}
        onCreateClick={onCreateClick}
        searchQuery={searchQuery}
        setSearchQuery={setSearchQuery}
        selectedNamespaces={selectedNamespaces}
        handleNamespaceChange={handleNamespaceChange}
        useSSE={useSSE}
        watchSupported={watchSupported}
        isConnected={isConnected}
        refreshInterval={refreshInterval}
        onUseSSEChange={handleUseSSEChange}
        onRefreshIntervalChange={handleRefreshIntervalChange}
        useRegex={useRegex}
        onUseRegexChange={handleUseRegexChange}
        selectedRowCount={table.getSelectedRowModel().rows.length}
        batchActions={batchActions}
        onOpenDeleteDialog={() => setDeleteDialogOpen(true)}
      />

      <ResourceTableView
        table={table}
        columnCount={enhancedColumns.length}
        isLoading={isLoading}
        data={data as T[] | undefined}
        fitViewportHeight={true}
        emptyState={emptyState}
        hasActiveFilters={hasActiveFilters}
        filteredRowCount={filteredRowCount}
        totalRowCount={totalRowCount}
        searchQuery={searchQuery}
        pagination={pagination}
        setPagination={setPagination}
      />

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('resourceTable.confirmDeletion')}</DialogTitle>
            <DialogDescription>
              {t('resourceTable.confirmDeletionMessage', {
                count: table.getSelectedRowModel().rows.length,
                resourceName: displayResourceName,
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteDialogOpen(false)}
              disabled={isDeleting}
            >
              {t('common.cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={handleBatchDelete}
              disabled={isDeleting}
            >
              {isDeleting ? t('resourceTable.deleting') : t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
