package quota

import (
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

func enableCycleQuotaManagementForTest(t *testing.T) {
	t.Helper()
	previous := operation_setting.CycleQuotaManagementEnabled
	settings := operation_setting.GetQuotaSetting()
	previousProjects := settings.TemporaryQuotaProjects
	operation_setting.CycleQuotaManagementEnabled = true
	settings.TemporaryQuotaProjects = map[string]bool{"Project A": true, "Project B": false}
	t.Cleanup(func() {
		operation_setting.CycleQuotaManagementEnabled = previous
		settings.TemporaryQuotaProjects = previousProjects
	})
}

func seedTemporaryQuotaCycleAndUser(t *testing.T, autoEnabled bool) (*model.QuotaCycle, *model.User) {
	t.Helper()
	now := time.Now().Unix()
	allocated := mustQuota(t, "10")
	cycle := &model.QuotaCycle{
		CycleStartAt: now - 20*daySeconds, CycleEndAt: now + 20*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		TemporaryQuotaReserve: mustQuota(t, "200"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status:                                   model.QuotaCycleStatusActive,
		TemporaryQuotaAutoApprovalEnabled:        autoEnabled,
		TemporaryQuotaAutoApprovalSingleQuota:    mustQuota(t, "30"),
		TemporaryQuotaAutoApprovalThresholdQuota: mustQuota(t, "20"),
		TemporaryQuotaAutoApprovalMaxCount:       1,
		TemporaryQuotaAutoApprovalMaxQuota:       mustQuota(t, "30"),
	}
	user := &model.User{
		Username: "temporary-user", AffCode: "temporary-user", Status: common.UserStatusEnabled,
		Quota: int(mustQuota(t, "10")),
	}
	require.NoError(t, model.DB.Create(cycle).Error)
	require.NoError(t, model.DB.Create(user).Error)
	executedAt := now - 60
	require.NoError(t, model.DB.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		SnapshotAt: now - 120, AlgorithmVersion: AlgorithmVersion,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	return cycle, user
}

func TestSubmitTemporaryQuotaAutoExecutesOnceForSameIdempotencyKey(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCycleQuotaManagementForTest(t)
	_, user := seedTemporaryQuotaCycleAndUser(t, true)
	input := TemporaryQuotaRequestInput{
		UserID: user.Id, IdempotencyKey: "same-temporary-key", RequestedQuota: mustQuota(t, "20"), Project: "Project A", Reason: "continue work",
	}

	first, err := SubmitTemporaryQuotaRequest(input)
	require.NoError(t, err)
	second, err := SubmitTemporaryQuotaRequest(input)
	require.NoError(t, err)
	assert.Equal(t, first.Id, second.Id)
	assert.Equal(t, model.TemporaryQuotaRequestStatusExecuted, first.Status)
	assert.Equal(t, model.TemporaryQuotaDecisionAuto, first.Decision)
	require.NotNil(t, first.PlanId)

	var storedUser model.User
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "30")), storedUser.Quota)
	var requests, plans int64
	require.NoError(t, db.Model(&model.TemporaryQuotaRequest{}).Count(&requests).Error)
	require.NoError(t, db.Model(&model.QuotaPlan{}).Where("id = ?", *first.PlanId).Count(&plans).Error)
	assert.Equal(t, int64(1), requests)
	assert.Equal(t, int64(1), plans)
	var item model.QuotaItem
	require.NoError(t, db.Where("plan_id = ?", *first.PlanId).First(&item).Error)
	assert.Contains(t, item.LogContent, "操作类型：临时额度发放")
	overview, err := GetTemporaryQuotaOverview(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(mustQuota(t, "30")), overview.CurrentQuota)
	assert.True(t, overview.CanRequest)
	assert.Empty(t, overview.UnavailableReason)
	assert.Equal(t, []string{"Project A"}, overview.Projects)
	_, err = SubmitTemporaryQuotaRequest(TemporaryQuotaRequestInput{
		UserID: user.Id, IdempotencyKey: "disabled-project", RequestedQuota: mustQuota(t, "1"), Project: "Project B", Reason: "not selectable",
	})
	require.EqualError(t, err, "申请项目不可选，请刷新后重试")
}

