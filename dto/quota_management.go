package dto

// QuotaCycleCreateRequest creates a purchasing cycle from Unix-second boundaries and raw quota strings.
type QuotaCycleCreateRequest struct {
	CycleStartAt               int64  `json:"cycle_start_at" binding:"required"`
	CycleEndAt                 int64  `json:"cycle_end_at" binding:"required"`
	BudgetQuota                string `json:"budget_quota" binding:"required"`
	InitialGrantQuota          string `json:"initial_grant_quota" binding:"required"`
	RecoveryReserveQuota       string `json:"recovery_reserve_quota"`
	AutoRecoveryEnabled        bool   `json:"auto_recovery_enabled"`
	AutoRecoverySingleQuota    string `json:"auto_recovery_single_quota"`
	AutoRecoveryThresholdQuota string `json:"auto_recovery_threshold_quota"`
	AutoRecoveryMaxCount       int    `json:"auto_recovery_max_count"`
	AutoRecoveryMaxQuota       string `json:"auto_recovery_max_quota"`
	ConcentrationMultiplier    int64  `json:"concentration_multiplier_basis_points" binding:"required"`
	BalancePolicy              string `json:"balance_policy" binding:"required"`
}

// QuotaCycleUpdateRequest updates budget and optionally the scheduled initial grant.
type QuotaCycleUpdateRequest struct {
	BudgetQuota                string  `json:"budget_quota" binding:"required"`
	InitialGrantQuota          *string `json:"initial_grant_quota"`
	RecoveryReserveQuota       *string `json:"recovery_reserve_quota"`
	AutoRecoveryEnabled        *bool   `json:"auto_recovery_enabled"`
	AutoRecoverySingleQuota    *string `json:"auto_recovery_single_quota"`
	AutoRecoveryThresholdQuota *string `json:"auto_recovery_threshold_quota"`
	AutoRecoveryMaxCount       *int    `json:"auto_recovery_max_count"`
	AutoRecoveryMaxQuota       *string `json:"auto_recovery_max_quota"`
	ConcentrationMultiplier    *int64  `json:"concentration_multiplier_basis_points"`
}

// QuotaPlanGenerateRequest contains the administrator-reviewed allocation controls.
type QuotaPlanGenerateRequest struct {
	CycleID           int    `json:"cycle_id" binding:"required"`
	PlanType          string `json:"plan_type" binding:"required"`
	StagePercent      int    `json:"stage_percent"`
	NextAdjustmentAt  int64  `json:"next_adjustment_at" binding:"required"`
	BasisMode         string `json:"basis_mode" binding:"required"`
	EarlyReclaim      bool   `json:"early_reclaim"`
	ReclaimCapPercent int    `json:"reclaim_cap_percent"`
	UsageBonusPercent int    `json:"usage_bonus_percent"`
	ThoroughRelease   bool   `json:"thorough_release"`
}

// QuotaPlanExecuteRequest carries the exact destructive-action confirmation phrase.
type QuotaPlanExecuteRequest struct {
	Confirmation string `json:"confirmation" binding:"required"`
}

// QuotaPlanCancelRequest carries the auditable cancellation reason.
type QuotaPlanCancelRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// QuotaCycleRestoreRequest carries the exact destructive-recovery confirmation phrase.
type QuotaCycleRestoreRequest struct {
	Confirmation string `json:"confirmation" binding:"required"`
}

// QuotaCycleResponse keeps quota values as decimal strings for JavaScript safety.
type QuotaCycleResponse struct {
	ID                         int    `json:"id"`
	CycleStartAt               int64  `json:"cycle_start_at"`
	CycleEndAt                 int64  `json:"cycle_end_at"`
	BudgetQuota                string `json:"budget_quota"`
	InitialGrantQuota          string `json:"initial_grant_quota"`
	RecoveryReserveQuota       string `json:"recovery_reserve_quota"`
	AutoRecoveryEnabled        bool   `json:"auto_recovery_enabled"`
	AutoRecoverySingleQuota    string `json:"auto_recovery_single_quota"`
	AutoRecoveryThresholdQuota string `json:"auto_recovery_threshold_quota"`
	AutoRecoveryMaxCount       int    `json:"auto_recovery_max_count"`
	AutoRecoveryMaxQuota       string `json:"auto_recovery_max_quota"`
	ConcentrationMultiplier    int64  `json:"concentration_multiplier_basis_points"`
	AllocationAlgorithmVersion string `json:"allocation_algorithm_version"`
	LegacyRollbackAllowed      bool   `json:"legacy_rollback_allowed"`
	BalancePolicy              string `json:"balance_policy"`
	Status                     string `json:"status"`
	SettlementPlanID           *int   `json:"settlement_plan_id"`
	SettledAt                  *int64 `json:"settled_at"`
	RestoredAt                 *int64 `json:"restored_at"`
	RestoredBy                 string `json:"restored_by"`
	CreatedAt                  int64  `json:"created_at"`
	CreatedBy                  string `json:"created_by"`
	UpdatedAt                  int64  `json:"updated_at"`
	UpdatedBy                  string `json:"updated_by"`
}

