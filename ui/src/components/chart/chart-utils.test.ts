import { describe, expect, it } from 'vitest'

import { UsageDataPoint } from '@/types/api'

import { mergeDualSeries } from './chart-utils'

function point(timestamp: string, value: number): UsageDataPoint {
  return { timestamp, value }
}

describe('mergeDualSeries', () => {
  it('merges samples whose query timestamps differ within one sample slot', () => {
    const incoming = [
      point('2026-07-16T00:00:00.000Z', 10),
      point('2026-07-16T00:00:15.000Z', 20),
    ]
    const outgoing = [
      point('2026-07-16T00:00:02.000Z', 30),
      point('2026-07-16T00:00:17.000Z', 40),
    ]

    expect(
      mergeDualSeries(incoming, outgoing, 'networkIn', 'networkOut')
    ).toMatchObject([
      { networkIn: 10, networkOut: 30 },
      { networkIn: 20, networkOut: 40 },
    ])
  })

  it('uses a small fallback tolerance for single-point series', () => {
    const result = mergeDualSeries(
      [point('2026-07-16T00:00:00.000Z', 10)],
      [point('2026-07-16T00:00:00.250Z', 20)],
      'diskRead',
      'diskWrite'
    )

    expect(result).toHaveLength(1)
    expect(result[0]).toMatchObject({ diskRead: 10, diskWrite: 20 })
  })

  it('keeps genuinely missing samples separate and sorted', () => {
    const result = mergeDualSeries(
      [
        point('2026-07-16T00:00:00.000Z', 10),
        point('2026-07-16T00:00:15.000Z', 20),
      ],
      [point('2026-07-16T00:00:45.000Z', 30)],
      'diskRead',
      'diskWrite'
    )

    expect(result).toHaveLength(3)
    expect(result.map(({ time }) => time)).toEqual(
      [...result.map(({ time }) => time)].sort((a, b) => a - b)
    )
  })
})
