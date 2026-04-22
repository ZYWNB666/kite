import { useState } from 'react'
import { IconLoader2, IconMaximize, IconMinimize } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { applyResource, useTemplates } from '@/lib/api'
import { cn, translateError } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SimpleYamlEditor } from '@/components/simple-yaml-editor'

interface CreateResourceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CreateResourceDialog({
  open,
  onOpenChange,
}: CreateResourceDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <CreateResourceDialogContent onOpenChange={onOpenChange} />
      ) : null}
    </Dialog>
  )
}

function CreateResourceDialogContent({
  onOpenChange,
}: Omit<CreateResourceDialogProps, 'open'>) {
  const { t } = useTranslation()
  const { data: templates = [] } = useTemplates()
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>('')
  const [yamlContent, setYamlContent] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [isMaximized, setIsMaximized] = useState(false)

  const handleTemplateChange = (templateName: string) => {
    if (templateName === 'empty') {
      setYamlContent('')
      setSelectedTemplateId('')
      return
    }

    const template = templates.find((t) => t.name === templateName)
    if (template) {
      setYamlContent(template.yaml)
      setSelectedTemplateId(template.name)
    }
  }

  const handleApply = async () => {
    if (!yamlContent) return

    setIsLoading(true)
    try {
      await applyResource(yamlContent)
      toast.success(
        t('createResource.success', 'Resource created successfully')
      )
    } catch (err) {
      console.error('Failed to apply resource', err)
      toast.error(translateError(err, t))
    } finally {
      setIsLoading(false)
    }
  }

  const handleCancel = () => {
    setYamlContent('')
    setSelectedTemplateId('')
    onOpenChange(false)
  }

  return (
    <DialogContent
      className={cn(
        'flex flex-col transition-all duration-200',
        isMaximized
          ? '!max-w-[100vw] !w-[100vw] !h-[100vh] !max-h-[100vh] m-0 !rounded-none border-0'
          : '!max-w-4xl sm:!max-w-4xl !h-[80vh]'
      )}
    >
      <DialogHeader className="shrink-0">
        <div className="flex items-center justify-between pr-8">
          <div className="text-left">
            <DialogTitle>Create Resource</DialogTitle>
            <DialogDescription>
              Paste any Kubernetes resource YAML configuration and apply it to the
              cluster
            </DialogDescription>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setIsMaximized(!isMaximized)}
            className="h-8 w-8 text-muted-foreground hover:text-foreground shrink-0"
          >
            {isMaximized ? (
              <IconMinimize className="h-4 w-4" />
            ) : (
              <IconMaximize className="h-4 w-4" />
            )}
          </Button>
        </div>
      </DialogHeader>

      <div className="flex-1 flex flex-col space-y-4 min-h-0 overflow-hidden">
        <div className="space-y-2 shrink-0">
          <Label htmlFor="template">Template</Label>
          <Select
            value={selectedTemplateId || 'empty'}
            onValueChange={handleTemplateChange}
          >
            <SelectTrigger>
              <SelectValue
                placeholder={t(
                  'createResource.selectTemplate',
                  'Select a template'
                )}
              />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="empty">
                {t('createResource.emptyTemplate', 'Empty Template')}
              </SelectItem>
              {templates.map((template) => (
                <SelectItem key={template.name} value={template.name}>
                  {template.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex-1 flex flex-col min-h-0">
          <Label htmlFor="yaml" className="shrink-0 mb-2">YAML Configuration</Label>
          <div className="flex-1 min-h-0">
            <SimpleYamlEditor
              value={yamlContent}
              onChange={(value) => setYamlContent(value || '')}
              height="100%"
            />
          </div>
        </div>
      </div>

      <DialogFooter className="shrink-0">
        <Button variant="outline" onClick={handleCancel} disabled={isLoading}>
          Cancel
        </Button>
        <Button onClick={handleApply} disabled={isLoading || !yamlContent}>
          {isLoading ? (
            <>
              <IconLoader2 className="mr-2 h-4 w-4 animate-spin" />
              {t('common.applying', 'Applying...')}
            </>
          ) : (
            t('common.apply', 'Apply')
          )}
        </Button>
      </DialogFooter>
    </DialogContent>
  )
}
