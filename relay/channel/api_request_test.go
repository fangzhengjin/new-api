package channel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	common2 "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNewTaskAPIRequestInheritsClientCancellation(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	requestContext, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestContext)

	upstream, err := newTaskAPIRequest(c, "https://provider.example/tasks", nil)
	require.NoError(t, err)
	cancel()

	require.ErrorIs(t, upstream.Context().Err(), context.Canceled)
}

type wssAuditAdaptor struct {
	Adaptor
	url  string
	host string
}

func (a wssAuditAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.url, nil
}

func (a wssAuditAdaptor) SetupRequestHeader(_ *gin.Context, header *http.Header, _ *relaycommon.RelayInfo) error {
	header.Set("Authorization", "Bearer upstream-secret")
	header.Set("X-Audit-Trace", "trace-123")
	header.Set("X-Blocked", "removed")
	if a.host != "" {
		header.Set("Host", a.host)
	}
	return nil
}

func setRequestHeaderRulesForTest(t *testing.T, rules []operation_setting.RequestHeaderRule) {
	t.Helper()
	common2.OptionMapRWMutex.Lock()
	if common2.OptionMap == nil {
		common2.OptionMap = make(map[string]string)
	}
	previousRules, hadPreviousRules := common2.OptionMap[operation_setting.RequestHeaderRulesOptionKey]
	encodedRules, err := common2.Marshal(rules)
	require.NoError(t, err)
	common2.OptionMap[operation_setting.RequestHeaderRulesOptionKey] = string(encodedRules)
	common2.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common2.OptionMapRWMutex.Lock()
		defer common2.OptionMapRWMutex.Unlock()
		if hadPreviousRules {
			common2.OptionMap[operation_setting.RequestHeaderRulesOptionKey] = previousRules
		} else {
			delete(common2.OptionMap, operation_setting.RequestHeaderRulesOptionKey)
		}
	})
}

func tieredHeaderTestInfo(format types.RelayFormat) *relaycommon.RelayInfo {
	const expr = `tier("base", p)`
	return &relaycommon.RelayInfo{
		IsChannelTest: true,
		RelayFormat:   format,
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr", ExprString: expr, ExprHash: billingexpr.ExprHashString(expr),
			GroupRatio: 1, EstimatedPromptTokens: 1, QuotaPerUnit: 1_000_000,
		},
	}
}

func TestDoWssRequestCapturesFinalForwardedHeaders(t *testing.T) {
	setRequestHeaderRulesForTest(t, []operation_setting.RequestHeaderRule{{Name: "X-Blocked", Record: true, Forward: false}})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upgrade rejected", http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	info := tieredHeaderTestInfo(types.RelayFormatOpenAIRealtime)
	_, err := DoWssRequest(wssAuditAdaptor{url: "ws" + strings.TrimPrefix(server.URL, "http")}, context, info, nil)
	require.Error(t, err)

	other := model.NewLogOther()
	service.AppendRequestHeadersAdminInfo(context, other)
	adminInfo := other.Snapshot()["admin_info"].(map[string]interface{})
	requestHeaders := adminInfo["request_headers"].(map[string]interface{})
	outgoing := requestHeaders["outgoing"].(map[string]string)
	require.Equal(t, "trace-123", outgoing["X-Audit-Trace"])
	require.NotContains(t, outgoing, "X-Blocked")
	require.NotContains(t, outgoing, "Authorization")
	require.Equal(t, "trace-123", info.BillingRequestInput.Headers["X-Audit-Trace"])
	require.NotContains(t, info.BillingRequestInput.Headers, "X-Blocked")
	require.Equal(t, strings.TrimPrefix(server.URL, "http://"), info.BillingRequestInput.Headers["Host"])
}

