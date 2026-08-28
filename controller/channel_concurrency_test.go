package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGetChannelConcurrencySkipsAuditOnlyAfterValidation(t *testing.T) {
	redisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = redisEnabled
	})

	tests := []struct {
		name        string
		body        string
		auditLogged bool
	}{
		{name: "valid read request", body: `{"ids":[1]}`, auditLogged: true},
		{name: "invalid request", body: `{"ids":[0]}`, auditLogged: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/channel/concurrency", strings.NewReader(test.body))
			context.Request.Header.Set("Content-Type", "application/json")

			GetChannelConcurrency(context)

			assert.Equal(t, test.auditLogged, common.GetContextKeyBool(context, constant.ContextKeyAuditLogged))
		})
	}
}
