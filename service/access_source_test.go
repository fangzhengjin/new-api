package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetAccessSourceMemory(t *testing.T) {
	t.Helper()
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	accessSourceMemory.mutex.Lock()
	accessSourceMemory.accounts = make(map[int]*memoryAccessSourceAccount)
	accessSourceMemory.ips = make(map[string]map[int]int64)
	accessSourceMemory.nextCleanup = 0
	accessSourceMemory.sequence = 0
	accessSourceMemory.mutex.Unlock()
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })
}

func enabledAccessSourceLimits() setting.AccessSourceLimits {
	return setting.AccessSourceLimits{
		Enabled:                true,
		AssociationWindowHours: 24,
		MaxIPsPerUser:          1,
		SwitchCooldownMinutes:  60,
		MaxUsersPerIP:          1,
	}
}

func TestCheckAccessSourceRejectsInvalidIP(t *testing.T) {
	decision, err := CheckAccessSource(context.Background(), 1, "not-an-ip", enabledAccessSourceLimits())
	assert.False(t, decision.Allowed)
	assert.ErrorIs(t, err, ErrAccessSourceInvalidIP)
}

func TestCheckAccessSourceDoesNotStoreWhenAllRulesAreUnlimited(t *testing.T) {
	resetAccessSourceMemory(t)
	limits := enabledAccessSourceLimits()
	limits.MaxIPsPerUser = 0
	limits.SwitchCooldownMinutes = 0
	limits.MaxUsersPerIP = 0

	decision, err := CheckAccessSource(context.Background(), 1, "192.0.2.1", limits)
	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	state, err := GetAccessSourceState(context.Background(), 1, limits)
	require.NoError(t, err)
	assert.Empty(t, state.Associations)
}

