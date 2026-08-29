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
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"gorm.io/gorm"
)

// CycleDefaults are server-derived values for the create-cycle form.
type CycleDefaults struct {
	StartAt int64 `json:"start_at"`
}

// InitialGrantRecommendation explains the historical calculation shown before cycle creation.
type InitialGrantRecommendation struct {
	Quota          int64 `json:"-"`
	ReferenceStart int64 `json:"reference_start"`
	ReferenceEnd   int64 `json:"reference_end"`
	UserCount      int   `json:"user_count"`
	AverageSpend   int64 `json:"-"`
	Buffered       int64 `json:"-"`
	StabilityFloor int64 `json:"-"`
}

// CycleListResult contains recent cycles and the next non-overlapping form defaults.
type CycleListResult struct {
	Cycles         []model.QuotaCycle
	Defaults       CycleDefaults
	Recommendation *InitialGrantRecommendation
}

// CycleDetail contains a cycle, its plans, and any scheduled-cycle recommendation.
type CycleDetail struct {
	Cycle          model.QuotaCycle
	Plans          []model.QuotaPlan
	Recommendation *InitialGrantRecommendation
}

// CreateCycleParams contains validated quota units and Unix-second boundaries.
type CreateCycleParams struct {
	StartAt                     int64
	EndAt                       int64
	BudgetQuota                 int64
	InitialGrantQuota           int64
	TemporaryQuotaReserve       int64
	TemporaryQuotaPolicy        TemporaryQuotaAutoApprovalPolicy
	ConcentrationMultiplier     int64
	BalancePolicy               model.QuotaCycleBalancePolicy
	CarryOverCountsTowardBudget bool
	CreatedBy                   string
}

// TemporaryQuotaAutoApprovalPolicy is the scheduled-cycle auto-approval policy frozen at activation.
type TemporaryQuotaAutoApprovalPolicy struct {
	Enabled        bool
	SingleQuota    int64
	ThresholdQuota int64
	MaxCount       int
	MaxQuota       int64
}

// ParseTemporaryQuotaAutoApprovalPolicy converts API strings into the policy
// used by the quota service. Full cross-field validation remains in the service
// create/update operations, where the reserve and budget are available.
func ParseTemporaryQuotaAutoApprovalPolicy(enabled bool, singleRaw, thresholdRaw string, maxCount int, maxRaw string) (TemporaryQuotaAutoApprovalPolicy, error) {
	if !enabled {
		return TemporaryQuotaAutoApprovalPolicy{}, nil
	}
	single, err := ParsePositiveQuota(singleRaw, "自动发放单次上限")
	if err != nil {
		return TemporaryQuotaAutoApprovalPolicy{}, err
	}
	threshold, err := ParsePositiveQuota(thresholdRaw, "自动发放余额门槛")
	if err != nil {
		return TemporaryQuotaAutoApprovalPolicy{}, err
	}
	maximum, err := ParsePositiveQuota(maxRaw, "自动发放每用户总额上限")
	if err != nil {
		return TemporaryQuotaAutoApprovalPolicy{}, err
	}
	return TemporaryQuotaAutoApprovalPolicy{Enabled: true, SingleQuota: single, ThresholdQuota: threshold, MaxCount: maxCount, MaxQuota: maximum}, nil
}

func validateInitialGrantQuota(quota int64) error {
	if quota <= 0 {
		return errors.New("人均首次额度必须大于0")
	}
	if quota > int64(common.MaxWalletQuota) {
		return errors.New("人均首次额度超出单用户可支持范围")
	}
	return nil
}

func validateTemporaryQuotaReserve(budgetQuota int64, reserveQuota int64) error {
	if reserveQuota < 0 || reserveQuota >= budgetQuota {
		return errors.New("临时额度预留必须大于等于0且小于周期总额度")
	}
	return nil
}

