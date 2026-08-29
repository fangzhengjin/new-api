package quota

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

var ErrAllocationNotInitialized = errors.New("当前周期尚未确认期初已发放额度，请先完成上线核对")

// CycleAllocationError validates saved allocation data for read responses.
// A fully empty snapshot means the cycle has not been initialized; a partial
// or out-of-range snapshot means persisted data is invalid.
func CycleAllocationError(cycle *model.QuotaCycle) error {
	if cycle == nil {
		return nil
	}
	present := 0
	if cycle.OpeningAllocatedQuota != nil {
		present++
	}
	if cycle.AllocatedQuota != nil {
		present++
	}
	if cycle.AllocationBaselineAt != nil {
		present++
	}
	if present == 0 {
		return nil
	}
	if present != 3 {
		return errors.New("周期额度记录不完整")
	}
	_, err := cycleAllocatedQuota(cycle)
	return err
}

func cycleAllocatedQuota(cycle *model.QuotaCycle) (int64, error) {
	if cycle == nil || cycle.AllocatedQuota == nil || cycle.OpeningAllocatedQuota == nil || cycle.AllocationBaselineAt == nil {
		return 0, ErrAllocationNotInitialized
	}
	if *cycle.AllocatedQuota < 0 || *cycle.OpeningAllocatedQuota < 0 {
		return 0, errors.New("周期已发放额度不能为负数")
	}
	if *cycle.AllocatedQuota > cycle.BudgetQuota || *cycle.OpeningAllocatedQuota > cycle.BudgetQuota {
		return 0, errors.New("周期已发放额度不能超过周期总额度")
	}
	return *cycle.AllocatedQuota, nil
}

func ordinaryAllocatedQuota(tx *gorm.DB, cycle *model.QuotaCycle) (int64, error) {
	allocated, err := cycleAllocatedQuota(cycle)
	if err != nil {
		return 0, err
	}
	temporaryQuotaUsed, err := temporaryQuotaReserveUsed(tx, cycle.Id)
	if err != nil {
		return 0, err
	}
	if temporaryQuotaUsed >= allocated {
		return 0, nil
	}
	return allocated - temporaryQuotaUsed, nil
}

func carriedBalanceExcludedFromBudget(tx *gorm.DB, cycle *model.QuotaCycle) (bool, error) {
	if cycle == nil {
		return false, nil
	}
	var previous model.QuotaCycle
	err := tx.Where("id <> ? AND status = ? AND cycle_end_at <= ?", cycle.Id, model.QuotaCycleStatusClosed, cycle.CycleStartAt).
		Order("cycle_end_at DESC, id DESC").First(&previous).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return previous.BalancePolicy == model.QuotaCycleBalancePolicyCarry &&
		!previous.CarryOverCountsTowardBudget, nil
}

func initializeCycleAllocation(tx *gorm.DB, cycle *model.QuotaCycle, initializedAt int64) error {
	if cycle == nil {
		return errors.New("周期不存在")
	}
	if cycle.Status != model.QuotaCycleStatusScheduled {
		return errors.New("只有已规划周期可以建立期初额度")
	}
	if cycle.AllocatedQuota != nil || cycle.OpeningAllocatedQuota != nil || cycle.AllocationBaselineAt != nil {
		_, err := cycleAllocatedQuota(cycle)
		return err
	}
	excluded, err := carriedBalanceExcludedFromBudget(tx, cycle)
	if err != nil {
		return err
	}
	var opening int64
	if !excluded {
		if err := tx.Unscoped().Model(&model.User{}).
			Select("COALESCE(SUM(CASE WHEN quota > 0 THEN quota ELSE 0 END), 0)").
			Where("quota_whitelist = ?", false).
			Scan(&opening).Error; err != nil {
			return err
		}
	}
	if opening > cycle.BudgetQuota {
		return fmt.Errorf("当前受管账户可用余额合计超过周期总额度，超出金额为 %s", FormatQuota(opening-cycle.BudgetQuota))
	}
	if err := tx.Model(&model.QuotaCycle{}).Where("id = ?", cycle.Id).Updates(map[string]interface{}{
		"opening_allocated_quota": opening,
		"allocated_quota":         opening,
		"allocation_baseline_at":  initializedAt,
	}).Error; err != nil {
		return err
	}
	cycle.OpeningAllocatedQuota = &opening
	cycle.AllocatedQuota = &opening
	cycle.AllocationBaselineAt = &initializedAt
	return nil
}

func applyCycleAllocationDelta(tx *gorm.DB, cycle *model.QuotaCycle, delta int64) (int64, int64, error) {
	before, err := cycleAllocatedQuota(cycle)
	if err != nil {
		return 0, 0, err
	}
	after, err := checkedAdd(before, delta)
	if err != nil {
		return 0, 0, err
	}
	if after < 0 {
		excluded, excludedErr := carriedBalanceExcludedFromBudget(tx, cycle)
		if excludedErr != nil {
			return 0, 0, excludedErr
		}
		if !excluded {
			return 0, 0, errors.New("本次收回超过周期内当前已发放额度")
		}
		after = 0
	}
	if after > cycle.BudgetQuota {
		return 0, 0, fmt.Errorf("本次发放超过周期总额度，超出金额为 %s", FormatQuota(after-cycle.BudgetQuota))
	}
	if err := tx.Model(&model.QuotaCycle{}).Where("id = ?", cycle.Id).Update("allocated_quota", after).Error; err != nil {
		return 0, 0, err
	}
	cycle.AllocatedQuota = &after
	return before, after, nil
}
