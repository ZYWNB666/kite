import { afterEach, describe, expect, it, vi } from 'vitest'

import { fetchAllAccessRequests } from './access-request'
import { fetchAuditLogDetail, fetchAuditLogs } from './admin'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  })
}

describe('paginated admin APIs', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sends audit page and operator name without requesting YAML details', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        data: [],
        total: 0,
        page: 2,
        size: 10,
      })
    )
    vi.stubGlobal('fetch', fetchMock)

    await fetchAuditLogs(2, 10, 'Alice')

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, options] = fetchMock.mock.calls[0]
    expect(url).toBe(
      '/api/v1/admin/audit-logs?page=2&size=10&operatorName=Alice'
    )
    expect(options).toMatchObject({ method: 'GET' })
    expect(String(url)).not.toContain('operatorId')
  })

  it('loads YAML from the detail endpoint only when requested', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        id: 42,
        resourceYaml: 'current',
        previousYaml: 'previous',
      })
    )
    vi.stubGlobal('fetch', fetchMock)

    await fetchAuditLogDetail(42)

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/admin/audit-logs/42')
  })

  it('sends the selected temp-permissions page and size', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        requests: [],
        total: 0,
        page: 3,
        size: 50,
      })
    )
    vi.stubGlobal('fetch', fetchMock)

    await fetchAllAccessRequests(3, 50)

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0][0]).toBe(
      '/api/v1/admin/access-requests/?page=3&size=50'
    )
  })
})
