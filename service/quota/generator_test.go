package quota

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGenerateInitializationPlanUsesManagedBalances(t *testing.T) {
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

	nextAdjustment := now + 7*daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, AlgorithmVersion, result.Plan.AlgorithmVersion)
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

func TestGenerateAdjustmentCapsAutomaticPositionByCycleMultiplier(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 10*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "2000"), InitialGrantQuota: mustQuota(t, "100"),
		TemporaryQuotaReserve:   mustQuota(t, "200"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	users := []model.User{
		{Username: "heavy", AffCode: "heavy-code", Status: common.UserStatusEnabled, CreatedAt: now - 60*daySeconds},
		{Username: "over-limit", AffCode: "over-limit-code", Status: common.UserStatusEnabled, CreatedAt: now - 60*daySeconds},
		{Username: "ordinary", AffCode: "ordinary-code", Status: common.UserStatusEnabled, CreatedAt: now - 60*daySeconds},
	}
	require.NoError(t, db.Create(&users).Error)
	executedAt := now - 7*daySeconds
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	periodSpend := mustQuota(t, "800")
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: users[0].Id, Username: users[0].Username, Type: model.LogTypeConsume, Quota: int(periodSpend), CreatedAt: now - daySeconds},
		{UserId: users[1].Id, Username: users[1].Username, Type: model.LogTypeConsume, Quota: int(mustQuota(t, "950")), CreatedAt: now - daySeconds},
	}).Error)

	nextAdjustment := now + 7*daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.NoError(t, err)
	var heavyItem *model.QuotaItem
	var overLimitIncrease *model.QuotaItem
	for index := range result.Items {
		if result.Items[index].UserId == users[0].Id {
			heavyItem = &result.Items[index]
		}
		if result.Items[index].UserId == users[1].Id && result.Items[index].AdjustmentQuota > 0 {
			overLimitIncrease = &result.Items[index]
		}
	}
	require.NotNil(t, heavyItem)
	assert.Positive(t, heavyItem.AdjustmentQuota)
	positionCeiling, err := concentrationPositionCeiling(cycle.BudgetQuota-cycle.TemporaryQuotaReserve, len(users), cycle.ConcentrationMultiplier)
	require.NoError(t, err)
	assert.LessOrEqual(t, periodSpend+heavyItem.SnapshotBalanceQuota+heavyItem.AdjustmentQuota, positionCeiling)
	assert.Nil(t, overLimitIncrease)
}

func TestGenerateAdjustmentKeepsFullCycleSpendAfterLaterPlan(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 10*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	user := model.User{
		Username: "cycle-spend", AffCode: "cycle-spend-code", Status: common.UserStatusEnabled,
		CreatedAt: now - 60*daySeconds,
	}
	require.NoError(t, db.Create(&user).Error)
	initialExecutedAt := now - 7*daySeconds
	laterExecutedAt := now - daySeconds
	require.NoError(t, db.Create(&[]model.QuotaPlan{
		{CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, Status: model.QuotaPlanStatusExecuted, ExecutedAt: &initialExecutedAt},
		{CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, Parameters: `{"observation_start_at":1}`, Status: model.QuotaPlanStatusExecuted, ExecutedAt: &laterExecutedAt},
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: user.Id, Username: user.Username, Type: model.LogTypeConsume,
		Quota: int(mustQuota(t, "1600")), CreatedAt: now - 2*daySeconds,
	}).Error)

	nextAdjustment := now + 7*daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.NoError(t, err)
	for _, item := range result.Items {
		assert.False(t, item.UserId == user.Id && item.AdjustmentQuota > 0)
	}
}

func TestGenerateAdjustmentUsesOnlyObservationWindowForDemand(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 10*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "10000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	user := model.User{
		Username: "window-spend", AffCode: "window-spend-code", Status: common.UserStatusEnabled,
		Quota: int(mustQuota(t, "100")), CreatedAt: now - 60*daySeconds,
	}
	require.NoError(t, db.Create(&user).Error)
	initialExecutedAt := now - 7*daySeconds
	laterExecutedAt := now - daySeconds
	require.NoError(t, db.Create(&[]model.QuotaPlan{
		{CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, Status: model.QuotaPlanStatusExecuted, ExecutedAt: &initialExecutedAt},
		{CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, Parameters: `{"observation_start_at":1}`, Status: model.QuotaPlanStatusExecuted, ExecutedAt: &laterExecutedAt},
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: user.Id, Username: user.Username, Type: model.LogTypeConsume,
		Quota: int(mustQuota(t, "200")), CreatedAt: now - 2*daySeconds,
	}).Error)

	nextAdjustment := now + 7*daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.NoError(t, err)
	for _, item := range result.Items {
		assert.False(t, item.UserId == user.Id && item.AdjustmentQuota > 0)
	}
}

