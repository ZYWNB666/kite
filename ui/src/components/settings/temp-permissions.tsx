import React, { useMemo, useState } from 'react'
import { IconLoader2 } from '@tabler/icons-react'
import { ColumnDef, PaginationState } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AccessRequest,
  AccessRequestStatus,
  useAllAccessRequests,
  useApproveAccess,
  useRevokeAccess,
} from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

import { Action, ActionTable } from '../action-table'

function statusVariant(
  status: AccessRequestStatus
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'approved':
      return 'default'
    case 'rejected':
    case 'expired':
      return 'destructive'
    case 'withdrawn':
      return 'secondary'
    default:
      return 'outline'
  }
}

function formatExpiry(
  expiresAt: string | undefined,
  t: (k: string, o?: { count?: number }) => string,
  language: string
): React.ReactNode {
  if (!expiresAt) return <span className="text-muted-foreground">-</span>
  const d = new Date(expiresAt)
  const now = new Date()
  const diffMs = d.getTime() - now.getTime()
  const dateStr = d.toLocaleString(
    language.startsWith('zh') ? 'zh-CN' : 'en-US',
    {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    }
  )
  if (diffMs <= 0)
    return (
      <span className="text-destructive text-xs">
        {dateStr} ({t('time.expired')})
      </span>
    )
  const diffH = Math.floor(diffMs / 3_600_000)
  const diffM = Math.floor((diffMs % 3_600_000) / 60_000)
  let remaining: string
  if (diffH >= 24) {
    remaining = t('time.days', { count: Math.floor(diffH / 24) })
  } else if (diffH > 0) {
    remaining = `${t('time.hours', { count: diffH })} ${t('time.minutes', { count: diffM })}`
  } else {
    remaining = t('time.minutes', { count: diffM })
  }
  return (
    <span className="text-xs">
      {dateStr}
      <span className="ml-1 text-muted-foreground">
        ({t('time.remaining')} {remaining})
      </span>
    </span>
  )
}

function formatDuration(
  hours: number,
  t: (k: string, o?: { count?: number }) => string
): string {
  if (hours < 24) return t('time.hours', { count: hours })
  const days = Math.floor(hours / 24)
  const rem = hours % 24
  return rem === 0
    ? t('time.days', { count: days })
    : `${t('time.days', { count: days })} ${t('time.hours', { count: rem })}`
}

function formatRiskLevel(
  level: string,
  t: (k: string) => string
): React.ReactNode {
  const map: Record<string, { key: string; className: string }> = {
    low: {
      key: 'accessRequest.risk.low',
      className: 'text-green-600 dark:text-green-400',
    },
    medium: {
      key: 'accessRequest.risk.medium',
      className: 'text-yellow-600 dark:text-yellow-400',
    },
    high: {
      key: 'accessRequest.risk.high',
      className: 'text-red-600 dark:text-red-400',
    },
  }
  const item = map[level]
  if (!item)
    return <span className="text-xs text-muted-foreground">{level || '-'}</span>
  return (
    <span className={`text-xs font-medium ${item.className}`}>
      {t(item.key)}
    </span>
  )
}

export function TempPermissionsManagement() {
  const { t, i18n } = useTranslation()
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 20,
  })
  const { data, isLoading, error } = useAllAccessRequests(
    pagination.pageIndex + 1,
    pagination.pageSize
  )
  const requests = data?.requests ?? []

  const revokeMutation = useRevokeAccess()
  const approveMutation = useApproveAccess()

  const columns = useMemo<ColumnDef<AccessRequest>[]>(
    () => [
      {
        id: 'requester',
        header: t('tempPermissions.table.requester', '申请人'),
        cell: ({ row: { original: r } }) => (
          <span className="font-medium">
            {r.requesterName || `#${r.requesterId}`}
          </span>
        ),
      },
      {
        id: 'namespace',
        header: t('tempPermissions.table.namespace', '命名空间'),
        cell: ({ row: { original: r } }) => (
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
            {r.namespace}
          </code>
        ),
      },
      {
        id: 'duration',
        header: t('tempPermissions.table.duration', '时长'),
        cell: ({ row: { original: r } }) => (
          <span className="text-sm">{formatDuration(r.durationHours, t)}</span>
        ),
      },
      {
        id: 'riskLevel',
        header: t('tempPermissions.table.riskLevel', '预估风险'),
        cell: ({ row: { original: r } }) => formatRiskLevel(r.riskLevel, t),
      },
      {
        id: 'status',
        header: t('common.status', '状态'),
        cell: ({ row: { original: r } }) => (
          <Badge variant={statusVariant(r.status)}>
            {t(`accessRequest.status.${r.status}`, r.status)}
          </Badge>
        ),
      },
      {
        id: 'approver',
        header: t('tempPermissions.table.approver', '审批人'),
        cell: ({ row: { original: r } }) => (
          <span className="text-sm text-muted-foreground">
            {r.approverName || '-'}
          </span>
        ),
      },
      {
        id: 'expiresAt',
        header: t('tempPermissions.table.expiresAt', '过期时间'),
        cell: ({ row: { original: r } }) =>
          formatExpiry(r.expiresAt, t, i18n.language),
      },
      {
        id: 'reason',
        header: t('accessRequest.fields.reason', '申请原因'),
        cell: ({ row: { original: r } }) => (
          <span
            className="text-xs text-muted-foreground max-w-[180px] truncate inline-block"
            title={r.reason}
          >
            {r.reason}
          </span>
        ),
      },
    ],
    [t, i18n.language]
  )

  const actions = useMemo<Action<AccessRequest>[]>(
    () => [
      {
        label: t('tempPermissions.approve', '审批通过'),
        shouldDisable: (r) => r.status !== 'pending',
        onClick: async (r) => {
          if (r.status !== 'pending') return
          try {
            await approveMutation.mutateAsync(r.id)
            toast.success(
              t(
                'tempPermissions.approveSuccess',
                '已批准 {{name}} 的权限申请',
                {
                  name: r.requesterName,
                }
              )
            )
          } catch {
            toast.error(t('tempPermissions.approveError', '审批失败'))
          }
        },
      },
      {
        label: t('tempPermissions.revoke', '吊销权限'),
        shouldDisable: (r) => r.status !== 'approved',
        onClick: async (r) => {
          if (r.status !== 'approved') return
          try {
            await revokeMutation.mutateAsync(r.id)
            toast.success(
              t(
                'tempPermissions.revokeSuccess',
                '已吊销 {{name}} 对 {{ns}} 的权限',
                {
                  name: r.requesterName,
                  ns: r.namespace,
                }
              )
            )
          } catch {
            toast.error(t('tempPermissions.revokeError', '吊销失败'))
          }
        },
      },
    ],
    [t, revokeMutation, approveMutation]
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('tempPermissions.title', '临时权限管理')}</CardTitle>
        <CardDescription>
          {t(
            'tempPermissions.description',
            '查看所有权限申请记录，对已批准的临时权限执行吊销操作。'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="flex justify-center py-8">
            <IconLoader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : error ? (
          <p className="text-sm text-destructive py-4">
            {t('common.error', '加载失败')}
          </p>
        ) : (
          <ActionTable
            data={requests}
            columns={columns}
            actions={actions}
            pagination={{
              state: pagination,
              setPagination,
              total: data?.total ?? 0,
            }}
          />
        )}
      </CardContent>
    </Card>
  )
}
