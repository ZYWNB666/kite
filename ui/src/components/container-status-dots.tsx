import { ContainerStatus } from 'kubernetes-types/core/v1'

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

type DotState = 'ready' | 'error' | 'waiting' | 'terminated' | 'unknown'

const ERROR_REASONS = new Set([
  'CrashLoopBackOff',
  'ImagePullBackOff',
  'ErrImagePull',
  'CreateContainerError',
  'CreateContainerConfigError',
  'InvalidImageName',
  'RunContainerError',
  'PostStartHookError',
  'PreStopHookError',
])

function getContainerDotState(cs: ContainerStatus): DotState {
  if (cs.ready && cs.state?.running) return 'ready'

  const waiting = cs.state?.waiting
  if (waiting) {
    if (waiting.reason && ERROR_REASONS.has(waiting.reason)) return 'error'
    return 'waiting'
  }

  const terminated = cs.state?.terminated
  if (terminated) {
    if (terminated.exitCode === 0) return 'terminated'
    return 'error'
  }

  return 'unknown'
}

function getDotClass(state: DotState): string {
  switch (state) {
    case 'ready':
      return 'bg-green-500'
    case 'error':
      return 'bg-red-500'
    case 'waiting':
      return 'bg-yellow-400'
    case 'terminated':
      return 'bg-gray-400'
    default:
      return 'bg-gray-300'
  }
}

function getStateLabel(cs: ContainerStatus, state: DotState): string {
  switch (state) {
    case 'ready':
      return 'running, ready'
    case 'error':
      return (
        cs.state?.waiting?.reason ||
        cs.state?.terminated?.reason ||
        (cs.state?.terminated?.exitCode !== undefined
          ? `ExitCode:${cs.state.terminated.exitCode}`
          : 'error')
      )
    case 'waiting':
      return cs.state?.waiting?.reason || 'waiting'
    case 'terminated':
      return 'completed'
    default:
      return 'unknown'
  }
}

function getStartedAt(cs: ContainerStatus, state: DotState): string | null {
  if (state === 'ready' && cs.state?.running?.startedAt) {
    return cs.state.running.startedAt
  }
  if (state === 'terminated' || state === 'error') {
    return cs.state?.terminated?.finishedAt || cs.state?.terminated?.startedAt || null
  }
  return null
}

function ContainerTooltip({ cs, state }: { cs: ContainerStatus; state: DotState }) {
  const startedAt = getStartedAt(cs, state)
  const stateLabel = getStateLabel(cs, state)

  return (
    <div className="space-y-0.5">
      <div className="font-semibold">{cs.name}</div>
      <div className="text-muted-foreground">{stateLabel}</div>
      {cs.restartCount > 0 && (
        <div className="text-muted-foreground">restarts: {cs.restartCount}</div>
      )}
      {startedAt && (
        <div className="text-muted-foreground">Started At {startedAt}</div>
      )}
    </div>
  )
}

export function ContainerStatusDots({
  containerStatuses,
  totalContainers,
}: {
  containerStatuses?: ContainerStatus[]
  totalContainers?: number
}) {
  if (!containerStatuses?.length) {
    const count = totalContainers ?? 0
    return (
      <span className="inline-flex items-center gap-0.5">
        {Array.from({ length: count }).map((_, i) => (
          <span
            key={i}
            className="inline-block w-2.5 h-2.5 rounded-none bg-gray-300"
          />
        ))}
      </span>
    )
  }

  return (
    <span className="inline-flex items-center gap-0.5">
      {containerStatuses.map((cs) => {
        const state = getContainerDotState(cs)
        return (
          <Tooltip key={cs.name}>
            <TooltipTrigger asChild>
              <span
                className={`inline-block w-2.5 h-2.5 rounded-none cursor-default ${getDotClass(state)}`}
              />
            </TooltipTrigger>
            <TooltipContent side="top" className="text-xs">
              <ContainerTooltip cs={cs} state={state} />
            </TooltipContent>
          </Tooltip>
        )
      })}
    </span>
  )
}

  )
}