func validateTemporaryQuotaAutoApprovalPolicy(policy TemporaryQuotaAutoApprovalPolicy, reserveQuota int64) error {
	if !policy.Enabled {
		if policy.SingleQuota != 0 || policy.ThresholdQuota != 0 || policy.MaxCount != 0 || policy.MaxQuota != 0 {
			return errors.New("关闭自动发放时额度和次数设置必须为0")
		}
		return nil
	}
	if reserveQuota <= 0 {
		return errors.New("开启自动发放前必须配置正数临时额度预留")
	}
	if policy.SingleQuota <= 0 || policy.SingleQuota > int64(common.MaxWalletQuota) {
		return errors.New("自动发放单次上限不在支持范围内")
	}
	if policy.ThresholdQuota <= 0 || policy.ThresholdQuota > int64(common.MaxWalletQuota) {
		return errors.New("自动发放余额门槛不在支持范围内")
	}
	if policy.MaxCount <= 0 || policy.MaxCount > common.MaxQuota {
		return errors.New("自动发放每用户次数上限不在支持范围内")
	}
	if policy.MaxQuota < policy.SingleQuota || policy.MaxQuota > int64(common.MaxWalletQuota) {
		return errors.New("自动发放每用户总额上限必须不小于单次上限且不超过单用户额度上限")
	}
	if policy.MaxQuota > reserveQuota {
		return errors.New("自动发放每用户总额上限不能超过临时额度预留")
	}
	return nil
}

// ListCycles returns the latest 50 cycles and the next available start time.
func ListCycles() (*CycleListResult, error) {
	now := time.Now().Unix()
	var cycles []model.QuotaCycle
	if err := model.DB.Order("cycle_start_at DESC").Limit(50).Find(&cycles).Error; err != nil {
		return nil, err
	}
	localNow := time.Unix(now, 0).In(shanghaiLocation)
	startAt := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, shanghaiLocation).Unix()
	for _, cycle := range cycles {
		if cycle.CycleEndAt > startAt {
			startAt = cycle.CycleEndAt
		}
	}
	recommendation, err := initialGrantRecommendation(startAt, now)
	if err != nil {
		return nil, err
	}
	return &CycleListResult{
		Cycles:         cycles,
		Defaults:       CycleDefaults{StartAt: startAt},
		Recommendation: recommendation,
	}, nil
}

// GetCycleDetail returns a cycle and its plans ordered by newest first.
func GetCycleDetail(cycleID int) (*CycleDetail, error) {
	var cycle model.QuotaCycle
	if err := model.DB.First(&cycle, cycleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("周期不存在")
		}
		return nil, err
	}
	var plans []model.QuotaPlan
	if err := model.DB.Where("cycle_id = ?", cycleID).Order("created_at DESC").Find(&plans).Error; err != nil {
		return nil, err
	}
	var recommendation *InitialGrantRecommendation
	if cycle.Status == model.QuotaCycleStatusScheduled {
		var err error
		recommendation, err = initialGrantRecommendation(cycle.CycleStartAt, time.Now().Unix())
		if err != nil {
			return nil, err
		}
	}
	return &CycleDetail{Cycle: cycle, Plans: plans, Recommendation: recommendation}, nil
}