func TestTemporaryQuotaBeyondAutoLimitWaitsForManualApproval(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCycleQuotaManagementForTest(t)
	_, user := seedTemporaryQuotaCycleAndUser(t, true)
	_, err := SubmitTemporaryQuotaRequest(TemporaryQuotaRequestInput{
		UserID: user.Id, IdempotencyKey: "first-auto-request", RequestedQuota: mustQuota(t, "10"), Project: "Project A", Reason: "first",
	})
	require.NoError(t, err)
	second, err := SubmitTemporaryQuotaRequest(TemporaryQuotaRequestInput{
		UserID: user.Id, IdempotencyKey: "second-manual-request", RequestedQuota: mustQuota(t, "10"), Project: "Project A", Reason: "second",
	})
	require.NoError(t, err)
	assert.Equal(t, model.TemporaryQuotaRequestStatusPending, second.Status)
	assert.Nil(t, second.PlanId)

	approved, err := ApproveTemporaryQuotaRequest(second.Id, mustQuota(t, "8"), "root", "approved for priority work")
	require.NoError(t, err)
	assert.Equal(t, model.TemporaryQuotaDecisionManual, approved.Decision)
	assert.Equal(t, mustQuota(t, "8"), approved.ApprovedQuota)
	var storedUser model.User
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "28")), storedUser.Quota)
}

func TestTemporaryQuotaReviewReasonsCountCharacters(t *testing.T) {
	enableCycleQuotaManagementForTest(t)
	tooLong := strings.Repeat("审", 501)

	_, err := ApproveTemporaryQuotaRequest(1, 1, "root", tooLong)
	require.EqualError(t, err, "审批参数不正确，审批说明不能为空且不得超过500个字符")
	_, err = RejectTemporaryQuotaRequest(1, "root", tooLong)
	require.EqualError(t, err, "拒绝原因不能为空且不得超过500个字符")
}

func TestClosedCycleTemporaryQuotaCanOnlyBeRejected(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCycleQuotaManagementForTest(t)
	cycle, user := seedTemporaryQuotaCycleAndUser(t, false)
	result, err := SubmitTemporaryQuotaRequest(TemporaryQuotaRequestInput{
		UserID: user.Id, IdempotencyKey: "closed-cycle-request", RequestedQuota: mustQuota(t, "10"),
		Project: "Project A", Reason: "cycle is closing",
	})
	require.NoError(t, err)
	require.Equal(t, model.TemporaryQuotaRequestStatusPending, result.Status)
	require.NoError(t, db.Model(&model.QuotaCycle{}).Where("id = ?", cycle.Id).Updates(map[string]interface{}{
		"status": model.QuotaCycleStatusClosed, "active_key": nil,
	}).Error)

	_, err = ApproveTemporaryQuotaRequest(result.Id, mustQuota(t, "10"), "root", "too late")
	require.EqualError(t, err, "申请所属周期已不可发放")
	rejected, err := RejectTemporaryQuotaRequest(result.Id, "root", "cycle closed")
	require.NoError(t, err)
	assert.Equal(t, model.TemporaryQuotaRequestStatusRejected, rejected.Status)

	var storedUser model.User
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "10")), storedUser.Quota)
}

func TestTemporaryQuotaPoolExhaustionAndWhitelistDoNotChangeBalance(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCycleQuotaManagementForTest(t)
	cycle, user := seedTemporaryQuotaCycleAndUser(t, true)
	executedAt := time.Now().Unix() - 10
	require.NoError(t, db.Create(&model.TemporaryQuotaRequest{
		CycleId: cycle.Id, UserId: 999, Username: "used", IdempotencyKey: "reserve-used-key",
		RequestedQuota: cycle.TemporaryQuotaReserve, ApprovedQuota: cycle.TemporaryQuotaReserve,
		Status: model.TemporaryQuotaRequestStatusExecuted, Decision: model.TemporaryQuotaDecisionManual,
		CreatedAt: executedAt, ExecutedAt: &executedAt,
	}).Error)
	overview, err := GetTemporaryQuotaOverview(user.Id)
	require.NoError(t, err)
	assert.False(t, overview.CanRequest)
	assert.Equal(t, "Temporary quota reserve is fully used", overview.UnavailableReason)

	_, err = SubmitTemporaryQuotaRequest(TemporaryQuotaRequestInput{
		UserID: user.Id, IdempotencyKey: "reserve-empty-key", RequestedQuota: mustQuota(t, "10"), Project: "Project A", Reason: "no reserve",
	})
	require.EqualError(t, err, "临时额度预留已用完")
	var replayedRequests int64
	require.NoError(t, db.Model(&model.TemporaryQuotaRequest{}).Where("idempotency_key = ?", "reserve-empty-key").Count(&replayedRequests).Error)
	assert.Zero(t, replayedRequests)

	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota_whitelist", true).Error)
	_, err = SubmitTemporaryQuotaRequest(TemporaryQuotaRequestInput{
		UserID: user.Id, IdempotencyKey: "whitelist-request", RequestedQuota: mustQuota(t, "10"), Project: "Project A", Reason: "whitelist",
	})
	require.EqualError(t, err, "白名单用户不参与临时额度申请")
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "10")), stored.Quota)
}