func TestLoadSpendSnapshotUsesNetConsumption(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	users := []userSnapshot{{ID: 101}, {ID: 102}}
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: 101, Type: model.LogTypeConsume, Quota: int(mustQuota(t, "100")), CreatedAt: now - 20},
		{UserId: 101, Type: model.LogTypeRefund, Quota: int(mustQuota(t, "100")), CreatedAt: now - 10},
		{UserId: 102, Type: model.LogTypeConsume, Quota: int(mustQuota(t, "100")), CreatedAt: now - 20},
		{UserId: 102, Type: model.LogTypeRefund, Quota: int(mustQuota(t, "30")), CreatedAt: now - 10},
	}).Error)

	stats, err := loadSpendSnapshot(db, users, now-30, now-30, now)
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats[101].PeriodSpend)
	assert.Equal(t, int64(0), stats[101].RecentSpend)
	assert.Equal(t, mustQuota(t, "70"), stats[102].PeriodSpend)
	assert.Equal(t, mustQuota(t, "70"), stats[102].RecentSpend)
	assert.Equal(t, now-20, stats[102].LastUse)
}

func TestLoadSpendSnapshotRejectsDisabledConsumeLog(t *testing.T) {
	setupQuotaTestDB(t)
	original := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = original })

	_, err := loadSpendSnapshot(model.LOG_DB, []userSnapshot{{ID: 101}}, 1, 1, 2)
	require.EqualError(t, err, "消费日志未启用，不能生成或执行自动调配方案")
}

func TestGenerateAdjustmentCapsLargeHeadroomByWalletLimit(t *testing.T) {
	userQuota := -int64(common.MaxWalletQuota)
	now := time.Now().Unix()
	user := userSnapshot{
		ID: 1, Username: "negative-balance", Status: common.UserStatusEnabled,
		ObservationStartAt: now - 30*daySeconds, Quota: userQuota,
	}

	t.Run("position headroom", func(t *testing.T) {
		ordinaryAllocationCap := int64(math.MaxInt64/3) * 2
		items, err := generateAdjustmentItems(
			[]userSnapshot{user}, map[int]DemandProfile{1: {}}, map[int]spendStats{1: {}},
			mustQuota(t, "100"), ordinaryAllocationCap,
			snapshotParams{
				GenerateParams: GenerateParams{},
				SnapshotAt:     now, CalculationDaysHundred: 700,
				TotalWorkdays: 30, RemainingWorkdays: 7,
			},
			ordinaryAllocationCap, 0, nil, 15_000,
		)
		require.NoError(t, err)
		require.Len(t, items, 1)
		safetyTarget, err := equalSafetyTarget(mustQuota(t, "100"), 30)
		require.NoError(t, err)
		expected, err := roundUpCent(safetyTarget - userQuota)
		require.NoError(t, err)
		assert.Equal(t, expected, items[0].Adjustment)
	})

	t.Run("demand gap", func(t *testing.T) {
		ordinaryAllocationCap := int64(common.MaxWalletQuota) * 10
		items, err := generateAdjustmentItems(
			[]userSnapshot{user},
			map[int]DemandProfile{1: {WeeklyDemand: math.MaxInt64}},
			map[int]spendStats{1: {RecentSpend: 1}},
			mustQuota(t, "100"), ordinaryAllocationCap,
			snapshotParams{
				GenerateParams: GenerateParams{},
				SnapshotAt:     now, CalculationDaysHundred: 700,
				TotalWorkdays: 30, RemainingWorkdays: 0,
			},
			ordinaryAllocationCap, 0, nil, 15_000,
		)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, roundDownCent(int64(common.MaxWalletQuota)-userQuota), items[0].Adjustment)
	})
}

func TestGenerateAdjustmentBasisDescribesObservationWindowDemand(t *testing.T) {
	now := time.Now().Unix()
	items, err := generateAdjustmentItems(
		[]userSnapshot{{
			ID: 1, Username: "low-usage", Status: common.UserStatusEnabled,
			ObservationStartAt: now - 30*daySeconds, Quota: mustQuota(t, "200"),
		}},
		map[int]DemandProfile{1: {WeeklyDemand: mustQuota(t, "100")}},
		map[int]spendStats{1: {PeriodSpend: mustQuota(t, "500")}},
		mustQuota(t, "100"), mustQuota(t, "1000"),
		snapshotParams{
			GenerateParams: GenerateParams{},
			SnapshotAt:     now, CalculationDaysHundred: 700,
			TotalWorkdays: 30, RemainingWorkdays: 7,
		},
		mustQuota(t, "1000"), mustQuota(t, "200"), nil, 15_000,
	)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Contains(t, items[0].BasisText, "按本次观察期用量及余额接近用完时的修正用量")
	assert.NotContains(t, items[0].BasisText, "按本期、近期")
}

