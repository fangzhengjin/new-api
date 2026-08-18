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
export type RecoveryRequestStatus = 'pending' | 'executed' | 'rejected'
export type RecoveryDecision = '' | 'auto' | 'manual'

export type QuotaRecoveryRequest = {
  id: number
  cycle_id: number
  user_id: number
  username: string
  display_name: string
  requested_quota: string
  reason: string
  status: RecoveryRequestStatus
  decision: RecoveryDecision
  approved_quota: string
  plan_id: number | null
  reviewed_by: string
  review_reason: string
  created_at: number
  reviewed_at: number | null
  executed_at: number | null
}

export type QuotaRecoveryOverview = {
  cycle_id: number
  current_quota: string
  auto_recovery_enabled: boolean
  auto_recovery_single_quota: string
  auto_recovery_threshold_quota: string
  auto_recovery_max_count: number
  auto_recovery_max_quota: string
  reserve_available: boolean
  requests: QuotaRecoveryRequest[]
}
