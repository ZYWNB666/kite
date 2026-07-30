import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiClient } from '../api-client'
import { fetchAPI } from './shared'

// ─── Types ────────────────────────────────────────────────────────────────────

export type AccessRequestStatus =
  | 'pending'
  | 'approved'
  | 'rejected'
  | 'withdrawn'
  | 'expired'

export interface AccessRequest {
  id: number
  createdAt: string
  updatedAt: string
  requesterId: number
  requesterName: string
  cluster: string
  namespace: string
  durationHours: number
  riskLevel: string
  reason: string
  approverUid: string
  approverName: string
  status: AccessRequestStatus
  expiresAt?: string
  messageId?: string
  roleId?: number
  reviewNote?: string
}

export interface AccessRequestPage {
  requests: AccessRequest[]
  total: number
  page: number
  size: number
}

export interface FeishuApprover {
  name: string
  openId: string
}

export interface FeishuSetting {
  id: number
  appId: string
  groupChatId: string
  approvers: FeishuApprover[]
  enabled: boolean
  appSecretSet: boolean
  verificationTokenSet: boolean
}

export interface CreateAccessRequestBody {
  cluster: string
  namespaces: string[]
  durationHours: number
  riskLevel: string
  reason: string
  approverUid: string
  approverName?: string
}

export interface UpdateFeishuSettingBody {
  appId: string
  appSecret?: string
  groupChatId: string
  verificationToken?: string
  approvers: FeishuApprover[]
  enabled: boolean
}

// ─── API Functions ────────────────────────────────────────────────────────────

export const fetchMyAccessRequests = (): Promise<{
  requests: AccessRequest[]
}> => fetchAPI<{ requests: AccessRequest[] }>('/access-requests')

export const createAccessRequest = (
  body: CreateAccessRequestBody
): Promise<AccessRequest> =>
  apiClient.post<AccessRequest>('/access-requests', body)

export const withdrawAccessRequest = (id: number): Promise<AccessRequest> =>
  apiClient.put<AccessRequest>(`/access-requests/${id}/withdraw`, {})

export const remindAccessRequest = (id: number): Promise<{ message: string }> =>
  apiClient.post<{ message: string }>(`/access-requests/${id}/remind`, {})

// Admin
export const fetchAllAccessRequests = (
  page = 1,
  size = 20
): Promise<AccessRequestPage> => {
  const params = new URLSearchParams({
    page: String(page),
    size: String(size),
  })
  return fetchAPI<AccessRequestPage>(`/admin/access-requests/?${params}`)
}

export const revokeAccess = (id: number): Promise<{ message: string }> =>
  apiClient.put<{ message: string }>(`/admin/access-requests/${id}/revoke`, {})

export const approveAccess = (id: number): Promise<AccessRequest> =>
  apiClient.put<AccessRequest>(`/admin/access-requests/${id}/approve`, {})

// Feishu Settings
export const fetchFeishuSetting = (): Promise<FeishuSetting> =>
  fetchAPI<FeishuSetting>('/admin/feishu-setting/')

export const updateFeishuSetting = (
  body: UpdateFeishuSettingBody
): Promise<FeishuSetting> =>
  apiClient.put<FeishuSetting>('/admin/feishu-setting/', body)

export const fetchFeishuApprovers = (): Promise<{
  approvers: FeishuApprover[]
}> => fetchAPI<{ approvers: FeishuApprover[] }>('/feishu-approvers')

// ─── Hooks ────────────────────────────────────────────────────────────────────

export const useMyAccessRequests = () =>
  useQuery({
    queryKey: ['my-access-requests'],
    queryFn: () => fetchMyAccessRequests().then((r) => r.requests),
  })

export const useAllAccessRequests = (page = 1, size = 20) =>
  useQuery({
    queryKey: ['all-access-requests', page, size],
    queryFn: () => fetchAllAccessRequests(page, size),
    refetchInterval: 30_000,
  })

export const useFeishuSetting = () =>
  useQuery({
    queryKey: ['feishu-setting'],
    queryFn: fetchFeishuSetting,
  })

export const useFeishuApprovers = () =>
  useQuery({
    queryKey: ['feishu-approvers'],
    queryFn: () => fetchFeishuApprovers().then((r) => r.approvers ?? []),
  })

export const useCreateAccessRequest = () => {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createAccessRequest,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['my-access-requests'] }),
  })
}

export const useWithdrawAccessRequest = () => {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: withdrawAccessRequest,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['my-access-requests'] }),
  })
}

export const useRemindAccessRequest = () =>
  useMutation({ mutationFn: remindAccessRequest })

export const useRevokeAccess = () => {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: revokeAccess,
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ['all-access-requests'] }),
  })
}

export const useApproveAccess = () => {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: approveAccess,
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ['all-access-requests'] }),
  })
}

export const useUpdateFeishuSetting = () => {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: updateFeishuSetting,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['feishu-setting'] })
      qc.invalidateQueries({ queryKey: ['feishu-approvers'] })
    },
  })
}
