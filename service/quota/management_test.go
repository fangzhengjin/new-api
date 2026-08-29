package quota

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidateTemporaryQuotaAutoApprovalPolicyRequiresCompleteBoundedConfiguration(t *testing.T) {
	require.NoError(t, validateTemporaryQuotaAutoApprovalPolicy(TemporaryQuotaAutoApprovalPolicy{}, mustQuota(t, "100")))
	require.EqualError(t, validateTemporaryQuotaAutoApprovalPolicy(TemporaryQuotaAutoApprovalPolicy{SingleQuota: 1}, mustQuota(t, "100")), "关闭自动发放时额度和次数设置必须为0")
	policy := TemporaryQuotaAutoApprovalPolicy{
		Enabled: true, SingleQuota: mustQuota(t, "10"), ThresholdQuota: mustQuota(t, "5"),
		MaxCount: 2, MaxQuota: mustQuota(t, "20"),
	}
	require.EqualError(t, validateTemporaryQuotaAutoApprovalPolicy(policy, 0), "开启自动发放前必须配置正数临时额度预留")
	require.NoError(t, validateTemporaryQuotaAutoApprovalPolicy(policy, mustQuota(t, "100")))
	policy.MaxQuota = mustQuota(t, "9")
	require.EqualError(t, validateTemporaryQuotaAutoApprovalPolicy(policy, mustQuota(t, "100")), "自动发放每用户总额上限必须不小于单次上限且不超过单用户额度上限")
}

func TestParseTemporaryQuotaAutoApprovalPolicy(t *testing.T) {
	policy, err := ParseTemporaryQuotaAutoApprovalPolicy(true, "10", "20", 2, "30")
	require.NoError(t, err)
	require.Equal(t, TemporaryQuotaAutoApprovalPolicy{Enabled: true, SingleQuota: 10, ThresholdQuota: 20, MaxCount: 2, MaxQuota: 30}, policy)

	policy, err = ParseTemporaryQuotaAutoApprovalPolicy(false, "bad", "bad", 0, "bad")
	require.NoError(t, err)
	require.Equal(t, TemporaryQuotaAutoApprovalPolicy{}, policy)

	_, err = ParseTemporaryQuotaAutoApprovalPolicy(true, "0", "20", 2, "30")
	require.EqualError(t, err, "自动发放单次上限必须是正整数额度")
}

func TestValidateInitialGrantQuotaRejectsUnsupportedUserBalance(t *testing.T) {
	require.NoError(t, validateInitialGrantQuota(int64(common.MaxQuota)+1))
	require.EqualError(
		t,
		validateInitialGrantQuota(int64(common.MaxWalletQuota)+1),
		"人均首次额度超出单用户可支持范围",
	)
}

func TestTemporaryQuotaReserveValidationUsesCycleBudget(t *testing.T) {
	t.Parallel()
	budget := mustQuota(t, "1000")
	require.NoError(t, validateTemporaryQuotaReserve(budget, 0))
	require.NoError(t, validateTemporaryQuotaReserve(budget, mustQuota(t, "999.99")))
	assert.EqualError(t, validateTemporaryQuotaReserve(budget, budget), "临时额度预留必须大于等于0且小于周期总额度")
}

func TestActiveCycleCannotChangeTemporaryQuotaReserve(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		TemporaryQuotaReserve: mustQuota(t, "50"), Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	updatedReserve := mustQuota(t, "60")

	err := UpdateCycleSettings(cycle.Id, cycle.BudgetQuota, nil, &updatedReserve, nil, "root")
	require.EqualError(t, err, "只有已规划周期可以修改临时额度预留")

	var stored model.QuotaCycle
	require.NoError(t, db.First(&stored, cycle.Id).Error)
	assert.Equal(t, mustQuota(t, "50"), stored.TemporaryQuotaReserve)
}

