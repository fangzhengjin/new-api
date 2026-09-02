package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInMemoryRateLimiterReservationCanBeReleased(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	assert.True(t, limiter.Reserve("success", 1, 60, "request-one"))
	assert.False(t, limiter.Reserve("success", 1, 60, "request-blocked"))
	limiter.Release("success", "request-one")
	assert.True(t, limiter.Reserve("success", 1, 60, "request-two"))

	assert.True(t, limiter.Request("unlimited", 0, 60))
	assert.True(t, limiter.Request("unlimited", 1, 60))
}
