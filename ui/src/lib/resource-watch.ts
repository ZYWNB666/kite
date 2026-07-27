export interface WatchableResource {
  metadata?: {
    uid?: string
    namespace?: string
    name?: string
    creationTimestamp?: string
  }
}

export interface ResourceWatchDelta<T> {
  type: 'added' | 'modified' | 'deleted'
  object: T
}

const pollingOnlyResources = new Set(['nodes', 'podmetrics', 'nodemetrics'])

export function supportsResourceWatch(resource: string): boolean {
  return !pollingOnlyResources.has(resource.toLowerCase())
}

export function getResourceWatchKey(resource: WatchableResource): string {
  const metadata = resource.metadata
  return metadata?.uid || `${metadata?.namespace || ''}/${metadata?.name || ''}`
}

export function sortWatchedResources<T extends WatchableResource>(
  resources: T[]
): T[] {
  return resources.sort((first, second) => {
    const firstCreated = Date.parse(first.metadata?.creationTimestamp || '')
    const secondCreated = Date.parse(second.metadata?.creationTimestamp || '')
    const firstTime = Number.isNaN(firstCreated) ? 0 : firstCreated
    const secondTime = Number.isNaN(secondCreated) ? 0 : secondCreated
    if (firstTime !== secondTime) return secondTime - firstTime
    return (first.metadata?.name || '').localeCompare(
      second.metadata?.name || ''
    )
  })
}

export function applyResourceWatchDeltas<T extends WatchableResource>(
  current: readonly T[],
  deltas: readonly ResourceWatchDelta<T>[]
): T[] {
  const resources = new Map(
    current.map((resource) => [getResourceWatchKey(resource), resource])
  )

  for (const delta of deltas) {
    const key = getResourceWatchKey(delta.object)
    if (delta.type === 'deleted') {
      resources.delete(key)
    } else {
      resources.set(key, delta.object)
    }
  }

  return sortWatchedResources(Array.from(resources.values()))
}
