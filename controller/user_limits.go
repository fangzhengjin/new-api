package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type userLimitOverrides struct {
	ModelRequestRateLimitCount        *int `json:"model_request_rate_limit_count"`
	ModelRequestRateLimitSuccessCount *int `json:"model_request_rate_limit_success_count"`
	ModelRequestConcurrencyLimit      *int `json:"model_request_concurrency_limit"`
	AccessSourceMaxIPs                *int `json:"access_source_max_ips"`
	AccessSourceSwitchCooldownMinutes *int `json:"access_source_switch_cooldown_minutes"`
}

type updateUserLimitsRequest userLimitOverrides

type accessSourceRequest struct {
	IP      string `json:"ip"`
	EventID string `json:"event_id"`
}

func validAccessSourceRequest(request accessSourceRequest, eventRequired bool) bool {
	if request.IP == "" || len(request.IP) > 45 {
		return false
	}
	return !eventRequired || (request.EventID != "" && len(request.EventID) <= 64)
}

func manageableLimitUser(c *gin.Context) (*model.User, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return nil, false
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	if !canManageTargetRole(c.GetInt("role"), user.Role) {
		common.ApiErrorI18n(c, i18n.MsgUserNoPermissionSameLevel)
		return nil, false
	}
	return user, true
}

func userLimitsData(c *gin.Context, user *model.User) (gin.H, error) {
	userSetting := user.GetSetting()
	accountLimits := setting.ResolveModelRequestRateLimits(user.Group, userSetting)
	concurrencyLimits := setting.ResolveModelRequestConcurrencyLimits(userSetting)
	accessLimits := setting.ResolveAccessSourceLimits(userSetting)
	state, err := service.GetAccessSourceState(c.Request.Context(), user.Id, accessLimits)
	if err != nil {
		return nil, err
	}
	return gin.H{
		"user": gin.H{"id": user.Id, "username": user.Username},
		"overrides": userLimitOverrides{
			ModelRequestRateLimitCount:        userSetting.ModelRequestRateLimitCount,
			ModelRequestRateLimitSuccessCount: userSetting.ModelRequestRateLimitSuccessCount,
			ModelRequestConcurrencyLimit:      userSetting.ModelRequestConcurrencyLimit,
			AccessSourceMaxIPs:                userSetting.AccessSourceMaxIPs,
			AccessSourceSwitchCooldownMinutes: userSetting.AccessSourceSwitchCooldownMinutes,
		},
		"effective": gin.H{
			"model_request_rate_limit_count":          accountLimits.TotalCount,
			"model_request_rate_limit_count_source":   accountLimits.TotalSource,
			"model_request_rate_limit_success_count":  accountLimits.SuccessCount,
			"model_request_rate_limit_success_source": accountLimits.SuccessSource,
			"model_request_concurrency_limit":         concurrencyLimits.AccountLimit,
			"model_request_concurrency_limit_source":  concurrencyLimits.AccountSource,
			"access_source_max_ips":                   accessLimits.MaxIPsPerUser,
			"access_source_max_ips_source":            accessLimits.MaxIPsPerUserSource,
			"access_source_switch_cooldown_minutes":   accessLimits.SwitchCooldownMinutes,
			"access_source_switch_cooldown_source":    accessLimits.SwitchCooldownSource,
		},
		"global": gin.H{
			"model_request_rate_limit_enabled":          setting.ModelRequestRateLimitEnabled,
			"model_request_rate_limit_duration_minutes": setting.ModelRequestRateLimitDurationMinutes,
			"model_request_ip_rate_limit_count":         setting.ModelRequestIPRateLimitCount,
			"model_request_ip_rate_limit_success_count": setting.ModelRequestIPRateLimitSuccessCount,
			"model_request_concurrency_limit_enabled":   concurrencyLimits.Enabled,
			"model_request_ip_concurrency_limit":        concurrencyLimits.IPLimit,
			"access_source_limit_enabled":               accessLimits.Enabled,
			"access_source_association_window_hours":    accessLimits.AssociationWindowHours,
			"access_source_max_users_per_ip":            accessLimits.MaxUsersPerIP,
		},
		"access_source_state": state,
	}, nil
}

