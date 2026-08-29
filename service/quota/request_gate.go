package quota

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var quotaAdmissionMu sync.RWMutex
var quotaSettlementTasks sync.WaitGroup

var quotaRequestLease = time.Minute

const quotaRequestObservationTimeout = 2 * time.Second

var errQuotaRequestObservationUnavailable = errors.New("额度请求观测不可用")
var errQuotaCycleHasInFlightWork = errors.New("额度周期仍有在途工作")

const acquireQuotaRequestLeaseScript = `
local now = redis.call('TIME')
local now_ms = now[1] * 1000 + math.floor(now[2] / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms)
redis.call('ZADD', KEYS[1], now_ms + tonumber(ARGV[2]), ARGV[1])
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[2]) * 2)
return 1
`

const releaseQuotaRequestLeaseScript = `
return redis.call('ZREM', KEYS[1], ARGV[1])
`

const renewQuotaRequestLeaseScript = `
local now = redis.call('TIME')
local now_ms = now[1] * 1000 + math.floor(now[2] / 1000)
if redis.call('ZSCORE', KEYS[1], ARGV[1]) == false then
  return 0
end
redis.call('ZADD', KEYS[1], now_ms + tonumber(ARGV[2]), ARGV[1])
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[2]) * 2)
return 1
`

const countQuotaRequestLeasesScript = `
local now = redis.call('TIME')
local now_ms = now[1] * 1000 + math.floor(now[2] / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms)
return redis.call('ZCARD', KEYS[1])
`

func settlementLeadSeconds() int64 {
	minutes := operation_setting.GetQuotaSetting().SettlementLeadMinutes
	if minutes < 3 || minutes > 60 {
		minutes = 10
	}
	return int64(minutes * 60)
}

func settlementStartsAt(cycle *model.QuotaCycle) int64 {
	return cycle.CycleEndAt - settlementLeadSeconds()
}

func settlementPrompt() string {
	prompt := operation_setting.GetQuotaSetting().SettlementPrompt
	if prompt == "" {
		return "本期额度正在结算，暂时无法发起新请求，请稍后重试"
	}
	return prompt
}

func loadQuotaAdmissionCycle() (*model.QuotaCycle, error) {
	var cycle model.QuotaCycle
	err := model.DB.Where("status IN ?", []model.QuotaCycleStatus{
		model.QuotaCycleStatusActive, model.QuotaCycleStatusSettling,
	}).Order("cycle_start_at DESC").First(&cycle).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cycle, nil
}

func quotaRequestLeaseKey(cycleID int) string {
	return fmt.Sprintf("quota_settlement:{%d}:requests:v1", cycleID)
}

func markQuotaRequestObservationFailed(cycleID int) {
	failedAt := time.Now().Unix()
	if err := model.DB.Model(&model.QuotaCycle{}).
		Where("id = ? AND status IN ? AND request_observation_failed_at IS NULL", cycleID, []model.QuotaCycleStatus{
			model.QuotaCycleStatusActive, model.QuotaCycleStatusSettling,
		}).
		Update("request_observation_failed_at", failedAt).Error; err != nil {
		common.SysError("failed to mark quota request observation unavailable: " + err.Error())
	}
}

func settleQuotaCycleIfDrained(cycleID int) {
	quotaSettlementTasks.Add(1)
	gopool.Go(func() {
		defer quotaSettlementTasks.Done()
		if err := settleDrainedCycle(cycleID, time.Now().Unix()); err != nil {
			common.SysError("failed to settle drained quota cycle: " + err.Error())
		}
	})
}

