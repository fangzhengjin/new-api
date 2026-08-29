package quota

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestExecutePlanCommitsBalanceLogAndStateTogether(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle, user, plan, item := seedExecutableDecrease(t, db, now)
	otherDraft := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, AlgorithmVersion: AlgorithmVersion, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&otherDraft).Error)

	var commit *executionCommit
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		commit, err = executePlanInTransaction(tx, plan.Id, plan.CycleId, "root", now+60)
		return err
	}))
	assert.Equal(t, map[int]int64{user.Id: item.AdjustmentQuota}, commit.QuotaDeltas)
	assert.Zero(t, commit.IssuedQuota)
	assert.Equal(t, mustQuota(t, "20"), commit.ReclaimedQuota)
	assert.Equal(t, mustQuota(t, "80"), commit.AllocatedQuota)

	var storedUser model.User
	var storedPlan, cancelledPlan model.QuotaPlan
	var storedItem model.QuotaItem
	var logs []model.Log
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	require.NoError(t, db.First(&storedPlan, plan.Id).Error)
	require.NoError(t, db.First(&cancelledPlan, otherDraft.Id).Error)
	require.NoError(t, db.First(&storedItem, item.Id).Error)
	require.NoError(t, db.Where("request_id = ?", quotaLogRequestID(plan.Id, item.Id)).Find(&logs).Error)
	assert.Equal(t, int(mustQuota(t, "80")), storedUser.Quota)
	assert.Equal(t, model.QuotaPlanStatusExecuted, storedPlan.Status)
	assert.Equal(t, model.QuotaPlanStatusCancelled, cancelledPlan.Status)
	assert.Equal(t, model.QuotaNotificationStatusSent, storedItem.LogStatus)
	assert.Equal(t, model.QuotaNotificationStatusPending, storedItem.EmailStatus)
	require.NotNil(t, storedItem.ActualBeforeQuota)
	require.NotNil(t, storedItem.ActualAfterQuota)
	assert.Equal(t, mustQuota(t, "100"), *storedItem.ActualBeforeQuota)
	assert.Equal(t, mustQuota(t, "80"), *storedItem.ActualAfterQuota)
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0].Content, "本次调减：＄20.000000")
}

func TestExecutePlanAppliesDecreaseAfterConsumptionWhenBalanceCoversDelta(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	_, user, plan, item := seedExecutableDecrease(t, db, now)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", int(mustQuota(t, "90"))).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, plan.CycleId, "root", now+60)
		return executeErr
	}))

	var storedUser model.User
	var storedPlan model.QuotaPlan
	var storedItem model.QuotaItem
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	require.NoError(t, db.First(&storedPlan, plan.Id).Error)
	require.NoError(t, db.First(&storedItem, item.Id).Error)
	assert.Equal(t, int(mustQuota(t, "70")), storedUser.Quota)
	assert.Equal(t, model.QuotaPlanStatusExecuted, storedPlan.Status)
	require.NotNil(t, storedItem.ActualAfterQuota)
	assert.Equal(t, mustQuota(t, "70"), *storedItem.ActualAfterQuota)
}

func TestExecutePlanRejectsDecreaseWhenLiveBalanceCannotCoverDelta(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	_, user, plan, _ := seedExecutableDecrease(t, db, now)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", int(mustQuota(t, "10"))).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, plan.CycleId, "root", now+60)
		return executeErr
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "余额不足以扣除方案额度")

	var storedUser model.User
	var storedPlan model.QuotaPlan
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	require.NoError(t, db.First(&storedPlan, plan.Id).Error)
	assert.Equal(t, int(mustQuota(t, "10")), storedUser.Quota)
	assert.Equal(t, model.QuotaPlanStatusDraft, storedPlan.Status)
}

func TestExecutePlanRejectsAtCycleEndBoundary(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle, _, plan, _ := seedExecutableDecrease(t, db, now)
	cycleEnd := now + 60
	require.NoError(t, db.Model(&model.QuotaCycle{}).Where("id = ?", cycle.Id).Update("cycle_end_at", cycleEnd).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, cycle.Id, "root", cycleEnd)
		return executeErr
	})
	require.EqualError(t, err, "当前时间已到或超过周期结束时间，不得执行跨周期方案")
}

