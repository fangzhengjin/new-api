package quota

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func configureLifecycleTest(t *testing.T, autoExecute bool, generationTime string) {
	t.Helper()
	setting := operation_setting.GetQuotaSetting()
	previousMode := operation_setting.CompanyQuotaModeEnabled
	previousAutoExecute := setting.AutoExecuteQuotaInitialization
	previousTime := setting.QuotaInitializationTime
	operation_setting.CompanyQuotaModeEnabled = true
	setting.AutoExecuteQuotaInitialization = autoExecute
	setting.QuotaInitializationTime = generationTime
	t.Cleanup(func() {
		operation_setting.CompanyQuotaModeEnabled = previousMode
		setting.AutoExecuteQuotaInitialization = previousAutoExecute
		setting.QuotaInitializationTime = previousTime
	})
}

func TestSettleCycleResetClearsManagedBalancesAndPreservesWhitelist(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyReset, Status: model.QuotaCycleStatusActive,
	}
	users := []model.User{
		{Username: "managed-reset", AffCode: "managed-reset", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "80"))},
		{Username: "managed-negative-reset", AffCode: "managed-negative-reset", Status: common.UserStatusEnabled, Quota: -int(mustQuota(t, "20"))},
		{Username: "whitelist-reset", AffCode: "whitelist-reset", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "200")), QuotaWhitelist: true},
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&users).Error)

	result, err := settleCycle(cycle.Id, now, "system")
	require.NoError(t, err)
	require.NotZero(t, result.planID)

	var storedCycle model.QuotaCycle
	var storedUsers []model.User
	var items []model.QuotaItem
	require.NoError(t, db.First(&storedCycle, cycle.Id).Error)
	require.NoError(t, db.Order("id").Find(&storedUsers).Error)
	require.NoError(t, db.Where("plan_id = ?", result.planID).Find(&items).Error)
	assert.Equal(t, model.QuotaCycleStatusClosed, storedCycle.Status)
	require.NotNil(t, storedCycle.SettledAt)
	require.NotNil(t, storedCycle.SettlementPlanId)
	assert.Zero(t, storedUsers[0].Quota)
	assert.Zero(t, storedUsers[1].Quota)
	assert.Equal(t, int(mustQuota(t, "200")), storedUsers[2].Quota)
	require.Len(t, items, 2)
	require.NotNil(t, items[0].ActualBeforeQuota)
	require.NotNil(t, items[0].ActualAfterQuota)
	assert.Equal(t, mustQuota(t, "80"), *items[0].ActualBeforeQuota)
	assert.Zero(t, *items[0].ActualAfterQuota)
	assert.Equal(t, model.QuotaAdjustmentActionIncrease, items[1].Action)
	assert.Equal(t, -mustQuota(t, "20"), *items[1].ActualBeforeQuota)
	assert.Zero(t, *items[1].ActualAfterQuota)
}

func TestSettleCycleCarryLeavesManagedBalanceUnchanged(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusActive,
	}
	users := []model.User{
		{Username: "managed-carry", AffCode: "managed-carry", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "80"))},
		{Username: "managed-negative-carry", AffCode: "managed-negative-carry", Status: common.UserStatusEnabled, Quota: -int(mustQuota(t, "20"))},
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&users).Error)

	result, err := settleCycle(cycle.Id, now, "system")
	require.NoError(t, err)
	assert.Zero(t, result.planID)
	require.NoError(t, db.Order("id").Find(&users).Error)
	assert.Equal(t, int(mustQuota(t, "80")), users[0].Quota)
	assert.Equal(t, -int(mustQuota(t, "20")), users[1].Quota)
}