// QuotaPlanResponse serializes every persisted quota snapshot as a string.
type QuotaPlanResponse struct {
	ID                  int                    `json:"id"`
	CycleID             int                    `json:"cycle_id"`
	PlanType            string                 `json:"plan_type"`
	StagePercent        int                    `json:"stage_percent"`
	SnapshotAt          int64                  `json:"snapshot_at"`
	NextAdjustmentAt    *int64                 `json:"next_adjustment_at"`
	AlgorithmVersion    string                 `json:"algorithm_version"`
	Parameters          map[string]interface{} `json:"parameters"`
	BudgetQuotaSnapshot string                 `json:"budget_quota_snapshot"`
	TotalSpendQuota     string                 `json:"total_spend_quota"`
	ManagedBalanceQuota string                 `json:"managed_balance_quota"`
	PlannedDeltaQuota   string                 `json:"planned_delta_quota"`
	Status              string                 `json:"status"`
	CreatedAt           int64                  `json:"created_at"`
	CreatedBy           string                 `json:"created_by"`
	ExecutedAt          *int64                 `json:"executed_at"`
	ExecutedBy          string                 `json:"executed_by"`
	CancelledAt         *int64                 `json:"cancelled_at"`
	CancelledBy         string                 `json:"cancelled_by"`
	CancelReason        string                 `json:"cancel_reason"`
}

// QuotaItemResponse serializes planned and actual balances without numeric precision loss.
type QuotaItemResponse struct {
	ID                   int                    `json:"id"`
	PlanID               int                    `json:"plan_id"`
	UserID               int                    `json:"user_id"`
	Username             string                 `json:"username"`
	DisplayName          string                 `json:"display_name"`
	Email                string                 `json:"email"`
	Action               string                 `json:"action"`
	SnapshotBalanceQuota string                 `json:"snapshot_balance_quota"`
	AdjustmentQuota      string                 `json:"adjustment_quota"`
	RetainedQuota        string                 `json:"retained_quota"`
	CalculationData      map[string]interface{} `json:"calculation_data"`
	BasisText            string                 `json:"basis_text"`
	ActualBeforeQuota    *string                `json:"actual_before_quota"`
	ActualAfterQuota     *string                `json:"actual_after_quota"`
	LogContent           string                 `json:"log_content"`
	LogStatus            string                 `json:"log_status"`
	LogSentAt            *int64                 `json:"log_sent_at"`
	LogError             string                 `json:"log_error"`
	EmailStatus          string                 `json:"email_status"`
	EmailSentAt          *int64                 `json:"email_sent_at"`
	EmailError           string                 `json:"email_error"`
}

// QuotaCategoryResponse summarizes one adjustment category.
type QuotaCategoryResponse struct {
	Count int    `json:"count"`
	Total string `json:"total"`
}

