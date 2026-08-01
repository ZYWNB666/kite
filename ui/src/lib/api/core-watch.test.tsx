import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useResourcesWatch } from './core'

type EventListener = (event: MessageEvent<string>) => void

class MockEventSource {
  static instances: MockEventSource[] = []
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2

  readonly url: string
  readonly withCredentials: boolean
  readonly listeners = new Map<string, EventListener[]>()
  readonly close = vi.fn()
  readyState = 0
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null

  constructor(url: string | URL, options?: EventSourceInit) {
    this.url = String(url)
    this.withCredentials = options?.withCredentials ?? false
    MockEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: EventListener) {
    const listeners = this.listeners.get(type) ?? []
    listeners.push(listener)
    this.listeners.set(type, listeners)
  }

  emit(type: string, data: unknown) {
    const event = { data: JSON.stringify(data) } as MessageEvent<string>
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event)
    }
  }

  open() {
    this.readyState = MockEventSource.OPEN
    this.onopen?.()
  }
}

describe('useResourcesWatch', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    MockEventSource.instances = []
    vi.stubGlobal('EventSource', MockEventSource)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads a snapshot and batches incremental events', () => {
    const { result, unmount } = renderHook(() =>
      useResourcesWatch('services', 'default', { enabled: true })
    )
    const source = MockEventSource.instances[0]
    expect(source.url).toContain('/services/default/_watch')
    expect(source.withCredentials).toBe(true)

    act(() => {
      source.emit('snapshot', {
        resourceVersion: '10',
        items: [
          {
            metadata: {
              uid: 'uid-old',
              name: 'old',
              namespace: 'default',
              creationTimestamp: '2025-01-01T00:00:00Z',
            },
            spec: { type: 'ClusterIP' },
          },
        ],
      })
    })
    expect(result.current.data?.map((item) => item.metadata?.name)).toEqual([
      'old',
    ])
    expect(result.current.isConnected).toBe(false)

    act(() => {
      source.emit('ready', { resourceVersion: '10' })
    })
    expect(result.current.isConnected).toBe(true)

    act(() => {
      source.emit('modified', {
        metadata: {
          uid: 'uid-old',
          name: 'old',
          namespace: 'default',
          creationTimestamp: '2025-01-01T00:00:00Z',
        },
        spec: { type: 'LoadBalancer' },
      })
      source.emit('added', {
        metadata: {
          uid: 'uid-new',
          name: 'new',
          namespace: 'default',
          creationTimestamp: '2025-02-01T00:00:00Z',
        },
      })
      vi.advanceTimersByTime(100)
    })

    expect(result.current.data?.map((item) => item.metadata?.name)).toEqual([
      'new',
      'old',
    ])
    expect(result.current.data?.[1].spec?.type).toBe('LoadBalancer')

    unmount()
    expect(source.close).toHaveBeenCalledOnce()
  })

  it('sends selected namespaces on an all-namespaces watch', () => {
    renderHook(() =>
      useResourcesWatch('services', '_all', {
        enabled: true,
        namespaces: ['team-a', 'team-b'],
      })
    )

    const source = MockEventSource.instances[0]
    expect(source.url).toContain('/services/_all/_watch')
    expect(source.url).toContain('namespaces=team-a%2Cteam-b')
  })

  it('marks a fatal server error as unsupported for polling fallback', () => {
    const { result } = renderHook(() =>
      useResourcesWatch('configmaps', 'default', { enabled: true })
    )
    const source = MockEventSource.instances[0]

    act(() => {
      source.emit('watch-error', {
        error: 'watch is not supported',
        fatal: true,
      })
    })

    expect(result.current.isUnsupported).toBe(true)
    expect(result.current.error?.message).toBe('watch is not supported')
    expect(source.close).toHaveBeenCalledOnce()
  })

  it('allows a single retry after polling fallback disables watch', () => {
    const { result, rerender } = renderHook(
      ({ enabled }) => useResourcesWatch('configmaps', 'default', { enabled }),
      { initialProps: { enabled: true } }
    )
    const firstSource = MockEventSource.instances[0]

    act(() => {
      firstSource.emit('watch-error', {
        error: 'watch failed',
        fatal: true,
      })
    })
    expect(result.current.isUnsupported).toBe(true)

    rerender({ enabled: false })
    expect(result.current.isUnsupported).toBe(false)

    rerender({ enabled: true })
    expect(MockEventSource.instances).toHaveLength(2)
  })

  it('retries a browser-closed SSE connection after five seconds', () => {
    const { result } = renderHook(() =>
      useResourcesWatch('services', 'default', { enabled: true })
    )
    const firstSource = MockEventSource.instances[0]

    act(() => {
      firstSource.readyState = MockEventSource.CLOSED
      firstSource.onerror?.()
    })

    expect(result.current.isUnsupported).toBe(false)
    expect(MockEventSource.instances).toHaveLength(1)

    act(() => {
      vi.advanceTimersByTime(5000)
    })

    expect(firstSource.close).toHaveBeenCalledOnce()
    expect(MockEventSource.instances).toHaveLength(2)
  })

  it('reconnects when SSE remains connecting for five seconds', () => {
    renderHook(() =>
      useResourcesWatch('services', 'default', { enabled: true })
    )
    const firstSource = MockEventSource.instances[0]

    act(() => {
      vi.advanceTimersByTime(4999)
    })
    expect(MockEventSource.instances).toHaveLength(1)
    expect(firstSource.close).not.toHaveBeenCalled()

    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(firstSource.close).toHaveBeenCalledOnce()
    expect(MockEventSource.instances).toHaveLength(2)
  })

  it('keeps an SSE connection that opens within five seconds', () => {
    renderHook(() =>
      useResourcesWatch('services', 'default', { enabled: true })
    )
    const source = MockEventSource.instances[0]

    act(() => {
      vi.advanceTimersByTime(4500)
      source.open()
      vi.advanceTimersByTime(1000)
    })

    expect(source.close).not.toHaveBeenCalled()
    expect(MockEventSource.instances).toHaveLength(1)
  })

  it('starts a new five-second deadline after an open SSE disconnects', () => {
    renderHook(() =>
      useResourcesWatch('services', 'default', { enabled: true })
    )
    const source = MockEventSource.instances[0]

    act(() => {
      source.open()
      source.readyState = MockEventSource.CONNECTING
      source.onerror?.()
      vi.advanceTimersByTime(4999)
    })
    expect(MockEventSource.instances).toHaveLength(1)

    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(source.close).toHaveBeenCalledOnce()
    expect(MockEventSource.instances).toHaveLength(2)
  })

  it('falls back after repeated reconnect failures before a snapshot', () => {
    const { result } = renderHook(() =>
      useResourcesWatch('services', 'default', { enabled: true })
    )
    const source = MockEventSource.instances[0]

    act(() => {
      source.onopen?.()
      source.onerror?.()
      source.onopen?.()
      source.onerror?.()
      source.onopen?.()
      source.onerror?.()
    })

    expect(result.current.isUnsupported).toBe(true)
    expect(source.close).toHaveBeenCalledOnce()
  })

  it('reconnects and clears old data when the cluster changes', () => {
    const { result, rerender } = renderHook(
      ({ cluster }) =>
        useResourcesWatch('services', 'default', {
          enabled: true,
          cluster,
        }),
      { initialProps: { cluster: 'cluster-a' } }
    )
    const firstSource = MockEventSource.instances[0]
    expect(firstSource.url).toContain('x-cluster-name=cluster-a')

    act(() => {
      firstSource.emit('snapshot', {
        items: [
          {
            metadata: {
              uid: 'uid-a',
              name: 'from-cluster-a',
              namespace: 'default',
            },
          },
        ],
      })
    })
    expect(result.current.data).toHaveLength(1)

    rerender({ cluster: 'cluster-b' })

    expect(firstSource.close).toHaveBeenCalledOnce()
    expect(MockEventSource.instances).toHaveLength(2)
    expect(MockEventSource.instances[1].url).toContain(
      'x-cluster-name=cluster-b'
    )
    expect(result.current.data).toBeUndefined()
  })
})
