package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useRequestLimitErrorTemplate(t *testing.T, key string, value string) {
	t.Helper()

	common.OptionMapRWMutex.Lock()
	wasNil := common.OptionMap == nil
	if wasNil {
		common.OptionMap = map[string]string{}
	}
	previous, existed := common.OptionMap[key]
	common.OptionMap[key] = value
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		if wasNil {
			common.OptionMap = nil
		} else if existed {
			common.OptionMap[key] = previous
		} else {
			delete(common.OptionMap, key)
		}
		common.OptionMapRWMutex.Unlock()
	})
}

func useModelRequestRateLimitSettings(t *testing.T, ipMaxCount, ipSuccessMaxCount, totalMaxCount, successMaxCount int) {
	t.Helper()

	previousEnabled := setting.ModelRequestRateLimitEnabled
	previousDuration := setting.ModelRequestRateLimitDurationMinutes
	previousIPMaxCount := setting.ModelRequestIPRateLimitCount
	previousIPSuccessMaxCount := setting.ModelRequestIPRateLimitSuccessCount
	previousTotalMaxCount := setting.ModelRequestRateLimitCount
	previousSuccessMaxCount := setting.ModelRequestRateLimitSuccessCount
	setting.ModelRequestRateLimitMutex.Lock()
	previousGroupLimits := setting.ModelRequestRateLimitGroup
	setting.ModelRequestRateLimitGroup = map[string][2]int{}
	setting.ModelRequestRateLimitMutex.Unlock()

	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestIPRateLimitCount = ipMaxCount
	setting.ModelRequestIPRateLimitSuccessCount = ipSuccessMaxCount
	setting.ModelRequestRateLimitCount = totalMaxCount
	setting.ModelRequestRateLimitSuccessCount = successMaxCount

	t.Cleanup(func() {
		setting.ModelRequestRateLimitEnabled = previousEnabled
		setting.ModelRequestRateLimitDurationMinutes = previousDuration
		setting.ModelRequestIPRateLimitCount = previousIPMaxCount
		setting.ModelRequestIPRateLimitSuccessCount = previousIPSuccessMaxCount
		setting.ModelRequestRateLimitCount = previousTotalMaxCount
		setting.ModelRequestRateLimitSuccessCount = previousSuccessMaxCount
		setting.ModelRequestRateLimitMutex.Lock()
		setting.ModelRequestRateLimitGroup = previousGroupLimits
		setting.ModelRequestRateLimitMutex.Unlock()
	})
}

func useModelRequestConcurrencySettings(t *testing.T, accountLimit, ipLimit int) {
	t.Helper()
	previousEnabled := setting.ModelRequestConcurrencyLimitEnabled
	previousAccountLimit := setting.ModelRequestConcurrencyLimit
	previousIPLimit := setting.ModelRequestIPConcurrencyLimit
	setting.ModelRequestConcurrencyLimitEnabled = true
	setting.ModelRequestConcurrencyLimit = accountLimit
	setting.ModelRequestIPConcurrencyLimit = ipLimit
	t.Cleanup(func() {
		setting.ModelRequestConcurrencyLimitEnabled = previousEnabled
		setting.ModelRequestConcurrencyLimit = previousAccountLimit
		setting.ModelRequestIPConcurrencyLimit = previousIPLimit
	})
}

func newModelRequestRateLimitTestRouter(t *testing.T, firstUserID int) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET(
		"/user-one",
		func(c *gin.Context) {
			c.Set("id", firstUserID)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "vip")
		},
		ModelRequestRateLimit(),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
	router.GET(
		"/user-two",
		func(c *gin.Context) {
			c.Set("id", firstUserID+1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "vip")
		},
		ModelRequestRateLimit(),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
	router.GET(
		"/user-one-fail",
		func(c *gin.Context) {
			c.Set("id", firstUserID)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "vip")
		},
		ModelRequestRateLimit(),
		func(c *gin.Context) { c.Status(http.StatusBadRequest) },
	)
	return router
}

