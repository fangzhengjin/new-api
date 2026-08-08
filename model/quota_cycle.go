package model

import (
	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// QuotaCycleStatus is the lifecycle state of a quota purchasing cycle.
type QuotaCycleStatus string

const (
	QuotaCycleStatusScheduled QuotaCycleStatus = "scheduled"
	QuotaCycleStatusActive    QuotaCycleStatus = "active"
	QuotaCycleStatusClosed    QuotaCycleStatus = "closed"
)

// QuotaCycle stores one quota purchasing window and its fixed initial grant.
type QuotaCycle struct {
	Id                int              `json:"id" gorm:"primaryKey"`
	CycleStartAt      int64            `json:"cycle_start_at" gorm:"bigint;uniqueIndex;index:idx_tool_quota_cycles_time,priority:1"`
	CycleEndAt        int64            `json:"cycle_end_at" gorm:"bigint;index:idx_tool_quota_cycles_time,priority:2"`
	BudgetQuota       int64            `json:"-" gorm:"bigint"`
	InitialGrantQuota int64            `json:"-" gorm:"bigint"`
	Status            QuotaCycleStatus `json:"status" gorm:"type:varchar(20);index"`
	ActiveKey         *int             `json:"-" gorm:"uniqueIndex:idx_tool_quota_cycles_active_key"`
	CreatedAt         int64            `json:"created_at" gorm:"bigint;index"`
	CreatedBy         string           `json:"created_by" gorm:"type:varchar(255)"`
	UpdatedAt         int64            `json:"updated_at" gorm:"bigint"`
	UpdatedBy         string           `json:"updated_by" gorm:"type:varchar(255)"`
}

// TableName returns the stable table used by the imported quota tool.
func (QuotaCycle) TableName() string {
	return "tool_quota_cycles"
}

// SetStatus keeps the cross-database single-active-cycle key in sync.
func (cycle *QuotaCycle) SetStatus(status QuotaCycleStatus) {
	cycle.Status = status
	cycle.ActiveKey = nil
	if status == QuotaCycleStatusActive {
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
