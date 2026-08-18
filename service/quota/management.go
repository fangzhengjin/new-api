package quota

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// CycleDefaults are server-derived values for the create-cycle form.
type CycleDefaults struct {
	StartAt int64 `json:"start_at"`
	EndAt   int64 `json:"end_at"`
}

// InitialGrantRecommendation explains the historical calculation shown before cycle creation.
type InitialGrantRecommendation struct {
	Quota          int64 `json:"-"`
	ReferenceStart int64 `json:"reference_start"`
	ReferenceEnd   int64 `json:"reference_end"`
	UserCount      int   `json:"user_count"`
	TotalSpend     int64 `json:"-"`
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
	StartAt           int64
	EndAt             int64
	BudgetQuota       int64
	InitialGrantQuota int64
	BalancePolicy     model.QuotaCycleBalancePolicy
	CreatedBy         string
}

func validateInitialGrantQuota(quota int64) error {
	if quota <= 0 {
		return errors.New("人均首次额度必须大于0")
	}
	if quota > int64(common.MaxQuota) {
		return errors.New("人均首次额度超出单用户可支持范围")
	}
	return nil
}

// ListCycles activates due schedules and returns the latest 50 cycles.
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
		Defaults:       CycleDefaults{StartAt: startAt, EndAt: time.Unix(startAt, 0).AddDate(0, 0, 30).Unix()},
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

