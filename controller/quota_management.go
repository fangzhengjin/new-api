package controller

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	quotaService "github.com/QuantumNous/new-api/service/quota"

	"github.com/gin-gonic/gin"
)

func quotaString(value int64) string {
	return strconv.FormatInt(value, 10)
}

func quotaCycleResponse(cycle model.QuotaCycle) dto.QuotaCycleResponse {
	return dto.QuotaCycleResponse{
		ID: cycle.Id, CycleStartAt: cycle.CycleStartAt, CycleEndAt: cycle.CycleEndAt,
		BudgetQuota: quotaString(cycle.BudgetQuota), InitialGrantQuota: quotaString(cycle.InitialGrantQuota),
		RecoveryReserveQuota:       quotaString(cycle.RecoveryReserveQuota),
		AutoRecoveryEnabled:        cycle.AutoRecoveryEnabled,
		AutoRecoverySingleQuota:    quotaString(cycle.AutoRecoverySingleQuota),
		AutoRecoveryThresholdQuota: quotaString(cycle.AutoRecoveryThresholdQuota),
		AutoRecoveryMaxCount:       cycle.AutoRecoveryMaxCount,
		AutoRecoveryMaxQuota:       quotaString(cycle.AutoRecoveryMaxQuota),
		AllocationAlgorithmVersion: cycle.AllocationAlgorithmVersion,
		LegacyRollbackAllowed:      cycle.LegacyRollbackAllowed,
		BalancePolicy:              string(cycle.BalancePolicy), Status: string(cycle.Status),
		SettlementPlanID: cycle.SettlementPlanId, SettledAt: cycle.SettledAt,
		RestoredAt: cycle.RestoredAt, RestoredBy: cycle.RestoredBy,
		CreatedAt: cycle.CreatedAt, CreatedBy: cycle.CreatedBy,
		UpdatedAt: cycle.UpdatedAt, UpdatedBy: cycle.UpdatedBy,
	}
}

func quotaGenerateParams(request dto.QuotaPlanGenerateRequest, createdBy string) quotaService.GenerateParams {
	nextAdjustmentAt := request.NextAdjustmentAt
	return quotaService.GenerateParams{
		CycleID: request.CycleID, PlanType: model.QuotaPlanType(request.PlanType), StagePercent: request.StagePercent,
		NextAdjustmentAt: &nextAdjustmentAt, BasisMode: request.BasisMode,
		EarlyReclaim: request.EarlyReclaim, ReclaimCapPercent: request.ReclaimCapPercent,
		UsageBonusPercent: request.UsageBonusPercent, ThoroughRelease: request.ThoroughRelease,
		CreatedBy: createdBy,
	}
}

func parseQuotaRecoveryPolicy(enabled bool, singleRaw, thresholdRaw string, maxCount int, maxRaw string) (quotaService.RecoveryPolicy, error) {
	if !enabled {
		return quotaService.RecoveryPolicy{}, nil
	}
	single, err := quotaService.ParsePositiveQuota(singleRaw, "自动恢复单次上限")
	if err != nil {
		return quotaService.RecoveryPolicy{}, err
	}
	threshold, err := quotaService.ParsePositiveQuota(thresholdRaw, "自动恢复余额门槛")
	if err != nil {
		return quotaService.RecoveryPolicy{}, err
	}
	maximum, err := quotaService.ParsePositiveQuota(maxRaw, "自动恢复每用户总额上限")
	if err != nil {
		return quotaService.RecoveryPolicy{}, err
	}
	return quotaService.RecoveryPolicy{
		Enabled: true, SingleQuota: single, ThresholdQuota: threshold,
		MaxCount: maxCount, MaxQuota: maximum,
	}, nil
}

