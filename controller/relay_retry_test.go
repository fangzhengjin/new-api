package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrencyCleanupDoesNotReplaceSuccessfulUpstreamResult(t *testing.T) {
	cleanupErr := service.ErrChannelConcurrencyUnavailable
	assert.False(t, shouldSurfaceConcurrencyCleanupError(true, cleanupErr))
	assert.True(t, shouldSurfaceConcurrencyCleanupError(false, cleanupErr))
	assert.False(t, shouldSurfaceConcurrencyCleanupError(false, nil))
}

func TestRelayAttemptDecisionUsesOneBudgetAcrossMultiKeyTargets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	service.GetChannelConstraints(c).AddPin(taskdto.ChannelPin{
		ChannelId: 1,
		Source:    taskdto.PinSourceOriginTask,
		Rank:      taskdto.PinRankOriginTask,
		RetryMode: taskdto.PinRetrySameChannel,
	})
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
	err := types.NewOpenAIError(errors.New("temporary upstream failure"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)

	canRetry, exhausted := relayAttemptDecision(c, err, 0, 3)
	assert.True(t, canRetry)
	assert.False(t, exhausted)
	assert.True(t, shouldRotateMultiKeyTarget(c, canRetry))
	assert.True(t, shouldRecordRelayFailure(c, err))

	canRetry, exhausted = relayAttemptDecision(c, err, 3, 3)
	assert.False(t, canRetry)
	assert.True(t, exhausted)
	assert.False(t, shouldRotateMultiKeyTarget(c, canRetry))
}

func TestRelayRetryKeepsSingleKeyTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.False(t, shouldRotateMultiKeyTarget(c, true))
}

func TestRecordRelayTargetCapturesOrderedKeysAndMaskedErrors(t *testing.T) {
	originalDebugEnabled := common.DebugEnabled
	common.DebugEnabled = true
	t.Cleanup(func() { common.DebugEnabled = originalDebugEnabled })
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelId:            2,
		ChannelIsMultiKey:    true,
		ChannelMultiKeyIndex: 3,
	}}

	recorded := recordRelayTarget(c, info)
	require.True(t, recorded)
	errorMessage := "api_key:secret " + strings.Repeat("x", common.LocalLogContentLimit+100)
	recordRelayTargetError(info, types.NewOpenAIError(errors.New(errorMessage), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests), recorded)
	info.ChannelMultiKeyIndex = 7
	require.True(t, recordRelayTarget(c, info))

	require.Len(t, info.RetryTargets, 2)
	assert.Equal(t, []string{"2", "2"}, c.GetStringSlice("use_channel"))
	assert.Equal(t, 3, *info.RetryTargets[0].MultiKeyIndex)
	assert.Equal(t, http.StatusTooManyRequests, info.RetryTargets[0].StatusCode)
	assert.Contains(t, info.RetryTargets[0].Error, "api_key:***")
	assert.NotContains(t, info.RetryTargets[0].Error, "secret")
	assert.Contains(t, info.RetryTargets[0].Error, "[truncated")
	assert.Less(t, len(info.RetryTargets[0].Error), len(errorMessage))
	assert.Equal(t, 7, *info.RetryTargets[1].MultiKeyIndex)
	assert.Zero(t, info.RetryTargets[1].StatusCode)
}

func TestRecordRelayTargetMarksBoundedChainAsTruncated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 2}}

	for range maxRelayRetryTargets {
		require.True(t, recordRelayTarget(c, info))
	}
	require.False(t, recordRelayTarget(c, info))

	assert.Len(t, info.RetryTargets, maxRelayRetryTargets)
	assert.Len(t, c.GetStringSlice("use_channel"), maxRelayRetryTargets)
	assert.True(t, info.RetryTargetsTruncated)
}

func TestSetupNextMultiKeyTargetRotatesWithoutReuse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 0)
	channel := &model.Channel{
		Id:   12,
		Name: "multi-key",
		Key:  "key-a\nkey-b\nkey-c\nkey-d",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 4,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	retryParam := &service.RetryParam{Ctx: c}

	seen := map[int]struct{}{0: {}}
	for range 3 {
		require.Nil(t, setupNextMultiKeyTarget(c, channel, "gpt-test", retryParam))
		keyIndex := common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		assert.NotContains(t, seen, keyIndex)
		seen[keyIndex] = struct{}{}
	}
	assert.Len(t, seen, 4)
	assert.Len(t, retryParam.ExcludedKeyIndexes(channel.Id), 3)
}

