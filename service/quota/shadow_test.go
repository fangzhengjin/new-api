package quota

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverageBasisPointsPreservesNegativeBalance(t *testing.T) {
	coverage, err := coverageBasisPoints(-20, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(-2_000), coverage)
}

func TestConcentrationShadowPreservesNegativeMaximumPositionShare(t *testing.T) {
	calculation := &planCalculation{
		Result:   &PlanResult{Summary: PlanSummary{StageCap: 100}},
		Cycle:    model.QuotaCycle{InitialGrantQuota: 100},
		Snapshot: snapshotParams{GenerateParams: GenerateParams{BasisMode: basisModeActual}, TotalWorkdays: 1},
		Users:    []userSnapshot{{ID: 1, Status: common.UserStatusEnabled, Quota: -20}},
		Profiles: map[int]DemandProfile{1: {WeeklyDemand: 100}},
		Stats:    map[int]spendStats{1: {}},
	}

	variant, err := concentrationShadowVariant(calculation, 15_000)
	require.NoError(t, err)
	assert.Equal(t, int64(-2_000), variant.MaximumPositionShare)
	metrics, _, err := fairnessMetrics(calculation)
	require.NoError(t, err)
	assert.Equal(t, 1, metrics.Population)
}

func TestFairnessShadowIsReadOnlyAndDeterministicForOneSnapshot(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })

	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 7*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "480"), InitialGrantQuota: mustQuota(t, "100"),
		RecoveryReserveQuota: mustQuota(t, "20"), Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	users := []model.User{
		{Username: "lowest-coverage", AffCode: "lowest-coverage", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "10")), CreatedAt: now - 30*daySeconds},
		{Username: "highest-coverage", AffCode: "highest-coverage", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "90")), CreatedAt: now - 30*daySeconds},
	}
	require.NoError(t, db.Create(&users).Error)
	executedAt := now - 60
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, StagePercent: 2_000,
		SnapshotAt: now - 120, AlgorithmVersion: AlgorithmVersion,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	require.NoError(t, db.Create(&[]model.QuotaCycleSettlement{
		{BusinessKey: "shadow-a", CycleId: cycle.Id, UserId: users[0].Id, BillingAt: now - 60, Quota: mustQuota(t, "100"), UpdatedAt: now},
		{BusinessKey: "shadow-b", CycleId: cycle.Id, UserId: users[1].Id, BillingAt: now - 60, Quota: mustQuota(t, "100"), UpdatedAt: now},
	}).Error)
	nextAdjustment := now + 7*daySeconds
	params := GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 7_500,
		NextAdjustmentAt: &nextAdjustment, BasisMode: basisModeWeek, CreatedBy: "root",
	}

	first, err := compareFairnessAt(params, now)
	require.NoError(t, err)
	second, err := compareFairnessAt(params, now)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.True(t, first.CandidateQualified)
	assert.Equal(t, CandidateAlgorithmVersion, first.CandidateAlgorithmVersion)
	assert.GreaterOrEqual(t, first.Candidate.MinimumCoverageBasisPoints, first.Current.MinimumCoverageBasisPoints)
	assert.GreaterOrEqual(t, first.Candidate.P10CoverageBasisPoints, first.Current.P10CoverageBasisPoints)
	assert.LessOrEqual(t, first.Candidate.SafetyUnmet, first.Current.SafetyUnmet)
	require.Len(t, first.ConcentrationVariants, 3)
	assert.Equal(t, int64(15_000), first.ConcentrationVariants[0].MultiplierBasisPoints)
	assert.Equal(t, int64(20_000), first.ConcentrationVariants[1].MultiplierBasisPoints)
	assert.Equal(t, int64(30_000), first.ConcentrationVariants[2].MultiplierBasisPoints)

	var plans, items int64
	require.NoError(t, db.Model(&model.QuotaPlan{}).Count(&plans).Error)
	require.NoError(t, db.Model(&model.QuotaItem{}).Count(&items).Error)
	assert.Equal(t, int64(1), plans)
	assert.Zero(t, items)
	var storedUsers []model.User
	require.NoError(t, db.Order("id").Find(&storedUsers).Error)
	assert.Equal(t, users[0].Quota, storedUsers[0].Quota)
	assert.Equal(t, users[1].Quota, storedUsers[1].Quota)
}

