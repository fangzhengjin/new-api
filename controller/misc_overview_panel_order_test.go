package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStatusReturnsOverviewPanelOrder(t *testing.T) {
	settings := console_setting.GetConsoleSetting()
	originalOrder := settings.OverviewPanelOrder
	originalOptionMap := common.OptionMap
	t.Cleanup(func() {
		settings.OverviewPanelOrder = originalOrder
		common.OptionMap = originalOptionMap
	})
	settings.OverviewPanelOrder = `["faq","api-info","announcements","uptime-kuma"]`
	common.OptionMap = map[string]string{}

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

	GetStatus(context)

	var payload struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	assert.Equal(t, settings.OverviewPanelOrder, payload.Data["overview_panel_order"])
}
