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
export type CycleStatus = 'scheduled' | 'active' | 'closed'
export type BalancePolicy = 'reset' | 'carry'
export type ConcentrationMultiplier = 15_000 | 20_000 | 30_000
export type PlanStatus = 'draft' | 'executed' | 'cancelled'
export type GeneratablePlanType = 'initialization' | 'adjustment'
export type PlanType = GeneratablePlanType | 'settlement'
export type AdjustmentAction =
  | 'initialize'
  | 'increase'
  | 'decrease'
  | 'grant'
  | 'reclaim'
  | 'restore'
export type NotificationStatus = 'pending' | 'sent' | 'failed' | 'skipped'

export type QuotaCycle = {
  id: number
  cycle_start_at: number
  cycle_end_at: number
  budget_quota: string
  initial_grant_quota: string
  recovery_reserve_quota: string
  auto_recovery_enabled: boolean
  auto_recovery_single_quota: string
  auto_recovery_threshold_quota: string
  auto_recovery_max_count: number
  auto_recovery_max_quota: string
  concentration_multiplier_basis_points: ConcentrationMultiplier | 0
  allocation_algorithm_version: string
  legacy_rollback_allowed: boolean
  balance_policy: BalancePolicy
  status: CycleStatus
  settlement_plan_id: number | null
  settled_at: number | null
  restored_at: number | null
  restored_by: string
  created_at: number
  created_by: string
  updated_at: number
  updated_by: string
}

export type QuotaPlan = {
  id: number
  cycle_id: number
  plan_type: PlanType
  stage_percent: number
  snapshot_at: number
  next_adjustment_at: number | null
  algorithm_version: string
  parameters: Record<string, unknown>
  budget_quota_snapshot: string
  total_spend_quota: string
  managed_balance_quota: string
  planned_delta_quota: string
  status: PlanStatus
  created_at: number
  created_by: string
  executed_at: number | null
  executed_by: string
  cancelled_at: number | null
  cancelled_by: string
  cancel_reason: string
}

export type QuotaItem = {
  id: number
  plan_id: number
  user_id: number
  username: string
  display_name: string
  email: string
  action: AdjustmentAction
  snapshot_balance_quota: string
  adjustment_quota: string
  retained_quota: string
  calculation_data: Record<string, unknown>
  basis_text: string
  actual_before_quota: string | null
  actual_after_quota: string | null
  log_content: string
  log_status: NotificationStatus
  log_sent_at: number | null
  log_error: string
  email_status: NotificationStatus
  email_sent_at: number | null
  email_error: string
}

export type QuotaCategory = { count: number; total: string }

export type PlanSummary = {
  base_increase: QuotaCategory
  supplement: QuotaCategory
  bonus: QuotaCategory
  weighted_pool: QuotaCategory
  increase: QuotaCategory
  ordinary_decrease: QuotaCategory
  final_low_decrease: QuotaCategory
  disabled_decrease: QuotaCategory
  decrease: QuotaCategory
  affected: number
  budget: string
  stage_cap: string
  total_spend: string
  managed_balance: string
  occupied_before: string
  occupied_after: string
  balance_after: string
  available_before_reclaim: string
  stage_source_total: string
  reclaimed_used_for_increases: string
  reclaimed_used_to_cover_overage: string
  reclaimed_unused: string
  stage_original_unused: string
  stage_remaining: string
  pool_remaining: string
  future_reserved: string
  recovery_reserve: string
  final_stage: boolean
}

export type CycleDefaults = { start_at: number; end_at: number }
export type InitialGrantRecommendation = {
  quota: string
  reference_start: number
  reference_end: number
  user_count: number
  total_spend: string
  average_spend: string
  buffered: string
  stability_floor: string
}

export type CycleListData = {
  cycles: QuotaCycle[]
  defaults: CycleDefaults
  recommendation: InitialGrantRecommendation | null
}

export type SchedulePoint = { time: number; percent: number; label: string }
export type QuotaSchedule = {
  current: SchedulePoint
  next: SchedulePoint
  events: SchedulePoint[]
  note: string
}

export type PlanDefaults = {
  plan_type: GeneratablePlanType
  stage_percent: number
  next_adjustment_at: number
  basis_mode: 'actual' | 'week'
  early_reclaim: boolean
  reclaim_cap_percent: number
  usage_bonus_percent: number
}

