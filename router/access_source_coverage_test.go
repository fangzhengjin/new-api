package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccessSourceLimitCoversConsumptionRoutesOnly(t *testing.T) {
	require.NoError(t, appI18n.Init())
	setupRelayRouterTestDB(t)
	dashboardPAT := "coverage-dashboard-pat"
	user := model.User{
		Id: 94001, Username: "access-source-coverage", Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, Group: "default", Quota: 100, AccessToken: &dashboardPAT,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId: user.Id, Key: "coveragekey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true,
	}).Error)

	previousEnabled := setting.AccessSourceLimitEnabled
	previousWindow := setting.AccessSourceAssociationWindowHours
	previousMaxIPs := setting.AccessSourceMaxIPsPerUser
	previousCooldown := setting.AccessSourceSwitchCooldownMinutes
	previousMaxUsers := setting.AccessSourceMaxUsersPerIP
	setting.AccessSourceLimitEnabled = true
	setting.AccessSourceAssociationWindowHours = 24
	setting.AccessSourceMaxIPsPerUser = 1
	setting.AccessSourceSwitchCooldownMinutes = 0
	setting.AccessSourceMaxUsersPerIP = 0
	t.Cleanup(func() {
		setting.AccessSourceLimitEnabled = previousEnabled
		setting.AccessSourceAssociationWindowHours = previousWindow
		setting.AccessSourceMaxIPsPerUser = previousMaxIPs
		setting.AccessSourceSwitchCooldownMinutes = previousCooldown
		setting.AccessSourceMaxUsersPerIP = previousMaxUsers
	})

	decision, err := service.CheckAccessSource(context.Background(), user.Id, "192.0.2.10", setting.AccessSourceLimits{
		Enabled: true, AssociationWindowHours: 24, MaxIPsPerUser: 1,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)

	engine := gin.New()
	SetRelayRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	SetVideoRouter(engine)
	SetTaskRouter(engine)

	consumptionRoutes := []struct {
		name       string
		path       string
		credential string
	}{
		{name: "playground", path: "/pg/chat/completions", credential: "coverage-dashboard-pat"},
		{name: "midjourney submit", path: "/mj/submit/imagine", credential: "coveragekey"},
		{name: "generic task submit", path: "/v1/tasks/demo", credential: "coveragekey"},
		{name: "legacy video submit", path: "/v1/video/generations", credential: "coveragekey"},
		{name: "video remix", path: "/v1/videos/task-1/remix", credential: "coveragekey"},
		{name: "video protocol submit", path: "/v1/videos", credential: "coveragekey"},
	}
	for _, test := range consumptionRoutes {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{}`))
			request.RemoteAddr = "198.51.100.10:1234"
			request.Header.Set("Authorization", "Bearer "+test.credential)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			engine.ServeHTTP(response, request)

			assert.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), "access_source_account_ip_limit")
		})
	}

	nonConsumptionRoutes := []struct {
		name   string
		method string
		path   string
	}{
		{name: "model list", method: http.MethodGet, path: "/v1/models"},
		{name: "file upload placeholder", method: http.MethodPost, path: "/v1/files"},
		{name: "token count", method: http.MethodPost, path: "/v1/messages/count_tokens"},
		{name: "midjourney task fetch", method: http.MethodGet, path: "/mj/task/missing/fetch"},
		{name: "midjourney upload", method: http.MethodPost, path: "/mj/submit/upload-discord-images"},
	}
	for _, test := range nonConsumptionRoutes {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(`{}`))
			request.RemoteAddr = "198.51.100.10:1234"
			request.Header.Set("Authorization", "Bearer coveragekey")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			engine.ServeHTTP(response, request)

			assert.NotContains(t, response.Body.String(), "access_source_account_ip_limit")
		})
	}
}
