package controller

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateChannelProxy(t *testing.T) {
	tests := []struct {
		name    string
		proxy   string
		wantErr bool
	}{
		{name: "empty"},
		{name: "http", proxy: "http://proxy.example:8080"},
		{name: "https", proxy: "https://proxy.example:8443"},
		{name: "socks5", proxy: "socks5://proxy.example"},
		{name: "socks5h", proxy: "socks5h://proxy.example:1080/"},
		{name: "unsupported", proxy: "ftp://proxy.example", wantErr: true},
		{name: "path", proxy: "socks5://proxy.example:1080/path", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setting, err := common.Marshal(dto.ChannelSettings{Proxy: test.proxy})
			require.NoError(t, err)
			channel := &model.Channel{
				Type:    constant.ChannelTypeOpenAI,
				Setting: common.GetPointer(string(setting)),
			}

			err = validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "invalid channel proxy")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateChannelRequiresNewAPIBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL *string
		wantErr bool
	}{
		{name: "missing", wantErr: true},
		{name: "blank", baseURL: common.GetPointer("  "), wantErr: true},
		{name: "configured", baseURL: common.GetPointer("https://new-api.example")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{
				Type:    constant.ChannelTypeNewAPI,
				BaseURL: test.baseURL,
			}

			err := validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "New API channel base URL cannot be empty")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewAPIChannelRegistration(t *testing.T) {
	apiType, ok := common.ChannelType2APIType(constant.ChannelTypeNewAPI)

	require.True(t, ok)
	assert.Equal(t, constant.APITypeNewAPI, apiType)
	assert.Equal(t, "New API", constant.GetChannelTypeName(constant.ChannelTypeNewAPI))
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeNewAPI)
	assert.Empty(t, constant.ChannelBaseURLs[constant.ChannelTypeNewAPI])
}

func TestResponsesCompactChannelSupport(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		apiType     int
		want        bool
	}{
		{name: "OpenAI", channelType: constant.ChannelTypeOpenAI, apiType: constant.APITypeOpenAI, want: true},
		{name: "Azure", channelType: constant.ChannelTypeAzure, apiType: constant.APITypeOpenAI, want: true},
		{name: "Codex", channelType: constant.ChannelTypeCodex, apiType: constant.APITypeCodex, want: true},
		{name: "Advanced Custom", channelType: constant.ChannelTypeAdvancedCustom, apiType: constant.APITypeAdvancedCustom, want: true},
		{name: "Sub2API", channelType: constant.ChannelTypeSub2API, apiType: constant.APITypeSub2API, want: true},
		{name: "New API", channelType: constant.ChannelTypeNewAPI, apiType: constant.APITypeNewAPI, want: true},
		{name: "Anthropic", channelType: constant.ChannelTypeAnthropic, apiType: constant.APITypeAnthropic, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, common.SupportsResponsesCompact(test.channelType, test.apiType))
		})
	}
}

func TestChannelTestStreamDefaultsAndEndpointSupport(t *testing.T) {
	assert.True(t, shouldUseStreamForAutomaticChannelTest(&model.Channel{Type: constant.ChannelTypeOpenAI}))

	defaultContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	defaultContext.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/1", nil)
	assert.True(t, parseChannelTestStream(defaultContext))

	synchronousContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	synchronousContext.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/1?stream=false", nil)
	assert.False(t, parseChannelTestStream(synchronousContext))

	imageRequest := buildTestRequest("gpt-image-1", string(constant.EndpointTypeImageGeneration), nil, true)
	assert.True(t, imageRequest.IsStream(defaultContext.Request))
	assert.Equal(t, string(constant.EndpointTypeImageGeneration), normalizeChannelTestEndpoint(nil, "gpt-image-1", ""))

	embeddingRequest := buildTestRequest("text-embedding-3-small", string(constant.EndpointTypeEmbeddings), nil, true)
	assert.False(t, embeddingRequest.IsStream(defaultContext.Request))
}

func TestMultiprotocolGatewayEndpointTypes(t *testing.T) {
	want := []constant.EndpointType{
		constant.EndpointTypeOpenAI,
		constant.EndpointTypeOpenAIResponse,
		constant.EndpointTypeOpenAIResponseCompact,
		constant.EndpointTypeAnthropic,
		constant.EndpointTypeGemini,
		constant.EndpointTypeOpenAIAlphaSearch,
	}

	assert.Equal(t, want, common.GetEndpointTypesByChannelType(constant.ChannelTypeNewAPI, "gpt-5"))
	assert.Equal(t, want, common.GetEndpointTypesByChannelType(constant.ChannelTypeSub2API, "gpt-5"))
}

