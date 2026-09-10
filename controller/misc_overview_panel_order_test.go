package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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

func TestUpdateOptionValidatesOverviewPanelOrderBeforePersisting(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Log{}))
	settings := console_setting.GetConsoleSetting()
	originalOrder := settings.OverviewPanelOrder
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
		settings.OverviewPanelOrder = originalOrder
	})

	update := func(key, value string) bool {
		body, err := common.Marshal(OptionUpdateRequest{Key: key, Value: value})
		require.NoError(t, err)
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest(http.MethodPut, "/api/option/", bytes.NewReader(body))
		UpdateOption(context)
		var payload struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		return payload.Success
	}

	assert.False(t, update("console_setting.overview_panel_order", `[{"id":"api-info","span":4}]`))
	var count int64
	require.NoError(t, db.Model(&model.Option{}).Count(&count).Error)
	assert.Zero(t, count)
	assert.True(t, update("console_setting.overview_panel_order", `["faq","api-info"]`))
	assert.True(t, update("console_setting.overview_panel_order", `[{"id":"announcements","span":2},{"id":"faq","span":1}]`))
	assert.False(t, update("channel_affinity_setting.renew_ttl_on_success", "enabled"))
	require.NoError(t, db.Model(&model.Option{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
