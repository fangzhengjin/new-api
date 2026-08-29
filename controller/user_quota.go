package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	quotaService "github.com/QuantumNous/new-api/service/quota"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func manageUserQuota(c *gin.Context, req ManageRequest) {
	if model.CycleQuotaManagementEnabled() {
		if req.Id <= 0 || (req.Mode != "add" && req.Mode != "subtract") {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		if req.Value <= 0 {
			common.ApiErrorI18n(c, i18n.MsgUserQuotaChangeZero)
			return
		}
		user, err := model.GetUserById(req.Id, false)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if !canManageTargetRole(c.GetInt("role"), user.Role) {
			common.ApiErrorI18n(c, i18n.MsgUserNoPermissionHigherLevel)
			return
		}
		adjustment := int64(req.Value)
		if req.Mode == "subtract" {
			adjustment = -adjustment
		}
		result, err := quotaService.ManualAdjustUserQuota(user.Id, c.GetInt("role"), adjustment, req.Reason, c.GetString("username"))
		if err != nil {
			if errors.Is(err, model.ErrUserQuotaPermission) {
				common.ApiErrorI18n(c, i18n.MsgUserNoPermissionHigherLevel)
				return
			}
			common.ApiError(c, err)
			return
		}
		recordManageAuditFor(c, user.Id, "user.quota_adjustment_plan", map[string]interface{}{
			"target_user_id": user.Id, "plan_id": result.PlanID, "adjustment_quota": quotaString(adjustment),
			"affected_users": result.AffectedUsers, "reason": strings.TrimSpace(req.Reason),
		})
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
		return
	}
	action := "generic"
	params := model.AuditFields{
		"target_user_id":  req.Id,
		"mode":            req.Mode,
		"requested_quota": req.Value,
	}
	switch req.Mode {
	case "add":
		action = "user.quota_add"
	case "subtract":
		action = "user.quota_subtract"
	case "override":
		action = "user.quota_override"
	default:
		params["action"] = "add_quota"
		params["method"] = c.Request.Method
		params["route"] = c.FullPath()
	}
	success := false
	defer func() {
		content := auditContentEN(action, params)
		if !success {
			// Failed requests have no committed balance changes to render.
			content = "Failed user quota adjustment"
		}
		model.RecordOperationAuditLog(c.GetInt("id"), c.GetInt("role"), content, c.ClientIP(), action, params,
			auditOperatorInfo(c), &model.AuditRequestInfo{
				Method: c.Request.Method, Route: c.FullPath(), Status: c.Writer.Status(), Success: success,
			}, c)
		markAuditLogged(c)
	}()

	adjustment, err := model.AdjustUserQuota(req.Id, c.GetInt("role"), req.Mode, req.Value)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrInvalidUserQuotaAdjustment):
			params["failure_reason"] = "invalid_parameters"
			if (req.Mode == "add" || req.Mode == "subtract") && req.Value <= 0 {
				common.ApiErrorI18n(c, i18n.MsgUserQuotaChangeZero)
			} else {
				common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			}
		case errors.Is(err, model.ErrUserQuotaPermission):
			params["failure_reason"] = "permission_denied"
			common.ApiErrorI18n(c, i18n.MsgUserNoPermissionHigherLevel)
		case errors.Is(err, gorm.ErrRecordNotFound):
			params["failure_reason"] = "target_not_found"
			common.ApiErrorI18n(c, i18n.MsgUserNotExists)
		case errors.Is(err, model.ErrWalletQuotaLimitExceeded):
			params["failure_reason"] = "quota_limit_exceeded"
			common.ApiError(c, err)
		default:
			params["failure_reason"] = "database_error"
			common.ApiError(c, err)
		}
		return
	}

	params["target_username"] = adjustment.Username
	params["from"] = adjustment.Before
	params["to"] = adjustment.After
	if req.Mode != "override" {
		params["quota"] = req.Value
	}
	success = true
	operation := model.AuditOperation{Action: action, Params: params}
	model.RecordLogWithAdminInfo(adjustment.UserID, model.LogTypeTopup,
		auditContentEN(action, params), auditOperatorInfo(c), &operation, c)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}
