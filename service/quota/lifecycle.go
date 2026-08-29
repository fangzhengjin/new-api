package quota

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// LifecycleResult reports durable work completed by one quota lifecycle pass.
type LifecycleResult struct {
	ClosedCycleIDs   []int `json:"closed_cycle_ids"`
	ActivatedCycleID *int  `json:"activated_cycle_id,omitempty"`
}

// QuotaLifecycleDue reports whether a scheduled pass can make durable progress.
func QuotaLifecycleDue(now int64) (bool, error) {
	if !model.CycleQuotaManagementEnabled() {
		return false, nil
	}
	var cycles []model.QuotaCycle
	if err := model.DB.Where("status <> ?", model.QuotaCycleStatusClosed).
		Order("cycle_start_at, id").Find(&cycles).Error; err != nil {
		return false, err
	}
	for index := range cycles {
		cycle := &cycles[index]
		if cycle.Status == model.QuotaCycleStatusSettling {
			return true, nil
		}
		if cycle.Status == model.QuotaCycleStatusScheduled &&
			(cycle.CycleStartAt <= now || cycle.CycleEndAt <= now) {
			return true, nil
		}
		if cycle.Status == model.QuotaCycleStatusActive && now >= settlementStartsAt(cycle) {
			return true, nil
		}
	}
	return false, nil
}

// RunQuotaLifecycle advances cycles only at their start, settlement and end boundaries.
func RunQuotaLifecycle(now int64) (*LifecycleResult, error) {
	if !model.CycleQuotaManagementEnabled() {
		return nil, errors.New("周期额度管理未启用")
	}
	if err := rejectBatchQuotaMutation(); err != nil {
		return nil, err
	}
	result := &LifecycleResult{}
	var cycles []model.QuotaCycle
	if err := model.DB.Where("status <> ?", model.QuotaCycleStatusClosed).
		Order("cycle_start_at, id").Find(&cycles).Error; err != nil {
		return nil, err
	}
	for index := range cycles {
		cycle := &cycles[index]
		if cycle.Status == model.QuotaCycleStatusScheduled && cycle.CycleEndAt <= now {
			if err := settleCycle(cycle.Id, now, "system"); err != nil {
				return nil, fmt.Errorf("关闭过期周期 %d 失败: %w", cycle.Id, err)
			}
			result.ClosedCycleIDs = append(result.ClosedCycleIDs, cycle.Id)
			continue
		}
		if cycle.Status == model.QuotaCycleStatusActive && now >= settlementStartsAt(cycle) {
			if err := beginCycleSettlement(cycle.Id, now, "system"); err != nil {
				return nil, fmt.Errorf("开始周期 %d 结算失败: %w", cycle.Id, err)
			}
			cycle.Status = model.QuotaCycleStatusSettling
		}
		if cycle.Status != model.QuotaCycleStatusSettling {
			continue
		}
		if err := settleCycle(cycle.Id, now, "system"); err != nil {
			if errors.Is(err, errQuotaCycleHasInFlightWork) {
				continue
			}
			return nil, fmt.Errorf("收口周期 %d 失败: %w", cycle.Id, err)
		}
		result.ClosedCycleIDs = append(result.ClosedCycleIDs, cycle.Id)
	}
	active, activated, err := activateScheduledCycle(now, "system")
	if err != nil {
		return nil, err
	}
	if active != nil && activated {
		result.ActivatedCycleID = &active.Id
		if now >= settlementStartsAt(active) {
			if err := beginCycleSettlement(active.Id, now, "system"); err != nil {
				return nil, err
			}
			if err := settleCycle(active.Id, now, "system"); err != nil {
				if !errors.Is(err, errQuotaCycleHasInFlightWork) {
					return nil, err
				}
			} else {
				result.ClosedCycleIDs = append(result.ClosedCycleIDs, active.Id)
			}
		}
	}
	return result, nil
}

