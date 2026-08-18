package dto

// QuotaCycleCreateRequest creates a purchasing cycle from Unix-second boundaries and raw quota strings.
type QuotaCycleCreateRequest struct {
	CycleStartAt      int64  `json:"cycle_start_at" binding:"required"`
	CycleEndAt        int64  `json:"cycle_end_at" binding:"required"`
	BudgetQuota       string `json:"budget_quota" binding:"required"`
	InitialGrantQuota string `json:"initial_grant_quota" binding:"required"`
	BalancePolicy     string `json:"balance_policy" binding:"required"`
}

// QuotaCycleUpdateRequest updates budget and optionally the scheduled initial grant.
type QuotaCycleUpdateRequest struct {
	BudgetQuota       string  `json:"budget_quota" binding:"required"`
	InitialGrantQuota *string `json:"initial_grant_quota"`
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
	ID                int    `json:"id"`
	CycleStartAt      int64  `json:"cycle_start_at"`
	CycleEndAt        int64  `json:"cycle_end_at"`
	BudgetQuota       string `json:"budget_quota"`
	InitialGrantQuota string `json:"initial_grant_quota"`
	BalancePolicy     string `json:"balance_policy"`
	Status            string `json:"status"`
	SettlementPlanID  *int   `json:"settlement_plan_id"`
	SettledAt         *int64 `json:"settled_at"`
	RestoredAt        *int64 `json:"restored_at"`
	RestoredBy        string `json:"restored_by"`
	CreatedAt         int64  `json:"created_at"`
	CreatedBy         string `json:"created_by"`
	UpdatedAt         int64  `json:"updated_at"`
	UpdatedBy         string `json:"updated_by"`
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
	FinalStage                  bool                  `json:"final_stage"`
}
