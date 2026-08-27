import { ColumnFiltersState } from '@tanstack/react-table'

export const RESOURCE_SEARCH_QUERY_KEY = 'q'
export const RESOURCE_SEARCH_MODE_KEY = 'searchMode'
export const RESOURCE_SEARCH_MODE_REGEX = 'regex'
export const RESOURCE_FILTER_PREFIX = 'filter.'

export function hasResourceFilters(searchParams: URLSearchParams) {
  return Array.from(searchParams.keys()).some((key) =>
    key.startsWith(RESOURCE_FILTER_PREFIX)
  )
}

export function readResourceFilters(
  searchParams: URLSearchParams,
  allowedColumnIds: string[]
): ColumnFiltersState {
  const filters: ColumnFiltersState = []

  for (const columnId of allowedColumnIds) {
    const values = [
      ...new Set(
        searchParams
          .getAll(`${RESOURCE_FILTER_PREFIX}${columnId}`)
          .filter(Boolean)
      ),
    ]
    if (values.length > 0) {
      filters.push({ id: columnId, value: values })
    }
  }

  return filters
}

export function setResourceQueryInSearchParams(
  searchParams: URLSearchParams,
  query: string
) {
  if (query) {
    searchParams.set(RESOURCE_SEARCH_QUERY_KEY, query)
  } else {
    searchParams.delete(RESOURCE_SEARCH_QUERY_KEY)
  }
  return searchParams
}

export function setResourceSearchModeInSearchParams(
  searchParams: URLSearchParams,
  useRegex: boolean
) {
  if (useRegex) {
    searchParams.set(RESOURCE_SEARCH_MODE_KEY, RESOURCE_SEARCH_MODE_REGEX)
  } else {
    searchParams.delete(RESOURCE_SEARCH_MODE_KEY)
  }
  return searchParams
}

export function setResourceFiltersInSearchParams(
  searchParams: URLSearchParams,
  filters: ColumnFiltersState,
  allowedColumnIds: string[]
) {
  Array.from(searchParams.keys()).forEach((key) => {
    if (key.startsWith(RESOURCE_FILTER_PREFIX)) {
      searchParams.delete(key)
    }
  })

  const allowed = new Set(allowedColumnIds)
  filters.forEach((filter) => {
    if (!allowed.has(filter.id) || !Array.isArray(filter.value)) return
    const values = [
      ...new Set(filter.value.filter((value) => typeof value === 'string')),
    ]
    values.filter(Boolean).forEach((value) => {
      searchParams.append(`${RESOURCE_FILTER_PREFIX}${filter.id}`, value)
    })
  })
  return searchParams
}

export function areResourceFiltersEqual(
  left: ColumnFiltersState,
  right: ColumnFiltersState
) {
  if (left.length !== right.length) return false
  return left.every((filter, index) => {
    const other = right[index]
    if (filter.id !== other?.id) return false
    const leftValue = filter.value
    const rightValue = other.value
    if (!Array.isArray(leftValue) || !Array.isArray(rightValue)) {
      return leftValue === rightValue
    }
    return (
      leftValue.length === rightValue.length &&
      leftValue.every((value, valueIndex) => value === rightValue[valueIndex])
    )
  })
}
