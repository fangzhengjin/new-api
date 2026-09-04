package helper

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringDataAppliesUserModelMappingVisibility(t *testing.T) {
	tests := []struct {
		name     string
		override string
		expected string
	}{
		{name: "visible by default", expected: "upstream-model"},
		{name: "hidden mapping uses requested model", override: "requested-model", expected: "requested-model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("GET", "/v1/chat/completions", nil)
			relaycommon.SetUserResponseModelOverride(ctx, tt.override)

			err := StringData(ctx, `{"model":"upstream-model"}`)

			require.NoError(t, err)
			assert.Contains(t, recorder.Body.String(), `"model":"`+tt.expected+`"`)
		})
	}
}
