package middleware

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestIPRateLimitCountMark      = "MRRLIP"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
	ModelRequestIPRateLimitSuccessMark    = "MRRLSIP"
)

const redisSuccessLimitReserveScript = `
local now = redis.call('TIME')
local now_ms = now[1] * 1000 + math.floor(now[2] / 1000)
local window_ms = tonumber(ARGV[2]) * 1000
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms - window_ms)
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[1]) then
  local oldest = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
  local retry_after = math.ceil((tonumber(oldest[2]) + window_ms - now_ms) / 1000)
  return {0, math.max(retry_after, 1)}
end
redis.call('ZADD', KEYS[1], now_ms, ARGV[3])
redis.call('EXPIRE', KEYS[1], ARGV[2])
return {1, tonumber(ARGV[2])}
`

type successLimitReservation struct {
	key    string
	member string
	rdb    *redis.Client
}

func reserveRedisSuccessLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64, member string) (bool, int64, error) {
	values, err := rdb.Eval(ctx, redisSuccessLimitReserveScript, []string{key}, maxCount, duration, member).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(values) != 2 {
		return false, 0, fmt.Errorf("unexpected Redis success rate limit reply length %d", len(values))
	}
	allowed, err := redisReplyInteger(values[0])
	if err != nil {
		return false, 0, err
	}
	retryAfter, err := redisReplyInteger(values[1])
	if err != nil {
		return false, 0, err
	}
	return allowed == 1, retryAfter, nil
}

func (r successLimitReservation) release(ctx context.Context) error {
	if r.member == "" {
		return nil
	}
	if r.rdb != nil {
		return r.rdb.ZRem(ctx, r.key, r.member).Err()
	}
	inMemoryRateLimiter.Release(r.key, r.member)
	return nil
}

func checkAccessSourceLimit(c *gin.Context) bool {
	userSetting, _ := common.GetContextKeyType[relaydto.UserSetting](c, constant.ContextKeyUserSetting)
	accessLimits := setting.ResolveAccessSourceLimits(userSetting)
	if !accessLimits.Enabled {
		return true
	}
	decision, err := service.CheckAccessSource(c.Request.Context(), c.GetInt("id"), c.ClientIP(), accessLimits)
	if err != nil {
		logger.LogError(c.Request.Context(), "access source check failed: "+err.Error())
		abortWithOpenAiMessage(c, http.StatusInternalServerError, "access_source_check_failed")
		return false
	}
	if !decision.Allowed {
		abortAccessSourceRejected(c, decision, accessLimits)
		return false
	}
	return true
}

// AccessSourceLimit validates and records the source of a consuming request.
func AccessSourceLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if checkAccessSourceLimit(c) {
			c.Next()
		}
	}
}

// ModelRequestRateLimit applies access-source, concurrency, and request-rate limits.
func ModelRequestRateLimit() gin.HandlerFunc {
	trafficLimit := ModelRequestTrafficLimit()
	return func(c *gin.Context) {
		if !checkAccessSourceLimit(c) {
			return
		}
		trafficLimit(c)
	}
}

// ModelRequestTrafficLimit applies concurrency and request-rate limits without source tracking.
func ModelRequestTrafficLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		runModelRequestTrafficLimit(c, c.Next)
	}
}

