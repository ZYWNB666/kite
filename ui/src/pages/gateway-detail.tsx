import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'

import {
  Gateway,
  GatewayListener,
  HTTPRoute,
  LocalObjectReference,
} from '@/types/gateway'
import { updateResource, useResource, useResources } from '@/lib/api'
import { getGatewayParentRefs, getRouteAcceptance } from '@/lib/gateway'
import { formatDate } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { EventTable } from '@/components/event-table'
import { LabelsAnno } from '@/components/lables-anno'

import {
  ResourceDetailShell,
  type ResourceDetailShellTab,
} from './resource-detail-shell'

export function GatewayDetail(props: { namespace: string; name: string }) {
  const { namespace, name } = props
  const {
    data: gateway,
    isLoading,
    error,
    refetch,
  } = useResource('gateways', name, namespace)
  const { data: routes, refetch: refetchRoutes } = useResources(
    'httproutes',
    namespace
  )
  const { data: gatewayClasses, refetch: refetchGatewayClasses } = useResources(
    'gatewayclasses',
    undefined
  )

  const gatewayClass = gatewayClasses?.find(
    (item) => item.metadata?.name === gateway?.spec?.gatewayClassName
  )
  const associatedRoutes = useMemo(
    () =>
      (routes || []).filter(
        (route) => getGatewayParentRefs(route, name, namespace).length > 0
      ),
    [name, namespace, routes]
  )

  const handleRefresh = async () => {
    await Promise.all([refetch(), refetchRoutes(), refetchGatewayClasses()])
  }

  const handleSaveYaml = async (content: Gateway) => {
    await updateResource('gateways', name, namespace, content)
    toast.success('Gateway YAML saved successfully')
    await handleRefresh()
  }

  const extraTabs = useMemo<ResourceDetailShellTab<Gateway>[]>(
    () => [
      {
        value: 'routes',
        label: (
          <>
            Routes
            <Badge variant="secondary">{associatedRoutes.length}</Badge>
          </>
        ),
        content: (
          <AssociatedRoutesTable
            routes={associatedRoutes}
            gatewayName={name}
            gatewayNamespace={namespace}
          />
        ),
      },
      {
        value: 'events',
        label: 'Events',
        content: (
          <EventTable resource="gateways" name={name} namespace={namespace} />
        ),
      },
    ],
    [associatedRoutes, name, namespace]
  )

  return (
    <ResourceDetailShell
      resourceType="gateways"
      resourceLabel="Gateway"
      name={name}
      namespace={namespace}
      data={gateway}
      isLoading={isLoading}
      error={error}
      onRefresh={handleRefresh}
      onSaveYaml={handleSaveYaml}
      overview={
        gateway ? (
          <div className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle>Gateway Overview</CardTitle>
              </CardHeader>
              <CardContent className="space-y-5">
                <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                  <div>
                    <p className="text-xs text-muted-foreground">
                      GatewayClass
                    </p>
                    {gateway.spec?.gatewayClassName ? (
                      <div className="mt-1 flex flex-wrap items-center gap-2">
                        <Link
                          className="app-link font-medium"
                          to={`/gatewayclasses/${gateway.spec.gatewayClassName}`}
                        >
                          {gateway.spec.gatewayClassName}
                        </Link>
                        {gatewayClass?.status?.conditions
                          ?.filter((condition) => condition.type === 'Accepted')
                          .map((condition) => (
                            <Badge
                              key={condition.type}
                              variant={
                                condition.status === 'True'
                                  ? 'default'
                                  : 'destructive'
                              }
                            >
                              {condition.status === 'True'
                                ? 'Accepted'
                                : 'Not Accepted'}
                            </Badge>
                          ))}
                      </div>
                    ) : (
                      <p className="text-sm text-muted-foreground">N/A</p>
                    )}
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Controller</p>
                    <p className="mt-1 break-all font-mono text-sm">
                      {gatewayClass?.spec?.controllerName || 'N/A'}
                    </p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Created</p>
                    <p className="mt-1 text-sm">
                      {formatDate(gateway.metadata?.creationTimestamp || '')}
                    </p>
                  </div>
                </div>
                {gateway.status?.conditions &&
                  gateway.status.conditions.length > 0 && (
                    <div>
                      <p className="mb-2 text-xs text-muted-foreground">
                        Conditions
                      </p>
                      <div className="flex flex-wrap gap-2">
                        {gateway.status.conditions.map((condition) => (
                          <Badge
                            key={condition.type}
                            variant={
                              condition.status === 'True'
                                ? 'default'
                                : 'secondary'
                            }
                            title={condition.message || condition.reason}
                          >
                            {condition.type}: {condition.status}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  )}
                <LabelsAnno
                  labels={gateway.metadata?.labels || {}}
                  annotations={gateway.metadata?.annotations || {}}
                />
              </CardContent>
            </Card>

            <GatewayAddresses gateway={gateway} />
            <GatewayListeners gateway={gateway} />

            <AssociatedRoutesTable
              routes={associatedRoutes}
              gatewayName={name}
              gatewayNamespace={namespace}
            />
          </div>
        ) : null
      }
      extraTabs={extraTabs}
    />
  )
}

function GatewayAddresses({ gateway }: { gateway: Gateway }) {
  const addresses = [
    ...(gateway.spec?.addresses || []).map((address) => ({
      ...address,
      source: 'Requested',
    })),
    ...(gateway.status?.addresses || []).map((address) => ({
      ...address,
      source: 'Assigned',
    })),
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>Addresses</CardTitle>
      </CardHeader>
      <CardContent>
        {addresses.length > 0 ? (
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            {addresses.map((address, index) => (
              <div
                key={`${address.source}-${address.type}-${address.value}-${index}`}
                className="rounded border p-3"
              >
                <div className="mb-1 flex items-center justify-between gap-2">
                  <Badge variant="outline">{address.type || 'IPAddress'}</Badge>
                  <span className="text-xs text-muted-foreground">
                    {address.source}
                  </span>
                </div>
                <p className="break-all font-mono text-sm">{address.value}</p>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            No requested or assigned addresses
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function GatewayListeners({ gateway }: { gateway: Gateway }) {
  const listeners = gateway.spec?.listeners || []

  return (
    <Card>
      <CardHeader>
        <CardTitle>Listeners ({listeners.length})</CardTitle>
      </CardHeader>
      <CardContent className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Protocol / Port</TableHead>
              <TableHead>Hostname</TableHead>
              <TableHead>TLS</TableHead>
              <TableHead>Allowed Routes</TableHead>
              <TableHead>Attached</TableHead>
              <TableHead>Conditions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {listeners.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className="text-center text-muted-foreground"
                >
                  No listeners configured
                </TableCell>
              </TableRow>
            ) : (
              listeners.map((listener) => (
                <GatewayListenerRow
                  key={listener.name}
                  listener={listener}
                  gateway={gateway}
                />
              ))
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

function GatewayListenerRow({
  listener,
  gateway,
}: {
  listener: GatewayListener
  gateway: Gateway
}) {
  const listenerStatus = gateway.status?.listeners?.find(
    (status) => status.name === listener.name
  )
  const certificates = listener.tls?.certificateRefs || []
  const allowedNamespace = listener.allowedRoutes?.namespaces?.from || 'Same'
  const allowedKinds = listener.allowedRoutes?.kinds
    ?.map((kind) => kind.kind)
    .join(', ')

  return (
    <TableRow>
      <TableCell className="font-medium">{listener.name}</TableCell>
      <TableCell>
        <Badge variant="outline">
          {listener.protocol} / {listener.port}
        </Badge>
      </TableCell>
      <TableCell className="font-mono text-sm">
        {listener.hostname || '*'}
      </TableCell>
      <TableCell>
        {listener.tls ? (
          <div className="space-y-1 text-xs">
            <Badge variant="secondary">
              {listener.tls.mode || 'Terminate'}
            </Badge>
            {certificates.map((certificate) => (
              <div key={referenceKey(certificate)} className="font-mono">
                {formatReference(certificate)}
              </div>
            ))}
          </div>
        ) : (
          '-'
        )}
      </TableCell>
      <TableCell>
        <div className="text-sm">
          <div>Namespaces: {allowedNamespace}</div>
          <div className="text-xs text-muted-foreground">
            Kinds: {allowedKinds || 'All supported'}
          </div>
        </div>
      </TableCell>
      <TableCell>{listenerStatus?.attachedRoutes ?? 0}</TableCell>
      <TableCell>
        <div className="flex max-w-64 flex-wrap gap-1">
          {(listenerStatus?.conditions || []).map((condition) => (
            <Badge
              key={condition.type}
              variant={condition.status === 'True' ? 'default' : 'secondary'}
              title={condition.message || condition.reason}
            >
              {condition.type}
            </Badge>
          ))}
          {!listenerStatus?.conditions?.length && '-'}
        </div>
      </TableCell>
    </TableRow>
  )
}

function AssociatedRoutesTable({
  routes,
  gatewayName,
  gatewayNamespace,
}: {
  routes: HTTPRoute[]
  gatewayName: string
  gatewayNamespace: string
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Associated Routes ({routes.length})</CardTitle>
      </CardHeader>
      <CardContent className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Hostnames</TableHead>
              <TableHead>Listener Sections</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Created</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {routes.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className="text-center text-muted-foreground"
                >
                  No HTTPRoutes reference this Gateway
                </TableCell>
              </TableRow>
            ) : (
              routes.map((route) => {
                const routeNamespace =
                  route.metadata?.namespace || gatewayNamespace
                const parentRefs = getGatewayParentRefs(
                  route,
                  gatewayName,
                  gatewayNamespace
                )
                const acceptance = getRouteAcceptance(
                  route,
                  gatewayName,
                  gatewayNamespace
                )
                return (
                  <TableRow key={`${routeNamespace}/${route.metadata?.name}`}>
                    <TableCell>
                      <Link
                        className="app-link font-medium"
                        to={`/httproutes/${routeNamespace}/${route.metadata?.name}`}
                      >
                        {route.metadata?.name}
                      </Link>
                    </TableCell>
                    <TableCell>
                      {route.spec?.hostnames?.join(', ') || '*'}
                    </TableCell>
                    <TableCell>
                      {parentRefs
                        .map((parentRef) => parentRef.sectionName || 'All')
                        .join(', ')}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          acceptance === 'Accepted'
                            ? 'default'
                            : acceptance === 'Rejected'
                              ? 'destructive'
                              : 'secondary'
                        }
                      >
                        {acceptance}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {formatDate(route.metadata?.creationTimestamp || '')}
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

function referenceKey(reference: LocalObjectReference) {
  return `${reference.group || ''}/${reference.kind || ''}/${reference.namespace || ''}/${reference.name}`
}

function formatReference(reference: LocalObjectReference) {
  return [reference.namespace, reference.name].filter(Boolean).join('/')
}
