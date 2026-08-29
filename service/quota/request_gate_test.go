package quota

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type quotaRenewalDeadlineHook struct {
	observed chan bool
}

func (hook quotaRenewalDeadlineHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	args := cmd.Args()
	if cmd.Name() == "eval" && len(args) > 1 && args[1] == renewQuotaRequestLeaseScript {
		_, hasDeadline := ctx.Deadline()
		hook.observed <- hasDeadline
		return ctx, errors.New("renewal unavailable")
	}
	return ctx, nil
}

func (quotaRenewalDeadlineHook) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (quotaRenewalDeadlineHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (quotaRenewalDeadlineHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func configureQuotaLeaseRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousRDB, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() {
		quotaSettlementTasks.Wait()
		common.RDB, common.RedisEnabled = previousRDB, previousEnabled
		_ = client.Close()
	})
	return server
}

func quotaGateTestContext() *gin.Context {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	return context
}

func TestQuotaRequestGateOnlyBlocksSettlementForEveryUser(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureQuotaLeaseRedis(t)
	settings := operation_setting.GetQuotaSetting()
	previousMode := operation_setting.CycleQuotaManagementEnabled
	previousLead := settings.SettlementLeadMinutes
	previousPrompt := settings.SettlementPrompt
	operation_setting.CycleQuotaManagementEnabled = true
	settings.SettlementLeadMinutes = 10
	settings.SettlementPrompt = "额度结算测试提示"
	t.Cleanup(func() {
		operation_setting.CycleQuotaManagementEnabled = previousMode
		settings.SettlementLeadMinutes = previousLead
		settings.SettlementPrompt = previousPrompt
	})

	now := time.Now().Unix()
	release, err := AdmitQuotaRequestDuringSettlement(quotaGateTestContext())
	require.NoError(t, err)
	release()

	cycle := model.QuotaCycle{
		CycleStartAt: now + 60, CycleEndAt: now + 1800,
		BudgetQuota: 1000, BalancePolicy: model.QuotaCycleBalancePolicyCarry,
		Status: model.QuotaCycleStatusScheduled,
	}
	require.NoError(t, db.Create(&cycle).Error)
	release, err = AdmitQuotaRequestDuringSettlement(quotaGateTestContext())
	require.NoError(t, err)
	release()

	require.NoError(t, db.Model(&model.QuotaCycle{}).Where("id = ?", cycle.Id).Updates(map[string]interface{}{
		"cycle_start_at": now - 60, "status": model.QuotaCycleStatusActive,
	}).Error)
	release, err = AdmitQuotaRequestDuringSettlement(quotaGateTestContext())
	require.NoError(t, err)
	secondRelease, err := AdmitQuotaRequestDuringSettlement(quotaGateTestContext())
	require.NoError(t, err)
	activeRequests, err := activeQuotaRequestCount(cycle.Id)
	require.NoError(t, err)
	assert.Equal(t, 2, activeRequests)
	release()
	activeRequests, err = activeQuotaRequestCount(cycle.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, activeRequests)
	secondRelease()
	activeRequests, err = activeQuotaRequestCount(cycle.Id)
	require.NoError(t, err)
	assert.Zero(t, activeRequests)

	release, err = AdmitQuotaRequestDuringSettlement(quotaGateTestContext())
	require.NoError(t, err)
	activeRequests, err = activeQuotaRequestCount(cycle.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, activeRequests)
	release()
	activeRequests, err = activeQuotaRequestCount(cycle.Id)
	require.NoError(t, err)
	assert.Zero(t, activeRequests)

	require.NoError(t, db.Model(&model.QuotaCycle{}).Where("id = ?", cycle.Id).Update("cycle_end_at", now+300).Error)
	_, err = AdmitQuotaRequestDuringSettlement(quotaGateTestContext())
	require.EqualError(t, err, "额度结算测试提示")
	whitelistUser := model.User{
		Username: "settlement-whitelist", AffCode: "settlement-whitelist",
		Status: common.UserStatusEnabled, QuotaWhitelist: true,
	}
	require.NoError(t, db.Create(&whitelistUser).Error)
	whitelistContext := quotaGateTestContext()
	whitelistContext.Set("id", whitelistUser.Id)
	_, err = AdmitQuotaRequestDuringSettlement(whitelistContext)
	require.EqualError(t, err, "额度结算测试提示")

	require.NoError(t, db.Model(&model.QuotaCycle{}).Where("id = ?", cycle.Id).Updates(map[string]interface{}{
		"status": model.QuotaCycleStatusClosed, "active_key": nil,
	}).Error)
	release, err = AdmitQuotaRequestDuringSettlement(quotaGateTestContext())
	require.NoError(t, err)
	release()
}

