import {
  createContext,
  ReactNode,
  useCallback,
  useContext,
  useMemo,
  useState,
} from 'react'
import { useLocation, useParams } from 'react-router-dom'

import { PageContext } from '@/components/ai-chat/ai-chat-types'
import { toSingularResource } from '@/components/ai-chat/ai-chat-utils'

import type { CornerPosition } from '@/components/ai-chat/ai-chatbox'

interface AIChatContextType {
  isOpen: boolean
  openChat: () => void
  closeChat: () => void
  toggleChat: () => void
  pageContext: PageContext
  /** Which corner the chat panel is docked to */
  corner: CornerPosition
  /** Move the chat panel to a specific corner */
  setCorner: (corner: CornerPosition) => void
}

const AIChatContext = createContext<AIChatContextType | undefined>(undefined)

export function AIChatProvider({ children }: { children: ReactNode }) {
  const [isOpen, setIsOpen] = useState(false)
  const [corner, setCorner] = useState<CornerPosition>(() => {
    const saved = localStorage.getItem('ai-chat-corner')
    if (
      saved === 'bottom-right' ||
      saved === 'bottom-left' ||
      saved === 'top-right' ||
      saved === 'top-left'
    ) {
      return saved
    }
    return 'bottom-right'
  })
  const location = useLocation()
  const params = useParams()

  const openChat = useCallback(() => {
    setIsOpen(true)
  }, [])

  const closeChat = useCallback(() => {
    setIsOpen(false)
  }, [])

  const toggleChat = useCallback(() => {
    setIsOpen((prev) => !prev)
  }, [])

  const handleSetCorner = useCallback((newCorner: CornerPosition) => {
    setCorner(newCorner)
    localStorage.setItem('ai-chat-corner', newCorner)
  }, [])

  const pageContext = useMemo<PageContext>(() => {
    const path = location.pathname
    const searchParams = new URLSearchParams(location.search)

    if (path === '/ai-chat-box') {
      return {
        page: searchParams.get('page') || 'overview',
        namespace: searchParams.get('namespace') || '',
        resourceName: searchParams.get('resourceName') || '',
        resourceKind: toSingularResource(
          searchParams.get('resourceKind') || ''
        ),
      }
    }

    const resource = params.resource || ''
    const name = params.name || ''
    const namespace = params.namespace || ''
    const normalizedKind = toSingularResource(resource)

    let page = 'overview'
    if (path === '/' || path === '/dashboard') {
      page = 'overview'
    } else if (name) {
      page = `${normalizedKind}-detail`
    } else if (resource) {
      page = `${resource}-list`
    }

    return {
      page,
      namespace,
      resourceName: name,
      resourceKind: normalizedKind,
    }
  }, [
    location.pathname,
    location.search,
    params.resource,
    params.name,
    params.namespace,
  ])

  return (
    <AIChatContext.Provider
      value={{
        isOpen,
        openChat,
        closeChat,
        toggleChat,
        pageContext,
        corner,
        setCorner: handleSetCorner,
      }}
    >
      {children}
    </AIChatContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAIChatContext() {
  const context = useContext(AIChatContext)
  if (context === undefined) {
    throw new Error('useAIChatContext must be used within an AIChatProvider')
  }
  return context
}