func TestAccessSourceMemoryRejectsWithoutChangingAcceptedState(t *testing.T) {
	resetAccessSourceMemory(t)
	limits := enabledAccessSourceLimits()
	now := time.Unix(1_800_000_000, 0)

	accepted, err := checkAccessSourceMemory(1, "192.0.2.1", limits, now)
	require.NoError(t, err)
	require.True(t, accepted.Allowed)
	rejected, err := checkAccessSourceMemory(1, "192.0.2.2", limits, now.Add(time.Minute))
	require.NoError(t, err)
	assert.False(t, rejected.Allowed)
	assert.Equal(t, AccessSourceSwitchCooldown, rejected.Reason)

	state, err := getAccessSourceStateMemory(1, limits, now.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.1", state.CurrentIP)
	assert.Equal(t, int64(1), state.AssociatedCount)
	require.NotNil(t, state.Pending)
	assert.Equal(t, "192.0.2.2", state.Pending.IP)
	_, err = allowLatestAccessSourceMemory(1, "192.0.2.2", "older-event", now.Add(2*time.Minute))
	assert.ErrorIs(t, err, ErrAccessSourcePendingChanged)

	_, err = allowLatestAccessSourceMemory(1, state.Pending.IP, state.Pending.EventID, now.Add(2*time.Minute))
	require.NoError(t, err)
	_, err = removeAccessSourceAssociationMemory(1, "192.0.2.2", now.Add(3*time.Minute))
	assert.ErrorIs(t, err, ErrAccessSourceCurrent)
	removed, err := removeAccessSourceAssociationMemory(1, "192.0.2.1", now.Add(3*time.Minute))
	require.NoError(t, err)
	assert.True(t, removed)
}

func TestAccessSourceMemoryEnforcesIPAccountLimit(t *testing.T) {
	resetAccessSourceMemory(t)
	limits := enabledAccessSourceLimits()
	limits.SwitchCooldownMinutes = 0
	now := time.Unix(1_800_000_000, 0)

	first, err := checkAccessSourceMemory(1, "198.51.100.1", limits, now)
	require.NoError(t, err)
	require.True(t, first.Allowed)
	second, err := checkAccessSourceMemory(2, "198.51.100.1", limits, now)
	require.NoError(t, err)
	assert.False(t, second.Allowed)
	assert.Equal(t, AccessSourceIPAccountLimit, second.Reason)
}

func TestAccessSourceMemoryPrioritizesAccountLimitBeforeIPLimit(t *testing.T) {
	resetAccessSourceMemory(t)
	limits := enabledAccessSourceLimits()
	limits.SwitchCooldownMinutes = 0
	now := time.Unix(1_800_000_000, 0)

	first, err := checkAccessSourceMemory(1, "198.51.100.1", limits, now)
	require.NoError(t, err)
	require.True(t, first.Allowed)
	second, err := checkAccessSourceMemory(2, "198.51.100.2", limits, now)
	require.NoError(t, err)
	require.True(t, second.Allowed)
	rejected, err := checkAccessSourceMemory(1, "198.51.100.2", limits, now.Add(time.Minute))
	require.NoError(t, err)
	assert.False(t, rejected.Allowed)
	assert.Equal(t, AccessSourceAccountIPLimit, rejected.Reason)
}

func TestAccessSourceMemoryRechecksExpiredRelationshipAndClearsAcceptedPending(t *testing.T) {
	resetAccessSourceMemory(t)
	limits := enabledAccessSourceLimits()
	limits.AssociationWindowHours = 1
	limits.SwitchCooldownMinutes = 0
	limits.MaxUsersPerIP = 0
	now := time.Unix(1_800_000_000, 0)

	first, err := checkAccessSourceMemory(1, "192.0.2.1", limits, now)
	require.NoError(t, err)
	require.True(t, first.Allowed)
	second, err := checkAccessSourceMemory(1, "192.0.2.2", limits, now.Add(2*time.Hour))
	require.NoError(t, err)
	require.True(t, second.Allowed)

	rejected, err := checkAccessSourceMemory(1, "192.0.2.1", limits, now.Add(2*time.Hour+time.Minute))
	require.NoError(t, err)
	assert.False(t, rejected.Allowed)
	assert.Equal(t, AccessSourceAccountIPLimit, rejected.Reason)
	acceptedCurrent, err := checkAccessSourceMemory(1, "192.0.2.2", limits, now.Add(2*time.Hour+2*time.Minute))
	require.NoError(t, err)
	require.True(t, acceptedCurrent.Allowed)
	state, err := getAccessSourceStateMemory(1, limits, now.Add(2*time.Hour+2*time.Minute))
	require.NoError(t, err)
	require.NotNil(t, state.Pending)
	assert.Equal(t, "192.0.2.1", state.Pending.IP)

	accepted, err := checkAccessSourceMemory(1, "192.0.2.1", limits, now.Add(4*time.Hour))
	require.NoError(t, err)
	assert.True(t, accepted.Allowed)
	state, err = getAccessSourceStateMemory(1, limits, now.Add(4*time.Hour))
	require.NoError(t, err)
	assert.Nil(t, state.Pending)

	limits.MaxIPsPerUser = 0
	limits.MaxUsersPerIP = 1
	later := now.Add(10 * time.Hour)
	firstAccount, err := checkAccessSourceMemory(11, "192.0.2.10", limits, later)
	require.NoError(t, err)
	require.True(t, firstAccount.Allowed)
	secondAccount, err := checkAccessSourceMemory(12, "192.0.2.10", limits, later.Add(2*time.Hour))
	require.NoError(t, err)
	require.True(t, secondAccount.Allowed)
	rejectedAccount, err := checkAccessSourceMemory(11, "192.0.2.10", limits, later.Add(2*time.Hour+time.Minute))
	require.NoError(t, err)
	assert.False(t, rejectedAccount.Allowed)
	assert.Equal(t, AccessSourceIPAccountLimit, rejectedAccount.Reason)
}

func TestAccessSourceMemoryKeepsActiveRelationshipsAfterLimitReduced(t *testing.T) {
	resetAccessSourceMemory(t)
	limits := enabledAccessSourceLimits()
	limits.MaxIPsPerUser = 2
	limits.SwitchCooldownMinutes = 0
	limits.MaxUsersPerIP = 0
	now := time.Unix(1_800_000_000, 0)

	for index, ip := range []string{"192.0.2.1", "192.0.2.2"} {
		decision, err := checkAccessSourceMemory(1, ip, limits, now.Add(time.Duration(index)*time.Minute))
		require.NoError(t, err)
		require.True(t, decision.Allowed)
	}
	limits.MaxIPsPerUser = 1
	for index, ip := range []string{"192.0.2.1", "192.0.2.2"} {
		decision, err := checkAccessSourceMemory(1, ip, limits, now.Add(time.Duration(index+2)*time.Minute))
		require.NoError(t, err)
		assert.True(t, decision.Allowed)
	}
	decision, err := checkAccessSourceMemory(1, "192.0.2.3", limits, now.Add(4*time.Minute))
	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, AccessSourceAccountIPLimit, decision.Reason)
}

