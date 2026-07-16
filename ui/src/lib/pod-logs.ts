const kubernetesTimestampPattern = /^(\S+)\s(.*)$/s

export interface ParsedPodLogLine {
  raw: string
  timestamp?: string
  message: string
}

export interface PresentedPodLogLine {
  timestamp?: string
  resourceName?: string
  message: string
}

const aggregateResourcePattern = /^\[([^\]]+)\]:\s?(.*)$/s

export function parsePodLogLine(raw: string): ParsedPodLogLine {
  const match = kubernetesTimestampPattern.exec(raw)
  if (!match || Number.isNaN(Date.parse(match[1]))) {
    return { raw, message: raw }
  }
  return {
    raw,
    timestamp: match[1],
    message: match[2],
  }
}

export function presentPodLogLine(
  raw: string,
  selectedPodName?: string
): PresentedPodLogLine {
  const parsed = parsePodLogLine(raw)
  if (selectedPodName === '_all') {
    const match = aggregateResourcePattern.exec(parsed.message)
    if (match) {
      return {
        timestamp: parsed.timestamp,
        resourceName: match[1],
        message: match[2],
      }
    }
  }

  return {
    timestamp: parsed.timestamp,
    resourceName: selectedPodName,
    message: parsed.message,
  }
}

export function getLastPodLogTimestamp(lines: string[]): string | undefined {
  for (let index = lines.length - 1; index >= 0; index -= 1) {
    const timestamp = parsePodLogLine(lines[index]).timestamp
    if (timestamp) return timestamp
  }
  return undefined
}

export function getNextPodLogTailLines(
  currentTailLines: number,
  currentLineCount: number,
  batchSize: number,
  maxLines: number
): number {
  return Math.min(
    Math.max(currentTailLines + batchSize, currentLineCount + batchSize),
    maxLines
  )
}

function timestampNanoseconds(timestamp: string): string {
  const fraction = timestamp.match(/\.(\d+)(?:Z|[+-]\d{2}:\d{2})$/)?.[1] || ''
  return fraction.padEnd(9, '0').slice(0, 9)
}

function comparePodLogLines(left: string, right: string): number {
  const leftTimestamp = parsePodLogLine(left).timestamp
  const rightTimestamp = parsePodLogLine(right).timestamp
  if (!leftTimestamp || !rightTimestamp) return 0

  const millisecondDifference =
    Date.parse(leftTimestamp) - Date.parse(rightTimestamp)
  if (millisecondDifference !== 0) return millisecondDifference
  return timestampNanoseconds(leftTimestamp).localeCompare(
    timestampNanoseconds(rightTimestamp)
  )
}

// Kubernetes sinceTime is inclusive. Drop the overlap ending at the last line
// already held by the client, then append only genuinely new lines.
export function mergeIncrementalPodLogs(
  current: string[],
  incoming: string[],
  maxLines: number
): string[] {
  if (incoming.length === 0) return current
  if (current.length === 0) return incoming.slice(-maxLines)

  const lastCurrentLine = current[current.length - 1]
  const overlapIndex = incoming.lastIndexOf(lastCurrentLine)
  const additions =
    overlapIndex >= 0 ? incoming.slice(overlapIndex + 1) : incoming

  if (additions.length === 0) return current
  if (additions.length >= maxLines) return additions.slice(-maxLines)
  return [...current.slice(-(maxLines - additions.length)), ...additions]
}

// Snapshot requests can race live polling and can therefore be either a
// superset or a subset of the current buffer. Merge the two chronological
// sequences instead of replacing the current buffer with the snapshot.
export function mergePodLogSnapshot(
  snapshot: string[],
  current: string[],
  maxLines: number
): string[] {
  if (current.length === 0) return snapshot.slice(-maxLines)
  if (snapshot.length === 0) return current.slice(-maxLines)

  const merged: string[] = []
  let snapshotIndex = 0
  let currentIndex = 0

  while (snapshotIndex < snapshot.length && currentIndex < current.length) {
    const snapshotLine = snapshot[snapshotIndex]
    const currentLine = current[currentIndex]
    if (snapshotLine === currentLine) {
      merged.push(snapshotLine)
      snapshotIndex += 1
      currentIndex += 1
      continue
    }

    const order = comparePodLogLines(snapshotLine, currentLine)
    const snapshotLineInCurrent =
      order === 0 ? current.indexOf(snapshotLine, currentIndex + 1) : -1
    const currentLineInSnapshot =
      order === 0 ? snapshot.indexOf(currentLine, snapshotIndex + 1) : -1

    if (
      order < 0 ||
      (order === 0 && (snapshotLineInCurrent < 0 || currentLineInSnapshot >= 0))
    ) {
      merged.push(snapshotLine)
      snapshotIndex += 1
    } else {
      merged.push(currentLine)
      currentIndex += 1
    }
  }

  merged.push(...snapshot.slice(snapshotIndex), ...current.slice(currentIndex))
  return merged.slice(-maxLines)
}

export function buildPodLogMatcher(
  query: string,
  useRegex: boolean,
  caseSensitive: boolean
): RegExp | undefined {
  if (!query) return undefined
  const source = useRegex ? query : query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return new RegExp(source, caseSensitive ? 'g' : 'gi')
}
