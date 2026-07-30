// @vitest-environment jsdom

import { beforeEach, describe, expect, it } from 'vitest'

import {
  appendCurrentClusterHeader,
  appendCurrentClusterParam,
  clearCurrentCluster,
  getClusterFromUrl,
  getClusterQueryKey,
  getCurrentCluster,
  setCurrentCluster,
  withClusterHref,
  withClusterRequestHref,
} from './current-cluster'

describe('tab-scoped cluster context', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    window.history.replaceState({}, '', '/pods?cluster=cluster-a&namespace=dev')
  })

  it('uses the URL as the authoritative cluster for the tab', () => {
    sessionStorage.setItem('current-cluster', 'cluster-b')
    localStorage.setItem('current-cluster', 'cluster-c')

    expect(getCurrentCluster()).toBe('cluster-a')
    expect(getClusterQueryKey('pods')).toEqual(['cluster', 'cluster-a', 'pods'])
  })

  it('falls back only to sessionStorage and never writes shared storage', () => {
    window.history.replaceState({}, '', '/pods')

    setCurrentCluster('cluster-b')
    expect(getClusterFromUrl()).toBeNull()
    expect(getCurrentCluster()).toBe('cluster-b')
    expect(sessionStorage.getItem('current-cluster')).toBe('cluster-b')
    expect(localStorage.getItem('current-cluster')).toBeNull()

    clearCurrentCluster()
    expect(getCurrentCluster()).toBeNull()
  })

  it('adds the selected cluster to headers and streaming URLs', () => {
    const headers = new Headers({ 'x-request-id': 'request-1' })
    appendCurrentClusterHeader(headers)
    expect(headers.get('x-cluster-name')).toBe('cluster-a')
    expect(headers.get('x-request-id')).toBe('request-1')

    const params = new URLSearchParams('tailLines=100')
    appendCurrentClusterParam(params)
    expect(params.get('x-cluster-name')).toBe('cluster-a')

    appendCurrentClusterParam(params, 'cluster-b')
    expect(params.getAll('x-cluster-name')).toEqual(['cluster-b'])
  })

  it('creates a same-origin URL for a different cluster', () => {
    expect(withClusterHref('/nodes?view=wide#top', 'cluster-b')).toBe(
      '/nodes?view=wide&cluster=cluster-b#top'
    )
    expect(withClusterHref('https://example.com/docs', 'cluster-b')).toBe(
      'https://example.com/docs'
    )
  })

  it('uses the backend cluster query key for browser-direct API URLs', () => {
    expect(
      withClusterRequestHref('/api/v1/namespaces/dev/services/web:80/proxy/')
    ).toBe(
      '/api/v1/namespaces/dev/services/web:80/proxy/?x-cluster-name=cluster-a'
    )
  })
})
