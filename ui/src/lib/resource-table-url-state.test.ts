// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'

import {
  hasResourceFilters,
  readResourceFilters,
  setResourceFiltersInSearchParams,
  setResourceQueryInSearchParams,
  setResourceSearchModeInSearchParams,
} from './resource-table-url-state'

describe('resource table URL state', () => {
  it('parses only allowed classified column filters', () => {
    const params = new URLSearchParams(
      'filter.status=Running&filter.status=Pending&filter.status=Running&filter.unknown=x'
    )

    expect(hasResourceFilters(params)).toBe(true)
    expect(readResourceFilters(params, ['status', 'nodeName'])).toEqual([
      { id: 'status', value: ['Running', 'Pending'] },
    ])
  })

  it('writes search categories without disturbing cluster and namespace', () => {
    const params = new URLSearchParams(
      'cluster=yunqiao&namespace=default&filter.old=value'
    )

    setResourceQueryInSearchParams(params, 'nginx-.*')
    setResourceSearchModeInSearchParams(params, true)
    setResourceFiltersInSearchParams(
      params,
      [
        { id: 'status', value: ['Running', 'Pending'] },
        { id: 'unknown', value: ['ignored'] },
      ],
      ['status']
    )

    expect(params.get('cluster')).toBe('yunqiao')
    expect(params.getAll('namespace')).toEqual(['default'])
    expect(params.get('q')).toBe('nginx-.*')
    expect(params.get('searchMode')).toBe('regex')
    expect(params.getAll('filter.status')).toEqual(['Running', 'Pending'])
    expect(params.has('filter.old')).toBe(false)
    expect(params.has('filter.unknown')).toBe(false)
  })

  it('removes empty and disabled search categories', () => {
    const params = new URLSearchParams(
      'q=nginx&searchMode=regex&filter.status=Running'
    )

    setResourceQueryInSearchParams(params, '')
    setResourceSearchModeInSearchParams(params, false)
    setResourceFiltersInSearchParams(params, [], ['status'])

    expect(params.toString()).toBe('')
  })
})
