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

function getTooltipText(cs: ContainerStatus, state: DotState): string {
  const prefix = cs.name
  const restarts = cs.restartCount ? ` (restarts: ${cs.restartCount})` : ''

  switch (state) {
    case 'ready':
      return `${prefix}: Running${restarts}`
    case 'error': {
      const reason =
        cs.state?.waiting?.reason ||
        cs.state?.terminated?.reason ||
        (cs.state?.terminated?.exitCode !== undefined
          ? `ExitCode:${cs.state.terminated.exitCode}`
          : 'Error')
      return `${prefix}: ${reason}${restarts}`
    }
    case 'waiting':
      return `${prefix}: ${cs.state?.waiting?.reason || 'Waiting'}${restarts}`
    case 'terminated':
      return `${prefix}: Completed${restarts}`
    default:
      return `${prefix}: Unknown${restarts}`
  }
}

export function ContainerStatusDots({
  containerStatuses,
  totalContainers,
}: {
  containerStatuses?: ContainerStatus[]
  totalContainers?: number
}) {
  // If no statuses yet but we know total count, render gray placeholders
  if (!containerStatuses?.length) {
    const count = totalContainers ?? 0
    return (
      <div className="flex items-center gap-0.5">
        {Array.from({ length: count }).map((_, i) => (
          <span
            key={i}
            className="inline-block w-2.5 h-2.5 rounded-sm bg-gray-300"
          />
        ))}
      </div>
    )
  }

  return (
    <div className="flex items-center gap-0.5">
      {containerStatuses.map((cs) => {
        const state = getContainerDotState(cs)
        return (
          <Tooltip key={cs.name}>
            <TooltipTrigger asChild>
              <span
                className={`inline-block w-2.5 h-2.5 rounded-sm cursor-default ${getDotClass(state)}`}
              />
            </TooltipTrigger>
            <TooltipContent side="top" className="text-xs">
              {getTooltipText(cs, state)}
            </TooltipContent>
          </Tooltip>
        )
      })}
    </div>
  )
}
