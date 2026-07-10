import { useState } from 'react'
import { IconChevronDown, IconLoader2, IconShieldPlus, IconX } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  useCreateAccessRequest,
  useFeishuApprovers,
} from '@/lib/api'
import { useResources } from '@/lib/api'
import { useCurrentClusterList } from '@/lib/api/cluster'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
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
]

const RISK_OPTIONS = [
  { label: '🟢 低风险', value: 'low' },
  { label: '🟡 中风险', value: 'medium' },
  { label: '🔴 高风险', value: 'high' },
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
  const { data: clusters = [] } = useCurrentClusterList({ enabled: open })
  const { data: nsItems = [] } = useResources('namespaces')

  const namespaceNames = nsItems
    .map((ns) => ns.metadata?.name)
    .filter((n): n is string => !!n)
    .sort()

  const [selectedCluster, setSelectedCluster] = useState('')
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([])
  const [nsPopoverOpen, setNsPopoverOpen] = useState(false)
  const [durationHours, setDurationHours] = useState<number>(4)
  const [riskLevel, setRiskLevel] = useState<string>('low')
  const [reason, setReason] = useState('')
  const [approverUid, setApproverUid] = useState('')

  const resetForm = () => {
    setSelectedCluster('')
    setSelectedNamespaces([])
    setDurationHours(4)
    setRiskLevel('low')
    setReason('')
    setApproverUid('')
  }

  const toggleNamespace = (ns: string) => {
    setSelectedNamespaces((prev) =>
      prev.includes(ns) ? prev.filter((n) => n !== ns) : [...prev, ns]
    )
  }

  const removeNamespace = (ns: string) => {
    setSelectedNamespaces((prev) => prev.filter((n) => n !== ns))
  }

  const handleSubmit = async () => {
    if (!selectedCluster) {
      toast.error(t('accessRequest.errors.clusterRequired', '请选择集群'))
      return
    }
    if (selectedNamespaces.length === 0) {
      toast.error(t('accessRequest.errors.namespaceRequired', '请选择命名空间'))
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
        cluster: selectedCluster,
        namespaces: selectedNamespaces,
        durationHours,
        riskLevel,
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
          {/* Cluster selector */}
          <div className="grid gap-1.5">
            <Label htmlFor="ar-cluster">
              {t('accessRequest.fields.cluster', '集群')}{' '}
              <span className="text-destructive">*</span>
            </Label>
            <Select value={selectedCluster} onValueChange={setSelectedCluster}>
              <SelectTrigger id="ar-cluster">
                <SelectValue
                  placeholder={t('accessRequest.fields.clusterPlaceholder', '请选择集群')}
                />
              </SelectTrigger>
              <SelectContent>
                {clusters.map((c) => (
                  <SelectItem key={c.name} value={c.name}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Namespace multi-select */}
          <div className="grid gap-1.5">
            <Label>
              {t('accessRequest.fields.namespace', '命名空间')}{' '}
              <span className="text-destructive">*</span>
            </Label>
            <Popover open={nsPopoverOpen} onOpenChange={setNsPopoverOpen}>
              <PopoverTrigger asChild>
                <button
                  type="button"
                  className="border-input bg-background ring-offset-background focus-visible:ring-ring flex min-h-9 w-full items-center justify-between rounded-md border px-3 py-2 text-sm focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none"
                >
                  <span className="flex flex-wrap gap-1 flex-1 min-w-0">
                    {selectedNamespaces.length === 0 ? (
                      <span className="text-muted-foreground">
                        {t('accessRequest.fields.namespacePlaceholder', '请选择命名空间')}
                      </span>
                    ) : (
                      selectedNamespaces.map((ns) => (
                        <Badge
                          key={ns}
                          variant="secondary"
                          className="text-xs gap-1"
                          onClick={(e) => { e.stopPropagation(); removeNamespace(ns) }}
                        >
                          {ns}
                          <IconX className="h-3 w-3 cursor-pointer" />
                        </Badge>
                      ))
                    )}
                  </span>
                  <IconChevronDown className="h-4 w-4 shrink-0 text-muted-foreground ml-2" />
                </button>
              </PopoverTrigger>
              <PopoverContent
                className="w-[var(--radix-popover-trigger-width)] p-0"
                align="start"
                onWheel={(e) => e.stopPropagation()}
              >
                <Command>
                  <CommandInput placeholder="搜索命名空间..." />
                  <CommandList className="max-h-48">
                    <CommandEmpty>暂无命名空间</CommandEmpty>
                    <CommandGroup>
                      {namespaceNames.map((ns) => (
                        <CommandItem
                          key={ns}
                          value={ns}
                          onSelect={() => toggleNamespace(ns)}
                          className="flex items-center gap-2"
                        >
                          <div
                            className={`flex h-4 w-4 items-center justify-center rounded border ${
                              selectedNamespaces.includes(ns)
                                ? 'bg-primary border-primary text-primary-foreground'
                                : 'border-input'
                            }`}
                          >
                            {selectedNamespaces.includes(ns) && (
                              <svg className="h-3 w-3" fill="currentColor" viewBox="0 0 12 12">
                                <path d="M10 3L5 8.5 2 5.5" stroke="currentColor" strokeWidth="1.5" fill="none" strokeLinecap="round" strokeLinejoin="round" />
                              </svg>
                            )}
                          </div>
                          {ns}
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  </CommandList>
                </Command>
              </PopoverContent>
            </Popover>
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

          {/* Risk Level */}
          <div className="grid gap-1.5">
            <Label htmlFor="ar-risk">
              {t('accessRequest.fields.riskLevel', '预估风险')}{' '}
              <span className="text-destructive">*</span>
            </Label>
            <Select
              value={riskLevel}
              onValueChange={setRiskLevel}
            >
              <SelectTrigger id="ar-risk">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {RISK_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
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
