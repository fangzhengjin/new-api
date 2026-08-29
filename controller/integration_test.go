package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupIntegrationControllerTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AuthFlow{}, &model.Token{}, &model.User{}, &model.UserSession{}))

	previousDB := model.DB
	previousRedisEnabled := common.RedisEnabled
	previousServerAddress := system_setting.ServerAddress
	previousChats := setting.GetChats()
	model.DB = db
	common.RedisEnabled = false
	system_setting.ServerAddress = "https://api.example.test"
	require.NoError(t, setting.UpdateChatsByJsonString(`[{"name":"Canvas","url":"https://canvas.example.test/image#canvas_code={authCode}","enabled":true,"open_mode":"new_tab"}]`))
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
		system_setting.ServerAddress = previousServerAddress
		encoded, marshalErr := common.Marshal(previousChats)
		require.NoError(t, marshalErr)
		require.NoError(t, setting.UpdateChatsByJsonString(string(encoded)))
	})

	require.NoError(t, db.Create(&model.User{
		Id: 42, Username: "canvas-user", Password: "password", Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, Group: "default", AuthVersion: 1,
	}).Error)
	require.NoError(t, db.Create(&model.UserSession{
		SID: "session-42", UserID: 42, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: "refresh-hash", LoginMethod: "test",
		CreatedAt: time.Now().Unix(), LastActiveAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id: 7, UserId: 42, Name: "Canvas", Key: "canvas-token", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, RemainQuota: 100,
	}).Error)
}

func integrationContext(method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	return context, recorder
}

func TestIntegrationLaunchAndExchangeUseGenericSessionBoundSingleUseCode(t *testing.T) {
	setupIntegrationControllerTest(t)
	launchContext, launchRecorder := integrationContext(
		http.MethodPost,
		"/api/integrations/launch",
		`{"preset_name":"Canvas","token_id":7}`,
	)
	launchContext.Set("id", 42)
	launchContext.Set("session_id", "session-42")
	launchContext.Set("auth_version", int64(1))
	launchContext.Set("session_version", int64(1))

	LaunchIntegration(launchContext)

	require.Equal(t, http.StatusOK, launchRecorder.Code)
	var launchResponse struct {
		Success bool `json:"success"`
		Data    struct {
			LaunchURL string `json:"launch_url"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(launchRecorder.Body.Bytes(), &launchResponse))
	require.True(t, launchResponse.Success)
	assert.NotContains(t, launchResponse.Data.LaunchURL, "canvas-token")
	parsedLaunchURL, err := url.Parse(launchResponse.Data.LaunchURL)
	require.NoError(t, err)
	code := strings.TrimPrefix(parsedLaunchURL.Fragment, "canvas_code=")
	require.NotEmpty(t, code)

	require.NoError(t, setting.UpdateChatsByJsonString(`[{"name":"Canvas","url":"https://canvas.example.test/image#canvas_code={authCode}","enabled":false,"open_mode":"new_tab"}]`))

	exchangeContext, exchangeRecorder := integrationContext(http.MethodPost, "/api/integrations/exchange", `{"code":"`+code+`"}`)
	exchangeContext.Request.Header.Set("Origin", "https://another.example.test")
	ExchangeIntegrationCode(exchangeContext)
	require.Equal(t, http.StatusOK, exchangeRecorder.Code)
	var exchangeResponse struct {
		Success bool `json:"success"`
		Data    struct {
			BaseURL   string `json:"base_url"`
			APIKey    string `json:"api_key"`
			APIFormat string `json:"api_format"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(exchangeRecorder.Body.Bytes(), &exchangeResponse))
	require.True(t, exchangeResponse.Success)
	assert.Equal(t, "https://api.example.test", exchangeResponse.Data.BaseURL)
	assert.Equal(t, "sk-canvas-token", exchangeResponse.Data.APIKey)
	assert.Equal(t, "openai", exchangeResponse.Data.APIFormat)

	replayContext, replayRecorder := integrationContext(http.MethodPost, "/api/integrations/exchange", `{"code":"`+code+`"}`)
	ExchangeIntegrationCode(replayContext)
	assert.Equal(t, http.StatusUnauthorized, replayRecorder.Code)
}

func TestIntegrationLaunchRejectsAnotherUsersToken(t *testing.T) {
	setupIntegrationControllerTest(t)
	require.NoError(t, model.DB.Create(&model.Token{
		Id: 8, UserId: 99, Name: "Other", Key: "other-token", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, RemainQuota: 100,
	}).Error)
	context, recorder := integrationContext(
		http.MethodPost,
		"/api/integrations/launch",
		`{"preset_name":"Canvas","token_id":8}`,
	)
	context.Set("id", 42)
	context.Set("session_id", "session-42")
	context.Set("auth_version", int64(1))
	context.Set("session_version", int64(1))

	LaunchIntegration(context)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestIntegrationLaunchRejectsWhenDisabled(t *testing.T) {
	setupIntegrationControllerTest(t)
	require.NoError(t, setting.UpdateChatsByJsonString(`[{"name":"Canvas","url":"https://canvas.example.test/image#canvas_code={authCode}","enabled":false,"open_mode":"new_tab"}]`))
	context, recorder := integrationContext(
		http.MethodPost,
		"/api/integrations/launch",
		`{"preset_name":"Canvas","token_id":7}`,
	)
	context.Set("id", 42)
	context.Set("session_id", "session-42")
	context.Set("auth_version", int64(1))
	context.Set("session_version", int64(1))

	LaunchIntegration(context)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestClassifyChatPresetModelsUsesOnlySupportedEndpointTypes(t *testing.T) {
	models := []dto.OpenAIModels{
		{Id: "multi", SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI, constant.EndpointTypeImageGeneration}},
		{Id: "video", SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIVideo}},
		{Id: "embedding", SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeEmbeddings}},
		{Id: "unknown"},
		{Id: "anthropic", SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeAnthropic}},
		{Id: "multi", SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeGemini}},
	}

	variables := classifyChatPresetModels(models)

	assert.Equal(t, []string{"anthropic", "multi"}, variables.TextModels)
	assert.Equal(t, []string{"multi"}, variables.ImageModels)
	assert.Equal(t, []string{"video"}, variables.VideoModels)
}

func TestIntegrationExchangeRejectsMalformedCodeBeforeLookup(t *testing.T) {
	setupIntegrationControllerTest(t)
	context, recorder := integrationContext(http.MethodPost, "/api/integrations/exchange", `{"code":"not-a-valid-code"}`)

	ExchangeIntegrationCode(context)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}
