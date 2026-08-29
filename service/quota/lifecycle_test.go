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

func configureLifecycleTest(t *testing.T) {
	t.Helper()
	if common.RDB == nil || !common.RedisEnabled {
		configureQuotaLeaseRedis(t)
	}
	setting := operation_setting.GetQuotaSetting()
	previousMode := operation_setting.CycleQuotaManagementEnabled
	previousLead := setting.SettlementLeadMinutes
	operation_setting.CycleQuotaManagementEnabled = true
	setting.SettlementLeadMinutes = 10
	t.Cleanup(func() {
		operation_setting.CycleQuotaManagementEnabled = previousMode
		setting.SettlementLeadMinutes = previousLead
	})
}

func TestSettleCycleResetsOnlyManagedBalances(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "60")
	baseline := now - 30*daySeconds
	cycle := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000,
		OpeningAllocatedQuota:   &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &baseline,
		BalancePolicy: model.QuotaCycleBalancePolicyReset, Status: model.QuotaCycleStatusSettling,
	}
	users := []model.User{
		{Username: "managed-reset", AffCode: "managed-reset", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "80"))},
		{Username: "managed-negative-reset", AffCode: "managed-negative-reset", Status: common.UserStatusEnabled, Quota: -int(mustQuota(t, "20"))},
		{Username: "whitelist-reset", AffCode: "whitelist-reset", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "200")), QuotaWhitelist: true},
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&users).Error)

	draft := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		AlgorithmVersion: AlgorithmVersion, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&draft).Error)

	err := settleCycle(cycle.Id, now, "system")
	require.NoError(t, err)

	var storedCycle model.QuotaCycle
	var storedUsers []model.User
	require.NoError(t, db.First(&storedCycle, cycle.Id).Error)
	require.NoError(t, db.Order("id").Find(&storedUsers).Error)
	require.NoError(t, db.First(&draft, draft.Id).Error)
	assert.Equal(t, model.QuotaCycleStatusClosed, storedCycle.Status)
	require.NotNil(t, storedCycle.SettledAt)
	var settlementPlan model.QuotaPlan
	require.NoError(t, db.Where("cycle_id = ? AND plan_type = ?", cycle.Id, model.QuotaPlanTypeSettlement).
		First(&settlementPlan).Error)
	assert.Equal(t, model.QuotaPlanStatusExecuted, settlementPlan.Status)
	assert.Zero(t, storedUsers[0].Quota)
	assert.Zero(t, storedUsers[1].Quota)
	assert.Equal(t, int(mustQuota(t, "200")), storedUsers[2].Quota)
	assert.Equal(t, model.QuotaPlanStatusCancelled, draft.Status)
	assert.NotEmpty(t, draft.CancelReason)
}

func TestSettleCycleIsIdempotent(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusSettling,
	}
	require.NoError(t, db.Create(&cycle).Error)

	err := settleCycle(cycle.Id, now, "system")
	require.NoError(t, err)
	err = settleCycle(cycle.Id, now+1, "system")
	require.NoError(t, err)
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	assert.Equal(t, model.QuotaCycleStatusClosed, cycle.Status)
	assert.Equal(t, now, *cycle.SettledAt)
}

func TestQuotaSettlementEntrypointsRejectBatchUpdatedBalances(t *testing.T) {
	tests := []struct {
		name   string
		status model.QuotaCycleStatus
		close  func(int, int64) error
	}{
		{name: "direct settlement", status: model.QuotaCycleStatusSettling, close: func(cycleID int, now int64) error {
			return settleCycle(cycleID, now, "system")
		}},
		{name: "drained settlement", status: model.QuotaCycleStatusSettling, close: settleDrainedCycle},
		{name: "admin close", status: model.QuotaCycleStatusActive, close: func(cycleID int, _ int64) error {
			_, err := CloseCycle(cycleID, "admin")
			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupQuotaTestDB(t)
			now := time.Now().Unix()
			cycle := model.QuotaCycle{
				CycleStartAt: now - 60, CycleEndAt: now + 60, BudgetQuota: 1000,
				BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: test.status,
			}
			require.NoError(t, db.Create(&cycle).Error)
			common.BatchUpdateEnabled = true

			err := test.close(cycle.Id, now)

			require.ErrorIs(t, err, errQuotaBatchUpdateEnabled)
			require.NoError(t, db.First(&cycle, cycle.Id).Error)
			assert.Equal(t, test.status, cycle.Status)
		})
	}
}

