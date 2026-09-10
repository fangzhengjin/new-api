package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// DecideRelayRetry is the single retry decision for relay attempts. The reason
// is recorded in the request policy decision events of the log details.
// 本地语义保留两条：客户端已取消的请求不再重试；SkipRetry 压过渠道错误捷径，
// 显式不可重试错误（如渠道并发配额饱和）不会在渠道间放大。
func DecideRelayRetry(c *gin.Context, err *types.NewAPIError, retryTimes int) PolicyDecision {
	if err == nil {
		return PolicyDecision{Action: "stop", Reason: "request_completed", Source: "system"}
	}
	if RelayRequestCanceled(c) || errors.Is(err, context.Canceled) {
		return PolicyDecision{Action: "stop", Reason: "client_canceled", Source: "system"}
	}
	if ShouldSkipRetryAfterChannelAffinityFailure(c) {
		source := RequestPolicy(c).SessionModeSource
		if source == "" {
			source = "session_rule"
		}
		return PolicyDecision{Action: "stop", Reason: "strict_session", Source: source}
	}
	if GetChannelConstraints(c).SuppressesRetry() {
		return PolicyDecision{Action: "stop", Reason: "pinned_channel", Source: "channel_constraint"}
	}
	if types.IsSkipRetryError(err) {
		return PolicyDecision{Action: "stop", Reason: "non_retryable_error", Source: "system"}
	}
	if types.IsChannelError(err) {
		return PolicyDecision{Action: "retry", Reason: "channel_error", Source: "system"}
	}
	if retryTimes <= 0 {
		return PolicyDecision{Action: "stop", Reason: "attempt_budget_exhausted", Source: "global"}
	}
	code := err.StatusCode
	if code >= 200 && code < 300 {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if code < 100 || code > 599 {
		return PolicyDecision{Action: "retry", Reason: "unrecognized_status", Source: "system"}
	}
	if operation_setting.IsAlwaysSkipRetryCode(err.GetErrorCode()) || operation_setting.IsAlwaysSkipRetryStatusCode(code) {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if operation_setting.ShouldRetryByStatusCode(code) {
		return PolicyDecision{Action: "retry", Reason: "retry_status_matched", Source: "global"}
	}
	return PolicyDecision{Action: "stop", Reason: "status_not_retryable", Source: "global"}
}

func ShouldRetryRelayError(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	return DecideRelayRetry(c, openaiErr, retryTimes).Action == "retry"
}

// RelayRequestCanceled reports whether the client already abandoned this request.
func RelayRequestCanceled(c *gin.Context) bool {
	return c != nil && c.Request != nil && c.Request.Context().Err() != nil
}

// ProcessChannelError records a channel failure, lets it disable the channel and
// writes the error log, which is what a single-attempt relay path needs.
func ProcessChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo) {
	processChannelError(c, channelError, err, relayInfo, true, true)
}

// ProcessChannelErrorForAttempt handles one attempt of a retry loop. Automatic
// disabling stays with the caller, which decides separately whether the attempt
// rotates or advances the target, and the error log is written once for the
// terminal error instead of once per attempt.
func ProcessChannelErrorForAttempt(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo, allowDisable bool) {
	processChannelError(c, channelError, err, relayInfo, allowDisable, false)
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo, allowDisable, recordErrorLog bool) {
	if err == nil {
		return
	}
	if RelayRequestCanceled(c) || errors.Is(err, context.Canceled) {
		return
	}
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.MaskSensitiveErrorWithStatusCode())))
	if allowDisable && ShouldDisableChannel(err) && channelError.AutoBan {
		reason := err.MaskSensitiveErrorWithStatusCode()
		gopool.Go(func() {
			DisableChannel(channelError, reason)
		})
	}

	if recordErrorLog {
		RecordRelayErrorLog(c, err, relayInfo, &channelError)
	}
}

// RecordRelayErrorLog writes the channel error log row for a relay failure. A nil
// channelError falls back to the channel recorded on the request context, so paths
// that only know the failure (for example channel tests) still attribute the log.
func RecordRelayErrorLog(c *gin.Context, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo, channelError *types.ChannelError) {
	if !constant.ErrorLogEnabled || !types.IsRecordErrorLog(err) {
		return
	}
	channelId := c.GetInt("channel_id")
	if channelError != nil {
		channelId = channelError.ChannelId
	}
	userId := c.GetInt("id")
	tokenName := c.GetString("token_name")
	modelName := c.GetString("original_model")
	tokenId := c.GetInt("token_id")
	userGroup := c.GetString("group")
	other := model.NewLogOther()
	if c.Request != nil && c.Request.URL != nil {
		other.SetPublic("request_path", c.Request.URL.Path)
	}
	other.SetPublic("error_type", err.GetErrorType())
	other.SetPublic("error_code", err.GetErrorCode())
	other.SetPublic("status_code", err.StatusCode)
	AppendRelayLogAdminInfo(c, relayInfo, other)
	AppendResponseModelLogInfo(relayInfo, other)
	AppendTaskPluginContextAuditInfo(c, other)
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	useTimeSeconds := int(time.Since(startTime).Seconds())
	model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
}
