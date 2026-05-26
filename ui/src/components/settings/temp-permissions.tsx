import React, { useMemo } from 'react'
import { IconLoader2 } from '@tabler/icons-react'
import { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AccessRequest,
  AccessRequestStatus,
  useAllAccessRequests,
  useRevokeAccess,
} from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

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

function formatExpiry(expiresAt?: string): React.ReactNode {
  if (!expiresAt) return <span className="text-muted-foreground">-</span>
  const d = new Date(expiresAt)
  const now = new Date()
  const diffMs = d.getTime() - now.getTime()
  const dateStr = d.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
  if (diffMs <= 0)
    return <span className="text-destructive text-xs">{dateStr}（已过期）</span>
  const diffH = Math.floor(diffMs / 3_600_000)
  const diffM = Math.floor((diffMs % 3_600_000) / 60_000)
  let remaining: string
  if (diffH >= 24) {
    remaining = `${Math.floor(diffH / 24)} 天`
  } else if (diffH > 0) {
    remaining = `${diffH} 小时 ${diffM} 分`
  } else {
    remaining = `${diffM} 分钟`
  }
  return (
    <span className="text-xs">
      {dateStr}
      <span className="ml-1 text-muted-foreground">（剩余 {remaining}）</span>
    </span>
  )
}

function formatDuration(hours: number): string {
  if (hours < 24) return `${hours} 小时`
  const days = Math.floor(hours / 24)
  const rem = hours % 24
  return rem === 0 ? `${days} 天` : `${days} 天 ${rem} 小时`
}

export function TempPermissionsManagement() {
  const { t } = useTranslation()
  const { data: requests = [], isLoading, error } = useAllAccessRequests()
  const revokeMutation = useRevokeAccess()

  const columns = useMemo<ColumnDef<AccessRequest>[]>(
    () => [
      {
        id: 'requester',
        header: t('tempPermissions.table.requester', '申请人'),
        cell: ({ row: { original: r } }) => (
          <span className="font-medium">{r.requesterName || `#${r.requesterId}`}</span>
        ),
      },
      {
        id: 'namespace',
        header: t('tempPermissions.table.namespace', '命名空间'),
        cell: ({ row: { original: r } }) => (
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{r.namespace}</code>
        ),
      },
      {
        id: 'duration',
        header: t('tempPermissions.table.duration', '时长'),
        cell: ({ row: { original: r } }) => (
          <span className="text-sm">{formatDuration(r.durationHours)}</span>
        ),
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
          <span className="text-sm text-muted-foreground">{r.approverName || '-'}</span>
        ),
      },
      {
        id: 'expiresAt',
        header: t('tempPermissions.table.expiresAt', '过期时间'),
        cell: ({ row: { original: r } }) => formatExpiry(r.expiresAt),
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
    [t]
  )

  const actions = useMemo<Action<AccessRequest>[]>(
    () => [
      {
        label: t('tempPermissions.revoke', '吊销权限'),
        shouldDisable: (r) => r.status !== 'approved',
        onClick: async (r) => {
          if (r.status !== 'approved') return
          try {
            await revokeMutation.mutateAsync(r.id)
            toast.success(
              t('tempPermissions.revokeSuccess', '已吊销 {{name}} 对 {{ns}} 的权限', {
                name: r.requesterName,
                ns: r.namespace,
              })
            )
          } catch {
            toast.error(t('tempPermissions.revokeError', '吊销失败'))
          }
        },
      },
    ],
    [t, revokeMutation]
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
          />
        )}
      </CardContent>
    </Card>
  )
}
