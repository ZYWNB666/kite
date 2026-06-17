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
  /** Search term for highlighting matches (not filtering). Empty = no highlight. */
  searchTerm?: string
  /** Whether to treat searchTerm as a regular expression. */
  useRegex?: boolean
  /** Whether the search is case-sensitive. */
  caseSensitive?: boolean
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
  const { maxLines = 10000, searchTerm = '', useRegex = false, caseSensitive = false, onClear } = options

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
  // Search refs kept in refs so the flush closure is stable.
  const searchRef = useRef(searchTerm)
  searchRef.current = searchTerm
  const useRegexRef = useRef(useRegex)
  useRegexRef.current = useRegex
  const caseSensitiveRef = useRef(caseSensitive)
  caseSensitiveRef.current = caseSensitive
  // onClear kept in ref to avoid re-subscribing.
  const onClearRef = useRef(onClear)
  onClearRef.current = onClear

  /**
   * Apply search highlighting to an HTML string by wrapping matches in <mark>.
   * Operates only on text nodes (not inside HTML tags) to avoid breaking the
   * pre-rendered ANSI spans.
   */
  const highlightHtml = useCallback((html: string): string => {
    const term = searchRef.current
    if (!term) return html

    let regex: RegExp
    if (useRegexRef.current) {
      try {
        regex = new RegExp(term, caseSensitiveRef.current ? 'g' : 'gi')
      } catch {
        // Invalid regex — no highlighting.
        return html
      }
    } else {
      // Escape regex special chars for literal search.
      const escaped = term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
      regex = new RegExp(escaped, caseSensitiveRef.current ? 'g' : 'gi')
    }

    // Split by HTML tags so we only highlight text between tags.
    // This is a simple split — safe because our ansiToHtml output is
    // well-formed with <span>...</span> structure.
    const parts = html.split(/(<[^>]*>)/)
    let result = ''
    for (const part of parts) {
      if (part.startsWith('<')) {
        // It's a tag, pass through unchanged.
        result += part
      } else {
        // It's text — escape HTML, then highlight matches.
        // But wait — the text is already escaped by ansiToHtml. So we can
        // apply regex directly, then wrap matches in <mark>.
        // We need to be careful: the text may contain &lt; &gt; &amp; entities.
        // We operate on the escaped text and wrap matches.
        result += part.replace(regex, (match) => `<mark class="log-highlight">${match}</mark>`)
      }
    }
    return result
  }, [])

  /**
   * Rebuild the visible entries array from the raw buffer applying the filter.
   * Called inside the RAF flush.
   */
  const rebuildEntries = useCallback(() => {
    const raws = rawBufferRef.current
    const htmls = htmlBufferRef.current
    const len = raws.length
    const hasSearch = !!searchRef.current

    // No filtering — all lines are visible. Highlight is applied on top of
    // the pre-rendered ANSI HTML.
    const prev = entriesRef.current
    const out: LogEntry[] = new Array(len)
    for (let i = 0; i < len; i++) {
      const prevEntry = i < prev.length ? prev[i] : undefined
      if (prevEntry && prevEntry.raw === raws[i]) {
        // Reuse existing entry, but re-apply highlight if search changed.
        out[i] = hasSearch
          ? { ...prevEntry, html: highlightHtml(htmls[i]) }
          : prevEntry
      } else {
        out[i] = {
          id: idCounterRef.current++,
          raw: raws[i],
          html: hasSearch ? highlightHtml(htmls[i]) : htmls[i],
        }
      }
    }
    entriesRef.current = out
  }, [highlightHtml])

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
  // Rebuild visible entries when the search options change.
  useEffect(() => {
    rebuildEntries()
    dispatch({ type: 'REFILTER' })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchTerm, useRegex, caseSensitive])

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