func TestGeneratePlanRejectsRepeatedInitialization(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	executedAt := now - 60
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)

	nextAdjustment := now + 7*daySeconds
	_, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.EqualError(t, err, "当前周期已完成首次发放，不能重复生成初始化方案")

	var planCount int64
	require.NoError(t, db.Model(&model.QuotaPlan{}).Where("cycle_id = ?", cycle.Id).Count(&planCount).Error)
	assert.Equal(t, int64(1), planCount)
}

func TestInternalQuotaActionsDoNotReplaceInitializationPlan(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	manualAt := now - 120
	temporaryGrantAt := now - 60
	require.NoError(t, db.Create(&[]model.QuotaPlan{
		{
			CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
			Parameters: `{"manual":true}`, Status: model.QuotaPlanStatusExecuted, ExecutedAt: &manualAt,
		},
		{
			CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
			Parameters: `{"temporary_quota_request_id":1}`, Status: model.QuotaPlanStatusExecuted, ExecutedAt: &temporaryGrantAt,
		},
	}).Error)

	nextAdjustment := now + 7*daySeconds
	_, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.EqualError(t, err, "当前周期尚无已执行方案，请先完成初始化发放")

	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.NoError(t, err)
	assert.Equal(t, model.QuotaPlanTypeInitialization, result.Plan.PlanType)
}

func TestAdjustmentWithoutInitializationDoesNotSatisfyAllocationGate(t *testing.T) {
	db := setupQuotaTestDB(t)
	executedAt := time.Now().Unix()
	cycle := model.QuotaCycle{Status: model.QuotaCycleStatusActive}
	require.NoError(t, db.Create(&cycle).Error)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		Parameters: `{"observation_start_at":1}`, Status: model.QuotaPlanStatusExecuted,
		ExecutedAt: &executedAt,
	}
	require.NoError(t, db.Create(&plan).Error)

	stats, err := loadAllocationExecutionStats(db, cycle.Id)
	require.NoError(t, err)
	assert.False(t, stats.AllocationExecuted)
	assert.Equal(t, executedAt, stats.LatestExecutedAt)
	assert.Equal(t, []int{plan.Id}, stats.PlanIDs)
}

func TestGenerateInitializationKeepsTemporaryGrantOutOfOrdinaryPool(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "200")
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "500"),
		TemporaryQuotaReserve: mustQuota(t, "200"), ConcentrationMultiplier: 15_000,
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	users := []model.User{
		{Username: "temporary-holder", AffCode: "temporary-holder", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "200"))},
		{Username: "ordinary-user", AffCode: "ordinary-user", Status: common.UserStatusEnabled},
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, db.Create(&model.TemporaryQuotaRequest{
		CycleId: cycle.Id, UserId: users[0].Id, Username: users[0].Username,
		IdempotencyKey: "before-initialization", RequestedQuota: mustQuota(t, "200"),
		ApprovedQuota: mustQuota(t, "200"), Status: model.TemporaryQuotaRequestStatusExecuted,
	}).Error)

	nextAdjustment := now + 7*daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.NoError(t, err)
	var plannedIncrease int64
	for _, item := range result.Items {
		if item.AdjustmentQuota > 0 {
			plannedIncrease += item.AdjustmentQuota
		}
	}
	assert.Equal(t, mustQuota(t, "800"), plannedIncrease)
}

func TestGenerateAdjustmentRequiresExecutedPlan(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)

	nextAdjustment := now + 7*daySeconds
	_, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.EqualError(t, err, "当前周期尚无已执行方案，请先完成初始化发放")
}

