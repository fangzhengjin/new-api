package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/quota"
)

// quotaLifecycleHandler closes and activates quota cycles independently of
// the admin page. It lives in this fork-only file so system_task_handlers.go
// stays identical to upstream except for the single Register call.
type quotaLifecycleHandler struct{}

func (quotaLifecycleHandler) Type() string { return model.SystemTaskTypeQuotaLifecycle }

func (quotaLifecycleHandler) Enabled() bool {
	due, err := quota.QuotaLifecycleDue(time.Now().Unix())
	if err != nil {
		common.SysError(fmt.Sprintf("quota lifecycle due check failed: %v", err))
		return false
	}
	return due
}

func (quotaLifecycleHandler) Interval() time.Duration { return time.Minute }

func (quotaLifecycleHandler) NewPayload() any { return nil }

func (quotaLifecycleHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	select {
	case <-ctx.Done():
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, context.Cause(ctx))
		return
	default:
	}
	result, err := quota.RunQuotaLifecycle(time.Now().Unix())
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, result, nil)
}
