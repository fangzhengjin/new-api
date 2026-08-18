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

func TestValidateRecoveryPolicyRequiresCompleteBoundedConfiguration(t *testing.T) {
	require.NoError(t, validateRecoveryPolicy(RecoveryPolicy{}, mustQuota(t, "100")))
	require.EqualError(t, validateRecoveryPolicy(RecoveryPolicy{SingleQuota: 1}, mustQuota(t, "100")), "关闭自动恢复时额度和次数参数必须为0")
	policy := RecoveryPolicy{
		Enabled: true, SingleQuota: mustQuota(t, "10"), ThresholdQuota: mustQuota(t, "5"),
		MaxCount: 2, MaxQuota: mustQuota(t, "20"),
	}
	require.EqualError(t, validateRecoveryPolicy(policy, 0), "开启自动恢复前必须配置正数小额恢复池")
	require.NoError(t, validateRecoveryPolicy(policy, mustQuota(t, "100")))
	policy.MaxQuota = mustQuota(t, "9")
	require.EqualError(t, validateRecoveryPolicy(policy, mustQuota(t, "100")), "自动恢复每用户总额上限必须不小于单次上限且不超过单用户额度上限")
}

func TestValidateInitialGrantQuotaRejectsUnsupportedUserBalance(t *testing.T) {
	require.EqualError(
		t,
		validateInitialGrantQuota(int64(common.MaxQuota)+1),
		"人均首次额度超出单用户可支持范围",
	)
}

func TestRecoveryReserveValidationUsesSmallestOrdinaryStage(t *testing.T) {
	t.Parallel()
	budget := mustQuota(t, "1000")
	require.NoError(t, validateRecoveryReserveQuota(budget, 0))
	require.NoError(t, validateRecoveryReserveQuota(budget, mustQuota(t, "750")))
	assert.EqualError(t, validateRecoveryReserveQuota(budget, budget), "小额恢复池必须大于等于0且小于采购总额")
	assert.EqualError(t, validateRecoveryReserveQuota(budget, mustQuota(t, "750.01")), "小额恢复池不能超过75%普通调配阶段上限")
}

func TestActiveCycleCannotChangeRecoveryReserve(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		RecoveryReserveQuota: mustQuota(t, "50"), Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	updatedReserve := mustQuota(t, "60")

	err := UpdateCycleSettings(cycle.Id, cycle.BudgetQuota, nil, &updatedReserve, nil, nil, "root")
	require.EqualError(t, err, "只有已规划周期可以修改小额恢复池")

	var stored model.QuotaCycle
	require.NoError(t, db.First(&stored, cycle.Id).Error)
	assert.Equal(t, mustQuota(t, "50"), stored.RecoveryReserveQuota)
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

func TestUpdateActiveCycleRejectsBudgetBelowManagedPosition(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed", AffCode: "managed-budget", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "100"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	err := UpdateCycleSettings(cycle.Id, mustQuota(t, "50"), nil, nil, nil, nil, "root")
	require.EqualError(t, err, "新周期预算不能低于当前受管头寸")

	var stored model.QuotaCycle
	require.NoError(t, db.First(&stored, cycle.Id).Error)
	assert.Equal(t, cycle.BudgetQuota, stored.BudgetQuota)
}

func TestCreateCycleRequiresSupportedConcentrationMultiplier(t *testing.T) {
	setupQuotaTestDB(t)
	now := time.Now().Unix()
	params := CreateCycleParams{
		StartAt: now, EndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		BalancePolicy: model.QuotaCycleBalancePolicyReset, CreatedBy: "root",
	}

	_, err := CreateCycle(params)
	require.EqualError(t, err, "自动分配上限倍率必须是1.5、2或3")
	params.ConcentrationMultiplier = 20_000
	cycle, err := CreateCycle(params)
	require.NoError(t, err)
	assert.Equal(t, int64(20_000), cycle.ConcentrationMultiplier)
	assert.Equal(t, ConcentrationAlgorithmVersion, cycle.AllocationAlgorithmVersion)
}

func TestScheduledCycleCanChangeConcentrationMultiplierButActiveCycleCannot(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	scheduled := model.QuotaCycle{
		CycleStartAt: now + daySeconds, CycleEndAt: now + 31*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusScheduled,
	}
	require.NoError(t, db.Create(&scheduled).Error)
	draft := model.QuotaPlan{
		CycleId: scheduled.Id, PlanType: model.QuotaPlanTypeAdjustment,
		AlgorithmVersion: ConcentrationAlgorithmVersion, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&draft).Error)
	updatedMultiplier := int64(30_000)

	require.NoError(t, UpdateCycleSettings(
		scheduled.Id, scheduled.BudgetQuota, nil, nil, nil, &updatedMultiplier, "root",
	))
	require.NoError(t, db.First(&scheduled, scheduled.Id).Error)
	require.NoError(t, db.First(&draft, draft.Id).Error)
	assert.Equal(t, updatedMultiplier, scheduled.ConcentrationMultiplier)
	assert.Equal(t, ConcentrationAlgorithmVersion, scheduled.AllocationAlgorithmVersion)
	assert.Equal(t, model.QuotaPlanStatusCancelled, draft.Status)

	active := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&active).Error)
	err := UpdateCycleSettings(active.Id, active.BudgetQuota, nil, nil, nil, &updatedMultiplier, "root")
	require.EqualError(t, err, "只有已规划周期可以修改自动分配上限")
}