func TestSettleCycleResetRollsBackEveryBalanceWhenOneUpdateFails(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyReset, Status: model.QuotaCycleStatusActive,
	}
	users := []model.User{
		{Username: "rollback-first", AffCode: "rollback-first", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "80"))},
		{Username: "rollback-second", AffCode: "rollback-second", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "60"))},
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&users).Error)

	updates := 0
	callbackName := "quota:settlement-test-failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "users" {
			return
		}
		updates++
		if updates == 2 {
			tx.AddError(errors.New("injected user update failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })

	_, err := settleCycle(cycle.Id, now, "system")
	require.ErrorContains(t, err, "injected user update failure")
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	require.NoError(t, db.Order("id").Find(&users).Error)
	assert.Equal(t, model.QuotaCycleStatusActive, cycle.Status)
	assert.Equal(t, int(mustQuota(t, "80")), users[0].Quota)
	assert.Equal(t, int(mustQuota(t, "60")), users[1].Quota)
	var settlementPlans int64
	require.NoError(t, db.Model(&model.QuotaPlan{}).
		Where("cycle_id = ? AND plan_type = ?", cycle.Id, model.QuotaPlanTypeSettlement).
		Count(&settlementPlans).Error)
	assert.Zero(t, settlementPlans)
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
	require.EqualError(t, err, "后续周期预算低于激活时受管头寸")
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	assert.Equal(t, model.QuotaCycleStatusScheduled, cycle.Status)
}

func TestRunQuotaLifecycleGeneratesDraftWithoutAutomaticConfirmation(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t, false, "00:00")
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "draft-only", AffCode: "draft-only", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	first, err := RunQuotaLifecycle(now)
	require.NoError(t, err)
	require.NotNil(t, first.GeneratedPlanID)
	assert.Nil(t, first.ExecutedPlanID)

	second, err := RunQuotaLifecycle(now + 60)
	require.NoError(t, err)
	assert.Nil(t, second.GeneratedPlanID)
	var plans []model.QuotaPlan
	require.NoError(t, db.Where("cycle_id = ? AND plan_type = ?", cycle.Id, model.QuotaPlanTypeInitialization).Find(&plans).Error)
	require.Len(t, plans, 1)
	assert.Equal(t, model.QuotaPlanStatusDraft, plans[0].Status)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Zero(t, user.Quota)
}

func TestRunQuotaLifecycleAutomaticallyExecutesOnlyGeneratedInitialization(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t, true, "00:00")
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "auto-execute", AffCode: "auto-execute", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	result, err := RunQuotaLifecycle(now)
	require.NoError(t, err)
	require.NotNil(t, result.GeneratedPlanID)
	require.NotNil(t, result.ExecutedPlanID)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "100")), user.Quota)

	var plan model.QuotaPlan
	require.NoError(t, db.First(&plan, *result.GeneratedPlanID).Error)
	assert.Equal(t, model.QuotaPlanStatusExecuted, plan.Status)
}

func TestRunQuotaLifecycleExecutesEmptyInitialization(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t, true, "00:00")
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusActive,
	}
	user := model.User{
		Username: "already-initialized", AffCode: "already-initialized",
		Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "100")),
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	result, err := RunQuotaLifecycle(now)
	require.NoError(t, err)
	require.NotNil(t, result.ExecutedPlanID)
	var plan model.QuotaPlan
	require.NoError(t, db.First(&plan, *result.ExecutedPlanID).Error)
	assert.Equal(t, model.QuotaPlanStatusExecuted, plan.Status)
	var items int64
	require.NoError(t, db.Model(&model.QuotaItem{}).Where("plan_id = ?", plan.Id).Count(&items).Error)
	assert.Zero(t, items)
}

