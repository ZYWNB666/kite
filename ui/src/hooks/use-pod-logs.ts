import { useCallback, useEffect, useRef, useState } from 'react'

import { fetchPodLogs } from '@/lib/api'
import {
  getLastPodLogTimestamp,
  getNextPodLogTailLines,
  mergeIncrementalPodLogs,
  mergePodLogSnapshot,
} from '@/lib/pod-logs'

const INITIAL_TAIL_LINES = 500
const HISTORY_BATCH_SIZE = 500
const MAX_LOG_LINES = 100_000
const POLL_INTERVAL_MS = 5_000

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
  const lastAggregatePollAtRef = useRef(Date.now())

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
    tailLinesRef.current = INITIAL_TAIL_LINES
    cursorRef.current = undefined
    clearedThroughLineRef.current = undefined
    lastAggregatePollAtRef.current = Date.now()
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

    const schedulePoll = (poll: () => Promise<void>) => {
      if (!isCurrent() || previous) return
      pollTimer = setTimeout(() => void poll(), POLL_INTERVAL_MS)
    }

    const poll = async () => {
      if (!isCurrent() || previous) return
      pollController?.abort()
      pollController = new AbortController()
      const requestStartedAt = Date.now()
      const aggregateSinceSeconds =
        podName === '_all'
          ? Math.max(
              10,
              Math.ceil(
                (requestStartedAt - lastAggregatePollAtRef.current) / 1000
              ) + 5
            )
          : undefined
      const isInitialRecovery = podName !== '_all' && !cursorRef.current
      try {
        const response = await fetchPodLogs(namespace, podName, {
          container,
          // Do not tail an incremental request: tailing would silently discard
          // entries when more than the limit arrive between two polls.
          tailLines: isInitialRecovery ? INITIAL_TAIL_LINES : -1,
          timestamps: true,
          previous: false,
          sinceTime: podName === '_all' ? undefined : cursorRef.current,
          sinceSeconds: aggregateSinceSeconds,
          labelSelector,
          signal: pollController.signal,
        })
        if (!isCurrent()) return
        let incoming = response.logs
        const clearedThrough = clearedThroughLineRef.current
        if (linesRef.current.length === 0 && clearedThrough) {
          const overlapIndex = incoming.lastIndexOf(clearedThrough)
          if (overlapIndex >= 0) incoming = incoming.slice(overlapIndex + 1)
        }
        const merged =
          podName === '_all'
            ? mergePodLogSnapshot(incoming, linesRef.current, MAX_LOG_LINES)
            : mergeIncrementalPodLogs(linesRef.current, incoming, MAX_LOG_LINES)
        commitLines(merged)
        if (merged.length > 0) clearedThroughLineRef.current = undefined
        cursorRef.current = getLastPodLogTimestamp(merged) || cursorRef.current
        if (isInitialRecovery) setHasMore(response.hasMore)
        setError(null)
        setWarning(response.warnings?.join('\n') || null)
        setIsLive(true)
        if (podName === '_all') {
          lastAggregatePollAtRef.current = requestStartedAt
        }
      } catch (pollError) {
        if (!isCurrent() || (pollError as Error).name === 'AbortError') return
        setError(
          pollError instanceof Error
            ? pollError
            : new Error('Failed to load logs')
        )
        setIsLive(false)
      } finally {
        schedulePoll(poll)
      }
    }

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
        lastAggregatePollAtRef.current = Date.now()
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
        schedulePoll(poll)
      }
    }

    void loadInitial()

    return () => {
      active = false
      initialController.abort()
      pollController?.abort()
      historyControllerRef.current?.abort()
      if (pollTimer) clearTimeout(pollTimer)
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
    isLive,
    hasMore,
    error,
    warning,
    loadOlder,
    refresh,
    clear,
    maxLines: MAX_LOG_LINES,
  }
}