func TestCreateCycleRejectsOverlappingWindow(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	_, err := CreateCycle(CreateCycleParams{
		StartAt: now, EndAt: now + 10*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000,
		BalancePolicy:           model.QuotaCycleBalancePolicyReset,
		CreatedBy:               "root",
	})
	require.NoError(t, err)

	_, err = CreateCycle(CreateCycleParams{
		StartAt: now + daySeconds, EndAt: now + 11*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000,
		BalancePolicy:           model.QuotaCycleBalancePolicyReset,
		CreatedBy:               "root",
	})
	require.EqualError(t, err, "周期时间与现有周期冲突")

	var count int64
	require.NoError(t, db.Model(&model.QuotaCycle{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestListCyclesDefaultsToNextAvailableStart(t *testing.T) {
	setupQuotaTestDB(t)

	result, err := ListCycles()
	require.NoError(t, err)
	assert.Positive(t, result.Defaults.StartAt)
}

func TestUpdateActiveCycleRejectsBudgetBelowAllocatedQuota(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	now := time.Now().Unix()
	allocated := mustQuota(t, "100")
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed", AffCode: "managed-budget", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "100"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	err := UpdateCycleSettings(cycle.Id, mustQuota(t, "50"), nil, nil, nil, "root")
	require.EqualError(t, err, "周期总额度不能低于当前已发放额度")

	var stored model.QuotaCycle
	require.NoError(t, db.First(&stored, cycle.Id).Error)
	assert.Equal(t, cycle.BudgetQuota, stored.BudgetQuota)
}

func TestUpdateSettlingCycleRejectsBudgetBelowAllocatedQuotaAndStillCloses(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "100")
	cycle := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now - 60,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		BalancePolicy: model.QuotaCycleBalancePolicyReset, Status: model.QuotaCycleStatusSettling,
	}
	user := model.User{
		Username: "settling-managed", AffCode: "settling-managed-budget",
		Status: common.UserStatusEnabled, Quota: int(allocated),
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	err := UpdateCycleSettings(cycle.Id, mustQuota(t, "50"), nil, nil, nil, "root")
	require.EqualError(t, err, "周期总额度不能低于当前已发放额度")
	require.NoError(t, settleCycle(cycle.Id, now, "root"))

	var storedCycle model.QuotaCycle
	require.NoError(t, db.First(&storedCycle, cycle.Id).Error)
	assert.Equal(t, cycle.BudgetQuota, storedCycle.BudgetQuota)
	assert.Equal(t, model.QuotaCycleStatusClosed, storedCycle.Status)
	var storedUser model.User
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	assert.Zero(t, storedUser.Quota)
}

func TestCreateCycleRequiresAndPersistsConcentrationMultiplier(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	params := CreateCycleParams{
		StartAt: now + daySeconds, EndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyReset, CreatedBy: "root",
	}

	_, err := CreateCycle(params)
	require.EqualError(t, err, "自动分配上限倍率必须是1.5、2或3")
	params.ConcentrationMultiplier = 20_000
	cycle, err := CreateCycle(params)
	require.NoError(t, err)
	assert.Equal(t, int64(20_000), cycle.ConcentrationMultiplier)
	var stored model.QuotaCycle
	require.NoError(t, db.First(&stored, cycle.Id).Error)
	assert.Equal(t, int64(20_000), stored.ConcentrationMultiplier)
}

func TestCreateCycleRejectsReplayWhenCycleQuotaManagementIsDisabled(t *testing.T) {
	db := setupQuotaTestDB(t)
	operation_setting.CycleQuotaManagementEnabled = false
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", model.CycleQuotaManagementOptionKey).Update("value", "false").Error)
	now := time.Now().Unix()

	_, err := CreateCycle(CreateCycleParams{
		StartAt: now + daySeconds, EndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000,
		BalancePolicy:           model.QuotaCycleBalancePolicyReset,
		CreatedBy:               "root",
	})
	require.EqualError(t, err, "周期额度管理未启用")

	var count int64
	require.NoError(t, db.Model(&model.QuotaCycle{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreateCycleRechecksPersistedCycleQuotaManagement(t *testing.T) {
	db := setupQuotaTestDB(t)
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", model.CycleQuotaManagementOptionKey).Update("value", "false").Error)
	operation_setting.CycleQuotaManagementEnabled = true
	now := time.Now().Unix()

	_, err := CreateCycle(CreateCycleParams{
		StartAt: now + daySeconds, EndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000,
		BalancePolicy:           model.QuotaCycleBalancePolicyReset,
		CreatedBy:               "root",
	})
	require.EqualError(t, err, "周期额度管理未启用")

	var count int64
	require.NoError(t, db.Model(&model.QuotaCycle{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestManualIncreaseUsesCycleAllocationWithoutPlanConfirmation(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	now := time.Now().Unix()
	allocated := mustQuota(t, "50")
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-stage", AffCode: "managed-stage", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "50"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	result, err := ManualAdjustUserQuota(user.Id, mustQuota(t, "150"), "业务需要", "root")
	require.NoError(t, err)
	assert.Positive(t, result.PlanID)
	assert.Equal(t, 1, result.AffectedUsers)
	assert.Equal(t, mustQuota(t, "150"), result.IssuedQuota)
	assert.Zero(t, result.ReclaimedQuota)

	var storedUser model.User
	var item model.QuotaItem
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	require.NoError(t, db.Order("id DESC").First(&item).Error)
	assert.Equal(t, int(mustQuota(t, "200")), storedUser.Quota)
	assert.Equal(t, model.QuotaAdjustmentActionIncrease, item.Action)
	assert.Equal(t, mustQuota(t, "150"), item.AdjustmentQuota)
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	require.NotNil(t, cycle.AllocatedQuota)
	assert.Equal(t, mustQuota(t, "200"), *cycle.AllocatedQuota)
}

func TestManualIncreaseCannotExceedCycleBudget(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	now := time.Now().Unix()
	allocated := mustQuota(t, "90")
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "100"), InitialGrantQuota: mustQuota(t, "10"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-budget", AffCode: "managed-budget-limit", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "90"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	_, err := ManualAdjustUserQuota(user.Id, mustQuota(t, "20"), "业务需要", "root")
	require.EqualError(t, err, "本次发放超过周期总额度，超出金额为 ＄10.000000")

	var plans int64
	require.NoError(t, db.Model(&model.QuotaPlan{}).Count(&plans).Error)
	assert.Zero(t, plans)
}

func TestManualIncreaseMayLeaveBalanceNegative(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-negative", AffCode: "managed-negative", Status: common.UserStatusEnabled, Quota: -100}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	result, err := ManualAdjustUserQuota(user.Id, 30, "signed balance", "root")
	require.NoError(t, err)
	assert.Equal(t, 1, result.AffectedUsers)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, -70, user.Quota)

	var item model.QuotaItem
	require.NoError(t, db.Order("id DESC").First(&item).Error)
	assert.Equal(t, model.QuotaAdjustmentActionIncrease, item.Action)
	assert.Equal(t, int64(-70), *item.ActualAfterQuota)

	_, err = ManualAdjustUserQuota(user.Id, -1, "signed balance", "root")
	require.EqualError(t, err, "调减额度不能超过用户当前余额")
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, -70, user.Quota)
}

func TestManualIncreaseUsesFinalBalanceRange(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: int64(common.MaxWalletQuota), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-minimum", AffCode: "managed-minimum", Status: common.UserStatusEnabled, Quota: common.MinQuota}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	result, err := ManualAdjustUserQuota(user.Id, int64(common.MaxQuota)+1, "return to zero", "root")
	require.NoError(t, err)
	assert.Equal(t, 1, result.AffectedUsers)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Zero(t, user.Quota)
}

func TestManualDecreaseUsesAdjustmentAmount(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	now := time.Now().Unix()
	allocated := mustQuota(t, "100")
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-decrease", AffCode: "managed-decrease", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "100"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	result, err := ManualAdjustUserQuota(user.Id, -mustQuota(t, "30"), "manual decrease", "root")
	require.NoError(t, err)
	assert.Equal(t, 1, result.AffectedUsers)

	var storedUser model.User
	var item model.QuotaItem
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	require.NoError(t, db.Order("id DESC").First(&item).Error)
	assert.Equal(t, int(mustQuota(t, "70")), storedUser.Quota)
	assert.Equal(t, model.QuotaAdjustmentActionDecrease, item.Action)
	assert.Equal(t, -mustQuota(t, "30"), item.AdjustmentQuota)
}

func TestManualAdjustmentRejectsIneligibleDecreaseTargets(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive,
	}
	users := []model.User{
		{Username: "managed-low-balance", AffCode: "managed-low-balance", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "10"))},
		{Username: "managed-whitelist", AffCode: "managed-whitelist", Status: common.UserStatusEnabled, QuotaWhitelist: true, Quota: int(mustQuota(t, "100"))},
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&users).Error)

	_, err := ManualAdjustUserQuota(users[0].Id, -mustQuota(t, "20"), "too much", "root")
	require.EqualError(t, err, "调减额度不能超过用户当前余额")
	_, err = ManualAdjustUserQuota(users[1].Id, -mustQuota(t, "20"), "whitelist", "root")
	require.EqualError(t, err, "该用户在额度白名单中，不能手工调额，请先移出白名单")

	var plans int64
	require.NoError(t, db.Model(&model.QuotaPlan{}).Count(&plans).Error)
	assert.Zero(t, plans)
}

func TestManualAdjustmentRequiresReasonAndCountsCharacters(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "100"), InitialGrantQuota: mustQuota(t, "10"),
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-reason", AffCode: "managed-reason", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	_, err := ManualAdjustUserQuota(user.Id, 1, "", "root")
	require.EqualError(t, err, "调整原因不能为空")
	_, err = ManualAdjustUserQuota(user.Id, 1, strings.Repeat("调", 255), "root")
	require.NoError(t, err)
	_, err = ManualAdjustUserQuota(user.Id, 1, strings.Repeat("调", 256), "root")
	require.EqualError(t, err, "调整原因不得超过255个字符")
}

func TestManualAdjustmentRollsBackDraftWhenExecutionFails(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "100"), InitialGrantQuota: mustQuota(t, "10"),
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-rollback", AffCode: "managed-rollback", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail_manual_quota_update", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(errors.New("forced quota write failure"))
		}
	}))

	_, err := ManualAdjustUserQuota(user.Id, 1, "rollback execution", "root")
	require.ErrorContains(t, err, "forced quota write failure")

	var plans, items int64
	require.NoError(t, db.Model(&model.QuotaPlan{}).Count(&plans).Error)
	require.NoError(t, db.Model(&model.QuotaItem{}).Count(&items).Error)
	assert.Zero(t, plans)
	assert.Zero(t, items)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Zero(t, user.Quota)
}

