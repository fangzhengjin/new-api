package setting

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimitResolversApplyPerFieldPrecedenceAndPreserveZero(t *testing.T) {
	previousTotal := ModelRequestRateLimitCount
	previousSuccess := ModelRequestRateLimitSuccessCount
	previousGroups := ModelRequestRateLimitGroup
	previousConcurrencyEnabled := ModelRequestConcurrencyLimitEnabled
	previousConcurrency := ModelRequestConcurrencyLimit
	previousIPConcurrency := ModelRequestIPConcurrencyLimit
	previousSourceEnabled := AccessSourceLimitEnabled
	previousMaxIPs := AccessSourceMaxIPsPerUser
	previousCooldown := AccessSourceSwitchCooldownMinutes
	ModelRequestRateLimitCount = 100
	ModelRequestRateLimitSuccessCount = 80
	ModelRequestRateLimitGroup = map[string][2]int{"vip": {50, 40}}
	ModelRequestConcurrencyLimitEnabled = true
	ModelRequestConcurrencyLimit = 6
	ModelRequestIPConcurrencyLimit = 4
	AccessSourceLimitEnabled = true
	AccessSourceMaxIPsPerUser = 4
	AccessSourceSwitchCooldownMinutes = 30
	t.Cleanup(func() {
		ModelRequestRateLimitCount = previousTotal
		ModelRequestRateLimitSuccessCount = previousSuccess
		ModelRequestRateLimitGroup = previousGroups
		ModelRequestConcurrencyLimitEnabled = previousConcurrencyEnabled
		ModelRequestConcurrencyLimit = previousConcurrency
		ModelRequestIPConcurrencyLimit = previousIPConcurrency
		AccessSourceLimitEnabled = previousSourceEnabled
		AccessSourceMaxIPsPerUser = previousMaxIPs
		AccessSourceSwitchCooldownMinutes = previousCooldown
	})

	zero := 0
	three := 3
	userSetting := dto.UserSetting{
		ModelRequestRateLimitCount:        &zero,
		ModelRequestConcurrencyLimit:      &three,
		AccessSourceSwitchCooldownMinutes: &zero,
	}
	rateLimits := ResolveModelRequestRateLimits("vip", userSetting)
	assert.Equal(t, 0, rateLimits.TotalCount)
	assert.Equal(t, RateLimitSourceUser, rateLimits.TotalSource)
	assert.Equal(t, 40, rateLimits.SuccessCount)
	assert.Equal(t, RateLimitSourceGroup, rateLimits.SuccessSource)
	concurrencyLimits := ResolveModelRequestConcurrencyLimits(userSetting)
	assert.True(t, concurrencyLimits.Enabled)
	assert.Equal(t, 3, concurrencyLimits.AccountLimit)
	assert.Equal(t, RateLimitSourceUser, concurrencyLimits.AccountSource)
	assert.Equal(t, 4, concurrencyLimits.IPLimit)

	accessLimits := ResolveAccessSourceLimits(userSetting)
	assert.Equal(t, 4, accessLimits.MaxIPsPerUser)
	assert.Equal(t, RateLimitSourceGlobal, accessLimits.MaxIPsPerUserSource)
	assert.Equal(t, 0, accessLimits.SwitchCooldownMinutes)
	assert.Equal(t, RateLimitSourceUser, accessLimits.SwitchCooldownSource)
}

func TestUpdateModelRequestRateLimitGroupRejectsInvalidJSONWithoutClearingCurrentValue(t *testing.T) {
	ModelRequestRateLimitMutex.Lock()
	previous := ModelRequestRateLimitGroup
	ModelRequestRateLimitGroup = map[string][2]int{"default": {20, 10}}
	ModelRequestRateLimitMutex.Unlock()
	t.Cleanup(func() {
		ModelRequestRateLimitMutex.Lock()
		ModelRequestRateLimitGroup = previous
		ModelRequestRateLimitMutex.Unlock()
	})

	require.Error(t, UpdateModelRequestRateLimitGroupByJSONString("not-json"))
	total, success, found := GetGroupRateLimit("default")
	assert.True(t, found)
	assert.Equal(t, 20, total)
	assert.Equal(t, 10, success)
}
