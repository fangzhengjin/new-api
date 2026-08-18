package model

// QuotaRecoveryRequestStatus is the persisted temporary-quota request state.
type QuotaRecoveryRequestStatus string

const (
	QuotaRecoveryRequestStatusPending  QuotaRecoveryRequestStatus = "pending"
	QuotaRecoveryRequestStatusExecuted QuotaRecoveryRequestStatus = "executed"
	QuotaRecoveryRequestStatusRejected QuotaRecoveryRequestStatus = "rejected"
)

// QuotaRecoveryDecision records whether a grant was approved automatically or manually.
type QuotaRecoveryDecision string

const (
	QuotaRecoveryDecisionAuto   QuotaRecoveryDecision = "auto"
	QuotaRecoveryDecisionManual QuotaRecoveryDecision = "manual"
)

// QuotaRecoveryRequest links one idempotent user request to at most one executed grant plan.
type QuotaRecoveryRequest struct {
	Id             int                        `json:"id" gorm:"primaryKey"`
	CycleId        int                        `json:"cycle_id" gorm:"index:idx_tool_quota_recovery_cycle_status,priority:1"`
	UserId         int                        `json:"user_id" gorm:"uniqueIndex:idx_tool_quota_recovery_user_key,priority:1;index"`
	Username       string                     `json:"username" gorm:"type:varchar(255)"`
	DisplayName    string                     `json:"display_name" gorm:"type:varchar(255)"`
	IdempotencyKey string                     `json:"idempotency_key" gorm:"type:varchar(64);uniqueIndex:idx_tool_quota_recovery_user_key,priority:2"`
	RequestedQuota int64                      `json:"-" gorm:"bigint"`
	Reason         string                     `json:"reason" gorm:"type:text"`
	Status         QuotaRecoveryRequestStatus `json:"status" gorm:"type:varchar(20);index:idx_tool_quota_recovery_cycle_status,priority:2"`
	Decision       QuotaRecoveryDecision      `json:"decision" gorm:"type:varchar(20)"`
	ApprovedQuota  int64                      `json:"-" gorm:"bigint"`
	PlanId         *int                       `json:"plan_id,omitempty" gorm:"uniqueIndex"`
	ReviewedBy     string                     `json:"reviewed_by" gorm:"type:varchar(255)"`
	ReviewReason   string                     `json:"review_reason" gorm:"type:text"`
	CreatedAt      int64                      `json:"created_at" gorm:"bigint;index"`
	ReviewedAt     *int64                     `json:"reviewed_at,omitempty" gorm:"bigint"`
	ExecutedAt     *int64                     `json:"executed_at,omitempty" gorm:"bigint"`
}

// TableName returns the stable temporary-quota request table.
func (QuotaRecoveryRequest) TableName() string { return "tool_quota_recovery_requests" }

// QuotaShadowEvidence stores one final-window, non-PII algorithm gate result per cycle.
type QuotaShadowEvidence struct {
	Id                        int    `json:"id" gorm:"primaryKey"`
	CycleId                   int    `json:"cycle_id" gorm:"uniqueIndex:idx_tool_quota_shadow_cycle_version,priority:1;index"`
	SnapshotAt                int64  `json:"snapshot_at" gorm:"bigint;index"`
	CurrentAlgorithmVersion   string `json:"current_algorithm_version" gorm:"type:varchar(50)"`
	CandidateAlgorithmVersion string `json:"candidate_algorithm_version" gorm:"type:varchar(50);uniqueIndex:idx_tool_quota_shadow_cycle_version,priority:2"`
	StagePercent              int    `json:"stage_percent"`
	Parameters                string `json:"-" gorm:"type:text"`
	Metrics                   string `json:"-" gorm:"type:text"`
	Qualified                 bool   `json:"qualified" gorm:"index"`
	CreatedAt                 int64  `json:"created_at" gorm:"bigint"`
	CreatedBy                 string `json:"created_by" gorm:"type:varchar(255)"`
}

// TableName returns the stable fairness-gate evidence table.
func (QuotaShadowEvidence) TableName() string { return "tool_quota_shadow_evidence" }
