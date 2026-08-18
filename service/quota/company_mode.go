package quota

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// SetQuotaWhitelist changes one user's allocation exception status atomically.
func SetQuotaWhitelist(userID int, enabled bool, operator string) error {
	if userID <= 0 {
		return errors.New("用户 ID 不正确")
	}
	companyModeEnabled := model.CompanyQuotaModeEnabled()
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var cycle *model.QuotaCycle
		if companyModeEnabled {
			var err error
			cycle, err = activeCycleInTx(model.LockForUpdate(tx), time.Now().Unix())
			if err != nil {
				return err
			}
		}
		var user model.User
		if err := model.LockForUpdate(tx.Unscoped()).First(&user, userID).Error; err != nil {
			return err
		}
		if user.QuotaWhitelist == enabled {
			return nil
		}
		if companyModeEnabled {
			if cycle == nil {
				return errors.New("当前没有进行中的额度周期，不能修改额度白名单")
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
				if err != nil {
					return err
				}
				if occupied > cycle.BudgetQuota {
					return fmt.Errorf("无法移出白名单：移出后，本期消费与全体受管账户余额合计将超过公司本周期采购总额，超出金额为 %s", FormatQuota(occupied-cycle.BudgetQuota))
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
					return fmt.Errorf("无法移出白名单：移出后，本期消费与全体受管账户余额合计将超过当前阶段公司额度上限，超出金额为 %s", FormatQuota(occupied-stageCap))
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

// ManualAdjustmentConfirmationError reports a permitted stage overage that requires explicit confirmation.
type ManualAdjustmentConfirmationError struct {
	StageCapQuota      int64
	OccupiedAfterQuota int64
	StageOverageQuota  int64
}

func (e *ManualAdjustmentConfirmationError) Error() string {
	return fmt.Sprintf("手工调增后，本期消费与全体受管账户余额合计将超过当前阶段公司额度上限，超出金额为 %s，请确认后重试", FormatQuota(e.StageOverageQuota))
}

// ManualAdjustUserQuota applies one administrator-requested balance delta in company mode
// and records it as an immutable executed plan item.
func ManualAdjustUserQuota(userID int, adjustment int64, reason, operator string, confirmed bool) (*ExecuteResult, error) {
	if !model.CompanyQuotaModeEnabled() {
		return nil, errors.New("公司额度模式未启用")
	}
	if common.BatchUpdateEnabled {
		return nil, errors.New("批量额度更新已启用，请关闭 BATCH_UPDATE_ENABLED 并等待队列落库后重试")
	}
	if userID <= 0 {
		return nil, errors.New("用户 ID 不正确")
	}
	if adjustment == 0 {
		return nil, errors.New("调整额度必须是非零整数")
	}
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > 255 {
		return nil, errors.New("调整原因不得超过255个字符")
	}
	var planID int
	var quotaDeltas map[int]int64
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
			return errors.New("该用户在额度白名单中，不能手工调额；请先移出白名单")
		}
		if user.Status != common.UserStatusEnabled || user.DeletedAt.Valid {
			return errors.New("该用户已停用，不能手工调额")
		}
		target, err := checkedAdd(int64(user.Quota), adjustment)
		if err != nil || target < int64(common.MinQuota) || target > int64(common.MaxQuota) {
			return errors.New("调整后余额超过系统支持的数值范围，请减少调整金额")
		}
		if adjustment < 0 && target < 0 {
			return errors.New("调减额度不能超过用户当前余额")
		}
		spend, err := model.SumQuotaCycleSettlement(tx, cycle.Id, now)
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
		if adjustment > 0 {
			occupied, err = checkedAdd(occupied, adjustment)
			if err != nil {
				return err
			}
			if occupied > cycle.BudgetQuota {
				return fmt.Errorf("本次调增无法执行：调增后，本期消费与全体受管账户余额合计将超过公司本周期采购总额，超出金额为 %s", FormatQuota(occupied-cycle.BudgetQuota))
			}
		}
		stagePercent, err := currentStagePercent(tx, cycle, now)
		if err != nil {
			return err
		}
		if adjustment > 0 && !confirmed {
			stageCap, err := regularStageCap(cycle.BudgetQuota, cycle.RecoveryReserveQuota, stagePercent)
			if err != nil {
				return err
			}
			if occupied > stageCap {
				return &ManualAdjustmentConfirmationError{
					StageCapQuota:      stageCap,
					OccupiedAfterQuota: occupied,
					StageOverageQuota:  occupied - stageCap,
				}
			}
		}
		params, err := common.Marshal(map[string]interface{}{
			"manual": true, "reason": reason,
			"concentration_multiplier_basis_points": cycle.ConcentrationMultiplier,
		})
		if err != nil {
			return err
		}
		plan := model.QuotaPlan{CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: stagePercent, SnapshotAt: now, NextAdjustmentAt: &cycle.CycleEndAt, AlgorithmVersion: cycleAlgorithmVersion(cycle), Parameters: string(params), BudgetQuotaSnapshot: cycle.BudgetQuota, TotalSpendQuota: spend, ManagedBalanceQuota: managed, PlannedDeltaQuota: adjustment, Status: model.QuotaPlanStatusDraft, CreatedAt: now, CreatedBy: operator}
		if err := tx.Create(&plan).Error; err != nil {
			return err
		}
		action := model.QuotaAdjustmentActionIncrease
		if adjustment < 0 {
			action = model.QuotaAdjustmentActionDecrease
		}
		item := model.QuotaItem{PlanId: plan.Id, UserId: user.Id, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email, Action: action, SnapshotBalanceQuota: int64(user.Quota), AdjustmentQuota: adjustment, RetainedQuota: target, CalculationData: string(params), BasisText: reason, LogStatus: model.QuotaNotificationStatusPending, EmailStatus: model.QuotaNotificationStatusPending}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		planID = plan.Id
		quotaDeltas, err = executePlanInTransaction(tx, plan.Id, cycle.Id, operator, now)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	return finishPlanExecution(planID, quotaDeltas), nil
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
