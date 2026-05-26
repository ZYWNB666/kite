import { useEffect, useState } from 'react'
import { IconLoader2, IconPlus, IconTrash } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  FeishuApprover,
  useFeishuSetting,
  useUpdateFeishuSetting,
} from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

interface ApproverRowProps {
  approver: FeishuApprover
  onRemove: () => void
}

function ApproverRow({ approver, onRemove }: ApproverRowProps) {
  return (
    <div className="flex items-center gap-2 rounded-md border px-3 py-2">
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium truncate">{approver.name}</p>
        <p className="text-xs text-muted-foreground font-mono truncate">{approver.openId}</p>
      </div>
      <Button
        variant="ghost"
        size="icon"
        className="shrink-0 h-7 w-7 text-destructive hover:text-destructive"
        onClick={onRemove}
      >
        <IconTrash className="h-4 w-4" />
      </Button>
    </div>
  )
}

export function AccessRequestSettings() {
  const { t } = useTranslation()
  const { data: setting, isLoading } = useFeishuSetting()
  const updateMutation = useUpdateFeishuSetting()

  const [appId, setAppId] = useState('')
  const [appSecret, setAppSecret] = useState('')
  const [groupChatId, setGroupChatId] = useState('')
  const [verificationToken, setVerificationToken] = useState('')
  const [enabled, setEnabled] = useState(false)
  const [approvers, setApprovers] = useState<FeishuApprover[]>([])

  const [newApproverName, setNewApproverName] = useState('')
  const [newApproverOpenId, setNewApproverOpenId] = useState('')

  useEffect(() => {
    if (setting) {
      setAppId(setting.appId ?? '')
      setGroupChatId(setting.groupChatId ?? '')
      setEnabled(setting.enabled ?? false)
      setApprovers(setting.approvers ?? [])
    }
  }, [setting])

  const handleAddApprover = () => {
    const name = newApproverName.trim()
    const openId = newApproverOpenId.trim()
    if (!name || !openId) {
      toast.error(t('accessRequestSettings.errors.approverFieldsRequired', '请填写审批人姓名和 Open ID'))
      return
    }
    if (approvers.some((a) => a.openId === openId)) {
      toast.error(t('accessRequestSettings.errors.approverDuplicate', '该 Open ID 已存在'))
      return
    }
    setApprovers((prev) => [...prev, { name, openId }])
    setNewApproverName('')
    setNewApproverOpenId('')
  }

  const handleRemoveApprover = (openId: string) => {
    setApprovers((prev) => prev.filter((a) => a.openId !== openId))
  }

  const handleSave = async () => {
    try {
      await updateMutation.mutateAsync({
        appId,
        appSecret: appSecret || undefined,
        groupChatId,
        verificationToken: verificationToken || undefined,
        approvers,
        enabled,
      })
      setAppSecret('')
      setVerificationToken('')
      toast.success(t('accessRequestSettings.saveSuccess', '飞书通知设置已保存'))
    } catch {
      toast.error(t('accessRequestSettings.saveError', '保存失败'))
    }
  }

  if (isLoading) {
    return (
      <div className="flex justify-center py-8">
        <IconLoader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Feishu Bot Config */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>{t('accessRequestSettings.feishuBot.title', '飞书机器人配置')}</CardTitle>
              <CardDescription className="mt-1">
                {t(
                  'accessRequestSettings.feishuBot.description',
                  '配置飞书自建应用，用于发送权限申请审批消息。需要开通「发送消息」权限。'
                )}
              </CardDescription>
            </div>
            <Switch checked={enabled} onCheckedChange={setEnabled} />
          </div>
        </CardHeader>
        <CardContent className="grid gap-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div className="grid gap-1.5">
              <Label htmlFor="fs-app-id">App ID</Label>
              <Input
                id="fs-app-id"
                placeholder="cli_xxxxxxxxxxxx"
                value={appId}
                onChange={(e) => setAppId(e.target.value)}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="fs-app-secret">
                App Secret{' '}
                {setting?.appSecretSet && (
                  <span className="text-xs text-muted-foreground">
                    ({t('accessRequestSettings.alreadySet', '已设置，留空保留')})
                  </span>
                )}
              </Label>
              <Input
                id="fs-app-secret"
                type="password"
                placeholder={setting?.appSecretSet ? '••••••••' : 'App Secret'}
                value={appSecret}
                onChange={(e) => setAppSecret(e.target.value)}
              />
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="fs-group-chat">
              {t('accessRequestSettings.feishuBot.groupChatId', '群聊 Chat ID')}
            </Label>
            <Input
              id="fs-group-chat"
              placeholder="oc_xxxxxxxxxxxx"
              value={groupChatId}
              onChange={(e) => setGroupChatId(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {t(
                'accessRequestSettings.feishuBot.groupChatIdHint',
                '在机器人设置中找到群组 Chat ID（以 oc_ 开头）'
              )}
            </p>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="fs-verify-token">
              {t('accessRequestSettings.feishuBot.verificationToken', 'Verification Token')}{' '}
              {setting?.verificationTokenSet && (
                <span className="text-xs text-muted-foreground">
                  ({t('accessRequestSettings.alreadySet', '已设置，留空保留')})
                </span>
              )}
            </Label>
            <Input
              id="fs-verify-token"
              type="password"
              placeholder={
                setting?.verificationTokenSet
                  ? '••••••••'
                  : t('accessRequestSettings.feishuBot.verificationTokenPlaceholder', '用于验证卡片回调签名')
              }
              value={verificationToken}
              onChange={(e) => setVerificationToken(e.target.value)}
            />
          </div>
        </CardContent>
      </Card>

      {/* Approver Management */}
      <Card>
        <CardHeader>
          <CardTitle>{t('accessRequestSettings.approvers.title', '审批人列表')}</CardTitle>
          <CardDescription>
            {t(
              'accessRequestSettings.approvers.description',
              '配置可接收审批请求的人员。需要填写飞书 Open ID（ou_ 开头）。'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            {approvers.length === 0 ? (
              <p className="text-sm text-muted-foreground py-2">
                {t('accessRequestSettings.approvers.empty', '暂无审批人，请添加')}
              </p>
            ) : (
              approvers.map((a) => (
                <ApproverRow
                  key={a.openId}
                  approver={a}
                  onRemove={() => handleRemoveApprover(a.openId)}
                />
              ))
            )}
          </div>

          {/* Add approver */}
          <div className="rounded-md border border-dashed p-3 space-y-3">
            <p className="text-sm font-medium">
              {t('accessRequestSettings.approvers.addTitle', '添加审批人')}
            </p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
              <Input
                placeholder={t('accessRequestSettings.approvers.namePlaceholder', '姓名')}
                value={newApproverName}
                onChange={(e) => setNewApproverName(e.target.value)}
              />
              <Input
                placeholder={t(
                  'accessRequestSettings.approvers.openIdPlaceholder',
                  'Open ID (ou_xxx)'
                )}
                value={newApproverOpenId}
                onChange={(e) => setNewApproverOpenId(e.target.value)}
              />
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={handleAddApprover}
              className="gap-1"
            >
              <IconPlus className="h-4 w-4" />
              {t('accessRequestSettings.approvers.add', '添加')}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Save */}
      <div className="flex justify-end">
        <Button onClick={handleSave} disabled={updateMutation.isPending}>
          {updateMutation.isPending && (
            <IconLoader2 className="mr-2 h-4 w-4 animate-spin" />
          )}
          {t('common.save', '保存')}
        </Button>
      </div>
    </div>
  )
}
