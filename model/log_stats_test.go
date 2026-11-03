package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestSumUsedQuotaNormalizesMixedAnthropicLogs(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	logs := []Log{
		{
			CreatedAt:        now,
			Type:             LogTypeConsume,
			PromptTokens:     70,
			CompletionTokens: 7,
			Other: common.MapToJsonStr(map[string]interface{}{
				"usage_semantic":           "anthropic",
				"cache_tokens":             30,
				"cache_creation_tokens":    20,
				"cache_creation_tokens_5m": 12,
				"cache_creation_tokens_1h": 8,
			}),
		},
		{
			CreatedAt:        now,
			Type:             LogTypeConsume,
			PromptTokens:     120,
			CompletionTokens: 7,
			Other: common.MapToJsonStr(map[string]interface{}{
				"usage_semantic":           "anthropic",
				"input_tokens_total":       120,
				"cache_tokens":             30,
				"cache_creation_tokens_5m": 12,
				"cache_creation_tokens_1h": 8,
			}),
		},
		{
			CreatedAt:        now,
			Type:             LogTypeConsume,
			PromptTokens:     70,
			CompletionTokens: 7,
			Other: common.MapToJsonStr(map[string]interface{}{
				"usage_semantic":           "anthropic",
				"input_tokens_total":       120,
				"cache_tokens":             30,
				"cache_creation_tokens_5m": 12,
				"cache_creation_tokens_1h": 8,
			}),
		},
		{
			CreatedAt:        now,
			Type:             LogTypeConsume,
			PromptTokens:     120,
			CompletionTokens: 7,
			Other: common.MapToJsonStr(map[string]interface{}{
				"usage_semantic":        "openai",
				"cache_tokens":          30,
				"cache_creation_tokens": 20,
			}),
		},
	}
	require.NoError(t, DB.Create(&logs).Error)

	stat, err := SumUsedQuota(LogTypeConsume, now-60, now, "", "", "", 0, "")

	require.NoError(t, err)
	require.Equal(t, 4, stat.Rpm)
	require.Equal(t, 508, stat.Tpm)
}
