import {
  IconAlertCircle,
  IconCheck,
  IconCircleFilled,
  IconBolt,
  IconCpu,
  IconServer,
} from '@tabler/icons-react'

import { GPUOverview } from '@/types/api'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Badge } from '@/components/ui/badge'

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
  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <IconCpu className="size-5" />
            GPU 资源概览
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
            GPU 资源概览
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col items-center justify-center py-8 gap-2 text-muted-foreground">
            <IconAlertCircle className="size-8" />
            <p className="text-sm">无法加载 GPU 信息</p>
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
            GPU 资源概览
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
            <IconServer className="size-8 mb-2" />
            <p className="text-sm">集群中未检测到 GPU 节点</p>
          </div>
        </CardContent>
      </Card>
    )
  }

  const { summary, fullyFreeNodes, partialFreeNodes, namespaceStats, modelStats, noModelGPUCount } = data

  return (
    <Card className="@container/gpu">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <IconCpu className="size-5" />
          GPU 资源概览
        </CardTitle>
        <CardDescription>
          集群 GPU 资源使用情况统计
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* 总体概况 */}
        <div>
          <h3 className="text-sm font-semibold mb-3">总体概况</h3>
          <div className="grid grid-cols-2 gap-4 @md/gpu:grid-cols-4">
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">GPU 节点</p>
              <p className="text-2xl font-bold tabular-nums">{summary.totalNodes}</p>
            </div>
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">总容量</p>
              <p className="text-2xl font-bold tabular-nums">{summary.totalGPUs}</p>
            </div>
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">已使用</p>
              <p className="text-2xl font-bold tabular-nums text-orange-600 dark:text-orange-400">
                {summary.usedGPUs}
              </p>
            </div>
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">空闲</p>
              <p className="text-2xl font-bold tabular-nums text-green-600 dark:text-green-400">
                {summary.freeGPUs}
              </p>
            </div>
          </div>
          <div className="mt-4 space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">使用率</span>
              <span className="font-semibold tabular-nums">{summary.usagePercent.toFixed(2)}%</span>
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

        {/* 空闲节点信息 */}
        <div className="grid grid-cols-1 gap-4 @lg/gpu:grid-cols-2">
          {/* 完全空闲的节点 */}
          <div>
            <h3 className="text-sm font-semibold mb-3 flex items-center gap-2">
              <IconCheck className="size-4 text-green-600" />
              完全空闲节点 ({fullyFreeNodes.length})
            </h3>
            <div className="h-40 rounded-md border p-3 overflow-y-auto">
              {fullyFreeNodes.length > 0 ? (
                <div className="space-y-2">
                  {fullyFreeNodes.map((node) => (
                    <div
                      key={node.nodeName}
                      className="flex items-center justify-between text-sm p-2 rounded hover:bg-muted/50"
                    >
                      <div className="flex items-center gap-2 flex-1 min-w-0">
                        <IconCircleFilled className="size-2 text-green-600 flex-shrink-0" />
                        <span className="font-mono text-xs truncate">{node.nodeName}</span>
                      </div>
                      <Badge variant="secondary" className="ml-2 flex-shrink-0">
                        {node.capacity} GPU
                      </Badge>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="flex items-center justify-center h-full text-xs text-muted-foreground">
                  无完全空闲节点
                </div>
              )}
            </div>
          </div>

          {/* 部分空闲的节点 */}
          <div>
            <h3 className="text-sm font-semibold mb-3 flex items-center gap-2">
              <IconBolt className="size-4 text-orange-600" />
              部分空闲节点 ({partialFreeNodes.length})
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
                        <span className="font-mono text-xs truncate">{node.nodeName}</span>
                      </div>
                      <Badge variant="secondary" className="ml-2 flex-shrink-0">
                        {node.free}/{node.capacity}
                      </Badge>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="flex items-center justify-center h-full text-xs text-muted-foreground">
                  无部分空闲节点
                </div>
              )}
            </div>
          </div>
        </div>

        <Separator />

        {/* 按 Namespace 统计 */}
        <div>
          <h3 className="text-sm font-semibold mb-3">按 Namespace 使用统计 (基于 LWS)</h3>
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
                      <span className="font-mono text-xs flex-1 truncate">{stat.namespace}</span>
                      <div className="flex items-center gap-2 ml-2 flex-shrink-0">
                        <span className="text-xs text-muted-foreground tabular-nums">
                          {stat.gpuCount} GPU
                        </span>
                        <span className="text-xs text-muted-foreground">
                          ({machines} 台{remaining > 0 && ` + ${remaining} GPU`})
                        </span>
                      </div>
                    </div>
                  )
                })}
              </div>
            ) : (
              <div className="flex items-center justify-center h-full text-xs text-muted-foreground">
                无 GPU 使用
              </div>
            )}
          </div>
        </div>

        <Separator />

        {/* 按模型统计 */}
        <div>
          <h3 className="text-sm font-semibold mb-3">按模型使用统计 (基于 LWS)</h3>
          <div className="h-48 rounded-md border overflow-y-auto">
            {modelStats.length > 0 || noModelGPUCount > 0 ? (
              <div className="p-3 space-y-2">
                {modelStats.map((stat) => {
                  const machines = Math.floor(stat.gpuCount / 8)
                  const remaining = stat.gpuCount % 8
                  return (
                    <div
                      key={stat.modelName}
                      className="flex items-center justify-between text-sm p-2 rounded hover:bg-muted/50"
                    >
                      <span className="font-mono text-xs flex-1 truncate">{stat.modelName}</span>
                      <div className="flex items-center gap-2 ml-2 flex-shrink-0">
                        <span className="text-xs text-muted-foreground tabular-nums">
                          {stat.gpuCount} GPU
                        </span>
                        <span className="text-xs text-muted-foreground">
                          ({machines} 台{remaining > 0 && ` + ${remaining} GPU`})
                        </span>
                      </div>
                    </div>
                  )
                })}
                {noModelGPUCount > 0 && (
                  <div className="flex items-center justify-between text-sm p-2 rounded hover:bg-muted/50">
                    <span className="font-mono text-xs flex-1 truncate text-muted-foreground">
                      &lt;未标记模型&gt;
                    </span>
                    <div className="flex items-center gap-2 ml-2 flex-shrink-0">
                      <span className="text-xs text-muted-foreground tabular-nums">
                        {noModelGPUCount} GPU
                      </span>
                      <span className="text-xs text-muted-foreground">
                        ({Math.floor(noModelGPUCount / 8)} 台
                        {noModelGPUCount % 8 > 0 && ` + ${noModelGPUCount % 8} GPU`})
                      </span>
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <div className="flex items-center justify-center h-full text-xs text-muted-foreground">
                无 GPU 使用
              </div>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
