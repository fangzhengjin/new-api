package deepseek

import (
	"io"
	"net/http"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInjectDeepSeekUserID(t *testing.T) {
	tests := []struct {
		name     string
		info     *relaycommon.RelayInfo
		body     string
		expected string
	}{
		{
			name:     "chat completions",
			info:     &relaycommon.RelayInfo{UserId: 42, RelayMode: relayconstant.RelayModeChatCompletions},
			body:     `{"model":"deepseek-chat"}`,
			expected: `{"model":"deepseek-chat","user_id":"42"}`,
		},
		{
			name:     "responses",
			info:     &relaycommon.RelayInfo{UserId: 42, RelayMode: relayconstant.RelayModeResponses},
			body:     `{"model":"deepseek-v4","user_id":"client"}`,
			expected: `{"model":"deepseek-v4","user_id":"42"}`,
		},
		{
			name:     "anthropic",
			info:     &relaycommon.RelayInfo{UserId: 42, RelayFormat: types.RelayFormatClaude},
			body:     `{"model":"deepseek-chat","metadata":{"trace_id":"keep","user_id":"client"}}`,
			expected: `{"model":"deepseek-chat","metadata":{"trace_id":"keep","user_id":"42"}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestBody, err := injectDeepSeekUserID(strings.NewReader(test.body), test.info)
			require.NoError(t, err)

			request, err := http.NewRequest(http.MethodPost, "https://api.deepseek.com", requestBody)
			require.NoError(t, err)
			data, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			assert.JSONEq(t, test.expected, string(data))
			assert.Equal(t, int64(len(data)), request.ContentLength)

			require.NotNil(t, request.GetBody)
			replay, err := request.GetBody()
			require.NoError(t, err)
			replayedData, err := io.ReadAll(replay)
			require.NoError(t, err)
			assert.Equal(t, data, replayedData)
		})
	}
}
