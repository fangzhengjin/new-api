package service

import (
	"fmt"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestIOCopyBytesGracefullyAppliesUserModelMappingVisibility(t *testing.T) {
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
			relaycommon.SetUserResponseModelOverride(ctx, tt.override)

			IOCopyBytesGracefully(ctx, nil, []byte(`{"model":"upstream-model"}`))

			expectedBody := `{"model":"` + tt.expected + `"}`
			assert.JSONEq(t, expectedBody, recorder.Body.String())
			assert.Equal(t, len(expectedBody), recorder.Body.Len())
			assert.Equal(t, fmt.Sprintf("%d", recorder.Body.Len()), recorder.Header().Get("Content-Length"))
		})
	}
}
