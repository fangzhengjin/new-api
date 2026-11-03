package helper

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelMappedHelperAppliesOnlyOneMapping(t *testing.T) {
	mapping := `{
		"deepseek-v4-pro": "deepseek-v4-flash",
		"deepseek-v4-pro-aliyun": "deepseek-v4-pro"
	}`
	tests := []struct {
		model    string
		upstream string
	}{
		{model: "deepseek-v4-pro", upstream: "deepseek-v4-flash"},
		{model: "deepseek-v4-pro-aliyun", upstream: "deepseek-v4-pro"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("model_mapping", mapping)
			request := &dto.GeneralOpenAIRequest{Model: tt.model}
			info := &relaycommon.RelayInfo{OriginModelName: tt.model}

			err := ModelMappedHelper(c, info, request)
			require.NoError(t, err)
			assert.True(t, info.IsModelMapped)
			assert.Equal(t, tt.upstream, info.UpstreamModelName)
			assert.Equal(t, tt.upstream, request.Model)
		})
	}
}
