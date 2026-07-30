/* eslint-disable react-refresh/only-export-components */
import React, {
  createContext,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useLocation, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'

import { Cluster } from '@/types/api'
import { useCurrentClusterList } from '@/lib/api'
import {
  clearCurrentCluster,
  getCurrentCluster,
  setCurrentCluster as persistCurrentCluster,
} from '@/lib/current-cluster'

interface ClusterContextType {
  clusters: Cluster[]
  currentCluster: string | null
  setCurrentCluster: (clusterName: string) => void
  isLoading: boolean
  isSwitching?: boolean
  error: Error | null
}

export const ClusterContext = createContext<ClusterContextType | undefined>(
  undefined
)

export const ClusterProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [currentCluster, setCurrentClusterState] = useState<string | null>(
    getCurrentCluster()
  )
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)
  const [isSwitching, setIsSwitching] = useState(false)
  const queryClient = useQueryClient()
  const location = useLocation()
  const navigate = useNavigate()
  const initialUrlCluster = useRef(
    new URLSearchParams(location.search).get('cluster')
  )
  const { refetch: refetchClusters } = useCurrentClusterList({
    enabled: false,
  })

  const replaceUrlCluster = useCallback(
    (clusterName: string) => {
      const searchParams = new URLSearchParams(location.search)
      if (searchParams.get('cluster') === clusterName) {
        return
      }
      searchParams.set('cluster', clusterName)
      navigate(
        {
          pathname: location.pathname,
          search: `?${searchParams.toString()}`,
          hash: location.hash,
        },
        { replace: true }
      )
    },
    [location.hash, location.pathname, location.search, navigate]
  )

  useEffect(() => {
    let cancelled = false

    const bootstrap = async () => {
      setIsLoading(true)
      const result = await refetchClusters()
      if (cancelled) {
        return
      }

      if (!result.data) {
        setClusters([])
        setError(
          result.error instanceof Error
            ? result.error
            : new Error('Failed to load clusters')
        )
        setIsLoading(false)
        return
      }

      setClusters(result.data)
      const availableClusters = result.data.filter((cluster) => !cluster.error)
      if (availableClusters.length === 0) {
        clearCurrentCluster()
        setCurrentClusterState(null)
        setError(new Error('No clusters available'))
        setIsLoading(false)
        return
      }

      const requestedCluster = initialUrlCluster.current
      const defaultCluster = availableClusters.find(
        (cluster) => cluster.isDefault
      )
      const selectedCluster =
        availableClusters.find((cluster) => cluster.name === requestedCluster)
          ?.name ??
        defaultCluster?.name ??
        availableClusters[0].name

      persistCurrentCluster(selectedCluster)
      setCurrentClusterState(selectedCluster)
      setError(null)
      setIsLoading(false)
    }

    void bootstrap()

    return () => {
      cancelled = true
    }
  }, [refetchClusters])

  useEffect(() => {
    if (isLoading || clusters.length === 0) {
      return
    }

    const urlCluster = new URLSearchParams(location.search).get('cluster')
    const validUrlCluster = clusters.some(
      (cluster) => cluster.name === urlCluster && !cluster.error
    )

    if (urlCluster && validUrlCluster) {
      persistCurrentCluster(urlCluster)
      if (urlCluster !== currentCluster) {
        setCurrentClusterState(urlCluster)
      }
      return
    }

    const fallbackCluster =
      clusters.find(
        (cluster) => cluster.name === currentCluster && !cluster.error
      )?.name ??
      clusters.find((cluster) => cluster.isDefault && !cluster.error)?.name ??
      clusters.find((cluster) => !cluster.error)?.name
    if (!fallbackCluster) {
      return
    }
    persistCurrentCluster(fallbackCluster)
    setCurrentClusterState(fallbackCluster)
    replaceUrlCluster(fallbackCluster)
  }, [clusters, currentCluster, isLoading, location.search, replaceUrlCluster])

  const setCurrentCluster = (clusterName: string) => {
    if (
      clusterName === currentCluster ||
      isSwitching ||
      !clusters.some(
        (cluster) => cluster.name === clusterName && !cluster.error
      )
    ) {
      return
    }

    setIsSwitching(true)
    void queryClient.cancelQueries({
      predicate: (query) => query.queryKey[0] === 'cluster',
    })
    persistCurrentCluster(clusterName)
    setCurrentClusterState(clusterName)
    replaceUrlCluster(clusterName)
    toast.success(`Switched to cluster: ${clusterName}`, {
      id: 'cluster-switch',
    })
    setIsSwitching(false)
  }

  const value: ClusterContextType = {
    clusters,
    currentCluster,
    setCurrentCluster,
    isLoading,
    isSwitching,
    error,
  }

  return (
    <ClusterContext.Provider value={value}>{children}</ClusterContext.Provider>
  )
}
