package model_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientIdentityModelPatterns(t *testing.T) {
	patterns := []string{`^gpt-.*$`, `^codex-.*$`}
	assert.True(t, MatchesClientIdentityModel(patterns, "gpt-5.4"))
	assert.True(t, MatchesClientIdentityModel(patterns, "codex-auto-review"))
	assert.False(t, MatchesClientIdentityModel(patterns, "claude-sonnet-4-5"))
	assert.False(t, MatchesClientIdentityModel(patterns, ""))

	require.NoError(t, ValidateClientIdentityModelPatterns(`["^gpt-.*$","^codex-.*$"]`))
	assert.Error(t, ValidateClientIdentityModelPatterns(`[]`))
	assert.Error(t, ValidateClientIdentityModelPatterns(`["("]`))
	assert.Error(t, ValidateClientIdentityModelPatterns(`{"model":"gpt"}`))
}

func TestBuiltInClientSettingsSnapshotsAreIsolated(t *testing.T) {
	codexDefaults := GetDefaultCodexSettings()
	codexDefaults.RequestHeaderModelPatterns[0] = "changed"
	assert.Equal(t, `^gpt-.*$`, GetDefaultCodexSettings().RequestHeaderModelPatterns[0])

	claudeDefaults := GetDefaultClaudeSettings()
	claudeDefaults.DefaultMaxTokens["default"] = 1
	claudeDefaults.RequestHeaderModelPatterns[0] = "changed"
	freshClaudeDefaults := GetDefaultClaudeSettings()
	assert.Equal(t, 8192, freshClaudeDefaults.DefaultMaxTokens["default"])
	assert.Equal(t, `^claude-.*$`, freshClaudeDefaults.RequestHeaderModelPatterns[0])
}
