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

  const endpointSummary = useMemo(() => {
    const slices = endpointSlices as EndpointSlice[]
    const total = slices.reduce((sum, s) => sum + (s.endpoints?.length ?? 0), 0)
    const ready = slices.reduce(
      (sum, s) =>
        sum + (s.endpoints?.filter((e) => e.conditions?.ready !== false).length ?? 0),
      0
    )
    return { total, ready }
  }, [endpointSlices])

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
    [data, name, namespace, endpointSlices]
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
                  {/* Created */}
                  <div>
                    <Label className="text-xs text-muted-foreground">Created</Label>
                    <p className="text-sm">
                      {formatDate(data.metadata?.creationTimestamp || '')}
                    </p>
                  </div>

                  {/* Type + ClusterIP */}
                  <div>
                    <Label className="text-xs text-muted-foreground">Type</Label>
                    <div className="flex items-center gap-2 mt-1">
                      <Badge
                        variant={
                          data.spec?.type === 'LoadBalancer'
                            ? 'default'
                            : data.spec?.type === 'NodePort'
                              ? 'secondary'
                              : 'outline'
                        }
                      >
                        {data.spec?.type || 'ClusterIP'}
                      </Badge>
                      {data.spec?.clusterIP && data.spec.clusterIP !== 'None' && (
                        <span className="text-sm font-mono text-muted-foreground">
                          {data.spec.clusterIP}
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Owner */}
                  <OwnerInfoDisplay metadata={data.metadata} />

                  {/* Endpoint health */}
                  <div>
                    <Label className="text-xs text-muted-foreground">Endpoints</Label>
                    <div className="mt-1">
                      <Badge
                        variant={
                          endpointSummary.total === 0
                            ? 'outline'
                            : endpointSummary.ready === endpointSummary.total
                              ? 'default'
                              : endpointSummary.ready === 0
                                ? 'destructive'
                                : 'secondary'
                        }
                      >
                        Ready {endpointSummary.ready} / {endpointSummary.total}
                      </Badge>
                    </div>
                  </div>

                  {/* External IP — LoadBalancer only */}
                  {data.spec?.type === 'LoadBalancer' &&
                    (data.status?.loadBalancer?.ingress?.length ?? 0) > 0 && (
                      <div>
                        <Label className="text-xs text-muted-foreground">External IP</Label>
                        <div className="flex flex-wrap gap-1 mt-1">
                          {data.status?.loadBalancer?.ingress?.map((ing, i) => {
                            const addr = ing.ip || ing.hostname || ''
                            return (
                              <a
                                key={i}
                                href={`http://${addr}`}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="inline-flex items-center gap-1 font-mono text-sm app-link"
                              >
                                {addr}
                                <IconExternalLink className="w-3 h-3" />
                              </a>
                            )
                          })}
                        </div>
                      </div>
                    )}

                  {/* Ports — enhanced: port → targetPort (proto) + nodePort badge */}
                  <div>
                    <Label className="text-xs text-muted-foreground">Ports</Label>
                    <div className="flex flex-col gap-1 mt-1">
                      {(data.spec?.ports || []).map((port) => {
                        const targetPort =
                          port.targetPort !== undefined ? String(port.targetPort) : ''
                        const proto = port.protocol || 'TCP'
                        const portLabel = port.name ? `${port.name}: ` : ''
                        return (
                          <div
                            key={`${port.port}-${proto}`}
                            className="flex items-center gap-2"
                          >
                            <a
                              href={withSubPath(
                                `/api/v1/namespaces/${namespace}/services/${name}:${port.port}/proxy/`
                              )}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="inline-flex items-center gap-1 font-mono text-sm app-link"
                            >
                              {portLabel}
                              {port.port}
                              {targetPort && targetPort !== String(port.port)
                                ? ` → ${targetPort}`
                                : ''}
                              {` (${proto})`}
                              <IconExternalLink className="w-3 h-3" />
                            </a>
                            {port.nodePort && (
                              <Badge variant="outline" className="text-xs font-mono">
                                nodePort: {port.nodePort}
                              </Badge>
                            )}
                          </div>
                        )
                      })}
                    </div>
                  </div>

                  {/* Selector */}
                  {data.spec?.selector &&
                    Object.keys(data.spec.selector).length > 0 && (
                      <div>
                        <Label className="text-xs text-muted-foreground">Selector</Label>
                        <div className="flex flex-wrap gap-1 mt-1">
                          {Object.entries(data.spec.selector).map(([key, value]) => (
                            <Badge
                              key={key}
                              variant="secondary"
                              className="text-xs font-mono"
                            >
                              {key}: {value}
                            </Badge>
                          ))}
                        </div>
                      </div>
                    )}

                  {/* Session Affinity — only when ClientIP */}
                  {data.spec?.sessionAffinity === 'ClientIP' && (
                    <div>
                      <Label className="text-xs text-muted-foreground">
                        Session Affinity
                      </Label>
                      <div className="mt-1">
                        <Badge variant="outline" className="text-xs">
                          ClientIP
                          {data.spec.sessionAffinityConfig?.clientIP?.timeoutSeconds
                            ? ` (${data.spec.sessionAffinityConfig.clientIP.timeoutSeconds}s)`
                            : ''}
                        </Badge>
                      </div>
                    </div>
                  )}

                  {/* External Traffic Policy — NodePort / LoadBalancer only */}
                  {(data.spec?.type === 'NodePort' ||
                    data.spec?.type === 'LoadBalancer') &&
                    data.spec?.externalTrafficPolicy && (
                      <div>
                        <Label className="text-xs text-muted-foreground">
                          External Traffic Policy
                        </Label>
                        <div className="mt-1">
                          <Badge
                            variant={
                              data.spec.externalTrafficPolicy === 'Local'
                                ? 'secondary'
                                : 'outline'
                            }
                            className="text-xs"
                          >
                            {data.spec.externalTrafficPolicy}
                          </Badge>
                        </div>
                      </div>
                    )}
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
