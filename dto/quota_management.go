package dto

// QuotaCycleCreateRequest creates a quota cycle from Unix-second boundaries and raw quota strings.
type QuotaCycleCreateRequest struct {
	CycleStartAt                             int64  `json:"cycle_start_at" binding:"required"`
	CycleEndAt                               int64  `json:"cycle_end_at" binding:"required"`
	BudgetQuota                              string `json:"budget_quota" binding:"required"`
	InitialGrantQuota                        string `json:"initial_grant_quota" binding:"required"`
	TemporaryQuotaReserve                    string `json:"temporary_quota_reserve"`
	TemporaryQuotaAutoApprovalEnabled        bool   `json:"temporary_quota_auto_approval_enabled"`
	TemporaryQuotaAutoApprovalSingleQuota    string `json:"temporary_quota_auto_approval_single_quota"`
	TemporaryQuotaAutoApprovalThresholdQuota string `json:"temporary_quota_auto_approval_threshold_quota"`
	TemporaryQuotaAutoApprovalMaxCount       int    `json:"temporary_quota_auto_approval_max_count"`
	TemporaryQuotaAutoApprovalMaxQuota       string `json:"temporary_quota_auto_approval_max_quota"`
	ConcentrationMultiplier                  int64  `json:"concentration_multiplier_basis_points" binding:"required"`
	BalancePolicy                            string `json:"balance_policy" binding:"required"`
	CarryOverCountsTowardBudget              bool   `json:"carry_over_counts_toward_budget"`
}

// QuotaCycleUpdateRequest updates budget and optionally the scheduled initial grant.
type QuotaCycleUpdateRequest struct {
	BudgetQuota                              string  `json:"budget_quota" binding:"required"`
	InitialGrantQuota                        *string `json:"initial_grant_quota"`
	TemporaryQuotaReserve                    *string `json:"temporary_quota_reserve"`
	TemporaryQuotaAutoApprovalEnabled        *bool   `json:"temporary_quota_auto_approval_enabled"`
	TemporaryQuotaAutoApprovalSingleQuota    *string `json:"temporary_quota_auto_approval_single_quota"`
	TemporaryQuotaAutoApprovalThresholdQuota *string `json:"temporary_quota_auto_approval_threshold_quota"`
	TemporaryQuotaAutoApprovalMaxCount       *int    `json:"temporary_quota_auto_approval_max_count"`
	TemporaryQuotaAutoApprovalMaxQuota       *string `json:"temporary_quota_auto_approval_max_quota"`
}

// QuotaPlanGenerateRequest contains the administrator-reviewed allocation controls.
type QuotaPlanGenerateRequest struct {
	CycleID          int    `json:"cycle_id" binding:"required"`
	Name             string `json:"name" binding:"required"`
	Purpose          string `json:"purpose" binding:"required"`
	PlanType         string `json:"plan_type" binding:"required"`
	NextAdjustmentAt int64  `json:"next_adjustment_at" binding:"required"`
}

// QuotaPlanExecuteRequest carries the exact destructive-action confirmation phrase.
type QuotaPlanExecuteRequest struct {
	Confirmation string `json:"confirmation" binding:"required"`
}

// QuotaPlanCancelRequest carries the auditable cancellation reason.
type QuotaPlanCancelRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// QuotaCycleResponse keeps quota values as decimal strings for JavaScript safety.
type QuotaCycleResponse struct {
	ID                                       int     `json:"id"`
	CycleStartAt                             int64   `json:"cycle_start_at"`
	CycleEndAt                               int64   `json:"cycle_end_at"`
	BudgetQuota                              string  `json:"budget_quota"`
	OpeningAllocatedQuota                    *string `json:"opening_allocated_quota"`
	AllocatedQuota                           *string `json:"allocated_quota"`
	RemainingQuota                           *string `json:"remaining_quota"`
	AllocationError                          *string `json:"allocation_error"`
	InitialGrantQuota                        string  `json:"initial_grant_quota"`
	TemporaryQuotaReserve                    string  `json:"temporary_quota_reserve"`
	TemporaryQuotaAutoApprovalEnabled        bool    `json:"temporary_quota_auto_approval_enabled"`
	TemporaryQuotaAutoApprovalSingleQuota    string  `json:"temporary_quota_auto_approval_single_quota"`
	TemporaryQuotaAutoApprovalThresholdQuota string  `json:"temporary_quota_auto_approval_threshold_quota"`
	TemporaryQuotaAutoApprovalMaxCount       int     `json:"temporary_quota_auto_approval_max_count"`
	TemporaryQuotaAutoApprovalMaxQuota       string  `json:"temporary_quota_auto_approval_max_quota"`
	ConcentrationMultiplier                  int64   `json:"concentration_multiplier_basis_points"`
	BalancePolicy                            string  `json:"balance_policy"`
	CarryOverCountsTowardBudget              bool    `json:"carry_over_counts_toward_budget"`
	Status                                   string  `json:"status"`
	CreatedAt                                int64   `json:"created_at"`
	CreatedBy                                string  `json:"created_by"`
	UpdatedAt                                int64   `json:"updated_at"`
	UpdatedBy                                string  `json:"updated_by"`
}

