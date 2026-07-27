import { describe, expect, it } from 'vitest'

import {
  applyResourceWatchDeltas,
  getResourceWatchKey,
  sortWatchedResources,
  supportsResourceWatch,
  WatchableResource,
} from './resource-watch'

interface TestResource extends WatchableResource {
  value: string
}

function resource(
  name: string,
  value: string,
  creationTimestamp: string,
  uid?: string
): TestResource {
  return {
    metadata: {
      name,
      namespace: 'default',
      uid,
      creationTimestamp,
    },
    value,
  }
}

describe('resource watch state', () => {
  it('uses uid when available and namespace/name as a fallback', () => {
    expect(getResourceWatchKey(resource('demo', 'one', '', 'uid-1'))).toBe(
      'uid-1'
    )
    expect(getResourceWatchKey(resource('demo', 'one', ''))).toBe(
      'default/demo'
    )
  })

  it('applies added, modified, and deleted events in a batch', () => {
    const current = [
      resource('old', 'old-value', '2025-01-01T00:00:00Z', 'uid-old'),
      resource('remove', 'remove-me', '2025-01-02T00:00:00Z', 'uid-remove'),
    ]

    const next = applyResourceWatchDeltas(current, [
      {
        type: 'modified',
        object: resource('old', 'new-value', '2025-01-01T00:00:00Z', 'uid-old'),
      },
      {
        type: 'deleted',
        object: resource(
          'remove',
          'remove-me',
          '2025-01-02T00:00:00Z',
          'uid-remove'
        ),
      },
      {
        type: 'added',
        object: resource(
          'new',
          'new-resource',
          '2025-01-03T00:00:00Z',
          'uid-new'
        ),
      },
    ])

    expect(next.map((item) => item.metadata?.name)).toEqual(['new', 'old'])
    expect(next[1].value).toBe('new-value')
  })

  it('sorts the newest resource first with a stable name fallback', () => {
    const sorted = sortWatchedResources([
      resource('beta', 'two', '2025-01-01T00:00:00Z'),
      resource('newest', 'three', '2025-02-01T00:00:00Z'),
      resource('alpha', 'one', '2025-01-01T00:00:00Z'),
    ])
    expect(sorted.map((item) => item.metadata?.name)).toEqual([
      'newest',
      'alpha',
      'beta',
    ])
  })

  it('keeps enriched aggregate resources on polling', () => {
    expect(supportsResourceWatch('services')).toBe(true)
    expect(supportsResourceWatch('configmaps')).toBe(true)
    expect(supportsResourceWatch('custom.example.com')).toBe(true)
    expect(supportsResourceWatch('nodes')).toBe(false)
    expect(supportsResourceWatch('podmetrics')).toBe(false)
  })
})
