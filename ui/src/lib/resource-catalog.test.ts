import { resourceCatalog } from './resource-catalog'

function resourcesInGroup(groupKey: string) {
  return resourceCatalog
    .filter(
      (resource) =>
        'sidebar' in resource && resource.sidebar.groupKey === groupKey
    )
    .map((resource) => resource.type)
}

describe('resource catalog navigation', () => {
  it('includes ReplicaSets in Workloads', () => {
    expect(resourcesInGroup('sidebar.groups.workloads')).toContain(
      'replicasets'
    )
  })

  it('includes the requested Config resources', () => {
    expect(resourcesInGroup('sidebar.groups.config')).toEqual(
      expect.arrayContaining([
        'resourcequotas',
        'limitranges',
        'poddisruptionbudgets',
        'priorityclasses',
        'runtimeclasses',
        'leases',
        'mutatingwebhookconfigurations',
        'validatingwebhookconfigurations',
        'validatingadmissionpolicies',
        'validatingadmissionpolicybindings',
      ])
    )
  })

  it('includes the requested Network resources', () => {
    expect(resourcesInGroup('sidebar.groups.network')).toEqual(
      expect.arrayContaining(['endpointslices', 'endpoints', 'ingressclasses'])
    )
  })
})
