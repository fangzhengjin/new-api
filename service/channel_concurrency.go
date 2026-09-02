package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const concurrencyPoll = 200 * time.Millisecond

var concurrencyLease = time.Minute

var (
	ErrChannelConcurrencyLimit       = errors.New("channel concurrency limit reached")
	ErrChannelConcurrencyUnavailable = errors.New("channel concurrency control unavailable")
	ErrAccountConcurrencyLimit       = errors.New("account concurrency limit reached")
	ErrIPConcurrencyLimit            = errors.New("IP concurrency limit reached")
	ErrModelConcurrencyUnavailable   = errors.New("model request concurrency control unavailable")
)

const acquireConcurrencyScript = `
local now = redis.call('TIME')
local now_ms = now[1] * 1000 + math.floor(now[2] / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms)
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now_ms)
if tonumber(ARGV[1]) > 0 and redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[1]) then
  return 0
end
redis.call('ZREM', KEYS[2], ARGV[2])
redis.call('ZADD', KEYS[1], now_ms + tonumber(ARGV[3]), ARGV[2])
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[3]) * 2)
return 1
`

const addConcurrencyWaiterScript = `
local now = redis.call('TIME')
local now_ms = now[1] * 1000 + math.floor(now[2] / 1000)
redis.call('ZADD', KEYS[1], now_ms + tonumber(ARGV[2]), ARGV[1])
local requested_ttl = tonumber(ARGV[2]) + tonumber(ARGV[3])
if redis.call('PTTL', KEYS[1]) < requested_ttl then
  redis.call('PEXPIRE', KEYS[1], requested_ttl)
end
return 1
`

const countConcurrencyScript = `
local now = redis.call('TIME')
local now_ms = now[1] * 1000 + math.floor(now[2] / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms)
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now_ms)
return {redis.call('ZCARD', KEYS[1]), redis.call('ZCARD', KEYS[2])}
`

const renewConcurrencyScript = `
local now = redis.call('TIME')
local now_ms = now[1] * 1000 + math.floor(now[2] / 1000)
if redis.call('ZSCORE', KEYS[1], ARGV[1]) == false then
  return 0
end
redis.call('ZADD', KEYS[1], now_ms + tonumber(ARGV[2]), ARGV[1])
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[2]) * 2)
return 1
`

// AcquireChannelConcurrency tracks one Redis-backed channel request and enforces a positive limit.
// The parameters are the request context, channel ID, non-negative concurrency limit, and wait timeout.
// It returns a derived context canceled on lease failure, an idempotent release function, and any acquisition error.
func AcquireChannelConcurrency(ctx context.Context, channelID, limit int, waitTimeout time.Duration) (context.Context, func(), error) {
	key := fmt.Sprintf("channel_concurrency:v1:%d", channelID)
	waitingKey := fmt.Sprintf("channel_concurrency_waiting:v1:%d", channelID)
	return acquireRedisConcurrency(ctx, key, waitingKey, limit, waitTimeout, ErrChannelConcurrencyLimit, ErrChannelConcurrencyUnavailable)
}

// AcquireModelRequestConcurrency enforces account then IP concurrency limits for one model request.
// Both limits are non-negative; zero disables that scope. The returned release function is idempotent.
func AcquireModelRequestConcurrency(ctx context.Context, userID int, ip string, accountLimit, ipLimit int) (context.Context, func(), error) {
	limitedCtx := ctx
	releases := make([]func(), 0, 2)
	releaseAll := func() {
		for index := len(releases) - 1; index >= 0; index-- {
			releases[index]()
		}
	}

	if accountLimit > 0 {
		var release func()
		var err error
		limitedCtx, release, err = acquireRedisConcurrency(
			limitedCtx,
			fmt.Sprintf("model_request_concurrency:v1:user:%d", userID),
			fmt.Sprintf("model_request_concurrency_waiting:v1:user:%d", userID),
			accountLimit,
			0,
			ErrAccountConcurrencyLimit,
			ErrModelConcurrencyUnavailable,
		)
		if err != nil {
			return ctx, nil, err
		}
		releases = append(releases, release)
	}

	if ipLimit > 0 {
		var release func()
		var err error
		limitedCtx, release, err = acquireRedisConcurrency(
			limitedCtx,
			"model_request_concurrency:v1:ip:"+ip,
			"model_request_concurrency_waiting:v1:ip:"+ip,
			ipLimit,
			0,
			ErrIPConcurrencyLimit,
			ErrModelConcurrencyUnavailable,
		)
		if err != nil {
			releaseAll()
			return ctx, nil, err
		}
		releases = append(releases, release)
	}
	if len(releases) > 0 {
		go func() {
			<-limitedCtx.Done()
			releaseAll()
		}()
	}

	return limitedCtx, releaseAll, nil
}