func TestTemporaryQuotaOverviewAndHistoryRespectAvailabilityAndOwnership(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCycleQuotaManagementForTest(t)
	cycle, user := seedTemporaryQuotaCycleAndUser(t, false)
	other := model.User{Username: "other-temporary-user", AffCode: "other-temporary-user", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&other).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&[]model.TemporaryQuotaRequest{
		{
			CycleId: cycle.Id, UserId: user.Id, Username: user.Username, IdempotencyKey: "owned-pending",
			RequestedQuota: mustQuota(t, "10"), Project: "Project A", Reason: "needle request",
			Status: model.TemporaryQuotaRequestStatusPending, CreatedAt: now - 2,
		},
		{
			CycleId: cycle.Id, UserId: user.Id, Username: user.Username, IdempotencyKey: "owned-rejected",
			RequestedQuota: mustQuota(t, "20"), Project: "Project A", Reason: "latest request",
			Status: model.TemporaryQuotaRequestStatusRejected, CreatedAt: now - 1,
		},
		{
			CycleId: cycle.Id, UserId: other.Id, Username: other.Username, IdempotencyKey: "other-private",
			RequestedQuota: mustQuota(t, "30"), Project: "Project A", Reason: "must stay private",
			Status: model.TemporaryQuotaRequestStatusPending, CreatedAt: now,
		},
	}).Error)

	overview, err := GetTemporaryQuotaOverview(user.Id)
	require.NoError(t, err)
	assert.True(t, overview.CanRequest)
	assert.Empty(t, overview.UnavailableReason)

	requests, total, err := ListTemporaryQuotaRequestsForUser(user.Id, 0, 1, "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, requests, 1)
	assert.Equal(t, "latest request", requests[0].Reason)
	assert.Equal(t, user.Id, requests[0].UserId)

	requests, total, err = ListTemporaryQuotaRequestsForUser(user.Id, 0, 20, "pending", "needle")
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, requests, 1)
	assert.Equal(t, "needle request", requests[0].Reason)
	_, _, err = ListTemporaryQuotaRequestsForUser(user.Id, 0, 20, "unknown", "")
	require.EqualError(t, err, "申请状态不正确")

	require.NoError(t, db.Model(&model.QuotaCycle{}).Where("id = ?", cycle.Id).Updates(map[string]interface{}{
		"status": model.QuotaCycleStatusClosed, "active_key": nil,
	}).Error)
	overview, err = GetTemporaryQuotaOverview(user.Id)
	require.NoError(t, err)
	assert.False(t, overview.CanRequest)
	assert.Equal(t, "No active cycle", overview.UnavailableReason)
	_, total, err = ListTemporaryQuotaRequestsForUser(user.Id, 0, 20, "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
}

func TestFinalZeroCanBeFollowedByTemporaryQuotaGrant(t *testing.T) {
	db := setupQuotaTestDB(t)
	enableCycleQuotaManagementForTest(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "50")
	cycle := model.QuotaCycle{
		CycleStartAt: now - 30*daySeconds, CycleEndAt: now + daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		TemporaryQuotaReserve: mustQuota(t, "100"), Status: model.QuotaCycleStatusActive,
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		ConcentrationMultiplier:                  15_000,
		TemporaryQuotaAutoApprovalEnabled:        true,
		TemporaryQuotaAutoApprovalSingleQuota:    mustQuota(t, "20"),
		TemporaryQuotaAutoApprovalThresholdQuota: mustQuota(t, "10"),
		TemporaryQuotaAutoApprovalMaxCount:       1,
		TemporaryQuotaAutoApprovalMaxQuota:       mustQuota(t, "20"),
	}
	user := model.User{Username: "final-zero", AffCode: "final-zero", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "50"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	params, err := common.Marshal(PlanParameters{ConcentrationMultiplier: 15_000})
	require.NoError(t, err)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, NextAdjustmentAt: &cycle.CycleEndAt, AlgorithmVersion: AlgorithmVersion,
		Parameters: string(params), BudgetQuotaSnapshot: cycle.BudgetQuota,
		PlannedDeltaQuota: -int64(user.Quota),
		Status:            model.QuotaPlanStatusDraft, CreatedAt: now,
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
	result, err := SubmitTemporaryQuotaRequest(TemporaryQuotaRequestInput{
		UserID: user.Id, IdempotencyKey: "after-final-zero", RequestedQuota: mustQuota(t, "20"), Project: "Project A", Reason: "temporary continuity",
	})
	require.NoError(t, err)
	assert.Equal(t, model.TemporaryQuotaRequestStatusExecuted, result.Status)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "20")), user.Quota)
}
