package controller

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	quotaService "github.com/QuantumNous/new-api/service/quota"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestQuotaCycleBudgetUpdatePreservesValidOpeningAllocation(t *testing.T) {
	db, _ := setupQuotaActivityDatabase(t, false)
	require.NoError(t, db.AutoMigrate(&model.QuotaCycle{}, &model.QuotaPlan{}))
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	now := time.Now().Unix()
	opening, allocated := int64(800), int64(200)
	cycle := model.QuotaCycle{
		CycleStartAt: now - 3600, CycleEndAt: now + 3600,
		BudgetQuota: 1000, InitialGrantQuota: 100,
		OpeningAllocatedQuota: &opening, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.Error(t, quotaService.UpdateCycleSettings(cycle.Id, 500, nil, nil, nil, "root"))
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	assert.Equal(t, int64(1000), cycle.BudgetQuota)
	require.NoError(t, quotaService.CycleAllocationError(&cycle))
	require.NoError(t, quotaService.UpdateCycleSettings(cycle.Id, 800, nil, nil, nil, "root"))
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	assert.Equal(t, int64(800), cycle.BudgetQuota)
	require.NoError(t, quotaService.CycleAllocationError(&cycle))
}

func setupQuotaActivityDatabase(t *testing.T, separate bool) (*gorm.DB, *gorm.DB) {
	t.Helper()
	kind := os.Getenv("TEST_SECURITY_DIALECT")
	if kind == "" {
		kind = "sqlite"
	}
	dsn := os.Getenv("TEST_" + strings.ToUpper(kind) + "_DSN")
	db, _ := newAuditTestDatabase(t, kind, dsn)
	logDB := db
	if separate {
		logDB, _ = newAuditTestDatabase(t, kind, dsn)
	}
	require.NoError(t, db.AutoMigrate(&model.User{}))
	require.NoError(t, logDB.AutoMigrate(&model.Log{}, &model.AuditLog{}))
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis := common.RedisEnabled
	model.DB, model.LOG_DB = db, logDB
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseType(kind), common.DatabaseType(kind))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled = previousRedis
	})
	return db, logDB
}