func TestSetupNextMultiKeyTargetReportsExhaustedKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 0)
	channel := &model.Channel{
		Id:   12,
		Name: "multi-key",
		Key:  "key-a",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	retryParam := &service.RetryParam{Ctx: c}

	setupErr := setupNextMultiKeyTarget(c, channel, "gpt-test", retryParam)
	require.NotNil(t, setupErr)
	assert.Equal(t, types.ErrorCodeChannelNoAvailableKey, setupErr.GetErrorCode())
	assert.Contains(t, retryParam.ExcludedKeyIndexes(channel.Id), 0)
}

func TestRelayAttemptDecisionWithZeroRetriesIsImmediatelyExhausted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := types.NewOpenAIError(errors.New("temporary upstream failure"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)

	canRetry, exhausted := relayAttemptDecision(c, err, 0, 0)
	assert.False(t, canRetry)
	assert.True(t, exhausted)
}

func TestRelayAttemptDecisionDoesNotExhaustSkipRetryError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := types.NewOpenAIError(
		errors.New("do not retry"),
		types.ErrorCodeChannelParamOverrideInvalid,
		http.StatusInternalServerError,
		types.ErrOptionWithSkipRetry(),
	)

	canRetry, exhausted := relayAttemptDecision(c, err, 0, 2)
	assert.False(t, canRetry)
	assert.False(t, exhausted)
	assert.False(t, shouldAdvanceRelayTarget(c, err, true, canRetry, exhausted))
}

func TestRelayAttemptDecisionDoesNotSwitchForNonRetryableError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := types.NewOpenAIError(errors.New("invalid request"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)

	canRetry, exhausted := relayAttemptDecision(c, err, 0, 2)
	assert.False(t, canRetry)
	assert.False(t, exhausted)
}

func TestRelayTargetDecisionMatrix(t *testing.T) {
	originalDisableEnabled := common.AutomaticDisableChannelEnabled
	originalRetryRanges := operation_setting.AutomaticRetryStatusCodeRanges
	originalDisableRanges := operation_setting.AutomaticDisableStatusCodeRanges
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalDisableEnabled
		operation_setting.AutomaticRetryStatusCodeRanges = originalRetryRanges
		operation_setting.AutomaticDisableStatusCodeRanges = originalDisableRanges
	})
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 501}}
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 501, End: 502}}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	tests := []struct {
		name          string
		status        int
		wantExhausted bool
		wantAdvance   bool
		wantDisable   bool
	}{
		{name: "retry and disable", status: 501, wantExhausted: true, wantAdvance: true, wantDisable: true},
		{name: "retry only", status: 500, wantExhausted: true, wantAdvance: true, wantDisable: false},
		{name: "disable only", status: 502, wantAdvance: true, wantDisable: true},
		{name: "neither", status: 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := types.NewOpenAIError(errors.New(tt.name), types.ErrorCodeBadResponseStatusCode, tt.status)
			canRetry, exhausted := relayAttemptDecision(c, err, 1, 1)
			advance := shouldAdvanceRelayTarget(c, err, true, canRetry, exhausted)
			assert.False(t, canRetry)
			assert.Equal(t, tt.wantExhausted, exhausted)
			assert.Equal(t, tt.wantAdvance, advance)
			assert.Equal(t, tt.wantDisable, advance && service.ShouldDisableChannel(err))
		})
	}
}

