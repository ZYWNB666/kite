import { useCallback, useEffect, useRef, useState } from 'react'

import { fetchPodLogs } from '@/lib/api'
import {
  getLastPodLogTimestamp,
  getNextPodLogTailLines,
  mergePodLogSnapshot,
} from '@/lib/pod-logs'

const INITIAL_TAIL_LINES = 500
const HISTORY_BATCH_SIZE = 500
const MAX_LOG_LINES = 100_000
const POLL_INTERVAL_MS = 10_000

interface UsePodLogsOptions {
  namespace: string
  podName: string
  container?: string
  labelSelector?: string
  previous: boolean
  enabled: boolean
}

export function usePodLogs({
  namespace,
  podName,
  container,
  labelSelector,
  previous,
  enabled,
}: UsePodLogsOptions) {
  const [lines, setLines] = useState<string[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [isLoadingOlder, setIsLoadingOlder] = useState(false)
  const [isJumpingToPresent, setIsJumpingToPresent] = useState(false)
  const [isLive, setIsLive] = useState(false)
  const [hasMore, setHasMore] = useState(true)
  const [error, setError] = useState<Error | null>(null)
  const [warning, setWarning] = useState<string | null>(null)
  const [refreshVersion, setRefreshVersion] = useState(0)

  const linesRef = useRef<string[]>([])
  const cursorRef = useRef<string | undefined>(undefined)
  const clearedThroughLineRef = useRef<string | undefined>(undefined)
  const tailLinesRef = useRef(INITIAL_TAIL_LINES)
  const generationRef = useRef(0)
  const historyControllerRef = useRef<AbortController | null>(null)
  const jumpToPresentRef = useRef<(() => Promise<number>) | null>(null)
  const jumpRequestRef = useRef<Promise<number> | null>(null)

  const commitLines = useCallback((next: string[]) => {
    linesRef.current = next
    setLines(next)
  }, [])

  useEffect(() => {
    const generation = ++generationRef.current
    const initialController = new AbortController()
    let pollController: AbortController | null = null
    let pollTimer: ReturnType<typeof setTimeout> | undefined
    let active = true

    historyControllerRef.current?.abort()
    setIsLoadingOlder(false)
    setIsJumpingToPresent(false)
    jumpRequestRef.current = null
    tailLinesRef.current = INITIAL_TAIL_LINES
    cursorRef.current = undefined
    clearedThroughLineRef.current = undefined
    commitLines([])
    setError(null)
    setWarning(null)
    setHasMore(true)
    setIsLive(false)

    if (!enabled || !namespace || !podName || !container) {
      setIsLoading(false)
      return () => {
        active = false
        initialController.abort()
      }
    }

    const isCurrent = () => active && generationRef.current === generation

    const schedulePoll = (
      refreshPresent: (replaceCurrent: boolean) => Promise<unknown>
    ) => {
      if (!isCurrent() || previous) return
      if (pollTimer) clearTimeout(pollTimer)
      pollTimer = setTimeout(() => void refreshPresent(false), POLL_INTERVAL_MS)
    }

    const refreshPresent = async (replaceCurrent: boolean): Promise<number> => {
      if (!isCurrent() || (previous && !replaceCurrent)) return 0
      if (pollTimer) {
        clearTimeout(pollTimer)
        pollTimer = undefined
      }
      pollController?.abort()
      pollController = new AbortController()
      try {
        const beforeCount = linesRef.current.length
        const response = await fetchPodLogs(namespace, podName, {
          container,
          tailLines: INITIAL_TAIL_LINES,
          timestamps: true,
          previous,
          labelSelector,
          signal: pollController.signal,
        })
        if (!isCurrent()) return 0
        let incoming = response.logs
        const clearedThrough = clearedThroughLineRef.current
        if (
          !replaceCurrent &&
          linesRef.current.length === 0 &&
          clearedThrough
        ) {
          const overlapIndex = incoming.lastIndexOf(clearedThrough)
          if (overlapIndex >= 0) incoming = incoming.slice(overlapIndex + 1)
        }
        const nextLines = replaceCurrent
          ? incoming.slice(-INITIAL_TAIL_LINES)
          : mergePodLogSnapshot(incoming, linesRef.current, MAX_LOG_LINES)
        commitLines(nextLines)
        if (replaceCurrent) tailLinesRef.current = INITIAL_TAIL_LINES
        if (nextLines.length > 0 || replaceCurrent) {
          clearedThroughLineRef.current = undefined
        }
        cursorRef.current = getLastPodLogTimestamp(nextLines)
        setHasMore(response.hasMore)
        setError(null)
        setWarning(response.warnings?.join('\n') || null)
        setIsLive(!previous)
        return Math.max(0, nextLines.length - beforeCount)
      } catch (pollError) {
        if (!isCurrent() || (pollError as Error).name === 'AbortError') return 0
        setError(
          pollError instanceof Error
            ? pollError
            : new Error('Failed to load logs')
        )
        setIsLive(false)
        return 0
      } finally {
        schedulePoll(refreshPresent)
      }
    }

    const jumpToPresentRequest = () => refreshPresent(true)
    jumpToPresentRef.current = jumpToPresentRequest

    const loadInitial = async () => {
      setIsLoading(true)
      try {
        const response = await fetchPodLogs(namespace, podName, {
          container,
          tailLines: INITIAL_TAIL_LINES,
          timestamps: true,
          previous,
          labelSelector,
          signal: initialController.signal,
        })
        if (!isCurrent()) return
        commitLines(response.logs)
        cursorRef.current = getLastPodLogTimestamp(response.logs)
        setHasMore(response.hasMore)
        setError(null)
        setWarning(response.warnings?.join('\n') || null)
        setIsLive(!previous)
      } catch (loadError) {
        if (!isCurrent() || (loadError as Error).name === 'AbortError') return
        setError(
          loadError instanceof Error
            ? loadError
            : new Error('Failed to load logs')
        )
        setIsLive(false)
      } finally {
        if (isCurrent()) setIsLoading(false)
        schedulePoll(refreshPresent)
      }
    }

    void loadInitial()

    return () => {
      active = false
      initialController.abort()
      pollController?.abort()
      historyControllerRef.current?.abort()
      if (pollTimer) clearTimeout(pollTimer)
      if (jumpToPresentRef.current === jumpToPresentRequest) {
        jumpToPresentRef.current = null
      }
    }
  }, [
    commitLines,
    container,
    enabled,
    namespace,
    labelSelector,
    podName,
    previous,
    refreshVersion,
  ])

  const loadOlder = useCallback(async () => {
    if (
      !enabled ||
      !namespace ||
      !podName ||
      !container ||
      !hasMore ||
      isLoadingOlder
    ) {
      return 0
    }

    const generation = generationRef.current
    const nextTailLines = getNextPodLogTailLines(
      tailLinesRef.current,
      linesRef.current.length,
      HISTORY_BATCH_SIZE,
      MAX_LOG_LINES
    )
    historyControllerRef.current?.abort()
    const controller = new AbortController()
    historyControllerRef.current = controller
    setIsLoadingOlder(true)
    try {
      const response = await fetchPodLogs(namespace, podName, {
        container,
        tailLines: nextTailLines,
        timestamps: true,
        previous,
        labelSelector,
        signal: controller.signal,
      })
      if (generationRef.current !== generation) return 0

      const beforeCount = linesRef.current.length
      const merged = mergePodLogSnapshot(
        response.logs,
        linesRef.current,
        MAX_LOG_LINES
      )
      tailLinesRef.current = nextTailLines
      commitLines(merged)
      clearedThroughLineRef.current = undefined
      cursorRef.current = getLastPodLogTimestamp(merged) || cursorRef.current
      setHasMore(response.hasMore && nextTailLines < MAX_LOG_LINES)
      setError(null)
      setWarning(response.warnings?.join('\n') || null)
      return Math.max(0, merged.length - beforeCount)
    } catch (loadError) {
      if ((loadError as Error).name !== 'AbortError') {
        setError(
          loadError instanceof Error
            ? loadError
            : new Error('Failed to load logs')
        )
      }
      return 0
    } finally {
      if (generationRef.current === generation) setIsLoadingOlder(false)
    }
  }, [
    commitLines,
    container,
    enabled,
    hasMore,
    isLoadingOlder,
    labelSelector,
    namespace,
    podName,
    previous,
  ])

  const refresh = useCallback(() => {
    setRefreshVersion((value) => value + 1)
  }, [])

  const jumpToPresent = useCallback(async () => {
    if (jumpRequestRef.current) return jumpRequestRef.current
    const runJump = jumpToPresentRef.current
    if (!runJump) return 0

    setIsJumpingToPresent(true)
    const request = runJump()
    jumpRequestRef.current = request
    try {
      return await request
    } finally {
      if (jumpRequestRef.current === request) {
        jumpRequestRef.current = null
        setIsJumpingToPresent(false)
      }
    }
  }, [])

  const clear = useCallback(() => {
    clearedThroughLineRef.current =
      linesRef.current[linesRef.current.length - 1]
    cursorRef.current =
      getLastPodLogTimestamp(linesRef.current) || cursorRef.current
    commitLines([])
  }, [commitLines])

  return {
    lines,
    isLoading,
    isLoadingOlder,
    isJumpingToPresent,
    isLive,
    hasMore,
    error,
    warning,
    loadOlder,
    jumpToPresent,
    refresh,
    clear,
    maxLines: MAX_LOG_LINES,
  }
}