export type PlanOptions = {
  cycle: QuotaCycle | null
  schedule: QuotaSchedule | null
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
  recovery_reserve_quota: string
  auto_recovery_enabled: boolean
  auto_recovery_single_quota: string
  auto_recovery_threshold_quota: string
  auto_recovery_max_count: number
  auto_recovery_max_quota: string
  concentration_multiplier_basis_points: ConcentrationMultiplier
  balance_policy: BalancePolicy
}

export type CycleUpdate = {
  budget_quota: string
  initial_grant_quota?: string
  recovery_reserve_quota?: string
  auto_recovery_enabled?: boolean
  auto_recovery_single_quota?: string
  auto_recovery_threshold_quota?: string
  auto_recovery_max_count?: number
  auto_recovery_max_quota?: string
  concentration_multiplier_basis_points?: ConcentrationMultiplier
}

export type PlanWrite = {
  cycle_id: number
  plan_type: GeneratablePlanType
  stage_percent: number
  next_adjustment_at: number
  basis_mode: 'actual' | 'week'
  early_reclaim: boolean
  reclaim_cap_percent: number
  usage_bonus_percent: number
  thorough_release: boolean
}

export type FairnessMetrics = {
  population: number
  minimum_coverage_basis_points: number
  p10_coverage_basis_points: number
  p50_coverage_basis_points: number
  p90_coverage_basis_points: number
  minimum_safety_coverage_basis_points: number
  safety_unmet: number
  new_user_count: number
  new_user_coverage_basis_points: number | null
  reclaimed_quota: string
  recovery_reserve_quota: string
  occupied_after_quota: string
}

export type FairnessShadowItem = {
  user_id: number
  username: string
  current_balance_quota: string
  safety_target_quota: string
  demand_target_quota: string
  target_quota: string
  current_adjustment_quota: string
  candidate_adjustment_quota: string
  current_after_quota: string
  candidate_after_quota: string
  current_coverage_basis_points: number
  candidate_coverage_basis_points: number
}

export type ConcentrationShadowItem = {
  user_id: number
  username: string
  period_spend_quota: string
  current_balance_quota: string
  current_position_quota: string
  spend_share_basis_points: number
  safety_target_quota: string
  raw_target_quota: string
  effective_target_quota: string
  position_ceiling_quota: string
  adjustment_quota: string
  after_balance_quota: string
  after_position_quota: string
  capped_quota: string
  raw_coverage_basis_points: number
  effective_coverage_basis_points: number
}

export type ConcentrationShadowVariant = {
  multiplier_basis_points: number
  population: number
  position_ceiling_quota: string
  maximum_position_share_basis_points: number
  capped_users: number
  capped_quota: string
  minimum_raw_coverage_basis_points: number
  p10_raw_coverage_basis_points: number
  p50_raw_coverage_basis_points: number
  minimum_effective_coverage_basis_points: number
  p10_effective_coverage_basis_points: number
  p50_effective_coverage_basis_points: number
  minimum_safety_coverage_basis_points: number
  safety_unmet: number
  planned_increase_quota: string
  reclaimed_quota: string
  occupied_after_quota: string
  unallocated_stage_quota: string
  items: ConcentrationShadowItem[]
}

export type FairnessShadowComparison = {
  snapshot_at: number
  stage_cap_quota: string
  current_algorithm_version: string
  candidate_algorithm_version: string
  candidate_qualified: boolean
  current: FairnessMetrics
  candidate: FairnessMetrics
  items: FairnessShadowItem[]
  concentration_variants: ConcentrationShadowVariant[]
}

export type NotificationRetryResult = {
  logs_sent: number
  logs_failed: number
  emails_sent: number
  emails_failed: number
  skipped: number
}

export type QuotaAlgorithmStatus = {
  legacy_version: string
  current_version: string
  candidate_version: string
  enable_confirmation_phrase: string
  rollback_confirmation_phrase: string
  required_qualified_cycles: number
  qualified_cycle_ids: number[]
  active_cycle_id: number | null
  recovery_ready: boolean
  draft_count: number
  can_switch: boolean
  rollback_allowed: boolean
  can_record_evidence: boolean
  blockers: string[]
}