func TestSetQuotaWhitelistChangesImmediatelyWithoutCurrentCycle(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	user := model.User{
		Username: "managed-no-cycle-whitelist", AffCode: "managed-no-cycle-whitelist",
		Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "25")),
	}
	require.NoError(t, db.Create(&user).Error)

	require.NoError(t, SetQuotaWhitelist(user.Id, true, "root"))
	require.NoError(t, SetQuotaWhitelist(user.Id, true, "root"))
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.True(t, user.QuotaWhitelist)
	assert.Equal(t, int(mustQuota(t, "25")), user.Quota)

	require.NoError(t, SetQuotaWhitelist(user.Id, false, "root"))
	require.NoError(t, SetQuotaWhitelist(user.Id, false, "root"))
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.False(t, user.QuotaWhitelist)
	assert.Equal(t, int(mustQuota(t, "25")), user.Quota)
}

func TestSetQuotaWhitelistRejectsReplayOutsideCycleQuotaManagement(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = false
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	user := model.User{
		Username: "normal-mode-whitelist", AffCode: "normal-mode-whitelist",
		Status: common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(&user).Error)

	require.EqualError(t, SetQuotaWhitelist(user.Id, true, "root"), "周期额度管理未启用，不能修改额度白名单")
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.False(t, user.QuotaWhitelist)
}
func TestSetQuotaWhitelistTransfersCurrentBalanceAcrossManagementScope(t *testing.T) {
	for _, status := range []model.QuotaCycleStatus{model.QuotaCycleStatusActive, model.QuotaCycleStatusSettling} {
		t.Run(string(status), func(t *testing.T) {
			db := setupQuotaTestDB(t)
			previousMode := operation_setting.CycleQuotaManagementEnabled
			operation_setting.CycleQuotaManagementEnabled = true
			t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
			now := time.Now().Unix()
			allocated := mustQuota(t, "100")
			cycle := model.QuotaCycle{
				CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
				BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
				OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
				Status: status,
			}
			user := model.User{
				Username: "managed-scope-transfer-" + string(status), AffCode: "managed-scope-transfer-" + string(status),
				Status: common.UserStatusEnabled, Quota: int(allocated),
			}
			require.NoError(t, db.Create(&cycle).Error)
			require.NoError(t, db.Create(&user).Error)
			require.NoError(t, db.Create(&model.QuotaPlan{
				CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
				Status: model.QuotaPlanStatusDraft,
			}).Error)

			require.NoError(t, SetQuotaWhitelist(user.Id, true, "root"))
			require.NoError(t, db.First(&cycle, cycle.Id).Error)
			require.NoError(t, db.First(&user, user.Id).Error)
			require.NotNil(t, cycle.AllocatedQuota)
			assert.Zero(t, *cycle.AllocatedQuota)
			assert.True(t, user.QuotaWhitelist)

			require.NoError(t, SetQuotaWhitelist(user.Id, false, "root"))
			require.NoError(t, db.First(&cycle, cycle.Id).Error)
			require.NoError(t, db.First(&user, user.Id).Error)
			require.NotNil(t, cycle.AllocatedQuota)
			assert.Equal(t, allocated, *cycle.AllocatedQuota)
			assert.False(t, user.QuotaWhitelist)

			var draft model.QuotaPlan
			require.NoError(t, db.Where("cycle_id = ?", cycle.Id).First(&draft).Error)
			assert.Equal(t, model.QuotaPlanStatusCancelled, draft.Status)
			assert.Equal(t, "额度管理范围发生变化", draft.CancelReason)
		})
	}
}