// QuotaPlanSummaryResponse contains adjustment categories and conserved fund flow.
type QuotaPlanSummaryResponse struct {
	BaseIncrease                QuotaCategoryResponse `json:"base_increase"`
	Supplement                  QuotaCategoryResponse `json:"supplement"`
	Bonus                       QuotaCategoryResponse `json:"bonus"`
	WeightedPool                QuotaCategoryResponse `json:"weighted_pool"`
	Increase                    QuotaCategoryResponse `json:"increase"`
	OrdinaryDecrease            QuotaCategoryResponse `json:"ordinary_decrease"`
	FinalLowDecrease            QuotaCategoryResponse `json:"final_low_decrease"`
	DisabledDecrease            QuotaCategoryResponse `json:"disabled_decrease"`
	Decrease                    QuotaCategoryResponse `json:"decrease"`
	Affected                    int                   `json:"affected"`
	Budget                      string                `json:"budget"`
	StageCap                    string                `json:"stage_cap"`
	TotalSpend                  string                `json:"total_spend"`
	ManagedBalance              string                `json:"managed_balance"`
	OccupiedBefore              string                `json:"occupied_before"`
	OccupiedAfter               string                `json:"occupied_after"`
	BalanceAfter                string                `json:"balance_after"`
	AvailableBeforeReclaim      string                `json:"available_before_reclaim"`
	StageSourceTotal            string                `json:"stage_source_total"`
	ReclaimedUsedForIncreases   string                `json:"reclaimed_used_for_increases"`
	ReclaimedUsedToCoverOverage string                `json:"reclaimed_used_to_cover_overage"`
	ReclaimedUnused             string                `json:"reclaimed_unused"`
	StageOriginalUnused         string                `json:"stage_original_unused"`
	StageRemaining              string                `json:"stage_remaining"`
	PoolRemaining               string                `json:"pool_remaining"`
	FutureReserved              string                `json:"future_reserved"`
	RecoveryReserve             string                `json:"recovery_reserve"`
	FinalStage                  bool                  `json:"final_stage"`
}

// QuotaFairnessMetricsResponse contains deterministic basis-point coverage metrics.
type QuotaFairnessMetricsResponse struct {
	Population            int    `json:"population"`
	MinimumCoverage       int64  `json:"minimum_coverage_basis_points"`
	P10Coverage           int64  `json:"p10_coverage_basis_points"`
	P50Coverage           int64  `json:"p50_coverage_basis_points"`
	P90Coverage           int64  `json:"p90_coverage_basis_points"`
	MinimumSafetyCoverage int64  `json:"minimum_safety_coverage_basis_points"`
	SafetyUnmet           int    `json:"safety_unmet"`
	NewUserCount          int    `json:"new_user_count"`
	NewUserCoverage       *int64 `json:"new_user_coverage_basis_points"`
	ReclaimedQuota        string `json:"reclaimed_quota"`
	RecoveryReserveQuota  string `json:"recovery_reserve_quota"`
	OccupiedAfterQuota    string `json:"occupied_after_quota"`
}

// QuotaFairnessShadowItemResponse compares both algorithms without a persisted plan.
type QuotaFairnessShadowItemResponse struct {
	UserID                   int    `json:"user_id"`
	Username                 string `json:"username"`
	CurrentBalanceQuota      string `json:"current_balance_quota"`
	SafetyTargetQuota        string `json:"safety_target_quota"`
	DemandTargetQuota        string `json:"demand_target_quota"`
	TargetQuota              string `json:"target_quota"`
	CurrentAdjustmentQuota   string `json:"current_adjustment_quota"`
	CandidateAdjustmentQuota string `json:"candidate_adjustment_quota"`
	CurrentAfterQuota        string `json:"current_after_quota"`
	CandidateAfterQuota      string `json:"candidate_after_quota"`
	CurrentCoverage          int64  `json:"current_coverage_basis_points"`
	CandidateCoverage        int64  `json:"candidate_coverage_basis_points"`
}

// QuotaConcentrationShadowItemResponse exposes one managed user's capped shadow result.
type QuotaConcentrationShadowItemResponse struct {
	UserID               int    `json:"user_id"`
	Username             string `json:"username"`
	PeriodSpendQuota     string `json:"period_spend_quota"`
	CurrentBalanceQuota  string `json:"current_balance_quota"`
	CurrentPositionQuota string `json:"current_position_quota"`
	SpendShare           int64  `json:"spend_share_basis_points"`
	SafetyTargetQuota    string `json:"safety_target_quota"`
	RawTargetQuota       string `json:"raw_target_quota"`
	EffectiveTargetQuota string `json:"effective_target_quota"`
	PositionCeilingQuota string `json:"position_ceiling_quota"`
	AdjustmentQuota      string `json:"adjustment_quota"`
	AfterBalanceQuota    string `json:"after_balance_quota"`
	AfterPositionQuota   string `json:"after_position_quota"`
	CappedQuota          string `json:"capped_quota"`
	RawCoverage          int64  `json:"raw_coverage_basis_points"`
	EffectiveCoverage    int64  `json:"effective_coverage_basis_points"`
}

