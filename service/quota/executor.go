package quota

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

const decreaseDraftLifetimeSeconds int64 = 12 * 60 * 60

// ExecuteResult reports the committed balance changes and post-commit deliveries.
type ExecuteResult struct {
	AffectedUsers int                 `json:"affected_users"`
	Notifications NotificationSummary `json:"notifications"`
}

// ExecutePlan atomically applies one reviewed draft, then delivers its durable notifications.
func ExecutePlan(planID int, executedBy string) (*ExecuteResult, error) {
	if common.BatchUpdateEnabled {
		return nil, errors.New("批量额度更新已启用，请关闭 BATCH_UPDATE_ENABLED 并等待队列落库后重试")
	}
	var planReference model.QuotaPlan
	if err := model.DB.Select("cycle_id").First(&planReference, planID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("方案不存在或状态不是 draft")
		}
		return nil, err
	}

	var quotaDeltas map[int]int64
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		quotaDeltas, err = executePlanInTransaction(tx, planID, planReference.CycleId, executedBy, 0)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	for userID, delta := range quotaDeltas {
		if err := model.SyncUserQuotaCacheDelta(userID, delta); err != nil {
			common.SysError(fmt.Sprintf("failed to sync quota-adjusted user %d cache: %v", userID, err))
		}
	}
	notifications := RetryNotifications(planID)
	return &ExecuteResult{AffectedUsers: len(quotaDeltas), Notifications: notifications}, nil
}

