import { useMemo, useState } from 'react'
import {
  IconChevronDown,
  IconLoader2,
  IconShieldPlus,
  IconX,
} from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  REQUEST_TYPE_OPTIONS,
  useCreateAccessRequest,
  useFeishuApprovers,
  useResources,
  type RequestType,
} from '@/lib/api'
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
import { Input } from '@/components/ui/input'
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

  const [requestType, setRequestType] = useState<RequestType>('canary_update')
  const [selectedCluster, setSelectedCluster] = useState('')
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([])
  const [nsPopoverOpen, setNsPopoverOpen] = useState(false)
  const [reportLink, setReportLink] = useState('')
  const [selectedCMs, setSelectedCMs] = useState<string[]>([])
  const [cmPopoverOpen, setCmPopoverOpen] = useState(false)
  const [durationHours, setDurationHours] = useState<number>(4)
  const [riskLevel, setRiskLevel] = useState<string>('low')
  const [reason, setReason] = useState('')
  const [approverUid, setApproverUid] = useState('')

  const isRouteAdjust = requestType === 'route_adjust'
  const needsReportLink = requestType !== 'route_adjust'

  // Namespace list (skip when route_adjust since namespace is fixed)
  const { data: nsItems = [] } = useResources('namespaces', undefined, {
    cluster: selectedCluster,
    disable: !open || !selectedCluster || isRouteAdjust,
  })
  const namespaceNames = useMemo(
    () =>
      nsItems
        .map((ns) => ns.metadata?.name)
        .filter((n): n is string => !!n)
        .sort(),
    [nsItems]
  )

  // Configmaps for route_adjust (from envoy-gateway-system)
  const { data: cmItems = [] } = useResources(
    'configmaps',
    'envoy-gateway-system',
    {
      cluster: selectedCluster,
      disable: !open || !isRouteAdjust || !selectedCluster,
    }
  )
  const cmNames = useMemo(
    () =>
      cmItems
        .map((cm) => cm.metadata?.name)
        .filter((n): n is string => !!n)
        .sort(),
    [cmItems]
  )

  const resetForm = () => {
    setRequestType('canary_update')
    setSelectedCluster('')
    setSelectedNamespaces([])
    setReportLink('')
    setSelectedCMs([])
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

  const toggleCM = (cm: string) => {
    setSelectedCMs((prev) =>
      prev.includes(cm) ? prev.filter((n) => n !== cm) : [...prev, cm]
    )
  }

  const removeCM = (cm: string) => {
    setSelectedCMs((prev) => prev.filter((n) => n !== cm))
  }

  const handleSubmit = async () => {
    if (!selectedCluster) {
      toast.error(t('accessRequest.errors.clusterRequired', '请选择集群'))
      return
    }
    if (!isRouteAdjust && selectedNamespaces.length === 0) {
      toast.error(t('accessRequest.errors.namespaceRequired', '请选择命名空间'))
      return
    }
    if (needsReportLink && !reportLink.trim()) {
      toast.error(
        t('accessRequest.errors.reportLinkRequired', '请填写测试报告链接')
      )
      return
    }
    if (isRouteAdjust && selectedCMs.length === 0) {
      toast.error(
        t('accessRequest.errors.cmRequired', '请选择网关配置 (configmap)')
      )
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
        namespaces: isRouteAdjust
          ? ['envoy-gateway-system']
          : selectedNamespaces,
        requestType,
        reportLink: reportLink.trim() || undefined,
        targetResources: isRouteAdjust ? selectedCMs : undefined,
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
          {/* Request Type */}
          <div className="grid gap-1.5">
            <Label>
              {t('accessRequest.fields.requestType', '申请类型')}{' '}
              <span className="text-destructive">*</span>
            </Label>
            <Select
              value={requestType}
              onValueChange={(v) => setRequestType(v as RequestType)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {REQUEST_TYPE_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Cluster selector */}
          <div className="grid gap-1.5">
            <Label htmlFor="ar-cluster">
              {t('accessRequest.fields.cluster', '集群')}{' '}
              <span className="text-destructive">*</span>
            </Label>
            <Select
              value={selectedCluster}
              onValueChange={(cluster) => {
                setSelectedCluster(cluster)
                setSelectedNamespaces([])
                setSelectedCMs([])
              }}
            >
              <SelectTrigger id="ar-cluster">
                <SelectValue
                  placeholder={t(
                    'accessRequest.fields.clusterPlaceholder',
                    '请选择集群'
                  )}
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

          {/* Conditionally: report link for full/canary; CM multi-select for route_adjust */}
          {needsReportLink && (
            <div className="grid gap-1.5">
              <Label htmlFor="ar-report-link">
                {requestType === 'full_update'
                  ? t(
                      'accessRequest.fields.canaryReportLink',
                      '灰度测试报告链接'
                    )
                  : t('accessRequest.fields.reportLink', '测试报告链接')}{' '}
                <span className="text-destructive">*</span>
              </Label>
              <Input
                id="ar-report-link"
                type="url"
                placeholder={t(
                  'accessRequest.fields.reportLinkPlaceholder',
                  '例如 https://docs.example.com/test-report'
                )}
                value={reportLink}
                onChange={(e) => setReportLink(e.target.value)}
              />
            </div>
          )}

          {isRouteAdjust ? (
            <div className="grid gap-1.5">
              <Label>
                {t('accessRequest.fields.targetCM', '目标配置 (ConfigMap)')}{' '}
                <span className="text-destructive">*</span>
              </Label>
              <Popover open={cmPopoverOpen} onOpenChange={setCmPopoverOpen}>
                <PopoverTrigger asChild>
                  <button
                    type="button"
                    className="border-input bg-background ring-offset-background focus-visible:ring-ring flex min-h-9 w-full items-center justify-between rounded-md border px-3 py-2 text-sm focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none"
                  >
                    <span className="flex flex-wrap gap-1 flex-1 min-w-0">
                      {selectedCMs.length === 0 ? (
                        <span className="text-muted-foreground">
                          {t(
                            'accessRequest.fields.targetCMPlaceholder',
                            '选择 envoy-gateway-system 下的配置'
                          )}
                        </span>
                      ) : (
                        selectedCMs.map((cm) => (
                          <Badge
                            key={cm}
                            variant="secondary"
                            className="text-xs gap-1"
                            onClick={(e) => {
                              e.stopPropagation()
                              removeCM(cm)
                            }}
                          >
                            {cm}
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
                    <CommandInput placeholder="搜索配置..." />
                    <CommandList className="max-h-48">
                      <CommandEmpty>暂无配置</CommandEmpty>
                      <CommandGroup>
                        {cmNames.map((cm) => (
                          <CommandItem
                            key={cm}
                            value={cm}
                            onSelect={() => toggleCM(cm)}
                            className="flex items-center gap-2"
                          >
                            <div
                              className={`flex h-4 w-4 items-center justify-center rounded border ${
                                selectedCMs.includes(cm)
                                  ? 'bg-primary border-primary text-primary-foreground'
                                  : 'border-input'
                              }`}
                            >
                              {selectedCMs.includes(cm) && (
                                <svg
                                  className="h-3 w-3"
                                  fill="currentColor"
                                  viewBox="0 0 12 12"
                                >
                                  <path
                                    d="M10 3L5 8.5 2 5.5"
                                    stroke="currentColor"
                                    strokeWidth="1.5"
                                    fill="none"
                                    strokeLinecap="round"
                                    strokeLinejoin="round"
                                  />
                                </svg>
                              )}
                            </div>
                            {cm}
                          </CommandItem>
                        ))}
                      </CommandGroup>
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
              <p className="text-xs text-muted-foreground">
                {t(
                  'accessRequest.fields.targetCMHint',
                  '审批通过后仅授予选中配置的增改查权限（禁删除）'
                )}
              </p>
            </div>
          ) : (
            /* Namespace multi-select (hidden for route_adjust) */
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
                          {t(
                            'accessRequest.fields.namespacePlaceholder',
                            '请选择命名空间'
                          )}
                        </span>
                      ) : (
                        selectedNamespaces.map((ns) => (
                          <Badge
                            key={ns}
                            variant="secondary"
                            className="text-xs gap-1"
                            onClick={(e) => {
                              e.stopPropagation()
                              removeNamespace(ns)
                            }}
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
                                <svg
                                  className="h-3 w-3"
                                  fill="currentColor"
                                  viewBox="0 0 12 12"
                                >
                                  <path
                                    d="M10 3L5 8.5 2 5.5"
                                    stroke="currentColor"
                                    strokeWidth="1.5"
                                    fill="none"
                                    strokeLinecap="round"
                                    strokeLinejoin="round"
                                  />
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
          )}

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
            <Select value={riskLevel} onValueChange={setRiskLevel}>
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
