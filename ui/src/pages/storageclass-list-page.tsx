import { useMemo } from 'react'
import { createColumnHelper } from '@tanstack/react-table'
import { StorageClass } from 'kubernetes-types/storage/v1'
import { Check, X } from 'lucide-react'
import { Link } from 'react-router-dom'

import { createSearchFilter } from '@/lib/k8s'
import {
  getStorageClassParameters,
  getStorageClassTopologyRules,
  isDefaultStorageClass,
} from '@/lib/storageclass'
import { formatDate } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { multiSelectFilter, ResourceTable } from '@/components/resource-table'

const storageClassSearchFilter = createSearchFilter<StorageClass>(
  (storageClass) => storageClass.metadata?.name,
  (storageClass) => storageClass.provisioner,
  (storageClass) => storageClass.reclaimPolicy,
  (storageClass) => storageClass.volumeBindingMode,
  (storageClass) => getStorageClassParameters(storageClass),
  (storageClass) => storageClass.mountOptions,
  (storageClass) => getStorageClassTopologyRules(storageClass),
  (storageClass) => (isDefaultStorageClass(storageClass) ? 'default' : ''),
  (storageClass) =>
    storageClass.allowVolumeExpansion ? 'expandable expansion' : ''
)

export function StorageClassListPage() {
  const columnHelper = createColumnHelper<StorageClass>()

  const columns = useMemo(
    () => [
      columnHelper.accessor('metadata.name', {
        header: 'Name',
        cell: ({ row }) => (
          <div className="flex items-center gap-2 font-medium app-link">
            <Link to={`/storageclasses/${row.original.metadata?.name}`}>
              {row.original.metadata?.name}
            </Link>
            {isDefaultStorageClass(row.original) && (
              <Badge variant="default">Default</Badge>
            )}
          </div>
        ),
      }),
      columnHelper.accessor('provisioner', {
        header: 'Provisioner',
        enableColumnFilter: true,
        filterFn: multiSelectFilter,
        meta: { style: { minWidth: '180px' } },
        cell: ({ getValue }) => (
          <span className="font-mono text-sm">{getValue()}</span>
        ),
      }),
      columnHelper.accessor((row) => row.reclaimPolicy || 'Delete', {
        id: 'reclaimPolicy',
        header: 'Reclaim Policy',
        enableColumnFilter: true,
        filterFn: multiSelectFilter,
        cell: ({ getValue }) => (
          <Badge variant={getValue() === 'Retain' ? 'secondary' : 'outline'}>
            {getValue()}
          </Badge>
        ),
      }),
      columnHelper.accessor((row) => row.volumeBindingMode || 'Immediate', {
        id: 'volumeBindingMode',
        header: 'Binding Mode',
        enableColumnFilter: true,
        filterFn: multiSelectFilter,
        meta: { style: { minWidth: '170px' } },
        cell: ({ getValue }) => (
          <Badge
            variant={
              getValue() === 'WaitForFirstConsumer' ? 'secondary' : 'outline'
            }
          >
            {getValue()}
          </Badge>
        ),
      }),
      columnHelper.accessor(
        (row) => (row.allowVolumeExpansion ? 'Enabled' : 'Disabled'),
        {
          id: 'allowVolumeExpansion',
          header: 'Expansion',
          enableColumnFilter: true,
          filterFn: multiSelectFilter,
          cell: ({ getValue }) => {
            const enabled = getValue() === 'Enabled'
            return (
              <span
                className={
                  enabled
                    ? 'inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400'
                    : 'inline-flex items-center gap-1 text-muted-foreground'
                }
              >
                {enabled ? (
                  <Check className="h-4 w-4" />
                ) : (
                  <X className="h-4 w-4" />
                )}
                {getValue()}
              </span>
            )
          },
        }
      ),
      columnHelper.accessor((row) => getStorageClassParameters(row), {
        id: 'parameters',
        header: 'Parameters',
        meta: { style: { minWidth: '220px' } },
        cell: ({ getValue }) => {
          const parameters = getValue()
          if (parameters.length === 0) return '-'

          return (
            <div
              className="flex max-w-80 flex-wrap gap-1"
              title={parameters.join('\n')}
            >
              {parameters.slice(0, 2).map((parameter) => (
                <Badge
                  key={parameter}
                  variant="outline"
                  className="max-w-56 truncate font-mono font-normal"
                >
                  {parameter}
                </Badge>
              ))}
              {parameters.length > 2 && (
                <Badge variant="secondary">+{parameters.length - 2}</Badge>
              )}
            </div>
          )
        },
      }),
      columnHelper.accessor((row) => row.mountOptions || [], {
        id: 'mountOptions',
        header: 'Mount Options',
        meta: { style: { minWidth: '150px' } },
        cell: ({ getValue }) => {
          const options = getValue()
          return options.length > 0 ? (
            <span className="text-sm" title={options.join(', ')}>
              {options.join(', ')}
            </span>
          ) : (
            '-'
          )
        },
      }),
      columnHelper.accessor((row) => getStorageClassTopologyRules(row), {
        id: 'allowedTopologies',
        header: 'Allowed Topologies',
        meta: { style: { minWidth: '160px' } },
        cell: ({ getValue }) => {
          const rules = getValue()
          return rules.length > 0 ? (
            <Badge variant="outline" title={rules.join('\n')}>
              {rules.length} rule{rules.length === 1 ? '' : 's'}
            </Badge>
          ) : (
            <span className="text-muted-foreground">Any</span>
          )
        },
      }),
      columnHelper.accessor('metadata.creationTimestamp', {
        header: 'Created',
        cell: ({ getValue }) => (
          <span className="text-sm text-muted-foreground">
            {formatDate(getValue() || '')}
          </span>
        ),
      }),
    ],
    [columnHelper]
  )

  return (
    <ResourceTable
      resourceName="StorageClasses"
      resourceType="storageclasses"
      columns={columns}
      clusterScope={true}
      searchQueryFilter={storageClassSearchFilter}
    />
  )
}
