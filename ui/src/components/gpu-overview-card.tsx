import {
  IconAlertCircle,
  IconBolt,
  IconCheck,
  IconCircleFilled,
  IconCpu,
  IconServer,
} from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { GPUOverview } from '@/types/api'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'

interface GPUOverviewCardProps {
  data?: GPUOverview
  isLoading?: boolean
  error?: Error | null
}

export function GPUOverviewCard({
  data,
  isLoading,
  error,
}: GPUOverviewCardProps) {
  const { t } = useTranslation()

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <IconCpu className="size-5" />
            {t('gpu.title')}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-4 animate-pulse">
            <div className="h-24 bg-muted rounded"></div>
            <div className="h-32 bg-muted rounded"></div>
          </div>
        </CardContent>
      </Card>
    )
  }

  if (error) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <IconCpu className="size-5" />
            {t('gpu.title')}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col items-center justify-center py-8 gap-2 text-muted-foreground">
            <IconAlertCircle className="size-8" />
            <p className="text-sm">{t('gpu.loadFailed')}</p>
          </div>
        </CardContent>
      </Card>
    )
  }

  if (!data || data.summary.totalNodes === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <IconCpu className="size-5" />
            {t('gpu.title')}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
            <IconServer className="size-8 mb-2" />
            <p className="text-sm">{t('gpu.noGpuNodes')}</p>
          </div>
        </CardContent>
      </Card>
    )
  }

  const {
    summary,
    fullyFreeNodes,
    untaintedFreeNodes,
    taintedFreeNodes,
    partialFreeNodes,
    namespaceStats,
    modelStats,
    noModelGPUCount,
    modelRoleStats,
  } = data

  return (
    <Card className="@container/gpu">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <IconCpu className="size-5" />
          {t('gpu.title')}
        </CardTitle>
        <CardDescription>{t('gpu.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Overall summary */}
        <div>
          <h3 className="text-sm font-semibold mb-3">{t('gpu.overview')}</h3>
          <div className="grid grid-cols-2 gap-4 @md/gpu:grid-cols-4">
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">{t('gpu.nodes')}</p>
              <p className="text-2xl font-bold tabular-nums">
                {summary.totalNodes}
              </p>
            </div>
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">
                {t('gpu.totalCapacity')}
              </p>
              <p className="text-2xl font-bold tabular-nums">
                {summary.totalGPUs}
              </p>
            </div>
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">{t('gpu.used')}</p>
              <p className="text-2xl font-bold tabular-nums text-orange-600 dark:text-orange-400">
                {summary.usedGPUs}
              </p>
            </div>
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">{t('gpu.free')}</p>
              <p className="text-2xl font-bold tabular-nums text-green-600 dark:text-green-400">
                {summary.freeGPUs}
              </p>
            </div>
          </div>
          <div className="mt-4 space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">{t('gpu.usage')}</span>
              <span className="font-semibold tabular-nums">
                {summary.usagePercent.toFixed(2)}%
              </span>
            </div>
            <div className="h-2 w-full bg-secondary rounded-full overflow-hidden">
              <div
                className="h-full bg-primary transition-all duration-300"
                style={{ width: `${Math.min(summary.usagePercent, 100)}%` }}
              />
            </div>
          </div>
        </div>

        <Separator />

        {/* Free node information */}
        <div className="grid grid-cols-1 gap-4 @lg/gpu:grid-cols-2">
          {/* Fully free nodes */}
          <div>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold flex items-center gap-2">
                <IconCheck className="size-4 text-green-600" />
                {t('gpu.fullyFreeNodes', { count: fullyFreeNodes.length })}
              </h3>
              {fullyFreeNodes.length > 0 && (
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <span className="flex items-center gap-1">
                    <IconCircleFilled className="size-2 text-green-600" />
                    {t('gpu.noTaints', { count: untaintedFreeNodes.length })}
                  </span>
                  <span className="flex items-center gap-1">
                    <IconCircleFilled className="size-2 text-yellow-500" />
                    {t('gpu.hasTaints', { count: taintedFreeNodes.length })}
                  </span>
                </div>
              )}
            </div>
            <div className="h-40 rounded-md border p-3 overflow-y-auto">
              {fullyFreeNodes.length > 0 ? (
                <div className="space-y-2">
                  {fullyFreeNodes.map((node) => (
                    <div
                      key={node.nodeName}
                      className="flex flex-col gap-1 p-2 rounded border border-transparent hover:border-border hover:bg-muted/50 transition-colors"
                    >
                      <div className="flex items-center justify-between text-sm">
                        <div className="flex items-center gap-2 flex-1 min-w-0">
                          <IconCircleFilled
                            className={`size-2 flex-shrink-0 ${node.taints && node.taints.length > 0 ? 'text-yellow-500' : 'text-green-600'}`}
                          />
                          <span className="font-mono text-xs truncate">
                            {node.nodeName}
                          </span>
                        </div>
                        <Badge
                          variant="secondary"
                          className="ml-2 flex-shrink-0"
                        >
                          {node.capacity} GPU
                        </Badge>
                      </div>
                      {node.taints && node.taints.length > 0 && (
                        <div className="flex flex-wrap gap-1 pl-6">
                          {node.taints.map((taint, i) => (
                            <Badge
                              key={i}
                              variant="outline"
                              className="text-[10px] h-4 px-1 text-muted-foreground border-yellow-500/30 bg-yellow-500/10"
                            >
                              {taint}
                            </Badge>
                          ))}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              ) : (
                <div className="flex items-center justify-center h-full text-xs text-muted-foreground">
                  {t('gpu.noFullyFreeNodes')}
                </div>
              )}
            </div>
          </div>

          {/* Partially free nodes */}
          <div>
            <h3 className="text-sm font-semibold mb-3 flex items-center gap-2">
              <IconBolt className="size-4 text-orange-600" />
              {t('gpu.partiallyFreeNodes', { count: partialFreeNodes.length })}
            </h3>
            <div className="h-40 rounded-md border p-3 overflow-y-auto">
              {partialFreeNodes.length > 0 ? (
                <div className="space-y-2">
                  {partialFreeNodes.map((node) => (
                    <div
                      key={node.nodeName}
                      className="flex items-center justify-between text-sm p-2 rounded hover:bg-muted/50"
                    >
                      <div className="flex items-center gap-2 flex-1 min-w-0">
                        <IconCircleFilled className="size-2 text-orange-600 flex-shrink-0" />
                        <span className="font-mono text-xs truncate">
                          {node.nodeName}
                        </span>
                      </div>
                      <Badge variant="secondary" className="ml-2 flex-shrink-0">
                        {node.free}/{node.capacity}
                      </Badge>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="flex items-center justify-center h-full text-xs text-muted-foreground">
                  {t('gpu.noPartiallyFreeNodes')}
                </div>
              )}
            </div>
          </div>
        </div>

        <Separator />

        {/* Usage by Namespace */}
        <div>
          <h3 className="text-sm font-semibold mb-3">{t('gpu.byNamespace')}</h3>
          <div className="h-48 rounded-md border overflow-y-auto">
            {namespaceStats.length > 0 ? (
              <div className="p-3 space-y-2">
                {namespaceStats.map((stat) => {
                  const machines = Math.floor(stat.gpuCount / 8)
                  const remaining = stat.gpuCount % 8
                  return (
                    <div
                      key={stat.namespace}
                      className="flex items-center justify-between text-sm p-2 rounded hover:bg-muted/50"
                    >
                      <span className="font-mono text-xs flex-1 truncate">
                        {stat.namespace}
                      </span>
                      <div className="flex items-center gap-2 ml-2 flex-shrink-0">
                        <span className="text-xs text-muted-foreground tabular-nums">
                          {stat.gpuCount} GPU
                        </span>
                        <span className="text-xs text-muted-foreground">
                          ({t('gpu.machines', { count: machines })}
                          {remaining > 0 && ` + ${remaining} GPU`})
                        </span>
                      </div>
                    </div>
                  )
                })}
              </div>
            ) : (
              <div className="flex items-center justify-center h-full text-xs text-muted-foreground">
                {t('gpu.noGpuUsage')}
              </div>
            )}
          </div>
        </div>

        <Separator />

        {/* Usage by Model */}
        <div>
          <h3 className="text-sm font-semibold mb-3">{t('gpu.byModel')}</h3>
          <div className="h-48 rounded-md border overflow-y-auto">
            {modelStats.length > 0 || noModelGPUCount > 0 ? (
              <div className="p-3 space-y-2">
                {modelStats.map((stat) => {
                  const machines = Math.floor(stat.gpuCount / 8)
                  const remaining = stat.gpuCount % 8
                  const roleStat = modelRoleStats?.find(
                    (r) => r.modelName === stat.modelName
                  )
                  return (
                    <div
                      key={stat.modelName}
                      className="flex items-center justify-between text-sm p-2 rounded hover:bg-muted/50"
                    >
                      <span className="font-mono text-xs flex-1 truncate">
                        {stat.modelName}
                      </span>
                      <div className="flex items-center gap-2 ml-2 flex-shrink-0">
                        {roleStat && (
                          <span className="text-xs text-muted-foreground tabular-nums">
                            <span className="text-blue-500">
                              {t('gpu.prefillMachines', {
                                count: roleStat.prefillNodes,
                              })}
                            </span>
                            {' / '}
                            <span className="text-orange-500">
                              {t('gpu.decodeMachines', {
                                count: roleStat.decodeNodes,
                              })}
                            </span>
                          </span>
                        )}
                        <span className="text-xs text-muted-foreground tabular-nums">
                          {stat.gpuCount} GPU
                        </span>
                        <span className="text-xs text-muted-foreground">
                          ({t('gpu.machines', { count: machines })}
                          {remaining > 0 && ` + ${remaining} GPU`})
                        </span>
                      </div>
                    </div>
                  )
                })}
                {noModelGPUCount > 0 && (
                  <div className="flex items-center justify-between text-sm p-2 rounded hover:bg-muted/50">
                    <span className="font-mono text-xs flex-1 truncate text-muted-foreground">
                      {t('gpu.unlabeledModel')}
                    </span>
                    <div className="flex items-center gap-2 ml-2 flex-shrink-0">
                      <span className="text-xs text-muted-foreground tabular-nums">
                        {noModelGPUCount} GPU
                      </span>
                      <span className="text-xs text-muted-foreground">
                        (
                        {t('gpu.machines', {
                          count: Math.floor(noModelGPUCount / 8),
                        })}
                        {noModelGPUCount % 8 > 0 &&
                          ` + ${noModelGPUCount % 8} GPU`}
                        )
                      </span>
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <div className="flex items-center justify-center h-full text-xs text-muted-foreground">
                {t('gpu.noGpuUsage')}
              </div>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