func runModelRequestTrafficLimit(c *gin.Context, next func()) {
	userSetting, _ := common.GetContextKeyType[relaydto.UserSetting](c, constant.ContextKeyUserSetting)
	concurrencyLimits := setting.ResolveModelRequestConcurrencyLimits(userSetting)
	if concurrencyLimits.Enabled && (concurrencyLimits.AccountLimit > 0 || concurrencyLimits.IPLimit > 0) {
		originalRequest := c.Request
		limitedCtx, release, err := service.AcquireModelRequestConcurrency(
			originalRequest.Context(),
			c.GetInt("id"),
			c.ClientIP(),
			concurrencyLimits.AccountLimit,
			concurrencyLimits.IPLimit,
		)
		if err != nil {
			abortModelRequestConcurrency(c, err, concurrencyLimits)
			return
		}
		c.Request = originalRequest.WithContext(limitedCtx)
		defer func() {
			cause := context.Cause(limitedCtx)
			c.Request = originalRequest
			release()
			if errors.Is(cause, service.ErrModelConcurrencyUnavailable) {
				logger.LogError(originalRequest.Context(), "model request concurrency lease failed: "+cause.Error())
				if !c.Writer.Written() {
					abortModelRequestConcurrency(c, cause, concurrencyLimits)
				}
			}
		}()
	}

	if !setting.ModelRequestRateLimitEnabled {
		next()
		return
	}

	duration := rateLimitDurationSeconds(setting.ModelRequestRateLimitDurationMinutes)
	group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	accountLimits := setting.ResolveModelRequestRateLimits(group, userSetting)
	userID := c.GetInt("id")
	clientIP := c.ClientIP()
	accountTotalKey := fmt.Sprintf("rateLimit:%d", userID)
	accountSuccessKey := redisUserRateLimitKey(ModelRequestRateLimitSuccessCountMark, userID)
	ipTotalKey := ModelRequestIPRateLimitCountMark + ":" + clientIP
	ipSuccessKey := redisIPRateLimitKey(ModelRequestIPRateLimitSuccessMark, clientIP)
	reservations := make([]successLimitReservation, 0, 2)
	keepReservations := false
	defer func() {
		if keepReservations {
			return
		}
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 2*time.Second)
		defer cancel()
		for _, reservation := range reservations {
			if err := reservation.release(releaseCtx); err != nil {
				logger.LogError(c.Request.Context(), "release success rate limit reservation failed: "+err.Error())
			}
		}
	}()

	if !takeAccountTotal(c, accountTotalKey, accountLimits.TotalCount, duration) {
		return
	}
	accountReservation, allowed := takeSuccessLimit(c, accountSuccessKey, accountLimits.SuccessCount, duration, setting.ModelRequestRateLimitAccountSuccessErrorTemplateOptionKey, types.ErrorCodeRateLimitAccountSuccess)
	if !allowed {
		return
	}
	reservations = append(reservations, accountReservation)
	if !takeIPTotal(c, ipTotalKey, setting.ModelRequestIPRateLimitCount, duration) {
		return
	}
	ipReservation, allowed := takeSuccessLimit(c, ipSuccessKey, setting.ModelRequestIPRateLimitSuccessCount, duration, setting.ModelRequestRateLimitIPSuccessErrorTemplateOptionKey, types.ErrorCodeRateLimitIPSuccess)
	if !allowed {
		return
	}
	reservations = append(reservations, ipReservation)

	next()
	keepReservations = c.Writer.Status() < http.StatusBadRequest &&
		(c.Writer.Written() || c.Request.Context().Err() == nil)
}

func abortModelRequestConcurrency(c *gin.Context, err error, limits setting.ModelRequestConcurrencyLimits) {
	status := http.StatusServiceUnavailable
	code := types.ErrorCodeModelRequestConcurrencyUnavailable
	message := i18n.T(c, i18n.MsgModelRequestConcurrencyUnavailable)
	switch {
	case errors.Is(err, service.ErrAccountConcurrencyLimit):
		status = http.StatusTooManyRequests
		code = types.ErrorCodeAccountConcurrencyLimit
		message = renderRequestLimitError(c, setting.ModelRequestConcurrencyAccountErrorTemplateOptionKey, setting.RequestLimitErrorTemplateValues{
			Limit: strconv.Itoa(limits.AccountLimit),
		})
	case errors.Is(err, service.ErrIPConcurrencyLimit):
		status = http.StatusTooManyRequests
		code = types.ErrorCodeIPConcurrencyLimit
		message = renderRequestLimitError(c, setting.ModelRequestConcurrencyIPErrorTemplateOptionKey, setting.RequestLimitErrorTemplateValues{
			Limit: strconv.Itoa(limits.IPLimit),
		})
	}
	abortWithOpenAiMessage(c, status, message, code)
}

func takeAccountTotal(c *gin.Context, key string, maxCount int, duration int64) bool {
	if maxCount == 0 {
		return true
	}
	if common.RedisEnabled {
		if common.RDB == nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return false
		}
		allowed, err := limiter.New(c.Request.Context(), common.RDB).Allow(
			c.Request.Context(), key,
			limiter.WithCapacity(rateLimitCapacity(maxCount, duration)),
			limiter.WithRate(int64(maxCount)),
			limiter.WithRequested(duration),
		)
		if err != nil {
			logger.LogError(c.Request.Context(), "account total rate limit check failed: "+err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return false
		}
		if allowed {
			return true
		}
	} else {
		inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)
		if inMemoryRateLimiter.Request(ModelRequestRateLimitCountMark+key, maxCount, duration) {
			return true
		}
	}
	abortRateLimit(c, setting.ModelRequestRateLimitAccountTotalErrorTemplateOptionKey, maxCount, duration, types.ErrorCodeRateLimitAccountTotal)
	return false
}

