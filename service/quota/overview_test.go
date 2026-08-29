package quota

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaOverviewUsesExecutedPlanAndIgnoresCancelledFutureCycle(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "100")
	cycles := []model.QuotaCycle{
		{
			CycleStartAt: now - 20*daySeconds, CycleEndAt: now - 10*daySeconds,
			BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
			Status: model.QuotaCycleStatusClosed,
		},
		{
			CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
			BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
			OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
			Status: model.QuotaCycleStatusActive,
		},
		{
			CycleStartAt: now + 2*daySeconds, CycleEndAt: now + 3*daySeconds,
			BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
			Status: model.QuotaCycleStatusClosed,
		},
	}
	require.NoError(t, db.Create(&cycles).Error)
	executedAt := now - 30
	plans := []model.QuotaPlan{
		{
			CycleId: cycles[1].Id, Name: "已执行方案", PlanType: model.QuotaPlanTypeAdjustment,
			BudgetQuotaSnapshot: cycles[1].BudgetQuota, AllocationBeforeQuota: &allocated,
			Status: model.QuotaPlanStatusExecuted, CreatedAt: now - 60, ExecutedAt: &executedAt,
		},
		{
			CycleId: cycles[1].Id, Name: "已取消方案", PlanType: model.QuotaPlanTypeAdjustment,
			BudgetQuotaSnapshot: cycles[1].BudgetQuota, AllocationBeforeQuota: &allocated,
			Status: model.QuotaPlanStatusCancelled, CreatedAt: now,
		},
	}
	require.NoError(t, db.Create(&plans).Error)

	overview, err := GetQuotaOverview()
	require.NoError(t, err)
	require.NotNil(t, overview.PreviousCycle)
	assert.Equal(t, cycles[0].Id, overview.PreviousCycle.Id)
	require.NotNil(t, overview.CurrentPlan)
	assert.Equal(t, plans[0].Id, overview.CurrentPlan.Id)
}

func TestQuotaOverviewReportsInvalidAllocationSnapshotWithoutHidingPage(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	opening := mustQuota(t, "50")
	allocated := mustQuota(t, "101")
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
		BudgetQuota: mustQuota(t, "100"), InitialGrantQuota: mustQuota(t, "10"),
		OpeningAllocatedQuota: &opening, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)

	overview, err := GetQuotaOverview()
	require.NoError(t, err)
	require.NotNil(t, overview.CurrentCycle)
	assert.False(t, overview.AllocationReady)
	require.EqualError(t, CycleAllocationError(overview.CurrentCycle), "周期已发放额度不能超过周期总额度")
}

func TestCycleAllocationErrorDistinguishesUninitializedAndPartialData(t *testing.T) {
	budget := mustQuota(t, "100")
	opening := mustQuota(t, "50")
	assert.NoError(t, CycleAllocationError(&model.QuotaCycle{BudgetQuota: budget}))
	require.EqualError(t, CycleAllocationError(&model.QuotaCycle{
		BudgetQuota: budget, OpeningAllocatedQuota: &opening,
	}), "周期额度记录不完整")
}
