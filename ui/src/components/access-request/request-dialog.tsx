import { useState } from 'react'
import { IconLoader2, IconShieldPlus } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  useCreateAccessRequest,
  useFeishuApprovers,
} from '@/lib/api'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

const DURATION_OPTIONS = [
  { label: '1 小时', value: 1 },
  { label: '2 小时', value: 2 },
  { label: '4 小时', value: 4 },
  { label: '8 小时', value: 8 },
  { label: '12 小时', value: 12 },
  { label: '1 天', value: 24 },
  { label: '2 天', value: 48 },
  { label: '3 天', value: 72 },
  { label: '7 天', value: 168 },
]

interface AccessRequestDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AccessRequestDialog({
  open,
  onOpenChange,
}: AccessRequestDialogProps) {
  const { t } = useTranslation()
  const { data: approvers = [] } = useFeishuApprovers()
  const createMutation = useCreateAccessRequest()

  const [namespace, setNamespace] = useState('')
  const [durationHours, setDurationHours] = useState<number>(4)
  const [reason, setReason] = useState('')
  const [approverUid, setApproverUid] = useState('')

  const resetForm = () => {
    setNamespace('')
    setDurationHours(4)
    setReason('')
    setApproverUid('')
  }

  const handleSubmit = async () => {
    if (!namespace.trim()) {
      toast.error(t('accessRequest.errors.namespaceRequired', '请填写命名空间'))
      return
    }
    if (!reason.trim()) {
      toast.error(t('accessRequest.errors.reasonRequired', '请填写申请原因'))
      return
    }
    if (approvers.length > 0 && !approverUid) {
      toast.error(t('accessRequest.errors.approverRequired', '请选择审批人'))
      return
    }
    const selectedApprover = approvers.find((a) => a.openId === approverUid)
    try {
      await createMutation.mutateAsync({
        namespace: namespace.trim(),
        durationHours,
        reason: reason.trim(),
        approverUid,
        approverName: selectedApprover?.name,
      })
      toast.success(t('accessRequest.createSuccess', '申请已提交，请等待审批'))
      onOpenChange(false)
      resetForm()
    } catch {
      toast.error(t('accessRequest.createError', '申请提交失败'))
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconShieldPlus className="h-5 w-5 text-primary" />
            {t('accessRequest.title', '申请命名空间权限')}
          </DialogTitle>
        </DialogHeader>

        <div className="grid gap-4 py-2">
          {/* Namespace */}
          <div className="grid gap-1.5">
            <Label htmlFor="ar-namespace">
              {t('accessRequest.fields.namespace', '命名空间')}{' '}
              <span className="text-destructive">*</span>
            </Label>
            <Input
              id="ar-namespace"
              placeholder={t(
                'accessRequest.fields.namespacePlaceholder',
                '例如: production'
              )}
              value={namespace}
              onChange={(e) => setNamespace(e.target.value)}
            />
          </div>

          {/* Duration */}
          <div className="grid gap-1.5">
            <Label htmlFor="ar-duration">
              {t('accessRequest.fields.duration', '使用时长')}{' '}
              <span className="text-destructive">*</span>
            </Label>
            <Select
              value={String(durationHours)}
              onValueChange={(v) => setDurationHours(Number(v))}
            >
              <SelectTrigger id="ar-duration">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {DURATION_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value} value={String(opt.value)}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Approver */}
          {approvers.length > 0 && (
            <div className="grid gap-1.5">
              <Label htmlFor="ar-approver">
                {t('accessRequest.fields.approver', '审批人')}{' '}
                <span className="text-destructive">*</span>
              </Label>
              <Select value={approverUid} onValueChange={setApproverUid}>
                <SelectTrigger id="ar-approver">
                  <SelectValue
                    placeholder={t(
                      'accessRequest.fields.approverPlaceholder',
                      '请选择审批人'
                    )}
                  />
                </SelectTrigger>
                <SelectContent>
                  {approvers.map((a) => (
                    <SelectItem key={a.openId} value={a.openId}>
                      {a.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          {/* Reason */}
          <div className="grid gap-1.5">
            <Label htmlFor="ar-reason">
              {t('accessRequest.fields.reason', '申请原因')}{' '}
              <span className="text-destructive">*</span>
            </Label>
            <Textarea
              id="ar-reason"
              placeholder={t(
                'accessRequest.fields.reasonPlaceholder',
                '请说明申请原因...'
              )}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              rows={3}
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={createMutation.isPending}
          >
            {t('common.cancel', '取消')}
          </Button>
          <Button onClick={handleSubmit} disabled={createMutation.isPending}>
            {createMutation.isPending && (
              <IconLoader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            {t('accessRequest.submit', '提交申请')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