func takeSuccessLimit(c *gin.Context, key string, maxCount int, duration int64, templateKey string, code types.ErrorCode) (successLimitReservation, bool) {
	if maxCount == 0 {
		return successLimitReservation{}, true
	}
	member := common.GetUUID()
	reservation := successLimitReservation{key: key, member: member}
	retryAfter := duration
	allowed := false
	if common.RedisEnabled {
		if common.RDB == nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return successLimitReservation{}, false
		}
		reservation.rdb = common.RDB
		var err error
		allowed, retryAfter, err = reserveRedisSuccessLimit(c.Request.Context(), common.RDB, key, maxCount, duration, member)
		if err != nil {
			logger.LogError(c.Request.Context(), "success rate limit check failed: "+err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return successLimitReservation{}, false
		}
	} else {
		inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)
		allowed = inMemoryRateLimiter.Reserve(key, maxCount, duration, member)
	}
	if allowed {
		return reservation, true
	}
	abortRateLimit(c, templateKey, maxCount, retryAfter, code)
	return successLimitReservation{}, false
}

func takeIPTotal(c *gin.Context, key string, maxCount int, duration int64) bool {
	if maxCount == 0 {
		return true
	}
	retryAfter := duration
	allowed := false
	if common.RedisEnabled {
		var err error
		allowed, _, retryAfter, err = redisFixedWindowTake(c.Request.Context(), redisIPRateLimitKey(ModelRequestIPRateLimitCountMark, c.ClientIP()), maxCount, duration)
		if err != nil {
			logger.LogError(c.Request.Context(), "IP total rate limit check failed: "+err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return false
		}
	} else {
		inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)
		allowed = inMemoryRateLimiter.Request(key, maxCount, duration)
	}
	if allowed {
		return true
	}
	abortRateLimit(c, setting.ModelRequestRateLimitIPTotalErrorTemplateOptionKey, maxCount, retryAfter, types.ErrorCodeRateLimitIPTotal)
	return false
}

func abortRateLimit(c *gin.Context, templateKey string, maxCount int, retryAfter int64, code types.ErrorCode) {
	if retryAfter > 0 {
		c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
	}
	abortWithOpenAiMessage(c, http.StatusTooManyRequests, renderRequestLimitError(c, templateKey, setting.RequestLimitErrorTemplateValues{
		Limit:      strconv.Itoa(maxCount),
		Period:     setting.FormatRequestLimitDuration(rateLimitDurationSeconds(setting.ModelRequestRateLimitDurationMinutes)),
		RetryAfter: setting.FormatRequestLimitDuration(retryAfter),
	}), code)
}

func abortAccessSourceRejected(c *gin.Context, decision service.AccessSourceDecision, limits setting.AccessSourceLimits) {
	status := http.StatusForbidden
	templateKey := setting.AccessSourceAccountIPLimitErrorTemplateOptionKey
	code := types.ErrorCodeAccessSourceAccountIPLimit
	values := setting.RequestLimitErrorTemplateValues{
		Limit:  strconv.Itoa(limits.MaxIPsPerUser),
		Period: setting.FormatRequestLimitDuration(int64(limits.AssociationWindowHours) * 60 * 60),
	}
	switch decision.Reason {
	case service.AccessSourceSwitchCooldown:
		status = http.StatusTooManyRequests
		templateKey = setting.AccessSourceSwitchCooldownErrorTemplateOptionKey
		code = types.ErrorCodeAccessSourceSwitchCooldown
		values = setting.RequestLimitErrorTemplateValues{
			RetryAfter: setting.FormatRequestLimitDuration(decision.RetryAfterSeconds),
		}
		if decision.RetryAfterSeconds > 0 {
			c.Header("Retry-After", strconv.FormatInt(decision.RetryAfterSeconds, 10))
		}
	case service.AccessSourceIPAccountLimit:
		templateKey = setting.AccessSourceIPAccountLimitErrorTemplateOptionKey
		code = types.ErrorCodeAccessSourceIPAccountLimit
		values.Limit = strconv.Itoa(limits.MaxUsersPerIP)
	}
	abortWithOpenAiMessage(c, status, renderRequestLimitError(c, templateKey, values), code)
}

func renderRequestLimitError(c *gin.Context, templateKey string, values setting.RequestLimitErrorTemplateValues) string {
	message, err := setting.RenderRequestLimitErrorTemplate(templateKey, values)
	if err != nil {
		logger.LogError(c.Request.Context(), "render request limit error template failed: "+err.Error())
	}
	return message
}

func rateLimitDurationSeconds(durationMinutes int) int64 {
	if durationMinutes <= 0 {
		return 0
	}
	minutes := int64(durationMinutes)
	if minutes > math.MaxInt64/60 {
		return math.MaxInt64
	}
	return minutes * 60
}

func rateLimitCapacity(count int, durationSeconds int64) int64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	c := int64(count)
	if c > math.MaxInt64/durationSeconds {
		return math.MaxInt64
	}
	return c * durationSeconds
}
