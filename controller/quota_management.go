package controller

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

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
	var openingAllocated, allocated, remaining, allocationError *string
	if err := quotaService.CycleAllocationError(&cycle); err != nil {
		message := err.Error()
		allocationError = &message
	} else {
		if cycle.OpeningAllocatedQuota != nil {
			value := quotaString(*cycle.OpeningAllocatedQuota)
			openingAllocated = &value
		}
		if cycle.AllocatedQuota != nil {
			value := quotaString(*cycle.AllocatedQuota)
			allocated = &value
			remainingValue := quotaString(cycle.BudgetQuota - *cycle.AllocatedQuota)
			remaining = &remainingValue
		}
	}
	return dto.QuotaCycleResponse{
		ID: cycle.Id, CycleStartAt: cycle.CycleStartAt, CycleEndAt: cycle.CycleEndAt,
		BudgetQuota: quotaString(cycle.BudgetQuota), InitialGrantQuota: quotaString(cycle.InitialGrantQuota),
		OpeningAllocatedQuota: openingAllocated, AllocatedQuota: allocated, RemainingQuota: remaining,
		AllocationError:                          allocationError,
		TemporaryQuotaReserve:                    quotaString(cycle.TemporaryQuotaReserve),
		TemporaryQuotaAutoApprovalEnabled:        cycle.TemporaryQuotaAutoApprovalEnabled,
		TemporaryQuotaAutoApprovalSingleQuota:    quotaString(cycle.TemporaryQuotaAutoApprovalSingleQuota),
		TemporaryQuotaAutoApprovalThresholdQuota: quotaString(cycle.TemporaryQuotaAutoApprovalThresholdQuota),
		TemporaryQuotaAutoApprovalMaxCount:       cycle.TemporaryQuotaAutoApprovalMaxCount,
		TemporaryQuotaAutoApprovalMaxQuota:       quotaString(cycle.TemporaryQuotaAutoApprovalMaxQuota),
		ConcentrationMultiplier:                  cycle.ConcentrationMultiplier,
		BalancePolicy:                            string(cycle.BalancePolicy),
		CarryOverCountsTowardBudget: cycle.BalancePolicy == model.QuotaCycleBalancePolicyCarry &&
			cycle.CarryOverCountsTowardBudget,
		Status:    string(cycle.Status),
		CreatedAt: cycle.CreatedAt, CreatedBy: cycle.CreatedBy,
		UpdatedAt: cycle.UpdatedAt, UpdatedBy: cycle.UpdatedBy,
	}
}

func quotaGenerateParams(request dto.QuotaPlanGenerateRequest, createdBy string) quotaService.GenerateParams {
	nextAdjustmentAt := request.NextAdjustmentAt
	return quotaService.GenerateParams{
		CycleID: request.CycleID, Name: request.Name, Purpose: request.Purpose,
		PlanType: model.QuotaPlanType(request.PlanType), NextAdjustmentAt: &nextAdjustmentAt,
		CreatedBy: createdBy,
	}
}

func quotaPlanResponse(plan model.QuotaPlan) dto.QuotaPlanResponse {
	var allocationAfter *string
	if plan.AllocationAfterQuota != nil {
		value := quotaString(*plan.AllocationAfterQuota)
		allocationAfter = &value
	}
	return dto.QuotaPlanResponse{
		ID: plan.Id, CycleID: plan.CycleId, Name: plan.Name, Purpose: plan.Purpose,
		PlanType:   string(plan.PlanType),
		SnapshotAt: plan.SnapshotAt, NextAdjustmentAt: plan.NextAdjustmentAt,
		PlannedDeltaQuota: quotaString(plan.PlannedDeltaQuota), AllocationAfterQuota: allocationAfter,
		Status: string(plan.Status), CreatedAt: plan.CreatedAt,
		ExecutedAt: plan.ExecutedAt, CancelledAt: plan.CancelledAt, CancelReason: plan.CancelReason,
	}
}

func quotaItemResponse(item model.QuotaItem) dto.QuotaItemResponse {
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
		ID: item.Id, UserID: item.UserId, Username: item.Username,
		DisplayName: item.DisplayName, Action: string(item.Action),
		SnapshotBalanceQuota: quotaString(item.SnapshotBalanceQuota), AdjustmentQuota: quotaString(item.AdjustmentQuota),
		BasisText:         item.BasisText,
		ActualBeforeQuota: actualBefore, ActualAfterQuota: actualAfter,
		LogStatus: string(item.LogStatus), EmailStatus: string(item.EmailStatus),
	}
}

