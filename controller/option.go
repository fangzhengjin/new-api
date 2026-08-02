package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

var completionRatioMetaOptionKeys = []string{
	"ModelPrice",
	"ModelRatio",
	"CompletionRatio",
	"CacheRatio",
	"CreateCacheRatio",
	"ImageRatio",
	"AudioRatio",
	"AudioCompletionRatio",
}

func isPaymentComplianceOptionKey(key string) bool {
	return strings.HasPrefix(key, "payment_setting.compliance_")
}

func isPositiveOptionValue(value string) bool {
	intValue, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil {
		return intValue > 0
	}
	floatValue, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return err == nil && floatValue > 0
}

func collectModelNamesFromOptionValue(raw string, modelNames map[string]struct{}) {
	if strings.TrimSpace(raw) == "" {
		return
	}

	var parsed map[string]any
	if err := common.UnmarshalJsonStr(raw, &parsed); err != nil {
		return
	}

	for modelName := range parsed {
		modelNames[modelName] = struct{}{}
	}
}

func buildCompletionRatioMetaValue(optionValues map[string]string) string {
	modelNames := make(map[string]struct{})
	for _, key := range completionRatioMetaOptionKeys {
		collectModelNamesFromOptionValue(optionValues[key], modelNames)
	}

	meta := make(map[string]ratio_setting.CompletionRatioInfo, len(modelNames))
	for modelName := range modelNames {
		meta[modelName] = ratio_setting.GetCompletionRatioInfo(modelName)
	}

	jsonBytes, err := common.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(jsonBytes)
}

func GetOptions(c *gin.Context) {
	var options []*model.Option
	optionValues := make(map[string]string)
	common.OptionMapRWMutex.Lock()
	for k, v := range common.OptionMap {
		if k == "theme.frontend" {
			continue
		}
		value := common.Interface2String(v)
		isSensitiveKey := strings.HasSuffix(k, "Token") ||
			strings.HasSuffix(k, "Secret") ||
			strings.HasSuffix(k, "Key") ||
			strings.HasSuffix(k, "secret") ||
			strings.HasSuffix(k, "api_key")
		if isSensitiveKey {
			continue
		}
		options = append(options, &model.Option{
			Key:   k,
			Value: value,
		})
		for _, optionKey := range completionRatioMetaOptionKeys {
			if optionKey == k {
				optionValues[k] = value
				break
			}
		}
	}
	common.OptionMapRWMutex.Unlock()
	options = append(options, &model.Option{
		Key:   "CompletionRatioMeta",
		Value: buildCompletionRatioMetaValue(optionValues),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
}

type OptionUpdateRequest struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

func normalizeOptionValue(value any) string {
	switch value := value.(type) {
	case bool:
		return common.Interface2String(value)
	case float64:
		return common.Interface2String(value)
	case int:
		return common.Interface2String(value)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func optionValue(values map[string]string, key string, current string) string {
	if value, ok := values[key]; ok {
		return value
	}
	return current
}

func validateRatioOption(value string) error {
	var ratios map[string]float64
	return common.UnmarshalJsonStr(value, &ratios)
}

func validateOptionUpdate(c *gin.Context, key string, value string, values map[string]string) bool {
	switch key {
	case "QuotaForInviter", "QuotaForInvitee":
		if isPositiveOptionValue(value) && !operation_setting.IsPaymentComplianceConfirmed() {
			common.ApiErrorI18n(c, i18n.MsgPaymentComplianceRequired)
			return false
		}
	default:
		if isPaymentComplianceOptionKey(key) {
			common.ApiErrorMsg(c, "合规确认字段不允许通过通用设置接口修改")
			return false
		}
	}

	var err error
	switch key {
	case "GitHubOAuthEnabled":
		if value == "true" && optionValue(values, "GitHubClientId", common.GitHubClientId) == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 GitHub OAuth，请先填入 GitHub Client Id 以及 GitHub Client Secret！",
			})
			return false
		}
	case "discord.enabled":
		if value == "true" && optionValue(values, "discord.client_id", system_setting.GetDiscordSettings().ClientId) == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Discord OAuth，请先填入 Discord Client Id 以及 Discord Client Secret！",
			})
			return false
		}
	case "oidc.enabled":
		if value == "true" && optionValue(values, "oidc.client_id", system_setting.GetOIDCSettings().ClientId) == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 OIDC 登录，请先填入 OIDC Client Id 以及 OIDC Client Secret！",
			})
			return false
		}
	case "LinuxDOOAuthEnabled":
		if value == "true" && optionValue(values, "LinuxDOClientId", common.LinuxDOClientId) == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 LinuxDO OAuth，请先填入 LinuxDO Client Id 以及 LinuxDO Client Secret！",
			})
			return false
		}
	case "EmailDomainRestrictionEnabled":
		if value == "true" && optionValue(values, "EmailDomainWhitelist", strings.Join(common.EmailDomainWhitelist, ",")) == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用邮箱域名限制，请先填入限制的邮箱域名！",
			})
			return false
		}
	case "WeChatAuthEnabled":
		if value == "true" && optionValue(values, "WeChatServerAddress", common.WeChatServerAddress) == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用微信登录，请先填入微信登录相关配置信息！",
			})
			return false
		}
	case "TurnstileCheckEnabled":
		if value == "true" && optionValue(values, "TurnstileSiteKey", common.TurnstileSiteKey) == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Turnstile 校验，请先填入 Turnstile 校验相关配置信息！",
			})
			return false
		}
	case "TelegramOAuthEnabled":
		if value == "true" && optionValue(values, "TelegramBotToken", common.TelegramBotToken) == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Telegram OAuth，请先填入 Telegram Bot Token！",
			})
			return false
		}
	case "theme.frontend":
		if value != "default" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Classic 前端已移除，主题只能设置为 default",
			})
			return false
		}
	case "GroupRatio":
		err = ratio_setting.CheckGroupRatio(value)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return false
		}
	case "gemini.safety_settings":
		err = model_setting.ValidateGeminiSafetySettings(value)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return false
		}
	case "claude.default_max_tokens":
		err = model_setting.ValidateClaudeDefaultMaxTokens(value)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return false
		}
	case operation_setting.ToolPriceOptionKey:
		err = operation_setting.ValidateToolPricesJSON(value)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return false
		}
	case "ImageRatio":
		err = validateRatioOption(value)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "图片倍率设置失败: " + err.Error(),
			})
			return false
		}
	case "AudioRatio":
		err = validateRatioOption(value)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "音频倍率设置失败: " + err.Error(),
			})
			return false
		}
	case "AudioCompletionRatio":
		err = validateRatioOption(value)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "音频补全倍率设置失败: " + err.Error(),
			})
			return false
		}
	case "CreateCacheRatio":
		err = validateRatioOption(value)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "缓存创建倍率设置失败: " + err.Error(),
			})
			return false
		}
	case "ModelRequestRateLimitGroup":
		err = setting.CheckModelRequestRateLimitGroup(value)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return false
		}
	case "AutomaticDisableStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(value)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return false
		}
	case "AutomaticRetryStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(value)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return false
		}
	case "console_setting.api_info":
		err = console_setting.ValidateConsoleSettings(value, "ApiInfo")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return false
		}
	case "console_setting.announcements":
		err = console_setting.ValidateConsoleSettings(value, "Announcements")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return false
		}
	case "console_setting.faq":
		err = console_setting.ValidateConsoleSettings(value, "FAQ")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return false
		}
	case "console_setting.uptime_kuma_groups":
		err = console_setting.ValidateConsoleSettings(value, "UptimeKumaGroups")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return false
		}
	}
	return true
}

