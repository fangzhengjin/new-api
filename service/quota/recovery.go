package quota

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecoveryRequestInput is one user-authenticated, idempotent quota request.
type RecoveryRequestInput struct {
	UserID         int
	IdempotencyKey string
	RequestedQuota int64
	Reason         string
}

// RecoveryOverview is the current user's active-cycle recovery state.
type RecoveryOverview struct {
	Cycle            *model.QuotaCycle
	CurrentQuota     int64
	ReserveUsedQuota int64
	ReserveLeftQuota int64
	Requests         []model.QuotaRecoveryRequest
}

// RecoveryRequestResult contains the durable request and any post-commit deliveries.
type RecoveryRequestResult struct {
	Request       model.QuotaRecoveryRequest
	Notifications NotificationSummary
}

func validateRecoveryRequestInput(input RecoveryRequestInput) (RecoveryRequestInput, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.UserID <= 0 {
		return input, errors.New("用户 ID 不正确")
	}
	if len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 64 {
		return input, errors.New("幂等键长度必须为8至64个字符")
	}
	for _, char := range input.IdempotencyKey {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return input, errors.New("幂等键只能包含字母、数字、连字符和下划线")
		}
	}
	if input.RequestedQuota <= 0 || input.RequestedQuota > int64(common.MaxQuota) {
		return input, errors.New("申请额度不在支持范围内")
	}
	if input.Reason == "" || len(input.Reason) > 500 {
		return input, errors.New("申请原因不能为空且不得超过500个字符")
	}
	return input, nil
}

func recoveryReserveUsed(tx *gorm.DB, cycleID int) (int64, error) {
	var used int64
	if err := tx.Model(&model.QuotaRecoveryRequest{}).
		Select("COALESCE(SUM(approved_quota), 0)").
		Where("cycle_id = ? AND status = ?", cycleID, model.QuotaRecoveryRequestStatusExecuted).
		Scan(&used).Error; err != nil {
		return 0, err
	}
	if used < 0 {
		return 0, errors.New("已发放恢复额度不能为负数")
	}
	return used, nil
}

