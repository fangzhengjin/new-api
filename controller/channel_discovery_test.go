package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseChannelDiscoveryBlocksKeepsKeysWithTheirOrigin(t *testing.T) {
	blocks, err := parseChannelDiscoveryBlocks("https://first.example/v1\nfirst-key\n\nKey: second-key https://second.example/api/v1/models")
	require.NoError(t, err)
	require.Len(t, blocks, 2)
	assert.Equal(t, "https://first.example", blocks[0].BaseURL)
	assert.Equal(t, []string{"first-key"}, blocks[0].Keys)
	assert.Equal(t, "https://second.example/api", blocks[1].BaseURL)
	assert.Equal(t, []string{"second-key"}, blocks[1].Keys)
}

func TestParseChannelDiscoveryBlocksUsesBlankLineBetweenReverseOrderBlocks(t *testing.T) {
	blocks, err := parseChannelDiscoveryBlocks("https://first.example\n\nsk-first123\n\nsk-second123\nhttps://second.example")
	require.NoError(t, err)
	require.Len(t, blocks, 2)
	assert.Equal(t, []string{"sk-first123"}, blocks[0].Keys)
	assert.Equal(t, []string{"sk-second123"}, blocks[1].Keys)
}

func TestParseChannelDiscoveryBlocksRejectsMultipleURLsOnOneLine(t *testing.T) {
	_, err := parseChannelDiscoveryBlocks("https://first.example https://second.example\nsk-example123")
	require.ErrorContains(t, err, "each upstream URL on its own line")
}

func TestParseChannelDiscoveryBlocksAcceptsURLAndKeyInEitherOrder(t *testing.T) {
	tests := []struct {
		name string
		text string
		key  string
	}{
		{name: "URL then key on one line", text: "https://cn.picpi.top sk-example123"},
		{name: "key then URL on one line", text: "sk-example123 https://cn.picpi.top"},
		{name: "URL then key on separate lines", text: "https://cn.picpi.top\nsk-example123"},
		{name: "key then URL on separate lines", text: "sk-example123\nhttps://cn.picpi.top"},
		{name: "uppercase explicit URL", text: "HTTPS://CN.PICPI.TOP sk-example123"},
		{name: "bare domain and endpoint path", text: "cn.picpi.top/v1/models sk-example123"},
		{name: "authorization bearer label", text: "Authorization: Bearer secret-value\nhttps://cn.picpi.top", key: "secret-value"},
		{name: "x api key equals label", text: "https://cn.picpi.top x-api-key = 'secret-value'", key: "secret-value"},
		{name: "API key is label", text: "API Key is secret-value\nhttps://cn.picpi.top", key: "secret-value"},
		{name: "Chinese key label", text: "地址：cn.picpi.top\n密钥：secret-value", key: "secret-value"},
		{name: "prefixed key in prose", text: "Use sk-example123 with https://cn.picpi.top", key: "sk-example123"},
		{name: "standalone quoted key", text: "https://cn.picpi.top\n- (`plainsecret123`)", key: "plainsecret123"},
		{name: "standalone padded key", text: "mykeysecret==\nhttps://cn.picpi.top", key: "mykeysecret=="},
		{name: "unrelated prose is ignored", text: "please\nhttps://cn.picpi.top\nsk-example123"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			blocks, err := parseChannelDiscoveryBlocks(test.text)
			require.NoError(t, err)
			require.Len(t, blocks, 1)
			expectedKey := test.key
			if expectedKey == "" {
				expectedKey = "sk-example123"
			}
			assert.Equal(t, "https://cn.picpi.top", blocks[0].BaseURL)
			assert.Equal(t, []string{expectedKey}, blocks[0].Keys)
		})
	}
}

