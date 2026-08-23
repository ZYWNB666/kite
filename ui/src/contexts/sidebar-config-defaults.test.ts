import { SidebarConfig } from '@/types/sidebar'

import {
  buildDefaultSidebarConfig,
  mergeSidebarConfigWithDefaults,
  SIDEBAR_CONFIG_VERSION,
} from './sidebar-config-defaults'

describe('mergeSidebarConfigWithDefaults', () => {
  it('places ReplicaSets in the default Workloads group', () => {
    const workloads = buildDefaultSidebarConfig().groups.find(
      (group) => group.id === 'sidebar-groups-workloads'
    )

    const urls = workloads?.items.map((item) => item.url)

    expect(urls).toContain('/replicasets')
    expect(urls?.indexOf('/replicasets')).toBe(
      (urls?.indexOf('/deployments') ?? -2) + 1
    )
  })

  it('adds new default resources while preserving user preferences', () => {
    const config: SidebarConfig = {
      version: SIDEBAR_CONFIG_VERSION - 1,
      groups: [
        {
          id: 'sidebar-groups-config',
          nameKey: 'sidebar.groups.config',
          items: [
            {
              id: 'sidebar-groups-config--configmaps',
              titleKey: 'nav.configMaps',
              url: '/configmaps',
              icon: 'IconMap',
              visible: true,
              pinned: false,
              order: 0,
            },
          ],
          visible: true,
          collapsed: true,
          order: 3,
        },
        {
          id: 'custom-team',
          nameKey: 'Team',
          items: [],
          visible: true,
          collapsed: false,
          order: 7,
          isCustom: true,
        },
      ],
      hiddenItems: ['sidebar-groups-config--configmaps'],
      pinnedItems: ['sidebar-groups-config--configmaps'],
      groupOrder: ['sidebar-groups-config', 'custom-team'],
      lastUpdated: 1,
    }

    const merged = mergeSidebarConfigWithDefaults(config)
    const configGroup = merged.groups.find(
      (group) => group.id === 'sidebar-groups-config'
    )

    expect(merged.version).toBe(SIDEBAR_CONFIG_VERSION)
    expect(configGroup?.collapsed).toBe(true)
    expect(configGroup?.items.map((item) => item.url)).toEqual(
      expect.arrayContaining([
        '/resourcequotas',
        '/limitranges',
        '/poddisruptionbudgets',
        '/priorityclasses',
        '/runtimeclasses',
        '/leases',
        '/mutatingwebhookconfigurations',
        '/validatingwebhookconfigurations',
        '/validatingadmissionpolicies',
        '/validatingadmissionpolicybindings',
      ])
    )
    expect(merged.groups).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ id: 'custom-team', isCustom: true }),
        expect.objectContaining({ id: 'sidebar-groups-network' }),
      ])
    )
    expect(merged.hiddenItems).toEqual(config.hiddenItems)
    expect(merged.pinnedItems).toEqual(config.pinnedItems)
  })

  it('merges the legacy Traffic group into Network without duplicates', () => {
    const config: SidebarConfig = {
      version: SIDEBAR_CONFIG_VERSION - 1,
      groups: [
        {
          id: 'sidebar-groups-traffic',
          nameKey: 'sidebar.groups.traffic',
          items: [
            {
              id: 'sidebar-groups-traffic--services',
              titleKey: 'nav.services',
              url: '/services',
              icon: 'IconNetwork',
              visible: true,
              pinned: false,
              order: 2,
            },
          ],
          visible: true,
          collapsed: true,
          order: 1,
        },
        {
          id: 'sidebar-groups-network',
          nameKey: 'sidebar.groups.network',
          items: [
            {
              id: 'sidebar-groups-network--services',
              titleKey: 'nav.services',
              url: '/services',
              icon: 'IconNetwork',
              visible: true,
              pinned: false,
              order: 2,
            },
          ],
          visible: true,
          collapsed: false,
          order: 6,
        },
      ],
      hiddenItems: ['sidebar-groups-traffic--services'],
      pinnedItems: [],
      groupOrder: ['sidebar-groups-traffic', 'sidebar-groups-network'],
      lastUpdated: 1,
    }

    const merged = mergeSidebarConfigWithDefaults(config)
    const networkGroups = merged.groups.filter(
      (group) => group.id === 'sidebar-groups-network'
    )

    expect(networkGroups).toHaveLength(1)
    expect(networkGroups[0].nameKey).toBe('sidebar.groups.network')
    expect(networkGroups[0].collapsed).toBe(true)
    expect(
      networkGroups[0].items.filter((item) => item.url === '/services')
    ).toHaveLength(1)
    expect(networkGroups[0].items.map((item) => item.url)).toEqual(
      expect.arrayContaining([
        '/endpointslices',
        '/endpoints',
        '/ingressclasses',
      ])
    )
    expect(
      merged.groupOrder.filter(
        (groupId) => groupId === 'sidebar-groups-network'
      )
    ).toHaveLength(1)
    expect(merged.hiddenItems).toContain('sidebar-groups-traffic--services')
  })
})
