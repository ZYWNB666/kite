// @vitest-environment jsdom

import { ClusterContext } from '@/contexts/cluster-context'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ClusterAwareLinks } from './cluster-aware-links'
import { ClusterSelector } from './cluster-selector'

describe('ClusterSelector new-tab action', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/pods?cluster=mars1')
  })

  it('renders a native new-tab link for the selected target cluster', async () => {
    const user = userEvent.setup()
    render(
      <ClusterContext.Provider
        value={{
          clusters: [
            {
              id: 1,
              name: 'mars1',
              enabled: true,
              inCluster: false,
              isDefault: true,
              createdAt: '',
              updatedAt: '',
            },
            {
              id: 2,
              name: 'mars2',
              enabled: true,
              inCluster: false,
              isDefault: false,
              createdAt: '',
              updatedAt: '',
            },
          ],
          currentCluster: 'mars1',
          setCurrentCluster: vi.fn(),
          isLoading: false,
          isSwitching: false,
          error: null,
        }}
      >
        <ClusterAwareLinks />
        <ClusterSelector />
      </ClusterContext.Provider>
    )

    await user.click(screen.getByRole('button', { name: /mars1/i }))
    const newTabLink = screen.getByRole('menuitem', {
      name: 'Open mars2 in a new tab',
    })

    expect(newTabLink).toHaveAttribute('href', '/pods?cluster=mars2')
    expect(newTabLink).toHaveAttribute('target', '_blank')
    expect(newTabLink).toHaveAttribute('rel', 'noopener noreferrer')
  })
})
