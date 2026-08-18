package quota

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// SetQuotaWhitelist changes one user's allocation exception status atomically.
func SetQuotaWhitelist(userID int, enabled bool, operator string) error {
	if userID <= 0 {
		return errors.New("用户 ID 不正确")
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := model.LockForUpdate(tx.Unscoped()).First(&user, userID).Error; err != nil {
			return err
		}
		if user.QuotaWhitelist == enabled {
			return nil
		}
		if model.CompanyQuotaModeEnabled() {
			cycle, err := activeCycleInTx(tx, time.Now().Unix())
			if err != nil {
				return err
			}
			if cycle == nil {
				return errors.New("当前没有活跃周期")
			}
			if !enabled {
				managed, err := currentManagedBalance(tx)
				if err != nil {
					return err
				}
				spend, err := model.SumQuotaCycleSettlement(tx, cycle.Id, time.Now().Unix())
				if err != nil {
					return err
				}
				occupied, err := checkedAdd(spend, managed)
				if err != nil {
					return err
				}
				occupied, err = checkedAdd(occupied, int64(user.Quota))
				if err != nil || occupied > cycle.BudgetQuota {
					return errors.New("移出白名单后受管余额将超过周期预算")
				}
				stagePercent, err := currentStagePercent(tx, cycle, time.Now().Unix())
				if err != nil {
					return err
				}
				stageCap, err := regularStageCap(cycle.BudgetQuota, cycle.RecoveryReserveQuota, stagePercent)
				if err != nil {
					return err
				}
				if occupied > stageCap {
					return errors.New("移出白名单后受管余额将超过当前阶段上限")
				}
			}
			if err := tx.Model(&model.QuotaPlan{}).Where("cycle_id = ? AND status = ?", cycle.Id, model.QuotaPlanStatusDraft).Updates(map[string]interface{}{
				"status": model.QuotaPlanStatusCancelled, "cancelled_at": time.Now().Unix(), "cancelled_by": operator, "cancel_reason": "白名单成员关系变化",
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

// ManualAdjustUserQuota is the only company-mode entry for administrator
// target-balance writes. It creates and executes one immutable plan item.
func ManualAdjustUserQuota(userID int, target int64, reason, operator string) (*ExecuteResult, error) {
	if !model.CompanyQuotaModeEnabled() {
		return nil, errors.New("公司额度模式未启用")
	}
	if userID <= 0 || target < 0 || target > int64(common.MaxQuota) {
		return nil, errors.New("目标额度不在允许范围内")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 255 {
		return nil, errors.New("调整原因不能为空且不得超过255个字符")
	}
	var planID int
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		cycle, err := activeCycleInTx(tx, time.Now().Unix())
		if err != nil {
			return err
		}
		if cycle == nil {
			return errors.New("当前没有活跃周期")
		}
		var user model.User
		if err := model.LockForUpdate(tx.Unscoped()).First(&user, userID).Error; err != nil {
			return err
		}
		if user.QuotaWhitelist {
			return errors.New("白名单用户不接受额度调配")
		}
		if user.Status != common.UserStatusEnabled || user.DeletedAt.Valid {
			return errors.New("停用用户不能手工调配额度")
		}
		if int64(user.Quota) == target {
			return errors.New("目标额度与当前额度相同")
		}
		spend, err := model.SumQuotaCycleSettlement(tx, cycle.Id, time.Now().Unix())
		if err != nil {
			return err
		}
		managed, err := currentManagedBalance(tx)
		if err != nil {
			return err
		}
		occupied, err := checkedAdd(spend, managed)
		if err != nil {
			return err
		}
		delta := target - int64(user.Quota)
		if delta > 0 {
			occupied, err = checkedAdd(occupied, delta)
			if err != nil || occupied > cycle.BudgetQuota {
				return errors.New("调整后受管余额将超过周期预算")
			}
			stagePercent, err := currentStagePercent(tx, cycle, time.Now().Unix())
			if err != nil {
				return err
			}
			stageCap, err := regularStageCap(cycle.BudgetQuota, cycle.RecoveryReserveQuota, stagePercent)
			if err != nil {
				return err
			}
			if occupied > stageCap {
				return errors.New("调整后受管余额将超过当前阶段上限")
			}
		}
		stagePercent, err := currentStagePercent(tx, cycle, time.Now().Unix())
		if err != nil {
			return err
		}
		now := time.Now().Unix()
		params, err := common.Marshal(map[string]interface{}{"manual": true, "reason": reason})
		if err != nil {
			return err
		}
		plan := model.QuotaPlan{CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: stagePercent, SnapshotAt: now, NextAdjustmentAt: &cycle.CycleEndAt, AlgorithmVersion: cycleAlgorithmVersion(cycle), Parameters: string(params), BudgetQuotaSnapshot: cycle.BudgetQuota, TotalSpendQuota: spend, ManagedBalanceQuota: managed, PlannedDeltaQuota: delta, Status: model.QuotaPlanStatusDraft, CreatedAt: now, CreatedBy: operator}
		if err := tx.Create(&plan).Error; err != nil {
			return err
		}
		action := model.QuotaAdjustmentActionGrant
		if delta < 0 {
			action = model.QuotaAdjustmentActionDecrease
		}
		item := model.QuotaItem{PlanId: plan.Id, UserId: user.Id, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email, Action: action, SnapshotBalanceQuota: int64(user.Quota), AdjustmentQuota: delta, RetainedQuota: target, CalculationData: string(params), BasisText: reason, LogStatus: model.QuotaNotificationStatusPending, EmailStatus: model.QuotaNotificationStatusPending}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		planID = plan.Id
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	return ExecutePlan(planID, operator)
}

func currentStagePercent(tx *gorm.DB, cycle *model.QuotaCycle, now int64) (int, error) {
	schedule := RecommendSchedule(cycle.CycleStartAt, cycle.CycleEndAt, now)
	stagePercent := schedule.Current.Percent * 100
	var executedPercent int
	if err := tx.Model(&model.QuotaPlan{}).
		Where("cycle_id = ? AND status = ?", cycle.Id, model.QuotaPlanStatusExecuted).
		Select("COALESCE(MAX(stage_percent), 0)").Scan(&executedPercent).Error; err != nil {
		return 0, err
	}
	if executedPercent > stagePercent {
		stagePercent = executedPercent
	}
	return stagePercent, nil
}
