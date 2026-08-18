package quota

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"gorm.io/gorm"
)

// LifecycleResult reports durable work completed by one quota lifecycle pass.
type LifecycleResult struct {
	ClosedCycleIDs   []int `json:"closed_cycle_ids"`
	ActivatedCycleID *int  `json:"activated_cycle_id,omitempty"`
	GeneratedPlanID  *int  `json:"generated_plan_id,omitempty"`
	ExecutedPlanID   *int  `json:"executed_plan_id,omitempty"`
}

type cycleSettlementResult struct {
	planID int
	deltas map[int]int64
}

// ValidateQuotaInitializationTime validates the shared Shanghai-time HH:mm setting.
func ValidateQuotaInitializationTime(value string) error {
	parsed, err := time.Parse("15:04", value)
	if err != nil || parsed.Format("15:04") != value {
		return errors.New("初始化方案自动生成时间必须是 HH:mm")
	}
	return nil
}

func initializationDueAt(cycle model.QuotaCycle) (int64, error) {
	value := operation_setting.GetQuotaSetting().QuotaInitializationTime
	if err := ValidateQuotaInitializationTime(value); err != nil {
		return 0, err
	}
	clock, _ := time.Parse("15:04", value)
	start := time.Unix(cycle.CycleStartAt, 0).In(shanghaiLocation)
	due := time.Date(start.Year(), start.Month(), start.Day(), clock.Hour(), clock.Minute(), 0, 0, shanghaiLocation).Unix()
	return maxQuota(due, cycle.CycleStartAt), nil
}

// QuotaLifecycleDue reports whether a scheduled pass can make durable progress.
func QuotaLifecycleDue(now int64) (bool, error) {
	if !model.CompanyQuotaModeEnabled() {
		return false, nil
	}
	var dueCycles int64
	if err := model.DB.Model(&model.QuotaCycle{}).
		Where("status IN ? AND cycle_end_at <= ?", []model.QuotaCycleStatus{
			model.QuotaCycleStatusScheduled, model.QuotaCycleStatusActive,
		}, now).
		Count(&dueCycles).Error; err != nil {
		return false, err
	}
	if dueCycles > 0 {
		return true, nil
	}
	var activeCount int64
	if err := model.DB.Model(&model.QuotaCycle{}).Where("status = ?", model.QuotaCycleStatusActive).Count(&activeCount).Error; err != nil {
		return false, err
	}
	if activeCount == 0 {
		var activatable int64
		if err := model.DB.Model(&model.QuotaCycle{}).
			Where("status = ? AND cycle_start_at <= ? AND cycle_end_at > ?", model.QuotaCycleStatusScheduled, now, now).
			Count(&activatable).Error; err != nil {
			return false, err
		}
		return activatable > 0, nil
	}
	var cycle model.QuotaCycle
	if err := model.DB.Where("status = ?", model.QuotaCycleStatusActive).First(&cycle).Error; err != nil {
		return false, err
	}
	dueAt, err := initializationDueAt(cycle)
	if err != nil || now < dueAt {
		return false, err
	}
	var initializationPlans int64
	if err := model.DB.Model(&model.QuotaPlan{}).
		Where("cycle_id = ? AND plan_type = ?", cycle.Id, model.QuotaPlanTypeInitialization).
		Count(&initializationPlans).Error; err != nil {
		return false, err
	}
	return initializationPlans == 0, nil
}

