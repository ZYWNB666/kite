import { useCallback, useEffect, useReducer, useRef } from 'react'

/**
 * A single rendered log line.
 * - `raw` is the original string (with ANSI codes) used for downloads.
 * - `html` is the pre-rendered HTML (ANSI converted to spans) for fast DOM injection.
 */
export interface LogEntry {
  id: number
  raw: string
  html: string
}

export interface LogBufferOptions {
  /** Maximum number of lines kept in memory. Older lines are dropped. */
  maxLines?: number
  /** Optional case-insensitive substring filter applied to raw text. */
  filterTerm?: string
  /** Called whenever the buffer content is cleared. */
  onClear?: () => void
}

interface LogBufferState {
  /** Version counter — bumped on every committed batch so React re-renders. */
  version: number
  /** Total number of log lines received (for stats display), not capped. */
  totalReceived: number
  /** Whether at least one batch has been committed. */
  hasData: boolean
}

type LogBufferAction =
  | { type: 'COMMIT'; count: number }
  | { type: 'REFILTER' }
  | { type: 'CLEAR' }
  | { type: 'RESET' }

function reducer(state: LogBufferState, action: LogBufferAction): LogBufferState {
  switch (action.type) {
    case 'COMMIT':
      return {
        version: state.version + 1,
        totalReceived: state.totalReceived + action.count,
        hasData: true,
      }
    case 'REFILTER':
      // Filter changed — just bump version to re-render, don't touch stats.
      return { ...state, version: state.version + 1 }
    case 'CLEAR':
    case 'RESET':
      return { version: 0, totalReceived: 0, hasData: false }
    default:
      return state
  }
}

/**
 * Escape HTML special characters in plain text segments.
 */
function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

/**
 * Convert ANSI escape sequences in a log line to styled HTML spans.
 * This runs once per line at ingestion time so the render path stays cheap.
 */
