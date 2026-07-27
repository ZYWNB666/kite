import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { useResourceTableState } from './use-resource-table-state'

describe('useResourceTableState watch defaults', () => {
  it('uses watch by default and restores 5s polling when disabled', () => {
    const { result } = renderHook(() =>
      useResourceTableState({
        resourceName: 'Services',
        clusterScope: false,
        defaultHiddenColumns: [],
        watchSupported: true,
      })
    )

    expect(result.current.useSSE).toBe(true)
    expect(result.current.refreshInterval).toBe(0)

    act(() => result.current.handleUseSSEChange(false))

    expect(result.current.useSSE).toBe(false)
    expect(result.current.refreshInterval).toBe(5000)
  })

  it('keeps unsupported resources on polling', () => {
    const { result } = renderHook(() =>
      useResourceTableState({
        resourceName: 'Nodes',
        clusterScope: true,
        defaultHiddenColumns: [],
        watchSupported: false,
      })
    )

    expect(result.current.useSSE).toBe(false)
    expect(result.current.refreshInterval).toBe(5000)

    act(() => result.current.handleUseSSEChange(true))
    expect(result.current.useSSE).toBe(false)
  })
})
