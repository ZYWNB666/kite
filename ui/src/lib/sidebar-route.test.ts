import { isSidebarRouteActive } from './sidebar-route'

describe('isSidebarRouteActive', () => {
  it('does not activate Endpoints for the Endpoint Slices route', () => {
    expect(isSidebarRouteActive('/endpointslices', '/endpointslices')).toBe(
      true
    )
    expect(isSidebarRouteActive('/endpointslices', '/endpoints')).toBe(false)
  })

  it('activates a resource for its list and detail routes', () => {
    expect(isSidebarRouteActive('/endpoints', '/endpoints')).toBe(true)
    expect(
      isSidebarRouteActive('/endpoints/default/kubernetes', '/endpoints')
    ).toBe(true)
  })

  it('requires exact matches for overview and CRD definitions', () => {
    expect(isSidebarRouteActive('/', '/')).toBe(true)
    expect(isSidebarRouteActive('/pods', '/')).toBe(false)
    expect(isSidebarRouteActive('/crds', '/crds')).toBe(true)
    expect(isSidebarRouteActive('/crds/widgets.example.com', '/crds')).toBe(
      false
    )
  })
})
