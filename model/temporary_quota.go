package model

// TemporaryQuotaRequestStatus is the persisted temporary-quota request state.
type TemporaryQuotaRequestStatus string

const (
	TemporaryQuotaRequestStatusPending  TemporaryQuotaRequestStatus = "pending"
	TemporaryQuotaRequestStatusExecuted TemporaryQuotaRequestStatus = "executed"
	TemporaryQuotaRequestStatusRejected TemporaryQuotaRequestStatus = "rejected"
)

// TemporaryQuotaDecision records whether a grant was approved automatically or manually.
type TemporaryQuotaDecision string

const (
	TemporaryQuotaDecisionAuto   TemporaryQuotaDecision = "auto"
	TemporaryQuotaDecisionManual TemporaryQuotaDecision = "manual"
)

// TemporaryQuotaRequest links one idempotent user request to at most one executed grant plan.
type TemporaryQuotaRequest struct {
	Id             int                         `json:"id" gorm:"primaryKey"`
	CycleId        int                         `json:"cycle_id" gorm:"index:idx_tool_quota_temporary_cycle_status,priority:1"`
	UserId         int                         `json:"user_id" gorm:"uniqueIndex:idx_tool_quota_temporary_user_key,priority:1;index"`
	Username       string                      `json:"username" gorm:"type:varchar(255)"`
	DisplayName    string                      `json:"display_name" gorm:"type:varchar(255)"`
	IdempotencyKey string                      `json:"idempotency_key" gorm:"type:varchar(64);uniqueIndex:idx_tool_quota_temporary_user_key,priority:2"`
	RequestedQuota int64                       `json:"-" gorm:"bigint"`
	Project        string                      `json:"project" gorm:"type:varchar(100)"`
	Reason         string                      `json:"reason" gorm:"type:text"`
	Status         TemporaryQuotaRequestStatus `json:"status" gorm:"type:varchar(20);index:idx_tool_quota_temporary_cycle_status,priority:2"`
	Decision       TemporaryQuotaDecision      `json:"decision" gorm:"type:varchar(20)"`
	ApprovedQuota  int64                       `json:"-" gorm:"bigint"`
	PlanId         *int                        `json:"plan_id,omitempty" gorm:"uniqueIndex"`
	ReviewedBy     string                      `json:"reviewed_by" gorm:"type:varchar(255)"`
	ReviewReason   string                      `json:"review_reason" gorm:"type:text"`
	CreatedAt      int64                       `json:"created_at" gorm:"bigint;index"`
	ReviewedAt     *int64                      `json:"reviewed_at,omitempty" gorm:"bigint"`
	ExecutedAt     *int64                      `json:"executed_at,omitempty" gorm:"bigint"`
}

// TableName returns the stable temporary-quota request table.
func (TemporaryQuotaRequest) TableName() string { return "tool_quota_temporary_requests" }
