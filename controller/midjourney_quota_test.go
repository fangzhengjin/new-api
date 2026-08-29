package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMidjourneyFailureRefundsWhenQuotaObservationIsUnavailable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "midjourney.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Channel{}, &model.Midjourney{}, &model.Task{},
		&model.QuotaCycle{}, &model.QuotaPlan{}, &model.Log{},
	))

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousCycleQuotaManagement := operation_setting.CycleQuotaManagementEnabled
	previousMemoryCache, previousRedisEnabled := common.MemoryCacheEnabled, common.RedisEnabled
	previousRDB := common.RDB
	model.DB, model.LOG_DB = db, db
	operation_setting.CycleQuotaManagementEnabled = true
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	common.RDB = nil
	service.InitHttpClient()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		operation_setting.CycleQuotaManagementEnabled = previousCycleQuotaManagement
		common.MemoryCacheEnabled, common.RedisEnabled = previousMemoryCache, previousRedisEnabled
		common.RDB = previousRDB
	})

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, writeErr := response.Write([]byte(`[{"id":"mj-lease","status":"FAILURE","progress":"100%","failReason":"upstream failed"}]`))
		require.NoError(t, writeErr)
	}))
	t.Cleanup(server.Close)

	user := model.User{Id: 1, Username: "managed", Quota: 100}
	require.NoError(t, db.Create(&user).Error)
	channel := model.Channel{Id: 1, Type: constant.ChannelTypeMidjourney, Key: "secret", BaseURL: &server.URL}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.QuotaCycle{
		CycleStartAt: now.Add(-time.Hour).Unix(), CycleEndAt: now.Add(time.Hour).Unix(),
		BudgetQuota: 1000, Status: model.QuotaCycleStatusSettling,
	}).Error)
	task := model.Midjourney{
		UserId: 1, MjId: "mj-lease", ChannelId: 1, SubmitTime: now.UnixMilli(),
		Status: "IN_PROGRESS", Progress: "0%", Quota: 40,
	}
	require.NoError(t, db.Create(&task).Error)

	runMidjourneyTaskUpdateOnce(t.Context(), nil)

	var persisted model.Midjourney
	require.NoError(t, db.First(&persisted, task.Id).Error)
	assert.Equal(t, "FAILURE", persisted.Status)
	assert.Equal(t, "100%", persisted.Progress)
	assert.Zero(t, persisted.Quota)
	var persistedUser model.User
	require.NoError(t, db.First(&persistedUser, user.Id).Error)
	assert.Equal(t, 140, persistedUser.Quota)
	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Count(&logCount).Error)
	assert.Equal(t, int64(2), logCount)
	require.Eventually(t, func() bool {
		var cycle model.QuotaCycle
		return db.First(&cycle).Error == nil && cycle.Status == model.QuotaCycleStatusClosed
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		var auditCount int64
		return db.Model(&model.Log{}).Where("type = ?", model.LogTypeManage).Count(&auditCount).Error == nil && auditCount == 1
	}, time.Second, 10*time.Millisecond)
}

func TestMidjourneySecondAndMillisecondSubmitTimesDoNotTimeout(t *testing.T) {
	now := time.Now().Add(-100 * time.Second)
	for name, submitTime := range map[string]int64{
		"seconds":      now.Unix(),
		"milliseconds": now.UnixMilli(),
	} {
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "midjourney.db")), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Midjourney{}))

			previousDB := model.DB
			previousCycleQuotaManagement := operation_setting.CycleQuotaManagementEnabled
			previousMemoryCache := common.MemoryCacheEnabled
			model.DB = db
			operation_setting.CycleQuotaManagementEnabled = false
			common.MemoryCacheEnabled = false
			service.InitHttpClient()
			t.Cleanup(func() {
				model.DB = previousDB
				operation_setting.CycleQuotaManagementEnabled = previousCycleQuotaManagement
				common.MemoryCacheEnabled = previousMemoryCache
			})

			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				_, writeErr := fmt.Fprintf(response, `[{"id":"mj-time","status":"IN_PROGRESS","progress":"0%%","submitTime":%d}]`, submitTime)
				require.NoError(t, writeErr)
			}))
			t.Cleanup(server.Close)

			channel := model.Channel{Id: 1, Type: constant.ChannelTypeMidjourney, Key: "secret", BaseURL: &server.URL}
			require.NoError(t, db.Create(&channel).Error)
			task := model.Midjourney{
				Code: 1, MjId: "mj-time", ChannelId: 1, SubmitTime: submitTime,
				Status: "IN_PROGRESS", Progress: "0%",
			}
			require.NoError(t, db.Create(&task).Error)

			runMidjourneyTaskUpdateOnce(t.Context(), nil)

			var persisted model.Midjourney
			require.NoError(t, db.First(&persisted, task.Id).Error)
			assert.Equal(t, "IN_PROGRESS", persisted.Status)
			assert.Empty(t, persisted.FailReason)
		})
	}
}