func TestQuotaRequestGateAllowsActiveRequestsWithoutRedis(t *testing.T) {
	db := setupQuotaTestDB(t)
	settings := operation_setting.GetQuotaSetting()
	previousMode := operation_setting.CycleQuotaManagementEnabled
	previousLead := settings.SettlementLeadMinutes
	operation_setting.CycleQuotaManagementEnabled = true
	settings.SettlementLeadMinutes = 10
	t.Cleanup(func() {
		operation_setting.CycleQuotaManagementEnabled = previousMode
		settings.SettlementLeadMinutes = previousLead
	})

	release, err := AdmitQuotaRequestDuringSettlement(quotaGateTestContext())
	require.NoError(t, err)
	release()

	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 1800,
		BudgetQuota: 1000, BalancePolicy: model.QuotaCycleBalancePolicyCarry,
		Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)
	release, err = AdmitQuotaRequestDuringSettlement(quotaGateTestContext())
	require.NoError(t, err)
	release()
}

func TestQuotaRequestGateFailsOpenWhenCycleStateIsUnavailable(t *testing.T) {
	db := setupQuotaTestDB(t)
	require.NoError(t, db.Migrator().DropTable(&model.QuotaCycle{}))

	release, err := AdmitQuotaRequestDuringSettlement(quotaGateTestContext())
	require.NoError(t, err)
	release()
}

func TestQuotaSettlementBillingLeaseTracksSettlingCycleWork(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureQuotaLeaseRedis(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })

	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60, BudgetQuota: 1000,
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusSettling,
	}
	require.NoError(t, db.Create(&cycle).Error)

	release := AcquireQuotaSettlementBillingLease(context.Background(), now)
	var err error
	activeRequests, err := activeQuotaRequestCount(cycle.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, activeRequests)
	release()
	require.Eventually(t, func() bool {
		return db.First(&cycle, cycle.Id).Error == nil && cycle.Status == model.QuotaCycleStatusClosed
	}, time.Second, 10*time.Millisecond)
}

func TestQuotaSettlementBillingLeaseReleaseFailureDoesNotDelayCycle(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureQuotaLeaseRedis(t)
	configureLifecycleTest(t)
	previousMode := operation_setting.CycleQuotaManagementEnabled
	operation_setting.CycleQuotaManagementEnabled = true
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previousMode })

	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60, BudgetQuota: 1000,
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)

	release := AcquireQuotaSettlementBillingLease(context.Background(), now)
	var err error
	activeRequests, err := activeQuotaRequestCount(cycle.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, activeRequests)
	require.NoError(t, beginCycleSettlement(cycle.Id, now, "system"))
	result, err := RunQuotaLifecycle(now)
	require.NoError(t, err)
	assert.Empty(t, result.ClosedCycleIDs)
	require.NoError(t, common.RDB.Close())
	release()
	require.Eventually(t, func() bool {
		return db.First(&cycle, cycle.Id).Error == nil && cycle.Status == model.QuotaCycleStatusClosed
	}, time.Second, 10*time.Millisecond)
}

func TestQuotaSettlementBillingLeaseSkipsRedisAfterObservationFailure(t *testing.T) {
	db := setupQuotaTestDB(t)
	server := configureQuotaLeaseRedis(t)
	now := time.Now().Unix()
	failedAt := now - 1
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60, BudgetQuota: 1000,
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusSettling,
		RequestObservationFailedAt: &failedAt,
	}
	require.NoError(t, db.Create(&cycle).Error)

	commandCount := server.CommandCount()
	release := AcquireQuotaSettlementBillingLease(context.Background(), now)
	assert.Equal(t, commandCount, server.CommandCount())
	release()
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	assert.Equal(t, model.QuotaCycleStatusClosed, cycle.Status)
}

func TestQuotaRequestObservationKeepsCancelledBusinessContext(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureQuotaLeaseRedis(t)
	configureLifecycleTest(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60, BudgetQuota: 1000,
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusSettling,
	}
	require.NoError(t, db.Create(&cycle).Error)
	businessCtx, cancel := context.WithCancel(context.Background())
	cancel()
	release, err := acquireQuotaRequestLease(businessCtx, cycle.Id)
	require.NoError(t, err)
	release()
	require.Eventually(t, func() bool {
		return db.First(&cycle, cycle.Id).Error == nil && cycle.Status == model.QuotaCycleStatusClosed
	}, time.Second, 10*time.Millisecond)
}

