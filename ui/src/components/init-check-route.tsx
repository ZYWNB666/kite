import { ReactNode } from 'react'
import { Navigate } from 'react-router-dom'

import { useInitCheck } from '@/lib/api'

import { ErrorMessage } from './error-message'

interface InitCheckRouteProps {
  children: ReactNode
}

export function InitCheckRoute({ children }: InitCheckRouteProps) {
  const { data: initCheck, error, isError, isLoading, refetch } = useInitCheck()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-32 w-32 border-b-2 border-primary"></div>
      </div>
    )
  }

  // A database or network failure must not be interpreted as an uninitialized
  // application. Keep an already verified application rendered during a
  // background refetch failure; otherwise show a retryable error state.
  if (isError) {
    if (initCheck?.initialized) {
      return <>{children}</>
    }

    return (
      <div className="flex min-h-screen items-center justify-center">
        <ErrorMessage
          resourceName="application status"
          error={error}
          refetch={refetch}
        />
      </div>
    )
  }

  // Check if app is initialized first
  if (!initCheck?.initialized) {
    return <Navigate to="/setup" replace />
  }

  return <>{children}</>
}