func executePlanInTransaction(tx *gorm.DB, planID int, cycleID int, executedBy string, executedAt int64) (map[int]int64, error) {
	var cycle model.QuotaCycle
	if err := model.LockForUpdate(tx).First(&cycle, cycleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("关联周期不存在")
		}
		return nil, err
	}
	var plan model.QuotaPlan
	if err := model.LockForUpdate(tx).
		Where("id = ? AND status = ?", planID, model.QuotaPlanStatusDraft).
		First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("方案不存在或状态不是 draft")
		}
		return nil, err
	}
	if plan.CycleId != cycle.Id {
		return nil, errors.New("方案关联周期已变化")
	}
	if executedAt == 0 {
		executedAt = time.Now().Unix()
	}
	if plan.AlgorithmVersion != cycleAlgorithmVersion(&cycle) {
		return nil, errors.New("该草稿由旧版调配规则生成，请重新生成后再执行")
	}
	if executedAt < plan.SnapshotAt {
		return nil, errors.New("执行时间不能早于快照时间")
	}
	var planParameters PlanParameters
	if strings.TrimSpace(plan.Parameters) != "" {
		if err := common.Unmarshal([]byte(plan.Parameters), &planParameters); err != nil {
			return nil, fmt.Errorf("方案参数无效: %w", err)
		}
	}
	if planParameters.ThoroughRelease && executedAt < finalAdjustmentTime(cycle.CycleStartAt, cycle.CycleEndAt) {
		return nil, errors.New("彻底释放只能在最终调配窗口执行")
	}
	if planParameters.ThoroughRelease &&
		(cycleAlgorithmVersion(&cycle) != CandidateAlgorithmVersion || cycle.RecoveryReserveQuota <= 0) {
		return nil, errors.New("彻底释放需要可用的小额恢复渠道和候选算法")
	}

	var items []model.QuotaItem
	if err := model.LockForUpdate(tx).Where("plan_id = ?", plan.Id).Order("id").Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) == 0 && plan.PlanType != model.QuotaPlanTypeInitialization {
		return nil, errors.New("方案没有调整明细")
	}
	hasDecrease := false
	hasIncrease := false
	hasRestore := false
	restoreOnly := len(items) > 0
	for _, item := range items {
		if item.AdjustmentQuota < 0 {
			hasDecrease = true
		} else if item.AdjustmentQuota > 0 {
			hasIncrease = true
		}
		if item.Action == model.QuotaAdjustmentActionRestore {
			hasRestore = true
		} else {
			restoreOnly = false
		}
	}
	if hasRestore && !restoreOnly {
		return nil, errors.New("恢复方案不能混入普通调配条目")
	}
	if hasDecrease && executedAt-plan.SnapshotAt > decreaseDraftLifetimeSeconds {
		snapshot := time.Unix(plan.SnapshotAt, 0).In(shanghaiLocation).Format("2006-01-02 15:04:05")
		return nil, fmt.Errorf("方案快照已超过12小时（快照时间：%s），请重新生成方案", snapshot)
	}

	if cycle.Status != model.QuotaCycleStatusActive {
		return nil, errors.New("关联周期不是活跃状态，不能执行方案")
	}
	if executedAt > cycle.CycleEndAt {
		return nil, errors.New("当前时间已超过周期结束时间，不得执行跨周期方案")
	}
	if cycle.BudgetQuota <= 0 || plan.StagePercent < 0 || plan.StagePercent > 10_000 {
		return nil, errors.New("周期预算或方案阶段不正确")
	}

	userIDs := make([]int, 0, len(items))
	for _, item := range items {
		userIDs = append(userIDs, item.UserId)
	}
	sort.Ints(userIDs)
	var users []model.User
	if len(userIDs) > 0 {
		if err := model.LockForUpdate(tx.Unscoped()).Where("id IN ?", userIDs).Order("id").Find(&users).Error; err != nil {
			return nil, err
		}
	}
	if len(users) != len(userIDs) {
		return nil, errors.New("方案中的用户已不存在")
	}
	usersByID := make(map[int]model.User, len(users))
	for _, user := range users {
		usersByID[user.Id] = user
	}
	stageCap, err := bigRatio([]int64{cycle.BudgetQuota, int64(plan.StagePercent)}, []int64{10_000}, false)
	if err != nil {
		return nil, err
	}
	if !restoreOnly {
		if plan.PlanType == model.QuotaPlanTypeInitialization {
			stageCap = cycle.BudgetQuota - cycle.RecoveryReserveQuota
			if stageCap < 0 {
				return nil, errors.New("小额恢复池超过周期预算")
			}
		} else {
			stageCap, err = regularStageCap(cycle.BudgetQuota, cycle.RecoveryReserveQuota, plan.StagePercent)
			if err != nil {
				return nil, err
			}
		}
	}
	if hasIncrease {
		totalSpendBefore, err := totalSpendAt(tx, logDatabaseForTransaction(tx), cycle.Id, cycle.CycleStartAt, executedAt)
		if err != nil {
			return nil, err
		}
		managedBalanceBefore, err := currentManagedBalance(tx)
		if err != nil {
			return nil, err
		}
		occupiedBefore, err := checkedAdd(totalSpendBefore, managedBalanceBefore)
		if err != nil {
			return nil, err
		}
		if occupiedBefore > stageCap || occupiedBefore > cycle.BudgetQuota {
			common.SysError(fmt.Sprintf("quota allocation frozen: cycle %d occupied position exceeds current cap", cycle.Id))
			return nil, errors.New("当前受管头寸已超过可用上限，所有正向调配已冻结")
		}
	}

	quotaDeltas := make(map[int]int64, len(items))
	sameLogDatabase := model.LOG_DB == model.DB
	for index := range items {
		item := &items[index]
		user := usersByID[item.UserId]
		if err := validateExecutionItem(*item, user); err != nil {
			return nil, err
		}
		before := int64(user.Quota)
		after, err := checkedAdd(before, item.AdjustmentQuota)
		if err != nil {
			return nil, err
		}
		if after < 0 && item.AdjustmentQuota < 0 {
			return nil, fmt.Errorf("用户 %d 当前余额不足以扣除方案额度", user.Id)
		}
		if after < 0 || after > int64(common.MaxQuota) {
			return nil, fmt.Errorf("用户 %d 调整后余额超出可支持范围", user.Id)
		}
		updateQuery := tx.Unscoped().Model(&model.User{}).Where("id = ?", user.Id)
		if item.AdjustmentQuota < 0 {
			updateQuery = updateQuery.Where("quota >= ?", -item.AdjustmentQuota)
		} else {
			updateQuery = updateQuery.Where("quota <= ?", int64(common.MaxQuota)-item.AdjustmentQuota)
		}
		update := updateQuery.Update("quota", gorm.Expr("quota + ?", item.AdjustmentQuota))
		if update.Error != nil {
			return nil, update.Error
		}
		if update.RowsAffected != 1 {
			if item.AdjustmentQuota < 0 {
				return nil, fmt.Errorf("用户 %d 当前余额不足以扣除方案额度", user.Id)
			}
			return nil, fmt.Errorf("用户 %d 调整后余额超出可支持范围", user.Id)
		}
		quotaDeltas[user.Id] = item.AdjustmentQuota

		logContent := GenerateLogContent(*item, before, after, cycle)
		itemUpdate := map[string]interface{}{
			"actual_before_quota": before,
			"actual_after_quota":  after,
			"log_content":         logContent,
			"log_status":          model.QuotaNotificationStatusPending,
			"log_error":           "",
			"email_status":        model.QuotaNotificationStatusPending,
			"email_error":         "",
		}
		if item.Action == model.QuotaAdjustmentActionReclaim {
			itemUpdate["email_status"] = model.QuotaNotificationStatusSkipped
		}
		if sameLogDatabase {
			requestID := quotaLogRequestID(plan.Id, item.Id)
			if err := tx.Create(&model.Log{
				UserId: item.UserId, Username: item.Username, CreatedAt: executedAt,
				Type: model.LogTypeManage, Content: logContent, RequestId: requestID,
			}).Error; err != nil {
				return nil, err
			}
			itemUpdate["log_status"] = model.QuotaNotificationStatusSent
			itemUpdate["log_sent_at"] = executedAt
		}
		if err := tx.Model(&model.QuotaItem{}).Where("id = ?", item.Id).Updates(itemUpdate).Error; err != nil {
			return nil, err
		}
	}

	totalSpend, err := totalSpendAt(tx, logDatabaseForTransaction(tx), cycle.Id, cycle.CycleStartAt, executedAt)
	if err != nil {
		return nil, err
	}
	managedBalance, err := currentManagedBalance(tx)
	if err != nil {
		return nil, err
	}
	occupied, err := checkedAdd(totalSpend, managedBalance)
	if err != nil {
		return nil, err
	}
	if occupied > stageCap && hasIncrease {
		return nil, fmt.Errorf(
			"阶段额度检查失败：本期已消费 %s + 账户余额 %s = %s 超过当前阶段上限 %s",
			FormatQuota(totalSpend), FormatQuota(managedBalance), FormatQuota(occupied), FormatQuota(stageCap),
		)
	}
	if occupied > cycle.BudgetQuota && hasIncrease {
		return nil, errors.New("计划执行结果超过公司采购总额")
	}

	planUpdate := tx.Model(&model.QuotaPlan{}).
		Where("id = ? AND status = ?", plan.Id, model.QuotaPlanStatusDraft).
		Updates(map[string]interface{}{
			"status": model.QuotaPlanStatusExecuted, "executed_at": executedAt, "executed_by": executedBy,
		})
	if planUpdate.Error != nil {
		return nil, planUpdate.Error
	}
	if planUpdate.RowsAffected != 1 {
		return nil, errors.New("方案状态已变化")
	}
	if err := tx.Model(&model.QuotaPlan{}).
		Where("cycle_id = ? AND status = ? AND id <> ?", plan.CycleId, model.QuotaPlanStatusDraft, plan.Id).
		Updates(map[string]interface{}{
			"status": model.QuotaPlanStatusCancelled, "cancelled_at": executedAt, "cancelled_by": executedBy,
			"cancel_reason": "同周期方案已执行，快照过期自动作废",
		}).Error; err != nil {
		return nil, err
	}
	return quotaDeltas, nil
}