func TestRestoreCycleSettlementPreservesSignedNetZeroAndEnforcesActiveBudget(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t, false, "09:00")
	now := time.Now().Unix()
	source := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now - 1,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyReset, Status: model.QuotaCycleStatusActive,
	}
	users := []model.User{
		{Username: "restore-positive", AffCode: "restore-positive", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "80"))},
		{Username: "restore-negative", AffCode: "restore-negative", Status: common.UserStatusEnabled, Quota: -int(mustQuota(t, "80"))},
	}
	require.NoError(t, db.Create(&source).Error)
	require.NoError(t, db.Create(&users).Error)
	_, err := settleCycle(source.Id, now, "system")
	require.NoError(t, err)
	target := model.QuotaCycle{
		CycleStartAt: now, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "50"), InitialGrantQuota: mustQuota(t, "10"),
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&model.QuotaCycleSettlement{
		BusinessKey: "restore-net-zero-spend", CycleId: target.Id, UserId: users[0].Id,
		BillingAt: now, Quota: mustQuota(t, "60"), UpdatedAt: now,
	}).Error)

	_, err = RestoreCycleSettlement(source.Id, "root")
	require.EqualError(t, err, "活跃周期预算不足以恢复清零快照")
	require.NoError(t, db.Order("id").Find(&users).Error)
	assert.Zero(t, users[0].Quota)
	assert.Zero(t, users[1].Quota)
	require.NoError(t, db.First(&source, source.Id).Error)
	assert.Nil(t, source.RestoredAt)

	require.NoError(t, db.Model(&target).Update("budget_quota", mustQuota(t, "1000")).Error)

	result, err := RestoreCycleSettlement(source.Id, "root")
	require.NoError(t, err)
	assert.Equal(t, 2, result.AffectedUsers)
	require.NoError(t, db.Order("id").Find(&users).Error)
	assert.Equal(t, int(mustQuota(t, "80")), users[0].Quota)
	assert.Equal(t, -int(mustQuota(t, "80")), users[1].Quota)
	require.NoError(t, db.First(&source, source.Id).Error)
	require.NotNil(t, source.RestoredAt)

	_, err = RestoreCycleSettlement(source.Id, "root")
	require.EqualError(t, err, "该周期清零快照已经恢复")
}

func TestRestoreCycleSettlementRestoresSignedSnapshotThatLowersPosition(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t, false, "09:00")
	now := time.Now().Unix()
	source := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now - 1,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyReset, Status: model.QuotaCycleStatusActive,
	}
	positiveUser := model.User{Username: "restore-positive-net", AffCode: "restore-positive-net", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "20"))}
	negativeUser := model.User{Username: "restore-negative-net", AffCode: "restore-negative-net", Status: common.UserStatusEnabled, Quota: -int(mustQuota(t, "40"))}
	require.NoError(t, db.Create(&source).Error)
	require.NoError(t, db.Create(&positiveUser).Error)
	require.NoError(t, db.Create(&negativeUser).Error)
	_, err := settleCycle(source.Id, now, "system")
	require.NoError(t, err)
	require.NoError(t, db.First(&positiveUser, positiveUser.Id).Error)
	require.NoError(t, db.First(&negativeUser, negativeUser.Id).Error)
	assert.Zero(t, positiveUser.Quota)
	assert.Zero(t, negativeUser.Quota)

	target := model.QuotaCycle{
		CycleStartAt: now, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&model.QuotaCycleSettlement{
		BusinessKey: "restore-signed-net-spend", CycleId: target.Id, UserId: positiveUser.Id,
		BillingAt: now, Quota: mustQuota(t, "100"), UpdatedAt: now,
	}).Error)

	result, err := RestoreCycleSettlement(source.Id, "root")
	require.NoError(t, err)
	assert.Equal(t, 2, result.AffectedUsers)
	require.NoError(t, db.First(&positiveUser, positiveUser.Id).Error)
	require.NoError(t, db.First(&negativeUser, negativeUser.Id).Error)
	assert.Equal(t, int(mustQuota(t, "20")), positiveUser.Quota)
	assert.Equal(t, -int(mustQuota(t, "40")), negativeUser.Quota)

	var item model.QuotaItem
	require.NoError(t, db.Where("user_id = ?", negativeUser.Id).Order("id DESC").First(&item).Error)
	assert.Equal(t, model.QuotaAdjustmentActionRestore, item.Action)
	assert.Equal(t, -mustQuota(t, "40"), item.AdjustmentQuota)
	assert.Contains(t, item.LogContent, "本次恢复调减")
}