func TestManualIncreaseRequiresConfirmationOnlyAfterExceedingCurrentStage(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-stage", AffCode: "managed-stage", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "50"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	executedAt := now - 30
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, StagePercent: 1_000,
		SnapshotAt: now - 60, AlgorithmVersion: AlgorithmVersion, Status: model.QuotaPlanStatusExecuted,
		ExecutedAt: &executedAt,
	}).Error)

	result, err := ManualAdjustUserQuota(user.Id, mustQuota(t, "150"), "mid-cycle grant", "root", false)
	require.Nil(t, result)
	var confirmation *ManualAdjustmentConfirmationError
	require.ErrorAs(t, err, &confirmation)
	assert.Equal(t, mustQuota(t, "100"), confirmation.StageCapQuota)
	assert.Equal(t, mustQuota(t, "200"), confirmation.OccupiedAfterQuota)
	assert.Equal(t, mustQuota(t, "100"), confirmation.StageOverageQuota)

	var unchangedUser model.User
	var draftPlans, items int64
	require.NoError(t, db.First(&unchangedUser, user.Id).Error)
	require.NoError(t, db.Model(&model.QuotaPlan{}).Where("status = ?", model.QuotaPlanStatusDraft).Count(&draftPlans).Error)
	require.NoError(t, db.Model(&model.QuotaItem{}).Count(&items).Error)
	assert.Equal(t, int(mustQuota(t, "50")), unchangedUser.Quota)
	assert.Zero(t, draftPlans)
	assert.Zero(t, items)

	result, err = ManualAdjustUserQuota(user.Id, mustQuota(t, "150"), "mid-cycle grant", "root", true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.AffectedUsers)

	var storedUser model.User
	var item model.QuotaItem
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	require.NoError(t, db.Order("id DESC").First(&item).Error)
	assert.Equal(t, int(mustQuota(t, "200")), storedUser.Quota)
	assert.Equal(t, model.QuotaAdjustmentActionIncrease, item.Action)
	assert.Equal(t, mustQuota(t, "150"), item.AdjustmentQuota)
}

func TestManualIncreaseAtCurrentStageCapDoesNotRequireConfirmation(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-stage-boundary", AffCode: "managed-stage-boundary", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "50"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	executedAt := now - 30
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, StagePercent: 1_000,
		SnapshotAt: now - 60, AlgorithmVersion: AlgorithmVersion, Status: model.QuotaPlanStatusExecuted,
		ExecutedAt: &executedAt,
	}).Error)

	result, err := ManualAdjustUserQuota(user.Id, mustQuota(t, "50"), "stage boundary", "root", false)
	require.NoError(t, err)
	assert.Equal(t, 1, result.AffectedUsers)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "100")), user.Quota)
}

func TestManualIncreaseCannotExceedCycleBudget(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "100"), InitialGrantQuota: mustQuota(t, "10"),
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-budget", AffCode: "managed-budget-limit", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "90"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	_, err := ManualAdjustUserQuota(user.Id, mustQuota(t, "20"), "manual increase", "root", false)
	require.EqualError(t, err, "本次调增无法执行：调增后，本期消费与全体受管账户余额合计将超过公司本周期采购总额，超出金额为 ＄10.000000")

	var plans int64
	require.NoError(t, db.Model(&model.QuotaPlan{}).Count(&plans).Error)
	assert.Zero(t, plans)
}