func activateScheduledCycle(now int64, updatedBy string) (*model.QuotaCycle, bool, error) {
	var active *model.QuotaCycle
	activated := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var current model.QuotaCycle
		err := model.LockForUpdate(tx).Where("status IN ?", []model.QuotaCycleStatus{
			model.QuotaCycleStatusActive, model.QuotaCycleStatusSettling,
		}).First(&current).Error
		if err == nil {
			active = &current
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var next model.QuotaCycle
		err = model.LockForUpdate(tx).
			Where("status = ? AND cycle_start_at <= ? AND cycle_end_at > ?", model.QuotaCycleStatusScheduled, now, now).
			Order("cycle_start_at DESC").First(&next).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := initializeCycleAllocation(tx, &next, now); err != nil {
			return err
		}
		update := tx.Model(&model.QuotaCycle{}).
			Where("id = ? AND status = ?", next.Id, model.QuotaCycleStatusScheduled).
			Updates(map[string]interface{}{
				"status": model.QuotaCycleStatusActive, "active_key": 1,
				"updated_at": now, "updated_by": updatedBy,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("待激活周期状态已变化")
		}
		next.SetStatus(model.QuotaCycleStatusActive)
		next.UpdatedAt = now
		next.UpdatedBy = updatedBy
		active = &next
		activated = true
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	return active, activated, err
}

func beginCycleSettlement(cycleID int, now int64, updatedBy string) error {
	quotaAdmissionMu.Lock()
	defer quotaAdmissionMu.Unlock()
	result := model.DB.Model(&model.QuotaCycle{}).
		Where("id = ? AND status = ?", cycleID, model.QuotaCycleStatusActive).
		Updates(map[string]interface{}{
			"status": model.QuotaCycleStatusSettling, "active_key": 1,
			"updated_at": now, "updated_by": updatedBy,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var cycle model.QuotaCycle
		if err := model.DB.Select("status").First(&cycle, cycleID).Error; err != nil {
			return err
		}
		if cycle.Status != model.QuotaCycleStatusSettling && cycle.Status != model.QuotaCycleStatusClosed {
			return errors.New("周期状态已变化")
		}
	}
	return nil
}

func createSettlementPlanInTransaction(tx *gorm.DB, cycle *model.QuotaCycle, now int64, updatedBy string) (*executionCommit, int, error) {
	var users []model.User
	if err := model.LockForUpdate(tx.Unscoped()).
		Where("quota_whitelist = ? AND quota <> 0", false).
		Order("id").Find(&users).Error; err != nil {
		return nil, 0, err
	}
	parameters, err := common.Marshal(map[string]interface{}{"balance_policy": cycle.BalancePolicy})
	if err != nil {
		return nil, 0, err
	}
	allocatedBefore, err := cycleAllocatedQuota(cycle)
	if err != nil {
		return nil, 0, err
	}
	plan := model.QuotaPlan{
		CycleId: cycle.Id, Name: "周期结算", Purpose: "周期到期余额不结转",
		PlanType: model.QuotaPlanTypeSettlement, SnapshotAt: now,
		AlgorithmVersion: AlgorithmVersion, Parameters: string(parameters),
		BudgetQuotaSnapshot: cycle.BudgetQuota, AllocationBeforeQuota: &allocatedBefore,
		Status: model.QuotaPlanStatusDraft, CreatedAt: now, CreatedBy: updatedBy,
	}
	for _, user := range users {
		plan.PlannedDeltaQuota -= int64(user.Quota)
	}
	if err := tx.Create(&plan).Error; err != nil {
		return nil, 0, err
	}
	for _, user := range users {
		item := model.QuotaItem{
			PlanId: plan.Id, UserId: user.Id, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
			Action: model.QuotaAdjustmentActionSettlement, SnapshotBalanceQuota: int64(user.Quota),
			AdjustmentQuota: -int64(user.Quota), CalculationData: string(parameters),
			BasisText: "本周期余额到期不结转", LogStatus: model.QuotaNotificationStatusPending,
			EmailStatus: model.QuotaNotificationStatusSkipped,
		}
		if err := tx.Create(&item).Error; err != nil {
			return nil, 0, err
		}
	}
	commit, err := executePlanInTransaction(tx, plan.Id, cycle.Id, updatedBy, now)
	return commit, plan.Id, err
}

func settleCycle(cycleID int, now int64, updatedBy string) error {
	if err := rejectBatchQuotaMutation(); err != nil {
		return err
	}
	var reference model.QuotaCycle
	if err := model.DB.Select("id", "status", "cycle_start_at", "cycle_end_at", "request_observation_failed_at").First(&reference, cycleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("周期不存在")
		}
		return err
	}
	if reference.Status == model.QuotaCycleStatusClosed {
		return nil
	}
	if reference.Status == model.QuotaCycleStatusSettling {
		hasInFlightWork, err := cycleHasInFlightWork(&reference)
		if err != nil {
			return err
		}
		if hasInFlightWork {
			return errQuotaCycleHasInFlightWork
		}
	}
	var commit *executionCommit
	var planID int
	var balancePolicy model.QuotaCycleBalancePolicy
	closed := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var cycle model.QuotaCycle
		if err := model.LockForUpdate(tx).First(&cycle, cycleID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("周期不存在")
			}
			return err
		}
		if cycle.Status == model.QuotaCycleStatusClosed {
			return nil
		}
		closed = true
		balancePolicy = cycle.BalancePolicy
		if cycle.Status == model.QuotaCycleStatusActive {
			return errors.New("活动周期必须先进入结算状态")
		}
		if cycle.Status == model.QuotaCycleStatusSettling && cycle.BalancePolicy == model.QuotaCycleBalancePolicyReset {
			var err error
			commit, planID, err = createSettlementPlanInTransaction(tx, &cycle, now, updatedBy)
			if err != nil {
				return err
			}
		}
		updates := map[string]interface{}{
			"status": model.QuotaCycleStatusClosed, "active_key": nil,
			"settled_at": now, "updated_at": now, "updated_by": updatedBy,
		}
		if err := tx.Model(&model.QuotaCycle{}).Where("id = ?", cycle.Id).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Model(&model.QuotaPlan{}).
			Where("cycle_id = ? AND status = ?", cycle.Id, model.QuotaPlanStatusDraft).
			Updates(map[string]interface{}{
				"status": model.QuotaPlanStatusCancelled, "cancelled_at": now,
				"cancelled_by": updatedBy, "cancel_reason": "额度周期已关闭",
			}).Error
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	if !closed {
		return nil
	}
	if commit != nil {
		for userID, delta := range commit.QuotaDeltas {
			if cacheErr := model.SyncUserQuotaCacheDelta(userID, delta); cacheErr != nil {
				common.SysError(fmt.Sprintf("failed to sync settled user %d cache: %v", userID, cacheErr))
			}
		}
		RetryNotifications(planID)
	}
	if updatedBy == "system" {
		model.RecordOperationAuditLog(0, fmt.Sprintf("Settled quota cycle %d", cycleID), "", "quota.cycle.settle", map[string]interface{}{
			"cycle_id": cycleID, "balance_policy": balancePolicy,
		}, nil, nil)
	}
	return nil
}

func settleDrainedCycle(cycleID int, now int64) error {
	if err := rejectBatchQuotaMutation(); err != nil {
		return err
	}
	var cycle model.QuotaCycle
	if err := model.DB.Select("status").First(&cycle, cycleID).Error; err != nil {
		return err
	}
	if cycle.Status != model.QuotaCycleStatusSettling {
		return nil
	}
	err := settleCycle(cycleID, now, "system")
	if errors.Is(err, errQuotaCycleHasInFlightWork) {
		return nil
	}
	return err
}