func TestRateLimitCustomErrorTemplatePreservesNewlineAndRetryAfter(t *testing.T) {
	const templateKey = setting.ModelRequestRateLimitAccountTotalErrorTemplateOptionKey
	useRequestLimitErrorTemplate(t, templateKey, "Limit {{.Limit}} per {{.Period}}, retry in {{.RetryAfter}}\n限制 {{.Limit}} 次，请在 {{.RetryAfter}} 后重试")
	previousDuration := setting.ModelRequestRateLimitDurationMinutes
	setting.ModelRequestRateLimitDurationMinutes = 1
	t.Cleanup(func() { setting.ModelRequestRateLimitDurationMinutes = previousDuration })

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	context.Set(common.RequestIdKey, "request-test")
	abortRateLimit(context, templateKey, 10, 135, types.ErrorCodeRateLimitAccountTotal)

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.Equal(t, "135", recorder.Header().Get("Retry-After"))
	var response struct {
		Error types.OpenAIError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "rate_limit_account_total", response.Error.Code)
	assert.Equal(t, "Limit 10 per 1m, retry in 2m 15s\n限制 10 次，请在 2m 15s 后重试 (request id: request-test)", response.Error.Message)
}

func TestAccessSourceDefaultErrorIncludesAssociationWindow(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	context.Set(common.RequestIdKey, "request-source")
	abortAccessSourceRejected(context, service.AccessSourceDecision{
		Allowed: false,
		Reason:  service.AccessSourceAccountIPLimit,
	}, setting.AccessSourceLimits{
		AssociationWindowHours: 24,
		MaxIPsPerUser:          1,
	})

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "24h")
	assert.Contains(t, recorder.Body.String(), "账号关联 IP 数已达到上限")
}

func TestConcurrencyInternalFailureDoesNotUseCustomBusinessTemplate(t *testing.T) {
	const templateKey = setting.ModelRequestConcurrencyAccountErrorTemplateOptionKey
	useRequestLimitErrorTemplate(t, templateKey, "CUSTOM {{.Limit}}")
	require.NoError(t, i18n.Init())

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	abortModelRequestConcurrency(context, service.ErrModelConcurrencyUnavailable, setting.ModelRequestConcurrencyLimits{AccountLimit: 5})

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "CUSTOM")
	assert.Contains(t, recorder.Body.String(), "model_request_concurrency_unavailable")
}

func TestRedisSuccessLimitReservationIsAtomicAndReleasable(t *testing.T) {
	_, redisClient := useRateLimitMiniRedis(t)
	ctx := context.Background()
	key := redisUserRateLimitKey(ModelRequestRateLimitSuccessCountMark, 42)
	allowed, retryAfter, err := reserveRedisSuccessLimit(ctx, redisClient, key, 1, 60, "request-one")
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, int64(60), retryAfter)

	allowed, retryAfter, err = reserveRedisSuccessLimit(ctx, redisClient, key, 1, 60, "request-two")
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, int64(60), retryAfter)
	count, err := redisClient.ZCard(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	require.NoError(t, (successLimitReservation{key: key, member: "request-one", rdb: redisClient}).release(ctx))
	allowed, _, err = reserveRedisSuccessLimit(ctx, redisClient, key, 1, 60, "request-three")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestModelRequestConcurrencyRunsBeforeRateLimitsAndReleasesSlots(t *testing.T) {
	useRateLimitMiniRedis(t)
	useModelRequestConcurrencySettings(t, 1, 1)
	useModelRequestRateLimitSettings(t, 0, 0, 1, 0)

	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	entered := make(chan struct{})
	release := make(chan struct{})
	setUser := func(id int) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set("id", id)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "vip")
		}
	}
	router.GET("/hold", setUser(19001), ModelRequestRateLimit(), func(c *gin.Context) {
		close(entered)
		<-release
		c.Status(http.StatusNoContent)
	})
	router.GET("/user-one", setUser(19001), ModelRequestRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/user-two", setUser(19002), ModelRequestRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	firstResponse := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstResponse <- performRateLimitRequest(router, "/hold", "192.0.2.111:1000")
	}()
	<-entered

	accountLimited := performRateLimitRequest(router, "/user-one", "192.0.2.112:1000")
	assert.Equal(t, http.StatusTooManyRequests, accountLimited.Code)
	assert.Contains(t, accountLimited.Body.String(), "account_concurrency_limit")
	assert.Empty(t, accountLimited.Header().Get("Retry-After"))
	ipLimited := performRateLimitRequest(router, "/user-two", "192.0.2.111:1001")
	assert.Equal(t, http.StatusTooManyRequests, ipLimited.Code)
	assert.Contains(t, ipLimited.Body.String(), "ip_concurrency_limit")
	assert.Empty(t, ipLimited.Header().Get("Retry-After"))

	close(release)
	assert.Equal(t, http.StatusNoContent, (<-firstResponse).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-two", "192.0.2.113:1000").Code)
	rateLimited := performRateLimitRequest(router, "/user-two", "192.0.2.113:1001")
	assert.Equal(t, http.StatusTooManyRequests, rateLimited.Code)
	assert.Contains(t, rateLimited.Body.String(), "rate_limit_account_total")
}

