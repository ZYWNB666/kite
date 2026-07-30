// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useCluster } from '@/hooks/use-cluster'

import { ClusterProvider } from './cluster-context'

const { refetchClusters } = vi.hoisted(() => ({
  refetchClusters: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  useCurrentClusterList: () => ({ refetch: refetchClusters }),
}))

function ClusterState() {
  const { currentCluster, isLoading } = useCluster()
  const location = useLocation()

  return (
    <div>
      {isLoading ? 'loading' : `${currentCluster ?? 'none'}${location.search}`}
    </div>
  )
}

describe('ClusterProvider default selection', () => {
  beforeEach(() => {
    sessionStorage.clear()
    sessionStorage.setItem('current-cluster', 'mars1')
    refetchClusters.mockReset()
    refetchClusters.mockResolvedValue({
      data: [
        {
          id: 1,
          name: 'mars1',
          enabled: true,
          inCluster: false,
          isDefault: false,
          createdAt: '',
          updatedAt: '',
        },
        {
          id: 2,
          name: 'mars2',
          enabled: true,
          inCluster: false,
          isDefault: true,
          createdAt: '',
          updatedAt: '',
        },
      ],
      error: null,
    })
  })

  it('uses the default cluster when the URL has no cluster parameter', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/pods']}>
          <ClusterProvider>
            <ClusterState />
          </ClusterProvider>
        </MemoryRouter>
      </QueryClientProvider>
    )

    await waitFor(() => {
      expect(screen.getByText('mars2?cluster=mars2')).toBeInTheDocument()
    })
    expect(sessionStorage.getItem('current-cluster')).toBe('mars2')
  })
})
