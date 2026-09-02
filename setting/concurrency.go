package setting

import (
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

const (
	ModelRequestConcurrencyLimitEnabledOptionKey = "ModelRequestConcurrencyLimitEnabled"
	ModelRequestConcurrencyLimitOptionKey        = "ModelRequestConcurrencyLimit"
	ModelRequestIPConcurrencyLimitOptionKey      = "ModelRequestIPConcurrencyLimit"
	MaxModelRequestConcurrency                   = dto.MaxChannelConcurrency
)

var ModelRequestConcurrencyLimitEnabled = false
var ModelRequestConcurrencyLimit = 0
var ModelRequestIPConcurrencyLimit = 0

type ModelRequestConcurrencyLimits struct {
	Enabled       bool
	AccountLimit  int
	AccountSource string
	IPLimit       int
}

// ResolveModelRequestConcurrencyLimits applies the per-user account override and global IP limit.
func ResolveModelRequestConcurrencyLimits(userSetting dto.UserSetting) ModelRequestConcurrencyLimits {
	limits := ModelRequestConcurrencyLimits{
		Enabled:       ModelRequestConcurrencyLimitEnabled,
		AccountLimit:  ModelRequestConcurrencyLimit,
		AccountSource: RateLimitSourceGlobal,
		IPLimit:       ModelRequestIPConcurrencyLimit,
	}
	if value := userSetting.ModelRequestConcurrencyLimit; value != nil && *value >= 0 && *value <= MaxModelRequestConcurrency {
		limits.AccountLimit = *value
		limits.AccountSource = RateLimitSourceUser
	}
	return limits
}

// CheckModelRequestConcurrencyLimit validates a persisted model request concurrency limit.
func CheckModelRequestConcurrencyLimit(value string) error {
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 0 || limit > MaxModelRequestConcurrency {
		return fmt.Errorf("模型请求并发限制必须为 0 至 %d 的整数", MaxModelRequestConcurrency)
	}
	return nil
}