func TestManualIncreaseMayLeaveBalanceNegative(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-negative", AffCode: "managed-negative", Status: common.UserStatusEnabled, Quota: -100}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	result, err := ManualAdjustUserQuota(user.Id, 30, "signed balance", "root", false)
	require.NoError(t, err)
	assert.Equal(t, 1, result.AffectedUsers)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, -70, user.Quota)

	var item model.QuotaItem
	require.NoError(t, db.Order("id DESC").First(&item).Error)
	assert.Equal(t, model.QuotaAdjustmentActionIncrease, item.Action)
	assert.Equal(t, int64(-70), *item.ActualAfterQuota)

	_, err = ManualAdjustUserQuota(user.Id, -1, "signed balance", "root", false)
	require.EqualError(t, err, "调减额度不能超过用户当前余额")
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, -70, user.Quota)
}

func TestManualIncreaseUsesFinalBalanceRange(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-minimum", AffCode: "managed-minimum", Status: common.UserStatusEnabled, Quota: common.MinQuota}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	result, err := ManualAdjustUserQuota(user.Id, int64(common.MaxQuota)+1, "return to zero", "root", false)
	require.NoError(t, err)
	assert.Equal(t, 1, result.AffectedUsers)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Zero(t, user.Quota)
}

func TestManualDecreaseUsesAdjustmentAmount(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-decrease", AffCode: "managed-decrease", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "100"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	result, err := ManualAdjustUserQuota(user.Id, -mustQuota(t, "30"), "manual decrease", "root", false)
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
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })
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

	_, err := ManualAdjustUserQuota(users[0].Id, -mustQuota(t, "20"), "too much", "root", false)
	require.EqualError(t, err, "调减额度不能超过用户当前余额")
	_, err = ManualAdjustUserQuota(users[1].Id, -mustQuota(t, "20"), "whitelist", "root", false)
	require.EqualError(t, err, "该用户在额度白名单中，不能手工调额；请先移出白名单")

	var plans int64
	require.NoError(t, db.Model(&model.QuotaPlan{}).Count(&plans).Error)
	assert.Zero(t, plans)
}

func TestManualAdjustmentAllowsEmptyReasonAndCountsCharacters(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60*daySeconds,
		BudgetQuota: mustQuota(t, "100"), InitialGrantQuota: mustQuota(t, "10"),
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "managed-reason", AffCode: "managed-reason", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)

	_, err := ManualAdjustUserQuota(user.Id, 1, "", "root", true)
	require.NoError(t, err)
	_, err = ManualAdjustUserQuota(user.Id, 1, strings.Repeat("调", 255), "root", true)
	require.NoError(t, err)
	_, err = ManualAdjustUserQuota(user.Id, 1, strings.Repeat("调", 256), "root", true)
	require.EqualError(t, err, "调整原因不得超过255个字符")
}

func TestManualAdjustmentRollsBackDraftWhenExecutionFails(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })
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

	_, err := ManualAdjustUserQuota(user.Id, 1, "rollback execution", "root", true)
	require.ErrorContains(t, err, "forced quota write failure")

	var plans, items int64
	require.NoError(t, db.Model(&model.QuotaPlan{}).Count(&plans).Error)
	require.NoError(t, db.Model(&model.QuotaItem{}).Count(&items).Error)
	assert.Zero(t, plans)
	assert.Zero(t, items)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Zero(t, user.Quota)
}

func TestSetQuotaWhitelistKeepsIdempotentRequestWithoutActiveCycle(t *testing.T) {
	db := setupQuotaTestDB(t)
	previousMode := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previousMode })
	user := model.User{
		Username: "managed-idempotent-whitelist", AffCode: "managed-idempotent-whitelist",
		Status: common.UserStatusEnabled, QuotaWhitelist: true,
	}
	require.NoError(t, db.Create(&user).Error)

	require.NoError(t, SetQuotaWhitelist(user.Id, true, "root"))
	require.EqualError(t, SetQuotaWhitelist(user.Id, false, "root"), "当前没有进行中的额度周期，不能修改额度白名单")
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

	summary, err := summarizePlan(plan, items, 0)
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

func TestSummarizePlanSeparatesRecoveryReserveFromFutureStages(t *testing.T) {
	t.Parallel()
	plan := model.QuotaPlan{
		PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 7_500,
		BudgetQuotaSnapshot: mustQuota(t, "1000"),
	}

	summary, err := summarizePlan(plan, nil, mustQuota(t, "100"))
	require.NoError(t, err)
	assert.Equal(t, mustQuota(t, "650"), summary.StageCap)
	assert.Equal(t, mustQuota(t, "250"), summary.FutureReserved)
	assert.Equal(t, mustQuota(t, "100"), summary.RecoveryReserve)
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

func TestRunQuotaLifecycleClosesExpiredSchedulesAndActivatesSuccessor(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t, false, "23:59")
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
	assert.Equal(t, "采购周期已收口", expiredPlan.CancelReason)
}
