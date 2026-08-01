import { useMemo } from 'react'
import { createColumnHelper } from '@tanstack/react-table'
import { Link, useParams } from 'react-router-dom'

import { CustomResource, ResourceType } from '@/types/api'
import { useCRDSummaries } from '@/lib/api'
import { createSearchFilter, getPrinterColumnValue } from '@/lib/k8s'
import { formatDate } from '@/lib/utils'
import { ErrorMessage } from '@/components/error-message'
import { ResourceTable } from '@/components/resource-table'

const searchQueryFilter = createSearchFilter<CustomResource>(
  (cr) => cr.metadata?.name,
  (cr) => cr.metadata?.namespace,
  (cr) => cr.kind,
  (cr) => cr.apiVersion,
  (cr) => (cr.metadata?.labels ? Object.keys(cr.metadata.labels) : undefined),
  (cr) => (cr.metadata?.labels ? Object.values(cr.metadata.labels) : undefined)
)

export function CRListPage() {
  const { crd } = useParams<{ crd: string }>()
  const {
    data: crds,
    isLoading: isLoadingCRD,
    isError: isCRDError,
    error: crdError,
    refetch: refetchCRDs,
  } = useCRDSummaries()
  const crdData = useMemo(
    () => crds?.find((item) => item.name === crd),
    [crd, crds]
  )

  const columnHelper = createColumnHelper<CustomResource>()
  const columns = useMemo(() => {
    const baseColumns = [
      columnHelper.accessor('metadata.name', {
        header: 'Name',
        cell: ({ row }) => {
          const resource = row.original
          const namespace = resource.metadata?.namespace
          const path = namespace
            ? `/crds/${crd}/${namespace}/${resource.metadata.name}`
            : `/crds/${crd}/${resource.metadata.name}`

          return (
            <div className="font-medium app-link">
              <Link to={path}>{resource.metadata.name}</Link>
            </div>
          )
        },
      }),
    ]
    const additionalColumns = crdData?.versions
      .find((version) => version.served)
      ?.additionalPrinterColumns?.map((printerColumn) => {
        const jsonPath = printerColumn.jsonPath

        return columnHelper.accessor(
          (row) => getPrinterColumnValue(row, jsonPath),
          {
            id: jsonPath || printerColumn.name,
            header: printerColumn.name,
            cell: ({ getValue }) => {
              const type = printerColumn.type
              const value = getValue()
              if (!value) {
                return <span className="text-sm text-muted-foreground">-</span>
              }
              if (type === 'date') {
                return (
                  <span className="text-sm text-muted-foreground">
                    {formatDate(value)}
                  </span>
                )
              }
              return (
                <span className="text-sm text-muted-foreground">{value}</span>
              )
            },
          }
        )
      })
    return [...baseColumns, ...(additionalColumns ?? [])]
  }, [columnHelper, crd, crdData?.versions])

  if (isLoadingCRD) {
    return <div>Loading...</div>
  }

  if (isCRDError) {
    return (
      <ErrorMessage
        resourceName="Custom Resource Definitions"
        error={crdError}
        refetch={refetchCRDs}
      />
    )
  }

  if (!crdData) {
    return <div>Error: Custom Resource Definition {crd} was not found</div>
  }

  return (
    <ResourceTable
      resourceName={crdData.kind || 'Custom Resources'}
      resourceType={crd as ResourceType}
      columns={columns}
      clusterScope={crdData.scope === 'Cluster'}
      searchQueryFilter={searchQueryFilter}
    />
  )
}