func TestCopyChannelRejectsInvalidLegacyProxySettings(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	settingBytes, err := common.Marshal(dto.ChannelSettings{
		Proxy: "socks5://proxy.example/legacy-path",
	})
	require.NoError(t, err)
	setting := string(settingBytes)
	origin := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Name:    "legacy proxy channel",
		Key:     "test-key",
		Models:  "gpt-test",
		Group:   "default",
		Setting: &setting,
	}
	require.NoError(t, db.Create(origin).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", origin.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/copy", nil)

	CopyChannel(ctx)

	assert.Contains(t, recorder.Body.String(), "invalid channel settings")
	var channelCount int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&channelCount).Error)
	assert.Equal(t, int64(1), channelCount)
}

func TestDeleteChannelResetsProxyCacheWhenPreReadFails(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	service.ResetProxyClientCache()
	t.Cleanup(service.ResetProxyClientCache)

	proxyURL := "http://proxy.example:8080"
	beforeDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "999999"}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/999999", nil)

	DeleteChannel(ctx)

	assert.Contains(t, recorder.Body.String(), `"success":true`)
	afterDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)
	assert.NotSame(t, beforeDelete, afterDelete)
}

func TestDeleteChannelBatchReportsAndAuditsActualDeletedCount(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	channel := &model.Channel{Name: "existing", Key: "test-key"}
	require.NoError(t, db.Create(channel).Error)

	requestBody, err := common.Marshal(ChannelBatch{Ids: []int{channel.Id, 999999}})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/batch", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	DeleteChannelBatch(ctx)

	var response struct {
		Success bool  `json:"success"`
		Data    int64 `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, int64(1), response.Data)

	var auditLog model.Log
	require.NoError(t, db.Order("id desc").First(&auditLog).Error)
	var auditData struct {
		Operation struct {
			Params map[string]any `json:"params"`
		} `json:"op"`
	}
	require.NoError(t, common.UnmarshalJsonStr(auditLog.Other, &auditData))
	assert.Equal(t, float64(1), auditData.Operation.Params["count"])
}

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	requestRules := []billingexpr.RequestRuleTrace{{
		Cond:       `param("service_tier") == "fast"`,
		Multiplier: 2,
		Matched:    true,
	}}
	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier:  "base",
		RequestRules: requestRules,
	})

	fields := other.Snapshot()
	require.Equal(t, "tiered_expr", fields["billing_mode"])
	require.Equal(t, "base", fields["matched_tier"])
	require.Equal(t, requestRules, fields["request_rules"])
	require.NotEmpty(t, fields["expr_b64"])
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestManualModelBatchRecoversOnlyAutoDisabledMultiKeys(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	originalAutomaticEnable := common.AutomaticEnableChannelEnabled
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalSelfUseModeEnabled := operation_setting.SelfUseModeEnabled
	originalStreamingTimeout := constant.StreamingTimeout
	common.AutomaticEnableChannelEnabled = true
	common.LogConsumeEnabled = false
	common.MemoryCacheEnabled = false
	operation_setting.SelfUseModeEnabled = true
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		common.AutomaticEnableChannelEnabled = originalAutomaticEnable
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		operation_setting.SelfUseModeEnabled = originalSelfUseModeEnabled
		constant.StreamingTimeout = originalStreamingTimeout
	})

	var requestMu sync.Mutex
	requestedKeys := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Authorization")
		requestMu.Lock()
		requestedKeys[key]++
		requestMu.Unlock()
		if key != "Bearer key-a" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid key"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(server.Close)

	root := &model.User{
		Id:       1,
		Username: "root",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    1000000,
	}
	require.NoError(t, db.Create(root).Error)
	channel := &model.Channel{
		Name:    "multi-key recovery",
		Type:    constant.ChannelTypeOpenAI,
		Key:     "key-a\nkey-b\nkey-c",
		BaseURL: common.GetPointer(server.URL),
		Models:  "gpt-4o-mini",
		Group:   "default",
		Status:  common.ChannelStatusAutoDisabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
				2: common.ChannelStatusManuallyDisabled,
			},
		},
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-4o-mini",
		ChannelId: channel.Id,
		Enabled:   false,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
	ctx.Set("id", root.Id)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/1?model=gpt-4o-mini&manual_model_batch=true", nil)

	TestChannel(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success, response.Message)

	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.NotContains(t, stored.ChannelInfo.MultiKeyStatusList, 0)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[1])
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.ChannelInfo.MultiKeyStatusList[2])

	requestMu.Lock()
	defer requestMu.Unlock()
	assert.Equal(t, map[string]int{
		"Bearer key-a": 1,
		"Bearer key-b": 1,
	}, requestedKeys)
}

func TestAutomaticHealthCheckRecoversAutoDisabledMultiKeys(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	originalAutomaticEnable := common.AutomaticEnableChannelEnabled
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalSelfUseModeEnabled := operation_setting.SelfUseModeEnabled
	originalStreamingTimeout := constant.StreamingTimeout
	common.AutomaticEnableChannelEnabled = true
	common.AutomaticDisableChannelEnabled = false
	common.LogConsumeEnabled = false
	common.MemoryCacheEnabled = false
	operation_setting.SelfUseModeEnabled = true
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		common.AutomaticEnableChannelEnabled = originalAutomaticEnable
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		operation_setting.SelfUseModeEnabled = originalSelfUseModeEnabled
		constant.StreamingTimeout = originalStreamingTimeout
	})

	requestedKeys := make(map[string]int)
	var requestMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		key := request.Header.Get("Authorization")
		requestMu.Lock()
		requestedKeys[key]++
		requestMu.Unlock()
		if key != "Bearer key-a" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid key"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(server.Close)

	root := &model.User{Id: 1, Username: "root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1000000}
	require.NoError(t, db.Create(root).Error)
	channel := &model.Channel{
		Name: "automatic multi-key recovery", Type: constant.ChannelTypeOpenAI,
		Key: "key-a\nkey-b\nkey-c", BaseURL: common.GetPointer(server.URL), Models: "gpt-4o-mini", Group: "default",
		Status: common.ChannelStatusAutoDisabled,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyStatusList: map[int]int{
			0: common.ChannelStatusAutoDisabled,
			1: common.ChannelStatusAutoDisabled,
			2: common.ChannelStatusManuallyDisabled,
		}},
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o-mini", ChannelId: channel.Id, Enabled: false}).Error)

	summary := testChannelForHealthCheck(context.Background(), channel, root.Id, true, 10_000_000)
	assert.Equal(t, channelTestSummary{Tested: 1, Succeeded: 1, Enabled: 1}, summary)

	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.NotContains(t, stored.ChannelInfo.MultiKeyStatusList, 0)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[1])
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.ChannelInfo.MultiKeyStatusList[2])
	requestMu.Lock()
	defer requestMu.Unlock()
	assert.Equal(t, map[string]int{"Bearer key-a": 1, "Bearer key-b": 1}, requestedKeys)
}

func TestAutomaticHealthCheckPreservesFailedPrimaryWhileRecoveringAnotherKey(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	originalAutomaticEnable := common.AutomaticEnableChannelEnabled
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalSelfUseModeEnabled := operation_setting.SelfUseModeEnabled
	originalStreamingTimeout := constant.StreamingTimeout
	common.AutomaticEnableChannelEnabled = true
	common.AutomaticDisableChannelEnabled = true
	common.LogConsumeEnabled = false
	common.MemoryCacheEnabled = false
	operation_setting.SelfUseModeEnabled = true
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		common.AutomaticEnableChannelEnabled = originalAutomaticEnable
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		operation_setting.SelfUseModeEnabled = originalSelfUseModeEnabled
		constant.StreamingTimeout = originalStreamingTimeout
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") == "Bearer key-a" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid key"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(server.Close)

	root := &model.User{Id: 1, Username: "root-mixed-key", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1000000}
	require.NoError(t, db.Create(root).Error)
	autoBan := 1
	channel := &model.Channel{
		Name: "automatic mixed multi-key recovery", Type: constant.ChannelTypeOpenAI,
		Key: "key-a\nkey-b", BaseURL: common.GetPointer(server.URL), Models: "gpt-4o-mini", Group: "default",
		Status: common.ChannelStatusEnabled, AutoBan: &autoBan,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyStatusList: map[int]int{
			1: common.ChannelStatusAutoDisabled,
		}},
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o-mini", ChannelId: channel.Id, Enabled: true}).Error)

	summary := testChannelForHealthCheck(context.Background(), channel, root.Id, true, 10_000_000)
	assert.Equal(t, channelTestSummary{Tested: 1, Failed: 1, Disabled: 1, Enabled: 1}, summary)

	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.NotContains(t, stored.ChannelInfo.MultiKeyStatusList, 1)
}

func TestAutomaticHealthCheckCountsLocalFailureAsFailure(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	originalAutomaticEnable := common.AutomaticEnableChannelEnabled
	common.AutomaticEnableChannelEnabled = false
	t.Cleanup(func() { common.AutomaticEnableChannelEnabled = originalAutomaticEnable })

	channel := &model.Channel{
		Name: "unsupported", Type: constant.ChannelTypeMidjourney, Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(channel).Error)

	summary := testChannelForHealthCheck(context.Background(), channel, 1, true, 10_000_000)
	assert.Equal(t, channelTestSummary{Tested: 1, Failed: 1}, summary)
}

func TestSelectChannelsForAutomaticTestPassiveRecoveryOnlyUsesAutoDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModePassiveRecovery)

	require.Len(t, selected, 1)
	require.Equal(t, 2, selected[0].Id)
}

func TestSelectChannelsForAutomaticTestScheduledSkipsManualDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}

func TestSelectChannelsForAutomaticTestAutoBanOnlyUsesEligibleChannels(t *testing.T) {
	autoBanEnabled := 1
	autoBanDisabled := 0
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled, AutoBan: &autoBanEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled, AutoBan: &autoBanDisabled},
		{Id: 3, Status: common.ChannelStatusAutoDisabled, AutoBan: &autoBanEnabled},
		{Id: 4, Status: common.ChannelStatusManuallyDisabled, AutoBan: &autoBanEnabled},
		{Id: 5, Status: common.ChannelStatusEnabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeAutoBanOnly)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 3, selected[1].Id)
}

func TestRunChannelTestWorkersHonorsConfiguredConcurrency(t *testing.T) {
	originalInterval := common.RequestInterval
	common.RequestInterval = 0
	t.Cleanup(func() { common.RequestInterval = originalInterval })

	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled},
		{Id: 3, Status: common.ChannelStatusEnabled},
		{Id: 4, Status: common.ChannelStatusEnabled},
	}
	started := make(chan struct{}, len(channels))
	release := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	progress := make([]int, 0, len(channels)+1)
	summaryResult := make(chan channelTestSummary, 1)

	go func() {
		summaryResult <- runChannelTestWorkers(
			context.Background(),
			channels,
			2,
			func(_ context.Context, _ *model.Channel) channelTestSummary {
				current := active.Add(1)
				defer active.Add(-1)
				for {
					observed := maxActive.Load()
					if current <= observed || maxActive.CompareAndSwap(observed, current) {
						break
					}
				}
				started <- struct{}{}
				<-release
				return channelTestSummary{Tested: 1, Succeeded: 1}
			},
			func(processed, _ int) {
				progress = append(progress, processed)
			},
		)
	}()

	<-started
	<-started
	select {
	case <-started:
		t.Fatal("started more channel tests than the configured concurrency")
	default:
	}
	close(release)

	summary := <-summaryResult

	assert.Equal(t, int32(2), maxActive.Load())
	assert.Equal(t, channelTestSummary{Tested: 4, Succeeded: 4}, summary)
	assert.Equal(t, []int{0, 1, 2, 3, 4}, progress)
}

func TestRunChannelTestWorkersStopsAfterCancellation(t *testing.T) {
	originalInterval := common.RequestInterval
	common.RequestInterval = 0
	t.Cleanup(func() { common.RequestInterval = originalInterval })

	ctx, cancel := context.WithCancel(context.Background())
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled},
		{Id: 3, Status: common.ChannelStatusEnabled},
		{Id: 4, Status: common.ChannelStatusEnabled},
	}
	started := make(chan struct{}, len(channels))
	progress := make([]int, 0, 1)
	summaryResult := make(chan channelTestSummary, 1)

	go func() {
		summaryResult <- runChannelTestWorkers(
			ctx,
			channels,
			2,
			func(ctx context.Context, _ *model.Channel) channelTestSummary {
				started <- struct{}{}
				<-ctx.Done()
				return channelTestSummary{Tested: 1, Succeeded: 1}
			},
			func(processed, _ int) {
				progress = append(progress, processed)
			},
		)
	}()

	<-started
	<-started
	cancel()

	summary := <-summaryResult

	select {
	case <-started:
		t.Fatal("started another channel test after cancellation")
	default:
	}
	assert.Equal(t, channelTestSummary{Tested: 2, Succeeded: 2}, summary)
	assert.Equal(t, []int{0}, progress)
}

func TestTestAllChannelsRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

	TestAllChannels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有通道测试任务正在运行或等待中")
}
