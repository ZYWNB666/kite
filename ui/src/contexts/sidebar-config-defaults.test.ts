import { SidebarConfig } from '@/types/sidebar'

import {
  mergeSidebarConfigWithDefaults,
  SIDEBAR_CONFIG_VERSION,
} from './sidebar-config-defaults'

describe('mergeSidebarConfigWithDefaults', () => {
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
})
