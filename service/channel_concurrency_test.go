package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failChannelConcurrencyRenewalHook struct {
	attempted chan struct{}
}

func (hook failChannelConcurrencyRenewalHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	args := cmd.Args()
	if cmd.Name() == "eval" && len(args) > 1 && args[1] == renewChannelConcurrencyScript {
		select {
		case hook.attempted <- struct{}{}:
		default:
		}
		return ctx, errors.New("renewal unavailable")
	}
	return ctx, nil
}

func (failChannelConcurrencyRenewalHook) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (failChannelConcurrencyRenewalHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (failChannelConcurrencyRenewalHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

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

func TestChannelConcurrencyRenewalDoesNotRestoreReleasedMember(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	const key = "channel_concurrency:v1:12"
	const member = "released-request"
	require.NoError(t, client.ZAdd(context.Background(), key, &redis.Z{Score: 1, Member: member}).Err())
	require.NoError(t, client.ZRem(context.Background(), key, member).Err())

	renewed, err := client.Eval(context.Background(), renewChannelConcurrencyScript, []string{key}, member, channelConcurrencyLease.Milliseconds()).Int()
	require.NoError(t, err)
	assert.Zero(t, renewed)
	assert.ErrorIs(t, client.ZScore(context.Background(), key, member).Err(), redis.Nil)
}

func TestUnlimitedChannelRenewalFailureDoesNotCancelRequest(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	renewalAttempted := make(chan struct{}, 1)
	client.AddHook(failChannelConcurrencyRenewalHook{attempted: renewalAttempted})
	previousRDB, previousEnabled := common.RDB, common.RedisEnabled
	previousLease := channelConcurrencyLease
	common.RDB, common.RedisEnabled = client, true
	channelConcurrencyLease = 30 * time.Millisecond
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = previousRDB, previousEnabled
		channelConcurrencyLease = previousLease
		_ = client.Close()
	})

	requestCtx, release, err := AcquireChannelConcurrency(context.Background(), 13, 0, 0)
	require.NoError(t, err)
	select {
	case <-renewalAttempted:
	case <-time.After(time.Second):
		t.Fatal("unlimited channel lease was not renewed")
	}

	assert.NoError(t, context.Cause(requestCtx))
	release()

	limitedCtx, limitedRelease, err := AcquireChannelConcurrency(context.Background(), 14, 1, 0)
	require.NoError(t, err)
	select {
	case <-renewalAttempted:
	case <-time.After(time.Second):
		t.Fatal("limited channel lease was not renewed")
	}
	select {
	case <-limitedCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("limited channel lease failure did not cancel the request")
	}
	assert.ErrorIs(t, context.Cause(limitedCtx), ErrChannelConcurrencyUnavailable)
	limitedRelease()
}