func TestTaskRelayPinnedMultiKeyExhaustionDoesNotSelectAnotherChannel(t *testing.T) {
	originalDB := model.DB
	originalRetryTimes := common.RetryTimes
	originalRetryRanges := operation_setting.AutomaticRetryStatusCodeRanges
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRedisEnabled := common.RedisEnabled
	originalErrorLogEnabled := constant.ErrorLogEnabled
	t.Cleanup(func() {
		common.RetryTimes = originalRetryTimes
		operation_setting.AutomaticRetryStatusCodeRanges = originalRetryRanges
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RedisEnabled = originalRedisEnabled
		constant.ErrorLogEnabled = originalErrorLogEnabled
		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&model.Channel{}) && originalDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
	})
	common.RetryTimes = 1
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 500}}
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	constant.ErrorLogEnabled = false
	events := []string{}
	database := setupTaskSubmissionDatabase(t, false, &events)
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}))
	autoBan := 0
	first := &model.Channel{
		Type: constant.ChannelTypeTaskPlugin, Name: "pinned", Key: "test-key-a", Status: common.ChannelStatusEnabled,
		Group: "default", Models: "plugin-model", AutoBan: &autoBan,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 1, MultiKeyMode: constant.MultiKeyModeRandom},
	}
	second := &model.Channel{
		Type: constant.ChannelTypeTaskPlugin, Name: "another channel", Key: "test-key-b", Status: common.ChannelStatusEnabled,
		Group: "default", Models: "plugin-model", AutoBan: &autoBan,
	}
	require.NoError(t, database.Create(first).Error)
	require.NoError(t, database.Create(second).Error)
	require.NoError(t, database.Create(&model.Ability{Group: "default", Model: "plugin-model", ChannelId: second.Id, Enabled: true}).Error)
	model.InitChannelCache()
	selection := &service.RetryParam{Ctx: taskSubmissionTestContext(), TokenGroup: "default", ModelName: "plugin-model"}
	selection.ExcludeChannel(first.Id)
	available, _, err := service.CacheGetRandomSatisfiedChannel(selection)
	require.NoError(t, err)
	require.NotNil(t, available)
	require.Equal(t, second.Id, available.Id, "the fallback must be selectable after the first channel is exhausted")

	for _, pinned := range []bool{false, true} {
		name := "unpinned selects fallback"
		wantAttempts := []int{first.Id, second.Id}
		if pinned {
			name = "pinned stops after exhausted key"
			wantAttempts = []int{first.Id}
		}
		t.Run(name, func(t *testing.T) {
			c := taskSubmissionTestContext()
			if pinned {
				service.GetChannelConstraints(c).AddPin(taskdto.ChannelPin{
					ChannelId: first.Id, Source: taskdto.PinSourceOriginTask, Rank: taskdto.PinRankOriginTask, RetryMode: taskdto.PinRetrySameChannel,
				})
			}
			require.Nil(t, middleware.SetupContextForSelectedChannel(c, first, "plugin-model"))
			info := taskSubmissionRelayInfo(nil)
			info.TokenGroup = "default"
			info.LockedChannel = nil
			info.ChannelMeta = nil
			attempted := []int{}
			outcome, taskErr := executeTaskSubmissionWith(c, info, func(_ *gin.Context, attemptInfo *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *taskdto.TaskError) {
				attempted = append(attempted, attemptInfo.ChannelId)
				return nil, &taskdto.TaskError{Error: errors.New("upstream failure"), StatusCode: 500, LocalError: attemptInfo.ChannelId == second.Id}
			})
			require.NotNil(t, taskErr)
			assert.Nil(t, outcome)
			assert.Equal(t, wantAttempts, attempted)
		})
	}
}

func TestRelayDisableOnlyRequiresChannelAutoBan(t *testing.T) {
	originalDisableEnabled := common.AutomaticDisableChannelEnabled
	originalDisableRanges := operation_setting.AutomaticDisableStatusCodeRanges
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalDisableEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = originalDisableRanges
	})
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 502, End: 502}}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := types.NewOpenAIError(errors.New("disable only"), types.ErrorCodeBadResponseStatusCode, 502)
	assert.False(t, shouldAdvanceRelayTarget(c, err, false, false, false))
	common.AutomaticDisableChannelEnabled = false
	assert.False(t, shouldAdvanceRelayTarget(c, err, true, false, false))
}

func TestRelayDisableOnlyRespectsAffinityHardStop(t *testing.T) {
	originalDisableEnabled := common.AutomaticDisableChannelEnabled
	originalDisableRanges := operation_setting.AutomaticDisableStatusCodeRanges
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalDisableEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = originalDisableRanges
	})
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 502, End: 502}}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("channel_affinity_skip_retry_on_failure", true)
	err := types.NewOpenAIError(errors.New("disable only"), types.ErrorCodeBadResponseStatusCode, 502)
	assert.False(t, shouldAdvanceRelayTarget(c, err, true, false, false))
}