// RunQuotaLifecycle closes due cycles, activates one due successor, and creates its initialization draft.
func RunQuotaLifecycle(now int64) (*LifecycleResult, error) {
	if !model.CompanyQuotaModeEnabled() {
		return nil, errors.New("公司额度模式未启用")
	}
	if common.BatchUpdateEnabled {
		return nil, errors.New("批量额度更新已启用，请关闭 BATCH_UPDATE_ENABLED 并等待队列落库后重试")
	}
	result := &LifecycleResult{}
	var expired []model.QuotaCycle
	if err := model.DB.Select("id").
		Where("status IN ? AND cycle_end_at <= ?", []model.QuotaCycleStatus{
			model.QuotaCycleStatusScheduled, model.QuotaCycleStatusActive,
		}, now).
		Order("cycle_end_at, id").Find(&expired).Error; err != nil {
		return nil, err
	}
	for _, cycle := range expired {
		if _, err := settleCycle(cycle.Id, now, "system"); err != nil {
			return nil, fmt.Errorf("收口周期 %d 失败: %w", cycle.Id, err)
		}
		result.ClosedCycleIDs = append(result.ClosedCycleIDs, cycle.Id)
	}
	active, activated, err := activateScheduledCycle(now, "system")
	if err != nil {
		return nil, err
	}
	if active == nil {
		return result, nil
	}
	if activated {
		result.ActivatedCycleID = &active.Id
	}
	dueAt, err := initializationDueAt(*active)
	if err != nil {
		return nil, err
	}
	if now < dueAt {
		return result, nil
	}
	planID, executed, err := autoGenerateInitialization(active.Id, now)
	if err != nil {
		return nil, err
	}
	if planID != 0 {
		result.GeneratedPlanID = &planID
		if executed {
			result.ExecutedPlanID = &planID
		}
	}
	return result, nil
}