// QuotaPlanResponse contains the plan fields displayed by the management UI.
type QuotaPlanResponse struct {
	ID                   int     `json:"id"`
	CycleID              int     `json:"cycle_id"`
	Name                 string  `json:"name"`
	Purpose              string  `json:"purpose"`
	PlanType             string  `json:"plan_type"`
	SnapshotAt           int64   `json:"snapshot_at"`
	NextAdjustmentAt     *int64  `json:"next_adjustment_at"`
	PlannedDeltaQuota    string  `json:"planned_delta_quota"`
	AllocationAfterQuota *string `json:"allocation_after_quota"`
	Status               string  `json:"status"`
	CreatedAt            int64   `json:"created_at"`
	ExecutedAt           *int64  `json:"executed_at"`
	CancelledAt          *int64  `json:"cancelled_at"`
	CancelReason         string  `json:"cancel_reason"`
}

// QuotaItemResponse serializes planned and actual balances without numeric precision loss.
type QuotaItemResponse struct {
	ID                   int     `json:"id"`
	UserID               int     `json:"user_id"`
	Username             string  `json:"username"`
	DisplayName          string  `json:"display_name"`
	Action               string  `json:"action"`
	SnapshotBalanceQuota string  `json:"snapshot_balance_quota"`
	AdjustmentQuota      string  `json:"adjustment_quota"`
	BasisText            string  `json:"basis_text"`
	ActualBeforeQuota    *string `json:"actual_before_quota"`
	ActualAfterQuota     *string `json:"actual_after_quota"`
	LogStatus            string  `json:"log_status"`
	EmailStatus          string  `json:"email_status"`
}

// QuotaCategoryResponse summarizes one adjustment category.
type QuotaCategoryResponse struct {
	Count int    `json:"count"`
	Total string `json:"total"`
}

// QuotaPlanSummaryResponse contains adjustment categories and conserved fund flow.
type QuotaPlanSummaryResponse struct {
	Increase                    QuotaCategoryResponse `json:"increase"`
	Decrease                    QuotaCategoryResponse `json:"decrease"`
	Affected                    int                   `json:"affected"`
	Budget                      string                `json:"budget"`
	OccupiedAfter               string                `json:"occupied_after"`
	CyclePoolIssuedQuota        string                `json:"cycle_pool_issued_quota"`
	ReclaimedUsedForIncreases   string                `json:"reclaimed_used_for_increases"`
	ReclaimedUsedToCoverOverage string                `json:"reclaimed_used_to_cover_overage"`
	ReclaimedUnused             string                `json:"reclaimed_unused"`
	PoolRemaining               string                `json:"pool_remaining"`
}

// TemporaryQuotaCreateRequest submits one idempotent temporary-quota request.
type TemporaryQuotaCreateRequest struct {
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
	RequestedQuota string `json:"requested_quota" binding:"required"`
	Project        string `json:"project" binding:"required"`
	Reason         string `json:"reason" binding:"required"`
}

// TemporaryQuotaDecisionRequest approves or rejects one pending request.
type TemporaryQuotaDecisionRequest struct {
	ApprovedQuota string `json:"approved_quota"`
	Reason        string `json:"reason" binding:"required"`
}

// TemporaryQuotaRequestResponse keeps persisted amounts safe for JavaScript clients.
type TemporaryQuotaRequestResponse struct {
	ID             int    `json:"id"`
	UserID         int    `json:"user_id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	RequestedQuota string `json:"requested_quota"`
	Project        string `json:"project"`
	Reason         string `json:"reason"`
	Status         string `json:"status"`
	ReviewReason   string `json:"review_reason"`
	CreatedAt      int64  `json:"created_at"`
}

// SelfTemporaryQuotaRequestResponse contains only fields visible to the requesting user.
type SelfTemporaryQuotaRequestResponse struct {
	ID             int    `json:"id"`
	RequestedQuota string `json:"requested_quota"`
	Project        string `json:"project"`
	Reason         string `json:"reason"`
	Status         string `json:"status"`
	ApprovedQuota  string `json:"approved_quota"`
	ReviewReason   string `json:"review_reason"`
	CreatedAt      int64  `json:"created_at"`
	ReviewedAt     *int64 `json:"reviewed_at"`
	ExecutedAt     *int64 `json:"executed_at"`
}

// SelfTemporaryQuotaOverviewResponse exposes application availability without internal approval rules.
type SelfTemporaryQuotaOverviewResponse struct {
	CurrentQuota      string   `json:"current_quota"`
	CanRequest        bool     `json:"can_request"`
	UnavailableReason string   `json:"unavailable_reason"`
	Projects          []string `json:"projects"`
}
