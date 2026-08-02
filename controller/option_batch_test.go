package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionsValidatesBatchBeforePersisting(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Log{}))

	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	originalSMTPServer := common.SMTPServer
	originalSMTPPort := common.SMTPPort
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
		common.SMTPServer = originalSMTPServer
		common.SMTPPort = originalSMTPPort
	})

	invalidBody := []byte(`{"options":[{"key":"SMTPServer","value":"smtp.example.com"},{"key":"theme.frontend","value":"classic"}]}`)
	invalidRecorder := httptest.NewRecorder()
	invalidContext, _ := gin.CreateTestContext(invalidRecorder)
	invalidContext.Request = httptest.NewRequest(http.MethodPut, "/api/option/batch", bytes.NewReader(invalidBody))

	UpdateOptions(invalidContext)

	var invalidResponse struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(invalidRecorder.Body.Bytes(), &invalidResponse))
	assert.False(t, invalidResponse.Success)
	var optionCount int64
	require.NoError(t, db.Model(&model.Option{}).Count(&optionCount).Error)
	assert.Zero(t, optionCount)
	assert.NotContains(t, common.OptionMap, "SMTPServer")

	validBody := []byte(`{"options":[{"key":"SMTPServer","value":"smtp.example.com"},{"key":"SMTPPort","value":587}]}`)
	validRecorder := httptest.NewRecorder()
	validContext, _ := gin.CreateTestContext(validRecorder)
	validContext.Set("id", 1)
	validContext.Request = httptest.NewRequest(http.MethodPut, "/api/option/batch", bytes.NewReader(validBody))

	UpdateOptions(validContext)

	var validResponse struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(validRecorder.Body.Bytes(), &validResponse))
	assert.True(t, validResponse.Success)
	var options []model.Option
	require.NoError(t, db.Order("key").Find(&options).Error)
	assert.Equal(t, []model.Option{
		{Key: "SMTPPort", Value: "587"},
		{Key: "SMTPServer", Value: "smtp.example.com"},
	}, options)
	assert.Equal(t, "smtp.example.com", common.OptionMap["SMTPServer"])
	assert.Equal(t, "587", common.OptionMap["SMTPPort"])
}
