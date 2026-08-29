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
import { getApiKeys } from '@/features/keys/api'
import { API_KEY_STATUS } from '@/features/keys/constants'
import type { ApiKey } from '@/features/keys/types'
import { api } from '@/lib/api'

type Envelope<T> = {
  success: boolean
  message?: string
  code?: string
  data?: T
}

const requestConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
} as const

/** Creates a chat error that the shared server-error handler may display. */
export function createChatError(
  message: string | undefined,
  fallback: string,
  code?: string
) {
  return Object.assign(new Error(message || fallback), { code })
}

export async function loadAllApiKeys() {
  const pageSize = 100
  const first = await getApiKeys({ p: 1, size: pageSize })
  if (!first.success) {
    throw createChatError(first.message, 'Failed to load API keys')
  }

  const items = [...(first.data?.items ?? [])]
  const pageCount = Math.ceil((first.data?.total ?? items.length) / pageSize)
  const remaining = await Promise.all(
    Array.from({ length: Math.max(0, pageCount - 1) }, (_, index) =>
      getApiKeys({ p: index + 2, size: pageSize })
    )
  )
  for (const result of remaining) {
    if (!result.success) {
      throw createChatError(result.message, 'Failed to load API keys')
    }
    items.push(...(result.data?.items ?? []))
  }
  return items
}

export function isApiKeyUsable(apiKey: ApiKey, now = Date.now() / 1000) {
  return (
    apiKey.status === API_KEY_STATUS.ENABLED &&
    (apiKey.expired_time === -1 || apiKey.expired_time > now) &&
    (apiKey.unlimited_quota || apiKey.remain_quota > 0)
  )
}

export async function launchChatPreset(presetName: string, tokenId: number) {
  const { data } = await api.post<Envelope<{ launch_url: string }>>(
    '/api/integrations/launch',
    { preset_name: presetName, token_id: tokenId },
    requestConfig
  )
  if (!data.success || !data.data?.launch_url) {
    throw createChatError(data.message, 'Unable to open integration', data.code)
  }
  return data.data.launch_url
}