// UpdateOption keeps the legacy single-option endpoint compatible while
// sharing validation and persistence behavior with batch option updates.
func UpdateOption(c *gin.Context) {
	var option OptionUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &option); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	value := normalizeOptionValue(option.Value)
	if !validateOptionUpdate(c, option.Key, value, map[string]string{option.Key: value}) {
		return
	}
	if err := model.UpdateOption(option.Key, value); err != nil {
		common.ApiError(c, err)
		return
	}
	// 出于安全考虑只记录被修改的配置项名称，不记录配置值（可能含密钥等敏感信息）。
	recordManageAudit(c, "option.update", map[string]interface{}{
		"key": option.Key,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// UpdateOptions validates a batch from c, persists it atomically, and writes the JSON result to c.
func UpdateOptions(c *gin.Context) {
	var request struct {
		Options []OptionUpdateRequest `json:"options"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if len(request.Options) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "设置项不能为空",
		})
		return
	}

	values := make(map[string]string, len(request.Options))
	keys := make([]string, 0, len(request.Options))
	for _, option := range request.Options {
		if strings.TrimSpace(option.Key) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "设置项名称不能为空",
			})
			return
		}
		if _, exists := values[option.Key]; exists {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "重复的设置项: " + option.Key,
			})
			return
		}
		values[option.Key] = normalizeOptionValue(option.Value)
		keys = append(keys, option.Key)
	}
	for _, option := range request.Options {
		if !validateOptionUpdate(c, option.Key, values[option.Key], values) {
			return
		}
	}

	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiError(c, err)
		return
	}
	sort.Strings(keys)
	// 出于安全考虑只记录被修改的配置项名称，不记录配置值（可能含密钥等敏感信息）。
	recordManageAudit(c, "option.update", map[string]interface{}{
		"key": strings.Join(keys, ", "),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