// CreateCycle creates a scheduled, non-overlapping quota allocation window.
func CreateCycle(params CreateCycleParams) (*model.QuotaCycle, error) {
	if params.EndAt <= params.StartAt {
		return nil, errors.New("结束时间必须晚于开始时间")
	}
	if params.BudgetQuota <= 0 {
		return nil, errors.New("周期总额度必须大于0")
	}
	if !params.BalancePolicy.Valid() {
		return nil, errors.New("余额结转规则不正确")
	}
	leadSeconds := int64(operation_setting.GetQuotaSetting().SettlementLeadMinutes * 60)
	if leadSeconds <= 0 || params.EndAt-params.StartAt <= leadSeconds {
		return nil, errors.New("结算提前时间必须小于周期时长")
	}
	if err := validateInitialGrantQuota(params.InitialGrantQuota); err != nil {
		return nil, err
	}
	if err := validateTemporaryQuotaReserve(params.BudgetQuota, params.TemporaryQuotaReserve); err != nil {
		return nil, err
	}
	if err := validateTemporaryQuotaAutoApprovalPolicy(params.TemporaryQuotaPolicy, params.TemporaryQuotaReserve); err != nil {
		return nil, err
	}
	if err := validateConcentrationMultiplier(params.ConcentrationMultiplier); err != nil {
		return nil, err
	}
	cycle := model.QuotaCycle{
		CycleStartAt: params.StartAt, CycleEndAt: params.EndAt,
		BudgetQuota: params.BudgetQuota, InitialGrantQuota: params.InitialGrantQuota,
		TemporaryQuotaReserve: params.TemporaryQuotaReserve, BalancePolicy: params.BalancePolicy,
		TemporaryQuotaAutoApprovalEnabled:        params.TemporaryQuotaPolicy.Enabled,
		TemporaryQuotaAutoApprovalSingleQuota:    params.TemporaryQuotaPolicy.SingleQuota,
		TemporaryQuotaAutoApprovalThresholdQuota: params.TemporaryQuotaPolicy.ThresholdQuota,
		TemporaryQuotaAutoApprovalMaxCount:       params.TemporaryQuotaPolicy.MaxCount,
		TemporaryQuotaAutoApprovalMaxQuota:       params.TemporaryQuotaPolicy.MaxQuota,
		ConcentrationMultiplier:                  params.ConcentrationMultiplier,
		Status:                                   model.QuotaCycleStatusScheduled, CreatedBy: params.CreatedBy,
	}
	cycle.CarryOverCountsTowardBudget = params.CarryOverCountsTowardBudget
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var modeOption model.Option
		if err := model.LockForUpdate(tx).Where("key = ?", model.CycleQuotaManagementOptionKey).First(&modeOption).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("周期额度管理未启用")
			}
			return err
		}
		if modeOption.Value != "true" {
			return errors.New("周期额度管理未启用")
		}
		var overlap int64
		if err := tx.Model(&model.QuotaCycle{}).
			Where("cycle_start_at < ? AND cycle_end_at > ?", params.EndAt, params.StartAt).
			Count(&overlap).Error; err != nil {
			return err
		}
		if overlap > 0 {
			return errors.New("周期时间与现有周期冲突")
		}
		return tx.Create(&cycle).Error
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	return &cycle, nil
}

