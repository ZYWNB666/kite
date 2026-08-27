export function setClassifiedSearchParam(
  searchParams: URLSearchParams,
  key: string,
  value: string
) {
  if (value) {
    searchParams.set(key, value)
  } else {
    searchParams.delete(key)
  }
  return searchParams
}