func activateScheduledCycle(now int64, updatedBy string) (*model.QuotaCycle, bool, error) {
	var active *model.QuotaCycle
	activated := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var current model.QuotaCycle
		err := model.LockForUpdate(tx).Where("status = ?", model.QuotaCycleStatusActive).First(&current).Error
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
		if !next.BalancePolicy.Valid() {
			return errors.New("待激活周期的余额策略不正确")
		}
		spend, err := model.SumQuotaCycleSettlement(tx, next.Id, now)
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
		if occupied > next.BudgetQuota {
			return errors.New("后续周期预算低于激活时受管头寸")
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

func settleCycle(cycleID int, now int64, updatedBy string) (*cycleSettlementResult, error) {
	if common.BatchUpdateEnabled {
		return nil, errors.New("批量额度更新已启用，请关闭 BATCH_UPDATE_ENABLED 并等待队列落库后重试")
	}
	result := &cycleSettlementResult{}
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
		if !cycle.BalancePolicy.Valid() {
			return errors.New("周期余额策略不正确")
		}
		if cycle.Status == model.QuotaCycleStatusActive && cycle.BalancePolicy == model.QuotaCycleBalancePolicyReset {
			planID, deltas, err := createResetSettlementPlan(tx, cycle, now, updatedBy)
			if err != nil {
				return err
			}
			result.planID = planID
			result.deltas = deltas
		}
		updates := map[string]interface{}{
			"status": model.QuotaCycleStatusClosed, "active_key": nil,
			"settled_at": now, "updated_at": now, "updated_by": updatedBy,
		}
		if result.planID != 0 {
			updates["settlement_plan_id"] = result.planID
		}
		if err := tx.Model(&model.QuotaCycle{}).Where("id = ?", cycle.Id).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Model(&model.QuotaPlan{}).
			Where("cycle_id = ? AND status = ?", cycle.Id, model.QuotaPlanStatusDraft).
			Updates(map[string]interface{}{
				"status": model.QuotaPlanStatusCancelled, "cancelled_at": now,
				"cancelled_by": updatedBy, "cancel_reason": "采购周期已收口",
			}).Error
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	for userID, delta := range result.deltas {
		if err := model.SyncUserQuotaCacheDelta(userID, delta); err != nil {
			common.SysError(fmt.Sprintf("failed to sync cycle-settled user %d cache: %v", userID, err))
		}
	}
	if result.planID != 0 {
		RetryNotifications(result.planID)
	}
	return result, nil
}

func createResetSettlementPlan(tx *gorm.DB, cycle model.QuotaCycle, now int64, updatedBy string) (int, map[int]int64, error) {
	var users []model.User
	if err := model.LockForUpdate(tx.Unscoped()).
		Where("quota_whitelist = ? OR quota_whitelist IS NULL", false).
		Order("id").Find(&users).Error; err != nil {
		return 0, nil, err
	}
	managed := int64(0)
	for _, user := range users {
		if user.Quota < 0 {
			return 0, nil, fmt.Errorf("用户 %d 的当前余额不能为负数", user.Id)
		}
		var err error
		managed, err = checkedAdd(managed, int64(user.Quota))
		if err != nil {
			return 0, nil, err
		}
	}
	spend, err := model.SumQuotaCycleSettlement(tx, cycle.Id, now)
	if err != nil {
		return 0, nil, err
	}
	parameterJSON, err := common.Marshal(map[string]interface{}{"settlement_policy": "reset"})
	if err != nil {
		return 0, nil, err
	}
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeSettlement, StagePercent: 10_000,
		SnapshotAt: now, AlgorithmVersion: AlgorithmVersion, Parameters: string(parameterJSON),
		BudgetQuotaSnapshot: cycle.BudgetQuota, TotalSpendQuota: spend,
		ManagedBalanceQuota: managed, PlannedDeltaQuota: -managed,
		Status: model.QuotaPlanStatusExecuted, CreatedAt: now, CreatedBy: updatedBy,
		ExecutedAt: &now, ExecutedBy: updatedBy,
	}
	if err := tx.Create(&plan).Error; err != nil {
		return 0, nil, err
	}
	deltas := make(map[int]int64)
	for _, user := range users {
		before := int64(user.Quota)
		if before == 0 {
			continue
		}
		item := model.QuotaItem{
			PlanId: plan.Id, UserId: user.Id, Username: user.Username,
			DisplayName: user.DisplayName, Email: user.Email,
			Action:               model.QuotaAdjustmentActionDecrease,
			SnapshotBalanceQuota: before, AdjustmentQuota: -before, RetainedQuota: 0,
			CalculationData:   `{"decrease_kind":"cycle_reset"}`,
			BasisText:         "周期到期按清零策略回收全部受管余额",
			ActualBeforeQuota: &before,
			LogStatus:         model.QuotaNotificationStatusPending,
			EmailStatus:       model.QuotaNotificationStatusPending,
		}
		after := int64(0)
		item.ActualAfterQuota = &after
		item.LogContent = GenerateLogContent(item, before, after, cycle)
		if err := tx.Create(&item).Error; err != nil {
			return 0, nil, err
		}
		if model.LOG_DB == model.DB {
			if err := tx.Create(&model.Log{
				UserId: user.Id, Username: user.Username, CreatedAt: now,
				Type: model.LogTypeManage, Content: item.LogContent,
				RequestId: quotaLogRequestID(plan.Id, item.Id),
			}).Error; err != nil {
				return 0, nil, err
			}
			if err := tx.Model(&model.QuotaItem{}).Where("id = ?", item.Id).Updates(map[string]interface{}{
				"log_status": model.QuotaNotificationStatusSent, "log_sent_at": now,
			}).Error; err != nil {
				return 0, nil, err
			}
		}
		update := tx.Unscoped().Model(&model.User{}).Where("id = ?", user.Id).Update("quota", 0)
		if update.Error != nil {
			return 0, nil, update.Error
		}
		if update.RowsAffected != 1 {
			return 0, nil, fmt.Errorf("用户 %d 清零失败", user.Id)
		}
		deltas[user.Id] = -before
	}
	return plan.Id, deltas, nil
}

func autoGenerateInitialization(cycleID int, now int64) (int, bool, error) {
	var planID int
	var deltas map[int]int64
	autoExecute := operation_setting.GetQuotaSetting().AutoExecuteQuotaInitialization
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var cycle model.QuotaCycle
		if err := model.LockForUpdate(tx).First(&cycle, cycleID).Error; err != nil {
			return err
		}
		if cycle.Status != model.QuotaCycleStatusActive || now < cycle.CycleStartAt || now >= cycle.CycleEndAt {
			return errors.New("当前周期不能生成初始化方案")
		}
		var existing int64
		if err := tx.Model(&model.QuotaPlan{}).
			Where("cycle_id = ? AND plan_type = ?", cycle.Id, model.QuotaPlanTypeInitialization).
			Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return nil
		}
		schedule := RecommendSchedule(cycle.CycleStartAt, cycle.CycleEndAt, now)
		nextAdjustment := schedule.Next.Time
		if nextAdjustment <= now {
			nextAdjustment = cycle.CycleEndAt
		}
		generated, err := generatePlanInTransaction(tx, GenerateParams{
			CycleID: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
			StagePercent: 0, NextAdjustmentAt: &nextAdjustment, BasisMode: basisModeActual,
			CreatedBy: "system",
		}, now)
		if err != nil {
			return err
		}
		planID = generated.Plan.Id
		if !autoExecute {
			return nil
		}
		deltas, err = executePlanInTransaction(tx, planID, cycle.Id, "system", now)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, false, err
	}
	for userID, delta := range deltas {
		if err := model.SyncUserQuotaCacheDelta(userID, delta); err != nil {
			common.SysError(fmt.Sprintf("failed to sync auto-initialized user %d cache: %v", userID, err))
		}
	}
	if autoExecute && planID != 0 {
		RetryNotifications(planID)
	}
	return planID, autoExecute && planID != 0, nil
}

