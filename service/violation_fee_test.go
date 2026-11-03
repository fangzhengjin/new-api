package service

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeViolationFeeErrorPreservesUpstreamStatus(t *testing.T) {
	upstreamError := types.WithOpenAIError(types.OpenAIError{
		Message: CSAMViolationMarker,
		Type:    "invalid_request_error",
		Code:    "content_policy_violation",
	}, http.StatusServiceUnavailable)
	upstreamError.UpstreamStatusCode = http.StatusTooManyRequests

	normalized := NormalizeViolationFeeError(upstreamError)

	require.NotSame(t, upstreamError, normalized)
	assert.Equal(t, http.StatusTooManyRequests, normalized.UpstreamStatusCode)
}