func TestExecuteAutomaticIncreaseRejectsLivePositionAboveCycleCap(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 10*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "20000"), InitialGrantQuota: mustQuota(t, "10000"),
		ConcentrationMultiplier: 15_000, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	user := model.User{Username: "live-position", AffCode: "live-position-code", Status: common.UserStatusEnabled, CreatedAt: now - 60*daySeconds}
	require.NoError(t, db.Create(&user).Error)
	executedAt := now - 7*daySeconds
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	nextAdjustment := now + 7*daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	positionCeiling, err := concentrationPositionCeiling(cycle.BudgetQuota, 1, cycle.ConcentrationMultiplier)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", positionCeiling).Error)

	err = db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, result.Plan.Id, cycle.Id, "root", now+60)
		return executeErr
	})
	require.EqualError(t, err, fmt.Sprintf("用户 %d 的当前余额或本期消费已使自动调增方案过期，请重新生成", user.Id))
}

func TestExecuteAutomaticIncreaseAllowsConsumptionAfterSnapshot(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "50")
	cycle := model.QuotaCycle{
		CycleStartAt: now - 10*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "20000"), InitialGrantQuota: mustQuota(t, "10000"),
		ConcentrationMultiplier: 15_000, OpeningAllocatedQuota: &allocated,
		AllocatedQuota: &allocated, AllocationBaselineAt: &now, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	user := model.User{
		Username: "spent-position", AffCode: "spent-position-code", Status: common.UserStatusEnabled,
		Quota: int(allocated), CreatedAt: now - 60*daySeconds,
	}
	require.NoError(t, db.Create(&user).Error)
	executedAt := now - 7*daySeconds
	require.NoError(t, db.Create(&model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeInitialization,
		Status: model.QuotaPlanStatusExecuted, ExecutedAt: &executedAt,
	}).Error)
	nextAdjustment := now + 7*daySeconds
	result, err := GeneratePlan(GenerateParams{
		CycleID: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		NextAdjustmentAt: &nextAdjustment, CreatedBy: "root",
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Positive(t, result.Items[0].AdjustmentQuota)
	expectedBalance := result.Items[0].AdjustmentQuota
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", 0).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: user.Id, Username: user.Username, Type: model.LogTypeConsume,
		Quota: int(allocated), CreatedAt: now + 1,
	}).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, result.Plan.Id, cycle.Id, "root", now+60)
		return executeErr
	}))
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, int(expectedBalance), user.Quota)
}

func TestExecutePlanCanReissueReclaimedQuota(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "800")
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	users := []model.User{
		{Username: "source", AffCode: "freeze-source", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "800"))},
		{Username: "recipient", AffCode: "freeze-recipient", Status: common.UserStatusEnabled},
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&users).Error)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, AlgorithmVersion: AlgorithmVersion, Parameters: `{}`, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&plan).Error)
	items := []model.QuotaItem{
		{PlanId: plan.Id, UserId: users[0].Id, Username: users[0].Username, Action: model.QuotaAdjustmentActionDecrease, SnapshotBalanceQuota: int64(users[0].Quota), AdjustmentQuota: -mustQuota(t, "100")},
		{PlanId: plan.Id, UserId: users[1].Id, Username: users[1].Username, Action: model.QuotaAdjustmentActionGrant, SnapshotBalanceQuota: 0, AdjustmentQuota: mustQuota(t, "10")},
	}
	require.NoError(t, db.Create(&items).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, cycle.Id, "root", now+60)
		return executeErr
	}))

	var stored []model.User
	require.NoError(t, db.Order("id").Find(&stored).Error)
	assert.Equal(t, int(mustQuota(t, "700")), stored[0].Quota)
	assert.Equal(t, int(mustQuota(t, "10")), stored[1].Quota)
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	require.NotNil(t, cycle.AllocatedQuota)
	assert.Equal(t, mustQuota(t, "710"), *cycle.AllocatedQuota)
}

