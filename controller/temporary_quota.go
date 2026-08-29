package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	quotaService "github.com/QuantumNous/new-api/service/quota"

	"github.com/gin-gonic/gin"
)

func temporaryQuotaRequestResponse(request model.TemporaryQuotaRequest) dto.TemporaryQuotaRequestResponse {
	return dto.TemporaryQuotaRequestResponse{
		ID: request.Id, UserID: request.UserId,
		Username: request.Username, DisplayName: request.DisplayName,
		RequestedQuota: quotaString(request.RequestedQuota), Project: request.Project, Reason: request.Reason,
		Status: string(request.Status), ReviewReason: request.ReviewReason, CreatedAt: request.CreatedAt,
	}
}

func temporaryQuotaRequestResponses(requests []model.TemporaryQuotaRequest) []dto.TemporaryQuotaRequestResponse {
	responses := make([]dto.TemporaryQuotaRequestResponse, 0, len(requests))
	for _, request := range requests {
		responses = append(responses, temporaryQuotaRequestResponse(request))
	}
	return responses
}

func selfTemporaryQuotaRequestResponse(request model.TemporaryQuotaRequest) dto.SelfTemporaryQuotaRequestResponse {
	return dto.SelfTemporaryQuotaRequestResponse{
		ID: request.Id, RequestedQuota: quotaString(request.RequestedQuota), Project: request.Project,
		Reason: request.Reason, Status: string(request.Status), ApprovedQuota: quotaString(request.ApprovedQuota),
		ReviewReason: request.ReviewReason, CreatedAt: request.CreatedAt,
		ReviewedAt: request.ReviewedAt, ExecutedAt: request.ExecutedAt,
	}
}

func selfTemporaryQuotaRequestResponses(requests []model.TemporaryQuotaRequest) []dto.SelfTemporaryQuotaRequestResponse {
	responses := make([]dto.SelfTemporaryQuotaRequestResponse, 0, len(requests))
	for _, request := range requests {
		responses = append(responses, selfTemporaryQuotaRequestResponse(request))
	}
	return responses
}

// GetSelfTemporaryQuota returns only the current user's application availability.
func GetSelfTemporaryQuota(c *gin.Context) {
	overview, err := quotaService.GetTemporaryQuotaOverview(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, dto.SelfTemporaryQuotaOverviewResponse{
		CurrentQuota: quotaString(overview.CurrentQuota),
		CanRequest:   overview.CanRequest, UnavailableReason: overview.UnavailableReason, Projects: overview.Projects,
	})
}

// GetSelfTemporaryQuotaRequests returns only the authenticated user's application history.
func GetSelfTemporaryQuotaRequests(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	requests, total, err := quotaService.ListTemporaryQuotaRequestsForUser(
		c.GetInt("id"), pageInfo.GetStartIdx(), pageInfo.GetPageSize(), c.Query("status"), c.Query("keyword"),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(selfTemporaryQuotaRequestResponses(requests))
	common.ApiSuccess(c, pageInfo)
}

// SubmitSelfTemporaryQuota creates or returns one idempotent user request.
func SubmitSelfTemporaryQuota(c *gin.Context) {
	var request dto.TemporaryQuotaCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	quota, err := quotaService.ParsePositiveQuota(request.RequestedQuota, "申请额度")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := quotaService.SubmitTemporaryQuotaRequest(quotaService.TemporaryQuotaRequestInput{
		UserID: c.GetInt("id"), IdempotencyKey: request.IdempotencyKey,
		RequestedQuota: quota, Project: request.Project, Reason: request.Reason,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"request": selfTemporaryQuotaRequestResponse(*result),
	})
}

// GetTemporaryQuotaRequests returns the administrator review queue and history.
func GetTemporaryQuotaRequests(c *gin.Context) {
	requests, err := quotaService.ListTemporaryQuotaRequests()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, temporaryQuotaRequestResponses(requests))
}

// ApproveTemporaryQuotaRequest atomically approves and executes a pending request.
func ApproveTemporaryQuotaRequest(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	var request dto.TemporaryQuotaDecisionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	approvedQuota, err := quotaService.ParsePositiveQuota(request.ApprovedQuota, "批准额度")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := quotaService.ApproveTemporaryQuotaRequest(id, approvedQuota, c.GetString("username"), request.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, result.UserId, "quota.temporary_request.approve", map[string]interface{}{
		"target_user_id": result.UserId, "request_id": result.Id, "plan_id": result.PlanId,
		"approved_quota": quotaString(result.ApprovedQuota), "reason": result.ReviewReason,
		"project": result.Project,
	})
	common.ApiSuccess(c, gin.H{
		"request": temporaryQuotaRequestResponse(*result),
	})
}

// RejectTemporaryQuotaRequest rejects a pending request without changing quota.
func RejectTemporaryQuotaRequest(c *gin.Context) {
	id, ok := quotaPathID(c)
	if !ok {
		return
	}
	var request dto.TemporaryQuotaDecisionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	result, err := quotaService.RejectTemporaryQuotaRequest(id, c.GetString("username"), request.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, result.UserId, "quota.temporary_request.reject", map[string]interface{}{
		"target_user_id": result.UserId, "request_id": result.Id, "reason": result.ReviewReason,
		"project": result.Project,
	})
	common.ApiSuccess(c, temporaryQuotaRequestResponse(*result))
}