func TestModelRequestConcurrencyRequiresRedisOnlyWhenEnabled(t *testing.T) {
	previousRDB, previousRedisEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = nil, false
	t.Cleanup(func() { common.RDB, common.RedisEnabled = previousRDB, previousRedisEnabled })
	previousRateEnabled := setting.ModelRequestRateLimitEnabled
	setting.ModelRequestRateLimitEnabled = false
	t.Cleanup(func() { setting.ModelRequestRateLimitEnabled = previousRateEnabled })
	useModelRequestConcurrencySettings(t, 1, 0)
	router := newModelRequestRateLimitTestRouter(t, 19101)

	unavailable := performRateLimitRequest(router, "/user-one", "192.0.2.121:1000")
	assert.Equal(t, http.StatusServiceUnavailable, unavailable.Code)
	assert.Contains(t, unavailable.Body.String(), "model_request_concurrency_unavailable")

	setting.ModelRequestConcurrencyLimitEnabled = false
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-one", "192.0.2.121:1001").Code)
}

func TestModelRequestConcurrencyUsesPerUserOverride(t *testing.T) {
	useRateLimitMiniRedis(t)
	useModelRequestConcurrencySettings(t, 1, 0)
	previousRateEnabled := setting.ModelRequestRateLimitEnabled
	setting.ModelRequestRateLimitEnabled = false
	t.Cleanup(func() { setting.ModelRequestRateLimitEnabled = previousRateEnabled })

	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	two := 2
	router.GET(
		"/hold",
		func(c *gin.Context) {
			c.Set("id", 19151)
			common.SetContextKey(c, constant.ContextKeyUserSetting, relaydto.UserSetting{ModelRequestConcurrencyLimit: &two})
		},
		ModelRequestRateLimit(),
		func(c *gin.Context) {
			entered <- struct{}{}
			<-release
			c.Status(http.StatusNoContent)
		},
	)

	responses := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() {
			responses <- performRateLimitRequest(router, "/hold", "192.0.2.123:1000")
		}()
	}
	<-entered
	<-entered
	third := performRateLimitRequest(router, "/hold", "192.0.2.123:1001")
	assert.Equal(t, http.StatusTooManyRequests, third.Code)
	assert.Contains(t, third.Body.String(), "account_concurrency_limit")
	close(release)
	assert.Equal(t, http.StatusNoContent, (<-responses).Code)
	assert.Equal(t, http.StatusNoContent, (<-responses).Code)
}

