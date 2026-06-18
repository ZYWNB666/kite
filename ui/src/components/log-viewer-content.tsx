import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  IconArrowDown,
  IconArrowUp,
  IconClearAll,
  IconDownload,
  IconMaximize,
  IconMinimize,
  IconPalette,
  IconSearch,
  IconSettings,
  IconX,
} from '@tabler/icons-react'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { Terminal as XTerm } from '@xterm/xterm'
import { Container, Pod } from 'kubernetes-types/core/v1'

import '@xterm/xterm/css/xterm.css'

import { TERMINAL_THEMES, TerminalTheme } from '@/types/themes'
import { useLogsWebSocket } from '@/lib/api'
import { toSimpleContainer } from '@/lib/k8s'
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
import { NetworkSpeedIndicator } from './network-speed-indicator'
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

const TERMINAL_THEME_KEYS = Object.keys(TERMINAL_THEMES) as TerminalTheme[]
const TERMINAL_THEME_ENTRIES = Object.entries(TERMINAL_THEMES) as Array<
  [TerminalTheme, (typeof TERMINAL_THEMES)[TerminalTheme]]
>

/** Max lines kept in xterm scrollback buffer */
const SCROLLBACK = 3000

function buildXtermTheme(t: (typeof TERMINAL_THEMES)[TerminalTheme]) {
  return {
    background: t.background,
    foreground: t.foreground,
    cursor: t.cursor,
    selectionBackground: t.selection,
    black: t.black,
    red: t.red,
    green: t.green,
    yellow: t.yellow,
    blue: t.blue,
    magenta: t.magenta,
    cyan: t.cyan,
    white: t.white,
    brightBlack: t.brightBlack,
    brightRed: t.brightRed,
    brightGreen: t.brightGreen,
    brightYellow: t.brightYellow,
    brightBlue: t.brightBlue,
    brightMagenta: t.brightMagenta,
    brightCyan: t.brightCyan,
    brightWhite: t.brightWhite,
  }
}