func TestSetQuotaWhitelistIgnoresNegativeBalanceInCycleAllocation(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })
	now := time.Now().Unix()
	allocated := mustQuota(t, "100")
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{
		Username: "managed-negative-whitelist", AffCode: "managed-negative-whitelist",
		Status: common.UserStatusEnabled, Quota: -int(mustQuota(t, "25")),
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	require.NoError(t, SetQuotaWhitelist(user.Id, true, "root"))
	require.NoError(t, SetQuotaWhitelist(user.Id, false, "root"))
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	require.NoError(t, db.First(&user, user.Id).Error)
	require.NotNil(t, cycle.AllocatedQuota)
	assert.Equal(t, allocated, *cycle.AllocatedQuota)
	assert.False(t, user.QuotaWhitelist)
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

func TestInitialGrantRecommendationExcludesWhitelistConsumption(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now - daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusClosed,
	}
	users := []model.User{
		{Username: "managed-recommendation", AffCode: "managed-recommendation", Status: common.UserStatusEnabled, CreatedAt: now - 60*daySeconds},
		{Username: "whitelist-recommendation", AffCode: "whitelist-recommendation", Status: common.UserStatusEnabled, QuotaWhitelist: true, CreatedAt: now - 60*daySeconds},
		{Username: "disabled-recommendation", AffCode: "disabled-recommendation", Status: common.UserStatusDisabled, CreatedAt: now - 60*daySeconds},
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: users[0].Id, Type: model.LogTypeConsume, Quota: int(mustQuota(t, "80")), CreatedAt: now - 2*daySeconds},
		{UserId: users[1].Id, Type: model.LogTypeConsume, Quota: int(mustQuota(t, "800")), CreatedAt: now - 2*daySeconds},
		{UserId: users[2].Id, Type: model.LogTypeConsume, Quota: int(mustQuota(t, "800")), CreatedAt: now - 2*daySeconds},
	}).Error)

	recommendation, err := initialGrantRecommendation(now, now)
	require.NoError(t, err)
	require.NotNil(t, recommendation)
	assert.Equal(t, mustQuota(t, "96"), recommendation.Quota)
	assert.Equal(t, 1, recommendation.UserCount)
}

