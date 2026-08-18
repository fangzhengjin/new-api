/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { api } from '@/lib/api'

import type {
  CycleDetailData,
  CycleListData,
  CycleUpdate,
  CycleWrite,
  NotificationRetryResult,
  PlanDetailData,
  PlanOptions,
  PlanWrite,
  QuotaCycle,
  QuotaPlan,
} from './types'

type Envelope<T> = {
  success: boolean
  message?: string
  code?: string
  data: T
}
const requestConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
} as const

function unwrap<T>(response: Envelope<T>): T {
  if (!response.success) {
    throw Object.assign(new Error(response.message || 'Request failed'), {
      code: response.code,
    })
  }
  return response.data
}

export async function listCycles() {
  const { data } = await api.get<Envelope<CycleListData>>(
    '/api/quota-management/cycles',
    requestConfig
  )
  return unwrap(data)
}

export async function createCycle(input: CycleWrite) {
  const { data } = await api.post<Envelope<QuotaCycle>>(
    '/api/quota-management/cycles',
    input,
    requestConfig
  )
  return unwrap(data)
}

export async function getCycle(id: number) {
  const { data } = await api.get<Envelope<CycleDetailData>>(
    `/api/quota-management/cycles/${id}`,
    requestConfig
  )
  return unwrap(data)
}

export async function updateCycle(id: number, input: CycleUpdate) {
  const { data } = await api.patch<Envelope<null>>(
    `/api/quota-management/cycles/${id}`,
    input,
    requestConfig
  )
  return unwrap(data)
}

export async function closeCycle(id: number) {
  const { data } = await api.post<Envelope<null>>(
    `/api/quota-management/cycles/${id}/close`,
    undefined,
    requestConfig
  )
  return unwrap(data)
}

export async function restoreCycle(id: number) {
  const { data } = await api.post<Envelope<unknown>>(
    `/api/quota-management/cycles/${id}/restore`,
    { confirmation: `确认恢复周期 #${id} 清零余额` },
    requestConfig
  )
  return unwrap(data)
}

export async function listPlans(cycleId?: number) {
  const { data } = await api.get<Envelope<QuotaPlan[]>>(
    '/api/quota-management/plans',
    {
      ...requestConfig,
      params: cycleId ? { cycle_id: cycleId } : undefined,
    }
  )
  return unwrap(data)
}

export async function getPlanOptions() {
  const { data } = await api.get<Envelope<PlanOptions>>(
    '/api/quota-management/plans/options',
    requestConfig
  )
  return unwrap(data)
}

export async function generatePlan(input: PlanWrite) {
  const { data } = await api.post<
    Envelope<{ plan_id: number; plan: QuotaPlan }>
  >('/api/quota-management/plans', input, requestConfig)
  return unwrap(data)
}

export async function getPlan(id: number) {
  const { data } = await api.get<Envelope<PlanDetailData>>(
    `/api/quota-management/plans/${id}`,
    requestConfig
  )
  return unwrap(data)
}

export async function executePlan(id: number, confirmation: string) {
  const { data } = await api.post<Envelope<unknown>>(
    `/api/quota-management/plans/${id}/execute`,
    { confirmation },
    requestConfig
  )
  return unwrap(data)
}

export async function cancelPlan(id: number, reason: string) {
  const { data } = await api.post<Envelope<null>>(
    `/api/quota-management/plans/${id}/cancel`,
    { reason },
    requestConfig
  )
  return unwrap(data)
}

export async function deletePlan(id: number) {
  const { data } = await api.delete<Envelope<null>>(
    `/api/quota-management/plans/${id}`,
    requestConfig
  )
  return unwrap(data)
}

export async function regeneratePlan(id: number) {
  const { data } = await api.post<Envelope<{ plan_id: number }>>(
    `/api/quota-management/plans/${id}/regenerate`,
    undefined,
    requestConfig
  )
  return unwrap(data)
}

export async function retryNotifications(id: number) {
  const { data } = await api.post<Envelope<NotificationRetryResult>>(
    `/api/quota-management/plans/${id}/notifications/retry`,
    undefined,
    requestConfig
  )
  return unwrap(data)
}

export async function manualQuotaAdjust(input: {
  user_id: number
  target_quota: string
  reason: string
}) {
  const { data } = await api.post<Envelope<unknown>>(
    '/api/quota-management/manual-adjust',
    input,
    requestConfig
  )
  return unwrap(data)
}
