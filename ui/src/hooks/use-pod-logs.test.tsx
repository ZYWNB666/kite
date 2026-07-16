import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { fetchPodLogs } from '@/lib/api'

import { usePodLogs } from './use-pod-logs'

vi.mock('@/lib/api', () => ({
  fetchPodLogs: vi.fn(),
}))

const mockedFetchPodLogs = vi.mocked(fetchPodLogs)

function logsResponse(logs: string[]) {
  return {
    logs,
    container: 'api',
    pod: 'pod-a',
    namespace: 'default',
    hasMore: false,
  }
}

describe('usePodLogs', () => {
  beforeEach(() => {
    mockedFetchPodLogs.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('replaces the current window with the latest 500-line snapshot', async () => {
    const first = '2026-07-16T08:00:00.000000000Z first'
    const second = '2026-07-16T08:00:01.000000000Z second'
    const third = '2026-07-16T08:00:02.000000000Z third'
    mockedFetchPodLogs.mockResolvedValueOnce(logsResponse([first]))

    const { result } = renderHook(() =>
      usePodLogs({
        namespace: 'default',
        podName: 'pod-a',
        container: 'api',
        previous: false,
        enabled: true,
      })
    )

    await waitFor(() => expect(result.current.lines).toEqual([first]))
    mockedFetchPodLogs.mockResolvedValueOnce(logsResponse([second, third]))

    let added = 0
    await act(async () => {
      added = await result.current.jumpToPresent()
    })

    expect(added).toBe(1)
    expect(result.current.lines).toEqual([second, third])
    expect(mockedFetchPodLogs).toHaveBeenLastCalledWith(
      'default',
      'pod-a',
      expect.objectContaining({
        container: 'api',
        tailLines: 500,
        timestamps: true,
      })
    )
  })

  it('refreshes from a 500-line snapshot every 10 seconds', async () => {
    vi.useFakeTimers()
    const first = '2026-07-16T08:00:00.000000000Z first'
    const second = '2026-07-16T08:00:01.000000000Z second'
    mockedFetchPodLogs
      .mockResolvedValueOnce(logsResponse([first]))
      .mockResolvedValueOnce(logsResponse([first, second]))

    const { result } = renderHook(() =>
      usePodLogs({
        namespace: 'default',
        podName: 'pod-a',
        container: 'api',
        previous: false,
        enabled: true,
      })
    )

    await act(async () => {})
    expect(result.current.lines).toEqual([first])

    await act(async () => {
      await vi.advanceTimersByTimeAsync(9_999)
    })
    expect(mockedFetchPodLogs).toHaveBeenCalledTimes(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(mockedFetchPodLogs).toHaveBeenCalledTimes(2)
    expect(mockedFetchPodLogs).toHaveBeenLastCalledWith(
      'default',
      'pod-a',
      expect.objectContaining({ tailLines: 500 })
    )
    expect(result.current.lines).toEqual([first, second])
  })
})
