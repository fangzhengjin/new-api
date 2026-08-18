package quota

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func enableCompanyQuotaModeForTest(t *testing.T) {
	t.Helper()
	previous := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previous })
}

func seedRecoveryCycleAndUser(t *testing.T, autoEnabled bool) (*model.QuotaCycle, *model.User) {
	t.Helper()
	now := time.Now().Unix()
	cycle := &model.QuotaCycle{
		CycleStartAt: now - 20*daySeconds, CycleEndAt: now + 20*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		RecoveryReserveQuota: mustQuota(t, "200"), Status: model.QuotaCycleStatusActive,
		AutoRecoveryEnabled: autoEnabled, AutoRecoverySingleQuota: mustQuota(t, "30"),
		AutoRecoveryThresholdQuota: mustQuota(t, "20"), AutoRecoveryMaxCount: 1,
		AutoRecoveryMaxQuota: mustQuota(t, "30"),
	}
	user := &model.User{
		Username: "recovery-user", AffCode: "recovery-user", Status: common.UserStatusEnabled,
		Quota: int(mustQuota(t, "10")),
	}
	require.NoError(t, model.DB.Create(cycle).Error)
	require.NoError(t, model.DB.Create(user).Error)
	executedAt := now - 60
	require.NoError(t, model.DB.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization, StagePercent: 7_500,
		SnapshotAt: now - 120, AlgorithmVersion: LegacyAlgorithmVersion,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	return cycle, user
}

func TestSubmitRecoveryAutoExecutesOnceForSameIdempotencyKey(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCompanyQuotaModeForTest(t)
	_, user := seedRecoveryCycleAndUser(t, true)
	input := RecoveryRequestInput{
		UserID: user.Id, IdempotencyKey: "same-recovery-key", RequestedQuota: mustQuota(t, "20"), Reason: "continue work",
	}

	first, err := SubmitRecoveryRequest(input)
	require.NoError(t, err)
	second, err := SubmitRecoveryRequest(input)
	require.NoError(t, err)
	assert.Equal(t, first.Request.Id, second.Request.Id)
	assert.Equal(t, model.QuotaRecoveryRequestStatusExecuted, first.Request.Status)
	assert.Equal(t, model.QuotaRecoveryDecisionAuto, first.Request.Decision)
	require.NotNil(t, first.Request.PlanId)

	var storedUser model.User
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "30")), storedUser.Quota)
	var requests, plans int64
	require.NoError(t, db.Model(&model.QuotaRecoveryRequest{}).Count(&requests).Error)
	require.NoError(t, db.Model(&model.QuotaPlan{}).Where("id = ?", *first.Request.PlanId).Count(&plans).Error)
	assert.Equal(t, int64(1), requests)
	assert.Equal(t, int64(1), plans)
	var item model.QuotaItem
	require.NoError(t, db.Where("plan_id = ?", *first.Request.PlanId).First(&item).Error)
	assert.Contains(t, item.LogContent, "操作类型：临时额度恢复")
	overview, err := GetRecoveryOverview(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(mustQuota(t, "30")), overview.CurrentQuota)
	require.Len(t, overview.Requests, 1)
}

func TestRecoveryBeyondAutoLimitWaitsForManualApproval(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCompanyQuotaModeForTest(t)
	_, user := seedRecoveryCycleAndUser(t, true)
	_, err := SubmitRecoveryRequest(RecoveryRequestInput{
		UserID: user.Id, IdempotencyKey: "first-auto-request", RequestedQuota: mustQuota(t, "10"), Reason: "first",
	})
	require.NoError(t, err)
	second, err := SubmitRecoveryRequest(RecoveryRequestInput{
		UserID: user.Id, IdempotencyKey: "second-manual-request", RequestedQuota: mustQuota(t, "10"), Reason: "second",
	})
	require.NoError(t, err)
	assert.Equal(t, model.QuotaRecoveryRequestStatusPending, second.Request.Status)
	assert.Nil(t, second.Request.PlanId)

	approved, err := ApproveRecoveryRequest(second.Request.Id, mustQuota(t, "8"), "root", "approved for priority work")
	require.NoError(t, err)
	assert.Equal(t, model.QuotaRecoveryDecisionManual, approved.Request.Decision)
	assert.Equal(t, mustQuota(t, "8"), approved.Request.ApprovedQuota)
	var storedUser model.User
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "28")), storedUser.Quota)
}