func TestDoWssRequestBillsExplicitHostOverride(t *testing.T) {
	setRequestHeaderRulesForTest(t, []operation_setting.RequestHeaderRule{{Name: "Host", Record: true, Forward: true}})
	receivedHost := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		receivedHost <- request.Host
		http.Error(w, "upgrade rejected", http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	info := tieredHeaderTestInfo(types.RelayFormatOpenAIRealtime)
	_, err := DoWssRequest(wssAuditAdaptor{
		url: "ws" + strings.TrimPrefix(server.URL, "http"), host: "billing.example",
	}, context, info, nil)
	require.Error(t, err)
	require.Equal(t, "billing.example", <-receivedHost)
	require.Equal(t, "billing.example", info.BillingRequestInput.Headers["Host"])
}

func TestDoRequestBillsFinalForwardedHeaders(t *testing.T) {
	setRequestHeaderRulesForTest(t, []operation_setting.RequestHeaderRule{{Name: "X-Blocked", Forward: false}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Empty(t, request.Header.Get("X-Blocked"))
		require.Empty(t, request.Header.Get("Connection"))
		require.Empty(t, request.Header.Get("X-Hop-Only"))
		require.Equal(t, "kept", request.Header.Get("X-Final"))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	request, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{}`))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Blocked", "removed")
	request.Header.Set("Connection", "X-Hop-Only")
	request.Header.Set("X-Hop-Only", "removed")
	request.Header.Set("X-Final", "kept")
	info := tieredHeaderTestInfo(types.RelayFormatOpenAI)

	response, err := doRequest(context, request, info)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "kept", info.BillingRequestInput.Headers["X-Final"])
	require.NotContains(t, info.BillingRequestInput.Headers, "X-Blocked")
	require.NotContains(t, info.BillingRequestInput.Headers, "Connection")
	require.NotContains(t, info.BillingRequestInput.Headers, "X-Hop-Only")
	require.Equal(t, request.URL.Host, info.BillingRequestInput.Headers["Host"])
}

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

func TestProcessHeaderOverride_PassthroughSkipsTransportAndChannelOwnedHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	skipped := []string{
		"Accept-Encoding",
		"Connection",
		"Content-Length",
		"Sec-WebSocket-Key",
		"api-key",
		"OpenAI-Organization",
		"OpenAI-Project",
		"chatgpt-account-id",
		"x-goog-user-project",
		"Sec-WebSocket-Protocol",
	}
	for _, name := range skipped {
		ctx.Request.Header.Set(name, "client-value")
	}

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

	for _, name := range skipped {
		_, exists := headers[strings.ToLower(name)]
		require.False(t, exists, name)
	}
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

func TestPassCodexClientHeadersPreservesOfficialIdentity(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalEnabled := settings.RequestHeaderFallbackEnabled
	settings.RequestHeaderFallbackEnabled = true
	defer func() { settings.RequestHeaderFallbackEnabled = originalEnabled }()

	userAgents := []string{
		"Codex Desktop/0.148.0-alpha.9 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.810.52044)",
		"codex-tui/0.147.0 (Mac OS 26.5.2; arm64) ghostty/1.3.1 (codex-tui; 0.147.0)",
		"codex_cli_rs/0.147.0 (Mac OS 26.5.2; arm64) ghostty/1.3.1",
		"codex-cli/0.147.0",
	}
	for _, userAgent := range userAgents {
		t.Run(userAgent, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			ctx.Request.Header.Set("User-Agent", userAgent)
			ctx.Request.Header.Set("Originator", "official-originator")
			ctx.Request.Header.Set("Session-Id", "official-session")
			ctx.Request.Header.Set("X-Codex-Installation-Id", "official-installation")
			ctx.Request.Header.Set("X-Codex-Future-Feature", "enabled")
			ctx.Request.Header.Set("X-OAI-Attestation", "official-attestation")
			ctx.Request.Header.Set("X-OpenAI-Internal-Codex-Responses-Lite", "true")
			ctx.Request.Header.Set("Authorization", "Bearer client-secret")
			ctx.Request.Header.Set("api-key", "client-api-key")
			ctx.Request.Header.Set("OpenAI-Organization", "client-organization")
			ctx.Request.Header.Set("OpenAI-Project", "client-project")
			ctx.Request.Header.Set("chatgpt-account-id", "client-account")
			ctx.Request.Header.Set("x-goog-user-project", "client-google-project")
			ctx.Request.Header.Set("Sec-WebSocket-Protocol", "realtime, openai-insecure-api-key.client-secret")

			upstream := http.Header{
				"Authorization": {"Bearer upstream-secret"},
				"Originator":    {"adapter-originator"},
				"User-Agent":    {"adapter/1.0"},
			}
			upstream.Set("api-key", "upstream-api-key")
			upstream.Set("OpenAI-Organization", "upstream-organization")
			upstream.Set("OpenAI-Project", "upstream-project")
			upstream.Set("chatgpt-account-id", "upstream-account")
			upstream.Set("x-goog-user-project", "upstream-google-project")
			upstream.Set("Sec-WebSocket-Protocol", "realtime, openai-insecure-api-key.upstream-secret")
			info := &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatOpenAIResponses,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "gpt-5.4",
					HeadersOverride: map[string]any{
						"Originator": "manual-originator",
						"User-Agent": "manual-user-agent",
					},
				},
			}

			passCodexClientHeaders(ctx, upstream, info)
			headerOverride, err := processHeaderOverride(info, ctx)
			require.NoError(t, err)
			applyHeaderOverrideToRequest(&http.Request{Header: upstream}, headerOverride)
			NormalizeClientIdentityHeaders(ctx, &http.Request{Header: upstream}, info)

			require.Equal(t, userAgent, upstream.Get("User-Agent"))
			require.Equal(t, "manual-originator", upstream.Get("Originator"))
			require.Equal(t, "official-session", upstream.Get("Session-Id"))
			require.Equal(t, "official-installation", upstream.Get("X-Codex-Installation-Id"))
			require.Equal(t, "enabled", upstream.Get("X-Codex-Future-Feature"))
			require.Equal(t, "official-attestation", upstream.Get("X-OAI-Attestation"))
			require.Equal(t, "true", upstream.Get("X-OpenAI-Internal-Codex-Responses-Lite"))
			require.Equal(t, "official-session", upstream.Get("Thread-Id"))
			require.Equal(t, "official-session", upstream.Get("X-Client-Request-Id"))
			require.Equal(t, "Bearer upstream-secret", upstream.Get("Authorization"))
			require.Equal(t, "upstream-api-key", upstream.Get("api-key"))
			require.Equal(t, "upstream-organization", upstream.Get("OpenAI-Organization"))
			require.Equal(t, "upstream-project", upstream.Get("OpenAI-Project"))
			require.Equal(t, "upstream-account", upstream.Get("chatgpt-account-id"))
			require.Equal(t, "upstream-google-project", upstream.Get("x-goog-user-project"))
			require.Equal(t, "realtime, openai-insecure-api-key.upstream-secret", upstream.Get("Sec-WebSocket-Protocol"))
		})
	}
}

func TestPassCodexClientHeadersPreservesProvidedCodexContext(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalSettings := *settings
	settings.RequestHeaderFallbackEnabled = true
	settings.RequestHeaderModelPatterns = []string{`^gpt-.*$`}
	defer func() { *settings = originalSettings }()

	const (
		sessionID = "01a01eaf-eb1c-7f50-81ab-d9f26841f0b1"
		metadata  = `{"installation_id":"f700c4db-5030-4ef8-8950-5def682d3461","session_id":"01a01eaf-eb1c-7f50-81ab-d9f26841f0b1","thread_id":"01a01eaf-eb1c-7f50-81ab-d9f26841f0b1","turn_id":"01a01eaf-ebd4-7c31-8bc7-9b9b9a5f8b0a","window_id":"01a01eaf-eb1c-7f50-81ab-d9f26841f0b1:0","request_kind":"turn","sandbox":"windows_elevated","turn_started_at_unix_ms":1787221240808}`
	)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("User-Agent", "deepseek-harness/0.147.0 (Windows 10.0.26100; x86_64)")
	ctx.Request.Header.Set("Originator", "deepseek-harness")
	ctx.Request.Header.Set("Session-Id", sessionID)
	ctx.Request.Header.Set("Thread-Id", sessionID)
	ctx.Request.Header.Set("X-Client-Request-Id", sessionID)
	ctx.Request.Header.Set("X-Codex-Beta-Features", "remote_compaction_v2")
	ctx.Request.Header.Set("X-Codex-Turn-Metadata", metadata)
	ctx.Request.Header.Set("X-Codex-Window-Id", sessionID+":0")
	ctx.Request.Header.Set("X-OpenAI-Internal-Codex-Responses-Lite", "true")

	upstream := http.Header{
		"Authorization": {"Bearer upstream-secret"},
		"User-Agent":    {"adapter/1.0"},
	}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.4"},
	}

	passCodexClientHeaders(ctx, upstream, info)
	request := &http.Request{Header: upstream}
	NormalizeClientIdentityHeaders(ctx, request, info)

	require.Contains(t, upstream.Get("User-Agent"), "codex-tui/")
	require.Equal(t, "codex-tui", upstream.Get("Originator"))
	require.Equal(t, sessionID, upstream.Get("Session-Id"))
	require.Empty(t, upstream.Get("X-Codex-Installation-Id"))
	require.Equal(t, metadata, upstream.Get("X-Codex-Turn-Metadata"))
	require.Equal(t, "remote_compaction_v2", upstream.Get("X-Codex-Beta-Features"))
	require.Equal(t, "true", upstream.Get("X-OpenAI-Internal-Codex-Responses-Lite"))
	require.Equal(t, "Bearer upstream-secret", upstream.Get("Authorization"))

	info.ChannelMeta.UpstreamModelName = "deepseek-chat"
	unmatched := http.Header{}
	passCodexClientHeaders(ctx, unmatched, info)
	require.Empty(t, unmatched.Get("X-Codex-Turn-Metadata"))
}

func TestNormalizeClientIdentityHeadersRepairsOnlyInvalidMetadataFields(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalSettings := *settings
	settings.RequestHeaderFallbackEnabled = true
	settings.RequestHeaderModelPatterns = []string{`^gpt-.*$`}
	defer func() { *settings = originalSettings }()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	req.Header.Set("User-Agent", "codex-tui/0.147.0")
	req.Header.Set("Session-Id", "invalid\nsession")
	req.Header.Set("X-Codex-Installation-Id", "standalone-installation")
	req.Header.Set("X-Codex-Turn-Metadata", `{"installation_id":"stale-installation","session_id":"metadata-session","thread_id":"metadata-thread","turn_id":"metadata-turn","window_id":"metadata-window","request_kind":"turn","sandbox":"windows_elevated","turn_started_at_unix_ms":1787221240808,"future_field":"preserved"}`)

	NormalizeClientIdentityHeaders(nil, req, &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.4"},
	})

	require.Equal(t, "metadata-thread", req.Header.Get("Thread-Id"))
	require.Equal(t, "metadata-window", req.Header.Get("X-Codex-Window-Id"))
	require.Equal(t, "metadata-session", req.Header.Get("Session-Id"))
	var metadata map[string]any
	require.NoError(t, common2.Unmarshal([]byte(req.Header.Get("X-Codex-Turn-Metadata")), &metadata))
	require.Equal(t, "standalone-installation", metadata["installation_id"])
	require.Equal(t, "metadata-session", metadata["session_id"])
	require.Equal(t, "metadata-thread", metadata["thread_id"])
	require.Equal(t, "metadata-turn", metadata["turn_id"])
	require.Equal(t, "metadata-window", metadata["window_id"])
	require.Equal(t, "windows_elevated", metadata["sandbox"])
	require.Equal(t, float64(1787221240808), metadata["turn_started_at_unix_ms"])
	require.Equal(t, "preserved", metadata["future_field"])
}

func TestNormalizeClientIdentityHeadersRebuildsInvalidMetadataFromRelatedHeaders(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalSettings := *settings
	settings.RequestHeaderFallbackEnabled = true
	settings.RequestHeaderModelPatterns = []string{`^gpt-.*$`}
	defer func() { *settings = originalSettings }()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	req.Header.Set("Session-Id", "provided-session")
	req.Header.Set("Thread-Id", "provided-thread")
	req.Header.Set("X-Codex-Installation-Id", "provided-installation")
	req.Header.Set("X-Codex-Turn-Metadata", `null`)

	NormalizeClientIdentityHeaders(nil, req, &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.4"},
	})

	var metadata map[string]any
	require.NoError(t, common2.Unmarshal([]byte(req.Header.Get("X-Codex-Turn-Metadata")), &metadata))
	require.Equal(t, "provided-session", metadata["session_id"])
	require.Equal(t, "provided-thread", metadata["thread_id"])
	require.Equal(t, "provided-installation", metadata["installation_id"])
	require.NotEmpty(t, metadata["turn_id"])
}

func TestNormalizeClientIdentityHeadersPreservesTargetHeaders(t *testing.T) {
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
		ChannelMeta:             &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.4"},
	}

	NormalizeClientIdentityHeaders(nil, req, info)

	require.Contains(t, req.Header.Get("User-Agent"), "codex-tui/")
	require.Equal(t, "custom-originator", req.Header.Get("Originator"))
	require.Equal(t, "session-from-client", req.Header.Get("Session-Id"))
	require.Equal(t, "session-from-client", req.Header.Get("Thread-Id"))
	require.Equal(t, "session-from-client", req.Header.Get("X-Client-Request-Id"))
	require.Empty(t, req.Header.Get("X-Codex-Installation-Id"))
	require.Equal(t, "session-from-client:0", req.Header.Get("X-Codex-Window-Id"))

	var metadata map[string]any
	require.NoError(t, common2.Unmarshal([]byte(req.Header.Get("X-Codex-Turn-Metadata")), &metadata))
	require.Equal(t, "session-from-client", metadata["session_id"])
	require.Equal(t, "session-from-client", metadata["thread_id"])
	require.NotEmpty(t, metadata["installation_id"])
	require.NotEqual(t, "session-from-client", metadata["installation_id"])
	require.Equal(t, "danger-full-access", metadata["sandbox_mode"])
}

func TestNormalizeClientIdentityHeadersReplacesCrossVendorIdentity(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalSettings := *settings
	settings.RequestHeaderFallbackEnabled = true
	defer func() { *settings = originalSettings }()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	req.Header.Set("User-Agent", "claude-cli/2.1.234 (external, cli)")
	req.Header.Set("X-Claude-Code-Session-Id", "claude-session")
	info := &relaycommon.RelayInfo{
		OriginModelName:        "claude-alias",
		RelayFormat:            types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAIResponses},
		ChannelMeta:            &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.4"},
	}

	NormalizeClientIdentityHeaders(nil, req, info)

	require.Contains(t, req.Header.Get("User-Agent"), "codex-tui/")
	require.Empty(t, req.Header.Get("X-Claude-Code-Session-Id"))
}

func TestNormalizeClientIdentityHeadersUsesMappedUpstreamModelOnRetry(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalSettings := *settings
	settings.RequestHeaderFallbackEnabled = true
	settings.RequestHeaderModelPatterns = []string{`^gpt-.*$`}
	defer func() { *settings = originalSettings }()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	req.Header.Set("User-Agent", "claude-cli/2.1.234 (external, cli)")
	info := &relaycommon.RelayInfo{
		OriginModelName:        "gpt-client-alias",
		RelayFormat:            types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAIResponses},
		ChannelMeta:            &relaycommon.ChannelMeta{UpstreamModelName: "deepseek-chat"},
	}

	NormalizeClientIdentityHeaders(nil, req, info)
	require.Equal(t, "claude-cli/2.1.234 (external, cli)", req.UserAgent())

	info.ChannelMeta.UpstreamModelName = "gpt-5.4"
	NormalizeClientIdentityHeaders(nil, req, info)
	require.Contains(t, req.UserAgent(), "codex-tui/")
}

func TestNormalizeClientIdentityHeadersReplacesCodexWithClaude(t *testing.T) {
	settings := model_setting.GetClaudeSettings()
	originalSettings := *settings
	settings.RequestHeaderFallbackEnabled = true
	settings.RequestHeaderFallbackVersion = "2.1.234"
	settings.RequestHeaderModelPatterns = []string{`^claude-.*$`}
	defer func() { *settings = originalSettings }()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/messages", nil)
	req.Header.Set("User-Agent", "codex-tui/0.147.0")
	req.Header.Set("Originator", "codex-tui")
	req.Header.Set("Session-Id", "codex-session")
	req.Header.Set("X-Codex-Installation-Id", "codex-installation")
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Authorization", "Bearer upstream-secret")
	info := &relaycommon.RelayInfo{
		RelayFormat:             types.RelayFormatOpenAIResponses,
		FinalRequestRelayFormat: types.RelayFormatClaude,
		ChannelMeta:             &relaycommon.ChannelMeta{UpstreamModelName: "claude-sonnet-4-5"},
	}

	NormalizeClientIdentityHeaders(nil, req, info)

	require.Equal(t, "claude-cli/2.1.234 (external, cli)", req.UserAgent())
	require.Equal(t, "cli", req.Header.Get("X-App"))
	require.Empty(t, req.Header.Get("Originator"))
	require.Empty(t, req.Header.Get("Session-Id"))
	require.Empty(t, req.Header.Get("X-Codex-Installation-Id"))
	require.Equal(t, "2023-06-01", req.Header.Get("Anthropic-Version"))
	require.Equal(t, "Bearer upstream-secret", req.Header.Get("Authorization"))
}

func TestNormalizeClientIdentityHeadersPreservesClaudeIdentity(t *testing.T) {
	settings := model_setting.GetClaudeSettings()
	originalSettings := *settings
	settings.RequestHeaderFallbackEnabled = true
	settings.RequestHeaderModelPatterns = []string{`^claude-.*$`}
	defer func() { *settings = originalSettings }()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/messages", nil)
	req.Header.Set("User-Agent", "claude-cli/2.1.234 (external, claude-desktop)")
	req.Header.Set("X-App", "claude-desktop")
	req.Header.Set("X-Claude-Code-Session-Id", "claude-session")
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-sonnet-4-5"},
	}

	NormalizeClientIdentityHeaders(nil, req, info)

	require.Equal(t, "claude-cli/2.1.234 (external, claude-desktop)", req.UserAgent())
	require.Equal(t, "claude-desktop", req.Header.Get("X-App"))
	require.Equal(t, "claude-session", req.Header.Get("X-Claude-Code-Session-Id"))
}

func TestNormalizeClientIdentityHeadersAlphaSearchUsesMinimalCodexIdentity(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalSettings := *settings
	settings.RequestHeaderFallbackEnabled = true
	settings.RequestHeaderModelPatterns = []string{`^codex-.*$`}
	defer func() { *settings = originalSettings }()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/alpha/search", nil)
	req.Header.Set("User-Agent", "claude-cli/2.1.234 (external, cli)")
	req.Header.Set("X-Stainless-Runtime", "node")
	info := &relaycommon.RelayInfo{
		RelayFormat:             types.RelayFormatClaude,
		FinalRequestRelayFormat: types.RelayFormatOpenAIAlphaSearch,
		ChannelMeta:             &relaycommon.ChannelMeta{UpstreamModelName: "codex-auto-review"},
	}

	NormalizeClientIdentityHeaders(nil, req, info)

	require.Contains(t, req.UserAgent(), "codex-tui/")
	require.Equal(t, "codex-tui", req.Header.Get("Originator"))
	require.Empty(t, req.Header.Get("X-Stainless-Runtime"))
	require.Empty(t, req.Header.Get("Session-Id"))
	require.Empty(t, req.Header.Get("X-Codex-Turn-Metadata"))
}

func TestNormalizeClientIdentityHeadersUsesPromptCacheKeyForSession(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalEnabled := settings.RequestHeaderFallbackEnabled
	settings.RequestHeaderFallbackEnabled = true
	defer func() { settings.RequestHeaderFallbackEnabled = originalEnabled }()

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.4"},
		Request: &dto.OpenAIResponsesRequest{
			PromptCacheKey: []byte(`"cache-key"`),
		},
	}
	first := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	second := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)

	NormalizeClientIdentityHeaders(nil, first, info)
	NormalizeClientIdentityHeaders(nil, second, info)

	require.NotEmpty(t, first.Header.Get("Session-Id"))
	require.Equal(t, first.Header.Get("Session-Id"), second.Header.Get("Session-Id"))
}

func TestNormalizeClientIdentityHeadersScopesMetadataInstallationToChannel(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalEnabled := settings.RequestHeaderFallbackEnabled
	settings.RequestHeaderFallbackEnabled = true
	defer func() { settings.RequestHeaderFallbackEnabled = originalEnabled }()

	installationForChannel := func(channelID int) string {
		req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
		NormalizeClientIdentityHeaders(nil, req, &relaycommon.RelayInfo{
			RelayFormat: types.RelayFormatOpenAIResponses,
			ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID, UpstreamModelName: "gpt-5.4"},
		})
		require.Empty(t, req.Header.Get("X-Codex-Installation-Id"))
		var metadata map[string]any
		require.NoError(t, common2.Unmarshal([]byte(req.Header.Get("X-Codex-Turn-Metadata")), &metadata))
		installationID, ok := metadata["installation_id"].(string)
		require.True(t, ok)
		require.NotEmpty(t, installationID)
		return installationID
	}

	first := installationForChannel(101)
	retry := installationForChannel(101)
	otherChannel := installationForChannel(202)
	require.Equal(t, first, retry)
	require.NotEqual(t, first, otherChannel)
}

func TestNormalizeClientIdentityHeadersKeepsSessionAcrossRetries(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalEnabled := settings.RequestHeaderFallbackEnabled
	settings.RequestHeaderFallbackEnabled = true
	defer func() { settings.RequestHeaderFallbackEnabled = originalEnabled }()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common2.RequestIdKey, "request-123")
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.4"},
	}
	first := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	second := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)

	NormalizeClientIdentityHeaders(c, first, info)
	NormalizeClientIdentityHeaders(c, second, info)

	require.NotEmpty(t, first.Header.Get("Session-Id"))
	require.Equal(t, first.Header.Get("Session-Id"), second.Header.Get("Session-Id"))

	var firstMetadata map[string]any
	var secondMetadata map[string]any
	require.NoError(t, common2.Unmarshal([]byte(first.Header.Get("X-Codex-Turn-Metadata")), &firstMetadata))
	require.NoError(t, common2.Unmarshal([]byte(second.Header.Get("X-Codex-Turn-Metadata")), &secondMetadata))
	firstTurnID, ok := firstMetadata["turn_id"].(string)
	require.True(t, ok)
	require.Equal(t, firstTurnID, secondMetadata["turn_id"])
	turnID, err := uuid.Parse(firstTurnID)
	require.NoError(t, err)
	require.Equal(t, uuid.Version(7), turnID.Version())
}

func TestNormalizeClientIdentityHeadersProtocolScope(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalEnabled := settings.RequestHeaderFallbackEnabled
	defer func() { settings.RequestHeaderFallbackEnabled = originalEnabled }()
	settings.RequestHeaderFallbackEnabled = true

	chat := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", nil)
	NormalizeClientIdentityHeaders(nil, chat, &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.4"},
	})
	require.Contains(t, chat.Header.Get("User-Agent"), "codex-tui/")
	require.Empty(t, chat.Header.Get("Originator"))

	for _, format := range []types.RelayFormat{
		types.RelayFormatOpenAIImage,
		types.RelayFormatOpenAIAudio,
		types.RelayFormatEmbedding,
	} {
		req := httptest.NewRequest(http.MethodPost, "https://example.com", nil)
		NormalizeClientIdentityHeaders(nil, req, &relaycommon.RelayInfo{
			RelayFormat: format,
			ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.4"},
		})
		require.Contains(t, req.Header.Get("User-Agent"), "codex-tui/")
		require.Empty(t, req.Header.Get("Originator"))
	}

	manual := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	manual.Header.Set("User-Agent", "custom-client/1.0")
	NormalizeClientIdentityHeaders(nil, manual, &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.4"},
	})
	require.Contains(t, manual.Header.Get("User-Agent"), "codex-tui/")
	require.Equal(t, "codex-tui", manual.Header.Get("Originator"))

	compaction := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses/compact", nil)
	NormalizeClientIdentityHeaders(nil, compaction, &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponsesCompaction,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "codex-auto-review"},
	})
	require.Contains(t, compaction.Header.Get("User-Agent"), "codex-tui/")
	require.Equal(t, "codex-tui", compaction.Header.Get("Originator"))
	var metadata map[string]any
	require.NoError(t, common2.Unmarshal([]byte(compaction.Header.Get("X-Codex-Turn-Metadata")), &metadata))
	require.Equal(t, "compaction", metadata["request_kind"])

	nonOpenAI := httptest.NewRequest(http.MethodPost, "https://example.com/v1/messages", nil)
	NormalizeClientIdentityHeaders(nil, nonOpenAI, &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-sonnet-4"},
	})
	require.Empty(t, nonOpenAI.Header.Get("User-Agent"))

	settings.RequestHeaderFallbackEnabled = false
	disabled := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	NormalizeClientIdentityHeaders(nil, disabled, &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.4"},
	})
	require.Empty(t, disabled.Header.Get("User-Agent"))
}

func TestDoRequestAppliesCodexRequestHeaderFallbackBeforeTransport(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	originalSettings := *settings
	settings.RequestHeaderFallbackEnabled = true
	settings.RequestHeaderFallbackClient = "custom-cli"
	settings.RequestHeaderFallbackVersion = "9.8.7"
	defer func() { *settings = originalSettings }()
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
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.4"},
	}

	resp, err := doRequest(c, req, info)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	headers := <-receivedHeaders
	require.Equal(t, "custom-cli/9.8.7 (Mac OS 26.5.2; arm64) ghostty/1.3.1 (custom-cli; 9.8.7)", headers.Get("User-Agent"))
	require.Equal(t, "custom-cli", headers.Get("Originator"))
	require.NotEmpty(t, headers.Get("Session-Id"))
	other := model.NewLogOther()
	service.AppendRequestHeadersAdminInfo(c, other)
	adminInfo := other.Snapshot()["admin_info"].(map[string]interface{})
	requestHeaders := adminInfo["request_headers"].(map[string]interface{})
	auditedHeaders := requestHeaders["outgoing"].(map[string]string)
	require.Equal(t, headers.Get("User-Agent"), auditedHeaders["User-Agent"])
}

func TestDoRequestRemovesBlockedHeadersBeforeAuditAndTransport(t *testing.T) {
	common2.OptionMapRWMutex.Lock()
	if common2.OptionMap == nil {
		common2.OptionMap = make(map[string]string)
	}
	previousRules, hadPreviousRules := common2.OptionMap[operation_setting.RequestHeaderRulesOptionKey]
	encodedRules, err := common2.Marshal([]operation_setting.RequestHeaderRule{
		{Name: "Host", Record: true, Forward: false},
	})
	require.NoError(t, err)
	common2.OptionMap[operation_setting.RequestHeaderRulesOptionKey] = string(encodedRules)
	common2.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common2.OptionMapRWMutex.Lock()
		defer common2.OptionMapRWMutex.Unlock()
		if hadPreviousRules {
			common2.OptionMap[operation_setting.RequestHeaderRulesOptionKey] = previousRules
		} else {
			delete(common2.OptionMap, operation_setting.RequestHeaderRulesOptionKey)
		}
	})
	service.InitHttpClient()

	type receivedRequest struct {
		header http.Header
		host   string
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		received <- receivedRequest{header: req.Header.Clone(), host: req.Host}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/test", nil)
	req, err := http.NewRequest(http.MethodPost, server.URL, http.NoBody)
	require.NoError(t, err)
	req.Host = "spoofed.example"
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Stainless-Runtime", "node")
	req.Header.Set("X-Trace-Id", "trace-123")

	resp, err := doRequest(c, req, &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatGemini,
		ChannelMeta: &relaycommon.ChannelMeta{},
	})
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	upstream := <-received
	require.Empty(t, upstream.header.Get("Cache-Control"))
	require.Equal(t, "application/json", upstream.header.Get("Content-Type"))
	require.Empty(t, upstream.header.Get("X-Stainless-Runtime"))
	require.Equal(t, "trace-123", upstream.header.Get("X-Trace-Id"))
	require.NotEqual(t, "spoofed.example", upstream.host)
	other := model.NewLogOther()
	service.AppendRequestHeadersAdminInfo(c, other)
	adminInfo := other.Snapshot()["admin_info"].(map[string]interface{})
	requestHeaders := adminInfo["request_headers"].(map[string]interface{})
	auditedHeaders := requestHeaders["outgoing"].(map[string]string)
	require.NotContains(t, auditedHeaders, "Cache-Control")
	require.NotContains(t, auditedHeaders, "Content-Type")
	require.NotContains(t, auditedHeaders, "X-Stainless-Runtime")
	require.Equal(t, "trace-123", auditedHeaders["X-Trace-Id"])
}
