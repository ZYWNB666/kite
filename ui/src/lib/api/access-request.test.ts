import { describe, expect, it } from 'vitest'

import {
  getSelectableAccessRequestNamespaces,
  ROUTE_ADJUST_NAMESPACE,
} from './access-request'

describe('getSelectableAccessRequestNamespaces', () => {
  const namespaces = ['default', ROUTE_ADJUST_NAMESPACE, 'production']

  it.each(['full_update', 'canary_update'] as const)(
    'excludes the reserved route namespace for %s',
    (requestType) => {
      expect(
        getSelectableAccessRequestNamespaces(namespaces, requestType)
      ).toEqual(['default', 'production'])
    }
  )

  it('does not expose the namespace selector for route adjustments', () => {
    expect(
      getSelectableAccessRequestNamespaces(namespaces, 'route_adjust')
    ).toEqual([])
  })
})