func TestModelRequestConcurrencyReleasesSlotAfterPanic(t *testing.T) {
	useRateLimitMiniRedis(t)
	useModelRequestConcurrencySettings(t, 1, 0)
	previousRateEnabled := setting.ModelRequestRateLimitEnabled
	setting.ModelRequestRateLimitEnabled = false
	t.Cleanup(func() { setting.ModelRequestRateLimitEnabled = previousRateEnabled })

	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.Use(gin.Recovery())
	setUser := func(c *gin.Context) { c.Set("id", 19201) }
	router.GET("/panic", setUser, ModelRequestRateLimit(), func(*gin.Context) { panic("test panic") })
	router.GET("/success", setUser, ModelRequestRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	assert.Equal(t, http.StatusInternalServerError, performRateLimitRequest(router, "/panic", "192.0.2.122:1000").Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/success", "192.0.2.122:1001").Code)
}

func TestModelRequestMemoryRateLimitUsesConfiguredPriority(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })
	useModelRequestRateLimitSettings(t, 1, 0, 1, 1000)
	setting.ModelRequestRateLimitMutex.Lock()
	setting.ModelRequestRateLimitGroup = map[string][2]int{"vip": {1, 1}}
	setting.ModelRequestRateLimitMutex.Unlock()
	router := newModelRequestRateLimitTestRouter(t, 10001)

	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-one", "192.0.2.71:1000").Code)
	accountLimited := performRateLimitRequest(router, "/user-one", "192.0.2.71:1001")
	assert.Equal(t, http.StatusTooManyRequests, accountLimited.Code)
	assert.Contains(t, accountLimited.Body.String(), "rate_limit_account_total")

	ipLimited := performRateLimitRequest(router, "/user-two", "192.0.2.71:1002")
	assert.Equal(t, http.StatusTooManyRequests, ipLimited.Code)
	assert.Equal(t, "60", ipLimited.Header().Get("Retry-After"))
	assert.Contains(t, ipLimited.Body.String(), "rate_limit_ip_total")
}

func TestModelRequestMemorySuccessLimitsDoNotCountFailedResponses(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })
	useModelRequestRateLimitSettings(t, 0, 1, 0, 1)
	router := newModelRequestRateLimitTestRouter(t, 11001)
	remoteAddr := "192.0.2.73:1000"

	assert.Equal(t, http.StatusBadRequest, performRateLimitRequest(router, "/user-one-fail", remoteAddr).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-one", remoteAddr).Code)
	accountLimited := performRateLimitRequest(router, "/user-one", "198.51.100.73:1000")
	assert.Equal(t, http.StatusTooManyRequests, accountLimited.Code)
	assert.Contains(t, accountLimited.Body.String(), "rate_limit_account_success")
	ipLimited := performRateLimitRequest(router, "/user-two", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, ipLimited.Code)
	assert.Contains(t, ipLimited.Body.String(), "rate_limit_ip_success")
}

func TestModelRequestRedisIPRateLimitCanBeEnabledAtRuntime(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	useModelRequestRateLimitSettings(t, 0, 0, 0, 1000)
	router := newModelRequestRateLimitTestRouter(t, 12001)
	remoteAddr := "192.0.2.72:1000"
	key := redisIPRateLimitKey(ModelRequestIPRateLimitCountMark, "192.0.2.72")

	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-one", remoteAddr).Code)
	assert.False(t, redisServer.Exists(key), "a disabled IP limit must not create a counter")

	setting.ModelRequestIPRateLimitCount = 2
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-one", remoteAddr).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-two", remoteAddr).Code)
	limited := performRateLimitRequest(router, "/user-one", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.Equal(t, "60", limited.Header().Get("Retry-After"))
	assert.Contains(t, limited.Body.String(), "rate_limit_ip_total")

	count, err := redisServer.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "3", count)
	assert.Equal(t, time.Minute, redisServer.TTL(key))

	ipv6Key := redisIPRateLimitKey(ModelRequestIPRateLimitCountMark, "2001:db8::72")
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-one", "[2001:db8::72]:1000").Code)
	assert.True(t, redisServer.Exists(ipv6Key))
}

