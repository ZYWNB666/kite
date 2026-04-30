import { useMemo } from 'react'
import { IconExternalLink } from '@tabler/icons-react'
import { EndpointSlice } from 'kubernetes-types/discovery/v1'
import { Service } from 'kubernetes-types/core/v1'
import { toast } from 'sonner'

import { updateResource, useResource, useResources } from '@/lib/api'
import { withSubPath } from '@/lib/subpath'
import { formatDate } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { EventTable } from '@/components/event-table'
import { LabelsAnno } from '@/components/lables-anno'
import { OwnerInfoDisplay } from '@/components/owner-info-display'
import { RelatedResourcesTable } from '@/components/related-resource-table'
import { ResourceHistoryTable } from '@/components/resource-history-table'
import { SimpleTable } from '@/components/simple-table'

import {
  ResourceDetailShell,
  type ResourceDetailShellTab,
} from './resource-detail-shell'

function EndpointSlicesTab({
  endpointSlices,
}: {
  endpointSlices: EndpointSlice[]
}) {
  const columns = useMemo(
    () => [
      {
        header: 'Name',
        accessor: (s: EndpointSlice) => s.metadata?.name,
        cell: (v: unknown) => (
          <span className="font-mono text-sm">{String(v ?? '-')}</span>
        ),
      },
      {
        header: 'Address Type',
        accessor: (s: EndpointSlice) => s.addressType,
        cell: (v: unknown) => <Badge variant="outline">{String(v ?? '-')}</Badge>,
      },
      {
        header: 'Endpoints',
        accessor: (s: EndpointSlice) => s.endpoints,
        cell: (v: unknown) => {
          const endpoints = v as EndpointSlice['endpoints']
          const ready = endpoints?.filter((e) => e.conditions?.ready !== false) ?? []
          const addresses = ready
            .flatMap((e) => e.addresses)
            .slice(0, 8)
            .join(', ')
          const extra = ready.flatMap((e) => e.addresses).length - 8
          return (
            <span className="font-mono text-sm text-muted-foreground">
              {addresses || '-'}
              {extra > 0 && ` +${extra} more`}
            </span>
          )
        },
      },
      {
        header: 'Ports',
        accessor: (s: EndpointSlice) => s.ports,
        cell: (v: unknown) => {
          const ports = v as EndpointSlice['ports']
          if (!ports?.length) return <span className="text-muted-foreground">-</span>
          return (
            <span className="font-mono text-sm text-muted-foreground">
              {ports.map((p) => `${p.port}/${p.protocol ?? 'TCP'}`).join(', ')}
            </span>
          )
        },
      },
      {
        header: 'Ready',
        accessor: (s: EndpointSlice) =>
          s.endpoints?.filter((e) => e.conditions?.ready !== false).length ?? 0,
        cell: (v: unknown) => {
          const count = Number(v)
          return (
            <Badge variant={count > 0 ? 'default' : 'destructive'}>
              {count}
            </Badge>
          )
        },
      },
    ],
    []
  )

  return (
    <SimpleTable<EndpointSlice>
      data={endpointSlices}
      columns={columns}
      emptyMessage="No endpoint slices found"
    />
  )
}

export function ServiceDetail(props: { name: string; namespace?: string }) {
  const { namespace, name } = props

  const { data, isLoading, isError, error, refetch } = useResource(
    'services',
    name,
    namespace
  )

  const { data: endpointSlices = [] } = useResources('endpointslices', namespace, {
    labelSelector: `kubernetes.io/service-name=${name}`,
    staleTime: 5000,
  })

  const handleSaveYaml = async (content: Service) => {
    await updateResource('services', name, namespace, content)
    toast.success('YAML saved successfully')
    await refetch()
  }

  const tabs = useMemo<ResourceDetailShellTab<Service>[]>(
    () => [
      {
        value: 'endpoint-slices',
        label: 'Endpoint Slices',
        content: (
          <EndpointSlicesTab
            endpointSlices={endpointSlices as EndpointSlice[]}
          />
        ),
      },
      {
        value: 'related',
        label: 'Related',
        content: (
          <RelatedResourcesTable
            resource="services"
            name={name}
            namespace={namespace}
          />
        ),
      },
      {
        value: 'events',
        label: 'Events',
        content: (
          <EventTable resource="services" name={name} namespace={namespace} />
        ),
      },
      {
        value: 'history',
        label: 'History',
        content: data ? (
          <ResourceHistoryTable
            resourceType="services"
            name={name}
            namespace={namespace}
            currentResource={data}
          />
        ) : null,
      },
    ],
    [data, name, namespace]
  )

  return (
    <ResourceDetailShell
      resourceType="services"
      resourceLabel="Service"
      name={name}
      namespace={namespace}
      data={data}
      isLoading={isLoading}
      error={isError ? error : null}
      onRefresh={refetch}
      onSaveYaml={handleSaveYaml}
      overview={
        data ? (
          <div className="space-y-6">
            <Card>
              <CardHeader>
                <CardTitle className="capitalize">
                  Service Information
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                  <div>
                    <Label className="text-xs text-muted-foreground">
                      Created
                    </Label>
                    <p className="text-sm">
                      {formatDate(data.metadata?.creationTimestamp || '')}
                    </p>
                  </div>
                  <div>
                    <Label className="text-xs text-muted-foreground">UID</Label>
                    <p className="text-sm font-mono">
                      {data.metadata?.uid || 'N/A'}
                    </p>
                  </div>
                  <OwnerInfoDisplay metadata={data.metadata} />
                  <div>
                    <Label className="text-xs text-muted-foreground">
                      Ports
                    </Label>
                    <div className="flex flex-wrap items-center gap-1">
                      {(data.spec?.ports || []).map((port, index, array) => (
                        <span key={`${port.port}-${port.protocol}`}>
                          <a
                            href={withSubPath(
                              `/api/v1/namespaces/${namespace}/services/${name}:${port.port}/proxy/`
                            )}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex items-center gap-1 font-mono app-link"
                          >
                            {(port.name || port.protocol) &&
                              `${port.name || port.protocol}:`}
                            {port.port}
                            <IconExternalLink className="w-3 h-3" />
                          </a>
                          {index < array.length - 1 && ', '}
                        </span>
                      ))}
                    </div>
                  </div>
                </div>
                <LabelsAnno
                  labels={data.metadata?.labels || {}}
                  annotations={data.metadata?.annotations || {}}
                />
              </CardContent>
            </Card>
          </div>
        ) : null
      }
      extraTabs={tabs}
    />
  )
}
