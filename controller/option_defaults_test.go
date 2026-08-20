package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetOptionsIncludesRestorableBuiltInDefaults(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/option/", nil)
	GetOptions(context)
	require.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Success bool            `json:"success"`
		Data    []*model.Option `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	values := make(map[string]string, len(response.Data))
	for _, option := range response.Data {
		values[option.Key] = option.Value
	}

	require.JSONEq(t, operation_setting.DefaultRequestHeaderRulesJSON(), values[operation_setting.RequestHeaderRulesDefaultOptionKey])
	require.JSONEq(t, operation_setting.SystemRequestHeaderRulesJSON(), values[operation_setting.RequestHeaderSystemRulesOptionKey])
	require.JSONEq(t, mustMarshalOptionDefault(t, model_setting.GetDefaultCodexSettings()), values[model_setting.CodexSettingsDefaultOptionKey])
	require.JSONEq(t, mustMarshalOptionDefault(t, model_setting.GetDefaultClaudeSettings()), values[model_setting.ClaudeSettingsDefaultOptionKey])
	require.JSONEq(t, mustMarshalOptionDefault(t, setting.GetDefaultChats()), values[setting.ChatsDefaultOptionKey])
	require.Equal(t, "3", values[setting.ChatMenuCollapseThresholdDefaultOptionKey])
}

func mustMarshalOptionDefault(t *testing.T, value any) string {
	t.Helper()
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}
