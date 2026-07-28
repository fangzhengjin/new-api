package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelConcurrencySharesAndReleasesRedisSlots(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousRDB, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = previousRDB, previousEnabled
		_ = client.Close()
	})

	_, release, err := AcquireChannelConcurrency(context.Background(), 7, 1, 0)
	require.NoError(t, err)
	require.NotNil(t, release)

	_, _, err = AcquireChannelConcurrency(context.Background(), 7, 1, 0)
	assert.ErrorIs(t, err, ErrChannelConcurrencyLimit)

	release()
	_, releaseAgain, err := AcquireChannelConcurrency(context.Background(), 7, 1, 0)
	require.NoError(t, err)
	releaseAgain()
}

func TestChannelConcurrencyWaitsForReleasedSlot(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousRDB, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = previousRDB, previousEnabled
		_ = client.Close()
	})

	_, release, err := AcquireChannelConcurrency(context.Background(), 8, 1, 0)
	require.NoError(t, err)

	type acquireResult struct {
		release func()
		err     error
	}
	result := make(chan acquireResult, 1)
	go func() {
		_, releaseAgain, acquireErr := AcquireChannelConcurrency(context.Background(), 8, 1, time.Second)
		result <- acquireResult{release: releaseAgain, err: acquireErr}
	}()

	require.Eventually(t, func() bool {
		counts, countErr := GetChannelConcurrencyCounts(context.Background(), []int{8})
		return countErr == nil && counts[8].Active == 1 && counts[8].Waiting == 1
	}, time.Second, 10*time.Millisecond)

	release()
	acquired := <-result
	require.NoError(t, acquired.err)
	require.NotNil(t, acquired.release)
	counts, err := GetChannelConcurrencyCounts(context.Background(), []int{8})
	require.NoError(t, err)
	assert.Equal(t, ChannelConcurrencyCounts{Active: 1}, counts[8])
	acquired.release()
}

func TestChannelConcurrencyTracksUnlimitedRequests(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousRDB, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = previousRDB, previousEnabled
		_ = client.Close()
	})

	_, releaseFirst, err := AcquireChannelConcurrency(context.Background(), 9, 0, 0)
	require.NoError(t, err)
	_, releaseSecond, err := AcquireChannelConcurrency(context.Background(), 9, 0, 0)
	require.NoError(t, err)

	counts, err := GetChannelConcurrencyCounts(context.Background(), []int{9, 10})
	require.NoError(t, err)
	assert.Equal(t, ChannelConcurrencyCounts{Active: 2}, counts[9])
	assert.Equal(t, ChannelConcurrencyCounts{}, counts[10])

	releaseFirst()
	counts, err = GetChannelConcurrencyCounts(context.Background(), []int{9})
	require.NoError(t, err)
	assert.Equal(t, ChannelConcurrencyCounts{Active: 1}, counts[9])
	releaseSecond()
}

func TestUnlimitedChannelTrackingDoesNotRequireRedis(t *testing.T) {
	previousRDB, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = nil, false
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = previousRDB, previousEnabled
	})

	_, release, err := AcquireChannelConcurrency(context.Background(), 11, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, release)
	release()

	_, _, err = AcquireChannelConcurrency(context.Background(), 11, 1, 0)
	assert.ErrorIs(t, err, ErrChannelConcurrencyUnavailable)
}
