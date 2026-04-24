import type { ComponentType, ReactNode } from 'react'
import type { TFunction } from 'i18next'
import type { User } from '@/types/api'

import { APIKeyManagement } from './apikey-management'
import { AuditLog } from './audit-log'
import { AuthenticationManagement } from './authentication-management'
import { ClusterManagement } from './cluster-management'
import { GeneralManagement } from './general-management'
import { RBACManagement } from './rbac-management'
import { TemplateManagement } from './template-management'
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
]

export function createSettingsTabs(t: TFunction, user?: User | null) {
  const isAdmin = user?.roles?.some((role) => role.name === 'admin') ?? false

  return settingsSectionRegistry
    .filter((section) => !section.requiresAdmin || isAdmin)
    .map((section) => ({
      value: section.value,
      label: t(section.labelKey, section.defaultLabel),
      content: section.render(),
    }))
}
