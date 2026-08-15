package model_setting

import "github.com/QuantumNous/new-api/setting/config"

// CodexSettings controls global Codex-compatible request behavior.
type CodexSettings struct {
	RequestHeaderFallbackEnabled bool `json:"request_header_fallback_enabled"`
}

var codexSettings = CodexSettings{}

func init() {
	config.GlobalConfig.Register("codex", &codexSettings)
}

// GetCodexSettings returns the global Codex settings.
func GetCodexSettings() *CodexSettings {
	return &codexSettings
}
