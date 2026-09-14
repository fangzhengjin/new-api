package service

import (
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTextOtherInfoWritesRelayRetryTargets(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("use_channel", []string{"2"})
	finalKeyIndex := 7
	now := time.Unix(100, 0)
	info := &relaycommon.RelayInfo{
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelIsMultiKey:    true,
			ChannelMultiKeyIndex: finalKeyIndex,
		},
		RetryTargets: []relaycommon.RelayRetryTarget{
			{ChannelId: 2, MultiKeyIndex: &finalKeyIndex},
		},
		RetryTargetsTruncated: true,
	}

	other := GenerateTextOtherInfo(c, info, 0, 0, 0, 0, 0, 0, 0).Snapshot()
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	targets, ok := adminInfo["retry_targets"].([]relaycommon.RelayRetryTarget)
	require.True(t, ok)
	assert.Equal(t, info.RetryTargets, targets)
	assert.Equal(t, []string{"2"}, adminInfo["use_channel"])
	assert.Equal(t, 7, adminInfo["multi_key_index"])
	assert.Equal(t, true, adminInfo["retry_targets_truncated"])
}
