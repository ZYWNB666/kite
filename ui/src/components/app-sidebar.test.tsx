import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useCRDSummaries } from '@/lib/api'
import { SidebarProvider } from '@/components/ui/sidebar'

import { AppSidebar } from './app-sidebar'

const refetchCRDs = vi.fn()

vi.mock('@/contexts/auth-context', () => ({
  useAuth: () => ({ user: { isAdmin: () => true } }),
}))

vi.mock('@/contexts/sidebar-config-context', () => ({
  useSidebarConfig: () => ({
    config: {
      groups: [],
      hiddenItems: [],
      pinnedItems: [],
      groupOrder: [],
      lastUpdated: 0,
    },
    isLoading: false,
    getIconComponent: () => () => null,
  }),
}))

vi.mock('@/lib/api', () => ({
  useCRDSummaries: vi.fn(),
  useVersionInfo: () => ({ data: undefined }),
}))

vi.mock('./cluster-selector', () => ({ ClusterSelector: () => null }))
vi.mock('./version-info', () => ({ VersionInfo: () => null }))

describe('AppSidebar CRDs', () => {
  beforeEach(() => {
    refetchCRDs.mockReset()
    vi.mocked(useCRDSummaries).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      isFetching: false,
      refetch: refetchCRDs,
    } as ReturnType<typeof useCRDSummaries>)
  })

  it('keeps the CRD definitions entry visible when CRD loading fails', async () => {
    const user = userEvent.setup()
    render(
      <MemoryRouter>
        <SidebarProvider>
          <AppSidebar />
        </SidebarProvider>
      </MemoryRouter>
    )

    await user.click(screen.getByText('Custom Resources'))

    expect(screen.getByRole('link', { name: 'Definitions' })).toHaveAttribute(
      'href',
      '/crds'
    )
    await user.click(screen.getByText('Retry loading CRDs'))
    expect(refetchCRDs).toHaveBeenCalledTimes(1)
  })
})
