import { ReactNode } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import {
  MemoryRouter,
  NavigateFunction,
  useLocation,
  useNavigate,
} from 'react-router-dom'
import { beforeEach, describe, expect, it } from 'vitest'

import { useResourceTableState } from './use-resource-table-state'

function RouterProbe({
  onLocation,
  onNavigate,
}: {
  onLocation: (search: string) => void
  onNavigate: (navigate: NavigateFunction) => void
}) {
  const location = useLocation()
  const navigate = useNavigate()
  onLocation(location.search)
  onNavigate(navigate)
  return null
}

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
          filterableColumnIds: [],
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
          filterableColumnIds: [],
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
          filterableColumnIds: [],
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

  it('classifies resource search state in the URL and follows navigation', async () => {
    let currentSearch = ''
    let navigate: NavigateFunction | undefined
    const searchWrapper = ({ children }: { children: ReactNode }) => (
      <MemoryRouter
        initialEntries={[
          '/pods?cluster=cluster-a&namespace=default&q=api-.*&searchMode=regex&filter.status=Running&filter.status=Pending',
        ]}
      >
        <RouterProbe
          onLocation={(search) => {
            currentSearch = search
          }}
          onNavigate={(nextNavigate) => {
            navigate = nextNavigate
          }}
        />
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
          filterableColumnIds: ['status', 'nodeName'],
        }),
      { wrapper: searchWrapper }
    )

    expect(result.current.searchQuery).toBe('api-.*')
    expect(result.current.useRegex).toBe(true)
    expect(result.current.columnFilters).toEqual([
      { id: 'status', value: ['Running', 'Pending'] },
    ])

    act(() => result.current.setSearchQuery('worker'))
    await waitFor(() => {
      expect(new URLSearchParams(currentSearch).get('q')).toBe('worker')
    })

    act(() => result.current.handleUseRegexChange(false))
    await waitFor(() => {
      expect(new URLSearchParams(currentSearch).has('searchMode')).toBe(false)
    })

    act(() =>
      result.current.setColumnFilters([{ id: 'nodeName', value: ['node-01'] }])
    )
    await waitFor(() => {
      const params = new URLSearchParams(currentSearch)
      expect(params.getAll('filter.nodeName')).toEqual(['node-01'])
      expect(params.has('filter.status')).toBe(false)
    })

    act(() => result.current.handleNamespaceChange(['test']))
    await waitFor(() => {
      const params = new URLSearchParams(currentSearch)
      expect(params.getAll('namespace')).toEqual(['test'])
      expect(params.get('q')).toBe('worker')
    })

    act(() =>
      navigate?.(
        '/pods?cluster=cluster-a&namespace=default&q=database&searchMode=regex&filter.status=Failed'
      )
    )
    await waitFor(() => {
      expect(result.current.searchQuery).toBe('database')
      expect(result.current.useRegex).toBe(true)
      expect(result.current.columnFilters).toEqual([
        { id: 'status', value: ['Failed'] },
      ])
    })
  })
})
