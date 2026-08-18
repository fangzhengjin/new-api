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

func TestFairnessShadowIsReadOnlyAndDeterministicForOneSnapshot(t *testing.T) {
	db := setupQuotaTestDB(t)
	setting := operation_setting.GetQuotaSetting()
	previousMode := setting.EnableCompanyQuotaMode
	setting.EnableCompanyQuotaMode = true
	t.Cleanup(func() { setting.EnableCompanyQuotaMode = previousMode })

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
