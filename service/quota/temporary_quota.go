package quota

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TemporaryQuotaRequestInput is one user-authenticated, idempotent quota request.
type TemporaryQuotaRequestInput struct {
	UserID         int
	IdempotencyKey string
	RequestedQuota int64
	Project        string
	Reason         string
}

// TemporaryQuotaOverview is the current user's temporary-quota application state.
type TemporaryQuotaOverview struct {
	CurrentQuota      int64
	CanRequest        bool
	UnavailableReason string
	Projects          []string
}

// TemporaryQuotaRequestEligible reports menu eligibility without loading application details.
func TemporaryQuotaRequestEligible(user *model.User) (bool, error) {
	if !model.CycleQuotaManagementEnabled() {
		return false, nil
	}
	cycle, err := activeCycleInTx(model.DB, time.Now().Unix())
	if err != nil {
		return false, err
	}
	return temporaryQuotaIneligibleReason(user, cycle) == "", nil
}

func temporaryQuotaIneligibleReason(user *model.User, cycle *model.QuotaCycle) string {
	if cycle == nil {
		return "No active cycle"
	}
	if user.QuotaWhitelist {
		return "Whitelist users cannot request temporary quota"
	}
	if user.Status != common.UserStatusEnabled || user.DeletedAt.Valid {
		return "This account cannot request temporary quota"
	}
	return ""
}

func validateTemporaryQuotaRequestInput(input TemporaryQuotaRequestInput) (TemporaryQuotaRequestInput, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Project = strings.TrimSpace(input.Project)
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
	if input.RequestedQuota <= 0 || input.RequestedQuota > int64(common.MaxWalletQuota) {
		return input, errors.New("申请额度不在支持范围内")
	}
	if input.Project == "" || utf8.RuneCountInString(input.Project) > 100 {
		return input, errors.New("申请项目不能为空且不得超过100个字符")
	}
	if !operation_setting.GetQuotaSetting().TemporaryQuotaProjects[input.Project] {
		return input, errors.New("申请项目不可选，请刷新后重试")
	}
	if input.Reason == "" || utf8.RuneCountInString(input.Reason) > 500 {
		return input, errors.New("申请原因不能为空且不得超过500个字符")
	}
	return input, nil
}

func temporaryQuotaReserveUsed(tx *gorm.DB, cycleID int) (int64, error) {
	var used int64
	if err := tx.Model(&model.TemporaryQuotaRequest{}).
		Select("COALESCE(SUM(approved_quota), 0)").
		Where("cycle_id = ? AND status = ?", cycleID, model.TemporaryQuotaRequestStatusExecuted).
		Scan(&used).Error; err != nil {
		return 0, err
	}
	if used < 0 {
		return 0, errors.New("已发放临时额度不能为负数")
	}
	return used, nil
}

func temporaryQuotaReserveUsedAt(tx *gorm.DB, cycleID int, snapshotAt int64, planID int) (int64, error) {
	var used int64
	if err := tx.Model(&model.TemporaryQuotaRequest{}).
		Select("COALESCE(SUM(approved_quota), 0)").
		Where("cycle_id = ? AND status = ? AND (executed_at < ? OR (executed_at = ? AND plan_id < ?))",
			cycleID, model.TemporaryQuotaRequestStatusExecuted, snapshotAt, snapshotAt, planID).
		Scan(&used).Error; err != nil {
		return 0, err
	}
	if used < 0 {
		return 0, errors.New("已发放临时额度不能为负数")
	}
	return used, nil
}

// GetTemporaryQuotaOverview returns user-visible application availability without approval rules.
func GetTemporaryQuotaOverview(userID int) (*TemporaryQuotaOverview, error) {
	if !model.CycleQuotaManagementEnabled() {
		return nil, errors.New("周期额度管理未启用")
	}
	if userID <= 0 {
		return nil, errors.New("用户 ID 不正确")
	}
	projects := make([]string, 0)
	for name, enabled := range operation_setting.GetQuotaSetting().TemporaryQuotaProjects {
		if enabled {
			projects = append(projects, name)
		}
	}
	sort.Strings(projects)
	var overview TemporaryQuotaOverview
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Unscoped().First(&user, userID).Error; err != nil {
			return err
		}
		overview.CurrentQuota = int64(user.Quota)
		overview.Projects = projects
		cycle, err := activeCycleInTx(tx, time.Now().Unix())
		if err != nil {
			return err
		}
		if reason := temporaryQuotaIneligibleReason(&user, cycle); reason != "" {
			overview.UnavailableReason = reason
			return nil
		}
		used, err := temporaryQuotaReserveUsed(tx, cycle.Id)
		if err != nil {
			return err
		}
		if cycle.TemporaryQuotaReserve-used <= 0 {
			overview.UnavailableReason = "Temporary quota reserve is fully used"
			return nil
		}
		if len(projects) == 0 {
			overview.UnavailableReason = "No temporary quota projects are available"
			return nil
		}
		overview.CanRequest = true
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return &overview, err
}

