import * as React from 'react'
import { useMemo, useRef, useEffect, useCallback } from 'react'
import Icon from '@/assets/icon.svg'
import { useSidebarConfig } from '@/contexts/sidebar-config-context'
import { CollapsibleContent } from '@radix-ui/react-collapsible'
import { IconBoxMultiple, IconCode, IconLayoutDashboard } from '@tabler/icons-react'
import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router-dom'
import { CustomResourceDefinition } from 'kubernetes-types/apiextensions/v1'

import { useVersionInfo, useResources } from '@/lib/api'
import { useAuth } from '@/contexts/auth-context'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from '@/components/ui/sidebar'

import { ClusterSelector } from './cluster-selector'
import { Collapsible, CollapsibleTrigger } from './ui/collapsible'
import { VersionInfo } from './version-info'

const SECURITY_RESOURCE_URL_PREFIXES = [
  '/serviceaccounts',
  '/roles',
  '/rolebindings',
  '/clusterroles',
  '/clusterrolebindings',
]

function isSecurityUrl(url: string) {
  return SECURITY_RESOURCE_URL_PREFIXES.some((prefix) => url.startsWith(prefix))
}

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const { t } = useTranslation()
  const location = useLocation()
  const { user } = useAuth()
  const { isMobile, setOpenMobile } = useSidebar()
  const { config, isLoading, getIconComponent } = useSidebarConfig()
  const { data: versionInfo } = useVersionInfo()
  const isAdmin = user?.isAdmin() ?? false

  // Fetch CRDs for auto-classification sidebar section
  const { data: crdItems } = useResources('crds', undefined, {
    staleTime: 30000,
    disable: isLoading || !config,
  })

  // Group CRDs by spec.group
  const crdsByGroup = useMemo(() => {
    if (!crdItems) return new Map<string, CustomResourceDefinition[]>()
    const map = new Map<string, CustomResourceDefinition[]>()
    for (const crd of crdItems) {
      const group = crd.spec?.group ?? 'other'
      if (!map.has(group)) map.set(group, [])
      map.get(group)!.push(crd)
    }
    // Sort each group's CRDs by kind
    map.forEach((crds) =>
      crds.sort((a, b) =>
        (a.spec?.names?.kind ?? '').localeCompare(b.spec?.names?.kind ?? '')
      )
    )
    return map
  }, [crdItems])

  // Sorted group names
  const crdGroupNames = useMemo(
    () => Array.from(crdsByGroup.keys()).sort(),
    [crdsByGroup]
  )

  const securityGroupIds = useMemo(() => {
    if (!config) return new Set<string>()
    return new Set(
      config.groups
        .filter(
          (group) =>
            group.id === 'sidebar-groups-security' ||
            group.nameKey === 'sidebar.groups.security'
        )
        .map((group) => group.id)
    )
  }, [config])

  const securityItemIds = useMemo(() => {
    if (!config || securityGroupIds.size === 0) return new Set<string>()
    const ids = new Set<string>()
    config.groups.forEach((group) => {
      if (securityGroupIds.has(group.id)) {
        group.items.forEach((item) => ids.add(item.id))
      }
    })
    return ids
  }, [config, securityGroupIds])

  const pinnedItems = useMemo(() => {
    if (!config) return []
    return config.groups
      .flatMap((group) => group.items)
      .filter((item) => config.pinnedItems.includes(item.id))
      .filter((item) => !config.hiddenItems.includes(item.id))
      .filter(
        (item) =>
          isAdmin ||
          (!securityItemIds.has(item.id) && !isSecurityUrl(item.url))
      )
  }, [config, isAdmin, securityItemIds])

  const visibleGroups = useMemo(() => {
    if (!config) return []
    return config.groups
      .filter((group) => group.visible)
      .filter((group) => isAdmin || !securityGroupIds.has(group.id))
      .sort((a, b) => a.order - b.order)
      .map((group) => ({
        ...group,
        items: group.items
          .filter((item) => !config.hiddenItems.includes(item.id))
          .filter((item) => !config.pinnedItems.includes(item.id))
          .filter(
            (item) =>
              isAdmin ||
              (!securityItemIds.has(item.id) && !isSecurityUrl(item.url))
          )
          .sort((a, b) => a.order - b.order),
      }))
      .filter((group) => group.items.length > 0)
  }, [config, isAdmin, securityGroupIds, securityItemIds])

  // All namespace items extracted from any group (handles legacy "other" config)
  const allNamespaceItems = useMemo(() => {
    if (!config) return []
    const seen = new Set<string>()
    return config.groups
      .filter((g) => g.visible)
      .flatMap((g) => g.items)
      .filter((item) => item.url === '/namespaces')
      .filter((item) => !config.hiddenItems.includes(item.id))
      .filter((item) => !config.pinnedItems.includes(item.id))
      .filter((item) => {
        if (seen.has(item.url)) return false
        seen.add(item.url)
        return true
      })
  }, [config])

  // Regular groups: exclude namespace/other groups; also strip /namespaces and /crds items
  const regularGroups = useMemo(
    () =>
      visibleGroups
        .filter(
          (g) =>
            g.nameKey !== 'sidebar.groups.namespace' &&
            g.nameKey !== 'sidebar.groups.other'
        )
        .map((group) => ({
          ...group,
          items: group.items.filter(
            (item) => item.url !== '/namespaces' && item.url !== '/crds'
          ),
        }))
        .filter((group) => group.items.length > 0),
    [visibleGroups]
  )

  const isActive = (url: string) => {
    if (url === '/') {
      return location.pathname === '/'
    }
    if (url === '/crds') {
      return location.pathname == '/crds'
    }
    return location.pathname.startsWith(url)
  }

  // Handle menu item click on mobile - close sidebar
  const handleMenuItemClick = () => {
    if (isMobile) {
      setOpenMobile(false)
    }
  }

  // ── Sidebar resize ──────────────────────────────────────────────────────────
  const resizeSentinelRef = useRef<HTMLDivElement>(null)

  // Restore persisted width on mount
  useEffect(() => {
    const saved = localStorage.getItem('kite-sidebar-width')
    if (saved && resizeSentinelRef.current) {
      const wrapper = resizeSentinelRef.current.closest(
        '[data-slot="sidebar-wrapper"]'
      ) as HTMLElement | null
      wrapper?.style.setProperty('--sidebar-width', saved)
    }
  }, [])

  const handleResizeMouseDown = useCallback(
    (e: React.MouseEvent) => {
      if (isMobile) return
      e.preventDefault()
      const wrapper = resizeSentinelRef.current?.closest(
        '[data-slot="sidebar-wrapper"]'
      ) as HTMLElement | null
      if (!wrapper) return

      const gapEl = wrapper.querySelector(
        '[data-slot="sidebar-gap"]'
      ) as HTMLElement | null
      const startWidth = gapEl ? gapEl.offsetWidth : 256
      const startX = e.clientX

      const onMouseMove = (ev: MouseEvent) => {
        const newWidth = Math.min(
          Math.max(startWidth + (ev.clientX - startX), 180),
          520
        )
        wrapper.style.setProperty('--sidebar-width', `${newWidth}px`)
      }
      const onMouseUp = () => {
        const w = wrapper.style.getPropertyValue('--sidebar-width')
        if (w) localStorage.setItem('kite-sidebar-width', w)
        document.removeEventListener('mousemove', onMouseMove)
        document.removeEventListener('mouseup', onMouseUp)
        document.body.style.cursor = ''
        document.body.style.userSelect = ''
      }
      document.body.style.cursor = 'col-resize'
      document.body.style.userSelect = 'none'
      document.addEventListener('mousemove', onMouseMove)
      document.addEventListener('mouseup', onMouseUp)
    },
    [isMobile]
  )
  // ────────────────────────────────────────────────────────────────────────────

  if (isLoading || !config) {
    return (
      <Sidebar collapsible="offcanvas" {...props}>
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                asChild
                className="data-[slot=sidebar-menu-button]:!p-1.5 hover:bg-accent/50 transition-colors"
               >
                <Link to="/" onClick={handleMenuItemClick}>
                  <img src={Icon} alt="Kite Logo" className="ml-1 h-8 w-8" />
                  <span className="text-base font-semibold">Kite</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent>
          <div className="p-4 text-center text-muted-foreground">
            {t('common.loading', 'Loading...')}
          </div>
        </SidebarContent>
      </Sidebar>
    )
  }

  return (
    <Sidebar collapsible="offcanvas" className="overflow-visible" {...props}>
      {/* DOM sentinel for sidebar-wrapper traversal */}
      <div ref={resizeSentinelRef} className="sr-only" aria-hidden="true" />

      {/* Resize handle — right edge of the sidebar */}
      <div
        onMouseDown={handleResizeMouseDown}
        className="absolute inset-y-0 right-0 z-50 w-1.5 cursor-col-resize group/resize hidden md:flex items-center justify-center"
        aria-hidden="true"
      >
        <div className="h-full w-px bg-border/40 group-hover/resize:w-1 group-hover/resize:bg-primary/50 group-hover/resize:shadow-[0_0_4px] group-hover/resize:shadow-primary/30 transition-all duration-150 rounded-full" />
      </div>

      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              asChild
              className="data-[slot=sidebar-menu-button]:!p-1.5 hover:bg-accent/50 transition-colors"
            >
              <Link to="/" onClick={handleMenuItemClick}>
                <div className="relative flex items-center justify-between w-full">
                  <div className="flex items-center gap-2">
                    <img src={Icon} alt="Kite Logo" className="h-8 w-8" />
                    <div className="flex flex-col">
                      <span className="text-base font-semibold bg-gradient-to-r from-primary to-primary/70 bg-clip-text text-transparent">
                        Kite
                      </span>
                      <VersionInfo />
                    </div>
                  </div>
                  {versionInfo?.hasNewVersion ? (
                    <button
                      type="button"
                      onClick={(e) => {
                        e.preventDefault()
                        e.stopPropagation()
                        if (versionInfo?.releaseUrl) {
                          window.open(versionInfo.releaseUrl, '_blank')
                        }
                      }}
                      className="absolute right-0 top-0 mr-1 mt-1 rounded-sm bg-red-500/10 px-1.5 py-0.5 text-[9px] font-semibold uppercase text-red-500 hover:bg-red-500/20"
                      title={t(
                        'A newer Kite version is available',
                        'A newer Kite version is available'
                      )}
                    >
                      New
                    </button>
                  ) : null}
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                tooltip={t('nav.overview')}
                asChild
                isActive={isActive('/')}
                className="transition-all duration-200 hover:bg-accent/60 active:scale-95 data-[active=true]:bg-primary/10 data-[active=true]:text-primary data-[active=true]:shadow-sm"
              >
                <Link to="/" onClick={handleMenuItemClick}>
                  <IconLayoutDashboard className="text-sidebar-primary" />
                  <span className="font-medium">{t('nav.overview')}</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>

        {pinnedItems.length > 0 && (
          <SidebarGroup>
            <SidebarGroupLabel className="text-xs font-bold uppercase tracking-wide text-muted-foreground">
              {t('sidebar.pinned', 'Pinned')}
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {pinnedItems.map((item) => {
                  const IconComponent = getIconComponent(item.icon)
                  const title = item.titleKey
                    ? t(item.titleKey, { defaultValue: item.titleKey })
                    : ''
                  return (
                    <SidebarMenuItem key={item.id}>
                      <SidebarMenuButton
                        tooltip={title}
                        asChild
                        isActive={isActive(item.url)}
                        className="transition-all duration-200 hover:bg-accent/60 active:scale-95 data-[active=true]:bg-primary/10 data-[active=true]:text-primary data-[active=true]:shadow-sm"
                      >
                        <Link to={item.url} onClick={handleMenuItemClick}>
                          <IconComponent className="text-sidebar-primary" />
                          <span>{title}</span>
                        </Link>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  )
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        )}

        {regularGroups.map((group) => (
          <Collapsible
            key={group.id}
            defaultOpen={!group.collapsed}
            className="group/collapsible"
          >
            <SidebarGroup>
              <SidebarGroupLabel asChild>
                <CollapsibleTrigger className="flex items-center justify-between w-full text-xs font-bold uppercase tracking-wide text-muted-foreground hover:text-foreground hover:bg-accent/60 active:bg-accent/80 rounded-md transition-all duration-150 group-data-[state=open]:text-foreground group-data-[state=open]:bg-accent/30">
                  <span>
                    {group.nameKey
                      ? t(group.nameKey, { defaultValue: group.nameKey })
                      : ''}
                  </span>
                  <ChevronDown className="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-180" />
                </CollapsibleTrigger>
              </SidebarGroupLabel>
              <CollapsibleContent>
                <SidebarGroupContent className="flex flex-col gap-2">
                  <SidebarMenu>
                    {group.items.map((item) => {
                      const IconComponent = getIconComponent(item.icon)
                      const title = item.titleKey
                        ? t(item.titleKey, { defaultValue: item.titleKey })
                        : ''
                      return (
                        <SidebarMenuItem key={item.id}>
                          <SidebarMenuButton
                            tooltip={title}
                            asChild
                            isActive={isActive(item.url)}
                            className="transition-all duration-200 hover:bg-accent/60 active:scale-95 data-[active=true]:bg-primary/10 data-[active=true]:text-primary data-[active=true]:shadow-sm"
                          >
                            <Link to={item.url} onClick={handleMenuItemClick}>
                              <IconComponent className="text-sidebar-primary" />
                              <span>{title}</span>
                            </Link>
                          </SidebarMenuButton>
                        </SidebarMenuItem>
                      )
                    })}
                  </SidebarMenu>
                </SidebarGroupContent>
              </CollapsibleContent>
            </SidebarGroup>
          </Collapsible>
        ))}

        {/* Namespace - standalone row without group header */}
        {allNamespaceItems.map((item) => {
          const IconComponent = getIconComponent(item.icon)
          const title = item.titleKey
            ? t(item.titleKey, { defaultValue: item.titleKey })
            : ''
          return (
            <SidebarGroup key={item.id} className="py-0">
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    tooltip={title}
                    asChild
                    isActive={isActive(item.url)}
                    className="transition-all duration-200 hover:bg-accent/60 active:scale-95 data-[active=true]:bg-primary/10 data-[active=true]:text-primary data-[active=true]:shadow-sm"
                  >
                    <Link to={item.url} onClick={handleMenuItemClick}>
                      <IconComponent className="text-sidebar-primary" />
                      <span className="font-medium">{title}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroup>
          )
        })}

        {/* Custom Resources - auto-classified by API group */}
        {crdGroupNames.length > 0 && (
          <Collapsible defaultOpen={false} className="group/collapsible">
            <SidebarGroup>
              <SidebarGroupLabel asChild>
                <CollapsibleTrigger className="flex items-center justify-between w-full text-xs font-bold uppercase tracking-wide text-muted-foreground hover:text-foreground hover:bg-accent/60 active:bg-accent/80 rounded-md transition-all duration-150 group-data-[state=open]:text-foreground group-data-[state=open]:bg-accent/30">
                  <span>
                    {t('nav.customResources', 'Custom Resources')}
                  </span>
                  <ChevronDown className="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-180" />
                </CollapsibleTrigger>
              </SidebarGroupLabel>
              <CollapsibleContent>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {/* Link to CRD definitions page */}
                    <SidebarMenuItem>
                      <SidebarMenuButton
                        tooltip={t('nav.crds', 'CRDs')}
                        asChild
                        isActive={location.pathname === '/crds'}
                        className="transition-all duration-200 hover:bg-accent/60 data-[active=true]:bg-primary/10 data-[active=true]:text-primary"
                      >
                        <Link to="/crds" onClick={handleMenuItemClick}>
                          <IconCode className="text-sidebar-primary" />
                          <span>{t('sidebar.crd.definitions', 'Definitions')}</span>
                        </Link>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  </SidebarMenu>

                  {/* API group collapsibles */}
                  {crdGroupNames.map((groupName) => {
                    const crds = crdsByGroup.get(groupName) ?? []
                    return (
                      <Collapsible
                        key={groupName}
                        defaultOpen={false}
                        className="group/crd-group"
                      >
                        <SidebarMenu>
                          <SidebarMenuItem>
                            <CollapsibleTrigger asChild>
                              <SidebarMenuButton
                                tooltip={groupName}
                                className="hover:bg-accent/60 active:bg-accent/80 transition-all duration-150"
                              >
                                <IconBoxMultiple className="text-sidebar-primary shrink-0" />
                                <span className="truncate text-xs font-mono">{groupName}</span>
                                <ChevronDown className="ml-auto shrink-0 transition-transform duration-200 group-data-[state=open]/crd-group:rotate-180" />
                              </SidebarMenuButton>
                            </CollapsibleTrigger>
                          </SidebarMenuItem>
                        </SidebarMenu>
                        <CollapsibleContent>
                          <SidebarMenu className="ml-3 border-l border-border/50 pl-2">
                            {crds.map((crd) => {
                              const crdName = crd.metadata?.name ?? ''
                              const kind = crd.spec?.names?.kind ?? crdName
                              const url = `/crds/${crdName}`
                              return (
                                <SidebarMenuItem key={crdName}>
                                  <SidebarMenuButton
                                    tooltip={kind}
                                    asChild
                                    isActive={location.pathname.startsWith(url)}
                                    className="transition-all duration-200 hover:bg-accent/60 data-[active=true]:bg-primary/10 data-[active=true]:text-primary"
                                  >
                                    <Link to={url} onClick={handleMenuItemClick}>
                                      <IconCode className="text-sidebar-primary shrink-0" />
                                      <span className="truncate">{kind}</span>
                                    </Link>
                                  </SidebarMenuButton>
                                </SidebarMenuItem>
                              )
                            })}
                          </SidebarMenu>
                        </CollapsibleContent>
                      </Collapsible>
                    )
                  })}
                </SidebarGroupContent>
              </CollapsibleContent>
            </SidebarGroup>
          </Collapsible>
        )}
      </SidebarContent>

      <SidebarFooter>
        <div className="flex items-center gap-2 rounded-md px-2 py-1.5 bg-gradient-to-r from-muted/40 to-muted/20 border border-border/60 backdrop-blur-sm">
          <ClusterSelector />
        </div>
      </SidebarFooter>
    </Sidebar>
  )
}
