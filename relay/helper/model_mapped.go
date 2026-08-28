package helper

import (
	"errors"
	"fmt"

	rootcommon "github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hostreasoning "github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/gin-gonic/gin"
)

// ModelMappedHelper applies one channel model mapping to the relay request.
// It reads the mapping from c, updates info and the optional request, and returns any parsing error.
func ModelMappedHelper(c *gin.Context, info *relaycommon.RelayInfo, request dto.Request) error {
	return modelMappedHelper(c, info, request, false)
}

// TaskModelMappedHelper follows a channel mapping chain so task aliases reach the model declared by the selected plugin.
func TaskModelMappedHelper(c *gin.Context, info *relaycommon.RelayInfo, request dto.Request) error {
	return modelMappedHelper(c, info, request, true)
}

func modelMappedHelper(c *gin.Context, info *relaycommon.RelayInfo, request dto.Request, followChain bool) error {
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}

	// map model name
	modelMapping := c.GetString("model_mapping")
	if modelMapping != "" && modelMapping != "{}" {
		modelMap := make(map[string]string)
		err := rootcommon.Unmarshal([]byte(modelMapping), &modelMap)
		if err != nil {
			return fmt.Errorf("unmarshal_model_mapping_failed")
		}

		mappedModel := info.OriginModelName
		visited := map[string]struct{}{mappedModel: {}}
		for {
			nextModel, exists := modelMap[mappedModel]
			baseModel := hostreasoning.BaseModelName(mappedModel)
			if (!exists || nextModel == "") && baseModel != mappedModel {
				nextModel, exists = modelMap[baseModel]
			}
			if !exists || nextModel == "" || nextModel == mappedModel {
				break
			}
			if _, seen := visited[nextModel]; seen {
				return errors.New("model_mapping_contains_cycle")
			}
			mappedModel = nextModel
			if !followChain {
				break
			}
			visited[mappedModel] = struct{}{}
		}
		info.IsModelMapped = mappedModel != info.OriginModelName
		if info.IsModelMapped {
			info.UpstreamModelName = mappedModel
		}
	}

	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}