func TestExecutePlanKeepsTemporaryQuotaReserveForOrdinaryItems(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "890")
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		TemporaryQuotaReserve: mustQuota(t, "100"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "ordinary-reserve", AffCode: "ordinary-reserve", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "890"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, AlgorithmVersion: AlgorithmVersion, Parameters: `{}`, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.QuotaItem{
		PlanId: plan.Id, UserId: user.Id, Username: user.Username, Action: model.QuotaAdjustmentActionIncrease,
		SnapshotBalanceQuota: int64(user.Quota), AdjustmentQuota: mustQuota(t, "20"),
	}).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, cycle.Id, "root", now+60)
		return executeErr
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "占用临时额度预留")
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "890")), user.Quota)
}

func TestExecutePlanAllowsTemporaryQuotaItemsToUseReservedCapacity(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "890")
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		TemporaryQuotaReserve: mustQuota(t, "100"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "temporary-reserve", AffCode: "temporary-reserve", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "890"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	request := model.TemporaryQuotaRequest{
		CycleId: cycle.Id, UserId: user.Id, Username: user.Username,
		IdempotencyKey: "temporary-reserve-request", RequestedQuota: mustQuota(t, "20"),
		Status: model.TemporaryQuotaRequestStatusPending,
	}
	require.NoError(t, db.Create(&request).Error)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, AlgorithmVersion: AlgorithmVersion,
		Parameters: fmt.Sprintf(`{"temporary_quota_request_id":%d}`, request.Id), Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.QuotaItem{
		PlanId: plan.Id, UserId: user.Id, Username: user.Username, Action: model.QuotaAdjustmentActionTemporaryGrant,
		SnapshotBalanceQuota: int64(user.Quota), AdjustmentQuota: mustQuota(t, "20"),
	}).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, cycle.Id, "root", now+60)
		return executeErr
	}))
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "910")), user.Quota)
}

func TestExecutePlanRejectsNegativeTemporaryQuota(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "100")
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
		BudgetQuota: mustQuota(t, "1000"), TemporaryQuotaReserve: mustQuota(t, "100"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "negative-temporary-grant", AffCode: "negative-temporary-grant", Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "50"))}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	request := model.TemporaryQuotaRequest{
		CycleId: cycle.Id, UserId: user.Id, Username: user.Username,
		IdempotencyKey: "negative-temporary-request", RequestedQuota: mustQuota(t, "20"),
		Status: model.TemporaryQuotaRequestStatusPending,
	}
	require.NoError(t, db.Create(&request).Error)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, AlgorithmVersion: AlgorithmVersion,
		Parameters: fmt.Sprintf(`{"temporary_quota_request_id":%d}`, request.Id), Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.QuotaItem{
		PlanId: plan.Id, UserId: user.Id, Username: user.Username,
		Action: model.QuotaAdjustmentActionTemporaryGrant, AdjustmentQuota: -mustQuota(t, "20"),
	}).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, cycle.Id, "root", now+60)
		return executeErr
	})
	require.EqualError(t, err, fmt.Sprintf("用户 %d 的临时额度发放条件已变化", user.Id))
}

func TestExecutePlanKeepsTemporaryGrantOutOfOrdinaryReserve(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "800")
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		TemporaryQuotaReserve: mustQuota(t, "200"), OpeningAllocatedQuota: &allocated,
		AllocatedQuota: &allocated, AllocationBaselineAt: &now, Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "ordinary-after-temporary", AffCode: "ordinary-after-temporary", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.TemporaryQuotaRequest{
		CycleId: cycle.Id, UserId: user.Id, Username: user.Username,
		IdempotencyKey: "ordinary-after-temporary", RequestedQuota: mustQuota(t, "200"),
		ApprovedQuota: mustQuota(t, "200"), Status: model.TemporaryQuotaRequestStatusExecuted,
	}).Error)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, AlgorithmVersion: AlgorithmVersion, Parameters: `{}`, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.QuotaItem{
		PlanId: plan.Id, UserId: user.Id, Username: user.Username, Action: model.QuotaAdjustmentActionIncrease,
		AdjustmentQuota: mustQuota(t, "100"),
	}).Error)

	var commit *executionCommit
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var executeErr error
		commit, executeErr = executePlanInTransaction(tx, plan.Id, cycle.Id, "root", now+60)
		return executeErr
	}))
	assert.Equal(t, mustQuota(t, "900"), commit.AllocatedQuota)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, int(mustQuota(t, "100")), user.Quota)
}