func TestChannelDiscoveryProbeProtocolsUsesOnlyKnownNativeFamilies(t *testing.T) {
	tests := []struct {
		name      string
		modelName string
		expected  []string
	}{
		{name: "image", modelName: "gpt-image-2", expected: nil},
		{name: "anthropic", modelName: "claude-sonnet-4-6", expected: []string{"messages"}},
		{name: "openai", modelName: "gpt-5.4", expected: []string{"responses"}},
		{name: "codex", modelName: "codex-mini-latest", expected: []string{"responses"}},
		{name: "grok", modelName: "grok-4", expected: []string{"responses"}},
		{name: "dual or unknown", modelName: "qwen-max", expected: []string{"responses", "messages"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, channelDiscoveryProbeProtocols(test.modelName))
		})
	}
}

func TestProbeChannelDiscoveryIncludesDiscoveredModelRoute(t *testing.T) {
	body, err := common.Marshal(map[string]any{
		"text":        "https://api.example.test\nsecret-key",
		"block_index": 0,
		"key_index":   0,
		"base_url":    "https://api.example.test",
		"model":       "gpt-image-2",
		"models_path": "/provider/models",
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/discovery/probe", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ProbeChannelDiscovery(ctx)

	var response struct {
		Success bool              `json:"success"`
		Data    map[string]string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.True(t, response.Success)
	assert.Equal(t, "/provider/models", response.Data["models"])
}

func TestChannelDiscoveryProtocolEndpointsPreferDiscoveredModelsPrefix(t *testing.T) {
	block, err := newChannelDiscoveryBlock("https://api.example.test")
	require.NoError(t, err)

	endpoints := channelDiscoveryProtocolEndpoints(block, "/vendor/v1/models", "responses")

	require.NotEmpty(t, endpoints)
	assert.Equal(t, "https://api.example.test/vendor/v1/responses", endpoints[0])
}

func TestChannelDiscoveryRouteTargetAvoidsBasePathDuplication(t *testing.T) {
	tests := []struct {
		name         string
		baseURL      string
		endpointPath string
		expected     string
	}{
		{name: "origin base", baseURL: "https://api.example.test", endpointPath: "/v1/models", expected: "/v1/models"},
		{name: "endpoint under base path", baseURL: "https://api.example.test/provider", endpointPath: "/provider/v1/models", expected: "/v1/models"},
		{name: "endpoint outside base path", baseURL: "https://api.example.test/provider", endpointPath: "/v1/models", expected: "https://api.example.test/v1/models"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block, err := newChannelDiscoveryBlock(test.baseURL)
			require.NoError(t, err)
			assert.Equal(t, test.expected, channelDiscoveryRouteTarget(block, test.endpointPath))
		})
	}
}

func TestChannelDiscoveryProbeResponseRequiresProtocolPayload(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		body     string
		valid    bool
	}{
		{name: "responses", protocol: "responses", body: `{"output":[]}`, valid: true},
		{name: "chat", protocol: "chat", body: `{"choices":[]}`, valid: true},
		{name: "messages", protocol: "messages", body: `{"content":[]}`, valid: true},
		{name: "generic JSON", protocol: "responses", body: `{"ok":true}`},
		{name: "upstream error", protocol: "chat", body: `{"error":{"message":"bad route"}}`},
		{name: "HTML", protocol: "messages", body: `<html>ok</html>`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.valid, isChannelDiscoveryProbeResponse(test.protocol, []byte(test.body)))
		})
	}
}