function ansiToHtml(input: string): string {
  // Regex matches CSI sequences like \x1b[1;31m
  const ansiRegex = /\x1b\[([0-9;]*)m/g
  let result = ''
  let lastIndex = 0
  let match: RegExpExecArray | null
  // Current open span class list
  let currentClass = ''

  const openSpan = (cls: string) => {
    if (currentClass !== cls) {
      if (currentClass) result += '</span>'
      currentClass = cls
      if (cls) result += `<span class="${cls}">`
    }
  }

  const closeSpan = () => {
    if (currentClass) {
      result += '</span>'
      currentClass = ''
    }
  }

  while ((match = ansiRegex.exec(input)) !== null) {
    // Emit escaped text before the escape sequence
    if (match.index > lastIndex) {
      const text = input.slice(lastIndex, match.index)
      if (text) {
        openSpan(currentClass) // ensure span open if we have a style
        result += escapeHtml(text)
      }
    }

    // Parse the codes
    const codes = match[1] ? match[1].split(';').map(Number) : [0]
    const newClass = buildClassFromCodes(codes, currentClass)
    openSpan(newClass)

    lastIndex = match.index + match[0].length
  }

  // Emit remaining text
  if (lastIndex < input.length) {
    const text = input.slice(lastIndex)
    openSpan(currentClass)
    result += escapeHtml(text)
  }

  closeSpan()
  return result
}

// ANSI color code → CSS class mapping (mirrors ansi-parser.ts colors)
const FG_CLASS: Record<number, string> = {
  30: 'ansi-fg-30', 31: 'ansi-fg-31', 32: 'ansi-fg-32', 33: 'ansi-fg-33',
  34: 'ansi-fg-34', 35: 'ansi-fg-35', 36: 'ansi-fg-36', 37: 'ansi-fg-37',
  90: 'ansi-fg-90', 91: 'ansi-fg-91', 92: 'ansi-fg-92', 93: 'ansi-fg-93',
  94: 'ansi-fg-94', 95: 'ansi-fg-95', 96: 'ansi-fg-96', 97: 'ansi-fg-97',
}
const BG_CLASS: Record<number, string> = {
  40: 'ansi-bg-40', 41: 'ansi-bg-41', 42: 'ansi-bg-42', 43: 'ansi-bg-43',
  44: 'ansi-bg-44', 45: 'ansi-bg-45', 46: 'ansi-bg-46', 47: 'ansi-bg-47',
  100: 'ansi-bg-100', 101: 'ansi-bg-101', 102: 'ansi-bg-102', 103: 'ansi-bg-103',
  104: 'ansi-bg-104', 105: 'ansi-bg-105', 106: 'ansi-bg-106', 107: 'ansi-bg-107',
}

function buildClassFromCodes(codes: number[], currentClass: string): string {
  if (codes.length === 0 || (codes.length === 1 && codes[0] === 0)) {
    return '' // reset
  }

  // Parse existing classes into a set for incremental updates
  const parts = new Set(currentClass.split(' ').filter(Boolean))

  for (const code of codes) {
    switch (code) {
      case 0:
        return ''
      case 1:
        parts.add('ansi-bold')
        break
      case 3:
        parts.add('ansi-italic')
        break
      case 4:
        parts.add('ansi-underline')
        break
      case 22:
        parts.delete('ansi-bold')
        break
      case 23:
        parts.delete('ansi-italic')
        break
      case 24:
        parts.delete('ansi-underline')
        break
      case 39:
        // remove all fg classes
        for (const p of parts) {
          if (p.startsWith('ansi-fg-')) parts.delete(p)
        }
        break
      case 49:
        for (const p of parts) {
          if (p.startsWith('ansi-bg-')) parts.delete(p)
        }
        break
      default:
        if (FG_CLASS[code]) {
          // remove existing fg
          for (const p of parts) {
            if (p.startsWith('ansi-fg-')) parts.delete(p)
          }
          parts.add(FG_CLASS[code])
        } else if (BG_CLASS[code]) {
          for (const p of parts) {
            if (p.startsWith('ansi-bg-')) parts.delete(p)
          }
          parts.add(BG_CLASS[code])
        }
        break
    }
  }

  return Array.from(parts).join(' ')
}

/**
 * useLogBuffer — high-performance log line buffer.
 *
 * - Ring buffer with configurable max line cap (default 10000).
 * - Incoming lines are accumulated in a pending array and flushed once per
 *   animation frame via requestAnimationFrame, so thousands of lines per
 *   second produce at most ~60 React re-renders.
 * - ANSI → HTML conversion happens at ingestion time (in the pending batch),
 *   not during render. The render path only does `dangerouslySetInnerHTML`.
 * - Filtering is applied at flush time so toggling the filter does not require
 *   re-streaming from the server.
 *
 * Returns:
 *   - `entries`: the visible (filtered) log lines currently in the buffer.
 *   - `append`: push a raw log line (called from WebSocket handler).
 *   - `clear`: wipe the buffer.
 *   - `downloadText`: produce a plain-text blob of all raw lines.
 *   - stats: `totalReceived`, `visibleCount`, `version`, `hasData`.
 */
export function useLogBuffer(options: LogBufferOptions = {}) {
  const { maxLines = 10000, filterTerm = '', onClear } = options

  const [state, dispatch] = useReducer(reducer, {
    version: 0,
    totalReceived: 0,
    hasData: false,
  })

  // ---- mutable refs (no re-render needed for these) ----
  // Raw ring buffer of all received lines (raw text only).
  const rawBufferRef = useRef<string[]>([])
  // Pre-rendered HTML for each raw line (same index as rawBufferRef).
  const htmlBufferRef = useRef<string[]>([])
  // The visible (filtered) entries exposed to React. We rebuild this on flush.
  const entriesRef = useRef<LogEntry[]>([])
  // Stable copy of entries exposed via ref for consumers that read outside render.
  const idCounterRef = useRef(0)
  // Pending batch awaiting next animation frame.
  const pendingRef = useRef<string[]>([])
  // RAF handle.
  const rafRef = useRef<number | null>(null)
  // Current filter (lower-cased) kept in a ref so the flush closure is stable.
  const filterRef = useRef(filterTerm)
  filterRef.current = filterTerm.toLowerCase()
  // onClear kept in ref to avoid re-subscribing.
  const onClearRef = useRef(onClear)
  onClearRef.current = onClear

  /**
   * Rebuild the visible entries array from the raw buffer applying the filter.
   * Called inside the RAF flush.
   */
  const rebuildEntries = useCallback(() => {
    const filter = filterRef.current
    const raws = rawBufferRef.current
    const htmls = htmlBufferRef.current
    const len = raws.length

    if (!filter) {
      // Fast path: no filter, map directly (but assign stable ids).
      // We reuse ids from the previous entries array when possible to keep
      // React keys stable across flushes.
      const prev = entriesRef.current
      const out: LogEntry[] = new Array(len)
      for (let i = 0; i < len; i++) {
        const prevEntry = i < prev.length ? prev[i] : undefined
        out[i] = prevEntry && prevEntry.raw === raws[i]
          ? prevEntry
          : { id: idCounterRef.current++, raw: raws[i], html: htmls[i] }
      }
      entriesRef.current = out
    } else {
      const out: LogEntry[] = []
      for (let i = 0; i < len; i++) {
        if (raws[i].toLowerCase().includes(filter)) {
          out.push({ id: idCounterRef.current++, raw: raws[i], html: htmls[i] })
        }
      }
      entriesRef.current = out
    }
  }, [])

  /**
   * Flush the pending batch into the ring buffer and trigger a re-render.
   * Runs inside requestAnimationFrame.
   */
  const flush = useCallback(() => {
    rafRef.current = null
    const pending = pendingRef.current
    if (pending.length === 0) return

    pendingRef.current = []

    // Append to ring buffer with cap.
    const raws = rawBufferRef.current
    const htmls = htmlBufferRef.current
    for (let i = 0; i < pending.length; i++) {
      const raw = pending[i]
      raws.push(raw)
      htmls.push(ansiToHtml(raw))
    }

    // Trim if over capacity.
    const overflow = raws.length - maxLines
    if (overflow > 0) {
      raws.splice(0, overflow)
      htmls.splice(0, overflow)
    }

    rebuildEntries()
    dispatch({ type: 'COMMIT', count: pending.length })
  }, [maxLines, rebuildEntries])

  const scheduleFlush = useCallback(() => {
    if (rafRef.current !== null) return
    rafRef.current = requestAnimationFrame(flush)
  }, [flush])

  /**
   * Append a raw log line. Called from the WebSocket onMessage handler —
   * may be invoked thousands of times per second. This function only pushes
   * to a plain array and schedules a single RAF; no React state is touched
   * here.
   */
  const append = useCallback(
    (raw: string) => {
      // Skip empty lines (server sometimes sends trailing newlines).
      if (raw === '') return
      pendingRef.current.push(raw)
      scheduleFlush()
    },
    [scheduleFlush]
  )

  /**
   * Clear all buffered lines.
   */
  const clear = useCallback(() => {
    // Cancel any pending flush.
    if (rafRef.current !== null) {
      cancelAnimationFrame(rafRef.current)
      rafRef.current = null
    }
    pendingRef.current = []
    rawBufferRef.current = []
    htmlBufferRef.current = []
    entriesRef.current = []
    idCounterRef.current = 0
    dispatch({ type: 'CLEAR' })
    onClearRef.current?.()
  }, [])

  /**
   * Produce a plain-text download of all raw lines currently in the buffer.
   */
  const downloadText = useCallback((): string => {
    return rawBufferRef.current.join('\n')
  }, [])

  /**
   * Rebuild visible entries when the filter changes (no re-stream needed).
   */
  useEffect(() => {
    rebuildEntries()
    dispatch({ type: 'REFILTER' })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterTerm])

  // Cleanup on unmount: cancel any pending RAF.
  useEffect(() => {
    return () => {
      if (rafRef.current !== null) {
        cancelAnimationFrame(rafRef.current)
        rafRef.current = null
      }
    }
  }, [])

  return {
    entries: entriesRef.current,
    append,
    clear,
    downloadText,
    totalReceived: state.totalReceived,
    visibleCount: entriesRef.current.length,
    version: state.version,
    hasData: state.hasData,
  }
}
