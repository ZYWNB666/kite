import { useEffect, useMemo, useState } from 'react'
import { createColumnHelper } from '@tanstack/react-table'
import { Service } from 'kubernetes-types/core/v1'
import { EndpointSlice } from 'kubernetes-types/discovery/v1'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { useResources } from '@/lib/api'
import { getCurrentNamespaces } from '@/lib/current-namespace'
import {
  createSearchFilter,
  getServiceExternalIP,
  getServicePortSearchValues,
} from '@/lib/k8s'
import { formatDate } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { multiSelectFilter, ResourceTable } from '@/components/resource-table'

const serviceSearchFilter = createSearchFilter<Service>(
  (s) => s.metadata?.name,
  (s) => s.spec?.type,
  (s) => s.spec?.clusterIP,
  (s) => getServicePortSearchValues(s)
)

export function ServiceListPage() {
  const { t } = useTranslation()
  // Define column helper outside of any hooks
  const columnHelper = createColumnHelper<Service>()

  const [namespace, setNamespace] = useState<string | undefined>(() => {
    const namespaces = getCurrentNamespaces()
    return namespaces.includes('_all') || namespaces.length > 1
      ? undefined
      : namespaces[0]
  })

  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent<{ namespaces: string[] }>).detail
      const nss = detail.namespaces || []
      if (nss.includes('_all') || nss.length > 1) {
        setNamespace(undefined)
      } else {
        setNamespace(nss[0] || 'default')
      }
    }
    window.addEventListener('kite:namespace-change', handler)
    return () => window.removeEventListener('kite:namespace-change', handler)
  }, [])

  const { data: endpointSlices = [] } = useResources(
    'endpointslices',
    namespace,
    {
      staleTime: 5000,
    }
  )

  const endpointSliceMap = useMemo(() => {
    const map = new Map<string, EndpointSlice[]>()
    for (const es of endpointSlices) {
      const svcName = es.metadata?.labels?.['kubernetes.io/service-name']
      if (svcName) {
        if (!map.has(svcName)) map.set(svcName, [])
        map.get(svcName)!.push(es as EndpointSlice)
      }
    }
    return map
  }, [endpointSlices])

  // Define columns for the service table
  const columns = useMemo(
    () => [
      columnHelper.accessor('metadata.name', {
        header: t('common.name'),
        cell: ({ row }) => (
          <div className="font-medium app-link">
            <Link
              to={`/services/${row.original.metadata!.namespace}/${
                row.original.metadata!.name
              }`}
            >
              {row.original.metadata!.name}
            </Link>
          </div>
        ),
      }),
      columnHelper.accessor('spec.type', {
        header: t('services.type'),
        enableColumnFilter: true,
        filterFn: multiSelectFilter,
        cell: ({ getValue }) => {
          const type = getValue() || 'ClusterIP'
          return <Badge variant="outline">{type}</Badge>
        },
      }),
      columnHelper.accessor('spec.clusterIP', {
        header: t('services.clusterIP'),
        cell: ({ getValue }) => {
          const val = getValue() || '-'
          return (
            <span className="font-mono text-sm text-muted-foreground">
              {val}
            </span>
          )
        },
      }),
      columnHelper.accessor('status.loadBalancer.ingress', {
        header: t('services.externalIP'),
        cell: ({ row }) => {
          const val = getServiceExternalIP(row.original)
          return (
            <span className="font-mono text-sm text-muted-foreground">
              {val}
            </span>
          )
        },
      }),
      columnHelper.accessor('spec.ports', {
        header: t('services.ports'),
        cell: ({ getValue }) => {
          const ports = getValue() || []
          if (ports.length === 0) return '-'
          const text = ports
            .map((port) => {
              const protocol = port.protocol || 'TCP'
              if (port.nodePort) {
                return `${port.port}:${port.nodePort}/${protocol}`
              }
              return `${port.port}/${protocol}`
            })
            .join(', ')
          return (
            <span className="font-mono text-sm text-muted-foreground">
              {text}
            </span>
          )
        },
      }),
      columnHelper.accessor('metadata.creationTimestamp', {
        header: t('common.created'),
        cell: ({ getValue }) => {
          const dateStr = formatDate(getValue() || '')

          return (
            <span className="text-muted-foreground text-sm">{dateStr}</span>
          )
        },
      }),
      columnHelper.accessor('metadata.name', {
        id: 'endpoints',
        header: 'Endpoints',
        cell: ({ row }) => {
          const svcName = row.original.metadata?.name
          const slices = svcName ? (endpointSliceMap.get(svcName) ?? []) : []
          const readyAddresses = slices
            .flatMap((s) => s.endpoints ?? [])
            .filter((e) => e.conditions?.ready !== false)
            .flatMap((e) => e.addresses)
          if (readyAddresses.length === 0) {
            return <span className="text-muted-foreground text-sm">-</span>
          }
          const display = readyAddresses.slice(0, 3).join(', ')
          const extra = readyAddresses.length - 3
          return (
            <span className="font-mono text-sm text-muted-foreground">
              {display}
              {extra > 0 && (
                <span className="text-xs opacity-70"> +{extra}</span>
              )}
            </span>
          )
        },
      }),
    ],
    [columnHelper, t, endpointSliceMap]
  )

  return (
    <ResourceTable
      resourceName="Services"
      columns={columns}
      clusterScope={false} // Services are namespace-scoped
      searchQueryFilter={serviceSearchFilter}
    />
  )
}
