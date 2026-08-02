package service

import (
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