// ListTemporaryQuotaRequestsForUser returns one authenticated user's paginated history.
func ListTemporaryQuotaRequestsForUser(userID, startIdx, pageSize int, status, keyword string) ([]model.TemporaryQuotaRequest, int64, error) {
	if !model.CycleQuotaManagementEnabled() {
		return nil, 0, errors.New("周期额度管理未启用")
	}
	if userID <= 0 || startIdx < 0 || pageSize <= 0 || pageSize > 100 {
		return nil, 0, errors.New("查询参数不正确")
	}
	status = strings.TrimSpace(status)
	if status != "" && status != string(model.TemporaryQuotaRequestStatusPending) &&
		status != string(model.TemporaryQuotaRequestStatusExecuted) && status != string(model.TemporaryQuotaRequestStatusRejected) {
		return nil, 0, errors.New("申请状态不正确")
	}
	keyword = strings.TrimSpace(keyword)
	if utf8.RuneCountInString(keyword) > 100 {
		return nil, 0, errors.New("搜索内容不得超过100个字符")
	}
	query := model.DB.Model(&model.TemporaryQuotaRequest{}).Where("user_id = ?", userID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if keyword != "" {
		pattern := "%" + keyword + "%"
		query = query.Where("project LIKE ? OR reason LIKE ?", pattern, pattern)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var requests []model.TemporaryQuotaRequest
	if err := query.Order("created_at DESC, id DESC").Offset(startIdx).Limit(pageSize).Find(&requests).Error; err != nil {
		return nil, 0, err
	}
	return requests, total, nil
}

// ListTemporaryQuotaRequests returns every pending request plus bounded recent history.
func ListTemporaryQuotaRequests() ([]model.TemporaryQuotaRequest, error) {
	var pending []model.TemporaryQuotaRequest
	if err := model.DB.Where("status = ?", model.TemporaryQuotaRequestStatusPending).
		Order("created_at ASC").Find(&pending).Error; err != nil {
		return nil, err
	}
	var history []model.TemporaryQuotaRequest
	if err := model.DB.Where("status <> ?", model.TemporaryQuotaRequestStatusPending).
		Order("created_at DESC").Limit(200).Find(&history).Error; err != nil {
		return nil, err
	}
	return append(pending, history...), nil
}

// SubmitTemporaryQuotaRequest creates one request and auto-executes it only when every frozen policy gate passes.
func SubmitTemporaryQuotaRequest(input TemporaryQuotaRequestInput) (*model.TemporaryQuotaRequest, error) {
	if !model.CycleQuotaManagementEnabled() {
		return nil, errors.New("周期额度管理未启用")
	}
	if err := rejectBatchQuotaMutation(); err != nil {
		return nil, err
	}
	input, err := validateTemporaryQuotaRequestInput(input)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	var stored model.TemporaryQuotaRequest
	var commit *executionCommit
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
			return errors.New("白名单用户不参与临时额度申请")
		}
		if user.Status != common.UserStatusEnabled || user.DeletedAt.Valid {
			return errors.New("停用用户不能申请临时额度")
		}
		request := model.TemporaryQuotaRequest{
			CycleId: cycle.Id, UserId: user.Id, Username: user.Username, DisplayName: user.DisplayName,
			IdempotencyKey: input.IdempotencyKey, RequestedQuota: input.RequestedQuota, Project: input.Project,
			Reason: input.Reason, Status: model.TemporaryQuotaRequestStatusPending, CreatedAt: now,
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
			if stored.CycleId != cycle.Id || stored.RequestedQuota != input.RequestedQuota || stored.Project != input.Project || stored.Reason != input.Reason {
				return errors.New("同一幂等键已用于不同的临时额度申请")
			}
			return nil
		}
		stored = request
		used, err := temporaryQuotaReserveUsed(tx, cycle.Id)
		if err != nil {
			return err
		}
		remaining := cycle.TemporaryQuotaReserve - used
		if remaining <= 0 {
			return errors.New("临时额度预留已用完")
		}
		autoCount := int64(0)
		autoQuota := int64(0)
		if err := tx.Model(&model.TemporaryQuotaRequest{}).
			Where("cycle_id = ? AND user_id = ? AND status = ? AND decision = ?", cycle.Id, user.Id, model.TemporaryQuotaRequestStatusExecuted, model.TemporaryQuotaDecisionAuto).
			Count(&autoCount).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.TemporaryQuotaRequest{}).
			Select("COALESCE(SUM(approved_quota), 0)").
			Where("cycle_id = ? AND user_id = ? AND status = ? AND decision = ?", cycle.Id, user.Id, model.TemporaryQuotaRequestStatusExecuted, model.TemporaryQuotaDecisionAuto).
			Scan(&autoQuota).Error; err != nil {
			return err
		}
		autoTotalAfter, addErr := checkedAdd(autoQuota, input.RequestedQuota)
		autoEligible := addErr == nil && cycle.TemporaryQuotaAutoApprovalEnabled && int64(user.Quota) < cycle.TemporaryQuotaAutoApprovalThresholdQuota &&
			input.RequestedQuota <= cycle.TemporaryQuotaAutoApprovalSingleQuota && autoCount < int64(cycle.TemporaryQuotaAutoApprovalMaxCount) &&
			autoTotalAfter <= cycle.TemporaryQuotaAutoApprovalMaxQuota && input.RequestedQuota <= remaining
		if !autoEligible {
			return nil
		}
		commit, planID, err = approveTemporaryQuotaInTransaction(
			tx, &stored, cycle, user, input.RequestedQuota, "system", model.TemporaryQuotaDecisionAuto,
			"符合自动发放设置", now,
		)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	if planID != 0 {
		finishTemporaryQuotaExecution(commit)
		RetryNotifications(planID)
	}
	return &stored, nil
}

