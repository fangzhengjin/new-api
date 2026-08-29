package quota

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

var errQuotaBatchUpdateEnabled = errors.New("批量额度更新已启用，请关闭 BATCH_UPDATE_ENABLED 并等待队列落库后重试")

func rejectBatchQuotaMutation() error {
	if common.BatchUpdateEnabled {
		return errQuotaBatchUpdateEnabled
	}
	return nil
}

// SetQuotaWhitelist changes one user's allocation exception status atomically.
func SetQuotaWhitelist(userID int, enabled bool, operator string) error {
	if userID <= 0 {
		return errors.New("用户 ID 不正确")
	}
	if !model.CycleQuotaManagementEnabled() {
		return errors.New("周期额度管理未启用，不能修改额度白名单")
	}
	if err := rejectBatchQuotaMutation(); err != nil {
		return err
	}
	now := time.Now().Unix()
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var cycle model.QuotaCycle
		cycleErr := model.LockForUpdate(tx).Where("status IN ?", []model.QuotaCycleStatus{
			model.QuotaCycleStatusActive, model.QuotaCycleStatusSettling,
		}).First(&cycle).Error
		if cycleErr != nil && !errors.Is(cycleErr, gorm.ErrRecordNotFound) {
			return cycleErr
		}
		var user model.User
		if err := model.LockForUpdate(tx.Unscoped()).First(&user, userID).Error; err != nil {
			return err
		}
		if user.QuotaWhitelist == enabled {
			return nil
		}
		if cycleErr == nil {
			balance := max(int64(user.Quota), 0)
			allocationDelta := balance
			if enabled {
				allocationDelta = -balance
			}
			if _, _, err := applyCycleAllocationDelta(tx, &cycle, allocationDelta); err != nil {
				return err
			}
			if err := tx.Model(&model.QuotaPlan{}).Where("cycle_id = ? AND status = ?", cycle.Id, model.QuotaPlanStatusDraft).Updates(map[string]interface{}{
				"status": model.QuotaPlanStatusCancelled, "cancelled_at": now, "cancelled_by": operator, "cancel_reason": "额度管理范围发生变化",
			}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.User{}).Where("id = ?", userID).Update("quota_whitelist", enabled).Error
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	return err
}

func activeCycleInTx(tx *gorm.DB, now int64) (*model.QuotaCycle, error) {
	var cycle model.QuotaCycle
	err := tx.Where("status = ? AND cycle_start_at <= ? AND cycle_end_at > ?", model.QuotaCycleStatusActive, now, now).First(&cycle).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &cycle, err
}

// ManualAdjustUserQuota applies one administrator-requested balance delta during cycle quota management
// and records it as an immutable executed plan item.
func ManualAdjustUserQuota(userID int, adjustment int64, reason, operator string) (*ExecuteResult, error) {
	if !model.CycleQuotaManagementEnabled() {
		return nil, errors.New("周期额度管理未启用")
	}
	if err := rejectBatchQuotaMutation(); err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, errors.New("用户 ID 不正确")
	}
	if adjustment == 0 {
		return nil, errors.New("调整额度必须是非零整数")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, errors.New("调整原因不能为空")
	}
	if utf8.RuneCountInString(reason) > 255 {
		return nil, errors.New("调整原因不得超过255个字符")
	}
	var planID int
	var commit *executionCommit
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		cycle, err := activeCycleInTx(model.LockForUpdate(tx), now)
		if err != nil {
			return err
		}
		if cycle == nil {
			return errors.New("当前没有进行中的额度周期，不能手工调额")
		}
		var user model.User
		if err := model.LockForUpdate(tx.Unscoped()).First(&user, userID).Error; err != nil {
			return err
		}
		if user.QuotaWhitelist {
			return errors.New("该用户在额度白名单中，不能手工调额，请先移出白名单")
		}
		if user.Status != common.UserStatusEnabled || user.DeletedAt.Valid {
			return errors.New("该用户已停用，不能手工调额")
		}
		target, err := checkedAdd(int64(user.Quota), adjustment)
		if err != nil || target < -int64(common.MaxWalletQuota) || target > int64(common.MaxWalletQuota) {
			return errors.New("调整后余额超过系统支持的数值范围，请减少调整金额")
		}
		if adjustment < 0 && target < 0 {
			return errors.New("调减额度不能超过用户当前余额")
		}
		allocatedBefore, err := cycleAllocatedQuota(cycle)
		if err != nil {
			return err
		}
		params, err := common.Marshal(map[string]interface{}{
			"manual": true, "reason": reason,
			"concentration_multiplier_basis_points": cycle.ConcentrationMultiplier,
		})
		if err != nil {
			return err
		}
		plan := model.QuotaPlan{CycleId: cycle.Id, Name: "管理员调整", Purpose: reason, PlanType: model.QuotaPlanTypeAdjustment, SnapshotAt: now, NextAdjustmentAt: &cycle.CycleEndAt, AlgorithmVersion: AlgorithmVersion, Parameters: string(params), BudgetQuotaSnapshot: cycle.BudgetQuota, PlannedDeltaQuota: adjustment, AllocationBeforeQuota: &allocatedBefore, Status: model.QuotaPlanStatusDraft, CreatedAt: now, CreatedBy: operator}
		if err := tx.Create(&plan).Error; err != nil {
			return err
		}
		action := model.QuotaAdjustmentActionIncrease
		if adjustment < 0 {
			action = model.QuotaAdjustmentActionDecrease
		}
		item := model.QuotaItem{PlanId: plan.Id, UserId: user.Id, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email, Action: action, SnapshotBalanceQuota: int64(user.Quota), AdjustmentQuota: adjustment, CalculationData: string(params), BasisText: reason, LogStatus: model.QuotaNotificationStatusPending, EmailStatus: model.QuotaNotificationStatusPending}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		planID = plan.Id
		commit, err = executePlanInTransaction(tx, plan.Id, cycle.Id, operator, now)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	return finishPlanExecution(planID, commit), nil
}