func validateExecutionItem(item model.QuotaItem, user model.User) error {
	if user.Username != item.Username || user.QuotaWhitelist {
		return fmt.Errorf("用户 %d 的身份信息不匹配或不允许参与调配", item.UserId)
	}
	enabled := user.Status == common.UserStatusEnabled && !user.DeletedAt.Valid
	switch item.Action {
	case model.QuotaAdjustmentActionReclaim:
		if enabled || item.AdjustmentQuota >= 0 {
			return fmt.Errorf("用户 %d 的停用回收条件已变化", item.UserId)
		}
	case model.QuotaAdjustmentActionDecrease:
		if !enabled || item.AdjustmentQuota >= 0 {
			return fmt.Errorf("用户 %d 的调减条件已变化", item.UserId)
		}
	case model.QuotaAdjustmentActionIncrease, model.QuotaAdjustmentActionGrant:
		if !enabled || item.AdjustmentQuota <= 0 {
			return fmt.Errorf("用户 %d 的调增条件已变化", item.UserId)
		}
	case model.QuotaAdjustmentActionInitialize:
		if !enabled || item.AdjustmentQuota == 0 {
			return fmt.Errorf("用户 %d 的初始化条件已变化", item.UserId)
		}
	case model.QuotaAdjustmentActionRestore:
		if item.AdjustmentQuota <= 0 {
			return fmt.Errorf("用户 %d 的恢复额度不正确", item.UserId)
		}
	default:
		return fmt.Errorf("用户 %d 的调整动作不正确", item.UserId)
	}
	return nil
}

func logDatabaseForTransaction(tx *gorm.DB) *gorm.DB {
	if model.LOG_DB == model.DB {
		return tx
	}
	return model.LOG_DB
}

