package controller

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

	tooMany := make([]OptionUpdateRequest, 101)
	for index := range tooMany {
		tooMany[index] = OptionUpdateRequest{Key: fmt.Sprintf("setting-%d", index), Value: index}
	}
	tooManyBody, err := common.Marshal(map[string]interface{}{"options": tooMany})
	require.NoError(t, err)
	tooManyRecorder := httptest.NewRecorder()
	tooManyContext, _ := gin.CreateTestContext(tooManyRecorder)
	tooManyContext.Request = httptest.NewRequest(http.MethodPut, "/api/option/batch", bytes.NewReader(tooManyBody))
	UpdateOptions(tooManyContext)
	var tooManyResponse struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(tooManyRecorder.Body.Bytes(), &tooManyResponse))
	assert.False(t, tooManyResponse.Success)
	require.NoError(t, db.Model(&model.Option{}).Count(&optionCount).Error)
	assert.Zero(t, optionCount)

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

func TestUpdateOptionRejectsBlankKeyBeforePersisting(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/option", bytes.NewReader([]byte(`{"key":"  ","value":"value"}`)))

	UpdateOption(context)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "设置项名称不能为空", response.Message)
	var count int64
	require.NoError(t, db.Model(&model.Option{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestUpdateOptionDoesNotMutateCacheWhenPersistenceFails(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	const callbackName = "test:fail_option_create"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "options" {
			tx.AddError(errors.New("forced option write failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/option", bytes.NewReader([]byte(`{"key":"SMTPServer","value":"smtp.example.com"}`)))

	UpdateOption(context)

	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.NotContains(t, common.OptionMap, "SMTPServer")
	var count int64
	require.NoError(t, db.Model(&model.Option{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestLogConsumeEnabledRejectsNonBooleanValue(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	previousOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = previousOptionMap })

	body, err := common.Marshal(map[string]interface{}{
		"options": []OptionUpdateRequest{{Key: "LogConsumeEnabled", Value: "invalid"}},
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/option/batch", bytes.NewReader(body))
	UpdateOptions(context)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "必须为 true 或 false")
}
