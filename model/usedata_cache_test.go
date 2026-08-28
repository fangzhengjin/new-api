package model

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func resetQuotaDataCache(t *testing.T) {
	t.Helper()
	CacheQuotaDataLock.Lock()
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()
	t.Cleanup(func() {
		CacheQuotaDataLock.Lock()
		CacheQuotaData = make(map[string]*QuotaData)
		CacheQuotaDataLock.Unlock()
	})
}

func TestSaveQuotaDataCacheRetainsFailedRowsForRetry(t *testing.T) {
	truncateTables(t)
	resetQuotaDataCache(t)
	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "claude-test",
		CreatedAt: time.Now().Unix(),
		TokenUsed: 127,
	})

	failedDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, failedDB.AutoMigrate(&QuotaData{}))
	require.NoError(t, failedDB.Exec(`
		CREATE TRIGGER reject_quota_data_insert
		BEFORE INSERT ON quota_data
		BEGIN
			SELECT RAISE(ABORT, 'forced write failure');
		END
	`).Error)
	workingDB := DB
	t.Cleanup(func() { DB = workingDB })
	DB = failedDB
	SaveQuotaDataCache()
	DB = workingDB

	require.Len(t, CacheQuotaData, 1)

	SaveQuotaDataCache()
	require.Empty(t, CacheQuotaData)
	var saved QuotaData
	require.NoError(t, DB.First(&saved).Error)
	require.Equal(t, 127, saved.TokenUsed)
	require.Equal(t, 1, saved.Count)
}

func TestDataExportDoesNotDependOnConsumeLogStorage(t *testing.T) {
	truncateTables(t)
	resetQuotaDataCache(t)
	previousLogConsumeEnabled := common.LogConsumeEnabled
	previousDataExportEnabled := common.DataExportEnabled
	common.LogConsumeEnabled = false
	common.DataExportEnabled = true
	t.Cleanup(func() {
		common.LogConsumeEnabled = previousLogConsumeEnabled
		common.DataExportEnabled = previousDataExportEnabled
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("username", "alice")
	RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ModelName:          "claude-test",
		PromptTokens:       70,
		CompletionTokens:   7,
		QuotaDataTokenUsed: 127,
	})
	RecordTaskBillingLog(RecordTaskBillingLogParams{
		UserId:    1,
		LogType:   LogTypeConsume,
		ModelName: "task-test",
		Quota:     10,
	})

	require.Len(t, CacheQuotaData, 2)
	for _, quotaData := range CacheQuotaData {
		if quotaData.ModelName == "claude-test" {
			require.Equal(t, 127, quotaData.TokenUsed)
		}
	}
	var logCount int64
	require.NoError(t, DB.Model(&Log{}).Count(&logCount).Error)
	require.Zero(t, logCount)
}