func TestAccessSourceStateUsesCurrentAssociationWindow(t *testing.T) {
	resetAccessSourceMemory(t)
	now := time.Unix(1_800_000_000, 0)
	limits := enabledAccessSourceLimits()
	limits.MaxIPsPerUser = 0
	limits.MaxUsersPerIP = 0
	limits.SwitchCooldownMinutes = 0

	_, err := checkAccessSourceMemory(1, "203.0.113.1", limits, now.Add(-2*time.Hour))
	require.NoError(t, err)
	limits.AssociationWindowHours = 1
	shortState, err := getAccessSourceStateMemory(1, limits, now)
	require.NoError(t, err)
	assert.Zero(t, shortState.AssociatedCount)
	limits.AssociationWindowHours = 6
	longState, err := getAccessSourceStateMemory(1, limits, now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), longState.AssociatedCount)
}

func TestAccessSourceRejectionHistoryKeepsLatestTen(t *testing.T) {
	resetAccessSourceMemory(t)
	now := time.Unix(1_800_000_000, 0)
	limits := enabledAccessSourceLimits()
	require.NoError(t, func() error {
		_, err := checkAccessSourceMemory(1, "192.0.2.1", limits, now)
		return err
	}())
	for index := 0; index < 12; index++ {
		_, err := checkAccessSourceMemory(1, fmt.Sprintf("192.0.2.%d", index+20), limits, now.Add(time.Duration(index+1)*time.Second))
		require.NoError(t, err)
	}
	state, err := getAccessSourceStateMemory(1, limits, now.Add(20*time.Second))
	require.NoError(t, err)
	require.Len(t, state.RecentRejections, 10)
	assert.Equal(t, "192.0.2.31", state.RecentRejections[0].IP)
	require.NotNil(t, state.Pending)
	assert.Equal(t, state.RecentRejections[0].EventID, state.Pending.EventID)
}

func TestAccessSourceRedisAtomicallyLimitsNewAccountIPs(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		_ = client.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
	})

	limits := enabledAccessSourceLimits()
	limits.SwitchCooldownMinutes = 0
	limits.MaxUsersPerIP = 0
	var allowed atomic.Int32
	var group sync.WaitGroup
	errorsSeen := make(chan error, 20)
	for index := 0; index < 20; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			decision, err := CheckAccessSource(context.Background(), 9, fmt.Sprintf("203.0.113.%d", index+1), limits)
			if err != nil {
				errorsSeen <- err
				return
			}
			if decision.Allowed {
				allowed.Add(1)
			}
		}(index)
	}
	group.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), allowed.Load())
	state, err := GetAccessSourceState(context.Background(), 9, limits)
	require.NoError(t, err)
	assert.Equal(t, int64(1), state.AssociatedCount)
	assert.True(t, server.Exists(accessSourceAccountKey(9)))
	assert.Equal(t, 7*24*time.Hour, server.TTL(accessSourceAccountKey(9)))
	assert.Equal(t, 24*time.Hour, server.TTL(accessSourceCurrentKey(9)))
	assert.Equal(t, 24*time.Hour, server.TTL(accessSourceHistoryKey(9)))
	assert.Equal(t, 24*time.Hour, server.TTL(accessSourcePendingKey(9)))

	_, err = AllowLatestAccessSource(context.Background(), 9, "203.0.113.20", "stale")
	assert.True(t, errors.Is(err, ErrAccessSourcePendingChanged))
}

