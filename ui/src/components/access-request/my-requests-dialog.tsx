import { useMemo } from 'react'
import {
  IconBell,
  IconClock,
  IconLoader2,
  IconX,
} from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AccessRequest,
  AccessRequestStatus,
  remindAccessRequest,
  useMyAccessRequests,
  useWithdrawAccessRequest,
} from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

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

function statusLabel(status: AccessRequestStatus, t: (k: string, d: string) => string): string {
  const map: Record<AccessRequestStatus, [string, string]> = {
    pending: ['accessRequest.status.pending', '审批中'],
    approved: ['accessRequest.status.approved', '已批准'],
    rejected: ['accessRequest.status.rejected', '已拒绝'],
    withdrawn: ['accessRequest.status.withdrawn', '已撤回'],
    expired: ['accessRequest.status.expired', '已过期'],
  }
  const [key, def] = map[status] ?? [`accessRequest.status.${status}`, status]
  return t(key, def)
}

function formatDuration(hours: number): string {
  if (hours < 24) return `${hours} 小时`
  const days = Math.floor(hours / 24)
  const rem = hours % 24
  return rem === 0 ? `${days} 天` : `${days} 天 ${rem} 小时`
}

function formatExpiry(expiresAt?: string): string {
  if (!expiresAt) return '-'
  const d = new Date(expiresAt)
  const now = new Date()
  const diffMs = d.getTime() - now.getTime()
  if (diffMs <= 0) return '已过期'
  const diffH = Math.floor(diffMs / 3_600_000)
  const diffM = Math.floor((diffMs % 3_600_000) / 60_000)
  if (diffH >= 24) {
    const days = Math.floor(diffH / 24)
    return `${days} 天后过期`
  }
  if (diffH > 0) return `${diffH} 小时 ${diffM} 分后过期`
  return `${diffM} 分后过期`
}

interface RequestRowProps {
  req: AccessRequest
}

function RequestRow({ req }: RequestRowProps) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const withdraw = useWithdrawAccessRequest()
  const remind = useMutation({
    mutationFn: () => remindAccessRequest(req.id),
    onSuccess: () => toast.success(t('accessRequest.remindSuccess', '催办通知已发送')),
    onError: () => toast.error(t('accessRequest.remindError', '催办发送失败')),
  })

  const handleWithdraw = async () => {
    try {
      await withdraw.mutateAsync(req.id)
      toast.success(t('accessRequest.withdrawSuccess', '申请已撤回'))
      qc.invalidateQueries({ queryKey: ['my-access-requests'] })
    } catch {
      toast.error(t('accessRequest.withdrawError', '撤回失败'))
    }
  }

  return (
    <div className="rounded-lg border p-4 space-y-3">
      <div className="flex items-start justify-between gap-2">
        <div className="space-y-0.5">
          <div className="flex items-center gap-2">
            <span className="font-medium text-sm font-mono">{req.namespace}</span>
            <Badge variant={statusVariant(req.status)}>
              {statusLabel(req.status, t)}
            </Badge>
          </div>
          <p className="text-xs text-muted-foreground">
            {t('accessRequest.fields.duration', '时长')}：{formatDuration(req.durationHours)}
            {req.approverName ? ` · ${t('accessRequest.fields.approver', '审批人')}：${req.approverName}` : ''}
          </p>
          {req.status === 'approved' && req.expiresAt && (
            <div className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
              <IconClock className="h-3 w-3" />
              {formatExpiry(req.expiresAt)}
            </div>
          )}
          {req.reviewNote && (
            <p className="text-xs text-muted-foreground italic">
              {t('accessRequest.fields.reviewNote', '审批意见')}：{req.reviewNote}
            </p>
          )}
        </div>

        {req.status === 'pending' && (
          <div className="flex gap-1 shrink-0">
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs"
              disabled={remind.isPending}
              onClick={() => remind.mutate()}
              title={t('accessRequest.remind', '催办')}
            >
              {remind.isPending ? (
                <IconLoader2 className="h-3 w-3 animate-spin" />
              ) : (
                <IconBell className="h-3 w-3" />
              )}
              <span className="ml-1 hidden sm:inline">
                {t('accessRequest.remind', '催办')}
              </span>
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs text-destructive hover:text-destructive"
              disabled={withdraw.isPending}
              onClick={handleWithdraw}
              title={t('accessRequest.withdraw', '撤回')}
            >
              {withdraw.isPending ? (
                <IconLoader2 className="h-3 w-3 animate-spin" />
              ) : (
                <IconX className="h-3 w-3" />
              )}
              <span className="ml-1 hidden sm:inline">
                {t('accessRequest.withdraw', '撤回')}
              </span>
            </Button>
          </div>
        )}
      </div>

      <p className="text-xs text-muted-foreground line-clamp-2">
        {req.reason}
      </p>

      <p className="text-xs text-muted-foreground">
        {t('accessRequest.fields.createdAt', '提交时间')}：
        {new Date(req.createdAt).toLocaleString('zh-CN')}
      </p>
    </div>
  )
}

interface MyRequestsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function MyRequestsDialog({ open, onOpenChange }: MyRequestsDialogProps) {
  const { t } = useTranslation()
  const { data: requests = [], isLoading } = useMyAccessRequests()

  const sorted = useMemo(
    () =>
      [...requests].sort(
        (a, b) =>
          new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
      ),
    [requests]
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg max-h-[80vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>
            {t('accessRequest.myRequestsTitle', '我的权限申请')}
          </DialogTitle>
        </DialogHeader>

        <div className="flex-1 overflow-y-auto space-y-3 pr-1 pb-2">
          {isLoading ? (
            <div className="flex justify-center py-8">
              <IconLoader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : sorted.length === 0 ? (
            <p className="text-center text-sm text-muted-foreground py-8">
              {t('accessRequest.noRequests', '暂无申请记录')}
            </p>
          ) : (
            sorted.map((req) => <RequestRow key={req.id} req={req} />)
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