// QuotaConcentrationShadowVariantResponse contains one fixed multiplier projection.
type QuotaConcentrationShadowVariantResponse struct {
	Multiplier               int64                                  `json:"multiplier_basis_points"`
	Population               int                                    `json:"population"`
	PositionCeilingQuota     string                                 `json:"position_ceiling_quota"`
	MaximumPositionShare     int64                                  `json:"maximum_position_share_basis_points"`
	CappedUsers              int                                    `json:"capped_users"`
	CappedQuota              string                                 `json:"capped_quota"`
	MinimumRawCoverage       int64                                  `json:"minimum_raw_coverage_basis_points"`
	P10RawCoverage           int64                                  `json:"p10_raw_coverage_basis_points"`
	P50RawCoverage           int64                                  `json:"p50_raw_coverage_basis_points"`
	MinimumEffectiveCoverage int64                                  `json:"minimum_effective_coverage_basis_points"`
	P10EffectiveCoverage     int64                                  `json:"p10_effective_coverage_basis_points"`
	P50EffectiveCoverage     int64                                  `json:"p50_effective_coverage_basis_points"`
	MinimumSafetyCoverage    int64                                  `json:"minimum_safety_coverage_basis_points"`
	SafetyUnmet              int                                    `json:"safety_unmet"`
	PlannedIncreaseQuota     string                                 `json:"planned_increase_quota"`
	ReclaimedQuota           string                                 `json:"reclaimed_quota"`
	OccupiedAfterQuota       string                                 `json:"occupied_after_quota"`
	UnallocatedStageQuota    string                                 `json:"unallocated_stage_quota"`
	Items                    []QuotaConcentrationShadowItemResponse `json:"items"`
}

// QuotaFairnessShadowResponse is the read-only current/candidate comparison.
type QuotaFairnessShadowResponse struct {
	SnapshotAt                int64                                     `json:"snapshot_at"`
	StageCapQuota             string                                    `json:"stage_cap_quota"`
	CurrentAlgorithmVersion   string                                    `json:"current_algorithm_version"`
	CandidateAlgorithmVersion string                                    `json:"candidate_algorithm_version"`
	CandidateQualified        bool                                      `json:"candidate_qualified"`
	Current                   QuotaFairnessMetricsResponse              `json:"current"`
	Candidate                 QuotaFairnessMetricsResponse              `json:"candidate"`
	Items                     []QuotaFairnessShadowItemResponse         `json:"items"`
	ConcentrationVariants     []QuotaConcentrationShadowVariantResponse `json:"concentration_variants"`
}

// QuotaRecoveryCreateRequest submits one idempotent temporary-quota request.
type QuotaRecoveryCreateRequest struct {
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
	RequestedQuota string `json:"requested_quota" binding:"required"`
	Reason         string `json:"reason" binding:"required"`
}

// QuotaRecoveryDecisionRequest approves or rejects one pending request.
type QuotaRecoveryDecisionRequest struct {
	ApprovedQuota string `json:"approved_quota"`
	Reason        string `json:"reason" binding:"required"`
}

// QuotaRecoveryRequestResponse keeps persisted amounts safe for JavaScript clients.
type QuotaRecoveryRequestResponse struct {
	ID             int    `json:"id"`
	CycleID        int    `json:"cycle_id"`
	UserID         int    `json:"user_id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	RequestedQuota string `json:"requested_quota"`
	Reason         string `json:"reason"`
	Status         string `json:"status"`
	Decision       string `json:"decision"`
	ApprovedQuota  string `json:"approved_quota"`
	PlanID         *int   `json:"plan_id"`
	ReviewedBy     string `json:"reviewed_by"`
	ReviewReason   string `json:"review_reason"`
	CreatedAt      int64  `json:"created_at"`
	ReviewedAt     *int64 `json:"reviewed_at"`
	ExecutedAt     *int64 `json:"executed_at"`
}

// QuotaAlgorithmSwitchRequest carries the exact switch confirmation phrase.
type QuotaAlgorithmSwitchRequest struct {
	TargetVersion string `json:"target_version" binding:"required"`
	Confirmation  string `json:"confirmation" binding:"required"`
}