func TestInitialGrantRecommendationUsesNetConsumptionThroughSettlement(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	settledAt := now - daySeconds
	cycle := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now - 2*daySeconds, SettledAt: &settledAt,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusClosed,
	}
	user := model.User{
		Username: "net-recommendation", AffCode: "net-recommendation",
		Status: common.UserStatusEnabled, CreatedAt: now - 60*daySeconds,
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: user.Id, Type: model.LogTypeConsume, Quota: int(mustQuota(t, "200")), CreatedAt: now - 3*daySeconds},
		{UserId: user.Id, Type: model.LogTypeRefund, Quota: int(mustQuota(t, "100")), CreatedAt: settledAt},
	}).Error)

	recommendation, err := initialGrantRecommendation(now, now)
	require.NoError(t, err)
	require.NotNil(t, recommendation)
	assert.Equal(t, mustQuota(t, "120"), recommendation.Quota)
	assert.Equal(t, settledAt, recommendation.ReferenceEnd)
}

func TestInitialGrantRecommendationIsHiddenWhenConsumeLogIsDisabled(t *testing.T) {
	setupQuotaTestDB(t)
	original := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = original })

	recommendation, err := initialGrantRecommendation(time.Now().Unix(), time.Now().Unix())
	require.NoError(t, err)
	assert.Nil(t, recommendation)
}

func TestListCyclesAllowsNegativeManagedBalance(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now - daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusClosed,
	}
	user := model.User{
		Username: "negative-recommendation", AffCode: "negative-recommendation",
		Status: common.UserStatusEnabled, Quota: -12070, CreatedAt: now - 60*daySeconds,
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	result, err := ListCycles()
	require.NoError(t, err)
	require.NotNil(t, result.Recommendation)
	assert.Equal(t, 1, result.Recommendation.UserCount)
}

