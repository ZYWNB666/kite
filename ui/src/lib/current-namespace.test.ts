// @vitest-environment jsdom

import { beforeEach, describe, expect, it } from 'vitest'

import {
  getCurrentNamespaces,
  getNamespacesFromUrl,
  setCurrentNamespaces,
  withCurrentNamespacesHref,
} from './current-namespace'

describe('tab-scoped namespace context', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    window.history.replaceState({}, '', '/pods?cluster=yunqiao')
  })

  it('uses repeated URL parameters as the authoritative selection', () => {
    sessionStorage.setItem('yunqiaoselectedNamespaces', '["test"]')
    window.history.replaceState(
      {},
      '',
      '/pods?cluster=yunqiao&namespace=default&namespace=team-a'
    )

    expect(getNamespacesFromUrl()).toEqual(['default', 'team-a'])
    expect(getCurrentNamespaces()).toEqual(['default', 'team-a'])
  })

  it('persists namespace changes only in sessionStorage', () => {
    expect(setCurrentNamespaces(['test'])).toEqual(['test'])
    expect(getCurrentNamespaces()).toEqual(['test'])
    expect(sessionStorage.getItem('yunqiaoselectedNamespaces')).toBe('["test"]')
    expect(localStorage.getItem('yunqiaoselectedNamespaces')).toBeNull()
  })

  it('reads shared storage only as a backwards-compatible fallback', () => {
    localStorage.setItem('yunqiaoselectedNamespaces', '["legacy"]')
    expect(getCurrentNamespaces()).toEqual(['legacy'])

    setCurrentNamespaces(['default'])
    expect(getCurrentNamespaces()).toEqual(['default'])
    expect(localStorage.getItem('yunqiaoselectedNamespaces')).toBe('["legacy"]')
  })

  it('adds the current selection to same-origin UI links only', () => {
    setCurrentNamespaces(['default', 'team-a'])

    expect(withCurrentNamespacesHref('/services?view=wide#top')).toBe(
      '/services?view=wide&namespace=default&namespace=team-a#top'
    )
    expect(withCurrentNamespacesHref('/api/v1/namespaces/default/pods')).toBe(
      '/api/v1/namespaces/default/pods'
    )
    expect(withCurrentNamespacesHref('https://example.com/docs')).toBe(
      'https://example.com/docs'
    )
  })
})