func approveTemporaryQuotaInTransaction(tx *gorm.DB, request *model.TemporaryQuotaRequest, cycle *model.QuotaCycle, user model.User, approvedQuota int64, reviewer string, decision model.TemporaryQuotaDecision, reviewReason string, now int64) (*executionCommit, int, error) {
	if approvedQuota <= 0 || approvedQuota > request.RequestedQuota || approvedQuota > int64(common.MaxWalletQuota) {
		return nil, 0, errors.New("批准额度必须大于0且不超过申请额度")
	}
	used, err := temporaryQuotaReserveUsed(tx, cycle.Id)
	if err != nil {
		return nil, 0, err
	}
	if approvedQuota > cycle.TemporaryQuotaReserve-used {
		return nil, 0, errors.New("本周期临时额度预留不足")
	}
	after, err := checkedAdd(int64(user.Quota), approvedQuota)
	if err != nil || after > int64(common.MaxWalletQuota) {
		return nil, 0, errors.New("发放后用户余额超出支持范围")
	}
	allocatedBefore, err := cycleAllocatedQuota(cycle)
	if err != nil {
		return nil, 0, err
	}
	parameters, err := common.Marshal(map[string]interface{}{
		"temporary_quota_request_id": request.Id, "decision": decision, "project": request.Project, "reason": request.Reason,
		"concentration_multiplier_basis_points": cycle.ConcentrationMultiplier,
	})
	if err != nil {
		return nil, 0, err
	}
	plan := model.QuotaPlan{
		CycleId: cycle.Id, Name: "临时额度", Purpose: request.Reason,
		PlanType:   model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, NextAdjustmentAt: &cycle.CycleEndAt, AlgorithmVersion: AlgorithmVersion,
		Parameters: string(parameters), BudgetQuotaSnapshot: cycle.BudgetQuota,
		PlannedDeltaQuota: approvedQuota, AllocationBeforeQuota: &allocatedBefore,
		Status: model.QuotaPlanStatusDraft, CreatedAt: now, CreatedBy: reviewer,
	}
	if err := tx.Create(&plan).Error; err != nil {
		return nil, 0, err
	}
	item := model.QuotaItem{
		PlanId: plan.Id, UserId: user.Id, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
		Action: model.QuotaAdjustmentActionTemporaryGrant, SnapshotBalanceQuota: int64(user.Quota),
		AdjustmentQuota: approvedQuota, CalculationData: string(parameters),
		BasisText: fmt.Sprintf("临时额度发放：项目 %s\n申请原因：%s", request.Project, request.Reason),
		LogStatus: model.QuotaNotificationStatusPending, EmailStatus: model.QuotaNotificationStatusPending,
	}
	if err := tx.Create(&item).Error; err != nil {
		return nil, 0, err
	}
	commit, err := executePlanInTransaction(tx, plan.Id, cycle.Id, reviewer, now)
	if err != nil {
		return nil, 0, err
	}
	request.Status = model.TemporaryQuotaRequestStatusExecuted
	request.Decision = decision
	request.ApprovedQuota = approvedQuota
	request.PlanId = &plan.Id
	request.ReviewedBy = reviewer
	request.ReviewReason = reviewReason
	request.ReviewedAt = &now
	request.ExecutedAt = &now
	update := tx.Model(&model.TemporaryQuotaRequest{}).Where("id = ? AND status = ?", request.Id, model.TemporaryQuotaRequestStatusPending).Updates(map[string]interface{}{
		"status": request.Status, "decision": decision, "approved_quota": approvedQuota,
		"plan_id": plan.Id, "reviewed_by": reviewer, "review_reason": reviewReason,
		"reviewed_at": now, "executed_at": now,
	})
	if update.Error != nil {
		return nil, 0, update.Error
	}
	if update.RowsAffected != 1 {
		return nil, 0, errors.New("临时额度申请状态已变化")
	}
	return commit, plan.Id, nil
}

