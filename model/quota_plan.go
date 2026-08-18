package model

// QuotaPlanType identifies initialization and in-cycle adjustment plans.
type QuotaPlanType string

const (
	QuotaPlanTypeInitialization QuotaPlanType = "initialization"
	QuotaPlanTypeAdjustment     QuotaPlanType = "adjustment"
	QuotaPlanTypeSettlement     QuotaPlanType = "settlement"
)

// QuotaPlanStatus is the persisted plan state machine.
type QuotaPlanStatus string

const (
	QuotaPlanStatusDraft     QuotaPlanStatus = "draft"
	QuotaPlanStatusExecuted  QuotaPlanStatus = "executed"
	QuotaPlanStatusCancelled QuotaPlanStatus = "cancelled"
)

// QuotaAdjustmentAction identifies the user-visible quota operation.
type QuotaAdjustmentAction string

const (
	QuotaAdjustmentActionInitialize QuotaAdjustmentAction = "initialize"
	QuotaAdjustmentActionIncrease   QuotaAdjustmentAction = "increase"
	QuotaAdjustmentActionDecrease   QuotaAdjustmentAction = "decrease"
	QuotaAdjustmentActionGrant      QuotaAdjustmentAction = "grant"
	QuotaAdjustmentActionReclaim    QuotaAdjustmentAction = "reclaim"
	QuotaAdjustmentActionRestore    QuotaAdjustmentAction = "restore"
)

// QuotaNotificationStatus tracks delivery without repeating balance changes.
type QuotaNotificationStatus string

const (
	QuotaNotificationStatusPending QuotaNotificationStatus = "pending"
	QuotaNotificationStatusSent    QuotaNotificationStatus = "sent"
	QuotaNotificationStatusFailed  QuotaNotificationStatus = "failed"
	QuotaNotificationStatusSkipped QuotaNotificationStatus = "skipped"
)

// QuotaPlan stores an immutable calculation snapshot and its execution state.
type QuotaPlan struct {
	Id                  int             `json:"id" gorm:"primaryKey"`
	CycleId             int             `json:"cycle_id" gorm:"index:idx_tool_quota_plans_cycle_status,priority:1"`
	PlanType            QuotaPlanType   `json:"plan_type" gorm:"type:varchar(20)"`
	StagePercent        int             `json:"stage_percent"`
	SnapshotAt          int64           `json:"snapshot_at" gorm:"bigint;index"`
	NextAdjustmentAt    *int64          `json:"next_adjustment_at,omitempty" gorm:"bigint"`
	AlgorithmVersion    string          `json:"algorithm_version" gorm:"type:varchar(50)"`
	Parameters          string          `json:"-" gorm:"type:text"`
	BudgetQuotaSnapshot int64           `json:"-" gorm:"bigint"`
	TotalSpendQuota     int64           `json:"-" gorm:"bigint"`
	ManagedBalanceQuota int64           `json:"-" gorm:"bigint"`
	PlannedDeltaQuota   int64           `json:"-" gorm:"bigint"`
	Status              QuotaPlanStatus `json:"status" gorm:"type:varchar(20);index:idx_tool_quota_plans_cycle_status,priority:2"`
	CreatedAt           int64           `json:"created_at" gorm:"bigint;index"`
	CreatedBy           string          `json:"created_by" gorm:"type:varchar(255)"`
	ExecutedAt          *int64          `json:"executed_at,omitempty" gorm:"bigint"`
	ExecutedBy          string          `json:"executed_by" gorm:"type:varchar(255)"`
	CancelledAt         *int64          `json:"cancelled_at,omitempty" gorm:"bigint"`
	CancelledBy         string          `json:"cancelled_by" gorm:"type:varchar(255)"`
	CancelReason        string          `json:"cancel_reason" gorm:"type:text"`
}

// TableName returns the stable plan table used by the imported quota tool.
func (QuotaPlan) TableName() string {
	return "tool_quota_adjustment_plans"
}

// QuotaItem stores one planned user delta and its execution/notification result.
type QuotaItem struct {
	Id                   int                     `json:"id" gorm:"primaryKey"`
	PlanId               int                     `json:"plan_id" gorm:"uniqueIndex:idx_tool_quota_items_plan_user,priority:1;index"`
	UserId               int                     `json:"user_id" gorm:"uniqueIndex:idx_tool_quota_items_plan_user,priority:2;index"`
	Username             string                  `json:"username" gorm:"type:varchar(255)"`
	DisplayName          string                  `json:"display_name" gorm:"type:varchar(255)"`
	Email                string                  `json:"email" gorm:"type:varchar(255)"`
	Action               QuotaAdjustmentAction   `json:"action" gorm:"type:varchar(20);index"`
	SnapshotBalanceQuota int64                   `json:"-" gorm:"bigint"`
	AdjustmentQuota      int64                   `json:"-" gorm:"bigint"`
	RetainedQuota        int64                   `json:"-" gorm:"bigint"`
	CalculationData      string                  `json:"-" gorm:"type:text"`
	BasisText            string                  `json:"basis_text" gorm:"type:text"`
	ActualBeforeQuota    *int64                  `json:"-" gorm:"bigint"`
	ActualAfterQuota     *int64                  `json:"-" gorm:"bigint"`
	LogContent           string                  `json:"log_content" gorm:"type:text"`
	LogStatus            QuotaNotificationStatus `json:"log_status" gorm:"type:varchar(20);index:idx_tool_quota_items_log_status"`
	LogSentAt            *int64                  `json:"log_sent_at,omitempty" gorm:"bigint"`
	LogError             string                  `json:"log_error" gorm:"type:text"`
	EmailStatus          QuotaNotificationStatus `json:"email_status" gorm:"type:varchar(20);index:idx_tool_quota_items_email_status"`
	EmailSentAt          *int64                  `json:"email_sent_at,omitempty" gorm:"bigint"`
	EmailError           string                  `json:"email_error" gorm:"type:text"`
}

// TableName returns the stable adjustment-item table used by the imported quota tool.
func (QuotaItem) TableName() string {
	return "tool_quota_adjustment_items"
}
