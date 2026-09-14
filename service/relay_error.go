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

// ShouldRetryRelayError keeps the upstream signature: retryTimes carries the
// remaining attempts, so an exhausted budget stops retrying. The classification
// of the failure itself lives in RelayErrorRetryable, which is what controller's
// attempt model uses to tell "retryable" apart from "target exhausted".
func ShouldRetryRelayError(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	if retryTimes <= 0 {
		return false
	}
	return RelayErrorRetryable(c, openaiErr)
}

// RelayErrorRetryable reports whether a failure deserves another target at all.
// A canceled request never retries, and SkipRetry outranks the channel-error
// shortcut so client cancellation and explicitly non-retryable errors (for
// example a saturated channel concurrency quota) are not amplified across
// channels.
func RelayErrorRetryable(c *gin.Context, openaiErr *types.NewAPIError) bool {
	if openaiErr == nil {
		return false
	}
	if RelayRequestCanceled(c) || errors.Is(openaiErr, context.Canceled) {
		return false
	}
	if ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}
	if GetChannelConstraints(c).SuppressesRetry() {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	code := openaiErr.StatusCode
	if code >= 200 && code < 300 {
		return false
	}
	if code < 100 || code > 599 {
		return true
	}
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return false
	}
	return operation_setting.ShouldRetryByStatusCode(code)
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

// RecordRelayErrorLog writes the channel error log row for a relay failure.
func RecordRelayErrorLog(c *gin.Context, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo, channelError *types.ChannelError) {
	if !constant.ErrorLogEnabled || !types.IsRecordErrorLog(err) {
		return
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
	AppendTaskPluginContextAuditInfo(c, other)
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	useTimeSeconds := int(time.Since(startTime).Seconds())
	model.RecordErrorLog(c, userId, channelError.ChannelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
}
