package quota

import (
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
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 7_500,
		SnapshotAt: now, AlgorithmVersion: AlgorithmVersion, Status: model.QuotaPlanStatusDraft,
	}
	require.NoError(t, db.Create(&otherDraft).Error)

	var affected map[int]int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		affected, err = executePlanInTransaction(tx, plan.Id, plan.CycleId, "root", now+60)
		return err
	}))
	assert.Equal(t, map[int]int64{user.Id: item.AdjustmentQuota}, affected)

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

func TestExecutePlanRejectsOrdinaryDecreaseBelowRetainedQuota(t *testing.T) {
	db := setupQuotaTestDB(t)
	now := time.Now().Unix()
	_, user, plan, item := seedExecutableDecrease(t, db, now)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", int(mustQuota(t, "90"))).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, executeErr := executePlanInTransaction(tx, plan.Id, plan.CycleId, "root", now+60)
		return executeErr
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "保留方案核定额度")

	var storedUser model.User
	var storedPlan model.QuotaPlan
	var storedItem model.QuotaItem
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	require.NoError(t, db.First(&storedPlan, plan.Id).Error)
	require.NoError(t, db.First(&storedItem, item.Id).Error)
	assert.Equal(t, int(mustQuota(t, "90")), storedUser.Quota)
	assert.Equal(t, model.QuotaPlanStatusDraft, storedPlan.Status)
	assert.Nil(t, storedItem.ActualAfterQuota)
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
	cycle := model.QuotaCycle{
		CycleStartAt: now - 7*daySeconds, CycleEndAt: now + 30*daySeconds,
		BudgetQuota: mustQuota(t, "1000"), InitialGrantQuota: mustQuota(t, "200"),
		Status: model.QuotaCycleStatusActive, CreatedBy: "root",
	}
	require.NoError(t, db.Create(&cycle).Error)
	user := model.User{
		Username: "alice", DisplayName: "Alice", Email: "alice@example.com", AffCode: "alice-code",
		Status: common.UserStatusEnabled, Quota: int(mustQuota(t, "100")), CreatedAt: now - daySeconds,
	}
	require.NoError(t, db.Create(&user).Error)
	plan := model.QuotaPlan{
		CycleId: cycle.Id, PlanType: model.QuotaPlanTypeAdjustment, StagePercent: 7_500,
		SnapshotAt: now, AlgorithmVersion: AlgorithmVersion, BudgetQuotaSnapshot: cycle.BudgetQuota,
		ManagedBalanceQuota: int64(user.Quota), PlannedDeltaQuota: -mustQuota(t, "20"),
		Status: model.QuotaPlanStatusDraft, CreatedAt: now, CreatedBy: "root",
	}
	require.NoError(t, db.Create(&plan).Error)
	item := model.QuotaItem{
		PlanId: plan.Id, UserId: user.Id, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
		Action: model.QuotaAdjustmentActionDecrease, SnapshotBalanceQuota: int64(user.Quota),
		AdjustmentQuota: -mustQuota(t, "20"), RetainedQuota: mustQuota(t, "80"),
		CalculationData: `{"decrease_kind":"ordinary"}`, BasisText: "本期无使用记录；当前为第1次调配",
		LogStatus: model.QuotaNotificationStatusPending, EmailStatus: model.QuotaNotificationStatusPending,
	}
	require.NoError(t, db.Create(&item).Error)
	return cycle, user, plan, item
}