func TestAccessSourceRedisRechecksExpiredRelationshipAndClearsAcceptedPending(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	server := miniredis.RunT(t)
	server.SetTime(time.Unix(1_800_000_000, 0))
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		_ = client.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
	})
	limits := enabledAccessSourceLimits()
	limits.AssociationWindowHours = 1
	limits.SwitchCooldownMinutes = 0
	limits.MaxUsersPerIP = 0

	first, err := CheckAccessSource(context.Background(), 10, "198.51.100.1", limits)
	require.NoError(t, err)
	require.True(t, first.Allowed)
	server.SetTime(time.Unix(1_800_000_000, 0).Add(2 * time.Hour))
	second, err := CheckAccessSource(context.Background(), 10, "198.51.100.2", limits)
	require.NoError(t, err)
	require.True(t, second.Allowed)

	server.SetTime(time.Unix(1_800_000_000, 0).Add(2*time.Hour + time.Minute))
	rejected, err := CheckAccessSource(context.Background(), 10, "198.51.100.1", limits)
	require.NoError(t, err)
	assert.False(t, rejected.Allowed)
	assert.Equal(t, AccessSourceAccountIPLimit, rejected.Reason)
	server.SetTime(time.Unix(1_800_000_000, 0).Add(2*time.Hour + 2*time.Minute))
	acceptedCurrent, err := CheckAccessSource(context.Background(), 10, "198.51.100.2", limits)
	require.NoError(t, err)
	require.True(t, acceptedCurrent.Allowed)
	state, err := GetAccessSourceState(context.Background(), 10, limits)
	require.NoError(t, err)
	require.NotNil(t, state.Pending)
	assert.Equal(t, "198.51.100.1", state.Pending.IP)

	server.SetTime(time.Unix(1_800_000_000, 0).Add(4 * time.Hour))
	accepted, err := CheckAccessSource(context.Background(), 10, "198.51.100.1", limits)
	require.NoError(t, err)
	assert.True(t, accepted.Allowed)
	state, err = GetAccessSourceState(context.Background(), 10, limits)
	require.NoError(t, err)
	assert.Nil(t, state.Pending)

	limits.MaxIPsPerUser = 0
	limits.MaxUsersPerIP = 1
	later := time.Unix(1_800_000_000, 0).Add(10 * time.Hour)
	server.SetTime(later)
	firstAccount, err := CheckAccessSource(context.Background(), 11, "198.51.100.10", limits)
	require.NoError(t, err)
	require.True(t, firstAccount.Allowed)
	server.SetTime(later.Add(2 * time.Hour))
	secondAccount, err := CheckAccessSource(context.Background(), 12, "198.51.100.10", limits)
	require.NoError(t, err)
	require.True(t, secondAccount.Allowed)
	server.SetTime(later.Add(2*time.Hour + time.Minute))
	rejectedAccount, err := CheckAccessSource(context.Background(), 11, "198.51.100.10", limits)
	require.NoError(t, err)
	assert.False(t, rejectedAccount.Allowed)
	assert.Equal(t, AccessSourceIPAccountLimit, rejectedAccount.Reason)
}

func TestAccessSourceRedisKeepsActiveRelationshipsAfterLimitReduced(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	server := miniredis.RunT(t)
	now := time.Unix(1_800_000_000, 0)
	server.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		_ = client.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
	})
	limits := enabledAccessSourceLimits()
	limits.MaxIPsPerUser = 2
	limits.SwitchCooldownMinutes = 0
	limits.MaxUsersPerIP = 0

	for index, ip := range []string{"198.51.100.1", "198.51.100.2"} {
		server.SetTime(now.Add(time.Duration(index) * time.Minute))
		decision, err := CheckAccessSource(context.Background(), 20, ip, limits)
		require.NoError(t, err)
		require.True(t, decision.Allowed)
	}
	limits.MaxIPsPerUser = 1
	for index, ip := range []string{"198.51.100.1", "198.51.100.2"} {
		server.SetTime(now.Add(time.Duration(index+2) * time.Minute))
		decision, err := CheckAccessSource(context.Background(), 20, ip, limits)
		require.NoError(t, err)
		assert.True(t, decision.Allowed)
	}
	server.SetTime(now.Add(4 * time.Minute))
	decision, err := CheckAccessSource(context.Background(), 20, "198.51.100.3", limits)
	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, AccessSourceAccountIPLimit, decision.Reason)
}