func quotaPlanResponse(plan model.QuotaPlan) (dto.QuotaPlanResponse, error) {
	parameters := map[string]interface{}{}
	if plan.Parameters != "" {
		if err := common.Unmarshal([]byte(plan.Parameters), &parameters); err != nil {
			return dto.QuotaPlanResponse{}, err
		}
	}
	return dto.QuotaPlanResponse{
		ID: plan.Id, CycleID: plan.CycleId, PlanType: string(plan.PlanType), StagePercent: plan.StagePercent,
		SnapshotAt: plan.SnapshotAt, NextAdjustmentAt: plan.NextAdjustmentAt,
		AlgorithmVersion: plan.AlgorithmVersion, Parameters: parameters,
		BudgetQuotaSnapshot: quotaString(plan.BudgetQuotaSnapshot), TotalSpendQuota: quotaString(plan.TotalSpendQuota),
		ManagedBalanceQuota: quotaString(plan.ManagedBalanceQuota), PlannedDeltaQuota: quotaString(plan.PlannedDeltaQuota),
		Status: string(plan.Status), CreatedAt: plan.CreatedAt, CreatedBy: plan.CreatedBy,
		ExecutedAt: plan.ExecutedAt, ExecutedBy: plan.ExecutedBy,
		CancelledAt: plan.CancelledAt, CancelledBy: plan.CancelledBy, CancelReason: plan.CancelReason,
	}, nil
}

func quotaItemResponse(item model.QuotaItem) (dto.QuotaItemResponse, error) {
	calculationData := map[string]interface{}{}
	if item.CalculationData != "" {
		if err := common.Unmarshal([]byte(item.CalculationData), &calculationData); err != nil {
			return dto.QuotaItemResponse{}, err
		}
	}
	var actualBefore, actualAfter *string
	if item.ActualBeforeQuota != nil {
		value := quotaString(*item.ActualBeforeQuota)
		actualBefore = &value
	}
	if item.ActualAfterQuota != nil {
		value := quotaString(*item.ActualAfterQuota)
		actualAfter = &value
	}
	return dto.QuotaItemResponse{
		ID: item.Id, PlanID: item.PlanId, UserID: item.UserId, Username: item.Username,
		DisplayName: item.DisplayName, Email: item.Email, Action: string(item.Action),
		SnapshotBalanceQuota: quotaString(item.SnapshotBalanceQuota), AdjustmentQuota: quotaString(item.AdjustmentQuota),
		RetainedQuota: quotaString(item.RetainedQuota), CalculationData: calculationData, BasisText: item.BasisText,
		ActualBeforeQuota: actualBefore, ActualAfterQuota: actualAfter,
		LogContent: item.LogContent, LogStatus: string(item.LogStatus), LogSentAt: item.LogSentAt, LogError: item.LogError,
		EmailStatus: string(item.EmailStatus), EmailSentAt: item.EmailSentAt, EmailError: item.EmailError,
	}, nil
}

func quotaCategoryResponse(category quotaService.CategorySummary) dto.QuotaCategoryResponse {
	return dto.QuotaCategoryResponse{Count: category.Count, Total: quotaString(category.Total)}
}

