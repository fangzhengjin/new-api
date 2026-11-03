package model

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
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

// A database that keeps failing must not let the retry backlog grow without
// bound: the oldest hour buckets are dropped so memory stays bounded.
func TestSaveQuotaDataCacheDropsOldestBacklogBeyondBound(t *testing.T) {
	resetQuotaDataCache(t)
	previousBound := maxCachedQuotaDataEntries
	maxCachedQuotaDataEntries = 10
	t.Cleanup(func() { maxCachedQuotaDataEntries = previousBound })

	hour := int64(1000) * 3600
	CacheQuotaDataLock.Lock()
	for i := range 30 {
		quotaData := &QuotaData{
			UserID:    i + 1,
			Username:  "alice",
			ModelName: "claude-test",
			CreatedAt: hour + int64(i)*3600,
			Count:     1,
		}
		CacheQuotaData[fmt.Sprintf("k%d", i)] = quotaData
	}
	CacheQuotaDataLock.Unlock()

	failedDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, failedDB.AutoMigrate(&QuotaData{}))
	originalDB := DB
	DB = failedDB
	require.NoError(t, DB.Exec("DROP TABLE quota_data").Error)
	t.Cleanup(func() { DB = originalDB })

	SaveQuotaDataCache()

	CacheQuotaDataLock.Lock()
	remaining := len(CacheQuotaData)
	newest := int64(0)
	for _, quotaData := range CacheQuotaData {
		if quotaData.CreatedAt > newest {
			newest = quotaData.CreatedAt
		}
	}
	oldestKept := int64(1<<62 - 1)
	for _, quotaData := range CacheQuotaData {
		if quotaData.CreatedAt < oldestKept {
			oldestKept = quotaData.CreatedAt
		}
	}
	CacheQuotaDataLock.Unlock()

	assert.LessOrEqual(t, remaining, 10)
	assert.Equal(t, hour+29*3600, newest, "the newest buckets are the ones kept")
	assert.Greater(t, oldestKept, hour, "the oldest buckets were dropped first")
}
