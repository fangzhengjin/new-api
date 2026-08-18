package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTextOtherInfoWritesReasoningEffort(t *testing.T) {
	tests := []struct {
		name     string
		request  string
		final    string
		checked  bool
		expected any
	}{
		{name: "transition", request: "xhigh", final: "high", expected: map[string]string{"original": "xhigh", "final": "high"}},
		{name: "upstream-only value", final: "medium", expected: map[string]string{"final": "medium"}},
		{name: "removed upstream value", request: "high", checked: true, expected: map[string]string{"original": "high"}},
		{name: "unchanged value", request: "high", final: "high", expected: "high"},
		{name: "missing value", checked: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			now := time.Unix(100, 0)
			info := &relaycommon.RelayInfo{
				StartTime:              now,
				FirstResponseTime:      now,
				ReasoningEffortRequest: test.request,
				ReasoningEffort:        test.final,
				ReasoningEffortChecked: test.checked,
				ChannelMeta:            &relaycommon.ChannelMeta{},
			}

			other := GenerateTextOtherInfo(c, info, 0, 0, 0, 0, 0, 0, 0)

			assert.Equal(t, test.expected, other["reasoning_effort"])
		})
	}
}

func TestGenerateTextOtherInfoRecordsAdminUserAgentAudit(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	optionMapWasNil := common.OptionMap == nil
	if optionMapWasNil {
		common.OptionMap = map[string]string{}
	}
	previousRules, hadPreviousRules := common.OptionMap[operation_setting.RequestHeaderRulesOptionKey]
	encodedRules, err := common.Marshal([]operation_setting.RequestHeaderRule{{
		Name: "X-Noise-*", Record: false, Forward: true,
	}})
	require.NoError(t, err)
	common.OptionMap[operation_setting.RequestHeaderRulesOptionKey] = string(encodedRules)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if hadPreviousRules {
			common.OptionMap[operation_setting.RequestHeaderRulesOptionKey] = previousRules
		} else {
			delete(common.OptionMap, operation_setting.RequestHeaderRulesOptionKey)
		}
		if optionMapWasNil {
			common.OptionMap = nil
		}
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", strings.Repeat("a", 600))
	c.Request.Header.Set("X-OpenAI-Subagent", "guardian")
	c.Request.Header.Set("X-Custom-Trace", "trace-value")
	c.Request.Header.Set("X-Noise-Debug", "noise")
	c.Request.Header.Set("Authorization", "secret")
	CaptureUpstreamRequestHeadersAudit(c, http.Header{
		"A-Oversized":      []string{strings.Repeat("b", operation_setting.RequestHeaderAuditCapacityBytes+1)},
		"Authorization":    []string{"secret"},
		"B-Small":          []string{"kept-after-oversized"},
		"User-Agent":       []string{"codex-tui/0.147.0"},
		"X-Noise-Upstream": []string{"noise"},
	})
	now := time.Unix(100, 0)
	info := &relaycommon.RelayInfo{
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}

	other := GenerateTextOtherInfo(c, info, 0, 0, 0, 0, 0, 0, 0)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	requestHeaders, ok := adminInfo["request_headers"].(map[string]interface{})
	require.True(t, ok)
	incoming, ok := requestHeaders["incoming"].(map[string]string)
	require.True(t, ok)
	outgoing, ok := requestHeaders["outgoing"].(map[string]string)
	require.True(t, ok)

	assert.Len(t, incoming["User-Agent"], 600)
	assert.Equal(t, "guardian", incoming["X-Openai-Subagent"])
	assert.Equal(t, "trace-value", incoming["X-Custom-Trace"])
	assert.NotContains(t, incoming, "X-Noise-Debug")
	assert.NotContains(t, incoming, "Authorization")
	assert.Equal(t, "codex-tui/0.147.0", outgoing["User-Agent"])
	assert.Equal(t, "kept-after-oversized", outgoing["B-Small"])
	assert.NotContains(t, outgoing, "A-Oversized")
	assert.NotContains(t, outgoing, "X-Noise-Upstream")
	assert.NotContains(t, outgoing, "Authorization")

	omittedByDirection, ok := requestHeaders["omitted"].(map[string]interface{})
	require.True(t, ok)
	outgoingOmitted, ok := omittedByDirection["outgoing"].([]requestHeaderAuditOmission)
	require.True(t, ok)
	assert.Equal(t, []requestHeaderAuditOmission{{
		Name:       "A-Oversized",
		ByteLength: operation_setting.RequestHeaderAuditCapacityBytes + 1,
	}}, outgoingOmitted)

	encoded, err := common.Marshal(other)
	require.NoError(t, err)
	var decoded struct {
		AdminInfo struct {
			RequestHeaders struct {
				Omitted struct {
					Outgoing []requestHeaderAuditOmission `json:"outgoing"`
				} `json:"omitted"`
			} `json:"request_headers"`
		} `json:"admin_info"`
	}
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	assert.Equal(t, outgoingOmitted, decoded.AdminInfo.RequestHeaders.Omitted.Outgoing)
}
