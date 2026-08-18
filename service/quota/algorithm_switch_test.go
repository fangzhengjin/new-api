package quota

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordShadowEvidenceOnlyPersistsOneFinalWindowResult(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCompanyQuotaModeForTest(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 37*daySeconds, CycleEndAt: now + daySeconds,
		BudgetQuota: mustQuota(t, "480"), InitialGrantQuota: mustQuota(t, "100"),
		RecoveryReserveQuota: mustQuota(t, "20"), Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	users := []model.User{
		{Username: "evidence-low", AffCode: "evidence-low", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "10"))},
		{Username: "evidence-high", AffCode: "evidence-high", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "90"))},
	}
	require.NoError(t, db.Create(&users).Error)
	executedAt := now - 60
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, StagePercent: 9_500,
		SnapshotAt: now - 120, AlgorithmVersion: LegacyAlgorithmVersion,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	require.NoError(t, db.Create(&[]model.QuotaCycleSettlement{
		{BusinessKey: "evidence-a", CycleId: cycle.Id, UserId: users[0].Id, BillingAt: now - 60, Quota: mustQuota(t, "100"), UpdatedAt: now},
		{BusinessKey: "evidence-b", CycleId: cycle.Id, UserId: users[1].Id, BillingAt: now - 60, Quota: mustQuota(t, "100"), UpdatedAt: now},
	}).Error)
	params := GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 10_000,
		NextAdjustmentAt: &cycle.CycleEndAt, BasisMode: basisModeWeek, EarlyReclaim: true,
		ReclaimCapPercent: 100, ThoroughRelease: true, CreatedBy: "root",
	}

	first, err := RecordShadowEvidence(params, "root")
	require.NoError(t, err)
	second, err := RecordShadowEvidence(params, "root")
	require.NoError(t, err)
	assert.Equal(t, first.Id, second.Id)
	assert.True(t, second.Qualified)
	var evidenceCount, planCount int64
	require.NoError(t, db.Model(&model.QuotaShadowEvidence{}).Count(&evidenceCount).Error)
	require.NoError(t, db.Model(&model.QuotaPlan{}).Count(&planCount).Error)
	assert.Equal(t, int64(1), evidenceCount)
	assert.Equal(t, int64(1), planCount)
	var storedUsers []model.User
	require.NoError(t, db.Order("id").Find(&storedUsers).Error)
	assert.Equal(t, users[0].Quota, storedUsers[0].Quota)
	assert.Equal(t, users[1].Quota, storedUsers[1].Quota)
}

func seedQualifiedClosedCycle(t *testing.T, start int64) int {
	t.Helper()
	cycle := model.QuotaCycle{
		CycleStartAt: start, CycleEndAt: start + 10*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusClosed,
	}
	require.NoError(t, model.DB.Create(&cycle).Error)
	require.NoError(t, model.DB.Create(&model.QuotaShadowEvidence{
		CycleId: cycle.Id, SnapshotAt: cycle.CycleEndAt - 1,
		CurrentAlgorithmVersion: LegacyAlgorithmVersion, CandidateAlgorithmVersion: CandidateAlgorithmVersion,
		StagePercent: 10_000, Qualified: true, CreatedAt: cycle.CycleEndAt - 1, CreatedBy: "root",
	}).Error)
	return cycle.Id
}

func TestGetAlgorithmStatusKeepsEmptyQualifiedCycleIDsNonNil(t *testing.T) {
	setupQuotaTestDB(t)

	status, err := GetAlgorithmStatus()

	require.NoError(t, err)
	assert.NotNil(t, status.QualifiedCycleIDs)
	assert.Empty(t, status.QualifiedCycleIDs)
}

