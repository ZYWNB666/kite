import { useVirtualizer } from '@tanstack/react-virtual'
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
} from 'react'

import { LogEntry } from '@/hooks/use-log-buffer'

export interface LogVirtualListHandle {
  scrollToBottom: () => void
  isAtBottom: () => boolean
  getVisibleEntries: () => LogEntry[]
}

interface LogVirtualListProps {
  entries: LogEntry[]
  /** Bumped whenever the buffer commits a batch — used to trigger auto-scroll. */
  version: number
  theme: {
    background: string
    foreground: string
  }
  fontSize: number
  wordWrap: boolean
  showLineNumbers: boolean
  lineHeight: number
}

/**
 * High-performance virtualized log list.
 *
 * Uses @tanstack/react-virtual to render only the visible rows (plus a small
 * overscan). Each row injects pre-rendered HTML via dangerouslySetInnerHTML
 * — no React reconciliation per log line, no Monaco, no per-line decorations.
 *
 * Auto-scroll-to-bottom behavior:
 *  - If the user is near the bottom when new lines arrive, we stick to bottom.
 *  - If the user has scrolled up, we do NOT auto-scroll (so they can read).
 *  - A `version` prop bump triggers a scroll check.
 */
export const LogVirtualList = forwardRef<
  LogVirtualListHandle,
  LogVirtualListProps
>(function LogVirtualList(
  {
    entries,
    version,
    theme,
    fontSize,
    wordWrap,
    showLineNumbers,
    lineHeight,
  },
  ref
) {
  const parentRef = useRef<HTMLDivElement | null>(null)
  const stickToBottomRef = useRef(true)
  const lastEntryCountRef = useRef(0)

  const count = entries.length

  const virtualizer = useVirtualizer({
    count,
    getScrollElement: () => parentRef.current,
    estimateSize: () => lineHeight,
    overscan: 20,
    // Stable keys via entry.id — but virtualizer uses index; that's fine
    // because rows are append-only / trimmed from the front.
    getItemKey: (index) => entries[index]?.id ?? index,
  })

  // Track whether the user is at (or near) the bottom of the scroll area.
  const handleScroll = useCallback(() => {
    const el = parentRef.current
    if (!el) return
    const distanceFromBottom =
      el.scrollHeight - el.scrollTop - el.clientHeight
    stickToBottomRef.current = distanceFromBottom < lineHeight * 2
  }, [lineHeight])

  // Auto-scroll to bottom when new lines arrive AND user was already at bottom.
  // Only trigger on count change (new lines), NOT on version change (filter rebuild).
  useLayoutEffect(() => {
    if (!stickToBottomRef.current) return
    if (count === 0) return
    if (count === lastEntryCountRef.current) return
    lastEntryCountRef.current = count
    // Use rAF to ensure DOM has been updated by the virtualizer.
    const raf = requestAnimationFrame(() => {
      virtualizer.scrollToIndex(count - 1, { align: 'end' })
    })
    return () => cancelAnimationFrame(raf)
  }, [count, virtualizer])

  // Reset scroll tracker when entries are cleared.
  useEffect(() => {
    if (count === 0) {
      stickToBottomRef.current = true
      lastEntryCountRef.current = 0
    }
  }, [count])

  // Ensure the virtualizer re-measures once after mount when the scroll element
  // becomes available. Only runs once — not on every count change.
  const didMeasureRef = useRef(false)
  useEffect(() => {
    if (!didMeasureRef.current && parentRef.current && count > 0) {
      didMeasureRef.current = true
      virtualizer.measure()
    }
  }, [count, virtualizer])

  useImperativeHandle(
    ref,
    () => ({
      scrollToBottom: () => {
        stickToBottomRef.current = true
        if (count > 0) {
          virtualizer.scrollToIndex(count - 1, { align: 'end' })
        }
      },
      isAtBottom: () => stickToBottomRef.current,
      getVisibleEntries: () => entries,
    }),
    [count, entries, virtualizer]
  )

  const items = virtualizer.getVirtualItems()

  // Total height of the virtual content (for the spacer divs).
  const totalSize = virtualizer.getTotalSize()
  // The first item's start offset.
  const firstItemStart = items.length > 0 ? items[0].start : 0

  return (
    <div
      ref={parentRef}
      onScroll={handleScroll}
      className="w-full overflow-auto"
      style={{
        height: '100%',
        backgroundColor: theme.background,
        color: theme.foreground,
        fontFamily:
          "'Maple Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace",
        fontSize: `${fontSize}px`,
        lineHeight: `${lineHeight}px`,
      }}
    >
      <div
        style={{
          height: `${totalSize}px`,
          position: 'relative',
          width: '100%',
        }}
      >
        <div
          style={{
            position: 'absolute',
            top: 0,
            left: 0,
            width: '100%',
            transform: `translateY(${firstItemStart}px)`,
          }}
        >
          {items.map((virtualItem) => {
            const entry = entries[virtualItem.index]
            if (!entry) return null
            const lineNumber = virtualItem.index + 1
            return (
              <div
                key={virtualItem.key}
                data-index={virtualItem.index}
                ref={virtualizer.measureElement}
                style={{
                  minHeight: `${lineHeight}px`,
                  display: 'flex',
                  alignItems: 'flex-start',
                }}
              >
                {showLineNumbers && (
                  <span
                    aria-hidden
                    style={{
                      flexShrink: 0,
                      width: `${Math.max(4, String(count).length) + 1}ch`,
                      textAlign: 'right',
                      paddingRight: '0.75rem',
                      marginRight: '0.5rem',
                      userSelect: 'none',
                      opacity: 0.45,
                      borderRight: `1px solid ${theme.foreground}22`,
                    }}
                  >
                    {lineNumber}
                  </span>
                )}
                <div
                  style={{
                    flex: 1,
                    whiteSpace: wordWrap ? 'pre-wrap' : 'pre',
                    wordBreak: wordWrap ? 'break-all' : 'normal',
                    // Padding at line end for horizontal scroll breathing room.
                    paddingRight: '1rem',
                  }}
                  dangerouslySetInnerHTML={{ __html: entry.html || '&nbsp;' }}
                />
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
})