func TestExecutePlanReturnsReclaimedTemporaryQuotaToOrdinaryPool(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "900")
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		TemporaryQuotaReserve: mustQuota(t, "200"), OpeningAllocatedQuota: &allocated,
		AllocatedQuota: &allocated, AllocationBaselineAt: &now, Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "ordinary-after-reclaim", AffCode: "ordinary-after-reclaim", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.TemporaryQuotaRequest{
		CycleId: cycle.Id, UserId: user.Id, Username: user.Username,
		IdempotencyKey: "reclaimed-temporary", RequestedQuota: mustQuota(t, "200"),
		ApprovedQuota: mustQuota(t, "200"), Status: model.TemporaryQuotaRequestStatusExecuted,
	}).Error)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, AlgorithmVersion: AlgorithmVersion, Parameters: `{}`, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.QuotaItem{
		PlanId: plan.Id, UserId: user.Id, Username: user.Username,
		Action: model.QuotaAdjustmentActionIncrease, AdjustmentQuota: mustQuota(t, "100"),
	}).Error)

	var commit *executionCommit
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var executeErr error
		commit, executeErr = executePlanInTransaction(tx, plan.Id, cycle.Id, "root", now+60)
		return executeErr
	}))
	assert.Equal(t, mustQuota(t, "1000"), commit.AllocatedQuota)
	used, err := temporaryQuotaReserveUsed(db, cycle.Id)
	require.NoError(t, err)
	assert.Equal(t, mustQuota(t, "200"), used)
}

func TestExecutePlanRequiresTemporaryQuotaRequestForTemporaryGrant(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	allocated := mustQuota(t, "100")
	cycle := model.QuotaCycle{
		CycleStartAt: now - daySeconds, CycleEndAt: now + daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "100"),
		OpeningAllocatedQuota: &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive,
	}
	user := model.User{Username: "invalid-temporary-grant", AffCode: "invalid-temporary-grant", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, db.Create(&user).Error)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, AlgorithmVersion: AlgorithmVersion,
		Parameters: `{}`, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.QuotaItem{
		PlanId: plan.Id, UserId: user.Id, Username: user.Username,
		Action: model.QuotaAdjustmentActionTemporaryGrant, AdjustmentQuota: mustQuota(t, "20"),
	}).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, cycle.Id, "root", now+60)
		return executeErr
	})
	require.EqualError(t, err, "临时额度申请参数不正确")
}

func TestExecutePlanRejectsOldAlgorithmVersion(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle, _, plan, _ := seedExecutableDecrease(t, db, now)
	require.NoError(t, db.Model(&model.QuotaPlan{}).Where("id = ?", plan.Id).Update("algorithm_version", "1.7.0").Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, cycle.Id, "root", now+60)
		return executeErr
	})
	require.EqualError(t, err, "该草稿的调配规则版本与当前版本不一致，请重新生成后再执行")
}

func TestExecutePlanRejectsMissingParameters(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle, _, plan, _ := seedExecutableDecrease(t, db, now)
	require.NoError(t, db.Model(&model.QuotaPlan{}).Where("id = ?", plan.Id).Update("parameters", "").Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, cycle.Id, "root", now+60)
		return executeErr
	})
	require.ErrorContains(t, err, "方案参数无效")
}

func TestExecutePlanRejectsChangedConcentrationMultiplier(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	cycle, _, plan, _ := seedExecutableDecrease(t, db, now)
	require.NoError(t, db.Model(&model.QuotaCycle{}).Where("id = ?", cycle.Id).Updates(map[string]interface{}{
		"concentration_multiplier": 15_000,
	}).Error)
	require.NoError(t, db.Model(&model.QuotaPlan{}).Where("id = ?", plan.Id).Updates(map[string]interface{}{
		"algorithm_version": AlgorithmVersion,
		"parameters":        `{"concentration_multiplier_basis_points":20000}`,
	}).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, cycle.Id, "root", now+60)
		return executeErr
	})
	require.EqualError(t, err, "方案记录的自动分配上限与周期设置不一致，请重新生成")
}