func TestGenerateAdjustmentKeepsTemporaryGrantOutOfOrdinaryPool(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "800")
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		TemporaryQuotaReserve: mustQuota(t, "200"), ConcentrationMultiplier: 15_000,
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	users := []model.User{
		{Username: "regular-holder", AffCode: "regular-holder-code", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "600")), CreatedAt: now - daySeconds},
		{Username: "temporary-holder", AffCode: "temporary-holder-code", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "200")), CreatedAt: now - daySeconds},
		{Username: "ordinary-target", AffCode: "ordinary-target-code", Status: common.UserStatusEnabled, CreatedAt: now - daySeconds},
	}
	require.NoError(t, db.Create(&users).Error)
	executedAt := now - 12*60*60
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	require.NoError(t, db.Create(&model.TemporaryQuotaRequest{
		CycleId: cycle.Id, UserId: users[1].Id, Username: users[1].Username,
		IdempotencyKey: "executed-temporary", RequestedQuota: mustQuota(t, "200"),
		ApprovedQuota: mustQuota(t, "200"), Status: model.TemporaryQuotaRequestStatusExecuted,
	}).Error)

	nextAdjustment := now + 7*daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.NoError(t, err)
	var targetIncrease int64
	for _, item := range result.Items {
		if item.UserId == users[2].Id && item.AdjustmentQuota > 0 {
			targetIncrease = item.AdjustmentQuota
		}
	}
	assert.Positive(t, targetIncrease)
	assert.LessOrEqual(t, targetIncrease, mustQuota(t, "200"))
}

func setupQuotaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "quota.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{}, &model.User{}, &model.Log{}, &model.QuotaCycle{}, &model.QuotaPlan{}, &model.QuotaItem{},
		&model.TemporaryQuotaRequest{}, &model.Task{}, &model.Midjourney{},
	))
	require.NoError(t, db.Create(&model.Option{Key: model.CycleQuotaManagementOptionKey, Value: "true"}).Error)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:initialize_active_cycle_allocation", func(tx *gorm.DB) {
		cycle, ok := tx.Statement.Dest.(*model.QuotaCycle)
		if !ok || cycle.Status != model.QuotaCycleStatusActive || cycle.AllocatedQuota != nil {
			return
		}
		zero := int64(0)
		now := time.Now().Unix()
		cycle.OpeningAllocatedQuota = &zero
		cycle.AllocatedQuota = &zero
		cycle.AllocationBaselineAt = &now
	}))

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousBatch := common.BatchUpdateEnabled
	previousRedis := common.RedisEnabled
	previousMode := operation_setting.CycleQuotaManagementEnabled
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	operation_setting.CycleQuotaManagementEnabled = true
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.BatchUpdateEnabled = previousBatch
		common.RedisEnabled = previousRedis
		operation_setting.CycleQuotaManagementEnabled = previousMode
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	return db
}

func TestGeneratePlanRejectsBatchUpdatedBalancesBeforeDatabaseAccess(t *testing.T) {
	previous := common.BatchUpdateEnabled
	previousMode := operation_setting.CycleQuotaManagementEnabled
	common.BatchUpdateEnabled = true
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() {
		common.BatchUpdateEnabled = previous
		operation_setting.CycleQuotaManagementEnabled = previousMode
	})

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
		ConcentrationMultiplier: 15_000,
		Status:                  model.QuotaCycleStatusActive, CreatedBy: "root",
	}
	require.NoError(t, db.Create(&cycle).Error)
	executedAt := now - 12*60*60
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	model.LOG_DB = nil
	nextAdjustment := now + daySeconds

	_, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		NextAdjustmentAt: &nextAdjustment,
	})

	require.EqualError(t, err, "日志数据库未初始化")
}

func TestLoadAllocationExecutionStatsRejectsMissingParameters(t *testing.T) {
	db := setupQuotaTestDB(t)
	plan := model.QuotaPlan{
		CycleId: 1, PlanType: model.QuotaPlanTypeAdjustment,
		Status: model.QuotaPlanStatusExecuted,
	}
	require.NoError(t, db.Create(&plan).Error)

	_, err := loadAllocationExecutionStats(db, plan.CycleId)
	require.ErrorContains(t, err, "参数无效")
}

func TestGenerateAdjustmentUsesLatestExecutedPlanAsObservationStart(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 10*daySeconds, CycleEndAt: now + 20*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000,
		BalancePolicy:           model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusActive,
	}
	user := model.User{
		Username: "recent-manual-grant", AffCode: "recent-manual-grant",
		Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "100")), CreatedAt: now - 60*daySeconds,
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	initialExecutedAt := now - 7*daySeconds
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &initialExecutedAt,
	}).Error)
	latestPlan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now - 60, AlgorithmVersion: AlgorithmVersion,
		Parameters: `{"observation_start_at":1}`,
		Status:     model.QuotaPlanStatusExecuted, ExecutedAt: &now,
	}
	require.NoError(t, db.Create(&latestPlan).Error)

	nextAdjustment := now + 7*daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		NextAdjustmentAt: &nextAdjustment,
		CreatedBy:        "root",
	})
	require.NoError(t, err)
	for _, item := range result.Items {
		assert.False(t, item.UserId == user.Id && item.AdjustmentQuota < 0)
	}
}

