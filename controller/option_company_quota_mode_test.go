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

func TestCompanyQuotaModeToggleDoesNotDependOnCycleState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "company-mode.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{}, &model.User{}, &model.Log{}, &model.QuotaCycle{}, &model.QuotaPlan{},
	))

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousMode := operation_setting.CompanyQuotaModeEnabled
	previousOptionMap := common.OptionMap
	previousRedisEnabled := common.RedisEnabled
	model.DB, model.LOG_DB = db, db
	operation_setting.CompanyQuotaModeEnabled = false
	common.RedisEnabled = false
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		operation_setting.CompanyQuotaModeEnabled = previousMode
		common.RedisEnabled = previousRedisEnabled
		common.OptionMap = previousOptionMap
	})

	for _, value := range []bool{true, false} {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Set("id", 1)
		context.Request = httptest.NewRequest(
			http.MethodPut,
			"/api/option/",
			strings.NewReader(`{"key":"CompanyQuotaModeEnabled","value":`+common.Interface2String(value)+`}`),
		)

		UpdateOption(context)

		assert.Equal(t, http.StatusOK, response.Code)
		var payload struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		assert.True(t, payload.Success, payload.Message)
		assert.Equal(t, value, operation_setting.CompanyQuotaModeEnabled)

		if value {
			require.NoError(t, db.Create(&model.QuotaCycle{Status: model.QuotaCycleStatusActive}).Error)
			require.NoError(t, db.Create(&model.QuotaPlan{Status: model.QuotaPlanStatusDraft}).Error)
		}
	}
}
