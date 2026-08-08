package quota

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateInitialGrantQuotaRejectsUnsupportedUserBalance(t *testing.T) {
	require.EqualError(
		t,
		validateInitialGrantQuota(int64(common.MaxQuota)+1),
		"人均首次额度超出单用户可支持范围",
	)
}

func TestCreateCycleRejectsOverlappingWindow(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	_, err := CreateCycle(CreateCycleParams{
		StartAt: now, EndAt: now + 10*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		CreatedBy: "root",
	})
	require.NoError(t, err)

	_, err = CreateCycle(CreateCycleParams{
		StartAt: now + daySeconds, EndAt: now + 11*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		CreatedBy: "root",
	})
	require.EqualError(t, err, "周期时间与现有周期冲突")

	var count int64
	require.NoError(t, db.Model(&model.QuotaCycle{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestGetScheduledCycleDetailIncludesInitialGrantRecommendation(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycles := []model.QuotaCycle{
		{
			CycleStartAt: now - 30*daySeconds, CycleEndAt: now - daySeconds,
			BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
			Status: model.QuotaCycleStatusClosed,
		},
		{
			CycleStartAt: now + daySeconds, CycleEndAt: now + 31*daySeconds,
			BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
			Status: model.QuotaCycleStatusScheduled,
		},
	}
	require.NoError(t, db.Create(&cycles).Error)
	user := model.User{
		Username: "alice", Status: common.UserStatusEnabled, CreatedAt: now - 60*daySeconds,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: user.Id, Type: model.LogTypeConsume, Quota: int(mustQuota(t, "80")), CreatedAt: now - 2*daySeconds,
	}).Error)

	detail, err := GetCycleDetail(cycles[1].Id)
	require.NoError(t, err)
	require.NotNil(t, detail.Recommendation)
	assert.Equal(t, mustQuota(t, "96"), detail.Recommendation.Quota)
	assert.Equal(t, 1, detail.Recommendation.UserCount)
}

func TestSummarizePlanPreservesAdjustmentAndFundFlowTotals(t *testing.T) {
	t.Parallel()
	plan := model.QuotaPlan{
		PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 9_500,
		BudgetQuotaSnapshot: mustQuota(t, "1000"), TotalSpendQuota: mustQuota(t, "100"),
		ManagedBalanceQuota: mustQuota(t, "500"),
	}
	items := []model.QuotaItem{
		{
			Action: model.QuotaAdjustmentActionIncrease, AdjustmentQuota: mustQuota(t, "100"),
			CalculationData: `{"base_quota":"30000000","bonus_quota":"10000000","weighted_pool_quota":"10000000"}`,
		},
		{
			Action: model.QuotaAdjustmentActionDecrease, AdjustmentQuota: -mustQuota(t, "100"),
			CalculationData: `{"decrease_kind":"ordinary"}`,
		},
	}

	summary, err := summarizePlan(plan, items)
	require.NoError(t, err)
	assert.Equal(t, CategorySummary{Count: 1, Total: mustQuota(t, "60")}, summary.BaseIncrease)
	assert.Equal(t, CategorySummary{Count: 1, Total: mustQuota(t, "20")}, summary.Bonus)
	assert.Equal(t, CategorySummary{Count: 1, Total: mustQuota(t, "20")}, summary.WeightedPool)
	assert.Equal(t, CategorySummary{Count: 1, Total: mustQuota(t, "100")}, summary.Increase)
	assert.Equal(t, CategorySummary{Count: 1, Total: mustQuota(t, "100")}, summary.OrdinaryDecrease)
	assert.Equal(t, mustQuota(t, "100"), summary.ReclaimedUsedForIncreases)
	assert.Equal(t, int64(0), summary.StageSourceTotal)
	assert.Equal(t, mustQuota(t, "600"), summary.OccupiedAfter)
}

func TestRegeneratePlanCreatesDraftAndCancelsOriginalTogether(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 7*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive, CreatedBy: "root",
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&model.User{
		Username: "alice", AffCode: "alice-code", Status: common.UserStatusEnabled,
		CreatedAt: now - daySeconds,
	}).Error)
	nextAdjustment := now + daySeconds
	original, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		NextAdjustmentAt: &nextAdjustment, BasisMode: basisModeActual, CreatedBy: "root",
	})
	require.NoError(t, err)

	regenerated, err := RegeneratePlan(original.Plan.Id, "root")
	require.NoError(t, err)
	require.NotEqual(t, original.Plan.Id, regenerated.Plan.Id)

	var storedOriginal model.QuotaPlan
	require.NoError(t, db.First(&storedOriginal, original.Plan.Id).Error)
	assert.Equal(t, model.QuotaPlanStatusCancelled, storedOriginal.Status)
	assert.Contains(t, storedOriginal.CancelReason, "重新生成方案")
}

func TestRegenerateLegacyPlanUsesSourceDefaults(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 7*daySeconds, CycleEndAt: now + 14*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		Status: model.QuotaPlanStatusExecuted,
	}).Error)
	legacy := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 7_500,
		Parameters: `{}`, Status: model.QuotaPlanStatusCancelled,
	}
	require.NoError(t, db.Create(&legacy).Error)

	regenerated, err := RegeneratePlan(legacy.Id, "root")
	require.NoError(t, err)

	var parameters PlanParameters
	require.NoError(t, common.Unmarshal([]byte(regenerated.Plan.Parameters), &parameters))
	assert.Equal(t, basisModeActual, parameters.BasisMode)
	assert.Equal(t, 30, parameters.ReclaimCapPercent)
	assert.Equal(t, 30, parameters.UsageBonusPercent)
}

func TestActivateDueCycleClosesExpiredSchedules(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycles := []model.QuotaCycle{
		{
			CycleStartAt: now - 20*daySeconds, CycleEndAt: now - 10*daySeconds,
			BudgetQuota: 1, InitialGrantQuota: 1, Status: model.QuotaCycleStatusScheduled,
		},
		{
			CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
			BudgetQuota: 1, InitialGrantQuota: 1, Status: model.QuotaCycleStatusScheduled,
		},
	}
	require.NoError(t, db.Create(&cycles).Error)
	expiredPlan := model.QuotaPlan{
		CycleId: cycles[0].Id, PlanType: model.QuotaPlanTypeInitialization,
		Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&expiredPlan).Error)

	require.NoError(t, activateDueCycle(now))
	var stored []model.QuotaCycle
	require.NoError(t, db.Order("id").Find(&stored).Error)
	require.Len(t, stored, 2)
	assert.Equal(t, model.QuotaCycleStatusClosed, stored[0].Status)
	assert.Equal(t, model.QuotaCycleStatusActive, stored[1].Status)
	require.Nil(t, stored[0].ActiveKey)
	require.NotNil(t, stored[1].ActiveKey)
	require.NoError(t, db.First(&expiredPlan, expiredPlan.Id).Error)
	assert.Equal(t, model.QuotaPlanStatusCancelled, expiredPlan.Status)
	assert.Equal(t, "采购周期已结束", expiredPlan.CancelReason)
}
