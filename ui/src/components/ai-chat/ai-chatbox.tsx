import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent,
} from 'react'
import { useAIChatContext } from '@/contexts/ai-chat-context'
import { useLocation, useSearchParams } from 'react-router-dom'

import { useAIStatus } from '@/lib/api'
import { useIsMobile } from '@/hooks/use-mobile'
import { AIChatPanel } from '@/components/ai-chat/ai-chat-panel'
import { AIChatTrigger } from '@/components/ai-chat/ai-chat-trigger'

const MIN_HEIGHT = 200
const DESKTOP_DEFAULT_HEIGHT_RATIO = 0.62
const MIN_WIDTH = 320
const DEFAULT_WIDTH = 420
const DESKTOP_MARGIN = 16
const MOBILE_DEFAULT_HEIGHT_RATIO = 0.62

export type CornerPosition =
  | 'bottom-right'
  | 'bottom-left'
  | 'top-right'
  | 'top-left'

export function StandaloneAIChatbox() {
  const [searchParams] = useSearchParams()
  return (
    <AIChatbox
      standalone
      sessionId={searchParams.get('sessionId')?.trim() || ''}
    />
  )
}

export function AIChatbox({
  standalone = false,
  sessionId = '',
}: {
  standalone?: boolean
  sessionId?: string
}) {
  const isMobile = useIsMobile()
  const { isOpen, openChat, closeChat, corner, setCorner } =
    useAIChatContext()
  const { data: { enabled: aiEnabled } = { enabled: false } } = useAIStatus()
  const { pathname } = useLocation()
  const shouldShowAIChatbox = standalone || !/^\/settings\/?$/.test(pathname)

  const [height, setHeight] = useState(() =>
    Math.round(
      (window.visualViewport?.height ?? window.innerHeight) *
        DESKTOP_DEFAULT_HEIGHT_RATIO
    )
  )
  const [width, setWidth] = useState(DEFAULT_WIDTH)
  const [viewportSize, setViewportSize] = useState(() => ({
    width: window.visualViewport?.width ?? window.innerWidth,
    height: window.visualViewport?.height ?? window.innerHeight,
  }))
  const heightDragging = useRef(false)
  const widthDragging = useRef(false)
  const startY = useRef(0)
  const startH = useRef(0)
  const startX = useRef(0)
  const startW = useRef(0)

  // Drag-to-corner state
  const [isDragging, setIsDragging] = useState(false)
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
  const dragStartPos = useRef({ x: 0, y: 0 })
  const dragStartCorner = useRef(corner)
  const containerRef = useRef<HTMLDivElement>(null)
  const dragMoved = useRef(false)

  const getDesktopBounds = useCallback((vw: number, vh: number) => {
    const maxWidth = Math.max(MIN_WIDTH, Math.min(720, vw - DESKTOP_MARGIN))
    const minWidth = Math.min(MIN_WIDTH, maxWidth)
    const maxHeight = Math.max(MIN_HEIGHT, vh * 0.85)
    const minHeight = Math.min(MIN_HEIGHT, maxHeight)
    return { minWidth, maxWidth, minHeight, maxHeight }
  }, [])

  useEffect(() => {
    const updateViewport = () =>
      setViewportSize({
        width: window.visualViewport?.width ?? window.innerWidth,
        height: window.visualViewport?.height ?? window.innerHeight,
      })

    updateViewport()
    window.addEventListener('resize', updateViewport)
    window.visualViewport?.addEventListener('resize', updateViewport)
    return () => {
      window.removeEventListener('resize', updateViewport)
      window.visualViewport?.removeEventListener('resize', updateViewport)
    }
  }, [])

  useEffect(() => {
    if (isMobile) return
    const bounds = getDesktopBounds(viewportSize.width, viewportSize.height)
    setWidth((prev) =>
      Math.min(bounds.maxWidth, Math.max(bounds.minWidth, prev))
    )
    setHeight((prev) =>
      Math.min(bounds.maxHeight, Math.max(bounds.minHeight, prev))
    )
  }, [getDesktopBounds, isMobile, viewportSize.height, viewportSize.width])

  const desktopBounds = getDesktopBounds(
    viewportSize.width,
    viewportSize.height
  )
  const desktopWidth = Math.min(
    desktopBounds.maxWidth,
    Math.max(desktopBounds.minWidth, width)
  )
  const desktopHeight = Math.min(
    desktopBounds.maxHeight,
    Math.max(desktopBounds.minHeight, height)
  )

  const onPointerDown = useCallback(
    (e: PointerEvent) => {
      if (isMobile) return
      heightDragging.current = true
      startY.current = e.clientY
      startH.current = height
      ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
    },
    [height, isMobile]
  )

  const onPointerMove = useCallback(
    (e: PointerEvent) => {
      if (!heightDragging.current || isMobile) return
      const { minHeight, maxHeight } = getDesktopBounds(
        window.innerWidth,
        window.innerHeight
      )
      // Determine resize direction based on corner
      const isTop = corner.startsWith('top')
      const delta = isTop
        ? e.clientY - startY.current
        : startY.current - e.clientY
      const newH = Math.min(
        maxHeight,
        Math.max(minHeight, startH.current + delta)
      )
      setHeight(newH)
    },
    [corner, getDesktopBounds, isMobile]
  )

  const onPointerUp = useCallback(() => {
    heightDragging.current = false
  }, [])

  const onWidthPointerDown = useCallback(
    (e: PointerEvent) => {
      if (isMobile) return
      widthDragging.current = true
      startX.current = e.clientX
      startW.current = width
      ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
    },
    [isMobile, width]
  )

  const onWidthPointerMove = useCallback(
    (e: PointerEvent) => {
      if (!widthDragging.current || isMobile) return
      const { minWidth, maxWidth } = getDesktopBounds(
        window.innerWidth,
        window.innerHeight
      )
      // Determine resize direction based on corner
      const isLeft = corner.endsWith('left')
      const delta = isLeft
        ? e.clientX - startX.current
        : startX.current - e.clientX
      const newW = Math.min(
        maxWidth,
        Math.max(minWidth, startW.current + delta)
      )
      setWidth(newW)
    },
    [corner, getDesktopBounds, isMobile]
  )

  const onWidthPointerUp = useCallback(() => {
    widthDragging.current = false
  }, [])

  // Drag-to-corner: determine nearest corner from center position
  const snapToNearestCorner = useCallback(
    (centerX: number, centerY: number): CornerPosition => {
      const vw = viewportSize.width
      const vh = viewportSize.height
      const isLeft = centerX < vw / 2
      const isTop = centerY < vh / 2
      if (isTop && isLeft) return 'top-left'
      if (isTop && !isLeft) return 'top-right'
      if (!isTop && isLeft) return 'bottom-left'
      return 'bottom-right'
    },
    [viewportSize]
  )

  const handleDragStart = useCallback(
    (e: PointerEvent) => {
      if (isMobile || standalone) return
      // Only drag from header area (first 44px)
      const rect = containerRef.current?.getBoundingClientRect()
      if (!rect) return

      setIsDragging(true)
      dragMoved.current = false
      dragStartPos.current = { x: e.clientX, y: e.clientY }
      dragStartCorner.current = corner
      setDragOffset({ x: 0, y: 0 })
      ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
    },
    [corner, isMobile, standalone]
  )

  const handleDragMove = useCallback(
    (e: PointerEvent) => {
      if (!isDragging || isMobile) return
      const dx = e.clientX - dragStartPos.current.x
      const dy = e.clientY - dragStartPos.current.y
      if (!dragMoved.current && Math.abs(dx) + Math.abs(dy) < 5) return
      dragMoved.current = true
      setDragOffset({ x: dx, y: dy })
    },
    [isDragging, isMobile]
  )

  const handleDragEnd = useCallback(() => {
    if (!isDragging) return
    setIsDragging(false)

    if (!dragMoved.current) {
      setDragOffset({ x: 0, y: 0 })
      return
    }

    // Calculate the center of the chatbox during drag
    const rect = containerRef.current?.getBoundingClientRect()
    if (rect) {
      const centerX = rect.left + rect.width / 2
      const centerY = rect.top + rect.height / 2
      const newCorner = snapToNearestCorner(centerX, centerY)
      setCorner(newCorner)
    }

    setDragOffset({ x: 0, y: 0 })
  }, [isDragging, snapToNearestCorner, setCorner])

  if (!shouldShowAIChatbox) return null
  if (!aiEnabled) return null

  if (!standalone && !isOpen) {
    return <AIChatTrigger onOpen={openChat} corner={corner} />
  }

  // Calculate position styles based on corner
  const getCornerStyles = (): React.CSSProperties => {
    if (standalone || isMobile) return {}

    const margin = DESKTOP_MARGIN
    const base: React.CSSProperties = {}

    if (corner.startsWith('bottom')) {
      base.bottom = margin
      base.top = 'auto'
    } else {
      base.top = margin
      base.bottom = 'auto'
    }

    if (corner.endsWith('right')) {
      base.right = margin
      base.left = 'auto'
    } else {
      base.left = margin
      base.right = 'auto'
    }

    if (isDragging && dragMoved.current) {
      base.transform = `translate(${dragOffset.x}px, ${dragOffset.y}px)`
      base.transition = 'none'
    } else {
      base.transition = 'top 0.3s ease, bottom 0.3s ease, left 0.3s ease, right 0.3s ease'
    }

    return base
  }

  // Determine resize handle positions based on corner
  const isTop = corner.startsWith('top')
  const isLeft = corner.endsWith('left')

  const heightResizeStyle: React.CSSProperties = isTop
    ? { bottom: -4, left: 16, right: 16, height: 8, cursor: 'ns-resize' }
    : { top: -4, left: 16, right: 16, height: 8, cursor: 'ns-resize' }

  const widthResizeStyle: React.CSSProperties = isLeft
    ? { right: -4, top: 44, bottom: 0, width: 8, cursor: 'ew-resize' }
    : { left: -4, top: 44, bottom: 0, width: 8, cursor: 'ew-resize' }

  return (
    <div
      ref={containerRef}
      className={
        standalone
          ? 'fixed inset-0 z-50 flex flex-col bg-background'
          : `fixed z-50 flex flex-col border bg-background shadow-2xl ${
              isMobile ? 'left-2 right-2 rounded-lg' : 'rounded-lg'
            }`
      }
      style={
        standalone
          ? undefined
          : isMobile
            ? {
                bottom: `calc(env(safe-area-inset-bottom, 0px) + 0.5rem)`,
                height: `${MOBILE_DEFAULT_HEIGHT_RATIO * 100}%`,
              }
            : {
                width: desktopWidth,
                height: desktopHeight,
                ...getCornerStyles(),
              }
      }
    >
      {!isMobile && !standalone && (
        <div
          className="absolute z-10"
          style={heightResizeStyle}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
        />
      )}
      {!isMobile && !standalone && (
        <div
          className="absolute z-10"
          style={widthResizeStyle}
          onPointerDown={onWidthPointerDown}
          onPointerMove={onWidthPointerMove}
          onPointerUp={onWidthPointerUp}
        />
      )}

      {/* Drag handle overlay on the title bar */}
      {!isMobile && !standalone && (
        <div
          className={`absolute top-0 left-0 z-20 h-11 ${isDragging ? 'cursor-grabbing' : 'cursor-grab'}`}
          style={{ touchAction: 'none', right: 180 }}
          onPointerDown={handleDragStart}
          onPointerMove={handleDragMove}
          onPointerUp={handleDragEnd}
          onPointerCancel={handleDragEnd}
        />
      )}

      <AIChatPanel
        standalone={standalone}
        sessionId={sessionId}
        onClose={standalone ? () => window.close() : closeChat}
      />
    </div>
  )
}