// RestoreCycleSettlement reverses one reset snapshot only while every affected balance is unchanged.
func RestoreCycleSettlement(cycleID int, restoredBy string) (*ExecuteResult, error) {
	if !model.CompanyQuotaModeEnabled() {
		return nil, errors.New("公司额度模式未启用")
	}
	if common.BatchUpdateEnabled {
		return nil, errors.New("批量额度更新已启用，请关闭 BATCH_UPDATE_ENABLED 并等待队列落库后重试")
	}
	now := time.Now().Unix()
	var restorePlanID int
	var deltas map[int]int64
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var source model.QuotaCycle
		if err := model.LockForUpdate(tx).First(&source, cycleID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("周期不存在")
			}
			return err
		}
		if source.Status != model.QuotaCycleStatusClosed || source.BalancePolicy != model.QuotaCycleBalancePolicyReset || source.SettlementPlanId == nil {
			return errors.New("该周期没有可恢复的清零快照")
		}
		if source.RestoredAt != nil {
			return errors.New("该周期清零快照已经恢复")
		}
		var target model.QuotaCycle
		if err := model.LockForUpdate(tx).Where("status = ?", model.QuotaCycleStatusActive).First(&target).Error; err != nil {
			return errors.New("恢复前必须存在活跃周期")
		}
		var sourceItems []model.QuotaItem
		if err := model.LockForUpdate(tx).
			Where("plan_id = ?", *source.SettlementPlanId).Order("id").Find(&sourceItems).Error; err != nil {
			return err
		}
		userIDs := make([]int, 0, len(sourceItems))
		for _, item := range sourceItems {
			userIDs = append(userIDs, item.UserId)
		}
		sort.Ints(userIDs)
		var users []model.User
		if len(userIDs) > 0 {
			if err := model.LockForUpdate(tx.Unscoped()).Where("id IN ?", userIDs).Order("id").Find(&users).Error; err != nil {
				return err
			}
		}
		if len(users) != len(userIDs) {
			return errors.New("清零快照中的用户已变化，不能自动恢复")
		}
		usersByID := make(map[int]model.User, len(users))
		for _, user := range users {
			usersByID[user.Id] = user
		}
		restoreTotal := int64(0)
		for _, item := range sourceItems {
			if item.ActualBeforeQuota == nil || item.ActualAfterQuota == nil {
				return errors.New("清零快照不完整")
			}
			user := usersByID[item.UserId]
			if user.QuotaWhitelist || int64(user.Quota) != *item.ActualAfterQuota {
				return fmt.Errorf("用户 %d 的余额或白名单状态已变化，不能自动恢复", user.Id)
			}
			amount := *item.ActualBeforeQuota - *item.ActualAfterQuota
			if amount < 0 {
				return errors.New("清零快照额度不正确")
			}
			var err error
			restoreTotal, err = checkedAdd(restoreTotal, amount)
			if err != nil {
				return err
			}
		}
		spend, err := model.SumQuotaCycleSettlement(tx, target.Id, now)
		if err != nil {
			return err
		}
		managed, err := currentManagedBalance(tx)
		if err != nil {
			return err
		}
		occupiedAfter, err := checkedSum(spend, managed, restoreTotal)
		if err != nil {
			return err
		}
		if occupiedAfter > target.BudgetQuota {
			return errors.New("活跃周期预算不足以恢复清零快照")
		}
		if restoreTotal == 0 {
			return tx.Model(&model.QuotaCycle{}).Where("id = ?", source.Id).Updates(map[string]interface{}{
				"restored_at": now, "restored_by": restoredBy, "updated_at": now, "updated_by": restoredBy,
			}).Error
		}
		stagePercent64, err := bigRatio([]int64{occupiedAfter, 10_000}, []int64{target.BudgetQuota}, true)
		if err != nil {
			return err
		}
		parameters, err := common.Marshal(map[string]interface{}{"restore_cycle_id": source.Id})
		if err != nil {
			return err
		}
		plan := model.QuotaPlan{
			CycleId: target.Id, PlanType: model.QuotaPlanTypeAdjustment,
			StagePercent: int(stagePercent64), SnapshotAt: now, NextAdjustmentAt: &target.CycleEndAt,
			AlgorithmVersion: AlgorithmVersion, Parameters: string(parameters),
			BudgetQuotaSnapshot: target.BudgetQuota, TotalSpendQuota: spend,
			ManagedBalanceQuota: managed, PlannedDeltaQuota: restoreTotal,
			Status: model.QuotaPlanStatusDraft, CreatedAt: now, CreatedBy: restoredBy,
		}
		if err := tx.Create(&plan).Error; err != nil {
			return err
		}
		restorePlanID = plan.Id
		items := make([]model.QuotaItem, 0, len(sourceItems))
		for _, sourceItem := range sourceItems {
			amount := *sourceItem.ActualBeforeQuota - *sourceItem.ActualAfterQuota
			if amount == 0 {
				continue
			}
			user := usersByID[sourceItem.UserId]
			items = append(items, model.QuotaItem{
				PlanId: plan.Id, UserId: user.Id, Username: user.Username,
				DisplayName: user.DisplayName, Email: user.Email,
				Action:               model.QuotaAdjustmentActionRestore,
				SnapshotBalanceQuota: int64(user.Quota), AdjustmentQuota: amount,
				RetainedQuota:   int64(user.Quota) + amount,
				CalculationData: fmt.Sprintf(`{"restore_cycle_id":%d}`, source.Id),
				BasisText:       fmt.Sprintf("恢复周期 #%d 的清零快照", source.Id),
				LogStatus:       model.QuotaNotificationStatusPending,
				EmailStatus:     model.QuotaNotificationStatusPending,
			})
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
		deltas, err = executePlanInTransaction(tx, plan.Id, target.Id, restoredBy, now)
		if err != nil {
			return err
		}
		return tx.Model(&model.QuotaCycle{}).Where("id = ?", source.Id).Updates(map[string]interface{}{
			"restored_at": now, "restored_by": restoredBy, "updated_at": now, "updated_by": restoredBy,
		}).Error
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	for userID, delta := range deltas {
		if err := model.SyncUserQuotaCacheDelta(userID, delta); err != nil {
			common.SysError(fmt.Sprintf("failed to sync cycle-restored user %d cache: %v", userID, err))
		}
	}
	notifications := NotificationSummary{}
	if restorePlanID != 0 {
		notifications = RetryNotifications(restorePlanID)
	}
	return &ExecuteResult{AffectedUsers: len(deltas), Notifications: notifications}, nil
}
