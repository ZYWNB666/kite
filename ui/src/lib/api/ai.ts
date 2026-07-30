import { useQuery } from '@tanstack/react-query'

import { getClusterQueryKey } from '../current-cluster'
import { fetchAPI } from './shared'

export const getAIStatus = async (): Promise<{ enabled: boolean }> => {
  return fetchAPI<{ enabled: boolean }>('/ai/status')
}

export const useAIStatus = () => {
  return useQuery({
    queryKey: getClusterQueryKey('ai-status'),
    queryFn: getAIStatus,
    retry: false,
  })
}