func TestAccessSourceRunsWhenRequestRateLimitIsDisabled(t *testing.T) {
	useRateLimitMiniRedis(t)
	previousRateEnabled := setting.ModelRequestRateLimitEnabled
	previousSourceEnabled := setting.AccessSourceLimitEnabled
	previousWindow := setting.AccessSourceAssociationWindowHours
	previousMaxIPs := setting.AccessSourceMaxIPsPerUser
	previousCooldown := setting.AccessSourceSwitchCooldownMinutes
	previousMaxUsers := setting.AccessSourceMaxUsersPerIP
	setting.ModelRequestRateLimitEnabled = false
	setting.AccessSourceLimitEnabled = true
	setting.AccessSourceAssociationWindowHours = 24
	setting.AccessSourceMaxIPsPerUser = 1
	setting.AccessSourceSwitchCooldownMinutes = 0
	setting.AccessSourceMaxUsersPerIP = 0
	t.Cleanup(func() {
		setting.ModelRequestRateLimitEnabled = previousRateEnabled
		setting.AccessSourceLimitEnabled = previousSourceEnabled
		setting.AccessSourceAssociationWindowHours = previousWindow
		setting.AccessSourceMaxIPsPerUser = previousMaxIPs
		setting.AccessSourceSwitchCooldownMinutes = previousCooldown
		setting.AccessSourceMaxUsersPerIP = previousMaxUsers
	})
	router := newModelRequestRateLimitTestRouter(t, 13001)

	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-one", "192.0.2.80:1000").Code)
	rejected := performRateLimitRequest(router, "/user-one", "192.0.2.81:1000")
	assert.Equal(t, http.StatusForbidden, rejected.Code)
	assert.Contains(t, rejected.Body.String(), "access_source_account_ip_limit")
}

func TestModelRequestIPSuccessLimitCountsSuccessfulResponses(t *testing.T) {
	useRateLimitMiniRedis(t)
	useModelRequestRateLimitSettings(t, 0, 1, 0, 0)
	router := newModelRequestRateLimitTestRouter(t, 14001)

	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-one", "198.51.100.80:1000").Code)
	rejected := performRateLimitRequest(router, "/user-two", "198.51.100.80:1001")
	assert.Equal(t, http.StatusTooManyRequests, rejected.Code)
	assert.Contains(t, rejected.Body.String(), "rate_limit_ip_success")
}

func TestModelRequestRedisSuccessLimitsDoNotCountFailedResponses(t *testing.T) {
	useRateLimitMiniRedis(t)
	useModelRequestRateLimitSettings(t, 0, 1, 0, 1)
	router := newModelRequestRateLimitTestRouter(t, 16001)
	remoteAddr := "192.0.2.75:1000"

	assert.Equal(t, http.StatusBadRequest, performRateLimitRequest(router, "/user-one-fail", remoteAddr).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-one", remoteAddr).Code)
	accountLimited := performRateLimitRequest(router, "/user-one", "198.51.100.75:1000")
	assert.Equal(t, http.StatusTooManyRequests, accountLimited.Code)
	assert.Contains(t, accountLimited.Body.String(), "rate_limit_account_success")
	ipLimited := performRateLimitRequest(router, "/user-two", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, ipLimited.Code)
	assert.Contains(t, ipLimited.Body.String(), "rate_limit_ip_success")
}

func TestModelRequestSuccessLimitsBlockConcurrentRequests(t *testing.T) {
	tests := []struct {
		name             string
		redis            bool
		accountSuccess   int
		ipSuccess        int
		secondPath       string
		secondRemoteAddr string
		userID           int
	}{
		{name: "memory account", accountSuccess: 1, secondPath: "/user-one", secondRemoteAddr: "198.51.100.91:1000", userID: 17001},
		{name: "memory IP", ipSuccess: 1, secondPath: "/user-two", secondRemoteAddr: "192.0.2.92:1001", userID: 17011},
		{name: "Redis account", redis: true, accountSuccess: 1, secondPath: "/user-one", secondRemoteAddr: "198.51.100.93:1000", userID: 17021},
		{name: "Redis IP", redis: true, ipSuccess: 1, secondPath: "/user-two", secondRemoteAddr: "192.0.2.94:1001", userID: 17031},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.redis {
				useRateLimitMiniRedis(t)
			} else {
				previousRedisEnabled := common.RedisEnabled
				common.RedisEnabled = false
				t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })
			}
			useModelRequestRateLimitSettings(t, 0, test.ipSuccess, 0, test.accountSuccess)

			gin.SetMode(gin.TestMode)
			require.NoError(t, i18n.Init())
			router := gin.New()
			require.NoError(t, router.SetTrustedProxies(nil))
			entered := make(chan struct{})
			release := make(chan struct{})
			var downstreamCalls atomic.Int32
			downstream := func(c *gin.Context) {
				if downstreamCalls.Add(1) == 1 {
					close(entered)
					<-release
				}
				c.Status(http.StatusNoContent)
			}
			setUser := func(id int) gin.HandlerFunc {
				return func(c *gin.Context) {
					c.Set("id", id)
					common.SetContextKey(c, constant.ContextKeyUserGroup, "vip")
				}
			}
			router.GET("/user-one", setUser(test.userID), ModelRequestRateLimit(), downstream)
			router.GET("/user-two", setUser(test.userID+1), ModelRequestRateLimit(), downstream)

			firstResponse := make(chan *httptest.ResponseRecorder, 1)
			firstRemoteAddr := test.secondRemoteAddr
			if test.accountSuccess > 0 {
				firstRemoteAddr = "192.0.2.91:1000"
			}
			go func() {
				firstResponse <- performRateLimitRequest(router, "/user-one", firstRemoteAddr)
			}()
			<-entered

			limited := performRateLimitRequest(router, test.secondPath, test.secondRemoteAddr)
			assert.Equal(t, http.StatusTooManyRequests, limited.Code)
			if test.accountSuccess > 0 {
				assert.Contains(t, limited.Body.String(), "rate_limit_account_success")
			} else {
				assert.Contains(t, limited.Body.String(), "rate_limit_ip_success")
			}
			assert.Equal(t, int32(1), downstreamCalls.Load())

			close(release)
			assert.Equal(t, http.StatusNoContent, (<-firstResponse).Code)
		})
	}
}

