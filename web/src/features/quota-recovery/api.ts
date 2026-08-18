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

import type { QuotaRecoveryOverview, QuotaRecoveryRequest } from './types'

type Envelope<T> = {
  success: boolean
  message?: string
  data: T
}

const requestConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
} as const

function unwrap<T>(response: Envelope<T>): T {
  if (!response.success) {
    throw new Error(response.message || 'Request failed')
  }
  return response.data
}

export async function getSelfQuotaRecovery() {
  const { data } = await api.get<Envelope<QuotaRecoveryOverview>>(
    '/api/user/quota-recovery',
    requestConfig
  )
  return unwrap(data)
}

export async function submitSelfQuotaRecovery(input: {
  idempotency_key: string
  requested_quota: string
  reason: string
}) {
  const { data } = await api.post<Envelope<{ request: QuotaRecoveryRequest }>>(
    '/api/user/quota-recovery',
    input,
    requestConfig
  )
  return unwrap(data)
}
