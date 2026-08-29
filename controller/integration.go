package controller

import (
	"encoding/base64"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

const integrationLaunchTTL = time.Minute

type integrationLaunchPayload struct {
	TokenID int `json:"token_id"`
}

func integrationRequestError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"success": false, "code": code, "message": message})
}

func LaunchIntegration(c *gin.Context) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		integrationRequestError(c, http.StatusForbidden, "INTEGRATION_SESSION_REQUIRED", "请使用当前登录会话打开")
		return
	}
	var request struct {
		PresetName string `json:"preset_name"`
		TokenID    int    `json:"token_id"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || strings.TrimSpace(request.PresetName) == "" || request.TokenID <= 0 {
		integrationRequestError(c, http.StatusBadRequest, "INTEGRATION_REQUEST_INVALID", "请选择可用的 Token")
		return
	}
	preset, ok := setting.GetChatPreset(request.PresetName)
	if !ok || !preset.Enabled || !preset.RequiresBackendLaunch() {
		integrationRequestError(c, http.StatusNotFound, "INTEGRATION_UNAVAILABLE", "聊天预设未启用或不需要授权")
		return
	}
	token, err := model.GetTokenByIds(request.TokenID, identity.UserID)
	if err != nil {
		integrationRequestError(c, http.StatusBadRequest, "INTEGRATION_TOKEN_INVALID", "Token 不存在或不属于当前用户")
		return
	}
	if err = model.ValidateTokenAvailability(token); err != nil {
		integrationRequestError(c, http.StatusBadRequest, "INTEGRATION_TOKEN_UNAVAILABLE", "Token 已停用、过期或额度不足")
		return
	}

	variables := setting.ChatPresetVariables{Address: strings.TrimRight(system_setting.ServerAddress, "/")}
	if preset.RequiresModelList() {
		user, userErr := model.GetUserCache(identity.UserID)
		if userErr != nil {
			common.ApiError(c, userErr)
			return
		}
		if user.Status != common.UserStatusEnabled {
			integrationRequestError(c, http.StatusForbidden, "INTEGRATION_USER_UNAVAILABLE", "当前用户不可用")
			return
		}
		usingGroup, groupErr := service.ResolveTokenGroup(user.Group, token.Group)
		if groupErr != nil {
			integrationRequestError(c, http.StatusForbidden, "INTEGRATION_TOKEN_GROUP_FORBIDDEN", groupErr.Error())
			return
		}
		user.WriteContext(c)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
		if err = middleware.SetupContextForToken(c, token); err != nil {
			integrationRequestError(c, http.StatusForbidden, "INTEGRATION_TOKEN_INVALID", err.Error())
			return
		}
		models, modelErr := getUserOpenAIModels(c)
		if modelErr != nil {
			common.ApiError(c, modelErr)
			return
		}
		modelVariables := classifyChatPresetModels(models)
		variables.TextModels = modelVariables.TextModels
		variables.ImageModels = modelVariables.ImageModels
		variables.VideoModels = modelVariables.VideoModels
	}
	if preset.RequiresAuthCode() {
		payload, marshalErr := common.Marshal(integrationLaunchPayload{TokenID: token.Id})
		if marshalErr != nil {
			common.ApiError(c, marshalErr)
			return
		}
		variables.AuthCode, _, err = model.CreateAuthFlow(model.AuthFlowCreate{
			Purpose:   model.AuthFlowPurposeIntegrationLaunch,
			UserId:    identity.UserID,
			SessionId: identity.SessionID,
			Payload:   string(payload),
			ExpiresAt: time.Now().Add(integrationLaunchTTL),
		})
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	launchURL, err := preset.RenderURL(variables)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"launch_url": launchURL})
}

func ExchangeIntegrationCode(c *gin.Context) {
	var request struct {
		Code string `json:"code"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || strings.TrimSpace(request.Code) == "" {
		integrationRequestError(c, http.StatusBadRequest, "INTEGRATION_CODE_REQUIRED", "缺少一次性授权码")
		return
	}
	request.Code = strings.TrimSpace(request.Code)
	decoded, decodeErr := base64.RawURLEncoding.DecodeString(request.Code)
	if decodeErr != nil || len(decoded) != model.AuthFlowTokenBytes {
		integrationRequestError(c, http.StatusUnauthorized, "INTEGRATION_CODE_INVALID", "一次性授权码无效、已过期或已使用")
		return
	}
	flow, err := model.GetAuthFlow(request.Code, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeIntegrationLaunch})
	if err != nil {
		integrationRequestError(c, http.StatusUnauthorized, "INTEGRATION_CODE_INVALID", "一次性授权码无效、已过期或已使用")
		return
	}
	if _, err = service.ValidateSessionReference(flow.UserId, flow.SessionId); err != nil {
		integrationRequestError(c, http.StatusUnauthorized, "INTEGRATION_SESSION_INVALID", "原登录会话已失效，请返回重新打开")
		return
	}
	var payload integrationLaunchPayload
	if err = common.UnmarshalJsonStr(flow.Payload, &payload); err != nil || payload.TokenID <= 0 {
		integrationRequestError(c, http.StatusUnauthorized, "INTEGRATION_CODE_INVALID", "一次性授权码无效")
		return
	}
	token, err := model.GetTokenByIds(payload.TokenID, flow.UserId)
	if err != nil {
		integrationRequestError(c, http.StatusUnauthorized, "INTEGRATION_TOKEN_INVALID", "Token 已删除")
		return
	}
	if err = model.ValidateTokenAvailability(token); err != nil {
		integrationRequestError(c, http.StatusUnauthorized, "INTEGRATION_TOKEN_UNAVAILABLE", "Token 已停用、过期或额度不足")
		return
	}
	if _, err = model.ConsumeAuthFlow(request.Code, model.AuthFlowMatch{
		Purpose:   model.AuthFlowPurposeIntegrationLaunch,
		UserId:    flow.UserId,
		SessionId: flow.SessionId,
	}); err != nil {
		if errors.Is(err, model.ErrAuthFlowInvalid) || errors.Is(err, model.ErrAuthFlowExpired) || errors.Is(err, model.ErrAuthFlowConsumed) {
			integrationRequestError(c, http.StatusUnauthorized, "INTEGRATION_CODE_INVALID", "一次性授权码无效、已过期或已使用")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"base_url":   strings.TrimRight(system_setting.ServerAddress, "/"),
		"api_key":    "sk-" + token.GetFullKey(),
		"api_format": "openai",
	})
}

func classifyChatPresetModels(models []dto.OpenAIModels) setting.ChatPresetVariables {
	textModels := make(map[string]struct{})
	imageModels := make(map[string]struct{})
	videoModels := make(map[string]struct{})
	for _, item := range models {
		for _, endpoint := range item.SupportedEndpointTypes {
			switch endpoint {
			case constant.EndpointTypeOpenAI, constant.EndpointTypeOpenAIResponse,
				constant.EndpointTypeOpenAIResponseCompact, constant.EndpointTypeOpenAIAlphaSearch,
				constant.EndpointTypeAnthropic, constant.EndpointTypeGemini:
				textModels[item.Id] = struct{}{}
			case constant.EndpointTypeImageGeneration:
				imageModels[item.Id] = struct{}{}
			case constant.EndpointTypeOpenAIVideo:
				videoModels[item.Id] = struct{}{}
			}
		}
	}
	return setting.ChatPresetVariables{
		TextModels:  sortedModelNames(textModels),
		ImageModels: sortedModelNames(imageModels),
		VideoModels: sortedModelNames(videoModels),
	}
}

func sortedModelNames(models map[string]struct{}) []string {
	result := make([]string, 0, len(models))
	for name := range models {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