// GetRecoveryOverview returns the current user's policy, capacity and requests.
func GetRecoveryOverview(userID int) (*RecoveryOverview, error) {
	if !model.CompanyQuotaModeEnabled() {
		return nil, errors.New("公司额度模式未启用")
	}
	var overview RecoveryOverview
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		cycle, err := activeCycleInTx(tx, time.Now().Unix())
		if err != nil {
			return err
		}
		if cycle == nil {
			return errors.New("当前没有活跃周期")
		}
		var user model.User
		if err := tx.Unscoped().First(&user, userID).Error; err != nil {
			return err
		}
		if user.QuotaWhitelist {
			return errors.New("白名单用户不参与额度恢复")
		}
		used, err := recoveryReserveUsed(tx, cycle.Id)
		if err != nil {
			return err
		}
		var requests []model.QuotaRecoveryRequest
		if err := tx.Where("user_id = ?", userID).Order("created_at DESC").Limit(50).Find(&requests).Error; err != nil {
			return err
		}
		overview = RecoveryOverview{
			Cycle: cycle, CurrentQuota: int64(user.Quota), ReserveUsedQuota: used,
			ReserveLeftQuota: maxQuota(0, cycle.RecoveryReserveQuota-used), Requests: requests,
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return &overview, err
}

// ListRecoveryRequests returns every pending request plus bounded recent history.
func ListRecoveryRequests() ([]model.QuotaRecoveryRequest, error) {
	var pending []model.QuotaRecoveryRequest
	if err := model.DB.Where("status = ?", model.QuotaRecoveryRequestStatusPending).
		Order("created_at ASC").Find(&pending).Error; err != nil {
		return nil, err
	}
	var history []model.QuotaRecoveryRequest
	if err := model.DB.Where("status <> ?", model.QuotaRecoveryRequestStatusPending).
		Order("created_at DESC").Limit(200).Find(&history).Error; err != nil {
		return nil, err
	}
	return append(pending, history...), nil
}

// SubmitRecoveryRequest creates one request and auto-executes it only when every frozen policy gate passes.
func SubmitRecoveryRequest(input RecoveryRequestInput) (*RecoveryRequestResult, error) {
	if !model.CompanyQuotaModeEnabled() {
		return nil, errors.New("公司额度模式未启用")
	}
	if common.BatchUpdateEnabled {
		return nil, errors.New("批量额度更新已启用，请关闭 BATCH_UPDATE_ENABLED 并等待队列落库后重试")
	}
	input, err := validateRecoveryRequestInput(input)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	var stored model.QuotaRecoveryRequest
	var deltas map[int]int64
	var planID int
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		cycle, err := activeCycleInTx(model.LockForUpdate(tx), now)
		if err != nil {
			return err
		}
		if cycle == nil {
			return errors.New("当前没有活跃周期")
		}
		var user model.User
		if err := model.LockForUpdate(tx.Unscoped()).First(&user, input.UserID).Error; err != nil {
			return err
		}
		if user.QuotaWhitelist {
			return errors.New("白名单用户不参与额度恢复")
		}
		if user.Status != common.UserStatusEnabled || user.DeletedAt.Valid {
			return errors.New("停用用户不能申请额度恢复")
		}
		request := model.QuotaRecoveryRequest{
			CycleId: cycle.Id, UserId: user.Id, Username: user.Username, DisplayName: user.DisplayName,
			IdempotencyKey: input.IdempotencyKey, RequestedQuota: input.RequestedQuota,
			Reason: input.Reason, Status: model.QuotaRecoveryRequestStatusPending, CreatedAt: now,
		}
		create := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "idempotency_key"}}, DoNothing: true,
		}).Create(&request)
		if create.Error != nil {
			return create.Error
		}
		if create.RowsAffected == 0 {
			if err := tx.Where("user_id = ? AND idempotency_key = ?", input.UserID, input.IdempotencyKey).First(&stored).Error; err != nil {
				return err
			}
			if stored.CycleId != cycle.Id || stored.RequestedQuota != input.RequestedQuota || stored.Reason != input.Reason {
				return errors.New("同一幂等键已用于不同的恢复申请")
			}
			return nil
		}
		stored = request
		used, err := recoveryReserveUsed(tx, cycle.Id)
		if err != nil {
			return err
		}
		remaining := cycle.RecoveryReserveQuota - used
		if remaining <= 0 {
			stored.Status = model.QuotaRecoveryRequestStatusRejected
			stored.Decision = model.QuotaRecoveryDecisionAuto
			stored.ReviewReason = "本周期小额恢复池已耗尽，请联系管理员处理"
			stored.ReviewedBy = "system"
			stored.ReviewedAt = &now
			return tx.Model(&model.QuotaRecoveryRequest{}).Where("id = ?", stored.Id).Updates(map[string]interface{}{
				"status": stored.Status, "decision": stored.Decision, "review_reason": stored.ReviewReason,
				"reviewed_by": stored.ReviewedBy, "reviewed_at": now,
			}).Error
		}
		autoCount := int64(0)
		autoQuota := int64(0)
		if err := tx.Model(&model.QuotaRecoveryRequest{}).
			Where("cycle_id = ? AND user_id = ? AND status = ? AND decision = ?", cycle.Id, user.Id, model.QuotaRecoveryRequestStatusExecuted, model.QuotaRecoveryDecisionAuto).
			Count(&autoCount).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.QuotaRecoveryRequest{}).
			Select("COALESCE(SUM(approved_quota), 0)").
			Where("cycle_id = ? AND user_id = ? AND status = ? AND decision = ?", cycle.Id, user.Id, model.QuotaRecoveryRequestStatusExecuted, model.QuotaRecoveryDecisionAuto).
			Scan(&autoQuota).Error; err != nil {
			return err
		}
		autoTotalAfter, addErr := checkedAdd(autoQuota, input.RequestedQuota)
		autoEligible := addErr == nil && cycle.AutoRecoveryEnabled && int64(user.Quota) < cycle.AutoRecoveryThresholdQuota &&
			input.RequestedQuota <= cycle.AutoRecoverySingleQuota && autoCount < int64(cycle.AutoRecoveryMaxCount) &&
			autoTotalAfter <= cycle.AutoRecoveryMaxQuota && input.RequestedQuota <= remaining
		if !autoEligible {
			return nil
		}
		deltas, planID, err = approveRecoveryInTransaction(
			tx, &stored, cycle, user, input.RequestedQuota, "system", model.QuotaRecoveryDecisionAuto,
			"符合周期自动恢复策略", now,
		)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	result := &RecoveryRequestResult{Request: stored}
	if planID != 0 {
		finishRecoveryExecution(deltas)
		result.Notifications = RetryNotifications(planID)
	}
	return result, nil
}

