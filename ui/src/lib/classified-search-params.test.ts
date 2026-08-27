// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'

import { setClassifiedSearchParam } from './classified-search-params'

describe('classified search parameters', () => {
  it('updates one search category without disturbing the others', () => {
    const params = new URLSearchParams(
      'tab=users&users.q=alice&audit.q=deployment&audit.operation=update'
    )

    setClassifiedSearchParam(params, 'users.role', 'developer')
    setClassifiedSearchParam(params, 'users.q', '')

    expect(params.get('tab')).toBe('users')
    expect(params.has('users.q')).toBe(false)
    expect(params.get('users.role')).toBe('developer')
    expect(params.get('audit.q')).toBe('deployment')
    expect(params.get('audit.operation')).toBe('update')
  })
})
