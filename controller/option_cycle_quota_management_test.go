package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCycleQuotaManagementToggleRequiresClosedCyclesAndKeepsUserBalance(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "cycle-quota-management.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{}, &model.User{}, &model.Log{}, &model.QuotaCycle{}, &model.QuotaPlan{},
	))

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousMode := operation_setting.CycleQuotaManagementEnabled
	previousOptionMap := common.OptionMap
	previousRedisEnabled := common.RedisEnabled
	model.DB, model.LOG_DB = db, db
	operation_setting.CycleQuotaManagementEnabled = false
	common.RedisEnabled = false
	common.OptionMap = map[string]string{
		"SidebarModulesAdmin": `{"personal":{"enabled":true,"topup":true,"wallet_add_funds":true,"wallet_affiliate":true,"wallet_subscriptions":false},"admin":{"enabled":false}}`,
	}
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		operation_setting.CycleQuotaManagementEnabled = previousMode
		common.RedisEnabled = previousRedisEnabled
		common.OptionMap = previousOptionMap
	})

	user := model.User{Username: "cycle-quota-user", AffCode: "cycle-quota-user", Status: common.UserStatusEnabled, Quota: 12345}
	require.NoError(t, db.Create(&user).Error)
	updateMode := func(value bool) (bool, string) {
		t.Helper()
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Set("id", 1)
		context.Request = httptest.NewRequest(
			http.MethodPut,
			"/api/option/",
			strings.NewReader(`{"key":"CycleQuotaManagementEnabled","value":`+common.Interface2String(value)+`}`),
		)

		UpdateOption(context)

		assert.Equal(t, http.StatusOK, response.Code)
		var payload struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		return payload.Success, payload.Message
	}

	success, message := updateMode(true)
	assert.True(t, success, message)
	assert.True(t, operation_setting.CycleQuotaManagementEnabled)
	var storedUser model.User
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	assert.Equal(t, 12345, storedUser.Quota)

	var sidebarModules map[string]map[string]bool
	require.NoError(t, common.UnmarshalJsonStr(common.OptionMap["SidebarModulesAdmin"], &sidebarModules))
	assert.True(t, sidebarModules["personal"]["wallet_add_funds"])
	assert.True(t, sidebarModules["personal"]["wallet_affiliate"])
	assert.False(t, sidebarModules["personal"]["wallet_subscriptions"])
	assert.True(t, sidebarModules["personal"]["topup"])

	cycle := model.QuotaCycle{Status: model.QuotaCycleStatusActive}
	require.NoError(t, db.Create(&cycle).Error)
	success, message = updateMode(false)
	assert.False(t, success)
	assert.Contains(t, message, "请先关闭")
	assert.True(t, operation_setting.CycleQuotaManagementEnabled)

	require.NoError(t, db.Model(&model.QuotaCycle{}).Where("id = ?", cycle.Id).Updates(map[string]any{
		"status": model.QuotaCycleStatusClosed, "active_key": nil,
	}).Error)
	success, message = updateMode(false)
	assert.True(t, success, message)
	assert.False(t, operation_setting.CycleQuotaManagementEnabled)

	require.NoError(t, db.First(&storedUser, user.Id).Error)
	assert.Equal(t, 12345, storedUser.Quota)
}
