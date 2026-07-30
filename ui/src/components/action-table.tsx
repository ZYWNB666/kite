import { useMemo } from 'react'
import {
  ColumnDef,
  flexRender,
  getCoreRowModel,
  PaginationState,
  useReactTable,
} from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { Button } from './ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from './ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from './ui/table'

interface ActionTableProps<T> {
  data: T[]
  columns: ColumnDef<T>[]
  actions: Action<T>[]
  pagination?: {
    state: PaginationState
    setPagination: React.Dispatch<React.SetStateAction<PaginationState>>
    total: number
  }
}

export interface Action<T> {
  label: string | React.ReactNode
  dynamicLabel?: (item: T) => string | React.ReactNode
  onClick: (item: T) => void
  shouldDisable?: (item: T) => boolean
}

export function ActionTable<T>({
  data,
  columns,
  actions,
  pagination,
}: ActionTableProps<T>) {
  const { t } = useTranslation()
  const tableColumns = useMemo(() => {
    if (actions.length > 0) {
      const actionColumn: ColumnDef<T> = {
        id: 'actions',
        header: t('common.actions'),
        cell: ({ row }) => (
          <div className="text-right">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="sm">
                  •••
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {actions.map((action, index) => (
                  <DropdownMenuItem
                    key={index}
                    disabled={action.shouldDisable?.(row.original)}
                    onClick={() => action.onClick(row.original)}
                    className="gap-2"
                  >
                    {action.dynamicLabel
                      ? action.dynamicLabel(row.original)
                      : action.label}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        ),
      }
      return [...columns, actionColumn]
    }
    return columns
  }, [actions, columns, t])

  const table = useReactTable<T>({
    data,
    columns: tableColumns,
    getCoreRowModel: getCoreRowModel(),
    state: pagination ? { pagination: pagination.state } : {},
    onPaginationChange: pagination?.setPagination,
    manualPagination: Boolean(pagination),
    pageCount: pagination
      ? Math.max(1, Math.ceil(pagination.total / pagination.state.pageSize))
      : undefined,
  })

  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader className="bg-muted sticky top-0 z-10">
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead
                  key={header.id}
                  className={header.id === 'actions' ? 'text-right' : ''}
                >
                  {header.isPlaceholder
                    ? null
                    : (header.column.columnDef.header as React.ReactNode)}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {table.getRowModel().rows.map((row) => (
            <TableRow
              key={row.id}
              data-state={row.getIsSelected() && 'selected'}
            >
              {row.getVisibleCells().map((cell) => (
                <TableCell key={cell.id}>
                  {cell.column.columnDef.cell
                    ? flexRender(cell.column.columnDef.cell, cell.getContext())
                    : String(cell.getValue() || '-')}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {pagination && data.length > 0 && (
        <div className="flex flex-col gap-3 border-t px-3 py-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="text-sm text-muted-foreground">
            {pagination.total} row(s) total.
          </div>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-4">
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">
                Rows per page:
              </span>
              <Select
                value={pagination.state.pageSize.toString()}
                onValueChange={(value) => {
                  pagination.setPagination((previous) => ({
                    ...previous,
                    pageIndex: 0,
                    pageSize: Number(value),
                  }))
                }}
              >
                <SelectTrigger size="sm" className="w-20">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {[10, 20, 50, 100].map((pageSize) => (
                    <SelectItem key={pageSize} value={String(pageSize)}>
                      {pageSize}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="text-sm font-medium">
              Page {pagination.state.pageIndex + 1} of {table.getPageCount()}
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => table.previousPage()}
                disabled={!table.getCanPreviousPage()}
              >
                Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => table.nextPage()}
                disabled={!table.getCanNextPage()}
              >
                Next
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
