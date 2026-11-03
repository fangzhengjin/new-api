package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteCodexMappedErrorResponseUsesOriginalUpstreamError(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	original := *settings
	t.Cleanup(func() { *settings = original })
	settings.ErrorResponseMappings = []model_setting.CodexErrorResponseMapping{
		{
			Match: model_setting.CodexErrorResponseMatch{
				StatusCode:      429,
				MessagePatterns: []string{"Request rate increased too quickly."},
			},
			Rewrite: model_setting.CodexErrorResponseRewrite{
				StatusCode: 503,
				Type:       "server_error",
				Code:       "server_is_overloaded",
				Message:    "Temporary upstream rate limit",
			},
		},
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	context.Request.Header.Set("User-Agent", "codex-tui/0.152.1")
	upstreamError := types.WithOpenAIError(types.OpenAIError{
		Message: "Request rate increased too quickly. To ensure system stability.",
		Type:    "rate_limit_error",
		Code:    "rate_limit_exceeded",
	}, http.StatusServiceUnavailable)
	upstreamError.UpstreamStatusCode = http.StatusTooManyRequests

	require.True(t, writeCodexMappedErrorResponse(context, types.RelayFormatOpenAIResponses, upstreamError))
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.JSONEq(t, `{"error":{"type":"server_error","code":"server_is_overloaded","message":"Temporary upstream rate limit"}}`, recorder.Body.String())
}

func TestWriteCodexMappedErrorResponsePreservesOutOfScopeErrors(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	original := *settings
	t.Cleanup(func() { *settings = original })
	settings.ErrorResponseMappings = []model_setting.CodexErrorResponseMapping{
		{
			Match:   model_setting.CodexErrorResponseMatch{StatusCode: 429, MessagePatterns: []string{"rate limit"}},
			Rewrite: model_setting.CodexErrorResponseRewrite{StatusCode: 503, Type: "server_error", Code: "server_is_overloaded", Message: "Temporary upstream rate limit"},
		},
	}

	testCases := []struct {
		name      string
		userAgent string
		path      string
		format    types.RelayFormat
	}{
		{name: "non Codex client", userAgent: "curl/8.7.1", path: "/v1/responses", format: types.RelayFormatOpenAIResponses},
		{name: "compact endpoint", userAgent: "codex-tui/0.152.1", path: "/v1/responses/compact", format: types.RelayFormatOpenAIResponsesCompaction},
		{name: "chat completions", userAgent: "codex-tui/0.152.1", path: "/v1/chat/completions", format: types.RelayFormatOpenAI},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, test.path, nil)
			context.Request.Header.Set("User-Agent", test.userAgent)
			upstreamError := types.WithOpenAIError(types.OpenAIError{Message: "rate limit", Type: "rate_limit_error", Code: "rate_limit_exceeded"}, http.StatusTooManyRequests)
			upstreamError.UpstreamStatusCode = http.StatusTooManyRequests

			assert.False(t, writeCodexMappedErrorResponse(context, test.format, upstreamError))
			assert.Empty(t, recorder.Body.String())
		})
	}
}

func TestWriteCodexMappedErrorResponsePreservesLocalErrors(t *testing.T) {
	settings := model_setting.GetCodexSettings()
	original := *settings
	t.Cleanup(func() { *settings = original })
	settings.ErrorResponseMappings = []model_setting.CodexErrorResponseMapping{
		{
			Match:   model_setting.CodexErrorResponseMatch{StatusCode: 429, MessagePatterns: []string{"rate limit"}},
			Rewrite: model_setting.CodexErrorResponseRewrite{StatusCode: 503, Type: "server_error", Code: "server_is_overloaded", Message: "Temporary upstream rate limit"},
		},
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	context.Request.Header.Set("User-Agent", "codex-tui/0.152.1")
	localError := types.NewErrorWithStatusCode(
		assert.AnError,
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)
	localError.SetMessage("rate limit")

	assert.False(t, writeCodexMappedErrorResponse(context, types.RelayFormatOpenAIResponses, localError))
	assert.Empty(t, recorder.Body.String())
}