// UpdateCycleSettings updates budget and scheduled-only allocation settings.
func UpdateCycleSettings(cycleID int, budgetQuota int64, initialGrantQuota *int64, temporaryQuotaReserve *int64, temporaryQuotaPolicy *TemporaryQuotaAutoApprovalPolicy, updatedBy string) error {
	if !model.CycleQuotaManagementEnabled() {
		return errors.New("周期额度管理未启用")
	}
	if budgetQuota <= 0 {
		return errors.New("周期总额度必须大于0")
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var cycle model.QuotaCycle
		if err := model.LockForUpdate(tx).First(&cycle, cycleID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("周期不存在")
			}
			return err
		}
		if cycle.Status == model.QuotaCycleStatusClosed {
			return errors.New("周期已关闭")
		}
		reserveQuota := cycle.TemporaryQuotaReserve
		if temporaryQuotaReserve != nil {
			if cycle.Status != model.QuotaCycleStatusScheduled {
				return errors.New("只有已规划周期可以修改临时额度预留")
			}
			reserveQuota = *temporaryQuotaReserve
		}
		if err := validateTemporaryQuotaReserve(budgetQuota, reserveQuota); err != nil {
			return err
		}
		if temporaryQuotaPolicy != nil {
			if cycle.Status != model.QuotaCycleStatusScheduled {
				return errors.New("只有已规划周期可以修改自动发放设置")
			}
			if err := validateTemporaryQuotaAutoApprovalPolicy(*temporaryQuotaPolicy, reserveQuota); err != nil {
				return err
			}
		} else if err := validateTemporaryQuotaAutoApprovalPolicy(TemporaryQuotaAutoApprovalPolicy{
			Enabled: cycle.TemporaryQuotaAutoApprovalEnabled, SingleQuota: cycle.TemporaryQuotaAutoApprovalSingleQuota,
			ThresholdQuota: cycle.TemporaryQuotaAutoApprovalThresholdQuota, MaxCount: cycle.TemporaryQuotaAutoApprovalMaxCount,
			MaxQuota: cycle.TemporaryQuotaAutoApprovalMaxQuota,
		}, reserveQuota); err != nil {
			return err
		}
		settingsChanged := budgetQuota != cycle.BudgetQuota ||
			(initialGrantQuota != nil && *initialGrantQuota != cycle.InitialGrantQuota) ||
			(temporaryQuotaReserve != nil && *temporaryQuotaReserve != cycle.TemporaryQuotaReserve) ||
			(temporaryQuotaPolicy != nil && (temporaryQuotaPolicy.Enabled != cycle.TemporaryQuotaAutoApprovalEnabled ||
				temporaryQuotaPolicy.SingleQuota != cycle.TemporaryQuotaAutoApprovalSingleQuota ||
				temporaryQuotaPolicy.ThresholdQuota != cycle.TemporaryQuotaAutoApprovalThresholdQuota ||
				temporaryQuotaPolicy.MaxCount != cycle.TemporaryQuotaAutoApprovalMaxCount || temporaryQuotaPolicy.MaxQuota != cycle.TemporaryQuotaAutoApprovalMaxQuota))
		if (cycle.Status == model.QuotaCycleStatusActive || cycle.Status == model.QuotaCycleStatusSettling) && budgetQuota != cycle.BudgetQuota {
			occupied, err := cycleAllocatedQuota(&cycle)
			if err != nil {
				return err
			}
			if occupied > budgetQuota {
				return errors.New("周期总额度不能低于当前已发放额度")
			}
		}
		updates := map[string]interface{}{
			"budget_quota": budgetQuota, "updated_at": time.Now().Unix(), "updated_by": updatedBy,
		}
		if initialGrantQuota != nil {
			if cycle.Status != model.QuotaCycleStatusScheduled {
				return errors.New("只有已规划周期可以修改人均首次额度")
			}
			if err := validateInitialGrantQuota(*initialGrantQuota); err != nil {
				return err
			}
			updates["initial_grant_quota"] = *initialGrantQuota
		}
		if temporaryQuotaReserve != nil {
			updates["temporary_quota_reserve"] = *temporaryQuotaReserve
		}
		if temporaryQuotaPolicy != nil {
			updates["temporary_quota_auto_approval_enabled"] = temporaryQuotaPolicy.Enabled
			updates["temporary_quota_auto_approval_single_quota"] = temporaryQuotaPolicy.SingleQuota
			updates["temporary_quota_auto_approval_threshold_quota"] = temporaryQuotaPolicy.ThresholdQuota
			updates["temporary_quota_auto_approval_max_count"] = temporaryQuotaPolicy.MaxCount
			updates["temporary_quota_auto_approval_max_quota"] = temporaryQuotaPolicy.MaxQuota
		}
		if err := tx.Model(&model.QuotaCycle{}).Where("id = ?", cycleID).Updates(updates).Error; err != nil {
			return err
		}
		if !settingsChanged {
			return nil
		}
		return tx.Model(&model.QuotaPlan{}).
			Where("cycle_id = ? AND status = ?", cycleID, model.QuotaPlanStatusDraft).
			Updates(map[string]interface{}{
				"status": model.QuotaPlanStatusCancelled, "cancelled_at": time.Now().Unix(),
				"cancelled_by": updatedBy, "cancel_reason": "周期预算、首次额度或临时额度设置变更",
			}).Error
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	return err
}

// CloseCycle cancels a scheduled cycle or starts a safe close for an active cycle.
func CloseCycle(cycleID int, updatedBy string) (model.QuotaCycleStatus, error) {
	if !model.CycleQuotaManagementEnabled() {
		return "", errors.New("周期额度管理未启用")
	}
	if err := rejectBatchQuotaMutation(); err != nil {
		return "", err
	}
	var cycle model.QuotaCycle
	if err := model.DB.First(&cycle, cycleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("周期不存在")
		}
		return "", err
	}
	now := time.Now().Unix()
	if cycle.Status == model.QuotaCycleStatusActive {
		if err := beginCycleSettlement(cycleID, now, updatedBy); err != nil {
			return "", err
		}
		cycle.Status = model.QuotaCycleStatusSettling
	}
	if err := settleCycle(cycleID, now, updatedBy); err != nil {
		if errors.Is(err, errQuotaCycleHasInFlightWork) {
			return model.QuotaCycleStatusSettling, nil
		}
		return "", err
	}
	return model.QuotaCycleStatusClosed, nil
}

func activeCycle() (*model.QuotaCycle, error) {
	var cycle model.QuotaCycle
	err := model.DB.Where("status = ?", model.QuotaCycleStatusActive).Order("cycle_start_at DESC").First(&cycle).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &cycle, err
}