func approveRecoveryInTransaction(tx *gorm.DB, request *model.QuotaRecoveryRequest, cycle *model.QuotaCycle, user model.User, approvedQuota int64, reviewer string, decision model.QuotaRecoveryDecision, reviewReason string, now int64) (map[int]int64, int, error) {
	if approvedQuota <= 0 || approvedQuota > request.RequestedQuota || approvedQuota > int64(common.MaxQuota) {
		return nil, 0, errors.New("批准额度必须大于0且不超过申请额度")
	}
	used, err := recoveryReserveUsed(tx, cycle.Id)
	if err != nil {
		return nil, 0, err
	}
	if approvedQuota > cycle.RecoveryReserveQuota-used {
		return nil, 0, errors.New("本周期小额恢复池剩余额度不足")
	}
	after, err := checkedAdd(int64(user.Quota), approvedQuota)
	if err != nil || after > int64(common.MaxQuota) {
		return nil, 0, errors.New("恢复后用户余额超出支持范围")
	}
	spend, err := model.SumQuotaCycleSettlement(tx, cycle.Id, now)
	if err != nil {
		return nil, 0, err
	}
	managed, err := currentManagedBalance(tx)
	if err != nil {
		return nil, 0, err
	}
	stagePercent, err := currentStagePercent(tx, cycle, now)
	if err != nil {
		return nil, 0, err
	}
	parameters, err := common.Marshal(map[string]interface{}{
		"recovery_request_id": request.Id, "decision": decision, "reason": request.Reason,
		"concentration_multiplier_basis_points": cycle.ConcentrationMultiplier,
	})
	if err != nil {
		return nil, 0, err
	}
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: stagePercent,
		SnapshotAt: now, NextAdjustmentAt: &cycle.CycleEndAt, AlgorithmVersion: cycleAlgorithmVersion(cycle),
		Parameters: string(parameters), BudgetQuotaSnapshot: cycle.BudgetQuota,
		TotalSpendQuota: spend, ManagedBalanceQuota: managed, PlannedDeltaQuota: approvedQuota,
		Status: model.QuotaPlanStatusDraft, CreatedAt: now, CreatedBy: reviewer,
	}
	if err := tx.Create(&plan).Error; err != nil {
		return nil, 0, err
	}
	item := model.QuotaItem{
		PlanId: plan.Id, UserId: user.Id, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
		Action: model.QuotaAdjustmentActionRestore, SnapshotBalanceQuota: int64(user.Quota),
		AdjustmentQuota: approvedQuota, RetainedQuota: after, CalculationData: string(parameters),
		BasisText: fmt.Sprintf("临时额度恢复：%s", request.Reason),
		LogStatus: model.QuotaNotificationStatusPending, EmailStatus: model.QuotaNotificationStatusPending,
	}
	if err := tx.Create(&item).Error; err != nil {
		return nil, 0, err
	}
	deltas, err := executePlanInTransaction(tx, plan.Id, cycle.Id, reviewer, now)
	if err != nil {
		return nil, 0, err
	}
	request.Status = model.QuotaRecoveryRequestStatusExecuted
	request.Decision = decision
	request.ApprovedQuota = approvedQuota
	request.PlanId = &plan.Id
	request.ReviewedBy = reviewer
	request.ReviewReason = reviewReason
	request.ReviewedAt = &now
	request.ExecutedAt = &now
	update := tx.Model(&model.QuotaRecoveryRequest{}).Where("id = ? AND status = ?", request.Id, model.QuotaRecoveryRequestStatusPending).Updates(map[string]interface{}{
		"status": request.Status, "decision": decision, "approved_quota": approvedQuota,
		"plan_id": plan.Id, "reviewed_by": reviewer, "review_reason": reviewReason,
		"reviewed_at": now, "executed_at": now,
	})
	if update.Error != nil {
		return nil, 0, update.Error
	}
	if update.RowsAffected != 1 {
		return nil, 0, errors.New("恢复申请状态已变化")
	}
	return deltas, plan.Id, nil
}