func TestSummarizePlanPreservesAdjustmentAndFundFlowTotals(t *testing.T) {
	t.Parallel()
	allocationBefore := mustQuota(t, "600")
	plan := model.QuotaPlan{
		PlanType:            model.QuotaPlanTypeAdjustment,
		BudgetQuotaSnapshot: mustQuota(t, "1000"), AllocationBeforeQuota: &allocationBefore,
	}
	items := []model.QuotaItem{
		{Action: model.QuotaAdjustmentActionIncrease, AdjustmentQuota: mustQuota(t, "100")},
		{Action: model.QuotaAdjustmentActionDecrease, AdjustmentQuota: -mustQuota(t, "100")},
	}

	summary, err := summarizePlan(plan, items, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, CategorySummary{Count: 1, Total: mustQuota(t, "100")}, summary.Increase)
	assert.Equal(t, CategorySummary{Count: 1, Total: mustQuota(t, "100")}, summary.Decrease)
	assert.Equal(t, mustQuota(t, "100"), summary.ReclaimedUsedForIncreases)
	assert.Equal(t, int64(0), summary.CyclePoolIssuedQuota)
	assert.Equal(t, mustQuota(t, "600"), summary.OccupiedAfter)
}

func TestSummarizePlanRejectsMissingAllocationSnapshot(t *testing.T) {
	t.Parallel()
	plan := model.QuotaPlan{
		PlanType: model.QuotaPlanTypeAdjustment, BudgetQuotaSnapshot: mustQuota(t, "1000"),
	}

	_, err := summarizePlan(plan, nil, 0, 0)
	require.EqualError(t, err, "方案缺少额度发放快照")
}

func TestSummarizePlanShowsEveryReclaimedQuotaDestination(t *testing.T) {
	t.Parallel()
	allocationBefore := mustQuota(t, "950")
	plan := model.QuotaPlan{
		PlanType: model.QuotaPlanTypeAdjustment, BudgetQuotaSnapshot: mustQuota(t, "1000"),
		AllocationBeforeQuota: &allocationBefore,
	}
	items := []model.QuotaItem{
		{Action: model.QuotaAdjustmentActionIncrease, AdjustmentQuota: mustQuota(t, "50")},
		{Action: model.QuotaAdjustmentActionDecrease, AdjustmentQuota: -mustQuota(t, "100")},
	}

	summary, err := summarizePlan(plan, items, mustQuota(t, "100"), 0)
	require.NoError(t, err)
	assert.Equal(t, mustQuota(t, "50"), summary.ReclaimedUsedToCoverOverage)
	assert.Equal(t, mustQuota(t, "50"), summary.ReclaimedUsedForIncreases)
	assert.Zero(t, summary.ReclaimedUnused)
	assert.Zero(t, summary.CyclePoolIssuedQuota)
	assert.Equal(t, summary.Decrease.Total,
		summary.ReclaimedUsedToCoverOverage+summary.ReclaimedUsedForIncreases+summary.ReclaimedUnused)
}

func TestGetPlanDetailUsesOrdinaryAllocationForTemporaryQuota(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		BudgetQuota: mustQuota(t, "1000"), TemporaryQuotaReserve: mustQuota(t, "200"),
	}
	require.NoError(t, db.Create(&cycle).Error)
	executedAt := now - 60
	require.NoError(t, db.Create(&model.TemporaryQuotaRequest{
		CycleId: cycle.Id, UserId: 1, IdempotencyKey: "summary-temporary",
		RequestedQuota: mustQuota(t, "200"), ApprovedQuota: mustQuota(t, "200"),
		Status: model.TemporaryQuotaRequestStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	allocationBefore := mustQuota(t, "900")
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, Parameters: `{}`, BudgetQuotaSnapshot: cycle.BudgetQuota,
		AllocationBeforeQuota: &allocationBefore, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.QuotaItem{
		PlanId: plan.Id, Action: model.QuotaAdjustmentActionDecrease,
		AdjustmentQuota: -mustQuota(t, "100"), CalculationData: `{"decrease_kind":"ordinary"}`,
	}).Error)

	detail, err := GetPlanDetail(plan.Id)
	require.NoError(t, err)
	assert.Zero(t, detail.Summary.ReclaimedUsedToCoverOverage)
	assert.Equal(t, mustQuota(t, "100"), detail.Summary.ReclaimedUnused)
}