export function LogViewer({
  namespace,
  podName,
  pods,
  containers: _containers,
  initContainers,
  onClose,
  labelSelector,
}: LogViewerProps) {
  // ── Persisted preferences ─────────────────────────────────────────────────
  const [logTheme, setLogTheme] = useState<TerminalTheme>(() => {
    const saved = localStorage.getItem('log-viewer-theme')
    return (saved as TerminalTheme) || 'classic'
  })
  const [fontSize, setFontSize] = useState(() => {
    const saved = localStorage.getItem('log-viewer-font-size')
    return saved ? parseInt(saved, 10) : 14
  })
  const [tailLines, setTailLines] = useState(() => {
    const saved = localStorage.getItem('log-viewer-tail-lines')
    return saved ? parseInt(saved, 10) : 100
  })
  const [wordWrap, setWordWrap] = useState(() => {
    const saved = localStorage.getItem('log-viewer-word-wrap')
    return saved === null ? true : saved === 'true'
  })

  // ── UI state ──────────────────────────────────────────────────────────────
  const [timestamps, setTimestamps] = useState(false)
  const [previous, setPrevious] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [useRegex, setUseRegex] = useState(false)
  const [caseSensitive, setCaseSensitive] = useState(false)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [isReconnecting, setIsReconnecting] = useState(false)
  const [lineCount, setLineCount] = useState(0)

  const containers = useMemo(
    () => toSimpleContainer(initContainers, _containers),
    [_containers, initContainers]
  )
  const [selectedContainer, setSelectedContainer] = useState<
    string | undefined
  >(containers.length > 0 ? containers[0].name : '')

  const sortedPods = useMemo(() => {
    if (!pods) return undefined
    return [...pods].sort((a, b) =>
      (a.metadata?.creationTimestamp || 0) >
      (b.metadata?.creationTimestamp || 0)
        ? -1
        : 1
    )
  }, [pods])

  const [selectPodName, setSelectPodName] = useState<string | undefined>(
    podName || pods?.[0]?.metadata?.name || undefined
  )

  const currentTheme = TERMINAL_THEMES[logTheme]

  // ── xterm refs ────────────────────────────────────────────────────────────
  const terminalDivRef = useRef<HTMLDivElement | null>(null)
  const xtermRef = useRef<XTerm | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const searchAddonRef = useRef<SearchAddon | null>(null)
  const pendingLinesRef = useRef<string[]>([])
  const rafRef = useRef<number | null>(null)
  const lineCountRef = useRef(0)
  const searchInputRef = useRef<HTMLInputElement | null>(null)
  // Keep wordWrap readable inside ResizeObserver closure without re-creating xterm
  const wordWrapRef = useRef(wordWrap)
  wordWrapRef.current = wordWrap

  // ── Pod / container sync ──────────────────────────────────────────────────
  useEffect(() => {
    if (podName) {
      setSelectPodName(podName)
      return
    }
    if (pods && pods.length > 0) {
      setSelectPodName((prev) => {
        if (prev === '_all') return prev
        if (!prev || !pods.find((p) => p.metadata?.name === prev)) {
          return pods[0].metadata?.name
        }
        return prev
      })
    }
  }, [podName, pods])

  useEffect(() => {
    if (containers.length > 0) setSelectedContainer(containers[0].name)
  }, [containers])

  // ── xterm initialisation — runs once on mount ─────────────────────────────
  useEffect(() => {
    if (!terminalDivRef.current) return

    const terminal = new XTerm({
      fontFamily:
        "'Maple Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace",
      fontSize: fontSize,
      theme: buildXtermTheme(TERMINAL_THEMES[logTheme]),
      disableStdin: true,
      cursorBlink: false,
      convertEol: true,
      scrollback: SCROLLBACK,
      allowTransparency: false,
      allowProposedApi: true,
      overviewRulerWidth: 10,
    })

    const fitAddon = new FitAddon()
    const searchAddon = new SearchAddon()
    terminal.loadAddon(fitAddon)
    terminal.loadAddon(searchAddon)
    terminal.open(terminalDivRef.current)
    fitAddon.fit()

    xtermRef.current = terminal
    fitAddonRef.current = fitAddon
    searchAddonRef.current = searchAddon

    const ro = new ResizeObserver(() => {
      if (wordWrapRef.current && fitAddonRef.current) {
        fitAddonRef.current.fit()
      }
    })
    ro.observe(terminalDivRef.current!)

    return () => {
      ro.disconnect()
      if (rafRef.current !== null) {
        cancelAnimationFrame(rafRef.current)
        rafRef.current = null
      }
      terminal.dispose()
      xtermRef.current = null
      fitAddonRef.current = null
      searchAddonRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ── Theme: update in-place (no xterm recreation, no data loss) ───────────
  useEffect(() => {
    if (!xtermRef.current) return
    xtermRef.current.options.theme = buildXtermTheme(TERMINAL_THEMES[logTheme])
    xtermRef.current.refresh(0, xtermRef.current.rows - 1)
  }, [logTheme])

  // ── Font size: update in-place ────────────────────────────────────────────
  useEffect(() => {
    if (!xtermRef.current || !fitAddonRef.current) return
    xtermRef.current.options.fontSize = fontSize
    fitAddonRef.current.fit()
  }, [fontSize])

  // ── Word wrap ─────────────────────────────────────────────────────────────
  useEffect(() => {
    if (!xtermRef.current || !fitAddonRef.current) return
    if (wordWrap) {
      fitAddonRef.current.fit()
    } else {
      // Large fixed cols → long lines scroll horizontally instead of wrapping
      xtermRef.current.resize(500, xtermRef.current.rows)
    }
  }, [wordWrap])

  // ── Re-fit after fullscreen CSS transition ────────────────────────────────
  useEffect(() => {
    const timer = setTimeout(() => {
      if (fitAddonRef.current && wordWrap) fitAddonRef.current.fit()
    }, 200)
    return () => clearTimeout(timer)
  }, [isFullscreen, wordWrap])

  // ── Batched line ingestion via requestAnimationFrame ──────────────────────
  const flush = useCallback(() => {
    rafRef.current = null
    const pending = pendingLinesRef.current
    if (pending.length === 0 || !xtermRef.current) return
    pendingLinesRef.current = []
    // Single write per frame — xterm handles ANSI codes natively, no parsing needed
    xtermRef.current.write(pending.join('\r\n') + '\r\n')
    lineCountRef.current += pending.length
    // If we exceeded the scrollback limit, trim old lines from the buffer.
    // xterm drops old lines automatically when scrollback is exceeded,
    // but we also update our counter so the displayed count stays accurate.
    const buf = xtermRef.current.buffer.active
    if (buf.length > SCROLLBACK) {
      lineCountRef.current = buf.length
    }
    setLineCount(lineCountRef.current)
  }, [])

  const appendLine = useCallback(
    (line: string) => {
      if (!line) return
      pendingLinesRef.current.push(line)
      if (rafRef.current === null) {
        rafRef.current = requestAnimationFrame(flush)
      }
    },
    [flush]
  )

  const clearTerminal = useCallback(() => {
    if (rafRef.current !== null) {
      cancelAnimationFrame(rafRef.current)
      rafRef.current = null
    }
    pendingLinesRef.current = []
    lineCountRef.current = 0
    setLineCount(0)
    xtermRef.current?.clear()
  }, [])

  // ── Search (SearchAddon highlights all occurrences natively) ─────────────
  const searchOptions = useMemo(
    () => ({
      regex: useRegex,
      caseSensitive,
      wholeWord: false,
      decorations: {
        matchBackground: 'rgba(255, 213, 0, 0.30)',
        matchBorder: 'rgba(255, 213, 0, 0.70)',
        matchOverviewRuler: '#ffd500',
        activeMatchBackground: 'rgba(255, 140, 0, 0.60)',
        activeMatchBorder: 'rgba(255, 140, 0, 1.00)',
        activeMatchColorOverviewRuler: '#ff8c00',
      },
    }),
    [useRegex, caseSensitive]
  )

  // Highlight all occurrences whenever term or options change
  useEffect(() => {
    if (!searchAddonRef.current) return
    if (searchTerm) {
      searchAddonRef.current.findNext(searchTerm, {
        ...searchOptions,
        incremental: true,
      })
    } else {
      searchAddonRef.current.findNext('', searchOptions)
    }
  }, [searchTerm, searchOptions])

  const doSearch = useCallback(
    (forward: boolean) => {
      if (!searchAddonRef.current || !searchTerm) return
      if (forward) {
        searchAddonRef.current.findNext(searchTerm, searchOptions)
      } else {
        searchAddonRef.current.findPrevious(searchTerm, searchOptions)
      }
    },
    [searchTerm, searchOptions]
  )

  // ── Download — reads from xterm scrollback buffer ─────────────────────────
  const downloadLogs = useCallback(() => {
    const terminal = xtermRef.current
    if (!terminal || lineCount === 0) return
    const buf = terminal.buffer.active
    const lines: string[] = []
    for (let i = 0; i < buf.length; i++) {
      const line = buf.getLine(i)
      if (line) lines.push(line.translateToString(true))
    }
    const content = lines.join('\n').trimEnd()
    if (!content) return
    const blob = new Blob([content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    const podFileName = selectPodName || 'all-pods'
    a.download = `${podFileName}-${selectedContainer || 'pod'}-logs.txt`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }, [lineCount, selectPodName, selectedContainer])

  // ── WebSocket ─────────────────────────────────────────────────────────────
  const logsOptions = useMemo(
    () => ({
      container: selectedContainer,
      tailLines,
      timestamps,
      previous,
      enabled: !!selectPodName,
      labelSelector,
      onNewLog: appendLine,
      onClear: clearTerminal,
    }),
    [
      selectedContainer,
      tailLines,
      timestamps,
      previous,
      selectPodName,
      labelSelector,
      appendLine,
      clearTerminal,
    ]
  )

  const {
    isLoading,
    isConnected,
    downloadSpeed,
    refetch,
    stopStreaming,
    clearLogs,
  } = useLogsWebSocket(namespace, selectPodName || '', logsOptions)

  const stopStreamingRef = useRef(stopStreaming)
  stopStreamingRef.current = stopStreaming
  useEffect(() => () => stopStreamingRef.current(), [])

  // Reconnecting indicator
  useEffect(() => {
    setIsReconnecting(true)
    const timer = setTimeout(() => {
      if (!isLoading) setIsReconnecting(false)
    }, 500)
    return () => clearTimeout(timer)
  }, [selectedContainer, selectPodName, tailLines, timestamps, previous, isLoading])

  useEffect(() => {
    if (!isLoading) {
      const timer = setTimeout(() => setIsReconnecting(false), 200)
      return () => clearTimeout(timer)
    }
  }, [isLoading])

  // ── Preference handlers ───────────────────────────────────────────────────
  const handleThemeChange = useCallback((theme: TerminalTheme) => {
    setLogTheme(theme)
    localStorage.setItem('log-viewer-theme', theme)
  }, [])

  const handleFontSizeChange = useCallback((size: number) => {
    setFontSize(size)
    localStorage.setItem('log-viewer-font-size', size.toString())
  }, [])

  const handleTailLinesChange = useCallback((lines: number) => {
    setTailLines(lines)
    if (lines !== -1) {
      localStorage.setItem('log-viewer-tail-lines', lines.toString())
    }
  }, [])

  const cycleTheme = useCallback(() => {
    const idx = TERMINAL_THEME_KEYS.indexOf(logTheme)
    handleThemeChange(
      TERMINAL_THEME_KEYS[(idx + 1) % TERMINAL_THEME_KEYS.length]
    )
  }, [logTheme, handleThemeChange])

  const toggleFullscreen = useCallback(() => setIsFullscreen((v) => !v), [])

  const toggleWordWrap = useCallback(() => {
    setWordWrap((prev) => {
      localStorage.setItem('log-viewer-word-wrap', `${!prev}`)
      return !prev
    })
  }, [])

  // ── Keyboard shortcuts ────────────────────────────────────────────────────
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
        e.preventDefault()
        searchInputRef.current?.focus()
        searchInputRef.current?.select()
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') toggleFullscreen()
      if (e.altKey && (e.key === 'z' || e.key === 'Z' || e.key === 'Ω')) {
        e.preventDefault()
        toggleWordWrap()
      }
      if ((e.ctrlKey || e.metaKey) && (e.key === '=' || e.key === '+')) {
        e.preventDefault()
        handleFontSizeChange(Math.min(24, fontSize + 1))
      }
      if ((e.ctrlKey || e.metaKey) && (e.key === '-' || e.key === '_')) {
        e.preventDefault()
        handleFontSizeChange(Math.max(10, fontSize - 1))
      }
      if ((e.ctrlKey || e.metaKey) && e.key === '0') {
        e.preventDefault()
        handleFontSizeChange(14)
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [toggleFullscreen, toggleWordWrap, fontSize, handleFontSizeChange])

  // ── Render ────────────────────────────────────────────────────────────────
  return (
    <Card
      className={`h-full flex flex-col py-4 gap-0 ${
        isFullscreen ? 'fixed inset-0 z-50 m-0 rounded-none' : ''
      }`}
    >
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <CardTitle className="text-lg">Logs</CardTitle>
            <CardDescription>
              <div className="flex items-center gap-4 text-sm text-muted-foreground">
                <span>{lineCount.toLocaleString()} lines</span>
                <ConnectionIndicator
                  isConnected={isConnected}
                  onReconnect={refetch}
                />
                <NetworkSpeedIndicator
                  downloadSpeed={downloadSpeed}
                  uploadSpeed={0}
                />
                {isLoading && <span>Loading...</span>}
                {isReconnecting && (
                  <span className="text-blue-600">Reconnecting...</span>
                )}
              </div>
            </CardDescription>
          </div>

          <div className="flex items-center gap-2">
            {/* Search bar: term + Aa + .* + ↑ + ↓ */}
            <div className="relative">
              <IconSearch className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground pointer-events-none" />
              <Input
                ref={searchInputRef}
                placeholder={useRegex ? 'Search (regex)...' : 'Search...'}
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') doSearch(!e.shiftKey)
                  if (e.key === 'Escape') setSearchTerm('')
                }}
                className="pl-8 pr-28 w-52"
              />
              <div className="absolute right-1 top-1.5 flex items-center gap-0.5">
                <Button
                  variant="ghost"
                  size="sm"
                  className={`h-7 px-1.5 text-xs ${
                    caseSensitive
                      ? 'bg-accent text-accent-foreground'
                      : 'text-muted-foreground'
                  }`}
                  onClick={() => setCaseSensitive((v) => !v)}
                  title="Case sensitive"
                >
                  Aa
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className={`h-7 px-1.5 text-xs font-mono ${
                    useRegex
                      ? 'bg-accent text-accent-foreground'
                      : 'text-muted-foreground'
                  }`}
                  onClick={() => setUseRegex((v) => !v)}
                  title="Use regex"
                >
                  .*
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-1 text-muted-foreground hover:text-foreground"
                  onClick={() => doSearch(false)}
                  title="Previous match (Shift+Enter)"
                  disabled={!searchTerm}
                >
                  <IconArrowUp className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-1 text-muted-foreground hover:text-foreground"
                  onClick={() => doSearch(true)}
                  title="Next match (Enter)"
                  disabled={!searchTerm}
                >
                  <IconArrowDown className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>

            {/* Container Selector */}
            {containers.length > 1 && (
              <ContainerSelector
                containers={containers}
                showAllOption={false}
                selectedContainer={selectedContainer}
                onContainerChange={setSelectedContainer}
              />
            )}

            {/* Pod Selector */}
            {sortedPods && (
              <PodSelector
                pods={sortedPods}
                showAllOption={true}
                selectedPod={selectPodName}
                onPodChange={(v) => setSelectPodName(v || '_all')}
              />
            )}

            {/* Theme cycle */}
            <Button
              variant="outline"
              size="sm"
              onClick={cycleTheme}
              title={`Current theme: ${currentTheme.name}`}
              className="relative"
            >
              <IconPalette className="h-4 w-4" />
              <div
                className="absolute -top-1 -right-1 w-3 h-3 rounded-full border border-gray-400"
                style={{ backgroundColor: currentTheme.background }}
              />
            </Button>

            {/* Settings popover */}
            <Popover>
              <PopoverTrigger asChild>
                <Button variant="outline" size="sm">
                  <IconSettings className="h-4 w-4" />
                </Button>
              </PopoverTrigger>
              <PopoverContent className="w-80" align="end">
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <Label>Tail Lines</Label>
                    <Select
                      value={tailLines.toString()}
                      onValueChange={(v) => handleTailLinesChange(Number(v))}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {[50, 100, 200, 500, 1000].map((n) => (
                          <SelectItem key={n} value={n.toString()}>
                            {n}
                          </SelectItem>
                        ))}
                        <SelectItem value="-1">All</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="flex items-center justify-between">
                    <Label htmlFor="timestamps">Show Timestamps</Label>
                    <Switch
                      id="timestamps"
                      checked={timestamps}
                      onCheckedChange={setTimestamps}
                    />
                  </div>

                  <div className="flex items-center justify-between">
                    <Label htmlFor="previous">Previous Container</Label>
                    <Switch
                      id="previous"
                      checked={previous}
                      onCheckedChange={setPrevious}
                    />
                  </div>

                  <div className="flex items-center justify-between">
                    <Label htmlFor="word-wrap">Word Wrap</Label>
                    <Switch
                      id="word-wrap"
                      checked={wordWrap}
                      onCheckedChange={toggleWordWrap}
                    />
                  </div>

                  <div className="flex items-center justify-between">
                    <Label>Log Theme</Label>
                    <Select value={logTheme} onValueChange={handleThemeChange}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {TERMINAL_THEME_ENTRIES.map(([key, theme]) => (
                          <SelectItem key={key} value={key}>
                            {theme.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="flex items-center justify-between">
                    <Label>Font Size</Label>
                    <Select
                      value={fontSize.toString()}
                      onValueChange={(v) => handleFontSizeChange(Number(v))}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {[10, 11, 12, 13, 14, 15, 16, 18, 20, 22, 24].map(
                          (s) => (
                            <SelectItem key={s} value={s.toString()}>
                              {s}px
                            </SelectItem>
                          )
                        )}
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="space-y-2 pt-2 border-t">
                    <Label className="text-xs font-medium text-muted-foreground">
                      Keyboard Shortcuts
                    </Label>
                    <div className="space-y-1 text-xs text-muted-foreground">
                      {[
                        ['Focus Search', 'Ctrl+F'],
                        ['Next Match', 'Enter'],
                        ['Prev Match', 'Shift+Enter'],
                        ['Toggle Fullscreen', 'Ctrl+Enter'],
                        ['Toggle Word Wrap', 'Alt+Z'],
                        ['Font +', 'Ctrl++'],
                        ['Font −', 'Ctrl+-'],
                        ['Font Reset', 'Ctrl+0'],
                      ].map(([lbl, key]) => (
                        <div key={key} className="flex justify-between">
                          <span>{lbl}</span>
                          <kbd className="px-1 py-0.5 bg-muted rounded text-xs">
                            {key}
                          </kbd>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              </PopoverContent>
            </Popover>

            {/* Clear */}
            <Button
              variant="outline"
              size="sm"
              onClick={clearLogs}
              title="Clear logs"
            >
              <IconClearAll className="h-4 w-4" />
            </Button>

            {/* Download */}
            <Button
              variant="outline"
              size="sm"
              onClick={downloadLogs}
              disabled={lineCount === 0}
            >
              <IconDownload className="h-4 w-4" />
            </Button>

            {/* Fullscreen */}
            <Button
              variant="outline"
              size="sm"
              onClick={toggleFullscreen}
              title={
                isFullscreen ? 'Exit fullscreen (ESC)' : 'Enter fullscreen'
              }
            >
              {isFullscreen ? (
                <IconMinimize className="h-4 w-4" />
              ) : (
                <IconMaximize className="h-4 w-4" />
              )}
            </Button>

            {/* Close */}
            {onClose && (
              <Button variant="outline" size="sm" onClick={onClose}>
                <IconX className="h-4 w-4" />
              </Button>
            )}
          </div>
        </div>
      </CardHeader>

      <CardContent
        className="p-0 overflow-hidden"
        style={{
          height: isFullscreen
            ? 'calc(100dvh - 60px)'
            : 'calc(100dvh - 255px)',
        }}
      >
        {/* xterm.js mounts here — it manages its own DOM/canvas */}
        <div
          ref={terminalDivRef}
          className="h-full w-full"
          style={{ overscrollBehavior: 'none' }}
        />
      </CardContent>
    </Card>
  )
}
