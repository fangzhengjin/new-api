package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

// InitTask persists the submission key for every channel type: multi-key
// channels may hold keys of different upstream accounts, and the task must
// keep polling with the single key that submitted it.
func TestInitTaskPersistsSubmissionKeyForAllChannelTypes(t *testing.T) {
	channelTypes := []int{
		constant.ChannelTypeGemini,
		constant.ChannelTypeVertexAi,
		constant.ChannelTypeNewAPI,
		constant.ChannelTypeKling,
	}
	for _, channelType := range channelTypes {
		relayInfo := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: channelType,
			ApiKey:      "sk-submit",
		}}
		task := InitTask(constant.TaskPlatform("kling"), relayInfo)
		assert.Equal(t, "sk-submit", task.PrivateData.Key, "channel type %d should persist the submission key", channelType)
	}
}
