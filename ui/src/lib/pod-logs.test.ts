import { describe, expect, it } from 'vitest'

import {
  buildPodLogMatcher,
  getLastPodLogTimestamp,
  getNextPodLogTailLines,
  mergeIncrementalPodLogs,
  mergePodLogSnapshot,
  parsePodLogLine,
  presentPodLogLine,
} from './pod-logs'

describe('pod log helpers', () => {
  it('splits the Kubernetes timestamp from the displayed message', () => {
    expect(
      parsePodLogLine('2026-07-15T10:20:30.123456789Z hello world')
    ).toEqual({
      raw: '2026-07-15T10:20:30.123456789Z hello world',
      timestamp: '2026-07-15T10:20:30.123456789Z',
      message: 'hello world',
    })
    expect(parsePodLogLine('plain message')).toEqual({
      raw: 'plain message',
      message: 'plain message',
    })
  })

  it('finds the newest usable timestamp', () => {
    expect(
      getLastPodLogTimestamp([
        '2026-07-15T10:20:30Z first',
        'plain line',
        '2026-07-15T10:20:31Z second',
      ])
    ).toBe('2026-07-15T10:20:31Z')
  })

  it('presents resource names after the timestamp for single and aggregate pods', () => {
    expect(presentPodLogLine('2026-07-15T10:20:30Z hello', 'pod-a')).toEqual({
      timestamp: '2026-07-15T10:20:30Z',
      resourceName: 'pod-a',
      message: 'hello',
    })
    expect(
      presentPodLogLine('2026-07-15T10:20:30Z [pod-b]: aggregate hello', '_all')
    ).toEqual({
      timestamp: '2026-07-15T10:20:30Z',
      resourceName: 'pod-b',
      message: 'aggregate hello',
    })
  })

  it('requests enough tail lines to cover live growth plus older history', () => {
    expect(getNextPodLogTailLines(500, 1_500, 500, 100_000)).toBe(2_000)
    expect(getNextPodLogTailLines(99_800, 100_000, 500, 100_000)).toBe(100_000)
  })

  it('removes the inclusive sinceTime overlap', () => {
    expect(
      mergeIncrementalPodLogs(['one', 'two'], ['two', 'three'], 100)
    ).toEqual(['one', 'two', 'three'])
  })

  it('keeps live lines when a historical snapshot arrives', () => {
    expect(
      mergePodLogSnapshot(['older', 'one', 'two'], ['one', 'two', 'live'], 100)
    ).toEqual(['older', 'one', 'two', 'live'])
  })

  it('does not discard existing history when a newer snapshot is smaller', () => {
    const current = Array.from(
      { length: 1_500 },
      (_, index) =>
        `2026-07-15T10:20:${String(Math.floor(index / 100)).padStart(2, '0')}.${String(index % 100).padStart(3, '0')}Z current-${index + 500}`
    )
    const snapshot = current.slice(500)

    const merged = mergePodLogSnapshot(snapshot, current, 100_000)

    expect(merged).toHaveLength(1_500)
    expect(merged[0]).toBe(current[0])
  })

  it('aligns overlapping entries that share a timestamp', () => {
    const timestamp = '2026-07-15T10:20:30.123456789Z'
    expect(
      mergePodLogSnapshot(
        [`${timestamp} second`, `${timestamp} third`],
        [`${timestamp} first`, `${timestamp} second`, `${timestamp} third`],
        100
      )
    ).toEqual([
      `${timestamp} first`,
      `${timestamp} second`,
      `${timestamp} third`,
    ])
  })

  it('builds plain and regular-expression matchers', () => {
    expect('a.b'.match(buildPodLogMatcher('a.b', false, true)!)).not.toBeNull()
    expect('axb'.match(buildPodLogMatcher('a.b', false, true)!)).toBeNull()
    expect('AXB'.match(buildPodLogMatcher('a.b', true, false)!)).not.toBeNull()
  })
})
