package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateUserLimitsRejectsOutOfRangeWithoutChangingSettings(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{
		Username: "limited-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Setting: `{"language":"zh"}`,
	}
	require.NoError(t, db.Create(&user).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(user.Id)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/user/limits", strings.NewReader(`{"access_source_max_ips":1001}`))
	c.Set("id", 9999)
	c.Set("role", common.RoleAdminUser)

	UpdateUserLimits(c)

	assert.Contains(t, recorder.Body.String(), `"success":false`)

	concurrencyRecorder := httptest.NewRecorder()
	concurrencyContext, _ := gin.CreateTestContext(concurrencyRecorder)
	concurrencyContext.Params = gin.Params{{Key: "id", Value: strconv.Itoa(user.Id)}}
	concurrencyContext.Request = httptest.NewRequest(http.MethodPut, "/api/user/limits", strings.NewReader(`{"model_request_concurrency_limit":10001}`))
	concurrencyContext.Set("id", 9999)
	concurrencyContext.Set("role", common.RoleAdminUser)
	UpdateUserLimits(concurrencyContext)
	assert.Contains(t, concurrencyRecorder.Body.String(), `"success":false`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, `{"language":"zh"}`, updated.Setting)
}

func TestGetUserLimitsRejectsSameRole(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{Username: "peer-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(user.Id)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/limits", nil)
	c.Set("id", 9999)
	c.Set("role", common.RoleAdminUser)

	GetUserLimits(c)

	assert.Contains(t, recorder.Body.String(), `"success":false`)
}

func TestGetUserLimitsReturnsConcurrencyOverrideAndGlobalIPLimit(t *testing.T) {
	db := setupManageUserTestDB(t)
	override := 4
	settings, err := common.Marshal(dto.UserSetting{ModelRequestConcurrencyLimit: &override})
	require.NoError(t, err)
	user := model.User{
		Username: "concurrency-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Setting: string(settings),
	}
	require.NoError(t, db.Create(&user).Error)
	previousEnabled := setting.ModelRequestConcurrencyLimitEnabled
	previousAccountLimit := setting.ModelRequestConcurrencyLimit
	previousIPLimit := setting.ModelRequestIPConcurrencyLimit
	setting.ModelRequestConcurrencyLimitEnabled = true
	setting.ModelRequestConcurrencyLimit = 2
	setting.ModelRequestIPConcurrencyLimit = 6
	t.Cleanup(func() {
		setting.ModelRequestConcurrencyLimitEnabled = previousEnabled
		setting.ModelRequestConcurrencyLimit = previousAccountLimit
		setting.ModelRequestIPConcurrencyLimit = previousIPLimit
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(user.Id)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/limits", nil)
	c.Set("id", 9999)
	c.Set("role", common.RoleAdminUser)
	GetUserLimits(c)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Effective struct {
				AccountLimit  int    `json:"model_request_concurrency_limit"`
				AccountSource string `json:"model_request_concurrency_limit_source"`
			} `json:"effective"`
			Global struct {
				Enabled bool `json:"model_request_concurrency_limit_enabled"`
				IPLimit int  `json:"model_request_ip_concurrency_limit"`
			} `json:"global"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, 4, response.Data.Effective.AccountLimit)
	assert.Equal(t, setting.RateLimitSourceUser, response.Data.Effective.AccountSource)
	assert.True(t, response.Data.Global.Enabled)
	assert.Equal(t, 6, response.Data.Global.IPLimit)
}
