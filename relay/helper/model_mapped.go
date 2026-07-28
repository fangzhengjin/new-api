package helper

import (
	"encoding/json"
	"fmt"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// ModelMappedHelper applies one channel model mapping to the relay request.
// It reads the mapping from c, updates info and the optional request, and returns any parsing error.
func ModelMappedHelper(c *gin.Context, info *common.RelayInfo, request dto.Request) error {
	if info.ChannelMeta == nil {
		info.ChannelMeta = &common.ChannelMeta{}
	}

	// map model name
	modelMapping := c.GetString("model_mapping")
	if modelMapping != "" && modelMapping != "{}" {
		modelMap := make(map[string]string)
		err := json.Unmarshal([]byte(modelMapping), &modelMap)
		if err != nil {
			return fmt.Errorf("unmarshal_model_mapping_failed")
		}

		if mappedModel, exists := modelMap[info.OriginModelName]; exists && mappedModel != "" {
			info.IsModelMapped = mappedModel != info.OriginModelName
			if info.IsModelMapped {
				info.UpstreamModelName = mappedModel
			}
		}
	}

	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}