// CreateCycle creates a scheduled, non-overlapping purchasing window.
func CreateCycle(params CreateCycleParams) (*model.QuotaCycle, error) {
	if params.EndAt <= params.StartAt {
		return nil, errors.New("结束时间必须晚于开始时间")
	}
	if params.BudgetQuota <= 0 {
		return nil, errors.New("采购总额必须大于0")
	}
	if err := validateInitialGrantQuota(params.InitialGrantQuota); err != nil {
		return nil, err
	}
	if !params.BalancePolicy.Valid() {
		return nil, errors.New("周期余额策略必须是 reset 或 carry")
	}
	cycle := model.QuotaCycle{
		CycleStartAt: params.StartAt, CycleEndAt: params.EndAt,
		BudgetQuota: params.BudgetQuota, InitialGrantQuota: params.InitialGrantQuota,
		BalancePolicy: params.BalancePolicy,
		Status:        model.QuotaCycleStatusScheduled, CreatedBy: params.CreatedBy,
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
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

// UpdateCycleSettings updates budget and, only before activation, the initial grant.
func UpdateCycleSettings(cycleID int, budgetQuota int64, initialGrantQuota *int64, updatedBy string) error {
	if budgetQuota <= 0 {
		return errors.New("采购总额必须大于0")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
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
		settingsChanged := budgetQuota != cycle.BudgetQuota || (initialGrantQuota != nil && *initialGrantQuota != cycle.InitialGrantQuota)
		if model.CompanyQuotaModeEnabled() && cycle.Status == model.QuotaCycleStatusActive && budgetQuota != cycle.BudgetQuota {
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
			if occupied > budgetQuota {
				return errors.New("新周期预算不能低于当前受管头寸")
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
				"cancelled_by": updatedBy, "cancel_reason": "周期预算或首次额度变更",
			}).Error
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
}

// CloseCycle closes a scheduled or active cycle through the lifecycle settlement path.
func CloseCycle(cycleID int, updatedBy string) error {
	_, err := settleCycle(cycleID, time.Now().Unix(), updatedBy)
	return err
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
		userIDs = append(userIDs, user.ID)
		if user.Status == common.UserStatusEnabled {
			activeCount++
		}
	}
	if activeCount == 0 {
		return nil, nil
	}
	referenceEnd := minQuota(now, cycle.CycleEndAt)
	var spend int64
	if model.CompanyQuotaModeEnabled() {
		spend, err = model.SumQuotaCycleSettlement(model.DB, cycle.Id, referenceEnd)
	} else {
		if model.LOG_DB == nil {
			return nil, errors.New("日志数据库未初始化")
		}
		query := model.LOG_DB.Model(&model.Log{}).
			Select("COALESCE(SUM(quota), 0)").
			Where("type = ? AND created_at >= ? AND created_at <= ?", model.LogTypeConsume, cycle.CycleStartAt, referenceEnd)
		if len(userIDs) > 0 {
			query = query.Where("user_id IN ?", userIDs)
		}
		err = query.Scan(&spend).Error
	}
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
		UserCount: activeCount, TotalSpend: spend, AverageSpend: average,
		Buffered: buffered, StabilityFloor: stabilityFloor,
	}, nil
}

// PlanOptions contains the active-cycle gate and schedule used by the generation form.
type PlanOptions struct {
	Cycle                  *model.QuotaCycle
	Schedule               *Schedule
	InitializationRequired bool
}

// PlanDetail contains all persisted inputs and calculated fund-flow totals.
type PlanDetail struct {
	Plan               model.QuotaPlan
	Cycle              model.QuotaCycle
	Items              []model.QuotaItem
	Parameters         PlanParameters
	Summary            DetailedPlanSummary
	ConfirmationPhrase string
}

// CategorySummary groups affected accounts and quota for one business action.
type CategorySummary struct {
	Count int
	Total int64
}

// DetailedPlanSummary mirrors the source tool's adjustment, fund-flow, and pool views.
type DetailedPlanSummary struct {
	BaseIncrease                CategorySummary
	Supplement                  CategorySummary
	Bonus                       CategorySummary
	WeightedPool                CategorySummary
	Increase                    CategorySummary
	OrdinaryDecrease            CategorySummary
	FinalLowDecrease            CategorySummary
	DisabledDecrease            CategorySummary
	Decrease                    CategorySummary
	Affected                    int
	Budget                      int64
	StageCap                    int64
	TotalSpend                  int64
	ManagedBalance              int64
	OccupiedBefore              int64
	OccupiedAfter               int64
	BalanceAfter                int64
	AvailableBeforeReclaim      int64
	StageSourceTotal            int64
	ReclaimedUsedForIncreases   int64
	ReclaimedUsedToCoverOverage int64
	ReclaimedUnused             int64
	StageOriginalUnused         int64
	StageRemaining              int64
	PoolRemaining               int64
	FutureReserved              int64
	FinalStage                  bool
}

// GetPlanOptions returns the active cycle, schedule, and initialization gate.
func GetPlanOptions() (*PlanOptions, error) {
	cycle, err := activeCycle()
	if err != nil || cycle == nil {
		return &PlanOptions{Cycle: cycle}, err
	}
	now := time.Now().Unix()
	schedule := RecommendSchedule(cycle.CycleStartAt, cycle.CycleEndAt, now)
	var executed int64
	if err := model.DB.Model(&model.QuotaPlan{}).
		Where("cycle_id = ? AND status = ?", cycle.Id, model.QuotaPlanStatusExecuted).
		Count(&executed).Error; err != nil {
		return nil, err
	}
	return &PlanOptions{Cycle: cycle, Schedule: &schedule, InitializationRequired: executed == 0}, nil
}

// ListPlans returns at most 50 recent plans, optionally limited to one cycle.
func ListPlans(cycleID *int) ([]model.QuotaPlan, error) {
	query := model.DB.Order("created_at DESC").Limit(50)
	if cycleID != nil {
		query = query.Where("cycle_id = ?", *cycleID)
	}
	var plans []model.QuotaPlan
	err := query.Find(&plans).Error
	return plans, err
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
	var parameters PlanParameters
	if err := common.Unmarshal([]byte(plan.Parameters), &parameters); err != nil {
		return nil, fmt.Errorf("方案参数无效: %w", err)
	}
	summary, err := summarizePlan(plan, items)
	if err != nil {
		return nil, err
	}
	return &PlanDetail{
		Plan: plan, Cycle: cycle, Items: items, Parameters: parameters, Summary: summary,
		ConfirmationPhrase: fmt.Sprintf("确认执行 %d 条记录", len(items)),
	}, nil
}

// CancelPlan cancels a draft with an auditable reason.
func CancelPlan(planID int, cancelledBy string, reason string) error {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 5 {
		return errors.New("取消原因至少需要5个字符")
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

// DeletePlan permanently removes a draft or cancelled plan and its items.
func DeletePlan(planID int) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var plan model.QuotaPlan
		if err := model.LockForUpdate(tx).First(&plan, planID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("方案不存在")
			}
			return err
		}
		if plan.Status == model.QuotaPlanStatusExecuted {
			return errors.New("已执行方案不能删除")
		}
		if err := tx.Where("plan_id = ?", plan.Id).Delete(&model.QuotaItem{}).Error; err != nil {
			return err
		}
		return tx.Delete(&plan).Error
	})
}

