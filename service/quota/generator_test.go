package quota

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGenerateInitializationPlanUsesManagedBalancesAndAllSpend(t *testing.T) {
	db := setupQuotaTestDB(t)

	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 7*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "200"),
		ConcentrationMultiplier: 15_000,
		Status:                  model.QuotaCycleStatusActive, CreatedBy: "root",
	}
	require.NoError(t, db.Create(&cycle).Error)
	users := []model.User{
		{Username: "alice", DisplayName: "Alice", Email: "alice@example.com", AffCode: "alice-code", Status: common.UserStatusEnabled, Quota: -int(mustQuota(t, "10")), CreatedAt: now - daySeconds},
		{Username: "bob", DisplayName: "Bob", Email: "bob@example.com", AffCode: "bob-code", Status: common.UserStatusDisabled, Quota: int(mustQuota(t, "10")), CreatedAt: now - daySeconds},
		{Username: "demo", AffCode: "demo-code", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "300")), QuotaWhitelist: true, CreatedAt: now - daySeconds},
		{Username: "admin", AffCode: "admin-code", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "400")), QuotaWhitelist: true, CreatedAt: now - daySeconds},
	}
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: users[0].Id, Username: "alice", Type: model.LogTypeConsume, Quota: int(mustQuota(t, "100")), CreatedAt: now - 60},
		{UserId: users[2].Id, Username: "demo", Type: model.LogTypeConsume, Quota: int(mustQuota(t, "20")), CreatedAt: now - 30},
	}).Error)

	nextAdjustment := now + 7*daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, StagePercent: 0,
		NextAdjustmentAt: &nextAdjustment, BasisMode: basisModeActual, CreatedBy: "root",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, mustQuota(t, "120"), result.Summary.TotalSpend, "demo/admin usage still consumes the purchased pool")
	assert.Zero(t, result.Summary.ManagedBalance, "negative managed balances participate with their signed value")
	assert.Equal(t, mustQuota(t, "320"), result.Summary.OccupiedAfter)
	assert.Equal(t, 3_200, result.Plan.StagePercent)
	assert.Equal(t, ConcentrationAlgorithmVersion, result.Plan.AlgorithmVersion)
	var parameters PlanParameters
	require.NoError(t, common.Unmarshal([]byte(result.Plan.Parameters), &parameters))
	assert.Equal(t, int64(15_000), parameters.ConcentrationMultiplier)
	require.Len(t, result.Items, 2)
	assert.Equal(t, model.QuotaAdjustmentActionInitialize, result.Items[0].Action)
	assert.Equal(t, mustQuota(t, "210"), result.Items[0].AdjustmentQuota)
	assert.Equal(t, model.QuotaAdjustmentActionReclaim, result.Items[1].Action)
	assert.Equal(t, -mustQuota(t, "10"), result.Items[1].AdjustmentQuota)

	var storedPlans int64
	var storedItems int64
	require.NoError(t, db.Model(&model.QuotaPlan{}).Count(&storedPlans).Error)
	require.NoError(t, db.Model(&model.QuotaItem{}).Count(&storedItems).Error)
	assert.Equal(t, int64(1), storedPlans)
	assert.Equal(t, int64(2), storedItems)
}

func setupQuotaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "quota.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Log{}, &model.QuotaCycle{}, &model.QuotaPlan{}, &model.QuotaItem{},
		&model.QuotaCycleSettlement{}, &model.QuotaRecoveryRequest{}, &model.QuotaShadowEvidence{},
	))

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousBatch := common.BatchUpdateEnabled
	previousRedis := common.RedisEnabled
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.BatchUpdateEnabled = previousBatch
		common.RedisEnabled = previousRedis
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	return db
}

func TestGeneratePlanRejectsBatchUpdatedBalancesBeforeDatabaseAccess(t *testing.T) {
	previous := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = previous })

	_, err := GeneratePlan(GenerateParams{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BATCH_UPDATE_ENABLED")
}

func TestGeneratePlanRejectsMissingLogDatabase(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 7*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive, CreatedBy: "root",
	}
	require.NoError(t, db.Create(&cycle).Error)
	model.LOG_DB = nil
	nextAdjustment := now + daySeconds

	_, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		NextAdjustmentAt: &nextAdjustment, BasisMode: basisModeActual,
	})

	require.EqualError(t, err, "日志数据库未初始化")
}

