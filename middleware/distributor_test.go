package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntryModelModifiers(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, tc := range []struct {
		name, path, body, want string
		invalid                bool
	}{
		{"chat", "/v1/chat/completions", `{"model":"qwen@effort:high@effort:MAX","reasoning_effort":"low","extra":9007199254740993,"temperature":0}`, `{"model":"qwen","reasoning_effort":"max","extra":9007199254740993,"temperature":0}`, false},
		{"responses", "/v1/responses", `{"model":"qwen@effort:none","reasoning":{"effort":"high","summary":"auto"}}`, `{"model":"qwen","reasoning":{"effort":"none","summary":"auto"}}`, false},
		{"messages", "/v1/messages", `{"model":"qwen@thinking:on@effort:max","thinking":{"budget_tokens":4096},"output_config":{"format":"test"}}`, `{"model":"qwen","thinking":{"type":"enabled","budget_tokens":4096},"output_config":{"format":"test","effort":"max"}}`, false},
		{"independent", "/v1/messages", `{"model":"qwen@effort:max@thinking:off"}`, `{"model":"qwen","thinking":{"type":"disabled"},"output_config":{"effort":"max"}}`, false},
		{"reverse", "/v1/messages", `{"model":"qwen@thinking:off@effort:max"}`, `{"model":"qwen","thinking":{"type":"disabled"},"output_config":{"effort":"max"}}`, false},
		{"adaptive", "/v1/messages", `{"model":"qwen@thinking:adaptive"}`, `{"model":"qwen","thinking":{"type":"adaptive"}}`, false},
		{"enabled", "/v1/messages", `{"model":"qwen@thinking:enabled"}`, `{"model":"qwen","thinking":{"type":"enabled"}}`, false},
		{"disabled", "/v1/messages", `{"model":"qwen@thinking:on@thinking:disabled"}`, `{"model":"qwen","thinking":{"type":"disabled"}}`, false},
		{"native", "/v1/messages", `{"model":"qwen","thinking":{"type":"adaptive"}}`, `{"model":"qwen","thinking":{"type":"adaptive"}}`, false},
		{"other endpoint", "/v1/images/generations", `{"model":"qwen@thinking:on"}`, `{"model":"qwen@thinking:on"}`, false},
		{"chat thinking", "/v1/chat/completions", `{"model":"qwen@thinking:off@effort:max"}`, "", true},
		{"responses thinking", "/v1/responses", `{"model":"qwen@thinking:adaptive"}`, "", true},
		{"bad effort", "/v1/messages", `{"model":"qwen@effort:typo"}`, "", true},
		{"empty effort", "/v1/responses", `{"model":"qwen@effort:"}`, "", true},
		{"bad thinking", "/v1/messages", `{"model":"qwen@thinking:4096"}`, "", true},
		{"empty thinking", "/v1/messages", `{"model":"qwen@thinking:"}`, "", true},
		{"bad parent", "/v1/messages", `{"model":"qwen@thinking:on","thinking":false}`, "", true},
		{"bad reasoning", "/v1/responses", `{"model":"qwen@effort:max","reasoning":[]}`, "", true},
		{"unknown", "/v1/messages", `{"model":"qwen@unknown:yes"}`, "", true},
		{"empty model", "/v1/messages", `{"model":"@effort:max"}`, "", true},
		{"empty model and effort", "/v1/messages", `{"model":"@effort:"}`, "", true},
		{"empty model chain", "/v1/messages", `{"model":"@thinking:on@effort:max"}`, "", true},
		{"opaque at name", "/v1/messages", `{"model":"@vendor/model"}`, `{"model":"@vendor/model"}`, false},
		{"opaque effort name", "/v1/messages", `{"model":"@effort"}`, `{"model":"@effort"}`, false},
		{"opaque with modifier", "/v1/chat/completions", `{"model":"@vendor/model@effort:max"}`, `{"model":"@vendor/model","reasoning_effort":"max"}`, false},
		{"sampling chat", "/v1/chat/completions", `{"model":"qwen@temperature:0.2@temperature:0@topp:0@effort:max","temperature":1,"top_p":1}`, `{"model":"qwen","temperature":0,"top_p":0,"reasoning_effort":"max"}`, false},
		{"sampling responses", "/v1/responses", `{"model":"qwen@temperature:0.2@topp:0.8@effort:low"}`, `{"model":"qwen","temperature":0.2,"top_p":0.8,"reasoning":{"effort":"low"}}`, false},
		{"sampling messages", "/v1/messages", `{"model":"qwen@temperature:0.2@topp:0.8@thinking:on@effort:max"}`, `{"model":"qwen","temperature":0.2,"top_p":0.8,"thinking":{"type":"enabled"},"output_config":{"effort":"max"}}`, false},
		{"invalid temperature", "/v1/messages", `{"model":"qwen@temperature:NaN"}`, "", true},
		{"invalid topp", "/v1/responses", `{"model":"qwen@topp:Inf"}`, "", true},
		{"empty temperature", "/v1/chat/completions", `{"model":"qwen@temperature:"}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			model, err := getModelFromRequest(c)
			if tc.invalid {
				require.Error(t, err)
				storage, storageErr := common.GetBodyStorage(c)
				require.NoError(t, storageErr)
				body, readErr := storage.Bytes()
				require.NoError(t, readErr)
				assert.Equal(t, tc.body, string(body))
				return
			}
			require.NoError(t, err)
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			body, err := storage.Bytes()
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(body))
			if strings.Contains(tc.body, "9007199254740993") {
				assert.Contains(t, string(body), "9007199254740993")
			}
			again, err := getModelFromRequest(c)
			require.NoError(t, err)
			assert.Equal(t, model, again)
			current, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			assert.Same(t, storage, current)
		})
	}
}

func TestInvalidEntryModifierStopsDistribution(t *testing.T) {
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/chat/completions", BodyStorageCleanup(), Distribute(), func(c *gin.Context) { t.Error("invalid modifier reached downstream") })
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen@thinking:on"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestEntryModifierPassthroughAndExemption(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	previous, blacklist := settings.PassThroughRequestEnabled, settings.ThinkingModelBlacklist
	t.Cleanup(func() { settings.PassThroughRequestEnabled, settings.ThinkingModelBlacklist = previous, blacklist })
	settings.PassThroughRequestEnabled = true
	settings.ThinkingModelBlacklist = []string{"opaque@effort:max"}
	for _, modelName := range []string{"opaque@effort:max", "qwen@effort:max", "qwen-max"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		body := fmt.Sprintf(`{"model":%q}`, modelName)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: true})
		t.Cleanup(func() { common.CleanupBodyStorage(c) })
		request, err := getModelFromRequest(c)
		require.NoError(t, err)
		storage, err := common.GetBodyStorage(c)
		require.NoError(t, err)
		payload, err := storage.Bytes()
		require.NoError(t, err)
		if modelName == "qwen@effort:max" {
			assert.Equal(t, "qwen", request.Model)
			assert.JSONEq(t, `{"model":"qwen","reasoning":{"effort":"max"}}`, string(payload))
			assert.Equal(t, int64(len(payload)), c.Request.ContentLength)
		} else {
			assert.Equal(t, modelName, request.Model)
			assert.Equal(t, body, string(payload))
		}
		c.Set(contextKeyTaskPluginEndpointModel, *request)
		cached, err := getModelFromRequest(c)
		require.NoError(t, err)
		assert.Equal(t, request, cached)
	}
}

func TestEntryEffortNativeFields(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"} {
		for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
			t.Run(path+"/"+effort, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(fmt.Sprintf(`{"model":"qwen@effort:%s"}`, effort)))
				c.Request.Header.Set("Content-Type", "application/json")
				t.Cleanup(func() { common.CleanupBodyStorage(c) })
				_, err := getModelFromRequest(c)
				require.NoError(t, err)
				switch path {
				case "/v1/chat/completions":
					var req dto.GeneralOpenAIRequest
					require.NoError(t, common.UnmarshalBodyReusable(c, &req))
					assert.Equal(t, effort, req.ReasoningEffort)
					assert.Nil(t, req.ReasoningConversion)
				case "/v1/responses":
					var req dto.OpenAIResponsesRequest
					require.NoError(t, common.UnmarshalBodyReusable(c, &req))
					require.NotNil(t, req.Reasoning)
					assert.Equal(t, effort, req.Reasoning.Effort)
					assert.Nil(t, req.ReasoningConversion)
				case "/v1/messages":
					var req dto.ClaudeRequest
					require.NoError(t, common.UnmarshalBodyReusable(c, &req))
					assert.JSONEq(t, fmt.Sprintf(`{"effort":%q}`, effort), string(req.OutputConfig))
					assert.Nil(t, req.Thinking)
				}
			})
		}
	}
}

func TestChannelMatchesExpectedTaskPluginUsesGenericChannelSetting(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeTaskPlugin}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "generic-alpha"})

	assert.True(t, channelMatchesExpectedTaskPlugin(nil, channel, "generic-alpha"))
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, channel, "generic-beta"))
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, channel, ""))
}

func TestChannelMatchesExpectedTaskPluginUsesPinnedLegacyIndex(t *testing.T) {
	registry := jsplugin.NewRegistry()
	alpha, err := registry.Register(distributorTaskPluginSource("legacy-alpha", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)
	pinnedGeneration := registry.Generation()

	require.NoError(t, registry.Unregister("legacy-alpha"))
	_, err = registry.Register(distributorTaskPluginSource("legacy-beta", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: pinnedGeneration,
		Plugin:     alpha,
	})
	channel := &model.Channel{Type: constant.ChannelTypeKling}

	assert.True(t, channelMatchesExpectedTaskPlugin(c, channel, "legacy-alpha"))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, channel, "legacy-beta"))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeJimeng}, "legacy-alpha"))
}

func TestChannelMatchesExpectedTaskPluginRejectsUnindexedLegacyChannel(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(distributorTaskPluginSource("legacy-alpha", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeJimeng}, "legacy-alpha"))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: 0}, "legacy-alpha"))
	assert.True(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeJimeng}, ""))
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, &model.Channel{Type: constant.ChannelTypeKling}, "legacy-alpha"))

	c.Set("expected_task_plugin_key", "legacy-alpha")
	setupErr := SetupContextForSelectedChannel(c, &model.Channel{Type: constant.ChannelTypeJimeng}, "task-model")
	require.NotNil(t, setupErr)
	assert.Contains(t, setupErr.Error(), "does not match")
}

func TestSharedEndpointRebindsToSelectedLegacyProvider(t *testing.T) {
	registry := jsplugin.NewRegistry()
	_, err := registry.Register(distributorEndpointPluginSource("gemini-shared", constant.ChannelTypeGemini), jsplugin.Options{})
	require.NoError(t, err)
	_, err = registry.Register(distributorEndpointPluginSource("vertex-shared", constant.ChannelTypeVertexAi), jsplugin.Options{})
	require.NoError(t, err)
	candidates := registry.Generation().LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: registry.Generation(), Plugin: candidates[0].Plugin})
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{
		Generation: registry.Generation(),
		Plugin:     candidates[0].Plugin,
		Protocol:   candidates[0].Protocol,
		Operation:  candidates[0].Operation,
		Model:      "task-model",
		Candidates: candidates,
	})
	c.Set("expected_task_plugin_key", candidates[0].Plugin.Meta.Key)

	geminiChannel := &model.Channel{Id: 1, Type: constant.ChannelTypeGemini}
	vertexChannel := &model.Channel{Id: 2, Type: constant.ChannelTypeVertexAi}
	assert.True(t, channelMatchesExpectedTaskPlugin(c, geminiChannel, candidates[0].Plugin.Meta.Key))
	assert.True(t, channelMatchesExpectedTaskPlugin(c, vertexChannel, candidates[0].Plugin.Meta.Key))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeKling}, candidates[0].Plugin.Meta.Key))

	require.Nil(t, SetupContextForSelectedChannel(c, vertexChannel, "task-model"))
	pinnedValue, exists := c.Get(jsplugin.ContextKeyPinnedEndpoint)
	require.True(t, exists)
	pinned, ok := pinnedValue.(jsplugin.PinnedEndpoint)
	require.True(t, ok)
	assert.Equal(t, "vertex-shared", pinned.Plugin.Meta.Key)
	assert.Equal(t, "vertex-shared", c.GetString("expected_task_plugin_key"))
	assert.Equal(t, "vertex-shared", c.GetString("task_plugin_key"))
	assert.True(t, channelMatchesExpectedTaskPlugin(c, geminiChannel, "vertex-shared"), "a retry may select another declared provider")
}

func distributorTaskPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d],
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelType)
}

func distributorEndpointPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d],
  models: ["task-model"],
  fetchMode: "per_task",
  protocols: [{name: "openai_responses", supports: ["stream", "sync", "background"]}],
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
export const protocols = {openai_responses: {
  decodeRequest: function(ctx) { return {kind: "submit", model: "task-model", requestBody: ctx.body.value}; },
  renderEvents: function() { return {events: [], state: null, done: false}; },
  renderFinal: function() { return {output: []}; },
}};
`, key, key, channelType)
}

func TestTokenModelLimitAllowsLegacyAliasAndModifierVariant(t *testing.T) {
	aliasOnly := map[string]bool{"claude-3-7-sonnet-thinking": true}
	assert.True(t, tokenModelLimitAllows(aliasOnly, "claude-3-7-sonnet-thinking"))
	assert.False(t, tokenModelLimitAllows(aliasOnly, "claude-3-7-sonnet"))

	baseOnly := map[string]bool{"claude-3-7-sonnet": true}
	assert.True(t, tokenModelLimitAllows(baseOnly, "claude-3-7-sonnet@thinking:on"))
	assert.True(t, tokenModelLimitAllows(baseOnly, "claude-3-7-sonnet-thinking"))

	wildcard := map[string]bool{"gemini-2.5-flash-thinking-*": true}
	assert.True(t, tokenModelLimitAllows(wildcard, "gemini-2.5-flash-thinking-8192"))
}

func TestTokenModelLimitAllowsExemptAtNameByFullName(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := append([]string(nil), settings.ThinkingModelBlacklist...)
	t.Cleanup(func() { settings.ThinkingModelBlacklist = original })
	settings.ThinkingModelBlacklist = append(original, "re:.*@sha256:.*")

	fullOnly := map[string]bool{"opaque@sha256:deadbeef": true}
	assert.True(t, tokenModelLimitAllows(fullOnly, "opaque@sha256:deadbeef"))

	baseOnly := map[string]bool{"opaque": true}
	assert.False(t, tokenModelLimitAllows(baseOnly, "opaque@sha256:deadbeef"))
}

func TestNoAvailableChannelMessageNamesClaimingTaskPlugin(t *testing.T) {
	require.NoError(t, i18n.Init())
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(distributorTaskPluginSource("claimer", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	pinned, _ := gin.CreateTestContext(nil)
	pinned.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	pinned.Request.Header.Set("Accept-Language", "en")
	pinned.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
	message := noAvailableChannelMessage(pinned, "default", "kling-v1")
	assert.Contains(t, message, `"claimer"`)
	assert.Contains(t, message, "disable or override")
	assert.Contains(t, message, "kling-v1")

	plain, _ := gin.CreateTestContext(nil)
	plain.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	plain.Request.Header.Set("Accept-Language", "en")
	generic := noAvailableChannelMessage(plain, "default", "gpt-4o")
	assert.NotContains(t, generic, "task plugin")
	assert.Contains(t, generic, "gpt-4o")
}