func quotaCategoryResponse(category quotaService.CategorySummary) dto.QuotaCategoryResponse {
	return dto.QuotaCategoryResponse{Count: category.Count, Total: quotaString(category.Total)}
}

func quotaSummaryResponse(summary quotaService.DetailedPlanSummary) dto.QuotaPlanSummaryResponse {
	return dto.QuotaPlanSummaryResponse{
		Increase:                    quotaCategoryResponse(summary.Increase),
		Decrease:                    quotaCategoryResponse(summary.Decrease),
		Affected:                    summary.Affected,
		Budget:                      quotaString(summary.Budget),
		OccupiedAfter:               quotaString(summary.OccupiedAfter),
		CyclePoolIssuedQuota:        quotaString(summary.CyclePoolIssuedQuota),
		ReclaimedUsedForIncreases:   quotaString(summary.ReclaimedUsedForIncreases),
		ReclaimedUsedToCoverOverage: quotaString(summary.ReclaimedUsedToCoverOverage),
		ReclaimedUnused:             quotaString(summary.ReclaimedUnused),
		PoolRemaining:               quotaString(summary.PoolRemaining),
	}
}

func quotaExecuteResultResponse(result *quotaService.ExecuteResult) gin.H {
	return gin.H{
		"issued_quota":    quotaString(result.IssuedQuota),
		"reclaimed_quota": quotaString(result.ReclaimedQuota),
		"allocated_quota": quotaString(result.AllocatedQuota),
		"remaining_quota": quotaString(result.RemainingQuota),
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
		"user_count":    recommendation.UserCount,
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

// GetQuotaOverview returns the current cycle quota allocation snapshot.
func GetQuotaOverview(c *gin.Context) {
	overview, err := quotaService.GetQuotaOverview()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response := gin.H{
		"updated_at":                    overview.UpdatedAt,
		"allocation_ready":              overview.AllocationReady,
		"pending_temporary_quota_count": overview.PendingTemporaryQuotaCount,
		"pending_allocation_count":      overview.PendingAllocationCount,
		"pending_reclaim_count":         overview.PendingReclaimCount,
	}
	if overview.PreviousCycle != nil {
		response["previous_cycle"] = quotaCycleResponse(*overview.PreviousCycle)
	}
	if overview.CurrentCycle != nil {
		response["current_cycle"] = quotaCycleResponse(*overview.CurrentCycle)
	}
	if overview.NextCycle != nil {
		response["next_cycle"] = quotaCycleResponse(*overview.NextCycle)
	}
	if overview.CurrentPlan != nil {
		response["current_plan"] = quotaPlanResponse(*overview.CurrentPlan)
	}
	if overview.CurrentPlanSummary != nil {
		response["current_plan_summary"] = quotaSummaryResponse(*overview.CurrentPlanSummary)
	}
	common.ApiSuccess(c, response)
}

// GetQuotaActivities returns recent quota-management actions without exposing
// the audit log's IP address or administrator-only metadata.
func GetQuotaActivities(c *gin.Context) {
	logs, _, err := model.GetAllLogs(model.LogTypeManage, 0, 0, "", "", "", 0, 200, 0, "", "", "")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	activities := make([]gin.H, 0, 50)
	targetIDs := make(map[int]struct{})
	activityTargetIDs := make([]int, 0, 50)
	for _, log := range logs {
		if strings.TrimSpace(log.Other) == "" {
			continue
		}
		var other struct {
			Operation struct {
				Action string                 `json:"action"`
				Params map[string]interface{} `json:"params"`
			} `json:"op"`
		}
		if err := common.UnmarshalJsonStr(log.Other, &other); err != nil {
			common.SysError("failed to parse quota activity log: " + err.Error())
			continue
		}
		action := other.Operation.Action
		if !strings.HasPrefix(action, "quota.") && action != "user.quota_adjustment_plan" && action != "user.quota_whitelist" {
			continue
		}
		if other.Operation.Params == nil {
			other.Operation.Params = map[string]interface{}{}
		}
		targetID := 0
		if value, ok := other.Operation.Params["target_user_id"].(float64); ok &&
			value > 0 && value <= float64(^uint(0)>>1) && value == float64(int(value)) {
			targetID = int(value)
		}
		if targetID > 0 {
			targetIDs[targetID] = struct{}{}
		}
		activities = append(activities, gin.H{
			"id": log.Id, "action": action, "params": other.Operation.Params,
			"operator_id": log.UserId, "operator": log.Username,
			"operator_display_name": log.DisplayName, "created_at": log.CreatedAt,
		})
		activityTargetIDs = append(activityTargetIDs, targetID)
		if len(activities) == cap(activities) {
			break
		}
	}
	if len(targetIDs) > 0 {
		ids := make([]int, 0, len(targetIDs))
		for id := range targetIDs {
			ids = append(ids, id)
		}
		var users []struct {
			ID          int
			Username    string
			DisplayName string
		}
		if err := model.DB.Unscoped().Model(&model.User{}).Select("id, username, display_name").Where("id IN ?", ids).Find(&users).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		usersByID := make(map[int]gin.H, len(users))
		for _, user := range users {
			usersByID[user.ID] = gin.H{"id": user.ID, "username": user.Username, "display_name": user.DisplayName}
		}
		for index, targetID := range activityTargetIDs {
			if targetID == 0 {
				continue
			}
			target := gin.H{"id": targetID}
			if user, ok := usersByID[targetID]; ok {
				target = user
			}
			activities[index]["target"] = target
		}
	}
	common.ApiSuccess(c, activities)
}

// CreateQuotaCycle creates a non-overlapping scheduled quota cycle.
func CreateQuotaCycle(c *gin.Context) {
	var request dto.QuotaCycleCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	budget, err := quotaService.ParsePositiveQuota(request.BudgetQuota, "周期总额度")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	initialGrant, err := quotaService.ParsePositiveQuota(request.InitialGrantQuota, "人均首次额度")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	temporaryQuotaReserve := int64(0)
	if request.TemporaryQuotaReserve != "" {
		temporaryQuotaReserve, err = quotaService.ParseNonNegativeQuotaTotal(request.TemporaryQuotaReserve, "临时额度预留")
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	temporaryQuotaPolicy, err := quotaService.ParseTemporaryQuotaAutoApprovalPolicy(
		request.TemporaryQuotaAutoApprovalEnabled, request.TemporaryQuotaAutoApprovalSingleQuota,
		request.TemporaryQuotaAutoApprovalThresholdQuota, request.TemporaryQuotaAutoApprovalMaxCount, request.TemporaryQuotaAutoApprovalMaxQuota,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cycle, err := quotaService.CreateCycle(quotaService.CreateCycleParams{
		StartAt: request.CycleStartAt, EndAt: request.CycleEndAt,
		BudgetQuota: budget, InitialGrantQuota: initialGrant, TemporaryQuotaReserve: temporaryQuotaReserve,
		TemporaryQuotaPolicy:        temporaryQuotaPolicy,
		ConcentrationMultiplier:     request.ConcentrationMultiplier,
		BalancePolicy:               model.QuotaCycleBalancePolicy(request.BalancePolicy),
		CarryOverCountsTowardBudget: request.CarryOverCountsTowardBudget,
		CreatedBy:                   c.GetString("username"),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "quota.cycle.create", map[string]interface{}{"cycle_id": cycle.Id})
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
		plans = append(plans, quotaPlanResponse(plan))
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
	budget, err := quotaService.ParsePositiveQuota(request.BudgetQuota, "周期总额度")
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
	var temporaryQuotaReserve *int64
	if request.TemporaryQuotaReserve != nil {
		value, err := quotaService.ParseNonNegativeQuotaTotal(*request.TemporaryQuotaReserve, "临时额度预留")
		if err != nil {
			common.ApiError(c, err)
			return
		}
		temporaryQuotaReserve = &value
	}
	var temporaryQuotaPolicy *quotaService.TemporaryQuotaAutoApprovalPolicy
	hasTemporaryQuotaPolicy := request.TemporaryQuotaAutoApprovalEnabled != nil || request.TemporaryQuotaAutoApprovalSingleQuota != nil ||
		request.TemporaryQuotaAutoApprovalThresholdQuota != nil || request.TemporaryQuotaAutoApprovalMaxCount != nil || request.TemporaryQuotaAutoApprovalMaxQuota != nil
	if hasTemporaryQuotaPolicy {
		if request.TemporaryQuotaAutoApprovalEnabled == nil || request.TemporaryQuotaAutoApprovalSingleQuota == nil ||
			request.TemporaryQuotaAutoApprovalThresholdQuota == nil || request.TemporaryQuotaAutoApprovalMaxCount == nil || request.TemporaryQuotaAutoApprovalMaxQuota == nil {
			common.ApiErrorMsg(c, "自动发放设置必须完整提交")
			return
		}
		value, err := quotaService.ParseTemporaryQuotaAutoApprovalPolicy(
			*request.TemporaryQuotaAutoApprovalEnabled, *request.TemporaryQuotaAutoApprovalSingleQuota,
			*request.TemporaryQuotaAutoApprovalThresholdQuota, *request.TemporaryQuotaAutoApprovalMaxCount, *request.TemporaryQuotaAutoApprovalMaxQuota,
		)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		temporaryQuotaPolicy = &value
	}
	if err := quotaService.UpdateCycleSettings(
		id, budget, initialGrant, temporaryQuotaReserve, temporaryQuotaPolicy, c.GetString("username"),
	); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "quota.cycle.update", map[string]interface{}{"cycle_id": id})
	common.ApiSuccess(c, nil)
}

// CloseQuotaCycle closes a cycle and cancels its remaining drafts.
func CloseQuotaCycle(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	status, err := quotaService.CloseCycle(id, c.GetString("username"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "quota.cycle.close", map[string]interface{}{"cycle_id": id})
	common.ApiSuccess(c, map[string]interface{}{"status": status})
}

// GetQuotaPlanOptions returns the active cycle, recommended schedule, and initialization gate.
func GetQuotaPlanOptions(c *gin.Context) {
	options, err := quotaService.GetPlanOptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if options.Cycle == nil {
		common.ApiSuccess(c, gin.H{"cycle": nil, "initialization_required": true})
		return
	}
	defaultPlanType := string(model.QuotaPlanTypeAdjustment)
	if options.InitializationRequired {
		defaultPlanType = string(model.QuotaPlanTypeInitialization)
	}
	common.ApiSuccess(c, gin.H{
		"cycle":                   quotaCycleResponse(*options.Cycle),
		"initialization_required": options.InitializationRequired,
		"defaults": gin.H{
			"plan_type": defaultPlanType, "next_adjustment_at": options.NextAdjustmentAt,
		},
	})
}

// GenerateQuotaPlan creates an immutable initialization or adjustment draft.
func GenerateQuotaPlan(c *gin.Context) {
	var request dto.QuotaPlanGenerateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Purpose = strings.TrimSpace(request.Purpose)
	if request.Name == "" || utf8.RuneCountInString(request.Name) > quotaService.PlanNameMaxRunes {
		common.ApiErrorMsg(c, "方案名称不能为空且不得超过64个字符")
		return
	}
	if request.Purpose == "" || utf8.RuneCountInString(request.Purpose) > quotaService.PlanPurposeMaxRunes {
		common.ApiErrorMsg(c, "方案用途不能为空且不得超过200个字符")
		return
	}
	result, err := quotaService.GeneratePlan(quotaGenerateParams(request, c.GetString("username")))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "quota.plan.generate", map[string]interface{}{
		"plan_id": result.Plan.Id, "cycle_id": result.Plan.CycleId, "plan_type": result.Plan.PlanType,
	})
	common.ApiSuccess(c, gin.H{"plan_id": result.Plan.Id, "plan": quotaPlanResponse(result.Plan)})
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
	items := make([]dto.QuotaItemResponse, 0, len(detail.Items))
	for _, item := range detail.Items {
		items = append(items, quotaItemResponse(item))
	}
	common.ApiSuccess(c, gin.H{
		"plan": quotaPlanResponse(detail.Plan), "cycle": quotaCycleResponse(detail.Cycle), "items": items,
		"summary": quotaSummaryResponse(detail.Summary), "confirmation_phrase": detail.ConfirmationPhrase,
	})
}

// ExecuteQuotaPlan applies a draft only when the exact server-generated phrase is supplied.
func ExecuteQuotaPlan(c *gin.Context) {
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
	recordManageAudit(c, "quota.plan.execute", map[string]interface{}{
		"plan_id": id, "affected_users": result.AffectedUsers,
		"issued_quota": quotaString(result.IssuedQuota), "reclaimed_quota": quotaString(result.ReclaimedQuota),
		"net_change_quota": quotaString(result.NetChangeQuota),
	})
	common.ApiSuccess(c, quotaExecuteResultResponse(result))
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
	recordManageAudit(c, "quota.plan.cancel", map[string]interface{}{"plan_id": id, "reason": request.Reason})
	common.ApiSuccess(c, nil)
}

// RegenerateQuotaPlan creates a fresh draft from a non-executed plan's saved settings.
func RegenerateQuotaPlan(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	result, err := quotaService.RegeneratePlan(id, c.GetString("username"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "quota.plan.regenerate", map[string]interface{}{"source_plan_id": id, "plan_id": result.Plan.Id})
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
	recordManageAudit(c, "quota.plan.notifications_retry", map[string]interface{}{
		"plan_id": id, "logs_sent": result.LogsSent, "emails_sent": result.EmailsSent,
	})
	common.ApiSuccess(c, result)
}
