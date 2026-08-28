package model_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCodexErrorResponseMappings(t *testing.T) {
	t.Parallel()

	valid := `[{"match":{"status_code":429,"message_patterns":["Request rate increased too quickly.","re:(request|token).*rate limit exceeded"]},"rewrite":{"status_code":503,"type":"server_error","code":"server_is_overloaded","message":"Temporary upstream rate limit"}}]`
	require.NoError(t, ValidateCodexErrorResponseMappings(`[]`))
	require.NoError(t, ValidateCodexErrorResponseMappings(valid))

	testCases := []string{
		`{}`,
		`[{"match":{"status_code":99,"message_patterns":["rate limit"]},"rewrite":{"status_code":503,"type":"server_error","code":"server_is_overloaded","message":"Temporary upstream rate limit"}}]`,
		`[{"match":{"status_code":200,"message_patterns":["rate limit"]},"rewrite":{"status_code":503,"type":"server_error","code":"server_is_overloaded","message":"Temporary upstream rate limit"}}]`,
		`[{"match":{"status_code":429,"message_patterns":[]},"rewrite":{"status_code":503,"type":"server_error","code":"server_is_overloaded","message":"Temporary upstream rate limit"}}]`,
		`[{"match":{"status_code":429,"message_patterns":["re:("]},"rewrite":{"status_code":503,"type":"server_error","code":"server_is_overloaded","message":"Temporary upstream rate limit"}}]`,
		`[{"match":{"status_code":429,"message_patterns":["rate limit"]},"rewrite":{"status_code":600,"type":"server_error","code":"server_is_overloaded","message":"Temporary upstream rate limit"}}]`,
		`[{"match":{"status_code":429,"message_patterns":["rate limit"]},"rewrite":{"status_code":503,"type":"","code":"server_is_overloaded","message":"Temporary upstream rate limit"}}]`,
	}

	for _, value := range testCases {
		assert.Error(t, ValidateCodexErrorResponseMappings(value), value)
	}
}

func TestMatchCodexErrorResponse(t *testing.T) {
	settings := GetCodexSettings()
	original := cloneCodexSettings(*settings)
	t.Cleanup(func() { *settings = original })
	settings.ErrorResponseMappings = []CodexErrorResponseMapping{
		{
			Match: CodexErrorResponseMatch{
				StatusCode:      429,
				MessagePatterns: []string{"Request rate increased too quickly.", `re:(request|token).*rate limit exceeded`},
			},
			Rewrite: CodexErrorResponseRewrite{
				StatusCode: 503,
				Type:       "server_error",
				Code:       "server_is_overloaded",
				Message:    "Temporary upstream rate limit",
			},
		},
	}

	testCases := []struct {
		name       string
		userAgent  string
		path       string
		statusCode int
		message    string
		matched    bool
	}{
		{name: "plain contains ignores case", userAgent: "codex-tui/0.152.1", path: "/v1/responses", statusCode: 429, message: "request RATE increased too quickly. To ensure system stability.", matched: true},
		{name: "regex matches alternate vendor wording", userAgent: "codex_cli_rs/0.152.1", path: "/v1/responses", statusCode: 429, message: "Token concurrency rate limit exceeded", matched: true},
		{name: "Desktop is recognized", userAgent: "Codex Desktop/0.148.0-alpha.9 (macOS 26.6; arm64) unknown (Codex Desktop; 26.810.52044)", path: "/v1/responses", statusCode: 429, message: "Request rate increased too quickly.", matched: true},
		{name: "hard quota is not matched", userAgent: "codex-tui/0.152.1", path: "/v1/responses", statusCode: 429, message: "Allocated quota exceeded", matched: false},
		{name: "wrong upstream status", userAgent: "codex-tui/0.152.1", path: "/v1/responses", statusCode: 500, message: "Request rate increased too quickly.", matched: false},
		{name: "non Codex client", userAgent: "curl/8.7.1", path: "/v1/responses", statusCode: 429, message: "Request rate increased too quickly.", matched: false},
		{name: "compact endpoint", userAgent: "codex-tui/0.152.1", path: "/v1/responses/compact", statusCode: 429, message: "Request rate increased too quickly.", matched: false},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			rewrite, matched := MatchCodexErrorResponse(test.userAgent, test.path, test.statusCode, test.message)
			assert.Equal(t, test.matched, matched)
			if test.matched {
				require.NotNil(t, rewrite)
				assert.Equal(t, 503, rewrite.StatusCode)
				assert.Equal(t, "server_is_overloaded", rewrite.Code)
			} else {
				assert.Nil(t, rewrite)
			}
		})
	}
}