func TestGetPlanDetailIgnoresTemporaryQuotaExecutedAfterSnapshot(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		BudgetQuota: mustQuota(t, "1000"), TemporaryQuotaReserve: mustQuota(t, "200"),
	}
	require.NoError(t, db.Create(&cycle).Error)
	allocationBefore := mustQuota(t, "950")
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, Parameters: `{}`, BudgetQuotaSnapshot: cycle.BudgetQuota,
		AllocationBeforeQuota: &allocationBefore, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.QuotaItem{
		PlanId: plan.Id, Action: model.QuotaAdjustmentActionDecrease,
		AdjustmentQuota: -mustQuota(t, "100"),
	}).Error)
	future := now + 60
	temporaryQuotaPlan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: future, Parameters: `{}`, BudgetQuotaSnapshot: cycle.BudgetQuota,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &future,
	}
	require.NoError(t, db.Create(&temporaryQuotaPlan).Error)
	require.NoError(t, db.Create(&model.TemporaryQuotaRequest{
		CycleId: cycle.Id, UserId: 1, IdempotencyKey: "future-summary-temporary",
		RequestedQuota: mustQuota(t, "200"), ApprovedQuota: mustQuota(t, "200"),
		Status: model.TemporaryQuotaRequestStatusExecuted, PlanId: &temporaryQuotaPlan.Id, ExecutedAt: &future,
	}).Error)

	detail, err := GetPlanDetail(plan.Id)
	require.NoError(t, err)
	assert.Equal(t, mustQuota(t, "100"), detail.Summary.ReclaimedUsedToCoverOverage)
}

func TestGetPlanDetailOrdersSameSecondTemporaryQuotaByPlanID(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		BudgetQuota: mustQuota(t, "1000"), TemporaryQuotaReserve: mustQuota(t, "300"),
	}
	require.NoError(t, db.Create(&cycle).Error)
	beforePlan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, Parameters: `{}`, Status: model.QuotaPlanStatusExecuted, ExecutedAt: &now,
	}
	require.NoError(t, db.Create(&beforePlan).Error)
	require.NoError(t, db.Create(&model.TemporaryQuotaRequest{
		CycleId: cycle.Id, UserId: 1, IdempotencyKey: "same-second-before",
		RequestedQuota: mustQuota(t, "100"), ApprovedQuota: mustQuota(t, "100"),
		Status: model.TemporaryQuotaRequestStatusExecuted, PlanId: &beforePlan.Id, ExecutedAt: &now,
	}).Error)
	allocationBefore := mustQuota(t, "900")
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, Parameters: `{}`, BudgetQuotaSnapshot: cycle.BudgetQuota,
		AllocationBeforeQuota: &allocationBefore, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.QuotaItem{
		PlanId: plan.Id, Action: model.QuotaAdjustmentActionDecrease,
		AdjustmentQuota: -mustQuota(t, "100"),
	}).Error)
	afterPlan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, Parameters: `{}`, Status: model.QuotaPlanStatusExecuted, ExecutedAt: &now,
	}
	require.NoError(t, db.Create(&afterPlan).Error)
	require.NoError(t, db.Create(&model.TemporaryQuotaRequest{
		CycleId: cycle.Id, UserId: 2, IdempotencyKey: "same-second-after",
		RequestedQuota: mustQuota(t, "200"), ApprovedQuota: mustQuota(t, "200"),
		Status: model.TemporaryQuotaRequestStatusExecuted, PlanId: &afterPlan.Id, ExecutedAt: &now,
	}).Error)

	detail, err := GetPlanDetail(plan.Id)
	require.NoError(t, err)
	assert.Equal(t, mustQuota(t, "100"), detail.Summary.ReclaimedUsedToCoverOverage)
}

func TestDefaultNextAdjustmentIsCappedButNotTiedToCycleEnd(t *testing.T) {
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, shanghaiLocation).Unix()
	longCycle := model.QuotaCycle{CycleEndAt: time.Date(2026, time.September, 1, 18, 0, 0, 0, shanghaiLocation).Unix()}
	shortCycle := model.QuotaCycle{CycleEndAt: time.Date(2026, time.August, 7, 18, 0, 0, 0, shanghaiLocation).Unix()}

	assert.Equal(t, time.Date(2026, time.August, 10, 9, 0, 0, 0, shanghaiLocation).Unix(), defaultNextAdjustmentAt(longCycle, now))
	assert.Equal(t, shortCycle.CycleEndAt, defaultNextAdjustmentAt(shortCycle, now))
}

