package model_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckClientVersion(t *testing.T) {
	codex := GetCodexSettings()
	claude := GetClaudeSettings()
	originalCodex := *codex
	originalClaude := *claude
	t.Cleanup(func() {
		*codex = originalCodex
		*claude = originalClaude
	})

	codex.ClientVersionCheckEnabled = true
	codex.MinimumClientVersion = "0.147.0"
	codex.DesktopClientVersionCheckEnabled = true
	codex.MinimumDesktopClientVersion = "26.810.52044"
	claude.ClientVersionCheckEnabled = true
	claude.MinimumClientVersion = "2.1.233"

	tests := []struct {
		name      string
		userAgent string
		client    string
		current   string
	}{
		{name: "outdated Codex TUI", userAgent: "codex-tui/0.146.0 (Mac OS 26.5.2; arm64) ghostty/1.3.1 (codex-tui; 0.146.0)", client: "Codex CLI", current: "0.146.0"},
		{name: "outdated legacy Codex CLI", userAgent: "codex_cli_rs/0.146.0 (Darwin 25.0.0; arm64) ghostty", client: "Codex CLI", current: "0.146.0"},
		{name: "legacy Codex CLI product", userAgent: "codex-cli/0.146.0", client: "Codex CLI", current: "0.146.0"},
		{name: "current Codex CLI", userAgent: "codex-tui/0.147.0"},
		{name: "current Codex Desktop", userAgent: "Codex Desktop/0.148.0-alpha.9 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.810.52044)"},
		{name: "outdated Codex Desktop", userAgent: "Codex Desktop/0.148.0-alpha.9 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.810.52043)", client: "Codex Desktop", current: "26.810.52043"},
		{name: "outdated Claude Code CLI", userAgent: "claude-cli/2.1.232 (external, cli)", client: "Claude Code", current: "2.1.232"},
		{name: "outdated Claude Desktop code mode", userAgent: "claude-cli/2.1.232 (external, claude-desktop)", client: "Claude Code", current: "2.1.232"},
		{name: "unrecognized client", userAgent: "Go-http-client/2.0"},
		{name: "malformed recognized version", userAgent: "codex_cli_rs/latest"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := CheckClientVersion(test.userAgent, true, true)
			if test.client == "" {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			assert.Equal(t, test.client, result.ClientName)
			assert.Equal(t, test.current, result.CurrentVersion)
			assert.Contains(t, result.Message(), test.current)
			assert.Contains(t, result.Message(), result.MinimumVersion)
		})
	}
}

func TestCheckClientVersionCodexPoliciesAreIndependent(t *testing.T) {
	codex := GetCodexSettings()
	original := *codex
	t.Cleanup(func() { *codex = original })

	codex.ClientVersionCheckEnabled = false
	codex.MinimumClientVersion = "0.147.0"
	codex.DesktopClientVersionCheckEnabled = true
	codex.MinimumDesktopClientVersion = "26.810.52044"

	assert.Nil(t, CheckClientVersion("codex-tui/0.1.0", false, true))
	assert.NotNil(t, CheckClientVersion("Codex Desktop/0.148.0-alpha.9 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.810.1)", false, true))

	codex.ClientVersionCheckEnabled = true
	codex.DesktopClientVersionCheckEnabled = false
	assert.NotNil(t, CheckClientVersion("codex-tui/0.1.0", false, true))
	assert.Nil(t, CheckClientVersion("Codex Desktop/0.148.0-alpha.9 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.810.1)", false, true))
}

func TestCheckClientVersionScopesClaudeCode(t *testing.T) {
	claude := GetClaudeSettings()
	original := *claude
	t.Cleanup(func() { *claude = original })

	claude.ClientVersionCheckEnabled = true
	claude.MinimumClientVersion = "2.1.233"
	userAgent := "claude-cli/2.1.232 (external, cli)"

	assert.NotNil(t, CheckClientVersion(userAgent, true, false))
	assert.Nil(t, CheckClientVersion(userAgent, false, false))
}

func TestCheckClientVersionScopesCodex(t *testing.T) {
	codex := GetCodexSettings()
	original := *codex
	t.Cleanup(func() { *codex = original })

	codex.ClientVersionCheckEnabled = true
	codex.MinimumClientVersion = "0.147.0"
	codex.DesktopClientVersionCheckEnabled = true
	codex.MinimumDesktopClientVersion = "26.810.52044"
	userAgents := []string{
		"codex-tui/0.146.0",
		"Codex Desktop/0.148.0-alpha.9 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.810.52043)",
	}

	for _, userAgent := range userAgents {
		assert.NotNil(t, CheckClientVersion(userAgent, false, true))
		assert.Nil(t, CheckClientVersion(userAgent, false, false))
	}
}

func TestValidateClientVersion(t *testing.T) {
	require.NoError(t, ValidateClientVersion("0.147.0"))
	require.NoError(t, ValidateClientVersion("2.1.233-beta.1"))
	assert.Error(t, ValidateClientVersion("latest"))
	assert.Error(t, ValidateClientVersion("v0.147.0"))
	assert.Error(t, ValidateClientVersion("0.147.0-beta..1"))
}

func TestValidateCodexUserAgentComponent(t *testing.T) {
	require.NoError(t, ValidateCodexUserAgentComponent("Mac OS"))
	require.NoError(t, ValidateCodexUserAgentComponent("26.5.2"))
	require.NoError(t, ValidateCodexUserAgentComponent("arm64"))
	require.NoError(t, ValidateCodexUserAgentComponent("Ghostty/1.2.3"))
	assert.Error(t, ValidateCodexUserAgentComponent(""))
	assert.Error(t, ValidateCodexUserAgentComponent("Mac OS; injected"))
	assert.Error(t, ValidateCodexUserAgentComponent("Ghostty\nX-Test: value"))
}

func TestValidateCodexUserAgentClient(t *testing.T) {
	require.NoError(t, ValidateCodexUserAgentClient("codex-tui"))
	require.NoError(t, ValidateCodexUserAgentClient("custom_cli.rs"))
	assert.Error(t, ValidateCodexUserAgentClient("codex tui"))
	assert.Error(t, ValidateCodexUserAgentClient("codex/tui"))
}