func TestModelRequestSuccessLimitsReleaseCanceledRequests(t *testing.T) {
	tests := []struct {
		name          string
		redis         bool
		userID        int
		writeResponse bool
		wantSecond    int
	}{
		{name: "memory unwritten", userID: 18001, wantSecond: http.StatusNoContent},
		{name: "memory written", userID: 18002, writeResponse: true, wantSecond: http.StatusTooManyRequests},
		{name: "Redis unwritten", redis: true, userID: 18003, wantSecond: http.StatusNoContent},
		{name: "Redis written", redis: true, userID: 18004, writeResponse: true, wantSecond: http.StatusTooManyRequests},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.redis {
				useRateLimitMiniRedis(t)
			} else {
				previousRedisEnabled := common.RedisEnabled
				common.RedisEnabled = false
				t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })
			}
			useModelRequestRateLimitSettings(t, 0, 0, 0, 1)

			gin.SetMode(gin.TestMode)
			require.NoError(t, i18n.Init())
			router := gin.New()
			setUser := func(c *gin.Context) {
				c.Set("id", test.userID)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "vip")
			}
			var cancel context.CancelFunc
			router.GET("/cancel", setUser, ModelRequestRateLimit(), func(c *gin.Context) {
				if test.writeResponse {
					c.String(http.StatusOK, "ok")
				}
				cancel()
			})
			router.GET("/success", setUser, ModelRequestRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

			requestCtx, cancelRequest := context.WithCancel(context.Background())
			cancel = cancelRequest
			request := httptest.NewRequest(http.MethodGet, "/cancel", nil).WithContext(requestCtx)
			request.RemoteAddr = "192.0.2.101:1000"
			router.ServeHTTP(httptest.NewRecorder(), request)
			assert.ErrorIs(t, requestCtx.Err(), context.Canceled)
			assert.Equal(t, test.wantSecond, performRateLimitRequest(router, "/success", "192.0.2.101:1001").Code)
		})
	}
}

func TestModelRequestRateLimitPrioritizesSuccessBeforeIPAndIPTotalBeforeIPSuccess(t *testing.T) {
	useRateLimitMiniRedis(t)
	useModelRequestRateLimitSettings(t, 1, 1, 0, 1)
	router := newModelRequestRateLimitTestRouter(t, 15001)
	remoteAddr := "203.0.113.90:1000"

	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-one", remoteAddr).Code)
	accountSuccessLimited := performRateLimitRequest(router, "/user-one", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, accountSuccessLimited.Code)
	assert.Contains(t, accountSuccessLimited.Body.String(), "rate_limit_account_success")

	ipTotalLimited := performRateLimitRequest(router, "/user-two", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, ipTotalLimited.Code)
	assert.Contains(t, ipTotalLimited.Body.String(), "rate_limit_ip_total")
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/user-two", "203.0.113.91:1000").Code)
}