func TestRegeneratePlanCreatesDraftAndCancelsOriginalTogether(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 7*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000,
		Status:                  model.QuotaCycleStatusActive, CreatedBy: "root",
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&model.User{
		Username: "alice", AffCode: "alice-code", Status: common.UserStatusEnabled,
		CreatedAt: now - daySeconds,
	}).Error)
	nextAdjustment := now + daySeconds
	original, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
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

func TestRegenerateAdjustmentRequiresExecutedPlan(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 7*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		Status: model.QuotaPlanStatusCancelled,
	}
	require.NoError(t, db.Create(&plan).Error)

	_, err := RegeneratePlan(plan.Id, "root")
	require.EqualError(t, err, "当前周期尚无已执行方案，请先完成初始化发放")
}

func TestGetPlanOptionsCountsOnlyExecutedPlans(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 7*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&[]model.QuotaPlan{
		{CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, Status: model.QuotaPlanStatusDraft},
		{CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, Status: model.QuotaPlanStatusCancelled},
	}).Error)

	options, err := GetPlanOptions()
	require.NoError(t, err)
	assert.True(t, options.InitializationRequired)

	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		Status: model.QuotaPlanStatusExecuted,
	}).Error)
	options, err = GetPlanOptions()
	require.NoError(t, err)
	assert.False(t, options.InitializationRequired)
}

func TestGetPlanOptionsIgnoresInternalQuotaActions(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 7*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&[]model.QuotaPlan{
		{
			CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
			Parameters: `{"manual":true}`, Status: model.QuotaPlanStatusExecuted,
		},
		{
			CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
			Parameters: `{"temporary_quota_request_id":1}`, Status: model.QuotaPlanStatusExecuted,
		},
	}).Error)

	options, err := GetPlanOptions()
	require.NoError(t, err)
	assert.True(t, options.InitializationRequired)
}

func TestCancelPlanCountsReasonCharacters(t *testing.T) {
	db := setupQuotaTestDB(t)
	plan := model.QuotaPlan{Status: model.QuotaPlanStatusDraft}
	require.NoError(t, db.Create(&plan).Error)

	require.EqualError(t, CancelPlan(plan.Id, "root", "   "), "取消原因不能为空")
	require.EqualError(t, CancelPlan(plan.Id, "root", strings.Repeat("取", 501)), "取消原因不得超过500个字符")
	require.NoError(t, CancelPlan(plan.Id, "root", "调整计划"))
}

func TestRunQuotaLifecycleClosesExpiredSchedulesAndActivatesSuccessor(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t)
	now := time.Now().Unix()
	cycles := []model.QuotaCycle{
		{
			CycleStartAt: now - 20*daySeconds, CycleEndAt: now - 10*daySeconds,
			BudgetQuota: 1, InitialGrantQuota: 1, Status: model.QuotaCycleStatusScheduled,
			BalancePolicy: model.QuotaCycleBalancePolicyCarry,
		},
		{
			CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
			BudgetQuota: 1, InitialGrantQuota: 1, Status: model.QuotaCycleStatusScheduled,
			BalancePolicy: model.QuotaCycleBalancePolicyCarry,
		},
	}
	require.NoError(t, db.Create(&cycles).Error)
	expiredPlan := model.QuotaPlan{
		CycleId: cycles[0].Id, PlanType: model.QuotaPlanTypeInitialization,
		Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&expiredPlan).Error)

	_, err := RunQuotaLifecycle(now)
	require.NoError(t, err)
	var stored []model.QuotaCycle
	require.NoError(t, db.Order("id").Find(&stored).Error)
	require.Len(t, stored, 2)
	assert.Equal(t, model.QuotaCycleStatusClosed, stored[0].Status)
	assert.Equal(t, model.QuotaCycleStatusActive, stored[1].Status)
	require.Nil(t, stored[0].ActiveKey)
	require.NotNil(t, stored[1].ActiveKey)
	require.NoError(t, db.First(&expiredPlan, expiredPlan.Id).Error)
	assert.Equal(t, model.QuotaPlanStatusCancelled, expiredPlan.Status)
	assert.Equal(t, "额度周期已关闭", expiredPlan.CancelReason)
}