func acquireRedisConcurrency(ctx context.Context, key, waitingKey string, limit int, waitTimeout time.Duration, limitError, unavailableError error) (context.Context, func(), error) {
	if common.RDB == nil || !common.RedisEnabled {
		if limit == 0 {
			return ctx, func() {}, nil
		}
		return ctx, nil, unavailableError
	}

	member := common.GetUUID()
	deadline := time.Now().Add(waitTimeout)
	waitingRegistered := false
	defer func() {
		if !waitingRegistered {
			return
		}
		removeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = common.RDB.ZRem(removeCtx, waitingKey, member).Err()
	}()
	for {
		acquired, err := common.RDB.Eval(ctx, acquireConcurrencyScript, []string{key, waitingKey}, limit, member, concurrencyLease.Milliseconds()).Int()
		if err != nil {
			if limit == 0 {
				return ctx, func() {}, nil
			}
			return ctx, nil, fmt.Errorf("%w: %v", unavailableError, err)
		}
		if acquired == 1 {
			break
		}
		if waitTimeout <= 0 || !time.Now().Before(deadline) {
			return ctx, nil, limitError
		}
		if !waitingRegistered {
			remaining := max(time.Until(deadline).Milliseconds(), int64(1))
			if _, err := common.RDB.Eval(ctx, addConcurrencyWaiterScript, []string{waitingKey}, member, remaining, concurrencyLease.Milliseconds()).Result(); err == nil {
				waitingRegistered = true
			}
		}

		wait := min(concurrencyPoll, time.Until(deadline))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx, nil, ctx.Err()
		case <-timer.C:
		}
	}

	leaseCtx, cancel := context.WithCancelCause(ctx)
	done := make(chan struct{})
	var once sync.Once
	release := func() {
		once.Do(func() {
			close(done)
			cancel(nil)
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer releaseCancel()
			_ = common.RDB.ZRem(releaseCtx, key, member).Err()
		})
	}

	go func() {
		ticker := time.NewTicker(concurrencyLease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-leaseCtx.Done():
				release()
				return
			case <-ticker.C:
				renewed, err := common.RDB.Eval(leaseCtx, renewConcurrencyScript, []string{key}, member, concurrencyLease.Milliseconds()).Int()
				if err != nil || renewed != 1 {
					if limit > 0 {
						cancel(unavailableError)
						release()
					}
					return
				}
			}
		}
	}()

	return leaseCtx, release, nil
}

// ChannelConcurrencyCounts contains active and waiting request counts for one channel.
type ChannelConcurrencyCounts struct {
	Active  int64 `json:"active"`
	Waiting int64 `json:"waiting"`
}

// GetChannelConcurrencyCounts returns active and waiting counts for the supplied channel IDs.
// It removes expired entries before reading and returns an error when Redis is unavailable.
func GetChannelConcurrencyCounts(ctx context.Context, channelIDs []int) (map[int]ChannelConcurrencyCounts, error) {
	if common.RDB == nil || !common.RedisEnabled {
		return nil, ErrChannelConcurrencyUnavailable
	}

	commands := make(map[int]*redis.Cmd, len(channelIDs))
	_, err := common.RDB.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, channelID := range channelIDs {
			if channelID <= 0 {
				continue
			}
			key := fmt.Sprintf("channel_concurrency:v1:%d", channelID)
			waitingKey := fmt.Sprintf("channel_concurrency_waiting:v1:%d", channelID)
			commands[channelID] = pipe.Eval(ctx, countConcurrencyScript, []string{key, waitingKey})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrChannelConcurrencyUnavailable, err)
	}

	counts := make(map[int]ChannelConcurrencyCounts, len(commands))
	for channelID, command := range commands {
		values, err := command.Int64Slice()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrChannelConcurrencyUnavailable, err)
		}
		if len(values) != 2 {
			return nil, fmt.Errorf("%w: invalid count response", ErrChannelConcurrencyUnavailable)
		}
		counts[channelID] = ChannelConcurrencyCounts{Active: values[0], Waiting: values[1]}
	}
	return counts, nil
}
