import { HTTPRoute } from '@/types/gateway'

import { getGatewayParentRefs, getRouteAcceptance } from './gateway'

describe('gateway route association', () => {
  const route = {
    metadata: { name: 'route', namespace: 'app' },
    spec: {
      parentRefs: [
        { name: 'public', namespace: 'infra', sectionName: 'https' },
        { name: 'other' },
        { name: 'public', kind: 'Service' },
      ],
    },
    status: {
      parents: [
        {
          parentRef: { name: 'public', namespace: 'infra' },
          conditions: [{ type: 'Accepted', status: 'True' }],
        },
      ],
    },
  } as HTTPRoute

  it('matches Gateway parentRefs with explicit namespaces', () => {
    expect(getGatewayParentRefs(route, 'public', 'infra')).toEqual([
      { name: 'public', namespace: 'infra', sectionName: 'https' },
    ])
    expect(getGatewayParentRefs(route, 'public', 'app')).toEqual([])
  })

  it('derives route acceptance for the matching Gateway parent', () => {
    expect(getRouteAcceptance(route, 'public', 'infra')).toBe('Accepted')
    expect(getRouteAcceptance(route, 'other', 'app')).toBe('Unknown')
  })
})
