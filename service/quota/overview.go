package quota

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type QuotaOverview struct {
	UpdatedAt                  int64
	PreviousCycle              *model.QuotaCycle
	CurrentCycle               *model.QuotaCycle
	NextCycle                  *model.QuotaCycle
	AllocationReady            bool
	CurrentPlan                *model.QuotaPlan
	CurrentPlanSummary         *DetailedPlanSummary
	PendingTemporaryQuotaCount int64
	PendingAllocationCount     int64
	PendingReclaimCount        int64
}

func GetQuotaOverview() (*QuotaOverview, error) {
	return buildQuotaOverview()
}

func buildQuotaOverview() (*QuotaOverview, error) {
	overview := &QuotaOverview{UpdatedAt: time.Now().Unix()}
	var previous, current, next model.QuotaCycle
	if err := model.DB.Where("status IN ?", []model.QuotaCycleStatus{model.QuotaCycleStatusActive, model.QuotaCycleStatusSettling}).
		Order("cycle_start_at DESC, id DESC").First(&current).Error; err == nil {
		overview.CurrentCycle = &current
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	previousBefore := overview.UpdatedAt
	if overview.CurrentCycle != nil {
		previousBefore = overview.CurrentCycle.CycleStartAt
	}
	if err := model.DB.Where("status = ? AND cycle_end_at <= ?", model.QuotaCycleStatusClosed, previousBefore).
		Order("cycle_end_at DESC, id DESC").First(&previous).Error; err == nil {
		overview.PreviousCycle = &previous
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := model.DB.Where("status = ?", model.QuotaCycleStatusScheduled).
		Order("cycle_start_at, id").First(&next).Error; err == nil {
		overview.NextCycle = &next
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if overview.CurrentCycle == nil {
		return overview, nil
	}
	if err := CycleAllocationError(overview.CurrentCycle); err != nil {
		common.SysError("invalid quota allocation data: " + err.Error())
	} else if overview.CurrentCycle.AllocatedQuota != nil {
		overview.AllocationReady = true
	}
	var plan model.QuotaPlan
	if err := model.DB.Where("cycle_id = ? AND status IN ?", overview.CurrentCycle.Id, []model.QuotaPlanStatus{
		model.QuotaPlanStatusDraft, model.QuotaPlanStatusExecuted,
	}).
		Order("CASE WHEN status = 'draft' THEN 0 ELSE 1 END, created_at DESC, id DESC").First(&plan).Error; err == nil {
		overview.CurrentPlan = &plan
		var items []model.QuotaItem
		if err := model.DB.Where("plan_id = ?", plan.Id).Order("id").Find(&items).Error; err != nil {
			return nil, err
		}
		temporaryQuotaUsed, err := temporaryQuotaReserveUsedAt(model.DB, overview.CurrentCycle.Id, plan.SnapshotAt, plan.Id)
		if err != nil {
			return nil, err
		}
		summary, err := summarizePlan(plan, items, overview.CurrentCycle.TemporaryQuotaReserve, temporaryQuotaUsed)
		if err != nil {
			return nil, err
		}
		overview.CurrentPlanSummary = &summary
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := model.DB.Model(&model.TemporaryQuotaRequest{}).
		Where("cycle_id = ? AND status = ?", overview.CurrentCycle.Id, model.TemporaryQuotaRequestStatusPending).
		Count(&overview.PendingTemporaryQuotaCount).Error; err != nil {
		return nil, err
	}
	if err := model.DB.Unscoped().Model(&model.User{}).
		Where("quota_whitelist = ? AND status = ? AND deleted_at IS NULL AND quota <= 0", false, common.UserStatusEnabled).
		Count(&overview.PendingAllocationCount).Error; err != nil {
		return nil, err
	}
	if err := model.DB.Unscoped().Model(&model.User{}).
		Where("quota_whitelist = ? AND (status <> ? OR deleted_at IS NOT NULL) AND quota > 0", false, common.UserStatusEnabled).
		Count(&overview.PendingReclaimCount).Error; err != nil {
		return nil, err
	}
	return overview, nil
}