func maintainQuotaRequestLease(ctx context.Context, cycleID int, key, member string) func() error {
	done := make(chan struct{})
	stopped := make(chan struct{})
	release := func() error {
		close(done)
		<-stopped
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), quotaRequestObservationTimeout)
		defer releaseCancel()
		_, err := common.RDB.Eval(releaseCtx, releaseQuotaRequestLeaseScript, []string{key}, member).Result()
		return err
	}

	go func() {
		defer close(stopped)
		ticker := time.NewTicker(quotaRequestLease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				renewCtx, renewCancel := context.WithTimeout(ctx, quotaRequestObservationTimeout)
				renewed, err := common.RDB.Eval(renewCtx, renewQuotaRequestLeaseScript, []string{key}, member, quotaRequestLease.Milliseconds()).Int()
				renewCancel()
				if err != nil {
					common.SysError("failed to renew quota request observation: " + err.Error())
					markQuotaRequestObservationFailed(cycleID)
					settleQuotaCycleIfDrained(cycleID)
					return
				}
				if renewed != 1 {
					common.SysError("failed to renew quota request observation: lease member is missing")
					markQuotaRequestObservationFailed(cycleID)
					settleQuotaCycleIfDrained(cycleID)
					return
				}
			}
		}
	}()

	return release
}

func acquireQuotaRequestLease(ctx context.Context, cycleID int) (func(), error) {
	if common.RDB == nil || !common.RedisEnabled {
		return nil, errQuotaRequestObservationUnavailable
	}

	key := quotaRequestLeaseKey(cycleID)
	member := common.GetUUID()
	observationCtx := context.WithoutCancel(ctx)
	acquireCtx, cancel := context.WithTimeout(observationCtx, quotaRequestObservationTimeout)
	defer cancel()
	_, err := common.RDB.Eval(acquireCtx, acquireQuotaRequestLeaseScript,
		[]string{key}, member, quotaRequestLease.Milliseconds()).Result()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errQuotaRequestObservationUnavailable, err)
	}
	releaseLease := maintainQuotaRequestLease(observationCtx, cycleID, key, member)
	var once sync.Once
	return func() {
		once.Do(func() {
			if releaseErr := releaseLease(); releaseErr != nil {
				common.SysError("failed to release quota lease: " + releaseErr.Error())
				markQuotaRequestObservationFailed(cycleID)
				settleQuotaCycleIfDrained(cycleID)
				return
			}
			remaining, countErr := activeQuotaRequestCount(cycleID)
			if countErr != nil {
				common.SysError("failed to count quota request leases after release: " + countErr.Error())
				markQuotaRequestObservationFailed(cycleID)
				settleQuotaCycleIfDrained(cycleID)
				return
			}
			if remaining == 0 {
				settleQuotaCycleIfDrained(cycleID)
			}
		})
	}, nil
}

// AdmitQuotaRequestDuringSettlement blocks new requests only during the settlement window.
func AdmitQuotaRequestDuringSettlement(c *gin.Context) (func(), error) {
	if !model.CycleQuotaManagementEnabled() {
		return func() {}, nil
	}

	quotaAdmissionMu.RLock()
	defer quotaAdmissionMu.RUnlock()
	now := time.Now().Unix()
	cycle, err := loadQuotaAdmissionCycle()
	if err != nil {
		common.SysError("failed to load quota admission cycle: " + err.Error())
		return func() {}, nil
	}
	if cycle == nil || now < cycle.CycleStartAt {
		return func() {}, nil
	}
	if cycle.Status == model.QuotaCycleStatusSettling || now >= settlementStartsAt(cycle) {
		return nil, errors.New(settlementPrompt())
	}
	releaseLease := func() {}
	if cycle.RequestObservationFailedAt == nil {
		if release, leaseErr := acquireQuotaRequestLease(c.Request.Context(), cycle.Id); leaseErr != nil {
			common.SysError("failed to acquire quota request lease: " + leaseErr.Error())
			markQuotaRequestObservationFailed(cycle.Id)
		} else {
			releaseLease = release
		}
	}

	var current model.QuotaCycle
	if err := model.DB.Select("id", "cycle_start_at", "cycle_end_at", "status").First(&current, cycle.Id).Error; err != nil {
		common.SysError("failed to confirm quota admission cycle: " + err.Error())
		return releaseLease, nil
	}
	now = time.Now().Unix()
	if current.Status != model.QuotaCycleStatusActive || now < current.CycleStartAt {
		releaseLease()
		if current.Status == model.QuotaCycleStatusSettling {
			return nil, errors.New(settlementPrompt())
		}
		return func() {}, nil
	}
	if now >= settlementStartsAt(&current) {
		releaseLease()
		return nil, errors.New(settlementPrompt())
	}
	return releaseLease, nil
}