func initialGrantRecommendation(beforeTime int64, now int64) (*InitialGrantRecommendation, error) {
	if !common.LogConsumeEnabled {
		return nil, nil
	}
	var cycle model.QuotaCycle
	err := model.DB.Where("cycle_start_at < ? AND cycle_start_at <= ?", beforeTime, now).
		Order("cycle_start_at DESC").First(&cycle).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	users, err := loadManagedUsers(model.DB)
	if err != nil {
		return nil, err
	}
	activeCount := 0
	userIDs := make([]int, 0, len(users))
	for _, user := range users {
		if user.Status == common.UserStatusEnabled {
			userIDs = append(userIDs, user.ID)
			activeCount++
		}
	}
	if activeCount == 0 {
		return nil, nil
	}
	referenceEnd := min(now, cycle.CycleEndAt)
	if cycle.SettledAt != nil && *cycle.SettledAt > referenceEnd {
		referenceEnd = min(now, *cycle.SettledAt)
	}
	if model.LOG_DB == nil {
		return nil, errors.New("日志数据库未初始化")
	}
	var spend int64
	query := model.LOG_DB.Model(&model.Log{}).
		Select("COALESCE(SUM(CASE WHEN type = ? THEN -quota ELSE quota END), 0)", model.LogTypeRefund).
		Where("type IN ? AND created_at >= ? AND created_at <= ?",
			[]int{model.LogTypeConsume, model.LogTypeRefund}, cycle.CycleStartAt, referenceEnd)
	if len(userIDs) > 0 {
		query = query.Where("user_id IN ?", userIDs)
	}
	err = query.Scan(&spend).Error
	if err != nil {
		return nil, err
	}
	if spend <= 0 {
		return &InitialGrantRecommendation{
			Quota: cycle.InitialGrantQuota, ReferenceStart: cycle.CycleStartAt,
			ReferenceEnd: referenceEnd, UserCount: activeCount,
		}, nil
	}
	average, err := ceilDiv(spend, int64(activeCount))
	if err != nil {
		return nil, err
	}
	buffered, err := bigRatio([]int64{average, initialUsageBufferPercent}, []int64{100}, false)
	if err != nil {
		return nil, err
	}
	buffered, err = roundUpCent(buffered)
	if err != nil {
		return nil, err
	}
	stabilityFloor, err := bigRatio([]int64{cycle.InitialGrantQuota, initialStabilityFloorPercent}, []int64{100}, false)
	if err != nil {
		return nil, err
	}
	stabilityFloor, err = roundUpCent(stabilityFloor)
	if err != nil {
		return nil, err
	}
	recommended, err := recommendInitialGrant(spend, activeCount, cycle.InitialGrantQuota)
	if err != nil {
		return nil, err
	}
	return &InitialGrantRecommendation{
		Quota: recommended, ReferenceStart: cycle.CycleStartAt, ReferenceEnd: referenceEnd,
		UserCount: activeCount, AverageSpend: average,
		Buffered: buffered, StabilityFloor: stabilityFloor,
	}, nil
}

// PlanOptions contains the active-cycle gate and schedule used by the generation form.
type PlanOptions struct {
	Cycle                  *model.QuotaCycle
	InitializationRequired bool
	NextAdjustmentAt       int64
}

// PlanDetail contains a plan, its items, and calculated fund-flow totals.
type PlanDetail struct {
	Plan               model.QuotaPlan
	Cycle              model.QuotaCycle
	Items              []model.QuotaItem
	Summary            DetailedPlanSummary
	ConfirmationPhrase string
}

// CategorySummary groups affected accounts and quota for one business action.
type CategorySummary struct {
	Count int
	Total int64
}

// DetailedPlanSummary contains the fund-flow values shown in plan details.
type DetailedPlanSummary struct {
	Increase                    CategorySummary
	Decrease                    CategorySummary
	Affected                    int
	Budget                      int64
	OccupiedAfter               int64
	CyclePoolIssuedQuota        int64
	ReclaimedUsedForIncreases   int64
	ReclaimedUsedToCoverOverage int64
	ReclaimedUnused             int64
	PoolRemaining               int64
}

