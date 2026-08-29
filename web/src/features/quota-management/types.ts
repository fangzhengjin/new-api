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
export type CycleStatus = 'scheduled' | 'active' | 'settling' | 'closed'
type BalancePolicy = 'reset' | 'carry'
export type ConcentrationMultiplier = 15_000 | 20_000 | 30_000
export type PlanStatus = 'draft' | 'executed' | 'cancelled'
type GeneratablePlanType = 'initialization' | 'adjustment'
export type PlanType = GeneratablePlanType | 'settlement'
export type AdjustmentAction =
  | 'initialize'
  | 'increase'
  | 'decrease'
  | 'grant'
  | 'reclaim'
  | 'temporary_grant'
  | 'settlement'
export type NotificationStatus = 'pending' | 'sent' | 'failed' | 'skipped'

export type QuotaCycle = {
  id: number
  cycle_start_at: number
  cycle_end_at: number
  budget_quota: string
  opening_allocated_quota: string | null
  allocated_quota: string | null
  remaining_quota: string | null
  allocation_error: string | null
  initial_grant_quota: string
  temporary_quota_reserve: string
  temporary_quota_auto_approval_enabled: boolean
  temporary_quota_auto_approval_single_quota: string
  temporary_quota_auto_approval_threshold_quota: string
  temporary_quota_auto_approval_max_count: number
  temporary_quota_auto_approval_max_quota: string
  concentration_multiplier_basis_points: ConcentrationMultiplier
  balance_policy: BalancePolicy
  carry_over_counts_toward_budget: boolean
  status: CycleStatus
  created_at: number
  created_by: string
  updated_at: number
  updated_by: string
}

export type QuotaPlan = {
  id: number
  cycle_id: number
  name: string
  purpose: string
  plan_type: PlanType
  snapshot_at: number
  next_adjustment_at: number | null
  planned_delta_quota: string
  allocation_after_quota: string | null
  status: PlanStatus
  created_at: number
  executed_at: number | null
  cancelled_at: number | null
  cancel_reason: string
}

export type QuotaItem = {
  id: number
  user_id: number
  username: string
  display_name: string
  action: AdjustmentAction
  snapshot_balance_quota: string
  adjustment_quota: string
  basis_text: string
  actual_before_quota: string | null
  actual_after_quota: string | null
  log_status: NotificationStatus
  email_status: NotificationStatus
}

type QuotaCategory = { count: number; total: string }

export type PlanSummary = {
  increase: QuotaCategory
  decrease: QuotaCategory
  affected: number
  budget: string
  occupied_after: string
  cycle_pool_issued_quota: string
  reclaimed_used_for_increases: string
  reclaimed_used_to_cover_overage: string
  reclaimed_unused: string
  pool_remaining: string
}

export type CycleDefaults = { start_at: number }
export type InitialGrantRecommendation = {
  quota: string
  reference_start: number
  reference_end: number
  user_count: number
  average_spend: string
  buffered: string
  stability_floor: string
}

export type CycleListData = {
  cycles: QuotaCycle[]
  defaults: CycleDefaults
  recommendation: InitialGrantRecommendation | null
}

type PlanDefaults = {
  plan_type: GeneratablePlanType
  next_adjustment_at: number
}

export type PlanOptions = {
  cycle: QuotaCycle | null
  initialization_required: boolean
  defaults?: PlanDefaults
}

export type CycleDetailData = {
  cycle: QuotaCycle
  plans: QuotaPlan[]
  recommendation: InitialGrantRecommendation | null
}
export type PlanDetailData = {
  plan: QuotaPlan
  cycle: QuotaCycle
  items: QuotaItem[]
  summary: PlanSummary
  confirmation_phrase: string
}

export type CycleWrite = {
  cycle_start_at: number
  cycle_end_at: number
  budget_quota: string
  initial_grant_quota: string
  temporary_quota_reserve: string
  temporary_quota_auto_approval_enabled: boolean
  temporary_quota_auto_approval_single_quota: string
  temporary_quota_auto_approval_threshold_quota: string
  temporary_quota_auto_approval_max_count: number
  temporary_quota_auto_approval_max_quota: string
  concentration_multiplier_basis_points: ConcentrationMultiplier
  balance_policy: BalancePolicy
  carry_over_counts_toward_budget: boolean
}

export type CycleUpdate = {
  budget_quota: string
  initial_grant_quota?: string
  temporary_quota_reserve?: string
  temporary_quota_auto_approval_enabled?: boolean
  temporary_quota_auto_approval_single_quota?: string
  temporary_quota_auto_approval_threshold_quota?: string
  temporary_quota_auto_approval_max_count?: number
  temporary_quota_auto_approval_max_quota?: string
}

export type PlanWrite = {
  cycle_id: number
  name: string
  purpose: string
  plan_type: GeneratablePlanType
  next_adjustment_at: number
}

export type QuotaOverview = {
  updated_at: number
  allocation_ready: boolean
  previous_cycle?: QuotaCycle
  current_cycle?: QuotaCycle
  next_cycle?: QuotaCycle
  current_plan?: QuotaPlan
  current_plan_summary?: PlanSummary
  pending_temporary_quota_count: number
  pending_allocation_count: number
  pending_reclaim_count: number
}

export type QuotaActivity = {
  id: number
  action: string
  params: Record<string, unknown>
  operator_id: number
  operator: string
  operator_display_name: string
  target?: {
    id: number
    username?: string
    display_name?: string
  }
  created_at: number
}

export type PlanExecuteResult = {
  issued_quota: string
  reclaimed_quota: string
  allocated_quota: string
  remaining_quota: string
}

export type NotificationRetryResult = {
  logs_sent: number
  emails_sent: number
}