// ApproveRecoveryRequest approves and executes one pending request atomically.
func ApproveRecoveryRequest(requestID int, approvedQuota int64, reviewer, reason string) (*RecoveryRequestResult, error) {
	if !model.CompanyQuotaModeEnabled() {
		return nil, errors.New("公司额度模式未启用")
	}
	if common.BatchUpdateEnabled {
		return nil, errors.New("批量额度更新已启用，请关闭 BATCH_UPDATE_ENABLED 并等待队列落库后重试")
	}
	reason = strings.TrimSpace(reason)
	if requestID <= 0 || reason == "" || len(reason) > 500 {
		return nil, errors.New("审批参数不正确，审批说明不能为空且不得超过500个字符")
	}
	now := time.Now().Unix()
	var request model.QuotaRecoveryRequest
	var deltas map[int]int64
	var planID int
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.LockForUpdate(tx).First(&request, requestID).Error; err != nil {
			return err
		}
		if request.Status != model.QuotaRecoveryRequestStatusPending {
			return errors.New("恢复申请不是待审批状态")
		}
		var cycle model.QuotaCycle
		if err := model.LockForUpdate(tx).First(&cycle, request.CycleId).Error; err != nil {
			return err
		}
		if cycle.Status != model.QuotaCycleStatusActive || now < cycle.CycleStartAt || now >= cycle.CycleEndAt {
			return errors.New("申请所属周期已不可发放")
		}
		var user model.User
		if err := model.LockForUpdate(tx.Unscoped()).First(&user, request.UserId).Error; err != nil {
			return err
		}
		if user.QuotaWhitelist || user.Status != common.UserStatusEnabled || user.DeletedAt.Valid {
			return errors.New("申请用户已不符合额度恢复条件")
		}
		var err error
		deltas, planID, err = approveRecoveryInTransaction(
			tx, &request, &cycle, user, approvedQuota, reviewer,
			model.QuotaRecoveryDecisionManual, reason, now,
		)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	finishRecoveryExecution(deltas)
	return &RecoveryRequestResult{Request: request, Notifications: RetryNotifications(planID)}, nil
}

// RejectRecoveryRequest closes one pending request without changing quota.
func RejectRecoveryRequest(requestID int, reviewer, reason string) (*model.QuotaRecoveryRequest, error) {
	if !model.CompanyQuotaModeEnabled() {
		return nil, errors.New("公司额度模式未启用")
	}
	reason = strings.TrimSpace(reason)
	if requestID <= 0 || reason == "" || len(reason) > 500 {
		return nil, errors.New("拒绝原因不能为空且不得超过500个字符")
	}
	now := time.Now().Unix()
	var request model.QuotaRecoveryRequest
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.LockForUpdate(tx).First(&request, requestID).Error; err != nil {
			return err
		}
		if request.Status != model.QuotaRecoveryRequestStatusPending {
			return errors.New("恢复申请不是待审批状态")
		}
		request.Status = model.QuotaRecoveryRequestStatusRejected
		request.Decision = model.QuotaRecoveryDecisionManual
		request.ReviewedBy = reviewer
		request.ReviewReason = reason
		request.ReviewedAt = &now
		result := tx.Model(&model.QuotaRecoveryRequest{}).
			Where("id = ? AND status = ?", request.Id, model.QuotaRecoveryRequestStatusPending).
			Updates(map[string]interface{}{
				"status": request.Status, "decision": request.Decision, "reviewed_by": reviewer,
				"review_reason": reason, "reviewed_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("恢复申请状态已变化")
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	return &request, err
}

func finishRecoveryExecution(deltas map[int]int64) {
	for userID, delta := range deltas {
		if err := model.SyncUserQuotaCacheDelta(userID, delta); err != nil {
			common.SysError(fmt.Sprintf("failed to sync recovered user %d cache: %v", userID, err))
		}
	}
}