// ApproveTemporaryQuotaRequest approves and executes one pending request atomically.
func ApproveTemporaryQuotaRequest(requestID int, approvedQuota int64, reviewer, reason string) (*model.TemporaryQuotaRequest, error) {
	if !model.CycleQuotaManagementEnabled() {
		return nil, errors.New("周期额度管理未启用")
	}
	if err := rejectBatchQuotaMutation(); err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if requestID <= 0 || reason == "" || utf8.RuneCountInString(reason) > 500 {
		return nil, errors.New("审批参数不正确，审批说明不能为空且不得超过500个字符")
	}
	now := time.Now().Unix()
	var request model.TemporaryQuotaRequest
	var commit *executionCommit
	var planID int
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.LockForUpdate(tx).First(&request, requestID).Error; err != nil {
			return err
		}
		if request.Status != model.TemporaryQuotaRequestStatusPending {
			return errors.New("临时额度申请不是待审批状态")
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
			return errors.New("申请用户已不符合临时额度发放条件")
		}
		var err error
		commit, planID, err = approveTemporaryQuotaInTransaction(
			tx, &request, &cycle, user, approvedQuota, reviewer,
			model.TemporaryQuotaDecisionManual, reason, now,
		)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	finishTemporaryQuotaExecution(commit)
	RetryNotifications(planID)
	return &request, nil
}

// RejectTemporaryQuotaRequest closes one pending request without changing quota.
func RejectTemporaryQuotaRequest(requestID int, reviewer, reason string) (*model.TemporaryQuotaRequest, error) {
	if !model.CycleQuotaManagementEnabled() {
		return nil, errors.New("周期额度管理未启用")
	}
	reason = strings.TrimSpace(reason)
	if requestID <= 0 || reason == "" || utf8.RuneCountInString(reason) > 500 {
		return nil, errors.New("拒绝原因不能为空且不得超过500个字符")
	}
	now := time.Now().Unix()
	var request model.TemporaryQuotaRequest
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.LockForUpdate(tx).First(&request, requestID).Error; err != nil {
			return err
		}
		if request.Status != model.TemporaryQuotaRequestStatusPending {
			return errors.New("临时额度申请不是待审批状态")
		}
		request.Status = model.TemporaryQuotaRequestStatusRejected
		request.Decision = model.TemporaryQuotaDecisionManual
		request.ReviewedBy = reviewer
		request.ReviewReason = reason
		request.ReviewedAt = &now
		result := tx.Model(&model.TemporaryQuotaRequest{}).
			Where("id = ? AND status = ?", request.Id, model.TemporaryQuotaRequestStatusPending).
			Updates(map[string]interface{}{
				"status": request.Status, "decision": request.Decision, "reviewed_by": reviewer,
				"review_reason": reason, "reviewed_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("临时额度申请状态已变化")
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	return &request, err
}

func finishTemporaryQuotaExecution(commit *executionCommit) {
	for userID, delta := range commit.QuotaDeltas {
		if err := model.SyncUserQuotaCacheDelta(userID, delta); err != nil {
			common.SysError(fmt.Sprintf("failed to sync temporary quota grant for user %d: %v", userID, err))
		}
	}
}