func GetUserLimits(c *gin.Context) {
	user, ok := manageableLimitUser(c)
	if !ok {
		return
	}
	data, err := userLimitsData(c, user)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func UpdateUserLimits(c *gin.Context) {
	user, ok := manageableLimitUser(c)
	if !ok {
		return
	}
	var request updateUserLimitsRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil ||
		!validOptionalLimit(request.ModelRequestRateLimitCount, setting.MaxModelRequestRateLimitCount) ||
		!validOptionalLimit(request.ModelRequestRateLimitSuccessCount, setting.MaxModelRequestRateLimitCount) ||
		!validOptionalLimit(request.ModelRequestConcurrencyLimit, int64(setting.MaxModelRequestConcurrency)) ||
		!validOptionalLimit(request.AccessSourceMaxIPs, setting.MaxAccessSourceAssociations) ||
		!validOptionalLimit(request.AccessSourceSwitchCooldownMinutes, setting.MaxAccessSourceSwitchCooldownMinutes) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.PatchUserSetting(user.Id, func(current *dto.UserSetting) {
		current.ModelRequestRateLimitCount = request.ModelRequestRateLimitCount
		current.ModelRequestRateLimitSuccessCount = request.ModelRequestRateLimitSuccessCount
		current.ModelRequestConcurrencyLimit = request.ModelRequestConcurrencyLimit
		current.AccessSourceMaxIPs = request.AccessSourceMaxIPs
		current.AccessSourceSwitchCooldownMinutes = request.AccessSourceSwitchCooldownMinutes
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, user.Id, "user.limit_settings_update", map[string]interface{}{
		"username":  user.Username,
		"overrides": request,
	})
	common.ApiSuccess(c, nil)
}

func RemoveUserAccessSource(c *gin.Context) {
	user, ok := manageableLimitUser(c)
	if !ok {
		return
	}
	var request accessSourceRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || !validAccessSourceRequest(request, false) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	removed, err := service.RemoveAccessSourceAssociation(c.Request.Context(), user.Id, request.IP)
	if err != nil {
		if errors.Is(err, service.ErrAccessSourceInvalidIP) {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		if errors.Is(err, service.ErrAccessSourceCurrent) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": i18n.T(c, i18n.MsgAccessSourceCurrentCannotRemove)})
			return
		}
		common.ApiError(c, err)
		return
	}
	if !removed {
		common.ApiErrorI18n(c, i18n.MsgAccessSourceAssociationNotFound)
		return
	}
	recordManageAuditFor(c, user.Id, "user.access_source_remove", map[string]interface{}{"username": user.Username, "ip": request.IP})
	common.ApiSuccess(c, nil)
}

func AllowLatestUserAccessSource(c *gin.Context) {
	user, ok := manageableLimitUser(c)
	if !ok {
		return
	}
	var request accessSourceRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || !validAccessSourceRequest(request, true) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	pending, err := service.AllowLatestAccessSource(c.Request.Context(), user.Id, request.IP, request.EventID)
	if err != nil {
		if errors.Is(err, service.ErrAccessSourceInvalidIP) {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		if errors.Is(err, service.ErrAccessSourcePendingChanged) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": i18n.T(c, i18n.MsgAccessSourcePendingChanged)})
			return
		}
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, user.Id, "user.access_source_allow", map[string]interface{}{
		"username": user.Username,
		"ip":       pending.IP,
		"reason":   pending.Reason,
	})
	common.ApiSuccess(c, nil)
}

func validOptionalLimit(value *int, max int64) bool {
	return value == nil || (*value >= 0 && int64(*value) <= max)
}
