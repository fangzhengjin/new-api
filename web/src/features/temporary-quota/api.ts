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
  SelfTemporaryQuotaRequest,
  TemporaryQuotaOverview,
  TemporaryQuotaPage,
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

export async function getSelfTemporaryQuota() {
  const { data } = await api.get<Envelope<TemporaryQuotaOverview>>(
    '/api/user/temporary-quota',
    requestConfig
  )
  return unwrap(data)
}

export async function listSelfTemporaryQuotaRequests(params: {
  page: number
  pageSize: number
  status?: string
  keyword?: string
}) {
  const { data } = await api.get<Envelope<TemporaryQuotaPage>>(
    '/api/user/temporary-quota/requests',
    {
      ...requestConfig,
      params: {
        p: params.page,
        page_size: params.pageSize,
        status: params.status,
        keyword: params.keyword,
      },
    }
  )
  return unwrap(data)
}

export async function submitSelfTemporaryQuota(input: {
  idempotency_key: string
  requested_quota: string
  project: string
  reason: string
}) {
  const { data } = await api.post<
    Envelope<{ request: SelfTemporaryQuotaRequest }>
  >('/api/user/temporary-quota', input, requestConfig)
  return unwrap(data)
}

export const temporaryQuotaQueryKeys = {
  all: ['temporary-quota'] as const,
  overview: ['temporary-quota', 'overview'] as const,
  requests: ['temporary-quota', 'requests'] as const,
}