func TestRunQuotaLifecycleWaitsForAsyncTasksAcrossCycleEnd(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 3600, CycleEndAt: now - 1,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusSettling,
	}
	user := model.User{Username: "async-settlement", AffCode: "async-settlement", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	task := model.Task{
		UserId: user.Id, TaskID: "task-settlement", SubmitTime: now - 300,
		Status: model.TaskStatusInProgress, Progress: "50%",
	}
	midjourney := model.Midjourney{
		UserId: user.Id, MjId: "mj-settlement", SubmitTime: (now - 200) * 1000,
		Status: string(model.TaskStatusInProgress), Progress: "50%",
	}
	legacyMidjourney := model.Midjourney{
		UserId: user.Id, MjId: "mj-settlement-seconds", SubmitTime: now - 100,
		Status: string(model.TaskStatusInProgress), Progress: "50%",
	}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Create(&midjourney).Error)
	require.NoError(t, db.Create(&legacyMidjourney).Error)

	result, err := RunQuotaLifecycle(now)
	require.NoError(t, err)
	assert.Empty(t, result.ClosedCycleIDs)
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	assert.Equal(t, model.QuotaCycleStatusSettling, cycle.Status)

	require.NoError(t, db.Model(&task).Updates(map[string]interface{}{
		"status": model.TaskStatusSuccess, "progress": "100%",
	}).Error)
	require.NoError(t, db.Model(&midjourney).Updates(map[string]interface{}{
		"status": string(model.TaskStatusSuccess), "progress": "100%",
	}).Error)
	result, err = RunQuotaLifecycle(now + 1)
	require.NoError(t, err)
	assert.Empty(t, result.ClosedCycleIDs)
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	assert.Equal(t, model.QuotaCycleStatusSettling, cycle.Status)

	require.NoError(t, db.Model(&legacyMidjourney).Updates(map[string]interface{}{
		"status": string(model.TaskStatusSuccess), "progress": "100%",
	}).Error)
	result, err = RunQuotaLifecycle(now + 2)
	require.NoError(t, err)
	assert.Equal(t, []int{cycle.Id}, result.ClosedCycleIDs)
}

func TestActivateScheduledCycleRejectsBudgetBelowCarriedPosition(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "50"), InitialGrantQuota: mustQuota(t, "10"),
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusScheduled,
	}
	user := model.User{Username: "carry-over-budget", AffCode: "carry-over-budget", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "80"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	_, _, err := activateScheduledCycle(now, "system")
	require.EqualError(t, err, "当前受管账户可用余额合计超过周期总额度，超出金额为 ＄30.000000")
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	assert.Equal(t, model.QuotaCycleStatusScheduled, cycle.Status)
}

func TestRunQuotaLifecycleDoesNotGenerateInitialAllocationPlan(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t)
	now := time.Now().Unix()
	allocated := int64(0)
	baseline := now - daySeconds
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &baseline,
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "draft-only", AffCode: "draft-only", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	result, err := RunQuotaLifecycle(now)
	require.NoError(t, err)
	assert.Nil(t, result.ActivatedCycleID)
	var plans []model.QuotaPlan
	require.NoError(t, db.Where("cycle_id = ?", cycle.Id).Find(&plans).Error)
	assert.Empty(t, plans)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Zero(t, user.Quota)
}

func TestActivateScheduledCycleCountsOnlyAvailableBalances(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusScheduled,
	}
	users := []model.User{
		{Username: "positive-balance", AffCode: "positive-balance", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "80"))},
		{Username: "negative-balance", AffCode: "negative-balance", Status: common.UserStatusEnabled, Quota: -int(mustQuota(t, "20"))},
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&users).Error)

	active, activated, err := activateScheduledCycle(now, "system")
	require.NoError(t, err)
	require.True(t, activated)
	require.NotNil(t, active)
	require.NotNil(t, active.OpeningAllocatedQuota)
	require.NotNil(t, active.AllocatedQuota)
	assert.Equal(t, mustQuota(t, "80"), *active.OpeningAllocatedQuota)
	assert.Equal(t, mustQuota(t, "80"), *active.AllocatedQuota)
}