// GetPlanOptions returns the active cycle, schedule, and initialization gate.
func GetPlanOptions() (*PlanOptions, error) {
	cycle, err := activeCycle()
	if err != nil || cycle == nil {
		return &PlanOptions{Cycle: cycle}, err
	}
	executionStats, err := loadAllocationExecutionStats(model.DB, cycle.Id)
	if err != nil {
		return nil, err
	}
	return &PlanOptions{
		Cycle: cycle, InitializationRequired: !executionStats.AllocationExecuted,
		NextAdjustmentAt: defaultNextAdjustmentAt(*cycle, time.Now().Unix()),
	}, nil
}

func defaultNextAdjustmentAt(cycle model.QuotaCycle, now int64) int64 {
	next := localDayAt(now, 7, 9)
	if next > cycle.CycleEndAt {
		return cycle.CycleEndAt
	}
	return next
}

// GetPlanDetail loads the immutable draft inputs, items, and derived fund flow.
func GetPlanDetail(planID int) (*PlanDetail, error) {
	var plan model.QuotaPlan
	if err := model.DB.First(&plan, planID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("方案不存在")
		}
		return nil, err
	}
	var cycle model.QuotaCycle
	if err := model.DB.First(&cycle, plan.CycleId).Error; err != nil {
		return nil, err
	}
	var items []model.QuotaItem
	if err := model.DB.Where("plan_id = ?", plan.Id).Order("id").Find(&items).Error; err != nil {
		return nil, err
	}
	temporaryQuotaUsed, err := temporaryQuotaReserveUsedAt(model.DB, cycle.Id, plan.SnapshotAt, plan.Id)
	if err != nil {
		return nil, err
	}
	summary, err := summarizePlan(plan, items, cycle.TemporaryQuotaReserve, temporaryQuotaUsed)
	if err != nil {
		return nil, err
	}
	return &PlanDetail{
		Plan: plan, Cycle: cycle, Items: items, Summary: summary,
		ConfirmationPhrase: fmt.Sprintf("确认发放%s并收回%s", FormatQuota(summary.Increase.Total), FormatQuota(summary.Decrease.Total)),
	}, nil
}

// CancelPlan cancels a draft with an auditable reason.
func CancelPlan(planID int, cancelledBy string, reason string) error {
	if !model.CycleQuotaManagementEnabled() {
		return errors.New("周期额度管理未启用")
	}
	reason = strings.TrimSpace(reason)
	length := utf8.RuneCountInString(reason)
	if length == 0 {
		return errors.New("取消原因不能为空")
	}
	if length > 500 {
		return errors.New("取消原因不得超过500个字符")
	}
	now := time.Now().Unix()
	result := model.DB.Model(&model.QuotaPlan{}).
		Where("id = ? AND status = ?", planID, model.QuotaPlanStatusDraft).
		Updates(map[string]interface{}{
			"status": model.QuotaPlanStatusCancelled, "cancelled_at": now,
			"cancelled_by": cancelledBy, "cancel_reason": reason,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("方案不存在或状态不是 draft")
	}
	return nil
}

// RegeneratePlan reuses a non-executed plan's reviewed settings with a current snapshot.
func RegeneratePlan(planID int, createdBy string) (*PlanResult, error) {
	if !model.CycleQuotaManagementEnabled() {
		return nil, errors.New("周期额度管理未启用")
	}
	if err := rejectBatchQuotaMutation(); err != nil {
		return nil, err
	}
	var planReference model.QuotaPlan
	if err := model.DB.Select("cycle_id").First(&planReference, planID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("方案不存在")
		}
		return nil, err
	}

	var result *PlanResult
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var cycle model.QuotaCycle
		if err := model.LockForUpdate(tx).First(&cycle, planReference.CycleId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("关联周期不存在")
			}
			return err
		}
		var plan model.QuotaPlan
		if err := model.LockForUpdate(tx).First(&plan, planID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("方案不存在")
			}
			return err
		}
		if plan.CycleId != cycle.Id {
			return errors.New("方案关联周期已变化")
		}
		if plan.Status == model.QuotaPlanStatusExecuted {
			return errors.New("已执行方案不能重新生成")
		}
		now := time.Now().Unix()
		if now < cycle.CycleStartAt || now >= cycle.CycleEndAt {
			return errors.New("只能重新生成当前处于周期内的方案")
		}
		nextAdjustment := defaultNextAdjustmentAt(cycle, now)
		if plan.NextAdjustmentAt != nil && *plan.NextAdjustmentAt > now && *plan.NextAdjustmentAt <= cycle.CycleEndAt {
			nextAdjustment = *plan.NextAdjustmentAt
		}
		params := GenerateParams{
			CycleID: cycle.Id, Name: plan.Name, Purpose: plan.Purpose,
			PlanType:         plan.PlanType,
			NextAdjustmentAt: &nextAdjustment,
			CreatedBy:        createdBy,
		}
		generated, err := generatePlanInTransaction(tx, params, now)
		if err != nil {
			return err
		}
		if plan.Status == model.QuotaPlanStatusDraft {
			if err := tx.Model(&model.QuotaPlan{}).Where("id = ?", plan.Id).Updates(map[string]interface{}{
				"status": model.QuotaPlanStatusCancelled, "cancelled_at": now,
				"cancelled_by": createdBy, "cancel_reason": fmt.Sprintf("重新生成方案 #%d，原方案作废", generated.Plan.Id),
			}).Error; err != nil {
				return err
			}
		}
		result = generated
		return nil
	}, generationTransactionOptions())
	return result, err
}