func TestRecoveryPoolExhaustionAndWhitelistDoNotChangeBalance(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCompanyQuotaModeForTest(t)
	cycle, user := seedRecoveryCycleAndUser(t, true)
	executedAt := time.Now().Unix() - 10
	require.NoError(t, db.Create(&model.QuotaRecoveryRequest{
		CycleId: cycle.Id, UserId: 999, Username: "used", IdempotencyKey: "reserve-used-key",
		RequestedQuota: cycle.RecoveryReserveQuota, ApprovedQuota: cycle.RecoveryReserveQuota,
		Status: model.QuotaRecoveryRequestStatusExecuted, Decision: model.QuotaRecoveryDecisionManual,
		CreatedAt: executedAt, ExecutedAt: &executedAt,
	}).Error)

	result, err := SubmitRecoveryRequest(RecoveryRequestInput{
		UserID: user.Id, IdempotencyKey: "reserve-empty-key", RequestedQuota: mustQuota(t, "10"), Reason: "no reserve",
	})
	require.NoError(t, err)
	assert.Equal(t, model.QuotaRecoveryRequestStatusRejected, result.Request.Status)
	assert.Contains(t, result.Request.ReviewReason, "恢复池已耗尽")

	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota_whitelist", true).Error)
	_, err = SubmitRecoveryRequest(RecoveryRequestInput{
		UserID: user.Id, IdempotencyKey: "whitelist-request", RequestedQuota: mustQuota(t, "10"), Reason: "whitelist",
	})
	require.EqualError(t, err, "白名单用户不参与额度恢复")
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "10")), stored.Quota)
}

func TestCandidateFinalZeroCanBeFollowedByRecovery(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCompanyQuotaModeForTest(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now + daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		RecoveryReserveQuota: mustQuota(t, "100"), Status: model.QuotaCycleStatusActive,
		AllocationAlgorithmVersion: CandidateAlgorithmVersion,
		AutoRecoveryEnabled:        true, AutoRecoverySingleQuota: mustQuota(t, "20"),
		AutoRecoveryThresholdQuota: mustQuota(t, "10"), AutoRecoveryMaxCount: 1,
		AutoRecoveryMaxQuota: mustQuota(t, "20"),
	}
	user := model.User{Username: "final-zero", AffCode: "final-zero", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "50"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	params, err := common.Marshal(PlanParameters{ThoroughRelease: true})
	require.NoError(t, err)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 10_000,
		SnapshotAt: now, NextAdjustmentAt: &cycle.CycleEndAt, AlgorithmVersion: CandidateAlgorithmVersion,
		Parameters: string(params), BudgetQuotaSnapshot: cycle.BudgetQuota,
		ManagedBalanceQuota: int64(user.Quota), PlannedDeltaQuota: -int64(user.Quota),
		Status: model.QuotaPlanStatusDraft, CreatedAt: now,
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.QuotaItem{
		PlanId: plan.Id, UserId: user.Id, Username: user.Username, Action: model.QuotaAdjustmentActionDecrease,
		SnapshotBalanceQuota: int64(user.Quota), AdjustmentQuota: -int64(user.Quota),
		LogStatus: model.QuotaNotificationStatusPending, EmailStatus: model.QuotaNotificationStatusPending,
	}).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, cycle.Id, "root", now)
		return executeErr
	}))
	result, err := SubmitRecoveryRequest(RecoveryRequestInput{
		UserID: user.Id, IdempotencyKey: "after-final-zero", RequestedQuota: mustQuota(t, "20"), Reason: "temporary continuity",
	})
	require.NoError(t, err)
	assert.Equal(t, model.QuotaRecoveryRequestStatusExecuted, result.Request.Status)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "20")), user.Quota)
}
