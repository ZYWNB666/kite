// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from 'vitest'

import { apiClient } from './api-client'

describe('ApiClient cluster isolation', () => {
  beforeEach(() => {
    sessionStorage.clear()
    window.history.replaceState({}, '', '/pods?cluster=cluster-a')
  })

  it('captures the URL cluster per request without dropping custom headers', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 200 }))

    await apiClient.request('/pods', {
      headers: new Headers({ 'x-request-id': 'request-1' }),
    })
    window.history.replaceState({}, '', '/pods?cluster=cluster-b')
    await apiClient.request('/nodes')

    const firstHeaders = fetchMock.mock.calls[0][1]?.headers as Headers
    const secondHeaders = fetchMock.mock.calls[1][1]?.headers as Headers
    expect(firstHeaders.get('x-cluster-name')).toBe('cluster-a')
    expect(firstHeaders.get('x-request-id')).toBe('request-1')
    expect(secondHeaders.get('x-cluster-name')).toBe('cluster-b')
  })

  it('binds every write request to the URL cluster at dispatch time', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 200 }))

    await apiClient.post('/resources/apply', { yaml: 'kind: ConfigMap' })
    window.history.replaceState({}, '', '/pods?cluster=cluster-b')
    await apiClient.patch('/pods/default/example', {
      metadata: { labels: { updated: 'true' } },
    })
    await apiClient.delete('/pods/default/example')

    expect(fetchMock).toHaveBeenCalledTimes(3)

    const [applyUrl, applyOptions] = fetchMock.mock.calls[0]
    const [patchUrl, patchOptions] = fetchMock.mock.calls[1]
    const [deleteUrl, deleteOptions] = fetchMock.mock.calls[2]

    expect(applyUrl).toBe('/api/v1/resources/apply')
    expect(applyOptions?.method).toBe('POST')
    expect((applyOptions?.headers as Headers).get('x-cluster-name')).toBe(
      'cluster-a'
    )
    expect(patchUrl).toBe('/api/v1/pods/default/example')
    expect(patchOptions?.method).toBe('PATCH')
    expect((patchOptions?.headers as Headers).get('x-cluster-name')).toBe(
      'cluster-b'
    )
    expect(deleteUrl).toBe('/api/v1/pods/default/example')
    expect(deleteOptions?.method).toBe('DELETE')
    expect((deleteOptions?.headers as Headers).get('x-cluster-name')).toBe(
      'cluster-b'
    )
  })
})