func TestSplitLogOutboxConvergesByDeterministicRequestID(t *testing.T) {
	mainDB := setupQuotaTestDB(t)
	logDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, logDB.AutoMigrate(&model.Log{}))
	model.LOG_DB = logDB

	now := time.Now().Unix()
	_, _, plan, item := seedExecutableDecrease(t, mainDB, now)
	require.NoError(t, mainDB.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, plan.CycleId, "root", now+60)
		return executeErr
	}))

	var storedItem model.QuotaItem
	require.NoError(t, mainDB.First(&storedItem, item.Id).Error)
	assert.Equal(t, model.QuotaNotificationStatusPending, storedItem.LogStatus)
	requestID := quotaLogRequestID(plan.Id, item.Id)
	require.NoError(t, logDB.Create(&model.Log{
		UserId: item.UserId, Username: item.Username, CreatedAt: now + 60,
		Type: model.LogTypeManage, Content: storedItem.LogContent, RequestId: requestID,
	}).Error)

	assert.True(t, deliverQuotaLog(plan.Id, item.Id))
	require.NoError(t, mainDB.First(&storedItem, item.Id).Error)
	assert.Equal(t, model.QuotaNotificationStatusSent, storedItem.LogStatus)
	var logCount int64
	require.NoError(t, logDB.Model(&model.Log{}).Where("request_id = ?", requestID).Count(&logCount).Error)
	assert.Equal(t, int64(1), logCount)
}

func TestSplitLogOutboxRejectsRequestIDCollision(t *testing.T) {
	mainDB := setupQuotaTestDB(t)
	logDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, logDB.AutoMigrate(&model.Log{}))
	model.LOG_DB = logDB

	now := time.Now().Unix()
	_, _, plan, item := seedExecutableDecrease(t, mainDB, now)
	require.NoError(t, mainDB.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, plan.CycleId, "root", now+60)
		return executeErr
	}))
	requestID := quotaLogRequestID(plan.Id, item.Id)
	require.NoError(t, logDB.Create(&model.Log{
		UserId: item.UserId, Username: item.Username, CreatedAt: now + 60,
		Type: model.LogTypeConsume, Content: "unrelated", RequestId: requestID,
	}).Error)

	assert.False(t, deliverQuotaLog(plan.Id, item.Id))
	var storedItem model.QuotaItem
	require.NoError(t, mainDB.First(&storedItem, item.Id).Error)
	assert.Equal(t, model.QuotaNotificationStatusFailed, storedItem.LogStatus)
	assert.Contains(t, storedItem.LogError, "request_id")
}

func seedExecutableDecrease(t *testing.T, db *gorm.DB, now int64) (model.QuotaCycle, model.User, model.QuotaPlan, model.QuotaItem) {
	t.Helper()
	allocated := mustQuota(t, "100")
	cycle := model.QuotaCycle{
		CycleStartAt: now - 7*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "200"),
		ConcentrationMultiplier: 15_000,
		OpeningAllocatedQuota:   &allocated, AllocatedQuota: &allocated, AllocationBaselineAt: &now,
		Status: model.QuotaCycleStatusActive, CreatedBy: "root",
	}
	require.NoError(t, db.Create(&cycle).Error)
	user := model.User{
		Username: "alice", DisplayName: "Alice", Email: "alice@example.com", AffCode: "alice-code",
		Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "100")), CreatedAt: now - daySeconds,
	}
	require.NoError(t, db.Create(&user).Error)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment,
		SnapshotAt: now, AlgorithmVersion: AlgorithmVersion, BudgetQuotaSnapshot: cycle.BudgetQuota,
		Parameters:        `{"concentration_multiplier_basis_points":15000}`,
		PlannedDeltaQuota: -mustQuota(t, "20"),
		Status:            model.QuotaPlanStatusDraft, CreatedAt: now, CreatedBy: "root",
	}
	require.NoError(t, db.Create(&plan).Error)
	item := model.QuotaItem{
		PlanId: plan.Id, UserId: user.Id, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
		Action: model.QuotaAdjustmentActionDecrease, SnapshotBalanceQuota: int64(user.Quota),
		AdjustmentQuota: -mustQuota(t, "20"),
		CalculationData: `{"decrease_kind":"ordinary"}`, BasisText: "本期无使用记录\n当前为第1次调配",
		LogStatus: model.QuotaNotificationStatusPending, EmailStatus: model.QuotaNotificationStatusPending,
	}
	require.NoError(t, db.Create(&item).Error)
	return cycle, user, plan, item
}