func totalSpendAt(tx *gorm.DB, logDB *gorm.DB, cycleID int, cycleStart int64, end int64) (int64, error) {
	if model.CompanyQuotaModeEnabled() {
		return model.SumQuotaCycleSettlement(tx, cycleID, end)
	}
	db := logDB
	if db == nil {
		return 0, errors.New("日志数据库未初始化")
	}
	var total int64
	if err := db.Model(&model.Log{}).
		Select("COALESCE(SUM(quota), 0)").
		Where("type = ? AND created_at >= ? AND created_at <= ?", model.LogTypeConsume, cycleStart, end).
		Scan(&total).Error; err != nil {
		return 0, err
	}
	if total < 0 {
		return 0, errors.New("本期消费总额不能为负数")
	}
	return total, nil
}

func currentManagedBalance(tx *gorm.DB) (int64, error) {
	var total int64
	if err := tx.Unscoped().Model(&model.User{}).
		Select("COALESCE(SUM(quota), 0)").
		Where("quota_whitelist = ? OR quota_whitelist IS NULL", false).
		Scan(&total).Error; err != nil {
		return 0, err
	}
	if total < 0 {
		return 0, errors.New("参与调配账户余额总额不能为负数")
	}
	return total, nil
}

func quotaLogRequestID(planID int, itemID int) string {
	return fmt.Sprintf("quota-plan-%d-item-%d", planID, itemID)
}

// GenerateLogContent produces the user-visible audit text preserved by the source tool.
func GenerateLogContent(item model.QuotaItem, actualBefore int64, actualAfter int64, cycle model.QuotaCycle) string {
	actionNames := map[model.QuotaAdjustmentAction]string{
		model.QuotaAdjustmentActionInitialize: "额度发放",
		model.QuotaAdjustmentActionIncrease:   "额度调增",
		model.QuotaAdjustmentActionDecrease:   "额度调减",
		model.QuotaAdjustmentActionGrant:      "额度补发",
		model.QuotaAdjustmentActionReclaim:    "停用回收",
		model.QuotaAdjustmentActionRestore:    "清零恢复",
	}
	if item.Action == model.QuotaAdjustmentActionRestore && strings.HasPrefix(item.BasisText, "临时额度恢复：") {
		actionNames[item.Action] = "临时额度恢复"
	}
	formatLogDate := func(timestamp int64) string {
		date := time.Unix(timestamp, 0).In(shanghaiLocation)
		return fmt.Sprintf("%d年%d月%d日", date.Year(), date.Month(), date.Day())
	}
	lines := []string{
		"操作类型：" + actionNames[item.Action],
		fmt.Sprintf("额度周期：%s 至 %s", formatLogDate(cycle.CycleStartAt), formatLogDate(cycle.CycleEndAt)),
		"",
		"调整依据：",
	}
	index := 1
	for _, part := range strings.Split(item.BasisText, "；") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %s", index, part))
		index++
	}
	lines = append(lines, "", "调整结果：")
	switch item.Action {
	case model.QuotaAdjustmentActionInitialize:
		if item.AdjustmentQuota > 0 {
			lines = append(lines, fmt.Sprintf("本次发放：%s", FormatQuota(item.AdjustmentQuota)))
		} else if item.AdjustmentQuota < 0 {
			lines = append(lines, fmt.Sprintf("本次调减：%s", FormatQuota(-item.AdjustmentQuota)))
		}
		lines = append(lines, fmt.Sprintf("调整后余额：%s", FormatQuota(actualAfter)))
	case model.QuotaAdjustmentActionReclaim:
		lines = append(lines,
			fmt.Sprintf("调整前余额：%s", FormatQuota(actualBefore)),
			fmt.Sprintf("本次回收：%s", FormatQuota(-item.AdjustmentQuota)),
			fmt.Sprintf("调整后余额：%s", FormatQuota(actualAfter)),
			"", "注：账户恢复使用后，需联系管理员重新核算额度。",
		)
	case model.QuotaAdjustmentActionDecrease:
		lines = append(lines,
			fmt.Sprintf("调整前余额：%s", FormatQuota(actualBefore)),
			fmt.Sprintf("本次调减：%s", FormatQuota(-item.AdjustmentQuota)),
			fmt.Sprintf("调整后余额：%s", FormatQuota(actualAfter)),
			"", "注：如后续用量增加，将在下次额度调配时重新核算。",
		)
	default:
		operation := "调增"
		if item.Action == model.QuotaAdjustmentActionGrant {
			operation = "补发"
		} else if item.Action == model.QuotaAdjustmentActionRestore {
			operation = "恢复"
		}
		lines = append(lines,
			fmt.Sprintf("调整前余额：%s", FormatQuota(actualBefore)),
			fmt.Sprintf("本次%s：%s", operation, FormatQuota(item.AdjustmentQuota)),
			fmt.Sprintf("调整后余额：%s", FormatQuota(actualAfter)),
		)
	}
	return strings.Join(lines, "\n")
}
