import React from 'react'
import { ColumnDef, Table } from '@tanstack/react-table'
import {
  Plus,
  RefreshCw,
  Regex,
  Search,
  Settings2,
  Trash2,
  XCircle,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ResourceType } from '@/types/api'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Toggle } from '@/components/ui/toggle'

import { NamespaceSelector } from './selector/namespace-selector'

interface ResourceTableToolbarProps<T> {
  table: Table<T>
  resourceName: string
  resourceType?: ResourceType
  clusterScope: boolean
  extraToolbars: React.ReactNode[]
  showCreateButton: boolean
  onCreateClick?: () => void
  searchQuery: string
  setSearchQuery: (value: string) => void
  selectedNamespaces: string[]
  handleNamespaceChange: (value: string[]) => void
  useSSE: boolean
  isConnected: boolean
  refreshInterval: number
  onUseSSEChange: (pressed: boolean) => void
  onRefreshIntervalChange: (value: number) => void
  useRegex: boolean
  onUseRegexChange: (pressed: boolean) => void
  selectedRowCount: number
  onOpenDeleteDialog: () => void
}

export function ResourceTableToolbar<T>({
  table,
  resourceName,
  resourceType,
  clusterScope,
  extraToolbars,
  showCreateButton,
  onCreateClick,
  searchQuery,
  setSearchQuery,
  selectedNamespaces,
  handleNamespaceChange,
  useSSE,
  isConnected,
  refreshInterval,
  onUseSSEChange,
  onRefreshIntervalChange,
  useRegex,
  onUseRegexChange,
  selectedRowCount,
  onOpenDeleteDialog,
}: ResourceTableToolbarProps<T>) {
  const { t } = useTranslation()

  const filterableColumns = table.getAllColumns().filter((column) => {
    const columnDef = column.columnDef as ColumnDef<T> & {
      enableColumnFilter?: boolean
    }
    return columnDef.enableColumnFilter && column.getCanFilter()
  })

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          {extraToolbars.map((toolbar, index) => (
            <React.Fragment key={index}>{toolbar}</React.Fragment>
          ))}
          {resourceType === 'pods' && (
            <Toggle
              pressed={useSSE}
              variant="outline"
              className="px-3 text-muted-foreground data-[state=on]:text-foreground"
              aria-label={t('resourceTable.watch')}
              onPressedChange={onUseSSEChange}
            >
              <span
                className={cn(
                  'bg-muted-foreground/25 size-2 rounded-full',
                  useSSE && isConnected && 'bg-emerald-500',
                  useSSE && !isConnected && 'bg-red-500'
                )}
              />
              <span>{t('resourceTable.watch')}</span>
            </Toggle>
          )}
          <Select
            value={refreshInterval.toString()}
            onValueChange={(value) => onRefreshIntervalChange(Number(value))}
            disabled={useSSE}
          >
            <SelectTrigger className="w-full sm:w-[120px]">
              <div className="flex items-center gap-2">
                <RefreshCw className="h-4 w-4" />
                <SelectValue />
              </div>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="0">Off</SelectItem>
              <SelectItem value="1000">1s</SelectItem>
              <SelectItem value="5000">5s</SelectItem>
              <SelectItem value="10000">10s</SelectItem>
              <SelectItem value="30000">30s</SelectItem>
            </SelectContent>
          </Select>
          {!clusterScope && (
            <NamespaceSelector
              selectedNamespaces={selectedNamespaces}
              handleNamespaceChange={handleNamespaceChange}
              showAll={true}
            />
          )}
          {filterableColumns.map((column) => {
            const columnDef = column.columnDef as ColumnDef<T> & {
              enableColumnFilter?: boolean
            }
            const uniqueValues = column.getFacetedUniqueValues()
            const filterValue = (column.getFilterValue() as string[]) ?? []
            const headerLabel =
              typeof columnDef.header === 'string' ? columnDef.header : 'Column'
            const allOptions = Array.from(uniqueValues.keys())
              .filter(Boolean)
              .map(String)
              .sort()

            const toggleValue = (value: string) => {
              const next = filterValue.includes(value)
                ? filterValue.filter((v) => v !== value)
                : [...filterValue, value]
              column.setFilterValue(next.length ? next : undefined)
            }

            return (
              <DropdownMenu key={column.id}>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="outline"
                    className="w-full sm:w-auto sm:min-w-[8.5rem] sm:max-w-[14rem] justify-between font-normal"
                  >
                    <span className="truncate">
                      {filterValue.length === 0
                        ? `Filter ${headerLabel}`
                        : filterValue.length === 1
                          ? filterValue[0]
                          : `${headerLabel}: ${filterValue.length} selected`}
                    </span>
                    {filterValue.length > 0 && (
                      <XCircle
                        className="ml-1 h-3.5 w-3.5 shrink-0 text-muted-foreground hover:text-foreground"
                        onClick={(e) => {
                          e.stopPropagation()
                          column.setFilterValue(undefined)
                        }}
                      />
                    )}
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" className="max-h-72 overflow-y-auto">
                  <DropdownMenuLabel className="flex items-center justify-between py-1">
                    <span>{headerLabel}</span>
                    {filterValue.length > 0 && (
                      <button
                        className="text-xs text-muted-foreground hover:text-foreground"
                        onClick={() => column.setFilterValue(undefined)}
                      >
                        Clear
                      </button>
                    )}
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuCheckboxItem
                    checked={filterValue.length === 0}
                    onCheckedChange={() => column.setFilterValue(undefined)}
                    onSelect={(e) => e.preventDefault()}
                  >
                    All ({allOptions.reduce((s, v) => s + (uniqueValues.get(v) ?? 0), 0)})
                  </DropdownMenuCheckboxItem>
                  <DropdownMenuSeparator />
                  {allOptions.map((value) => (
                    <DropdownMenuCheckboxItem
                      key={value}
                      checked={filterValue.includes(value)}
                      onCheckedChange={() => toggleValue(value)}
                      onSelect={(e) => e.preventDefault()}
                    >
                      {value}
                      <span className="ml-auto text-xs text-muted-foreground">
                        {uniqueValues.get(value)}
                      </span>
                    </DropdownMenuCheckboxItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            )
          })}
        </div>

        <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row sm:items-center sm:justify-end">
          <div className="flex w-full items-center gap-2 sm:w-auto">
            <div className="relative min-w-0 flex-1 sm:w-[280px]">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder={
                  useRegex
                    ? t('resourceTable.regexSearchPlaceholder', {
                        defaultValue: `Regex search ${resourceName}...`,
                      })
                    : `Search ${resourceName}...`
                }
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full pl-9 pr-4"
              />
            </div>
            <Toggle
              pressed={useRegex}
              variant="outline"
              className="px-2 text-muted-foreground data-[state=on]:text-foreground"
              aria-label={t('resourceTable.regexSearch', {
                defaultValue: 'Regex search',
              })}
              onPressedChange={onUseRegexChange}
            >
              <Regex className="h-4 w-4" />
            </Toggle>
            {searchQuery && (
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setSearchQuery('')}
                className="h-9 w-9"
                aria-label="Clear search"
              >
                <XCircle className="h-4 w-4" />
              </Button>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-2 sm:justify-end">
            {selectedRowCount > 0 && (
              <Button
                variant="destructive"
                onClick={onOpenDeleteDialog}
                className="gap-2"
              >
                <Trash2 className="h-4 w-4" />
                {t('resourceTable.deleteSelected', {
                  count: selectedRowCount,
                })}
              </Button>
            )}
            {showCreateButton && onCreateClick && (
              <Button onClick={onCreateClick} className="gap-1">
                <Plus className="h-2 w-2" />
                New
              </Button>
            )}

            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="outline"
                  size="icon"
                  aria-label="Toggle columns"
                >
                  <Settings2 className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuLabel>Toggle columns</DropdownMenuLabel>
                <DropdownMenuSeparator />
                {table
                  .getAllLeafColumns()
                  .filter((column) => column.getCanHide())
                  .map((column) => {
                    const header = column.columnDef.header
                    const headerText =
                      typeof header === 'string' ? header : column.id

                    return (
                      <DropdownMenuCheckboxItem
                        key={column.id}
                        className="capitalize"
                        checked={column.getIsVisible()}
                        onCheckedChange={(value) =>
                          column.toggleVisibility(!!value)
                        }
                      >
                        {headerText}
                      </DropdownMenuCheckboxItem>
                    )
                  })}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </div>
    </div>
  )
}