func TestQuotaRequestLeaseBoundsEachRedisRenewal(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureQuotaLeaseRedis(t)
	observed := make(chan bool, 1)
	common.RDB.AddHook(quotaRenewalDeadlineHook{observed: observed})
	previousLease := quotaRequestLease
	quotaRequestLease = 30 * time.Millisecond
	t.Cleanup(func() { quotaRequestLease = previousLease })

	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 60, BudgetQuota: 1000,
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusSettling,
	}
	require.NoError(t, db.Create(&cycle).Error)
	release, err := acquireQuotaRequestLease(context.Background(), cycle.Id)
	require.NoError(t, err)

	select {
	case hasDeadline := <-observed:
		assert.True(t, hasDeadline)
	case <-time.After(time.Second):
		t.Fatal("quota request lease was not renewed")
	}
	release()
	require.Eventually(t, func() bool {
		return db.First(&cycle, cycle.Id).Error == nil && cycle.Status == model.QuotaCycleStatusClosed
	}, time.Second, 10*time.Millisecond)
}

func TestQuotaLifecycleWaitsForSharedRequestLease(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureQuotaLeaseRedis(t)
	configureLifecycleTest(t)
	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 60, CycleEndAt: now + 1800,
		BudgetQuota: 1000, InitialGrantQuota: 100,
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusActive,
	}
	require.NoError(t, db.Create(&cycle).Error)

	release, err := AdmitQuotaRequestDuringSettlement(quotaGateTestContext())
	require.NoError(t, err)
	require.NoError(t, beginCycleSettlement(cycle.Id, now, "system"))
	result, err := RunQuotaLifecycle(now)
	require.NoError(t, err)
	assert.Empty(t, result.ClosedCycleIDs)

	release()
	require.Eventually(t, func() bool {
		return db.First(&cycle, cycle.Id).Error == nil && cycle.Status == model.QuotaCycleStatusClosed
	}, time.Second, 10*time.Millisecond)
}

func TestQuotaLifecycleClosesImmediatelyWhenRedisIsUnavailable(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureLifecycleTest(t)
	previousEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousEnabled })

	now := time.Now().Unix()
	cycle := model.QuotaCycle{
		CycleStartAt: now - 3600, CycleEndAt: now - 1,
		BudgetQuota: 1000, InitialGrantQuota: 100,
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusSettling,
	}
	require.NoError(t, db.Create(&cycle).Error)

	result, err := RunQuotaLifecycle(now)
	require.NoError(t, err)
	assert.Equal(t, []int{cycle.Id}, result.ClosedCycleIDs)
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	assert.Equal(t, model.QuotaCycleStatusClosed, cycle.Status)
}

func TestRecoveredRedisLeaseCannotDelayCycleAfterObservationFailure(t *testing.T) {
	db := setupQuotaTestDB(t)
	configureQuotaLeaseRedis(t)
	configureLifecycleTest(t)
	now := time.Now().Unix()
	failedAt := now - int64(2*quotaRequestLease/time.Second)
	cycle := model.QuotaCycle{
		CycleStartAt: now - 3600, CycleEndAt: now + 3600,
		BudgetQuota: 1000, InitialGrantQuota: 100,
		BalancePolicy: model.QuotaCycleBalancePolicyCarry, Status: model.QuotaCycleStatusActive,
		RequestObservationFailedAt: &failedAt,
	}
	require.NoError(t, db.Create(&cycle).Error)

	release, err := AdmitQuotaRequestDuringSettlement(quotaGateTestContext())
	require.NoError(t, err)
	release()
	require.NoError(t, common.RDB.ZAdd(context.Background(), quotaRequestLeaseKey(cycle.Id), &redis.Z{
		Score: float64(time.Now().Add(time.Minute).UnixMilli()), Member: "stale-request",
	}).Err())

	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	require.NotNil(t, cycle.RequestObservationFailedAt)
	require.NoError(t, beginCycleSettlement(cycle.Id, now, "system"))
	result, err := RunQuotaLifecycle(now)
	require.NoError(t, err)
	assert.Equal(t, []int{cycle.Id}, result.ClosedCycleIDs)
	require.NoError(t, db.First(&cycle, cycle.Id).Error)
	assert.Equal(t, model.QuotaCycleStatusClosed, cycle.Status)
}
