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
export type TemporaryQuotaRequestStatus = 'pending' | 'executed' | 'rejected'

export type TemporaryQuotaRequest = {
  id: number
  user_id: number
  username: string
  display_name: string
  requested_quota: string
  project: string
  reason: string
  status: TemporaryQuotaRequestStatus
  review_reason: string
  created_at: number
}

export type TemporaryQuotaOverview = {
  current_quota: string
  can_request: boolean
  unavailable_reason: string
  projects: string[]
}

export type SelfTemporaryQuotaRequest = {
  id: number
  requested_quota: string
  project: string
  reason: string
  status: TemporaryQuotaRequestStatus
  approved_quota: string
  review_reason: string
  created_at: number
  reviewed_at: number | null
  executed_at: number | null
}

export type TemporaryQuotaPage = {
  page: number
  page_size: number
  total: number
  items: SelfTemporaryQuotaRequest[]
}