func TestGenerateAdjustmentUsesManualGrantExecutionAsObservationStart(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 10*daySeconds, CycleEndAt: now + 20*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusActive,
	}
	user := model.User{
		Username: "recent-manual-grant", AffCode: "recent-manual-grant",
		Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "100")), CreatedAt: now - 60*daySeconds,
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	grantPlan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 1_000,
		SnapshotAt: now - 60, AlgorithmVersion: AlgorithmVersion,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &now,
	}
	require.NoError(t, db.Create(&grantPlan).Error)
	require.NoError(t, db.Create(&model.QuotaItem{
		PlanId: grantPlan.Id, UserId: user.Id, Action: model.QuotaAdjustmentActionGrant,
	}).Error)

	nextAdjustment := now + 7*daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 7_500,
		NextAdjustmentAt: &nextAdjustment, BasisMode: basisModeActual,
		EarlyReclaim: true, ReclaimCapPercent: 30, UsageBonusPercent: 30, CreatedBy: "root",
	})
	require.NoError(t, err)
	for _, item := range result.Items {
		assert.False(t, item.UserId == user.Id && item.AdjustmentQuota < 0)
	}
}

func TestAdjustmentsToItemsRejectsUnsupportedUserBalance(t *testing.T) {
	_, err := adjustmentsToItems(1, []userAdjustment{{
		UserID: 1, SnapshotBalance: int64(common.MaxQuota), Adjustment: 1,
	}})

	require.EqualError(t, err, "用户 1 调整后余额超出可支持范围")

	_, err = adjustmentsToItems(1, []userAdjustment{{
		UserID: 2, SnapshotBalance: int64(common.MinQuota), Adjustment: -1,
	}})
	require.EqualError(t, err, "用户 2 调整后余额超出可支持范围")
}

func TestAdjustmentsToItemsAllowsSupportedNegativeUserBalance(t *testing.T) {
	items, err := adjustmentsToItems(1, []userAdjustment{{
		UserID: 1, SnapshotBalance: -100, Adjustment: 30,
	}})

	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, int64(-100), items[0].SnapshotBalanceQuota)
	assert.Equal(t, int64(30), items[0].AdjustmentQuota)
}

func TestNormalizeGenerateParamsForcesThoroughReleaseRules(t *testing.T) {
	requestedNext := int64(10)
	params := normalizeGenerateParams(GenerateParams{
		PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 10_000,
		NextAdjustmentAt: &requestedNext, ReclaimCapPercent: 30,
		UsageBonusPercent: 30, ThoroughRelease: true,
	}, model.QuotaCycle{CycleEndAt: 20})

	require.NotNil(t, params.NextAdjustmentAt)
	assert.Equal(t, int64(20), *params.NextAdjustmentAt)
	assert.True(t, params.EarlyReclaim)
	assert.Equal(t, 100, params.ReclaimCapPercent)
	assert.Zero(t, params.UsageBonusPercent)
}

func TestNormalizeGenerateParamsUsesShanghaiAdjustmentDate(t *testing.T) {
	requestedNext := time.Date(2026, time.August, 10, 15, 45, 0, 0, shanghaiLocation).Unix()
	cycleEnd := time.Date(2026, time.August, 20, 18, 0, 0, 0, shanghaiLocation).Unix()
	params := normalizeGenerateParams(
		GenerateParams{NextAdjustmentAt: &requestedNext},
		model.QuotaCycle{CycleEndAt: cycleEnd},
	)

	require.NotNil(t, params.NextAdjustmentAt)
	assert.Equal(t, time.Date(2026, time.August, 10, 9, 0, 0, 0, shanghaiLocation).Unix(), *params.NextAdjustmentAt)

	requestedNext = time.Date(2026, time.August, 20, 9, 0, 0, 0, shanghaiLocation).Unix()
	params = normalizeGenerateParams(
		GenerateParams{NextAdjustmentAt: &requestedNext},
		model.QuotaCycle{CycleEndAt: cycleEnd},
	)
	require.NotNil(t, params.NextAdjustmentAt)
	assert.Equal(t, cycleEnd, *params.NextAdjustmentAt)
}

func TestGeneratePlanRejectsOutOfRangeControlsBeforeNormalization(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	nextAdjustment := now + 12*60*60

	_, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		NextAdjustmentAt: &nextAdjustment, BasisMode: basisModeActual,
		ReclaimCapPercent: 101,
	})

	require.EqualError(t, err, "调减上限和按用量追加比例必须是0%至100%的整数")
}

func TestInitializationReclaimsSoftDeletedUserBalance(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 7*daySeconds, CycleEndAt: now + 7*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	user := model.User{
		Username: "deleted", AffCode: "deleted-code", Status: common.UserStatusEnabled,
		Quota: int(mustQuota(t, "10")), CreatedAt: now - daySeconds,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Delete(&user).Error)
	nextAdjustment := now + daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		NextAdjustmentAt: &nextAdjustment, BasisMode: basisModeActual,
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, model.QuotaAdjustmentActionReclaim, result.Items[0].Action)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, result.Plan.Id, cycle.Id, "root", now+60)
		return executeErr
	}))
	var stored model.User
	require.NoError(t, db.Unscoped().First(&stored, user.Id).Error)
	assert.Zero(t, stored.Quota)
}
