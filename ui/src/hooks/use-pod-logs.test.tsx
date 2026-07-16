import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

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

  it('immediately fetches and merges incremental logs when loading latest', async () => {
    const first = '2026-07-16T08:00:00.000000000Z first'
    const second = '2026-07-16T08:00:01.000000000Z second'
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
    mockedFetchPodLogs.mockResolvedValueOnce(logsResponse([first, second]))

    let added = 0
    await act(async () => {
      added = await result.current.loadLatest()
    })

    expect(added).toBe(1)
    expect(result.current.lines).toEqual([first, second])
    expect(mockedFetchPodLogs).toHaveBeenLastCalledWith(
      'default',
      'pod-a',
      expect.objectContaining({
        container: 'api',
        tailLines: -1,
        timestamps: true,
        sinceTime: '2026-07-16T08:00:00.000000000Z',
      })
    )
  })
})
