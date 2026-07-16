import {
  ReactNode,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import {
  IconArrowDown,
  IconArrowUp,
  IconClearAll,
  IconDownload,
  IconFilter,
  IconHistory,
  IconMaximize,
  IconMinimize,
  IconSearch,
  IconSettings,
  IconX,
} from '@tabler/icons-react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Container, Pod } from 'kubernetes-types/core/v1'

import { TERMINAL_THEMES, TerminalTheme } from '@/types/themes'
import { stripAnsi } from '@/lib/ansi-parser'
import { toSimpleContainer } from '@/lib/k8s'
import { buildPodLogMatcher, parsePodLogLine } from '@/lib/pod-logs'
import { usePodLogs } from '@/hooks/use-pod-logs'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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
import { Switch } from '@/components/ui/switch'

import { ConnectionIndicator } from './connection-indicator'
import { ContainerSelector } from './selector/container-selector'
import { PodSelector } from './selector/pod-selector'

export interface LogViewerProps {
  namespace: string
  podName?: string
  pods?: Pod[]
  labelSelector?: string
  containers?: Container[]
  initContainers?: Container[]
  onClose?: () => void
}

interface DisplayLogRow {
  line: string
  originalIndex: number
}

function matcherTest(matcher: RegExp, value: string) {
  matcher.lastIndex = 0
  return matcher.test(value)
}

function highlightMatches(text: string, matcher?: RegExp): ReactNode {
  if (!matcher) return text

  const regex = new RegExp(matcher.source, matcher.flags)
  const parts: ReactNode[] = []
  let cursor = 0
  let match: RegExpExecArray | null

  while ((match = regex.exec(text)) !== null) {
    if (match.index > cursor) parts.push(text.slice(cursor, match.index))
    parts.push(
      <mark
        key={`${match.index}-${match[0]}`}
        className="rounded-sm bg-yellow-400/60 px-0 text-inherit"
      >
        {match[0]}
      </mark>
    )
    cursor = match.index + match[0].length
    if (match[0].length === 0) regex.lastIndex += 1
  }

  if (cursor < text.length) parts.push(text.slice(cursor))
  return parts.length > 0 ? parts : text
}

function formatLogTimestamp(timestamp: string) {
  const date = new Date(timestamp)
  return Number.isNaN(date.getTime()) ? timestamp : date.toLocaleString()
}

export function LogViewer({
  namespace,
  podName,
  pods,
  containers: fallbackContainers,
  initContainers: fallbackInitContainers,
  onClose,
  labelSelector,
}: LogViewerProps) {
  const [logTheme, setLogTheme] = useState<TerminalTheme>(() => {
    const saved = localStorage.getItem('log-viewer-theme')
    return (saved as TerminalTheme) || 'classic'
  })
  const [fontSize, setFontSize] = useState(() => {
    const saved = localStorage.getItem('log-viewer-font-size')
    return saved ? Number.parseInt(saved, 10) : 14
  })
  const [wordWrap, setWordWrap] = useState(() => {
    const saved = localStorage.getItem('log-viewer-word-wrap')
    return saved === null ? true : saved === 'true'
  })
  const [timestamps, setTimestamps] = useState(false)
  const [previous, setPrevious] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [useRegex, setUseRegex] = useState(false)
  const [caseSensitive, setCaseSensitive] = useState(false)
  const [filterMatches, setFilterMatches] = useState(false)
  const [activeMatch, setActiveMatch] = useState(-1)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [followLatest, setFollowLatest] = useState(true)

  const sortedPods = useMemo(() => {
    if (!pods) return undefined
    return [...pods].sort((left, right) =>
      (left.metadata?.creationTimestamp || '') >
      (right.metadata?.creationTimestamp || '')
        ? -1
        : 1
    )
  }, [pods])

  const [selectedPodName, setSelectedPodName] = useState<string | undefined>(
    podName || sortedPods?.[0]?.metadata?.name
  )
  const selectedPod =
    sortedPods?.find((pod) => pod.metadata?.name === selectedPodName) ||
    (selectedPodName === '_all' ? sortedPods?.[0] : undefined)
  const containers = useMemo(
    () =>
      toSimpleContainer(
        selectedPod?.spec?.initContainers || fallbackInitContainers,
        selectedPod?.spec?.containers || fallbackContainers
      ),
    [
      fallbackContainers,
      fallbackInitContainers,
      selectedPod?.spec?.containers,
      selectedPod?.spec?.initContainers,
    ]
  )
  const [selectedContainer, setSelectedContainer] = useState<
    string | undefined
  >(containers[0]?.name)

  useEffect(() => {
    if (podName) {
      setSelectedPodName(podName)
      return
    }
    setSelectedPodName((current) => {
      if (current === '_all') return current
      if (
        current &&
        sortedPods?.some((pod) => pod.metadata?.name === current)
      ) {
        return current
      }
      return sortedPods?.[0]?.metadata?.name
    })
  }, [podName, sortedPods])

  useEffect(() => {
    setSelectedContainer((current) => {
      if (
        current &&
        containers.some((container) => container.name === current)
      ) {
        return current
      }
      return containers[0]?.name
    })
  }, [containers])

  const {
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
    maxLines,
  } = usePodLogs({
    namespace,
    podName: selectedPodName || '',
    container: selectedContainer,
    labelSelector,
    previous,
    enabled: Boolean(selectedPodName && selectedContainer),
  })

  const matcherResult = useMemo(() => {
    try {
      return {
        matcher: buildPodLogMatcher(searchTerm, useRegex, caseSensitive),
        invalid: false,
      }
    } catch {
      return { matcher: undefined, invalid: true }
    }
  }, [caseSensitive, searchTerm, useRegex])

  const rows = useMemo<DisplayLogRow[]>(() => {
    const allRows = lines.map((line, originalIndex) => ({
      line,
      originalIndex,
    }))
    if (!filterMatches || !matcherResult.matcher) return allRows
    return allRows.filter(({ line }) =>
      matcherTest(
        matcherResult.matcher!,
        stripAnsi(parsePodLogLine(line).message)
      )
    )
  }, [filterMatches, lines, matcherResult.matcher])

  const matchingRowIndices = useMemo(() => {
    if (!matcherResult.matcher) return []
    const result: number[] = []
    rows.forEach(({ line }, index) => {
      if (
        matcherTest(
          matcherResult.matcher!,
          stripAnsi(parsePodLogLine(line).message)
        )
      ) {
        result.push(index)
      }
    })
    return result
  }, [matcherResult.matcher, rows])

  const scrollRef = useRef<HTMLDivElement | null>(null)
  const searchInputRef = useRef<HTMLInputElement | null>(null)
  const loadingHistoryRef = useRef(false)
  const previousLineCountRef = useRef(0)

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => Math.ceil(fontSize * 1.5),
    overscan: 35,
    measureElement: (element) => element.getBoundingClientRect().height,
  })

  useLayoutEffect(() => {
    virtualizer.measure()
  }, [fontSize, virtualizer, wordWrap])

  const scrollToLatest = useCallback(() => {
    if (rows.length === 0) return
    virtualizer.scrollToIndex(rows.length - 1, { align: 'end' })
    setFollowLatest(true)
  }, [rows.length, virtualizer])

  useLayoutEffect(() => {
    const previousCount = previousLineCountRef.current
    previousLineCountRef.current = lines.length
    if (lines.length > previousCount && followLatest && !isLoadingOlder) {
      requestAnimationFrame(scrollToLatest)
    }
  }, [followLatest, isLoadingOlder, lines.length, scrollToLatest])

  useEffect(() => {
    setFollowLatest(true)
    setActiveMatch(-1)
  }, [previous, selectedContainer, selectedPodName])

  const handleLoadOlder = useCallback(async () => {
    const element = scrollRef.current
    if (!element || loadingHistoryRef.current || !hasMore) return
    loadingHistoryRef.current = true
    const oldHeight = element.scrollHeight
    const oldTop = element.scrollTop
    try {
      const added = await loadOlder()
      if (added > 0) {
        requestAnimationFrame(() => {
          requestAnimationFrame(() => {
            const current = scrollRef.current
            if (current)
              current.scrollTop = oldTop + current.scrollHeight - oldHeight
          })
        })
      }
    } finally {
      loadingHistoryRef.current = false
    }
  }, [hasMore, loadOlder])

  const handleScroll = useCallback(() => {
    const element = scrollRef.current
    if (!element) return
    const distanceFromBottom =
      element.scrollHeight - element.scrollTop - element.clientHeight
    setFollowLatest(distanceFromBottom < 80)
    if (element.scrollTop < 48 && hasMore && !isLoadingOlder) {
      void handleLoadOlder()
    }
  }, [handleLoadOlder, hasMore, isLoadingOlder])

  const moveSearch = useCallback(
    (forward: boolean) => {
      if (matchingRowIndices.length === 0) return
      const next = forward
        ? (activeMatch + 1) % matchingRowIndices.length
        : (activeMatch - 1 + matchingRowIndices.length) %
          matchingRowIndices.length
      setActiveMatch(next)
      virtualizer.scrollToIndex(matchingRowIndices[next], { align: 'center' })
      setFollowLatest(false)
    },
    [activeMatch, matchingRowIndices, virtualizer]
  )

  useEffect(() => setActiveMatch(-1), [searchTerm, useRegex, caseSensitive])

  const downloadLogs = useCallback(() => {
    if (lines.length === 0) return
    const content = lines
      .map((line) => {
        const parsed = parsePodLogLine(line)
        const message = stripAnsi(parsed.message)
        return timestamps && parsed.timestamp
          ? `${formatLogTimestamp(parsed.timestamp)} ${message}`
          : message
      })
      .join('\n')
    const blob = new Blob([content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `${selectedPodName || 'pod'}-${selectedContainer || 'container'}-logs.txt`
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
    URL.revokeObjectURL(url)
  }, [lines, selectedContainer, selectedPodName, timestamps])

  const handleThemeChange = useCallback((theme: TerminalTheme) => {
    setLogTheme(theme)
    localStorage.setItem('log-viewer-theme', theme)
  }, [])

  const handleFontSizeChange = useCallback((size: number) => {
    setFontSize(size)
    localStorage.setItem('log-viewer-font-size', size.toString())
  }, [])

  const toggleWordWrap = useCallback(() => {
    setWordWrap((current) => {
      localStorage.setItem('log-viewer-word-wrap', String(!current))
      return !current
    })
  }, [])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key === 'f') {
        event.preventDefault()
        searchInputRef.current?.focus()
        searchInputRef.current?.select()
      }
      if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
        setIsFullscreen((current) => !current)
      }
      if (event.altKey && event.key.toLowerCase() === 'z') {
        event.preventDefault()
        toggleWordWrap()
      }
      if (event.key === 'Escape' && isFullscreen) setIsFullscreen(false)
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [isFullscreen, toggleWordWrap])

  const theme = TERMINAL_THEMES[logTheme]
  const firstTimestamp = lines.length
    ? parsePodLogLine(lines[0]).timestamp
    : undefined

  return (
    <Card
      className={`relative flex h-full flex-col gap-0 py-4 ${
        isFullscreen ? 'fixed inset-0 z-50 m-0 rounded-none' : ''
      }`}
    >
      <CardHeader className="gap-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <CardTitle className="text-lg">Logs</CardTitle>
            <CardDescription className="flex flex-wrap items-center gap-3">
              <span>
                {rows.length.toLocaleString()}
                {rows.length !== lines.length
                  ? ` / ${lines.length.toLocaleString()}`
                  : ''}{' '}
                lines
              </span>
              <ConnectionIndicator
                isConnected={(isLive || previous) && !error}
                onReconnect={refresh}
              >
                <span>
                  {previous ? 'Previous' : isLive ? 'Live' : 'Paused'}
                </span>
              </ConnectionIndicator>
              {firstTimestamp && (
                <span>From {formatLogTimestamp(firstTimestamp)}</span>
              )}
              {isLoading && <span>Loading…</span>}
            </CardDescription>
          </div>

          <div className="flex flex-wrap items-center justify-end gap-2">
            <div className="relative">
              <IconSearch className="pointer-events-none absolute top-2.5 left-2 h-4 w-4 text-muted-foreground" />
              <Input
                ref={searchInputRef}
                value={searchTerm}
                onChange={(event) => setSearchTerm(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') moveSearch(!event.shiftKey)
                  if (event.key === 'Escape') setSearchTerm('')
                }}
                placeholder={useRegex ? 'Search regex…' : 'Search logs…'}
                className={`w-56 pr-28 pl-8 ${matcherResult.invalid ? 'border-destructive' : ''}`}
              />
              <div className="absolute top-1.5 right-1 flex items-center gap-0.5">
                <Button
                  variant="ghost"
                  size="sm"
                  className={`h-7 px-1.5 text-xs ${caseSensitive ? 'bg-accent' : ''}`}
                  onClick={() => setCaseSensitive((value) => !value)}
                  title="Case sensitive"
                >
                  Aa
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className={`h-7 px-1.5 font-mono text-xs ${useRegex ? 'bg-accent' : ''}`}
                  onClick={() => setUseRegex((value) => !value)}
                  title="Regular expression"
                >
                  .*
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-1"
                  onClick={() => moveSearch(false)}
                  disabled={matchingRowIndices.length === 0}
                  title="Previous match"
                >
                  <IconArrowUp className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-1"
                  onClick={() => moveSearch(true)}
                  disabled={matchingRowIndices.length === 0}
                  title="Next match"
                >
                  <IconArrowDown className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>

            <Button
              variant={filterMatches ? 'secondary' : 'outline'}
              size="sm"
              onClick={() => setFilterMatches((value) => !value)}
              disabled={!searchTerm || matcherResult.invalid}
              title="Show only matching lines"
            >
              <IconFilter className="h-4 w-4" />
            </Button>

            {containers.length > 1 && (
              <ContainerSelector
                containers={containers}
                showAllOption={false}
                selectedContainer={selectedContainer}
                onContainerChange={setSelectedContainer}
              />
            )}

            {sortedPods && sortedPods.length > 1 && (
              <PodSelector
                pods={sortedPods}
                showAllOption={Boolean(labelSelector)}
                selectedPod={
                  selectedPodName === '_all' ? undefined : selectedPodName
                }
                onPodChange={(value) => setSelectedPodName(value || '_all')}
              />
            )}

            <Button
              variant="outline"
              size="sm"
              onClick={() => void handleLoadOlder()}
              disabled={!hasMore || isLoadingOlder}
              title="Load 500 older lines"
            >
              <IconHistory className="h-4 w-4" />
              {isLoadingOlder ? 'Loading…' : 'Older'}
            </Button>

            <Button
              variant="outline"
              size="sm"
              onClick={scrollToLatest}
              disabled={rows.length === 0}
              title="Jump to latest logs"
            >
              <IconArrowDown className="h-4 w-4" />
              Latest
            </Button>

            <Popover>
              <PopoverTrigger asChild>
                <Button variant="outline" size="sm">
                  <IconSettings className="h-4 w-4" />
                </Button>
              </PopoverTrigger>
              <PopoverContent className="w-80" align="end">
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <Label htmlFor="timestamps">Show timestamps</Label>
                    <Switch
                      id="timestamps"
                      checked={timestamps}
                      onCheckedChange={setTimestamps}
                    />
                  </div>
                  <div className="flex items-center justify-between">
                    <Label htmlFor="previous">Previous container</Label>
                    <Switch
                      id="previous"
                      checked={previous}
                      onCheckedChange={setPrevious}
                    />
                  </div>
                  <div className="flex items-center justify-between">
                    <Label htmlFor="word-wrap">Word wrap</Label>
                    <Switch
                      id="word-wrap"
                      checked={wordWrap}
                      onCheckedChange={toggleWordWrap}
                    />
                  </div>
                  <div className="flex items-center justify-between gap-3">
                    <Label>Theme</Label>
                    <Select value={logTheme} onValueChange={handleThemeChange}>
                      <SelectTrigger className="w-44">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {Object.entries(TERMINAL_THEMES).map(([key, value]) => (
                          <SelectItem key={key} value={key}>
                            {value.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="flex items-center justify-between gap-3">
                    <Label>Font size</Label>
                    <Select
                      value={fontSize.toString()}
                      onValueChange={(value) =>
                        handleFontSizeChange(Number(value))
                      }
                    >
                      <SelectTrigger className="w-44">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {[10, 11, 12, 13, 14, 15, 16, 18, 20, 22, 24].map(
                          (size) => (
                            <SelectItem key={size} value={size.toString()}>
                              {size}px
                            </SelectItem>
                          )
                        )}
                      </SelectContent>
                    </Select>
                  </div>
                  <p className="border-t pt-3 text-xs text-muted-foreground">
                    Loads 500 lines initially and 500 more when you reach the
                    top. Live updates poll every 5 seconds. At most{' '}
                    {maxLines.toLocaleString()} lines are kept in memory.
                  </p>
                </div>
              </PopoverContent>
            </Popover>

            <Button variant="outline" size="sm" onClick={clear} title="Clear">
              <IconClearAll className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={downloadLogs}
              disabled={lines.length === 0}
              title="Download loaded logs"
            >
              <IconDownload className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setIsFullscreen((value) => !value)}
              title={isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
            >
              {isFullscreen ? (
                <IconMinimize className="h-4 w-4" />
              ) : (
                <IconMaximize className="h-4 w-4" />
              )}
            </Button>
            {onClose && (
              <Button variant="outline" size="sm" onClick={onClose}>
                <IconX className="h-4 w-4" />
              </Button>
            )}
          </div>
        </div>

        {error && (
          <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error.message}
          </div>
        )}
        {warning && (
          <div className="whitespace-pre-wrap rounded-md border border-yellow-500/40 bg-yellow-500/10 px-3 py-2 text-sm text-yellow-700 dark:text-yellow-300">
            Some pod logs could not be loaded:
            <br />
            {warning}
          </div>
        )}
      </CardHeader>

      <CardContent
        className="relative min-h-0 p-0"
        style={{
          height: isFullscreen
            ? 'calc(100dvh - 150px)'
            : 'calc(100dvh - 285px)',
        }}
      >
        <div
          ref={scrollRef}
          onScroll={handleScroll}
          className="h-full w-full overflow-auto"
          style={{
            backgroundColor: theme.background,
            color: theme.foreground,
            overscrollBehavior: 'contain',
          }}
        >
          {isLoading && rows.length === 0 ? (
            <div className="flex h-full items-center justify-center text-sm">
              Loading logs…
            </div>
          ) : rows.length === 0 ? (
            <div className="flex h-full items-center justify-center text-sm">
              {filterMatches && searchTerm
                ? 'No matching log lines'
                : `There are no logs available for ${selectedContainer || 'container'}`}
            </div>
          ) : (
            <div
              className={wordWrap ? 'w-full' : 'min-w-max'}
              style={{
                height: `${virtualizer.getTotalSize()}px`,
                position: 'relative',
              }}
            >
              {virtualizer.getVirtualItems().map((virtualRow) => {
                const row = rows[virtualRow.index]
                const parsed = parsePodLogLine(row.line)
                const message = stripAnsi(parsed.message)
                return (
                  <div
                    key={`${row.originalIndex}-${virtualRow.key}`}
                    ref={virtualizer.measureElement}
                    data-index={virtualRow.index}
                    className="px-3 font-mono"
                    style={{
                      position: 'absolute',
                      top: 0,
                      left: 0,
                      width: wordWrap ? '100%' : 'max-content',
                      minWidth: '100%',
                      transform: `translateY(${virtualRow.start}px)`,
                      fontSize: `${fontSize}px`,
                      lineHeight: 1.5,
                      whiteSpace: wordWrap ? 'pre-wrap' : 'pre',
                      overflowWrap: wordWrap ? 'anywhere' : 'normal',
                    }}
                  >
                    {timestamps && parsed.timestamp && (
                      <span style={{ color: theme.brightBlack }}>
                        {formatLogTimestamp(parsed.timestamp)}{' '}
                      </span>
                    )}
                    {highlightMatches(message, matcherResult.matcher)}
                  </div>
                )
              })}
            </div>
          )}
        </div>

        {!followLatest && rows.length > 0 && (
          <Button
            size="sm"
            className="absolute right-4 bottom-4 shadow-lg"
            onClick={scrollToLatest}
          >
            <IconArrowDown className="h-4 w-4" />
            Latest
          </Button>
        )}
      </CardContent>
    </Card>
  )
}