func summarizePlan(plan model.QuotaPlan, items []model.QuotaItem, temporaryQuotaReserve int64, temporaryQuotaUsed int64) (DetailedPlanSummary, error) {
	ordinaryAllocationCap := plan.BudgetQuotaSnapshot - temporaryQuotaReserve
	if plan.AllocationBeforeQuota == nil {
		return DetailedPlanSummary{}, errors.New("方案缺少额度发放快照")
	}
	occupiedBefore := *plan.AllocationBeforeQuota
	temporaryQuotaDeduction := min(temporaryQuotaUsed, occupiedBefore)
	ordinaryAllocatedBefore := occupiedBefore - temporaryQuotaDeduction
	var increaseCount, decreaseCount int
	var increaseTotal, decreaseTotal int64
	for _, item := range items {
		if item.AdjustmentQuota > 0 {
			increaseCount++
			var err error
			increaseTotal, err = checkedAdd(increaseTotal, item.AdjustmentQuota)
			if err != nil {
				return DetailedPlanSummary{}, err
			}
		}
		if item.AdjustmentQuota >= 0 {
			continue
		}
		decreaseCount++
		amount := -item.AdjustmentQuota
		var err error
		decreaseTotal, err = checkedAdd(decreaseTotal, amount)
		if err != nil {
			return DetailedPlanSummary{}, err
		}
	}
	occupiedAfter, err := checkedSum(occupiedBefore, increaseTotal, -decreaseTotal)
	if err != nil {
		return DetailedPlanSummary{}, err
	}
	if plan.Status == model.QuotaPlanStatusExecuted && plan.AllocationAfterQuota != nil {
		occupiedAfter = *plan.AllocationAfterQuota
	}
	reclaimedUsedToCoverOverage := min(decreaseTotal, max(int64(0), ordinaryAllocatedBefore-ordinaryAllocationCap))
	reclaimedAvailableForIncreases := decreaseTotal - reclaimedUsedToCoverOverage
	reclaimedUsedForIncreases := min(increaseTotal, reclaimedAvailableForIncreases)
	cyclePoolIssuedQuota := increaseTotal - reclaimedUsedForIncreases
	reclaimedUnused := max(int64(0), reclaimedAvailableForIncreases-reclaimedUsedForIncreases)
	return DetailedPlanSummary{
		Increase:                    CategorySummary{Count: increaseCount, Total: increaseTotal},
		Decrease:                    CategorySummary{Count: decreaseCount, Total: decreaseTotal},
		Affected:                    len(items),
		Budget:                      plan.BudgetQuotaSnapshot,
		OccupiedAfter:               occupiedAfter,
		CyclePoolIssuedQuota:        cyclePoolIssuedQuota,
		ReclaimedUsedForIncreases:   reclaimedUsedForIncreases,
		ReclaimedUsedToCoverOverage: reclaimedUsedToCoverOverage,
		ReclaimedUnused:             reclaimedUnused,
		PoolRemaining:               max(int64(0), plan.BudgetQuotaSnapshot-occupiedAfter),
	}, nil
}
