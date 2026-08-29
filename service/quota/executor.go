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

// ExecuteResult reports the committed balance changes and post-commit deliveries.
type ExecuteResult struct {
	PlanID         int   `json:"-"`
	AffectedUsers  int   `json:"affected_users"`
	IssuedQuota    int64 `json:"-"`
	ReclaimedQuota int64 `json:"-"`
	NetChangeQuota int64 `json:"-"`
	AllocatedQuota int64 `json:"-"`
	RemainingQuota int64 `json:"-"`
}

type executionCommit struct {
	QuotaDeltas    map[int]int64
	IssuedQuota    int64
	ReclaimedQuota int64
	AllocatedQuota int64
	BudgetQuota    int64
}

// ExecutePlan atomically applies one reviewed draft, then delivers its durable notifications.
func ExecutePlan(planID int, executedBy string) (*ExecuteResult, error) {
	if !model.CycleQuotaManagementEnabled() {
		return nil, errors.New("周期额度管理未启用")
	}
	if err := rejectBatchQuotaMutation(); err != nil {
		return nil, err
	}
	var planReference model.QuotaPlan
	if err := model.DB.Select("cycle_id").First(&planReference, planID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("方案不存在或状态不是 draft")
		}
		return nil, err
	}

	var commit *executionCommit
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		commit, err = executePlanInTransaction(tx, planID, planReference.CycleId, executedBy, 0)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	return finishPlanExecution(planID, commit), nil
}

func finishPlanExecution(planID int, commit *executionCommit) *ExecuteResult {
	for userID, delta := range commit.QuotaDeltas {
		if err := model.SyncUserQuotaCacheDelta(userID, delta); err != nil {
			common.SysError(fmt.Sprintf("failed to sync quota-adjusted user %d cache: %v", userID, err))
		}
	}
	RetryNotifications(planID)
	return &ExecuteResult{
		PlanID: planID, AffectedUsers: len(commit.QuotaDeltas), IssuedQuota: commit.IssuedQuota,
		ReclaimedQuota: commit.ReclaimedQuota, NetChangeQuota: commit.IssuedQuota - commit.ReclaimedQuota,
		AllocatedQuota: commit.AllocatedQuota, RemainingQuota: commit.BudgetQuota - commit.AllocatedQuota,
	}
}

