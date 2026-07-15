import { StorageClass } from 'kubernetes-types/storage/v1'

import {
  getStorageClassParameters,
  getStorageClassTopologyRules,
  isDefaultStorageClass,
} from './storageclass'

describe('storageclass helpers', () => {
  it('recognizes stable and legacy default class annotations', () => {
    const stable = {
      metadata: {
        annotations: { 'storageclass.kubernetes.io/is-default-class': 'true' },
      },
    } as StorageClass
    const legacy = {
      metadata: {
        annotations: {
          'storageclass.beta.kubernetes.io/is-default-class': 'TRUE',
        },
      },
    } as StorageClass

    expect(isDefaultStorageClass(stable)).toBe(true)
    expect(isDefaultStorageClass(legacy)).toBe(true)
    expect(isDefaultStorageClass({} as StorageClass)).toBe(false)
  })

  it('formats parameters and topology expressions for compact display', () => {
    const storageClass = {
      parameters: { type: 'ssd', encrypted: 'true' },
      allowedTopologies: [
        {
          matchLabelExpressions: [
            {
              key: 'topology.kubernetes.io/zone',
              values: ['zone-a', 'zone-b'],
            },
          ],
        },
      ],
    } as StorageClass

    expect(getStorageClassParameters(storageClass)).toEqual([
      'type=ssd',
      'encrypted=true',
    ])
    expect(getStorageClassTopologyRules(storageClass)).toEqual([
      'topology.kubernetes.io/zone: zone-a, zone-b',
    ])
  })
})