// RegeneratePlan reuses a non-executed plan's reviewed settings with a current snapshot.
func RegeneratePlan(planID int, createdBy string) (*PlanResult, error) {
	if common.BatchUpdateEnabled {
		return nil, errors.New("批量额度更新已启用，请关闭 BATCH_UPDATE_ENABLED 并等待队列落库后重试")
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
		var oldParams PlanParameters
		if err := common.Unmarshal([]byte(plan.Parameters), &oldParams); err != nil {
			return fmt.Errorf("原方案参数无效: %w", err)
		}
		var oldParamValues map[string]interface{}
		if err := common.Unmarshal([]byte(plan.Parameters), &oldParamValues); err != nil {
			return fmt.Errorf("原方案参数无效: %w", err)
		}
		if _, exists := oldParamValues["reclaim_cap_percent"]; !exists {
			oldParams.ReclaimCapPercent = 30
		}
		if _, exists := oldParamValues["usage_bonus_percent"]; !exists {
			oldParams.UsageBonusPercent = 30
		}
		schedule := RecommendSchedule(cycle.CycleStartAt, cycle.CycleEndAt, now)
		stagePercent := plan.StagePercent
		if plan.PlanType == model.QuotaPlanTypeInitialization {
			stagePercent = 0
		} else if oldParams.ThoroughRelease {
			stagePercent = 10_000
		} else if stagePercent != 7_500 && stagePercent != 8_500 && stagePercent != 9_500 && stagePercent != 10_000 {
			stagePercent = schedule.Current.Percent * 100
		}
		if plan.PlanType == model.QuotaPlanTypeAdjustment &&
			stagePercent != 7_500 && stagePercent != 8_500 && stagePercent != 9_500 && stagePercent != 10_000 {
			return errors.New("当前时间尚未进入动态调配阶段")
		}
		nextAdjustment := cycle.CycleEndAt
		if !oldParams.ThoroughRelease {
			nextAdjustment = NextAdjustmentForStage(schedule.Events, stagePercent/100, now, cycle.CycleEndAt).Time
		}
		basisMode := oldParams.BasisMode
		if basisMode == "" {
			basisMode = basisModeActual
		}
		params := GenerateParams{
			CycleID: cycle.Id, PlanType: plan.PlanType, StagePercent: stagePercent,
			NextAdjustmentAt: &nextAdjustment, BasisMode: basisMode,
			EarlyReclaim: oldParams.EarlyReclaim, ReclaimCapPercent: oldParams.ReclaimCapPercent,
			UsageBonusPercent: oldParams.UsageBonusPercent, ThoroughRelease: oldParams.ThoroughRelease,
			CreatedBy: createdBy,
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

type calculationFields struct {
	DecreaseKind      string `json:"decrease_kind"`
	BaseQuota         string `json:"base_quota"`
	BonusQuota        string `json:"bonus_quota"`
	WeightedPoolQuota string `json:"weighted_pool_quota"`
}

func summarizePlan(plan model.QuotaPlan, items []model.QuotaItem) (DetailedPlanSummary, error) {
	stageCap, err := bigRatio([]int64{plan.BudgetQuotaSnapshot, int64(plan.StagePercent)}, []int64{10_000}, false)
	if err != nil {
		return DetailedPlanSummary{}, err
	}
	occupiedBefore, err := checkedAdd(plan.TotalSpendQuota, plan.ManagedBalanceQuota)
	if err != nil {
		return DetailedPlanSummary{}, err
	}
	var baseIncrease, supplement, bonus, weighted CategorySummary
	var ordinary, finalLow, disabled CategorySummary
	var increaseTotal, decreaseTotal int64
	for _, item := range items {
		fields, err := parseCalculationFields(item.CalculationData)
		if err != nil {
			return DetailedPlanSummary{}, fmt.Errorf("方案明细 %d 的计算数据无效: %w", item.Id, err)
		}
		baseQuota, err := parseCalculationQuota(fields.BaseQuota)
		if err != nil {
			return DetailedPlanSummary{}, err
		}
		bonusQuota, err := parseCalculationQuota(fields.BonusQuota)
		if err != nil {
			return DetailedPlanSummary{}, err
		}
		weightedQuota, err := parseCalculationQuota(fields.WeightedPoolQuota)
		if err != nil {
			return DetailedPlanSummary{}, err
		}
		if item.AdjustmentQuota > 0 {
			increaseTotal, err = checkedAdd(increaseTotal, item.AdjustmentQuota)
			if err != nil {
				return DetailedPlanSummary{}, err
			}
			if item.Action == model.QuotaAdjustmentActionIncrease && baseQuota > 0 {
				baseIncrease.Count++
				baseIncrease.Total, err = checkedAdd(baseIncrease.Total, baseQuota)
				if err != nil {
					return DetailedPlanSummary{}, err
				}
			}
			if item.Action == model.QuotaAdjustmentActionGrant || item.Action == model.QuotaAdjustmentActionInitialize {
				grant := baseQuota
				if grant <= 0 {
					grant = item.AdjustmentQuota
				}
				supplement.Count++
				supplement.Total, err = checkedAdd(supplement.Total, grant)
				if err != nil {
					return DetailedPlanSummary{}, err
				}
			}
		}
		if bonusQuota > 0 {
			bonus.Count++
			bonus.Total, err = checkedAdd(bonus.Total, bonusQuota)
			if err != nil {
				return DetailedPlanSummary{}, err
			}
		}
		if weightedQuota > 0 {
			weighted.Count++
			weighted.Total, err = checkedAdd(weighted.Total, weightedQuota)
			if err != nil {
				return DetailedPlanSummary{}, err
			}
		}
		if item.AdjustmentQuota >= 0 {
			continue
		}
		amount := -item.AdjustmentQuota
		decreaseTotal, err = checkedAdd(decreaseTotal, amount)
		if err != nil {
			return DetailedPlanSummary{}, err
		}
		switch {
		case fields.DecreaseKind == "disabled":
			disabled.Count++
			disabled.Total, err = checkedAdd(disabled.Total, amount)
		case fields.DecreaseKind == "final_low":
			finalLow.Count++
			finalLow.Total, err = checkedAdd(finalLow.Total, amount)
		case fields.DecreaseKind == "ordinary" || item.Action == model.QuotaAdjustmentActionInitialize:
			ordinary.Count++
			ordinary.Total, err = checkedAdd(ordinary.Total, amount)
		}
		if err != nil {
			return DetailedPlanSummary{}, err
		}
	}
	balanceAfter, err := checkedSum(plan.ManagedBalanceQuota, increaseTotal, -decreaseTotal)
	if err != nil {
		return DetailedPlanSummary{}, err
	}
	occupiedAfter, err := checkedAdd(plan.TotalSpendQuota, balanceAfter)
	if err != nil {
		return DetailedPlanSummary{}, err
	}
	availableBeforeReclaim := int64(0)
	if plan.PlanType != model.QuotaPlanTypeInitialization {
		availableBeforeReclaim = maxQuota(0, stageCap-occupiedBefore)
	}
	reclaimedUsedToCoverOverage := minQuota(decreaseTotal, maxQuota(0, occupiedBefore-stageCap))
	reclaimedAvailableForIncreases := decreaseTotal - reclaimedUsedToCoverOverage
	reclaimedUsedForIncreases := minQuota(increaseTotal, reclaimedAvailableForIncreases)
	stageSourceTotal := increaseTotal - reclaimedUsedForIncreases
	reclaimedUnused := maxQuota(0, decreaseTotal-reclaimedUsedToCoverOverage-reclaimedUsedForIncreases)
	return DetailedPlanSummary{
		BaseIncrease: baseIncrease, Supplement: supplement, Bonus: bonus, WeightedPool: weighted,
		Increase:         CategorySummary{Count: countPositiveItems(items), Total: increaseTotal},
		OrdinaryDecrease: ordinary, FinalLowDecrease: finalLow, DisabledDecrease: disabled,
		Decrease: CategorySummary{Count: countNegativeItems(items), Total: decreaseTotal},
		Affected: len(items), Budget: plan.BudgetQuotaSnapshot, StageCap: stageCap,
		TotalSpend: plan.TotalSpendQuota, ManagedBalance: plan.ManagedBalanceQuota,
		OccupiedBefore: occupiedBefore, OccupiedAfter: occupiedAfter, BalanceAfter: balanceAfter,
		AvailableBeforeReclaim: availableBeforeReclaim, StageSourceTotal: stageSourceTotal,
		ReclaimedUsedForIncreases:   reclaimedUsedForIncreases,
		ReclaimedUsedToCoverOverage: reclaimedUsedToCoverOverage,
		ReclaimedUnused:             reclaimedUnused,
		StageOriginalUnused:         maxQuota(0, availableBeforeReclaim-stageSourceTotal),
		StageRemaining:              maxQuota(0, stageCap-occupiedAfter),
		PoolRemaining:               maxQuota(0, plan.BudgetQuotaSnapshot-occupiedAfter),
		FutureReserved:              maxQuota(0, plan.BudgetQuotaSnapshot-stageCap),
		FinalStage:                  plan.StagePercent >= 10_000,
	}, nil
}

func parseCalculationFields(value string) (calculationFields, error) {
	if value == "" {
		return calculationFields{}, nil
	}
	var fields calculationFields
	err := common.Unmarshal([]byte(value), &fields)
	return fields, err
}

func parseCalculationQuota(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	quota, err := strconv.ParseInt(value, 10, 64)
	if err != nil || quota < 0 {
		return 0, errors.New("方案计算额度不正确")
	}
	return quota, nil
}

func countPositiveItems(items []model.QuotaItem) int {
	count := 0
	for _, item := range items {
		if item.AdjustmentQuota > 0 {
			count++
		}
	}
	return count
}

func countNegativeItems(items []model.QuotaItem) int {
	count := 0
	for _, item := range items {
		if item.AdjustmentQuota < 0 {
			count++
		}
	}
	return count
}