func quotaSummaryResponse(summary quotaService.DetailedPlanSummary) dto.QuotaPlanSummaryResponse {
	return dto.QuotaPlanSummaryResponse{
		BaseIncrease: quotaCategoryResponse(summary.BaseIncrease), Supplement: quotaCategoryResponse(summary.Supplement),
		Bonus: quotaCategoryResponse(summary.Bonus), WeightedPool: quotaCategoryResponse(summary.WeightedPool),
		Increase: quotaCategoryResponse(summary.Increase), OrdinaryDecrease: quotaCategoryResponse(summary.OrdinaryDecrease),
		FinalLowDecrease: quotaCategoryResponse(summary.FinalLowDecrease), DisabledDecrease: quotaCategoryResponse(summary.DisabledDecrease),
		Decrease: quotaCategoryResponse(summary.Decrease), Affected: summary.Affected,
		Budget: quotaString(summary.Budget), StageCap: quotaString(summary.StageCap), TotalSpend: quotaString(summary.TotalSpend),
		ManagedBalance: quotaString(summary.ManagedBalance), OccupiedBefore: quotaString(summary.OccupiedBefore),
		OccupiedAfter: quotaString(summary.OccupiedAfter), BalanceAfter: quotaString(summary.BalanceAfter),
		AvailableBeforeReclaim: quotaString(summary.AvailableBeforeReclaim), StageSourceTotal: quotaString(summary.StageSourceTotal),
		ReclaimedUsedForIncreases:   quotaString(summary.ReclaimedUsedForIncreases),
		ReclaimedUsedToCoverOverage: quotaString(summary.ReclaimedUsedToCoverOverage),
		ReclaimedUnused:             quotaString(summary.ReclaimedUnused), StageOriginalUnused: quotaString(summary.StageOriginalUnused),
		StageRemaining: quotaString(summary.StageRemaining), PoolRemaining: quotaString(summary.PoolRemaining),
		FutureReserved: quotaString(summary.FutureReserved), RecoveryReserve: quotaString(summary.RecoveryReserve),
		FinalStage: summary.FinalStage,
	}
}

func quotaFairnessMetricsResponse(metrics quotaService.FairnessMetrics) dto.QuotaFairnessMetricsResponse {
	return dto.QuotaFairnessMetricsResponse{
		Population: metrics.Population, MinimumCoverage: metrics.MinimumCoverageBasisPoints,
		P10Coverage: metrics.P10CoverageBasisPoints, P50Coverage: metrics.P50CoverageBasisPoints,
		P90Coverage: metrics.P90CoverageBasisPoints, MinimumSafetyCoverage: metrics.MinimumSafetyBasisPoints,
		SafetyUnmet: metrics.SafetyUnmet, NewUserCount: metrics.NewUserCount,
		NewUserCoverage: metrics.NewUserCoverageBasisPoints, ReclaimedQuota: quotaString(metrics.ReclaimedQuota),
		RecoveryReserveQuota: quotaString(metrics.RecoveryReserveQuota), OccupiedAfterQuota: quotaString(metrics.OccupiedAfterQuota),
	}
}

func quotaFairnessShadowResponse(result *quotaService.FairnessShadowComparison) dto.QuotaFairnessShadowResponse {
	items := make([]dto.QuotaFairnessShadowItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, dto.QuotaFairnessShadowItemResponse{
			UserID: item.UserID, Username: item.Username, CurrentBalanceQuota: quotaString(item.CurrentBalanceQuota),
			SafetyTargetQuota: quotaString(item.SafetyTargetQuota), DemandTargetQuota: quotaString(item.DemandTargetQuota),
			TargetQuota: quotaString(item.TargetQuota), CurrentAdjustmentQuota: quotaString(item.CurrentAdjustmentQuota),
			CandidateAdjustmentQuota: quotaString(item.CandidateAdjustmentQuota), CurrentAfterQuota: quotaString(item.CurrentAfterQuota),
			CandidateAfterQuota: quotaString(item.CandidateAfterQuota), CurrentCoverage: item.CurrentCoverageBasisPoints,
			CandidateCoverage: item.CandidateCoverageBasisPoints,
		})
	}
	return dto.QuotaFairnessShadowResponse{
		SnapshotAt: result.SnapshotAt, StageCapQuota: quotaString(result.StageCapQuota),
		CurrentAlgorithmVersion: result.CurrentAlgorithmVersion, CandidateAlgorithmVersion: result.CandidateAlgorithmVersion,
		CandidateQualified: result.CandidateQualified, Current: quotaFairnessMetricsResponse(result.Current),
		Candidate: quotaFairnessMetricsResponse(result.Candidate), Items: items,
	}
}

func quotaPathID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "ID 不正确")
		return 0, false
	}
	return id, true
}

