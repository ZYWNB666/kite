import { useEffect } from 'react'

import { withClusterHref, withClusterRequestHref } from '@/lib/current-cluster'
import { useCluster } from '@/hooks/use-cluster'

export function ClusterAwareLinks() {
  const { currentCluster } = useCluster()

  useEffect(() => {
    if (!currentCluster) {
      return
    }

    const updateAnchor = (anchor: HTMLAnchorElement) => {
      const href = anchor.getAttribute('href')
      if (
        !href ||
        href.startsWith('#') ||
        anchor.dataset.clusterUnscoped === 'true'
      ) {
        return
      }

      const targetUrl = new URL(href, window.location.href)
      const clusterHref = targetUrl.pathname.includes('/api/v1/')
        ? withClusterRequestHref(href, currentCluster)
        : withClusterHref(href, currentCluster)
      if (clusterHref !== href) {
        anchor.setAttribute('href', clusterHref)
      }
    }

    const updateLinks = (root: ParentNode) => {
      root.querySelectorAll<HTMLAnchorElement>('a[href]').forEach(updateAnchor)
      if (root instanceof HTMLAnchorElement) {
        updateAnchor(root)
      }
    }

    updateLinks(document)
    const observer = new MutationObserver((mutations) => {
      mutations.forEach((mutation) => {
        if (mutation.type === 'attributes') {
          updateAnchor(mutation.target as HTMLAnchorElement)
          return
        }
        mutation.addedNodes.forEach((node) => {
          if (node instanceof HTMLElement) {
            updateLinks(node)
          }
        })
      })
    })
    observer.observe(document.body, {
      subtree: true,
      childList: true,
      attributes: true,
      attributeFilter: ['href'],
    })

    return () => observer.disconnect()
  }, [currentCluster])

  return null
}
