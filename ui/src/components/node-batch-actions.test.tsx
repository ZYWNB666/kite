import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, vi } from 'vitest'

import { NodeWithMetrics } from '@/types/api'

import { NodeBatchActions } from './node-batch-actions'

const {
  mockCordonNode,
  mockDrainNode,
  mockTaintNode,
  mockUncordonNode,
  mockUntaintNode,
} = vi.hoisted(() => ({
  mockCordonNode: vi.fn(),
  mockDrainNode: vi.fn(),
  mockTaintNode: vi.fn(),
  mockUncordonNode: vi.fn(),
  mockUntaintNode: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  cordonNode: mockCordonNode,
  drainNode: mockDrainNode,
  taintNode: mockTaintNode,
  uncordonNode: mockUncordonNode,
  untaintNode: mockUntaintNode,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
  },
}))

function node(name: string, unschedulable = false): NodeWithMetrics {
  return {
    metadata: { name },
    spec: { unschedulable },
  } as NodeWithMetrics
}

describe('NodeBatchActions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockCordonNode.mockResolvedValue({})
  })

  it('cordons only schedulable nodes and reports skipped nodes', async () => {
    const user = userEvent.setup()
    const refresh = vi.fn().mockResolvedValue(undefined)
    const clearSelection = vi.fn()

    render(
      <NodeBatchActions
        selectedNodes={[node('worker-1'), node('worker-2', true)]}
        clearSelection={clearSelection}
        refresh={refresh}
      />
    )

    await user.click(screen.getByRole('button', { name: /bulk actions/i }))
    await user.click(screen.getByRole('menuitem', { name: 'Cordon' }))

    expect(screen.getByText(/Apply this operation to 1 of 2/)).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Cordon 1 node' }))

    await waitFor(() => expect(mockCordonNode).toHaveBeenCalledTimes(1))
    expect(mockCordonNode).toHaveBeenCalledWith('worker-1')
    expect(screen.getByText('Already cordoned')).toBeVisible()
    expect(refresh).toHaveBeenCalledTimes(1)

    await user.click(screen.getAllByRole('button', { name: 'Close' })[0])
    expect(clearSelection).toHaveBeenCalledTimes(1)
  })

  it('uses serial drain execution by default', async () => {
    const user = userEvent.setup()
    let activeCalls = 0
    let maxActiveCalls = 0
    mockDrainNode.mockImplementation(async () => {
      activeCalls += 1
      maxActiveCalls = Math.max(maxActiveCalls, activeCalls)
      await new Promise((resolve) => setTimeout(resolve, 5))
      activeCalls -= 1
      return { pods: 1 }
    })

    render(
      <NodeBatchActions
        selectedNodes={[node('worker-1'), node('worker-2')]}
        clearSelection={vi.fn()}
        refresh={vi.fn().mockResolvedValue(undefined)}
      />
    )

    await user.click(screen.getByRole('button', { name: /bulk actions/i }))
    await user.click(screen.getByRole('menuitem', { name: 'Drain' }))
    fireEvent.click(screen.getByRole('button', { name: 'Drain 2 nodes' }))

    await waitFor(() => expect(mockDrainNode).toHaveBeenCalledTimes(2))
    expect(maxActiveCalls).toBe(1)
  })
})
