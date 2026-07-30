// @vitest-environment jsdom

import { ClusterContext } from '@/contexts/cluster-context'
import { render, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { ClusterAwareLinks } from './cluster-aware-links'

describe('ClusterAwareLinks', () => {
  it('keeps the current cluster when a link is opened in another tab', async () => {
    const { getByRole } = render(
      <ClusterContext.Provider
        value={{
          clusters: [],
          currentCluster: 'cluster-a',
          setCurrentCluster: vi.fn(),
          isLoading: false,
          isSwitching: false,
          error: null,
        }}
      >
        <ClusterAwareLinks />
        <a href="/nodes?view=wide">Nodes</a>
      </ClusterContext.Provider>
    )

    await waitFor(() => {
      expect(getByRole('link', { name: 'Nodes' })).toHaveAttribute(
        'href',
        '/nodes?view=wide&cluster=cluster-a'
      )
    })
  })

  it('uses x-cluster-name for browser-direct API links', async () => {
    const { getByRole } = render(
      <ClusterContext.Provider
        value={{
          clusters: [],
          currentCluster: 'cluster-a',
          setCurrentCluster: vi.fn(),
          isLoading: false,
          isSwitching: false,
          error: null,
        }}
      >
        <ClusterAwareLinks />
        <a href="/api/v1/namespaces/dev/services/web:80/proxy/">
          Service proxy
        </a>
      </ClusterContext.Provider>
    )

    await waitFor(() => {
      expect(getByRole('link', { name: 'Service proxy' })).toHaveAttribute(
        'href',
        '/api/v1/namespaces/dev/services/web:80/proxy/?x-cluster-name=cluster-a'
      )
    })
  })
})