func quotaRecommendationResponse(recommendation *quotaService.InitialGrantRecommendation) interface{} {
	if recommendation == nil {
		return nil
	}
	return gin.H{
		"quota":           quotaString(recommendation.Quota),
		"reference_start": recommendation.ReferenceStart, "reference_end": recommendation.ReferenceEnd,
		"user_count": recommendation.UserCount, "total_spend": quotaString(recommendation.TotalSpend),
		"average_spend": quotaString(recommendation.AverageSpend), "buffered": quotaString(recommendation.Buffered),
		"stability_floor": quotaString(recommendation.StabilityFloor),
	}
}

// GetQuotaCycles returns recent cycles, create defaults, and the historical initial-grant recommendation.
func GetQuotaCycles(c *gin.Context) {
	result, err := quotaService.ListCycles()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cycles := make([]dto.QuotaCycleResponse, 0, len(result.Cycles))
	for _, cycle := range result.Cycles {
		cycles = append(cycles, quotaCycleResponse(cycle))
	}
	common.ApiSuccess(c, gin.H{
		"cycles": cycles, "defaults": result.Defaults,
		"recommendation": quotaRecommendationResponse(result.Recommendation),
	})
}

// CreateQuotaCycle creates a non-overlapping scheduled quota cycle.
func CreateQuotaCycle(c *gin.Context) {
	var request dto.QuotaCycleCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	budget, err := quotaService.ParsePositiveQuota(request.BudgetQuota, "采购总额")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	initialGrant, err := quotaService.ParsePositiveQuota(request.InitialGrantQuota, "人均首次额度")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recoveryReserve := int64(0)
	if request.RecoveryReserveQuota != "" {
		recoveryReserve, err = quotaService.ParseNonNegativeQuotaTotal(request.RecoveryReserveQuota, "小额恢复池")
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	recoveryPolicy, err := parseQuotaRecoveryPolicy(
		request.AutoRecoveryEnabled, request.AutoRecoverySingleQuota,
		request.AutoRecoveryThresholdQuota, request.AutoRecoveryMaxCount, request.AutoRecoveryMaxQuota,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cycle, err := quotaService.CreateCycle(quotaService.CreateCycleParams{
		StartAt: request.CycleStartAt, EndAt: request.CycleEndAt,
		BudgetQuota: budget, InitialGrantQuota: initialGrant, RecoveryReserveQuota: recoveryReserve,
		RecoveryPolicy: recoveryPolicy,
		BalancePolicy:  model.QuotaCycleBalancePolicy(request.BalancePolicy), CreatedBy: c.GetString("username"),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, quotaCycleResponse(*cycle))
}

// GetQuotaCycle returns one cycle and all related plans.
func GetQuotaCycle(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	detail, err := quotaService.GetCycleDetail(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	plans := make([]dto.QuotaPlanResponse, 0, len(detail.Plans))
	for _, plan := range detail.Plans {
		response, err := quotaPlanResponse(plan)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		plans = append(plans, response)
	}
	common.ApiSuccess(c, gin.H{
		"cycle": quotaCycleResponse(detail.Cycle), "plans": plans,
		"recommendation": quotaRecommendationResponse(detail.Recommendation),
	})
}

// UpdateQuotaCycle updates the budget and, for scheduled cycles, the initial grant.
func UpdateQuotaCycle(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	var request dto.QuotaCycleUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	budget, err := quotaService.ParsePositiveQuota(request.BudgetQuota, "采购总额")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var initialGrant *int64
	if request.InitialGrantQuota != nil {
		value, err := quotaService.ParsePositiveQuota(*request.InitialGrantQuota, "人均首次额度")
		if err != nil {
			common.ApiError(c, err)
			return
		}
		initialGrant = &value
	}
	var recoveryReserve *int64
	if request.RecoveryReserveQuota != nil {
		value, err := quotaService.ParseNonNegativeQuotaTotal(*request.RecoveryReserveQuota, "小额恢复池")
		if err != nil {
			common.ApiError(c, err)
			return
		}
		recoveryReserve = &value
	}
	var recoveryPolicy *quotaService.RecoveryPolicy
	hasRecoveryPolicy := request.AutoRecoveryEnabled != nil || request.AutoRecoverySingleQuota != nil ||
		request.AutoRecoveryThresholdQuota != nil || request.AutoRecoveryMaxCount != nil || request.AutoRecoveryMaxQuota != nil
	if hasRecoveryPolicy {
		if request.AutoRecoveryEnabled == nil || request.AutoRecoverySingleQuota == nil ||
			request.AutoRecoveryThresholdQuota == nil || request.AutoRecoveryMaxCount == nil || request.AutoRecoveryMaxQuota == nil {
			common.ApiErrorMsg(c, "自动恢复策略字段必须完整提交")
			return
		}
		value, err := parseQuotaRecoveryPolicy(
			*request.AutoRecoveryEnabled, *request.AutoRecoverySingleQuota,
			*request.AutoRecoveryThresholdQuota, *request.AutoRecoveryMaxCount, *request.AutoRecoveryMaxQuota,
		)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		recoveryPolicy = &value
	}
	if err := quotaService.UpdateCycleSettings(id, budget, initialGrant, recoveryReserve, recoveryPolicy, c.GetString("username")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// CloseQuotaCycle closes a cycle and cancels its remaining drafts.
func CloseQuotaCycle(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	if err := quotaService.CloseCycle(id, c.GetString("username")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// RestoreQuotaCycleSettlement restores one unchanged reset snapshot into the active cycle.
func RestoreQuotaCycleSettlement(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	var request dto.QuotaCycleRestoreRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	confirmation := fmt.Sprintf("确认恢复周期 #%d 清零余额", id)
	if request.Confirmation != confirmation {
		common.ApiErrorMsg(c, "确认短语不正确")
		return
	}
	result, err := quotaService.RestoreCycleSettlement(id, c.GetString("username"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "quota.cycle.restore", map[string]interface{}{"cycle_id": id})
	common.ApiSuccess(c, result)
}

// GetQuotaPlans returns recent plans with an optional cycle filter.
func GetQuotaPlans(c *gin.Context) {
	var cycleID *int
	if raw := c.Query("cycle_id"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			common.ApiErrorMsg(c, "cycle_id 不正确")
			return
		}
		cycleID = &value
	}
	plans, err := quotaService.ListPlans(cycleID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	responses := make([]dto.QuotaPlanResponse, 0, len(plans))
	for _, plan := range plans {
		response, err := quotaPlanResponse(plan)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		responses = append(responses, response)
	}
	common.ApiSuccess(c, responses)
}

// GetQuotaPlanOptions returns the active cycle, recommended schedule, and initialization gate.
func GetQuotaPlanOptions(c *gin.Context) {
	options, err := quotaService.GetPlanOptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if options.Cycle == nil {
		common.ApiSuccess(c, gin.H{"cycle": nil, "schedule": nil, "initialization_required": true})
		return
	}
	stagePercent := options.Schedule.Current.Percent * 100
	if options.InitializationRequired {
		stagePercent = 0
	} else if stagePercent == 0 {
		stagePercent = 7_500
	}
	earlyReclaim := true
	reclaimCapPercent, usageBonusPercent := 30, 30
	defaultPlanType := string(model.QuotaPlanTypeAdjustment)
	if options.InitializationRequired {
		defaultPlanType = string(model.QuotaPlanTypeInitialization)
	}
	common.ApiSuccess(c, gin.H{
		"cycle": quotaCycleResponse(*options.Cycle), "schedule": options.Schedule,
		"initialization_required": options.InitializationRequired,
		"defaults": gin.H{
			"plan_type":     defaultPlanType,
			"stage_percent": stagePercent, "next_adjustment_at": options.Schedule.Next.Time,
			"basis_mode": "actual", "early_reclaim": earlyReclaim,
			"reclaim_cap_percent": reclaimCapPercent, "usage_bonus_percent": usageBonusPercent,
		},
	})
}

// GenerateQuotaPlan creates an immutable initialization or adjustment draft.
func GenerateQuotaPlan(c *gin.Context) {
	if !model.CompanyQuotaModeEnabled() {
		common.ApiErrorMsg(c, "公司额度模式未启用")
		return
	}
	var request dto.QuotaPlanGenerateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	result, err := quotaService.GeneratePlan(quotaGenerateParams(request, c.GetString("username")))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	plan, err := quotaPlanResponse(result.Plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"plan_id": result.Plan.Id, "plan": plan})
}

// CompareQuotaPlanFairness compares the current and candidate algorithms without saving a draft.
func CompareQuotaPlanFairness(c *gin.Context) {
	var request dto.QuotaPlanGenerateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	result, err := quotaService.CompareFairness(quotaGenerateParams(request, c.GetString("username")))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, quotaFairnessShadowResponse(result))
}

// GetQuotaPlan returns one plan, its cycle, fund summary, items, and confirmation phrase.
func GetQuotaPlan(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	detail, err := quotaService.GetPlanDetail(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	plan, err := quotaPlanResponse(detail.Plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]dto.QuotaItemResponse, 0, len(detail.Items))
	for _, item := range detail.Items {
		response, err := quotaItemResponse(item)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		items = append(items, response)
	}
	common.ApiSuccess(c, gin.H{
		"plan": plan, "cycle": quotaCycleResponse(detail.Cycle), "items": items,
		"summary": quotaSummaryResponse(detail.Summary), "confirmation_phrase": detail.ConfirmationPhrase,
	})
}

// ExecuteQuotaPlan applies a draft only when the exact server-generated phrase is supplied.
func ExecuteQuotaPlan(c *gin.Context) {
	if !model.CompanyQuotaModeEnabled() {
		common.ApiErrorMsg(c, "公司额度模式未启用")
		return
	}
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	var request dto.QuotaPlanExecuteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	detail, err := quotaService.GetPlanDetail(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if request.Confirmation != detail.ConfirmationPhrase {
		common.ApiErrorMsg(c, "确认短语不正确")
		return
	}
	result, err := quotaService.ExecutePlan(id, c.GetString("username"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// CancelQuotaPlan cancels a draft with an auditable reason.
func CancelQuotaPlan(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	var request dto.QuotaPlanCancelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := quotaService.CancelPlan(id, c.GetString("username"), request.Reason); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// DeleteQuotaPlan permanently removes a non-executed plan and its items.
func DeleteQuotaPlan(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	if err := quotaService.DeletePlan(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// RegenerateQuotaPlan creates a fresh draft from a non-executed plan's saved settings.
func RegenerateQuotaPlan(c *gin.Context) {
	if !model.CompanyQuotaModeEnabled() {
		common.ApiErrorMsg(c, "公司额度模式未启用")
		return
	}
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	result, err := quotaService.RegeneratePlan(id, c.GetString("username"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"plan_id": result.Plan.Id})
}

// RetryQuotaPlanNotifications retries only incomplete log and email outbox rows.
func RetryQuotaPlanNotifications(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	result := quotaService.RetryNotifications(id)
	if result.Error != "" {
		common.ApiError(c, errors.New(result.Error))
		return
	}
	common.ApiSuccess(c, result)
}

func ManualQuotaAdjust(c *gin.Context) {
	var request struct {
		UserID int    `json:"user_id"`
		Target string `json:"target_quota"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	target, err := quotaService.ParseNonNegativeQuota(request.Target, "目标额度")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := quotaService.ManualAdjustUserQuota(request.UserID, target, request.Reason, c.GetString("username"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, request.UserID, "user.quota_adjustment_plan", map[string]interface{}{
		"affected_users": result.AffectedUsers,
	})
	common.ApiSuccess(c, result)
}
