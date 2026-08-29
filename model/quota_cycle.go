package model

import (
	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// QuotaCycleStatus is the lifecycle state of a quota allocation cycle.
type QuotaCycleStatus string

const (
	QuotaCycleStatusScheduled QuotaCycleStatus = "scheduled"
	QuotaCycleStatusActive    QuotaCycleStatus = "active"
	QuotaCycleStatusSettling  QuotaCycleStatus = "settling"
	QuotaCycleStatusClosed    QuotaCycleStatus = "closed"
)

// QuotaCycleBalancePolicy defines what happens to managed balances at cycle close.
type QuotaCycleBalancePolicy string

const (
	QuotaCycleBalancePolicyReset QuotaCycleBalancePolicy = "reset"
	QuotaCycleBalancePolicyCarry QuotaCycleBalancePolicy = "carry"
)

// Valid reports whether the persisted cycle balance policy is supported.
func (policy QuotaCycleBalancePolicy) Valid() bool {
	return policy == QuotaCycleBalancePolicyReset || policy == QuotaCycleBalancePolicyCarry
}

// QuotaCycle stores one quota allocation window and its fixed initial grant.
type QuotaCycle struct {
	Id                                       int                     `json:"id" gorm:"primaryKey"`
	CycleStartAt                             int64                   `json:"cycle_start_at" gorm:"bigint;uniqueIndex;index:idx_tool_quota_cycles_time,priority:1"`
	CycleEndAt                               int64                   `json:"cycle_end_at" gorm:"bigint;index:idx_tool_quota_cycles_time,priority:2"`
	BudgetQuota                              int64                   `json:"-" gorm:"bigint"`
	OpeningAllocatedQuota                    *int64                  `json:"-" gorm:"bigint"`
	AllocatedQuota                           *int64                  `json:"-" gorm:"bigint"`
	AllocationBaselineAt                     *int64                  `json:"allocation_baseline_at,omitempty" gorm:"bigint;index"`
	InitialGrantQuota                        int64                   `json:"-" gorm:"bigint"`
	TemporaryQuotaReserve                    int64                   `json:"-" gorm:"bigint"`
	TemporaryQuotaAutoApprovalEnabled        bool                    `json:"temporary_quota_auto_approval_enabled"`
	TemporaryQuotaAutoApprovalSingleQuota    int64                   `json:"-" gorm:"bigint"`
	TemporaryQuotaAutoApprovalThresholdQuota int64                   `json:"-" gorm:"bigint"`
	TemporaryQuotaAutoApprovalMaxCount       int                     `json:"temporary_quota_auto_approval_max_count"`
	TemporaryQuotaAutoApprovalMaxQuota       int64                   `json:"-" gorm:"bigint"`
	ConcentrationMultiplier                  int64                   `json:"concentration_multiplier_basis_points" gorm:"bigint;not null"`
	BalancePolicy                            QuotaCycleBalancePolicy `json:"balance_policy" gorm:"type:varchar(20);index;not null"`
	CarryOverCountsTowardBudget              bool                    `json:"-" gorm:"not null"`
	Status                                   QuotaCycleStatus        `json:"status" gorm:"type:varchar(20);index"`
	ActiveKey                                *int                    `json:"-" gorm:"uniqueIndex:idx_tool_quota_cycles_active_key"`
	RequestObservationFailedAt               *int64                  `json:"-" gorm:"bigint"`
	SettledAt                                *int64                  `json:"settled_at,omitempty" gorm:"bigint;index"`
	CreatedAt                                int64                   `json:"created_at" gorm:"bigint;index"`
	CreatedBy                                string                  `json:"created_by" gorm:"type:varchar(255)"`
	UpdatedAt                                int64                   `json:"updated_at" gorm:"bigint"`
	UpdatedBy                                string                  `json:"updated_by" gorm:"type:varchar(255)"`
}

// TableName returns the stable quota-cycle table.
func (QuotaCycle) TableName() string {
	return "tool_quota_cycles"
}

// SetStatus keeps the cross-database single-active-cycle key in sync.
func (cycle *QuotaCycle) SetStatus(status QuotaCycleStatus) {
	cycle.Status = status
	cycle.ActiveKey = nil
	if status == QuotaCycleStatusActive || status == QuotaCycleStatusSettling {
		active := 1
		cycle.ActiveKey = &active
	}
}

// BeforeCreate initializes timestamps and the active-cycle key.
func (cycle *QuotaCycle) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if cycle.CreatedAt == 0 {
		cycle.CreatedAt = now
	}
	if cycle.UpdatedAt == 0 {
		cycle.UpdatedAt = now
	}
	cycle.SetStatus(cycle.Status)
	return nil
}