func TestChannelDiscoveryValidationBlockUsesThePersistedAddress(t *testing.T) {
	source, err := newChannelDiscoveryBlock("https://api.example.test/original")
	require.NoError(t, err)

	created, err := channelDiscoveryValidationBlock(source, channelDiscoveryDraft{
		Operation:         "create",
		BaseURL:           "https://api.example.test/edited",
		SyncConfiguration: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.test/edited", created.BaseURL)

	keyOnlyUpdate, err := channelDiscoveryValidationBlock(source, channelDiscoveryDraft{
		Operation:         "update",
		BaseURL:           "https://api.example.test/ignored",
		SyncConfiguration: false,
	})
	require.NoError(t, err)
	assert.Equal(t, source.BaseURL, keyOnlyUpdate.BaseURL)
}

func TestChannelDiscoveryKeyOnlyUpdateStillValidatesSelectedKeys(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	block, err := newChannelDiscoveryBlock("https://api.example.test")
	require.NoError(t, err)
	block.Keys = []string{"selected-key"}

	_, err = validateChannelDiscoveryDraft(ctx, block, channelDiscoveryDraft{
		Operation:          "update",
		BaseURL:            block.BaseURL,
		AcceptedKeyIndexes: []int{0},
		SyncConfiguration:  false,
	})
	require.ErrorContains(t, err, context.Canceled.Error())
}

func TestChannelDiscoveryPreviewHashExcludesRuntimeStateAndBindsWriteBehavior(t *testing.T) {
	baseURL := "https://api.example.test"
	channel := &model.Channel{
		Id:        12,
		Type:      constant.ChannelTypeOpenAI,
		Key:       "secret",
		Status:    common.ChannelStatusEnabled,
		Name:      "example",
		BaseURL:   &baseURL,
		Models:    "gpt-5.4",
		Group:     "default",
		Balance:   1,
		UsedQuota: 10,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         2,
			MultiKeyPollingIndex: 1,
			MultiKeyStatusList:   map[int]int{1: common.ChannelStatusManuallyDisabled},
		},
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		UpstreamModelUpdateLastCheckTime:      10,
		UpstreamModelUpdateLastDetectedModels: []string{"gpt-5.5"},
	})
	plan := channelDiscoveryPlan{Channel: channel, Operation: "update", SnapshotHash: "snapshot", KeyCount: 1}
	first, err := channelDiscoveryPreviewFromPlan(plan)
	require.NoError(t, err)
	channel.Balance = 999
	channel.UsedQuota = 999
	channel.ChannelInfo.MultiKeyPollingIndex = 0
	channel.ChannelInfo.MultiKeyStatusList = map[int]int{0: common.ChannelStatusManuallyDisabled}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		UpstreamModelUpdateLastCheckTime:      20,
		UpstreamModelUpdateLastDetectedModels: []string{"gpt-5.6"},
	})
	second, err := channelDiscoveryPreviewFromPlan(plan)
	require.NoError(t, err)
	assert.Equal(t, first.PreviewHash, second.PreviewHash)

	plan.ReplaceKeys = true
	replace, err := channelDiscoveryPreviewFromPlan(plan)
	require.NoError(t, err)
	assert.NotEqual(t, second.PreviewHash, replace.PreviewHash)
	plan.ReplaceKeys = false
	testModel := "gpt-5.6-luna"
	channel.TestModel = &testModel
	testModelPreview, err := channelDiscoveryPreviewFromPlan(plan)
	require.NoError(t, err)
	assert.NotEqual(t, second.PreviewHash, testModelPreview.PreviewHash)
}

func TestBuildChannelDiscoveryConfigurationDerivesManagedModels(t *testing.T) {
	draft := channelDiscoveryDraft{
		SelectedModels: []string{"gpt-5.4", "gpt-5.5", "claude-opus-4-7"},
		Mapping:        map[string]string{"claude-opus-4-7": "claude-opus-4.7"},
		Routes: channelDiscoveryRouteSelection{
			"responses": "/v1/responses",
			"messages":  "/v1/messages",
			"compact":   "/v1/responses/compact",
		},
	}
	models, mapping, err := buildChannelDiscoveryConfiguration(draft, []string{"gpt-5.4", "gpt-5.5", "claude-opus-4.7"})
	require.NoError(t, err)
	assert.NotContains(t, models, "gpt-5.4-mini")
	assert.NotContains(t, models, "gpt-5.5-openai-compact")
	assert.NotContains(t, models, "codex-auto-review")
	assert.NotContains(t, mapping, "gpt-5.4-mini")
	assert.NotContains(t, mapping, "codex-auto-review")
	assert.Equal(t, "claude-opus-4.7", mapping["claude-opus-4-7"])
}