func executePlanInTransaction(tx *gorm.DB, planID int, cycleID int, executedBy string, executedAt int64) (*executionCommit, error) {
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
	if plan.AlgorithmVersion != AlgorithmVersion {
		return nil, errors.New("该草稿的调配规则版本与当前版本不一致，请重新生成后再执行")
	}
	if executedAt < plan.SnapshotAt {
		return nil, errors.New("执行时间不能早于快照时间")
	}
	var planParameters PlanParameters
	if err := common.Unmarshal([]byte(plan.Parameters), &planParameters); err != nil {
		return nil, fmt.Errorf("方案参数无效: %w", err)
	}
	if plan.PlanType != model.QuotaPlanTypeSettlement && planParameters.ConcentrationMultiplier != cycle.ConcentrationMultiplier {
		return nil, errors.New("方案记录的自动分配上限与周期设置不一致，请重新生成")
	}

	var items []model.QuotaItem
	if err := model.LockForUpdate(tx).Where("plan_id = ?", plan.Id).Order("id").Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) == 0 && plan.PlanType != model.QuotaPlanTypeInitialization && plan.PlanType != model.QuotaPlanTypeSettlement {
		return nil, errors.New("方案没有调整明细")
	}
	hasTemporaryGrant := false
	temporaryGrantOnly := len(items) > 0
	allocationDelta := int64(0)
	issuedQuota := int64(0)
	reclaimedQuota := int64(0)
	for _, item := range items {
		if (plan.PlanType == model.QuotaPlanTypeSettlement) != (item.Action == model.QuotaAdjustmentActionSettlement) {
			return nil, errors.New("周期结算方案不能混入普通调配条目")
		}
		var err error
		allocationDelta, err = checkedAdd(allocationDelta, item.AdjustmentQuota)
		if err != nil {
			return nil, err
		}
		if item.AdjustmentQuota > 0 {
			issuedQuota, err = checkedAdd(issuedQuota, item.AdjustmentQuota)
		} else if item.AdjustmentQuota < 0 {
			reclaimedQuota, err = checkedAdd(reclaimedQuota, -item.AdjustmentQuota)
		}
		if err != nil {
			return nil, err
		}
		if item.Action == model.QuotaAdjustmentActionTemporaryGrant {
			hasTemporaryGrant = true
		} else {
			temporaryGrantOnly = false
		}
	}
	if hasTemporaryGrant && !temporaryGrantOnly {
		return nil, errors.New("临时额度方案不能混入普通调配条目")
	}
	if temporaryGrantOnly {
		if planParameters.TemporaryQuotaRequestID <= 0 {
			return nil, errors.New("临时额度申请参数不正确")
		}
		var request model.TemporaryQuotaRequest
		if err := tx.Where("id = ? AND cycle_id = ? AND status = ?", planParameters.TemporaryQuotaRequestID, cycle.Id, model.TemporaryQuotaRequestStatusPending).
			First(&request).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("临时额度申请不存在或状态已变化")
			}
			return nil, err
		}
	}

	if plan.PlanType == model.QuotaPlanTypeSettlement {
		if cycle.Status != model.QuotaCycleStatusSettling {
			return nil, errors.New("关联周期不是结算状态，不能执行结算方案")
		}
	} else if cycle.Status != model.QuotaCycleStatusActive {
		return nil, errors.New("关联周期不是活跃状态，不能执行方案")
	}
	if plan.PlanType != model.QuotaPlanTypeSettlement && executedAt >= cycle.CycleEndAt {
		return nil, errors.New("当前时间已到或超过周期结束时间，不得执行跨周期方案")
	}
	if cycle.BudgetQuota <= 0 {
		return nil, errors.New("周期总额度不正确")
	}
	allocationBeforeCheck, err := cycleAllocatedQuota(&cycle)
	if err != nil {
		return nil, err
	}
	ordinaryLimit := cycle.BudgetQuota - cycle.TemporaryQuotaReserve
	if plan.PlanType != model.QuotaPlanTypeSettlement && cycle.TemporaryQuotaReserve > 0 && !temporaryGrantOnly && allocationDelta > 0 {
		ordinaryBefore, err := ordinaryAllocatedQuota(tx, &cycle)
		if err != nil {
			return nil, err
		}
		ordinaryAfter, err := checkedAdd(ordinaryBefore, allocationDelta)
		if err != nil {
			return nil, err
		}
		if ordinaryAfter > ordinaryLimit {
			return nil, errors.New("本次方案会占用临时额度预留，请重新计算方案")
		}
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
	if plan.PlanType == model.QuotaPlanTypeAdjustment && !planParameters.Manual && !temporaryGrantOnly && planParameters.ObservationStartAt > 0 {
		managedUsers, err := loadManagedUsers(tx)
		if err != nil {
			return nil, err
		}
		population := 0
		for _, user := range managedUsers {
			if user.Status == common.UserStatusEnabled {
				population++
			}
		}
		positionCeiling, err := concentrationPositionCeiling(ordinaryLimit, population, cycle.ConcentrationMultiplier)
		if err != nil {
			return nil, err
		}
		logDB := model.LOG_DB
		if logDB == model.DB {
			logDB = tx
		}
		liveStats, err := loadSpendSnapshot(logDB, managedUsers, cycle.CycleStartAt, cycle.CycleStartAt, executedAt)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.Action != model.QuotaAdjustmentActionIncrease && item.Action != model.QuotaAdjustmentActionGrant {
				continue
			}
			user := usersByID[item.UserId]
			positionAfter, err := checkedSum(liveStats[item.UserId].PeriodSpend, int64(user.Quota), item.AdjustmentQuota)
			if err != nil {
				return nil, err
			}
			if positionAfter > positionCeiling {
				return nil, fmt.Errorf("用户 %d 的当前余额或本期消费已使自动调增方案过期，请重新生成", item.UserId)
			}
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
		if after < -int64(common.MaxWalletQuota) || after > int64(common.MaxWalletQuota) {
			return nil, fmt.Errorf("用户 %d 调整后余额超出可支持范围", user.Id)
		}
		updateQuery := tx.Unscoped().Model(&model.User{}).Where("id = ?", user.Id)
		if item.AdjustmentQuota < 0 {
			updateQuery = updateQuery.Where("quota >= ?", -item.AdjustmentQuota)
		} else {
			if item.AdjustmentQuota > 0 {
				updateQuery = updateQuery.Where("quota <= ?", int64(common.MaxWalletQuota)-item.AdjustmentQuota)
			}
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
		if item.Action == model.QuotaAdjustmentActionReclaim || item.Action == model.QuotaAdjustmentActionSettlement {
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

	allocationChange := allocationDelta
	if plan.PlanType == model.QuotaPlanTypeSettlement {
		allocationChange = -allocationBeforeCheck
	}
	allocationBefore, allocationAfter, err := applyCycleAllocationDelta(tx, &cycle, allocationChange)
	if err != nil {
		return nil, err
	}

	planUpdate := tx.Model(&model.QuotaPlan{}).
		Where("id = ? AND status = ?", plan.Id, model.QuotaPlanStatusDraft).
		Updates(map[string]interface{}{
			"status": model.QuotaPlanStatusExecuted, "executed_at": executedAt, "executed_by": executedBy,
			"allocation_before_quota": allocationBefore, "allocation_after_quota": allocationAfter,
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
	return &executionCommit{
		QuotaDeltas: quotaDeltas, IssuedQuota: issuedQuota, ReclaimedQuota: reclaimedQuota,
		AllocatedQuota: allocationAfter, BudgetQuota: cycle.BudgetQuota,
	}, nil
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
	case model.QuotaAdjustmentActionTemporaryGrant:
		if !enabled || item.AdjustmentQuota <= 0 {
			return fmt.Errorf("用户 %d 的临时额度发放条件已变化", item.UserId)
		}
	case model.QuotaAdjustmentActionSettlement:
		if item.AdjustmentQuota == 0 || int64(user.Quota)+item.AdjustmentQuota != 0 {
			return fmt.Errorf("用户 %d 的周期结算余额已变化", item.UserId)
		}
	default:
		return fmt.Errorf("用户 %d 的调整动作不正确", item.UserId)
	}
	return nil
}

func quotaLogRequestID(planID int, itemID int) string {
	return fmt.Sprintf("quota-plan-%d-item-%d", planID, itemID)
}

// GenerateLogContent produces the user-visible quota adjustment audit text.
func GenerateLogContent(item model.QuotaItem, actualBefore int64, actualAfter int64, cycle model.QuotaCycle) string {
	actionNames := map[model.QuotaAdjustmentAction]string{
		model.QuotaAdjustmentActionInitialize:     "额度发放",
		model.QuotaAdjustmentActionIncrease:       "额度调增",
		model.QuotaAdjustmentActionDecrease:       "额度调减",
		model.QuotaAdjustmentActionGrant:          "额度补发",
		model.QuotaAdjustmentActionReclaim:        "停用回收",
		model.QuotaAdjustmentActionTemporaryGrant: "临时额度发放",
		model.QuotaAdjustmentActionSettlement:     "周期结算",
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
	for _, part := range strings.Split(item.BasisText, "\n") {
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
			"", "注：账户重新启用后，需联系管理员重新核算额度。",
		)
	case model.QuotaAdjustmentActionDecrease:
		lines = append(lines,
			fmt.Sprintf("调整前余额：%s", FormatQuota(actualBefore)),
			fmt.Sprintf("本次调减：%s", FormatQuota(-item.AdjustmentQuota)),
			fmt.Sprintf("调整后余额：%s", FormatQuota(actualAfter)),
			"", "注：如后续用量增加，将在下次额度调配时重新核算。",
		)
	case model.QuotaAdjustmentActionSettlement:
		lines = append(lines,
			fmt.Sprintf("调整前余额：%s", FormatQuota(actualBefore)),
			fmt.Sprintf("本次结算：%s", FormatQuota(item.AdjustmentQuota)),
			fmt.Sprintf("调整后余额：%s", FormatQuota(actualAfter)),
		)
	case model.QuotaAdjustmentActionTemporaryGrant:
		lines = append(lines,
			fmt.Sprintf("调整前余额：%s", FormatQuota(actualBefore)),
			fmt.Sprintf("本次临时额度发放：%s", FormatQuota(item.AdjustmentQuota)),
			fmt.Sprintf("调整后余额：%s", FormatQuota(actualAfter)),
		)
	default:
		operation := "调增"
		amount := item.AdjustmentQuota
		if item.Action == model.QuotaAdjustmentActionGrant {
			operation = "补发"
		}
		lines = append(lines,
			fmt.Sprintf("调整前余额：%s", FormatQuota(actualBefore)),
			fmt.Sprintf("本次%s：%s", operation, FormatQuota(amount)),
			fmt.Sprintf("调整后余额：%s", FormatQuota(actualAfter)),
		)
	}
	return strings.Join(lines, "\n")
}
