import { HTTPRoute, HTTPRouteParentRef } from '@/types/gateway'

function isGatewayParentRef(parentRef: HTTPRouteParentRef) {
  return (
    (!parentRef.group || parentRef.group === 'gateway.networking.k8s.io') &&
    (!parentRef.kind || parentRef.kind === 'Gateway')
  )
}

export function getGatewayParentRefs(
  route: HTTPRoute,
  gatewayName: string,
  gatewayNamespace: string
) {
  const routeNamespace = route.metadata?.namespace || 'default'
  return (route.spec?.parentRefs || []).filter(
    (parentRef) =>
      isGatewayParentRef(parentRef) &&
      parentRef.name === gatewayName &&
      (parentRef.namespace || routeNamespace) === gatewayNamespace
  )
}

export function getRouteAcceptance(
  route: HTTPRoute,
  gatewayName: string,
  gatewayNamespace: string
) {
  const routeNamespace = route.metadata?.namespace || 'default'
  const matchingParents = (route.status?.parents || []).filter(
    ({ parentRef }) =>
      isGatewayParentRef(parentRef) &&
      parentRef.name === gatewayName &&
      (parentRef.namespace || routeNamespace) === gatewayNamespace
  )
  const accepted = matchingParents
    .flatMap((parent) => parent.conditions || [])
    .find((condition) => condition.type === 'Accepted')

  if (!accepted) return 'Unknown'
  return accepted.status === 'True' ? 'Accepted' : 'Rejected'
}
