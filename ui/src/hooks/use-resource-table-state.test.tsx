import { ReactNode } from 'react'
import { act, renderHook } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it } from 'vitest'

import { useResourceTableState } from './use-resource-table-state'

describe('useResourceTableState watch defaults', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    window.history.replaceState({}, '', '/pods?cluster=cluster-a')
  })

  const wrapper = ({ children }: { children: ReactNode }) => (
    <MemoryRouter initialEntries={['/pods?cluster=cluster-a']}>
      {children}
    </MemoryRouter>
  )

  it('uses watch by default and restores 5s polling when disabled', () => {
    const { result } = renderHook(
      () =>
        useResourceTableState({
          resourceName: 'Services',
          clusterScope: false,
          defaultHiddenColumns: [],
          watchSupported: true,
        }),
      { wrapper }
    )

    expect(result.current.useSSE).toBe(true)
    expect(result.current.refreshInterval).toBe(0)

    act(() => result.current.handleUseSSEChange(false))

    expect(result.current.useSSE).toBe(false)
    expect(result.current.refreshInterval).toBe(5000)
  })

  it('keeps unsupported resources on polling', () => {
    const { result } = renderHook(
      () =>
        useResourceTableState({
          resourceName: 'Nodes',
          clusterScope: true,
          defaultHiddenColumns: [],
          watchSupported: false,
        }),
      { wrapper }
    )

    expect(result.current.useSSE).toBe(false)
    expect(result.current.refreshInterval).toBe(5000)

    act(() => result.current.handleUseSSEChange(true))
    expect(result.current.useSSE).toBe(false)
  })

  it('uses URL namespaces and persists changes only in the current tab', () => {
    const namespaceWrapper = ({ children }: { children: ReactNode }) => (
      <MemoryRouter
        initialEntries={[
          '/pods?cluster=cluster-a&namespace=default&namespace=team-a',
        ]}
      >
        {children}
      </MemoryRouter>
    )
    const { result } = renderHook(
      () =>
        useResourceTableState({
          resourceName: 'Pods',
          clusterScope: false,
          defaultHiddenColumns: [],
          watchSupported: true,
        }),
      { wrapper: namespaceWrapper }
    )

    expect(result.current.selectedNamespaces).toEqual(['default', 'team-a'])

    act(() => result.current.handleNamespaceChange(['test']))

    expect(result.current.selectedNamespaces).toEqual(['test'])
    expect(sessionStorage.getItem('cluster-aselectedNamespaces')).toBe(
      '["test"]'
    )
    expect(localStorage.getItem('cluster-aselectedNamespaces')).toBeNull()
  })
})