func TestBuildChannelDiscoveryConfigurationKeepsUpstreamCodexAutoReview(t *testing.T) {
	draft := channelDiscoveryDraft{
		SelectedModels: []string{"codex-auto-review"},
		Routes: channelDiscoveryRouteSelection{
			"responses": channelDiscoveryRouteDefaults["responses"],
		},
	}
	models, mapping, err := buildChannelDiscoveryConfiguration(draft, []string{"codex-auto-review"})
	require.NoError(t, err)
	assert.Contains(t, models, "codex-auto-review")
	assert.NotContains(t, mapping, "codex-auto-review")
}

func TestBuildChannelDiscoveryConfigurationRequiresAnInferenceRoute(t *testing.T) {
	_, _, err := buildChannelDiscoveryConfiguration(channelDiscoveryDraft{
		SelectedModels: []string{"gpt-5.4"},
		Routes: channelDiscoveryRouteSelection{
			"models": channelDiscoveryRouteDefaults["models"],
		},
	}, []string{"gpt-5.4"})

	require.ErrorContains(t, err, "at least one inference route")
}

func TestBuildChannelDiscoveryConfigurationAllowsManualImageRoutes(t *testing.T) {
	models, _, err := buildChannelDiscoveryConfiguration(channelDiscoveryDraft{
		SelectedModels: []string{"vendor-vision-model"},
		Routes: channelDiscoveryRouteSelection{
			"image_generations": channelDiscoveryRouteDefaults["image_generations"],
			"image_edits":       channelDiscoveryRouteDefaults["image_edits"],
		},
	}, []string{"vendor-vision-model"})

	require.NoError(t, err)
	assert.Equal(t, []string{"vendor-vision-model"}, models)
}

func TestBuildChannelDiscoveryPlanUsesOnlyUpstreamLunaAsTestModel(t *testing.T) {
	tests := []struct {
		name             string
		selectedModels   []string
		mapping          map[string]string
		discoveredModels []string
		expected         string
	}{
		{
			name:             "prefers selected upstream Luna",
			selectedModels:   []string{"gpt-5.4", "gpt-5.6-luna"},
			discoveredModels: []string{"gpt-5.4", "gpt-5.6-luna"},
			expected:         "gpt-5.6-luna",
		},
		{
			name:             "does not fall back to mini",
			selectedModels:   []string{"gpt-5.4"},
			discoveredModels: []string{"gpt-5.4"},
		},
		{
			name:             "does not use unselected upstream Luna",
			selectedModels:   []string{"gpt-5.4"},
			discoveredModels: []string{"gpt-5.4", "gpt-5.6-luna"},
		},
		{
			name:             "does not treat a mapped Luna alias as upstream Luna",
			selectedModels:   []string{"gpt-5.6-luna"},
			mapping:          map[string]string{"gpt-5.6-luna": "gpt-5.4"},
			discoveredModels: []string{"gpt-5.4"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block, err := newChannelDiscoveryBlock("https://api.example.test")
			require.NoError(t, err)
			block.Keys = []string{"secret"}
			plan, err := buildChannelDiscoveryPlan(block, channelDiscoveryDraft{
				Operation:          "create",
				BaseURL:            block.BaseURL,
				AcceptedKeyIndexes: []int{0},
				SelectedModels:     test.selectedModels,
				Mapping:            test.mapping,
				Routes: channelDiscoveryRouteSelection{
					"responses": channelDiscoveryRouteDefaults["responses"],
				},
				Name:              "example",
				Groups:            []string{"default"},
				Enabled:           true,
				SyncConfiguration: true,
			}, channelDiscoveryFetchResult{Models: test.discoveredModels})
			require.NoError(t, err)
			if test.expected == "" {
				assert.Nil(t, plan.Channel.TestModel)
				return
			}
			require.NotNil(t, plan.Channel.TestModel)
			assert.Equal(t, test.expected, *plan.Channel.TestModel)
		})
	}
}

