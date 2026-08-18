package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	quotaService "github.com/QuantumNous/new-api/service/quota"

	"github.com/gin-gonic/gin"
)

func quotaRecoveryRequestResponse(request model.QuotaRecoveryRequest) dto.QuotaRecoveryRequestResponse {
	return dto.QuotaRecoveryRequestResponse{
		ID: request.Id, CycleID: request.CycleId, UserID: request.UserId,
		Username: request.Username, DisplayName: request.DisplayName,
		RequestedQuota: quotaString(request.RequestedQuota), Reason: request.Reason,
		Status: string(request.Status), Decision: string(request.Decision), ApprovedQuota: quotaString(request.ApprovedQuota),
		PlanID: request.PlanId, ReviewedBy: request.ReviewedBy, ReviewReason: request.ReviewReason,
		CreatedAt: request.CreatedAt, ReviewedAt: request.ReviewedAt, ExecutedAt: request.ExecutedAt,
	}
}

func quotaRecoveryRequestResponses(requests []model.QuotaRecoveryRequest) []dto.QuotaRecoveryRequestResponse {
	responses := make([]dto.QuotaRecoveryRequestResponse, 0, len(requests))
	for _, request := range requests {
		responses = append(responses, quotaRecoveryRequestResponse(request))
	}
	return responses
}

// GetSelfQuotaRecovery returns only the current user's recovery policy and history.
func GetSelfQuotaRecovery(c *gin.Context) {
	overview, err := quotaService.GetRecoveryOverview(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"cycle_id": overview.Cycle.Id, "current_quota": quotaString(overview.CurrentQuota),
		"auto_recovery_enabled":         overview.Cycle.AutoRecoveryEnabled,
		"auto_recovery_single_quota":    quotaString(overview.Cycle.AutoRecoverySingleQuota),
		"auto_recovery_threshold_quota": quotaString(overview.Cycle.AutoRecoveryThresholdQuota),
		"auto_recovery_max_count":       overview.Cycle.AutoRecoveryMaxCount,
		"auto_recovery_max_quota":       quotaString(overview.Cycle.AutoRecoveryMaxQuota),
		"reserve_available":             overview.ReserveLeftQuota > 0,
		"requests":                      quotaRecoveryRequestResponses(overview.Requests),
	})
}

// SubmitSelfQuotaRecovery creates or returns one idempotent user request.
func SubmitSelfQuotaRecovery(c *gin.Context) {
	var request dto.QuotaRecoveryCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	quota, err := quotaService.ParsePositiveQuota(request.RequestedQuota, "申请额度")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := quotaService.SubmitRecoveryRequest(quotaService.RecoveryRequestInput{
		UserID: c.GetInt("id"), IdempotencyKey: request.IdempotencyKey,
		RequestedQuota: quota, Reason: request.Reason,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"request": quotaRecoveryRequestResponse(result.Request), "notifications": result.Notifications,
	})
}

// GetQuotaRecoveryRequests returns the administrator review queue and history.
func GetQuotaRecoveryRequests(c *gin.Context) {
	requests, err := quotaService.ListRecoveryRequests()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, quotaRecoveryRequestResponses(requests))
}

// ApproveQuotaRecoveryRequest atomically approves and executes a pending request.
func ApproveQuotaRecoveryRequest(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	var request dto.QuotaRecoveryDecisionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	approvedQuota, err := quotaService.ParsePositiveQuota(request.ApprovedQuota, "批准额度")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := quotaService.ApproveRecoveryRequest(id, approvedQuota, c.GetString("username"), request.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, result.Request.UserId, "quota.recovery.approve", map[string]interface{}{
		"request_id": result.Request.Id, "plan_id": result.Request.PlanId,
	})
	common.ApiSuccess(c, gin.H{
		"request": quotaRecoveryRequestResponse(result.Request), "notifications": result.Notifications,
	})
}

// RejectQuotaRecoveryRequest rejects a pending request without changing quota.
func RejectQuotaRecoveryRequest(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	var request dto.QuotaRecoveryDecisionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	result, err := quotaService.RejectRecoveryRequest(id, c.GetString("username"), request.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, result.UserId, "quota.recovery.reject", map[string]interface{}{"request_id": result.Id})
	common.ApiSuccess(c, quotaRecoveryRequestResponse(*result))
}

// GetQuotaAlgorithmStatus returns the evidence-backed switch state.
func GetQuotaAlgorithmStatus(c *gin.Context) {
	status, err := quotaService.GetAlgorithmStatus()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"legacy_version": status.LegacyVersion, "current_version": status.CurrentVersion, "candidate_version": status.CandidateVersion,
		"enable_confirmation_phrase": status.EnableConfirmation, "rollback_confirmation_phrase": status.RollbackConfirmation,
		"required_qualified_cycles": status.RequiredQualifiedCycles,
		"qualified_cycle_ids":       status.QualifiedCycleIDs, "active_cycle_id": status.ActiveCycleID,
		"recovery_ready": status.RecoveryReady, "draft_count": status.DraftCount,
		"can_switch": status.CanSwitch, "rollback_allowed": status.RollbackAllowed,
		"can_record_evidence": status.CanRecordEvidence, "blockers": status.Blockers,
	})
}

// RecordQuotaShadowEvidence stores a final-window comparison without plan or balance writes.
func RecordQuotaShadowEvidence(c *gin.Context) {
	var request dto.QuotaPlanGenerateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	evidence, err := quotaService.RecordShadowEvidence(quotaGenerateParams(request, c.GetString("username")), c.GetString("username"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "quota.algorithm.evidence", map[string]interface{}{
		"cycle_id": evidence.CycleId, "qualified": evidence.Qualified,
	})
	common.ApiSuccess(c, gin.H{
		"cycle_id": evidence.CycleId, "snapshot_at": evidence.SnapshotAt,
		"candidate_algorithm_version": evidence.CandidateAlgorithmVersion, "qualified": evidence.Qualified,
	})
}

// SwitchQuotaAlgorithm applies the candidate switch or its bounded rollback.
func SwitchQuotaAlgorithm(c *gin.Context) {
	var request dto.QuotaAlgorithmSwitchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	status, err := quotaService.SwitchProductionAlgorithm(request.TargetVersion, request.Confirmation, c.GetString("username"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "quota.algorithm.switch", map[string]interface{}{"target_version": request.TargetVersion})
	common.ApiSuccess(c, gin.H{
		"legacy_version": status.LegacyVersion, "current_version": status.CurrentVersion, "can_switch": status.CanSwitch,
		"enable_confirmation_phrase": status.EnableConfirmation, "rollback_confirmation_phrase": status.RollbackConfirmation,
		"rollback_allowed": status.RollbackAllowed, "blockers": status.Blockers,
	})
}