func TestConcentrationShadowCapsCumulativePositionBeforeDemandTopUp(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })

	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 7*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1600"), InitialGrantQuota: mustQuota(t, "100"),
		RecoveryReserveQuota: mustQuota(t, "200"), ConcentrationMultiplier: 15_000,
		Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	users := []model.User{
		{Username: "at-position-cap", AffCode: "at-position-cap", Status: common.UserStatusEnabled, CreatedAt: now - 30*daySeconds},
		{Username: "below-position-cap", AffCode: "below-position-cap", Status: common.UserStatusEnabled, CreatedAt: now - 30*daySeconds},
	}
	require.NoError(t, db.Create(&users).Error)
	executedAt := now - 60
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, StagePercent: 1_000,
		SnapshotAt: now - 120, AlgorithmVersion: AlgorithmVersion,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	require.NoError(t, db.Create(&[]model.QuotaCycleSettlement{
		{BusinessKey: "cap-high", CycleId: cycle.Id, UserId: users[0].Id, BillingAt: now - 60, Quota: mustQuota(t, "800"), UpdatedAt: now},
		{BusinessKey: "cap-low", CycleId: cycle.Id, UserId: users[1].Id, BillingAt: now - 60, Quota: mustQuota(t, "100"), UpdatedAt: now},
	}).Error)
	nextAdjustment := now + 7*daySeconds
	comparison, err := compareFairnessAt(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 7_500,
		NextAdjustmentAt: &nextAdjustment, BasisMode: basisModeWeek, CreatedBy: "root",
	}, now)
	require.NoError(t, err)
	require.Len(t, comparison.ConcentrationVariants, 3)

	adjustments := make([]int64, 0, 3)
	capped := make([]int64, 0, 3)
	for _, variant := range comparison.ConcentrationVariants {
		require.Len(t, variant.Items, 2)
		item := variant.Items[0]
		assert.Equal(t, users[0].Id, item.UserID)
		assert.Equal(t, mustQuota(t, "800"), item.CurrentPositionQuota)
		assert.Equal(t, item.SafetyTargetQuota, variant.Items[1].SafetyTargetQuota)
		assert.Greater(t, item.CappedQuota, int64(0))
		adjustments = append(adjustments, item.AdjustmentQuota)
		capped = append(capped, item.CappedQuota)
	}
	assert.Equal(t, comparison.ConcentrationVariants[0].Items[0].SafetyTargetQuota, adjustments[0])
	assert.LessOrEqual(t, adjustments[0], adjustments[1])
	assert.LessOrEqual(t, adjustments[1], adjustments[2])
	assert.GreaterOrEqual(t, capped[0], capped[1])
	assert.GreaterOrEqual(t, capped[1], capped[2])
	assert.Greater(t, comparison.ConcentrationVariants[0].MinimumEffectiveCoverage, comparison.ConcentrationVariants[0].MinimumRawCoverage)

	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 7_500,
		NextAdjustmentAt: &nextAdjustment, BasisMode: basisModeWeek, CreatedBy: "root",
	})
	require.NoError(t, err)
	assert.Equal(t, ConcentrationAlgorithmVersion, result.Plan.AlgorithmVersion)
	var parameters PlanParameters
	require.NoError(t, common.Unmarshal([]byte(result.Plan.Parameters), &parameters))
	assert.Equal(t, int64(15_000), parameters.ConcentrationMultiplier)
	previewAdjustments := make(map[int]int64)
	for _, item := range comparison.ConcentrationVariants[0].Items {
		previewAdjustments[item.UserID] = item.AdjustmentQuota
	}
	for _, item := range result.Items {
		assert.Equal(t, previewAdjustments[item.UserId], item.AdjustmentQuota)
	}
}

func TestConcentrationShadowLeavesUnneededCapacityInCompanyPool(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })

	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 7*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		RecoveryReserveQuota: mustQuota(t, "100"), Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	users := []model.User{
		{Username: "already-funded-a", AffCode: "already-funded-a", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "100")), CreatedAt: now - 30*daySeconds},
		{Username: "already-funded-b", AffCode: "already-funded-b", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "100")), CreatedAt: now - 30*daySeconds},
	}
	require.NoError(t, db.Create(&users).Error)
	executedAt := now - 60
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, StagePercent: 2_000,
		SnapshotAt: now - 120, AlgorithmVersion: AlgorithmVersion,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	nextAdjustment := now + 7*daySeconds
	comparison, err := compareFairnessAt(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 7_500,
		NextAdjustmentAt: &nextAdjustment, BasisMode: basisModeWeek, CreatedBy: "root",
	}, now)
	require.NoError(t, err)
	for _, variant := range comparison.ConcentrationVariants {
		assert.Zero(t, variant.PlannedIncreaseQuota)
		assert.Equal(t, mustQuota(t, "450"), variant.UnallocatedStageQuota)
	}
}

func TestConcentrationShadowWithNoEligibleUsersHasNoPositiveAllocation(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })

	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 7*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		RecoveryReserveQuota: mustQuota(t, "100"), Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	executedAt := now - 60
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, StagePercent: 0,
		SnapshotAt: now - 120, AlgorithmVersion: AlgorithmVersion,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	nextAdjustment := now + 7*daySeconds
	comparison, err := compareFairnessAt(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 7_500,
		NextAdjustmentAt: &nextAdjustment, BasisMode: basisModeWeek, CreatedBy: "root",
	}, now)
	require.NoError(t, err)
	for _, variant := range comparison.ConcentrationVariants {
		assert.Zero(t, variant.Population)
		assert.Zero(t, variant.PositionCeilingQuota)
		assert.Zero(t, variant.PlannedIncreaseQuota)
		assert.Empty(t, variant.Items)
	}
}