func TestCandidateSwitchRequiresTwoClosedEvidenceCyclesAndNoDrafts(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCompanyQuotaModeForTest(t)
	now := time.Now().Unix()
	seedQualifiedClosedCycle(t, now-40*daySeconds)
	active := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 20*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		RecoveryReserveQuota: mustQuota(t, "100"), Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&active).Error)

	_, err := SwitchProductionAlgorithm(CandidateAlgorithmVersion, enableCandidateConfirmation, "root")
	require.EqualError(t, err, "候选算法尚未取得两个已完成周期的合格影子证据")
	seedQualifiedClosedCycle(t, now-25*daySeconds)
	draft := model.QuotaPlan{
		CycleId: active.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 7_500,
		SnapshotAt: now, AlgorithmVersion: LegacyAlgorithmVersion, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&draft).Error)
	_, err = SwitchProductionAlgorithm(CandidateAlgorithmVersion, enableCandidateConfirmation, "root")
	assert.Contains(t, err.Error(), "待执行草稿")
	var stored model.QuotaCycle
	require.NoError(t, db.First(&stored, active.Id).Error)
	assert.Equal(t, LegacyAlgorithmVersion, cycleAlgorithmVersion(&stored))
}

func TestCandidateSwitchChangesPlanVersionWithoutChangingBalanceAndCanRollback(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCompanyQuotaModeForTest(t)
	now := time.Now().Unix()
	firstID := seedQualifiedClosedCycle(t, now-40*daySeconds)
	secondID := seedQualifiedClosedCycle(t, now-25*daySeconds)
	active := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 20*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		RecoveryReserveQuota: mustQuota(t, "100"), Status: model.QuotaCycleStatusActive,
	}
	scheduled := model.QuotaCycle{
		CycleStartAt: now + 20*daySeconds, CycleEndAt: now + 40*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		RecoveryReserveQuota: mustQuota(t, "100"), Status: model.QuotaCycleStatusScheduled,
	}
	require.NoError(t, db.Create(&active).Error)
	require.NoError(t, db.Create(&scheduled).Error)
	user := model.User{Username: "switch-user", AffCode: "switch-user", Status: 1, Quota: int(mustQuota(t, "10"))}
	require.NoError(t, db.Create(&user).Error)

	status, err := SwitchProductionAlgorithm(CandidateAlgorithmVersion, enableCandidateConfirmation, "root")
	require.NoError(t, err)
	assert.Equal(t, CandidateAlgorithmVersion, status.CurrentVersion)
	assert.Equal(t, enableCandidateConfirmation, status.EnableConfirmation)
	assert.Equal(t, rollbackLegacyConfirmation, status.RollbackConfirmation)
	assert.ElementsMatch(t, []int{firstID, secondID}, status.QualifiedCycleIDs)
	assert.True(t, status.RollbackAllowed)
	require.NoError(t, db.First(&active, active.Id).Error)
	require.NoError(t, db.First(&scheduled, scheduled.Id).Error)
	assert.Equal(t, CandidateAlgorithmVersion, active.AllocationAlgorithmVersion)
	assert.Equal(t, CandidateAlgorithmVersion, scheduled.AllocationAlgorithmVersion)
	assert.True(t, active.LegacyRollbackAllowed)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "10")), user.Quota)

	next := now + daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: active.Id, PlanType: model.QuotaPlanTypeInitialization,
		NextAdjustmentAt: &next, BasisMode: basisModeActual, CreatedBy: "root",
	})
	require.NoError(t, err)
	assert.Equal(t, CandidateAlgorithmVersion, result.Plan.AlgorithmVersion)

	status, err = SwitchProductionAlgorithm(LegacyAlgorithmVersion, rollbackLegacyConfirmation, "root")
	require.NoError(t, err)
	assert.Equal(t, LegacyAlgorithmVersion, status.CurrentVersion)
	assert.False(t, status.RollbackAllowed)
	var plan model.QuotaPlan
	require.NoError(t, db.First(&plan, result.Plan.Id).Error)
	assert.Equal(t, model.QuotaPlanStatusCancelled, plan.Status)
}