func TestAdjustmentObservationIgnoresInternalQuotaActions(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 10*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	initialAt := now - 7*daySeconds
	manualAt := now - 2*daySeconds
	temporaryGrantAt := now - daySeconds
	require.NoError(t, db.Create(&[]model.QuotaPlan{
		{
			CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
			Status: model.QuotaPlanStatusExecuted, ExecutedAt: &initialAt,
		},
		{
			CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
			Parameters: `{"manual":true}`, Status: model.QuotaPlanStatusExecuted, ExecutedAt: &manualAt,
		},
		{
			CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
			Parameters: `{"temporary_quota_request_id":1}`, Status: model.QuotaPlanStatusExecuted, ExecutedAt: &temporaryGrantAt,
		},
	}).Error)

	nextAdjustment := now + 7*daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.NoError(t, err)
	var parameters PlanParameters
	require.NoError(t, common.Unmarshal([]byte(result.Plan.Parameters), &parameters))
	assert.Equal(t, initialAt, parameters.ObservationStartAt)
}

func TestLatestPositiveAutomaticAdjustmentIgnoresInternalQuotaActions(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{Status: model.QuotaCycleStatusActive}
	user := model.User{Username: "alice", AffCode: "alice-code", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	initialAt := now - 7*daySeconds
	manualAt := now - 2*daySeconds
	temporaryGrantAt := now - daySeconds
	plans := []model.QuotaPlan{
		{
			CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
			Status: model.QuotaPlanStatusExecuted, ExecutedAt: &initialAt,
		},
		{
			CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
			Parameters: `{"manual":true}`, Status: model.QuotaPlanStatusExecuted, ExecutedAt: &manualAt,
		},
		{
			CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
			Parameters: `{"temporary_quota_request_id":1}`, Status: model.QuotaPlanStatusExecuted, ExecutedAt: &temporaryGrantAt,
		},
	}
	require.NoError(t, db.Create(&plans).Error)
	for _, plan := range plans {
		require.NoError(t, db.Create(&model.QuotaItem{
			PlanId: plan.Id, UserId: user.Id, AdjustmentQuota: mustQuota(t, "10"),
		}).Error)
	}

	executionStats, err := loadAllocationExecutionStats(db, cycle.Id)
	require.NoError(t, err)
	latest, err := loadLatestPositiveAdjustmentTimes(db, executionStats.PlanIDs)
	require.NoError(t, err)
	assert.Equal(t, initialAt, latest[user.Id])
}

func TestAdjustmentsToItemsRejectsUnsupportedUserBalance(t *testing.T) {
	_, err := adjustmentsToItems(1, []userAdjustment{{
		UserID: 1, SnapshotBalance: int64(common.MaxWalletQuota), Adjustment: 1,
	}})

	require.EqualError(t, err, "用户 1 调整后余额超出可支持范围")

	_, err = adjustmentsToItems(1, []userAdjustment{{
		UserID: 2, SnapshotBalance: -int64(common.MaxWalletQuota), Adjustment: -1,
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

func TestGeneratePlanRejectsOverlongName(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000,
		Status:                  model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	nextAdjustment := now + 12*60*60

	_, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		Name: strings.Repeat("长", PlanNameMaxRunes+1), NextAdjustmentAt: &nextAdjustment,
	})

	require.EqualError(t, err, "方案名称或目的过长")
}

func TestGenerationUsesNextAdjustmentAsPlanWindow(t *testing.T) {
	snapshotAt := time.Date(2026, time.August, 3, 9, 0, 0, 0, shanghaiLocation).Unix()
	nextAdjustment := time.Date(2026, time.August, 10, 9, 0, 0, 0, shanghaiLocation).Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: snapshotAt - daySeconds,
		CycleEndAt:   time.Date(2026, time.August, 31, 18, 0, 0, 0, shanghaiLocation).Unix(),
		BudgetQuota:  mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}

	snapshot, err := validateGenerationParams(GenerateParams{
		PlanType:         model.QuotaPlanTypeAdjustment,
		NextAdjustmentAt: &nextAdjustment,
	}, cycle, snapshotAt)
	require.NoError(t, err)
	assert.Equal(t, CountWeekdays(snapshotAt, nextAdjustment, true), snapshot.RemainingWorkdays)
	assert.Less(t, snapshot.RemainingWorkdays, CountWeekdays(snapshotAt, cycle.CycleEndAt, true))
}

func TestInitializationReclaimsSoftDeletedUserBalance(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "10")
	cycle := model.QuotaCycle{
		CycleStartAt: now - 7*daySeconds, CycleEndAt: now + 7*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
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
		NextAdjustmentAt: &nextAdjustment,
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
