import type { ComponentType } from 'react'
import { IconBox, type Icon, type IconProps } from '@tabler/icons-react'

import {
  DefaultMenus,
  SidebarConfig,
  SidebarGroup,
  SidebarItem,
} from '@/types/sidebar'
import {
  getResourceIconComponent,
  resourceCatalog,
  resourceIconMap,
  sidebarGroupOrder,
} from '@/lib/resource-catalog'

const sidebarIconMap = resourceIconMap
type CatalogResource = (typeof resourceCatalog)[number]
type SidebarResource = CatalogResource & {
  sidebar: {
    groupKey: (typeof sidebarGroupOrder)[number]
    order: number
    titleKey?: string
    url?: string
  }
}

function hasSidebar(resource: CatalogResource): resource is SidebarResource {
  return 'sidebar' in resource
}

const defaultMenus: DefaultMenus = Object.fromEntries(
  sidebarGroupOrder.map((groupKey) => [groupKey, []])
) as DefaultMenus

resourceCatalog
  .filter(hasSidebar)
  .slice()
  .sort((a, b) => a.sidebar.order - b.sidebar.order)
  .forEach((resource) => {
    const sidebar = resource.sidebar
    defaultMenus[sidebar.groupKey].push({
      titleKey:
        sidebar.titleKey ||
        ('titleKey' in resource ? resource.titleKey : undefined) ||
        resource.pluralLabel,
      url: sidebar.url || `/${resource.type}`,
      icon: getResourceIconComponent(resource.icon),
    })
  })

export const SIDEBAR_CONFIG_VERSION = 8

const legacyGroupAliases = new Map([
  ['sidebar-groups-traffic', 'sidebar-groups-network'],
  ['sidebar.groups.traffic', 'sidebar-groups-network'],
])

function getIconName(iconComponent: ComponentType<{ className?: string }>) {
  const entry = Object.entries(sidebarIconMap).find(
    ([, component]) => component === iconComponent
  )
  return entry ? entry[0] : 'IconBox'
}

export function getSidebarIconComponent(
  iconName: string
):
  | React.ForwardRefExoticComponent<IconProps & React.RefAttributes<Icon>>
  | React.ElementType {
  return sidebarIconMap[iconName as keyof typeof sidebarIconMap] || IconBox
}

export function buildDefaultSidebarConfig(): SidebarConfig {
  const groups: SidebarGroup[] = []
  let groupOrder = 0

  Object.entries(defaultMenus).forEach(([groupKey, items]) => {
    const groupId = groupKey
      .toLowerCase()
      .replace(/\./g, '-')
      .replace(/\s+/g, '-')
    const sidebarItems: SidebarItem[] = items.map((item, index) => ({
      id: `${groupId}-${item.url.replace(/[^a-zA-Z0-9]/g, '-')}`,
      titleKey: item.titleKey,
      url: item.url,
      icon: getIconName(item.icon),
      visible: true,
      pinned: false,
      order: index,
    }))

    groups.push({
      id: groupId,
      nameKey: groupKey,
      items: sidebarItems,
      visible: true,
      collapsed: false,
      order: groupOrder++,
    })
  })

  return {
    version: SIDEBAR_CONFIG_VERSION,
    groups,
    hiddenItems: [],
    pinnedItems: [],
    groupOrder: groups.map((g) => g.id),
    lastUpdated: Date.now(),
  }
}

export function mergeSidebarConfigWithDefaults(
  config: SidebarConfig
): SidebarConfig {
  const defaults = buildDefaultSidebarConfig()
  const defaultGroups = new Map(
    defaults.groups.map((group) => [group.id, group] as const)
  )

  const normalizedGroups: SidebarGroup[] = []
  config.groups.forEach((group) => {
    const canonicalGroupId =
      legacyGroupAliases.get(group.id) ||
      legacyGroupAliases.get(group.nameKey) ||
      group.id
    const defaultGroup = defaultGroups.get(canonicalGroupId)
    const normalizedGroup =
      canonicalGroupId === group.id
        ? group
        : {
            ...group,
            id: canonicalGroupId,
            nameKey: defaultGroup?.nameKey || group.nameKey,
          }
    const existingGroup = normalizedGroups.find(
      (candidate) => candidate.id === canonicalGroupId
    )

    if (!existingGroup) {
      normalizedGroups.push(normalizedGroup)
      return
    }

    const existingItemIds = new Set(existingGroup.items.map((item) => item.id))
    const existingItemUrls = new Set(
      existingGroup.items.map((item) => item.url)
    )
    existingGroup.items = [
      ...existingGroup.items,
      ...normalizedGroup.items.filter(
        (item) =>
          !existingItemIds.has(item.id) && !existingItemUrls.has(item.url)
      ),
    ]
  })

  const groups = normalizedGroups.map((group) => {
    const defaultGroup = defaultGroups.get(group.id)
    if (!defaultGroup) {
      return group
    }

    const existingItemIds = new Set(group.items.map((item) => item.id))
    const existingItemUrls = new Set(group.items.map((item) => item.url))
    const missingItems = defaultGroup.items.filter(
      (item) => !existingItemIds.has(item.id) && !existingItemUrls.has(item.url)
    )

    return missingItems.length > 0
      ? { ...group, items: [...group.items, ...missingItems] }
      : group
  })

  const existingGroupIds = new Set(groups.map((group) => group.id))
  const missingGroups = defaults.groups.filter(
    (group) => !existingGroupIds.has(group.id)
  )

  return {
    ...config,
    version: SIDEBAR_CONFIG_VERSION,
    groups: [...groups, ...missingGroups],
    groupOrder: [
      ...Array.from(
        new Set(
          config.groupOrder.map(
            (groupId) => legacyGroupAliases.get(groupId) || groupId
          )
        )
      ),
      ...missingGroups
        .map((group) => group.id)
        .filter((groupId) => !config.groupOrder.includes(groupId)),
    ],
    lastUpdated: Date.now(),
  }
}
