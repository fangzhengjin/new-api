package channel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	common2 "github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}

func TestApplyCodexRequestHeaderFallbackPreservesManualHeaders(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalEnabled := settings.RequestHeaderFallbackEnabled
	settings.RequestHeaderFallbackEnabled = true
	defer func() { settings.RequestHeaderFallbackEnabled = originalEnabled }()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	req.Header.Set("Originator", "custom-originator")
	req.Header.Set("Session_id", "session-from-client")
	info := &relaycommon.RelayInfo{
		RelayFormat:             types.RelayFormatClaude,
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	ApplyCodexRequestHeaderFallback(nil, req, info)

	require.Contains(t, req.Header.Get("User-Agent"), "codex-tui/")
	require.Equal(t, "custom-originator", req.Header.Get("Originator"))
	require.Equal(t, "session-from-client", req.Header.Get("Session-Id"))
	require.Equal(t, "session-from-client", req.Header.Get("Thread-Id"))
	require.Equal(t, "session-from-client", req.Header.Get("X-Client-Request-Id"))
	require.Equal(t, "session-from-client", req.Header.Get("X-Codex-Installation-Id"))
	require.Equal(t, "session-from-client:0", req.Header.Get("X-Codex-Window-Id"))

	var metadata map[string]any
	require.NoError(t, common2.Unmarshal([]byte(req.Header.Get("X-Codex-Turn-Metadata")), &metadata))
	require.Equal(t, "session-from-client", metadata["session_id"])
	require.Equal(t, "session-from-client", metadata["thread_id"])
}

func TestApplyCodexRequestHeaderFallbackUsesPromptCacheKeyForSession(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalEnabled := settings.RequestHeaderFallbackEnabled
	settings.RequestHeaderFallbackEnabled = true
	defer func() { settings.RequestHeaderFallbackEnabled = originalEnabled }()

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		Request: &dto.OpenAIResponsesRequest{
			PromptCacheKey: []byte(`"cache-key"`),
		},
	}
	first := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	second := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)

	ApplyCodexRequestHeaderFallback(nil, first, info)
	ApplyCodexRequestHeaderFallback(nil, second, info)

	require.NotEmpty(t, first.Header.Get("Session-Id"))
	require.Equal(t, first.Header.Get("Session-Id"), second.Header.Get("Session-Id"))
}

func TestApplyCodexRequestHeaderFallbackProtocolScope(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalEnabled := settings.RequestHeaderFallbackEnabled
	defer func() { settings.RequestHeaderFallbackEnabled = originalEnabled }()
	settings.RequestHeaderFallbackEnabled = true

	chat := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)
	ApplyCodexRequestHeaderFallback(nil, chat, &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI})
	require.Contains(t, chat.Header.Get("User-Agent"), "codex-tui/")
	require.Empty(t, chat.Header.Get("Originator"))

	for _, format := range []types.RelayFormat{
		types.RelayFormatOpenAIImage,
		types.RelayFormatOpenAIAudio,
		types.RelayFormatEmbedding,
	} {
		req := httptest.NewRequest(http.MethodPost, "https://example.com", nil)
		ApplyCodexRequestHeaderFallback(nil, req, &relaycommon.RelayInfo{RelayFormat: format})
		require.Contains(t, req.Header.Get("User-Agent"), "codex-tui/")
		require.Empty(t, req.Header.Get("Originator"))
	}

	manual := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	manual.Header.Set("User-Agent", "custom-client/1.0")
	ApplyCodexRequestHeaderFallback(nil, manual, &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses})
	require.Equal(t, "custom-client/1.0", manual.Header.Get("User-Agent"))
	require.Empty(t, manual.Header.Get("Originator"))

	nonOpenAI := httptest.NewRequest(http.MethodPost, "https://example.com/v1/messages", nil)
	ApplyCodexRequestHeaderFallback(nil, nonOpenAI, &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude})
	require.Empty(t, nonOpenAI.Header.Get("User-Agent"))

	settings.RequestHeaderFallbackEnabled = false
	disabled := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	ApplyCodexRequestHeaderFallback(nil, disabled, &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses})
	require.Empty(t, disabled.Header.Get("User-Agent"))
}

func TestDoRequestAppliesCodexRequestHeaderFallbackBeforeTransport(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalEnabled := settings.RequestHeaderFallbackEnabled
	settings.RequestHeaderFallbackEnabled = true
	defer func() { settings.RequestHeaderFallbackEnabled = originalEnabled }()
	service.InitHttpClient()

	receivedHeaders := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		receivedHeaders <- req.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/responses", http.NoBody)
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	resp, err := doRequest(c, req, info)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	headers := <-receivedHeaders
	require.Contains(t, headers.Get("User-Agent"), "codex-tui/")
	require.Equal(t, codexTUIClientName, headers.Get("Originator"))
	require.NotEmpty(t, headers.Get("Session-Id"))
}
