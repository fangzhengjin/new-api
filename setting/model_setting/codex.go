package model_setting

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	CodexSettingsDefaultOptionKey     = "codex.default_settings"
	DefaultCodexUserAgentClient       = "codex-tui"
	DefaultCodexUserAgentOS           = "Mac OS"
	DefaultCodexUserAgentOSVersion    = "26.5.2"
	DefaultCodexUserAgentArchitecture = "arm64"
	DefaultCodexUserAgentTerminal     = "ghostty/1.3.1"
)

var codexUserAgentClientPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$`)
var codexUserAgentComponentPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z ._/-]{0,63}$`)

// CodexSettings controls global Codex-compatible request behavior.
type CodexSettings struct {
	ClientVersionCheckEnabled         bool     `json:"client_version_check_enabled"`
	MinimumClientVersion              string   `json:"minimum_client_version"`
	DesktopClientVersionCheckEnabled  bool     `json:"desktop_client_version_check_enabled"`
	MinimumDesktopClientVersion       string   `json:"minimum_desktop_client_version"`
	RequestHeaderFallbackEnabled      bool     `json:"request_header_fallback_enabled"`
	RequestHeaderFallbackClient       string   `json:"request_header_fallback_client"`
	RequestHeaderFallbackVersion      string   `json:"request_header_fallback_version"`
	RequestHeaderFallbackOS           string   `json:"request_header_fallback_os"`
	RequestHeaderFallbackOSVersion    string   `json:"request_header_fallback_os_version"`
	RequestHeaderFallbackArchitecture string   `json:"request_header_fallback_architecture"`
	RequestHeaderFallbackTerminal     string   `json:"request_header_fallback_terminal"`
	RequestHeaderModelPatterns        []string `json:"request_header_model_patterns"`
}

var defaultCodexSettings = CodexSettings{
	MinimumClientVersion:              DefaultCodexClientVersion,
	MinimumDesktopClientVersion:       DefaultCodexDesktopClientVersion,
	RequestHeaderFallbackClient:       DefaultCodexUserAgentClient,
	RequestHeaderFallbackVersion:      DefaultCodexClientVersion,
	RequestHeaderFallbackOS:           DefaultCodexUserAgentOS,
	RequestHeaderFallbackOSVersion:    DefaultCodexUserAgentOSVersion,
	RequestHeaderFallbackArchitecture: DefaultCodexUserAgentArchitecture,
	RequestHeaderFallbackTerminal:     DefaultCodexUserAgentTerminal,
	RequestHeaderModelPatterns:        []string{`^gpt-.*$`, `^codex-.*$`},
}

var codexSettings = cloneCodexSettings(defaultCodexSettings)

func cloneCodexSettings(settings CodexSettings) CodexSettings {
	settings.RequestHeaderModelPatterns = append([]string(nil), settings.RequestHeaderModelPatterns...)
	return settings
}

func init() {
	config.GlobalConfig.Register("codex", &codexSettings)
}

// GetCodexSettings returns the global Codex settings.
func GetCodexSettings() *CodexSettings {
	return &codexSettings
}

// GetDefaultCodexSettings returns an isolated copy of the built-in settings.
func GetDefaultCodexSettings() CodexSettings {
	return cloneCodexSettings(defaultCodexSettings)
}

// ValidateCodexUserAgentClient validates the product token placed before the
// version in the generated User-Agent.
func ValidateCodexUserAgentClient(value string) error {
	value = strings.TrimSpace(value)
	if !codexUserAgentClientPattern.MatchString(value) {
		return fmt.Errorf("invalid Codex User-Agent client %q", value)
	}
	return nil
}

// ValidateCodexUserAgentComponent validates one administrator-configured
// synthetic client field before it is embedded in an outbound User-Agent.
func ValidateCodexUserAgentComponent(value string) error {
	value = strings.TrimSpace(value)
	if !codexUserAgentComponentPattern.MatchString(value) {
		return fmt.Errorf("invalid Codex User-Agent component %q", value)
	}
	return nil
}
