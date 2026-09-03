import type { ComponentType, ReactNode } from 'react'
import type { TFunction } from 'i18next'

import type { AuthUser } from '@/lib/api'

import { AccessRequestSettings } from './access-request-settings'
import { APIKeyManagement } from './apikey-management'
import { AuditLog } from './audit-log'
import { AuthenticationManagement } from './authentication-management'
import { ClusterManagement } from './cluster-management'
import { GeneralManagement } from './general-management'
import { RBACManagement } from './rbac-management'
import { TempPermissionsManagement } from './temp-permissions'
import { TemplateManagement } from './template-management'
import { UserGroupManagement } from './user-group-management'
import { UserManagement } from './user-management'

export interface SettingsSectionDefinition {
  value: string
  labelKey: string
  defaultLabel: string
  render: () => ReactNode
  requiresAdmin?: boolean
}

function createSettingsSectionDefinition(
  value: string,
  labelKey: string,
  defaultLabel: string,
  Component: ComponentType,
  requiresAdmin: boolean = false
): SettingsSectionDefinition {
  return {
    value,
    labelKey,
    defaultLabel,
    render: () => <Component />,
    requiresAdmin,
  }
}

export const settingsSectionRegistry: SettingsSectionDefinition[] = [
  createSettingsSectionDefinition(
    'general',
    'settings.tabs.general',
    'General',
    GeneralManagement,
    true
  ),
  createSettingsSectionDefinition(
    'clusters',
    'settings.tabs.clusters',
    'Cluster',
    ClusterManagement,
    true
  ),
  createSettingsSectionDefinition(
    'oauth',
    'settings.tabs.oauth',
    'Authentication',
    AuthenticationManagement,
    true
  ),
  createSettingsSectionDefinition(
    'rbac',
    'settings.tabs.rbac',
    'RBAC',
    RBACManagement,
    true
  ),
  createSettingsSectionDefinition(
    'users',
    'settings.tabs.users',
    'User',
    UserManagement,
    true
  ),
  createSettingsSectionDefinition(
    'user-groups',
    'settings.tabs.userGroups',
    'User Groups',
    UserGroupManagement,
    true
  ),
  createSettingsSectionDefinition(
    'apikeys',
    'settings.tabs.apikeys',
    'API Keys',
    APIKeyManagement,
    true
  ),
  createSettingsSectionDefinition(
    'templates',
    'settings.tabs.templates',
    'Templates',
    TemplateManagement
  ),
  createSettingsSectionDefinition(
    'audit',
    'settings.tabs.audit',
    'Audit',
    AuditLog,
    true
  ),
  createSettingsSectionDefinition(
    'access-request',
    'settings.tabs.accessRequest',
    'Access Request',
    AccessRequestSettings,
    true
  ),
  createSettingsSectionDefinition(
    'temp-permissions',
    'settings.tabs.tempPermissions',
    'Temp Permissions',
    TempPermissionsManagement,
    true
  ),
]

export function createSettingsTabs(t: TFunction, user?: AuthUser | null) {
  const isAdmin =
    user?.roles?.some((role: { name: string }) => role.name === 'admin') ??
    false

  return settingsSectionRegistry
    .filter((section) => !section.requiresAdmin || isAdmin)
    .map((section) => ({
      value: section.value,
      label: t(section.labelKey, section.defaultLabel),
      content: section.render(),
    }))
}