func TestRelayCancellationIsHardStop(t *testing.T) {
	originalDisableEnabled := common.AutomaticDisableChannelEnabled
	originalDisableRanges := operation_setting.AutomaticDisableStatusCodeRanges
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalDisableEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = originalDisableRanges
	})
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 500}}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	requestCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestCtx)
	cancel()
	err := types.NewOpenAIError(context.Canceled, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)

	canRetry, exhausted := relayAttemptDecision(c, err, 0, 2)
	assert.False(t, canRetry)
	assert.False(t, exhausted)
	assert.False(t, shouldAdvanceRelayTarget(c, err, true, canRetry, exhausted))
	assert.False(t, shouldRetryTaskRelay(c, &taskdto.TaskError{Error: context.Canceled, StatusCode: http.StatusInternalServerError}))
	assert.False(t, shouldRecordRelayFailure(c, err))
	respondTaskError(c, &taskdto.TaskError{Error: context.Canceled, StatusCode: http.StatusInternalServerError})
	assert.False(t, c.Writer.Written())
}

func TestRelayUpstreamCancellationIsHardStop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	err := types.NewError(context.Canceled, types.ErrorCodeDoRequestFailed)

	canRetry, exhausted := relayAttemptDecision(c, err, 0, 2)
	assert.False(t, canRetry)
	assert.False(t, exhausted)
	assert.False(t, shouldAdvanceRelayTarget(c, err, true, canRetry, exhausted))
	assert.False(t, shouldRetryTaskRelay(c, &taskdto.TaskError{Error: context.Canceled, StatusCode: http.StatusInternalServerError}))
	assert.False(t, shouldRecordRelayFailure(c, err))
}

func TestRelayRetryDelayUsesBoundedExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		min     time.Duration
		max     time.Duration
	}{
		{attempt: 0, min: 200 * time.Millisecond, max: 250 * time.Millisecond},
		{attempt: 1, min: 400 * time.Millisecond, max: 500 * time.Millisecond},
		{attempt: 2, min: 800 * time.Millisecond, max: time.Second},
		{attempt: 3, min: 1600 * time.Millisecond, max: 2 * time.Second},
		{attempt: 4, min: 1600 * time.Millisecond, max: 2 * time.Second},
	}

	for _, tt := range tests {
		delay := relayRetryDelay(tt.attempt)
		assert.GreaterOrEqual(t, delay, tt.min)
		assert.LessOrEqual(t, delay, tt.max)
	}
}

func TestWaitRelayRetryBackoffStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.False(t, waitRelayRetryBackoff(ctx, 0))
}

func TestTaskRelayAttemptDecisionMatchesRelayExhaustion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := &taskdto.TaskError{Error: errors.New("temporary upstream failure"), StatusCode: http.StatusInternalServerError}

	canRetry, exhausted := taskRelayAttemptDecision(c, err, 1, 1)
	assert.False(t, canRetry)
	assert.True(t, exhausted)
}

func TestTaskRelayAttemptDecisionDoesNotRetryLocalServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := &taskdto.TaskError{
		Error:      errors.New("local failure"),
		StatusCode: http.StatusInternalServerError,
		LocalError: true,
	}

	canRetry, exhausted := taskRelayAttemptDecision(c, err, 0, 2)
	assert.False(t, canRetry)
	assert.False(t, exhausted)
}

func TestTaskRelayRetryClassificationMatchesConfiguredRelayStatuses(t *testing.T) {
	originalRetryRanges := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = originalRetryRanges })
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: 200, End: 200},
		{Start: 429, End: 429},
		{Start: 500, End: 501},
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, status := range []int{0, 200, 400, 429, 500, 501, 502, 600} {
		relayErr := types.NewOpenAIError(errors.New("relay"), types.ErrorCodeBadResponseStatusCode, status)
		taskErr := &taskdto.TaskError{Error: errors.New("task"), StatusCode: status}
		assert.Equal(t, shouldRetry(c, relayErr), shouldRetryTaskRelay(c, taskErr), "status %d", status)
	}
}
