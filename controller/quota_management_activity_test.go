package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetQuotaActivitiesSkipsLegacyEmptyLogWithoutWarning(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "quota-activities.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})

	require.NoError(t, db.Create(&model.Log{
		Id: 1, Type: model.LogTypeManage, CreatedAt: 1, Other: "",
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		Id: 2, Type: model.LogTypeManage, CreatedAt: 2,
		Other: `{"op":{"action":"quota.cycle.create","params":{"cycle_id":8}}}`,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		Id: 3, Type: model.LogTypeManage, CreatedAt: 3,
		Other: `{"op":{"action":"quota.plan.execute","params":{"plan_id":9}}}`,
	}).Error)

	var errorOutput bytes.Buffer
	common.LogWriterMu.Lock()
	previousErrorWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &errorOutput
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = previousErrorWriter
		common.LogWriterMu.Unlock()
	})

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/quota/activities", nil)
	GetQuotaActivities(context)

	assert.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    []struct {
			Action string                 `json:"action"`
			Params map[string]interface{} `json:"params"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.True(t, payload.Success)
	require.Len(t, payload.Data, 2)
	assert.Equal(t, "quota.plan.execute", payload.Data[0].Action)
	assert.NotNil(t, payload.Data[0].Params)
	assert.Equal(t, float64(9), payload.Data[0].Params["plan_id"])
	assert.Equal(t, "quota.cycle.create", payload.Data[1].Action)
	assert.NotContains(t, errorOutput.String(), "failed to parse quota activity log")
}

func TestGetQuotaActivitiesSeparatesOperatorAndTargetIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "quota-activity-identities.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})

	operator := model.User{Id: 8, Username: "quota-admin", DisplayName: "王小明", AffCode: "activity-operator"}
	target := model.User{Id: 11, Username: "zhangsan", DisplayName: "张三(北京)", AffCode: "activity-target"}
	require.NoError(t, db.Create(&operator).Error)
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&model.Log{
		Id: 1, UserId: operator.Id, Username: operator.Username,
		Type: model.LogTypeManage, CreatedAt: 1,
		Other: `{"op":{"action":"user.quota_adjustment_plan","params":{"target_user_id":11,"plan_id":73,"adjustment_quota":"250000"}}}`,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		Id: 2, UserId: operator.Id, Username: operator.Username,
		Type: model.LogTypeManage, CreatedAt: 2,
		Other: `{"op":{"action":"user.quota_adjustment_plan"}}`,
	}).Error)

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/quota/activities", nil)
	GetQuotaActivities(context)

	assert.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    []struct {
			Params              map[string]interface{} `json:"params"`
			OperatorID          int                    `json:"operator_id"`
			Operator            string                 `json:"operator"`
			OperatorDisplayName string                 `json:"operator_display_name"`
			Target              *struct {
				ID          int    `json:"id"`
				Username    string `json:"username"`
				DisplayName string `json:"display_name"`
			} `json:"target"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.Len(t, payload.Data, 2)
	assert.NotNil(t, payload.Data[0].Params)
	assert.Empty(t, payload.Data[0].Params)
	assert.Nil(t, payload.Data[0].Target)
	assert.Equal(t, operator.Id, payload.Data[1].OperatorID)
	assert.Equal(t, operator.Username, payload.Data[1].Operator)
	assert.Equal(t, operator.DisplayName, payload.Data[1].OperatorDisplayName)
	assert.Equal(t, float64(target.Id), payload.Data[1].Params["target_user_id"])
	assert.Equal(t, float64(73), payload.Data[1].Params["plan_id"])
	assert.Equal(t, "250000", payload.Data[1].Params["adjustment_quota"])
	require.NotNil(t, payload.Data[1].Target)
	assert.Equal(t, target.Id, payload.Data[1].Target.ID)
	assert.Equal(t, target.Username, payload.Data[1].Target.Username)
	assert.Equal(t, target.DisplayName, payload.Data[1].Target.DisplayName)
}