func activeQuotaRequestCount(cycleID int) (int, error) {
	if common.RDB == nil || !common.RedisEnabled {
		return 0, errQuotaRequestObservationUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), quotaRequestObservationTimeout)
	defer cancel()
	count, err := common.RDB.Eval(ctx, countQuotaRequestLeasesScript, []string{quotaRequestLeaseKey(cycleID)}).Int()
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errQuotaRequestObservationUnavailable, err)
	}
	return count, nil
}

// AcquireQuotaSettlementBillingLease keeps one asynchronous billing update visible to cycle settlement.
func AcquireQuotaSettlementBillingLease(ctx context.Context, billingAt int64) func() {
	if !model.CycleQuotaManagementEnabled() {
		return func() {}
	}
	if billingAt <= 0 {
		return func() {}
	}

	var cycle model.QuotaCycle
	err := model.DB.Select("id", "request_observation_failed_at").Where(
		"status IN ? AND cycle_start_at <= ? AND cycle_end_at > ?",
		[]model.QuotaCycleStatus{model.QuotaCycleStatusActive, model.QuotaCycleStatusSettling}, billingAt, billingAt,
	).Order("cycle_start_at DESC").First(&cycle).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return func() {}
	}
	if err != nil {
		common.SysError("failed to locate quota settlement billing cycle: " + err.Error())
		return func() {}
	}
	if cycle.RequestObservationFailedAt == nil {
		release, acquireErr := acquireQuotaRequestLease(ctx, cycle.Id)
		if acquireErr == nil {
			return release
		}
		common.SysError("failed to observe quota settlement billing work: " + acquireErr.Error())
		markQuotaRequestObservationFailed(cycle.Id)
	}
	return sync.OnceFunc(func() {
		if settleErr := settleDrainedCycle(cycle.Id, time.Now().Unix()); settleErr != nil {
			common.SysError("failed to settle unobserved quota billing work: " + settleErr.Error())
		}
	})
}

func cycleHasInFlightWork(cycle *model.QuotaCycle) (bool, error) {
	var taskID int64
	if err := model.DB.Model(&model.Task{}).
		Where("submit_time >= ? AND submit_time < ?", cycle.CycleStartAt, cycle.CycleEndAt).
		Where("progress != ?", "100%").
		Where("status NOT IN ?", []model.TaskStatus{model.TaskStatusFailure, model.TaskStatusSuccess}).
		Limit(1).Pluck("id", &taskID).Error; err != nil {
		return false, err
	}
	if taskID != 0 {
		return true, nil
	}
	var midjourneyID int
	if err := model.DB.Model(&model.Midjourney{}).
		Where("(submit_time >= ? AND submit_time < ?) OR (submit_time >= ? AND submit_time < ?)",
			cycle.CycleStartAt*1000, cycle.CycleEndAt*1000, cycle.CycleStartAt, cycle.CycleEndAt).
		Where("progress != ?", "100%").
		Where("status NOT IN ?", []string{string(model.TaskStatusFailure), string(model.TaskStatusSuccess)}).
		Limit(1).Pluck("id", &midjourneyID).Error; err != nil {
		return false, err
	}
	if midjourneyID != 0 {
		return true, nil
	}
	if cycle.RequestObservationFailedAt == nil {
		activeRequests, err := activeQuotaRequestCount(cycle.Id)
		if err == nil && activeRequests > 0 {
			return true, nil
		}
		if err != nil {
			common.SysError("failed to count quota settlement request observations: " + err.Error())
			markQuotaRequestObservationFailed(cycle.Id)
		}
	}
	return false, nil
}