func TestBuildChannelDiscoveryPlanAddsDiscoveredModelRouteAuth(t *testing.T) {
	block, err := newChannelDiscoveryBlock("https://api.example.test/provider")
	require.NoError(t, err)
	block.Keys = []string{"secret"}

	plan, err := buildChannelDiscoveryPlan(block, channelDiscoveryDraft{
		Operation:          "create",
		BaseURL:            block.BaseURL,
		AcceptedKeyIndexes: []int{0},
		SelectedModels:     []string{"vendor-model"},
		Routes: channelDiscoveryRouteSelection{
			"responses": "/responses",
			"models":    "https://api.example.test/v1/models",
		},
		Name:              "example",
		Groups:            []string{"default"},
		Enabled:           true,
		SyncConfiguration: true,
	}, channelDiscoveryFetchResult{
		Models:         []string{"vendor-model"},
		ModelsPath:     "/v1/models",
		ModelsAuthType: "anthropic",
	})
	require.NoError(t, err)

	settings := dto.ChannelOtherSettings{}
	require.NoError(t, common.UnmarshalJsonStr(plan.Channel.OtherSettings, &settings))
	require.NotNil(t, settings.AdvancedCustom)
	route, ok := settings.AdvancedCustom.ModelListRoute()
	require.True(t, ok)
	assert.Equal(t, "https://api.example.test/v1/models", route.UpstreamPath)
	require.NotNil(t, route.Auth)
	assert.Equal(t, "x-api-key", route.Auth.Name)
	assert.Equal(t, "{api_key}", route.Auth.Value)
}

func TestBuildChannelDiscoveryPlanDoesNotRestoreUnselectedModelRoute(t *testing.T) {
	block, err := newChannelDiscoveryBlock("https://api.example.test")
	require.NoError(t, err)
	block.Keys = []string{"secret"}

	plan, err := buildChannelDiscoveryPlan(block, channelDiscoveryDraft{
		Operation:          "create",
		BaseURL:            block.BaseURL,
		AcceptedKeyIndexes: []int{0},
		SelectedModels:     []string{"gpt-5.4"},
		Routes: channelDiscoveryRouteSelection{
			"responses": channelDiscoveryRouteDefaults["responses"],
		},
		Name:              "example",
		Groups:            []string{"default"},
		Enabled:           true,
		SyncConfiguration: true,
	}, channelDiscoveryFetchResult{
		Models:     []string{"gpt-5.4"},
		ModelsPath: "/v1/models",
	})
	require.NoError(t, err)

	for _, route := range plan.Routes {
		assert.NotEqual(t, "models", route.Protocol)
	}
}

func TestBuildChannelDiscoveryInfoPreservesExistingMultiKeyMode(t *testing.T) {
	current := &model.Channel{ChannelInfo: model.ChannelInfo{MultiKeyMode: constant.MultiKeyModePolling}}

	info := buildChannelDiscoveryInfo(current, "first-key\nsecond-key\nthird-key", false)

	assert.Equal(t, constant.MultiKeyModePolling, info.MultiKeyMode)
	assert.Equal(t, 3, info.MultiKeySize)
}

func TestInferChannelDiscoveryTypeUsesAdvancedCustomForCustomPath(t *testing.T) {
	channelType := inferChannelDiscoveryType([]string{"gpt-5.5"}, channelDiscoveryRouteSelection{
		"responses": "/vendor/responses",
	})

	assert.Equal(t, constant.ChannelTypeAdvancedCustom, channelType)
}

func TestApplyChannelDiscoveryRoutesRejectsCrossOriginTarget(t *testing.T) {
	baseURL := "https://api.example.test"
	channel := &model.Channel{Type: constant.ChannelTypeAdvancedCustom, BaseURL: &baseURL}

	_, err := applyChannelDiscoveryRoutes(channel, channelDiscoveryRouteSelection{
		"responses": "https://other.example.test/v1/responses",
	}, "")

	require.ErrorContains(t, err, "must keep the channel origin")
}
