package model_setting

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	CodexSettingsDefaultOptionKey      = "codex.default_settings"
	DefaultCodexUserAgentClient        = "codex-tui"
	DefaultCodexUserAgentOS            = "Mac OS"
	DefaultCodexUserAgentOSVersion     = "26.5.2"
	DefaultCodexUserAgentArchitecture  = "arm64"
	DefaultCodexUserAgentTerminal      = "ghostty/1.3.1"
	maxCodexErrorResponseMappings      = 100
	maxCodexErrorMessagePatterns       = 100
	maxCodexErrorMessagePatternLength  = 1024
	maxCodexErrorResponseMessageLength = 4096
)

var codexUserAgentClientPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$`)
var codexUserAgentComponentPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z ._/-]{0,63}$`)

// CodexErrorResponseMatch describes the upstream error conditions for one rule.
type CodexErrorResponseMatch struct {
	StatusCode      int      `json:"status_code"`
	MessagePatterns []string `json:"message_patterns"`
}

// CodexErrorResponseRewrite describes the client-facing error returned by one rule.
type CodexErrorResponseRewrite struct {
	StatusCode int    `json:"status_code"`
	Type       string `json:"type"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

// CodexErrorResponseMapping maps one upstream error condition to a client response.
type CodexErrorResponseMapping struct {
	Match   CodexErrorResponseMatch   `json:"match"`
	Rewrite CodexErrorResponseRewrite `json:"rewrite"`
}

// CodexSettings controls global Codex-compatible request behavior.
type CodexSettings struct {
	ClientVersionCheckEnabled         bool                        `json:"client_version_check_enabled"`
	MinimumClientVersion              string                      `json:"minimum_client_version"`
	DesktopClientVersionCheckEnabled  bool                        `json:"desktop_client_version_check_enabled"`
	MinimumDesktopClientVersion       string                      `json:"minimum_desktop_client_version"`
	RequestHeaderFallbackEnabled      bool                        `json:"request_header_fallback_enabled"`
	RequestHeaderFallbackClient       string                      `json:"request_header_fallback_client"`
	RequestHeaderFallbackVersion      string                      `json:"request_header_fallback_version"`
	RequestHeaderFallbackOS           string                      `json:"request_header_fallback_os"`
	RequestHeaderFallbackOSVersion    string                      `json:"request_header_fallback_os_version"`
	RequestHeaderFallbackArchitecture string                      `json:"request_header_fallback_architecture"`
	RequestHeaderFallbackTerminal     string                      `json:"request_header_fallback_terminal"`
	RequestHeaderModelPatterns        []string                    `json:"request_header_model_patterns"`
	ErrorResponseMappings             []CodexErrorResponseMapping `json:"error_response_mappings"`
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
	ErrorResponseMappings:             []CodexErrorResponseMapping{},
}

var codexSettings = cloneCodexSettings(defaultCodexSettings)

func cloneCodexSettings(settings CodexSettings) CodexSettings {
	settings.RequestHeaderModelPatterns = append([]string(nil), settings.RequestHeaderModelPatterns...)
	settings.ErrorResponseMappings = append([]CodexErrorResponseMapping{}, settings.ErrorResponseMappings...)
	for index := range settings.ErrorResponseMappings {
		settings.ErrorResponseMappings[index].Match.MessagePatterns = append(
			[]string(nil),
			settings.ErrorResponseMappings[index].Match.MessagePatterns...,
		)
	}
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

func validHTTPStatusCode(statusCode int) bool {
	return statusCode >= 400 && statusCode <= 599
}

// ValidateCodexErrorResponseMappings validates the JSON persisted by the
// option API before it becomes active for Codex clients.
func ValidateCodexErrorResponseMappings(value string) error {
	var mappings []CodexErrorResponseMapping
	if err := common.UnmarshalJsonStr(value, &mappings); err != nil {
		return fmt.Errorf("invalid Codex error response mappings: %w", err)
	}
	if len(mappings) > maxCodexErrorResponseMappings {
		return fmt.Errorf("Codex error response mappings cannot exceed %d rules", maxCodexErrorResponseMappings)
	}
	for ruleIndex, mapping := range mappings {
		if !validHTTPStatusCode(mapping.Match.StatusCode) {
			return fmt.Errorf("Codex error response mapping %d has invalid upstream status code %d", ruleIndex+1, mapping.Match.StatusCode)
		}
		if len(mapping.Match.MessagePatterns) == 0 || len(mapping.Match.MessagePatterns) > maxCodexErrorMessagePatterns {
			return fmt.Errorf("Codex error response mapping %d must contain 1 to %d message patterns", ruleIndex+1, maxCodexErrorMessagePatterns)
		}
		for patternIndex, pattern := range mapping.Match.MessagePatterns {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" || len(pattern) > maxCodexErrorMessagePatternLength {
				return fmt.Errorf("Codex error response mapping %d message pattern %d is empty or too long", ruleIndex+1, patternIndex+1)
			}
			if strings.HasPrefix(pattern, "re:") {
				expression := strings.TrimSpace(strings.TrimPrefix(pattern, "re:"))
				if expression == "" {
					return fmt.Errorf("Codex error response mapping %d message pattern %d has an empty regular expression", ruleIndex+1, patternIndex+1)
				}
				if _, err := regexp.Compile("(?i)" + expression); err != nil {
					return fmt.Errorf("Codex error response mapping %d message pattern %d has an invalid regular expression: %w", ruleIndex+1, patternIndex+1, err)
				}
			}
		}
		if !validHTTPStatusCode(mapping.Rewrite.StatusCode) {
			return fmt.Errorf("Codex error response mapping %d has invalid return status code %d", ruleIndex+1, mapping.Rewrite.StatusCode)
		}
		rewriteType := strings.TrimSpace(mapping.Rewrite.Type)
		rewriteCode := strings.TrimSpace(mapping.Rewrite.Code)
		rewriteMessage := strings.TrimSpace(mapping.Rewrite.Message)
		if rewriteType == "" || rewriteCode == "" || rewriteMessage == "" {
			return fmt.Errorf("Codex error response mapping %d requires type, code, and message", ruleIndex+1)
		}
		if len(rewriteType) > 128 || len(rewriteCode) > 128 || len(rewriteMessage) > maxCodexErrorResponseMessageLength {
			return fmt.Errorf("Codex error response mapping %d rewrite fields are too long", ruleIndex+1)
		}
	}
	return nil
}

// MatchCodexErrorResponse returns the first configured response rewrite whose
// upstream status and message match an in-scope Codex Responses request.
func MatchCodexErrorResponse(userAgent, path string, statusCode int, message string) (*CodexErrorResponseRewrite, bool) {
	if path != "/v1/responses" || !IsCodexUserAgent(userAgent) {
		return nil, false
	}

	messageLower := strings.ToLower(message)
	for _, mapping := range GetCodexSettings().ErrorResponseMappings {
		if mapping.Match.StatusCode != statusCode {
			continue
		}
		for _, rawPattern := range mapping.Match.MessagePatterns {
			pattern := strings.TrimSpace(rawPattern)
			matched := false
			if strings.HasPrefix(pattern, "re:") {
				expression := strings.TrimSpace(strings.TrimPrefix(pattern, "re:"))
				compiled, err := regexp.Compile("(?i)" + expression)
				matched = err == nil && compiled.MatchString(message)
			} else if pattern != "" {
				matched = strings.Contains(messageLower, strings.ToLower(pattern))
			}
			if matched {
				rewrite := mapping.Rewrite
				rewrite.Type = strings.TrimSpace(rewrite.Type)
				rewrite.Code = strings.TrimSpace(rewrite.Code)
				rewrite.Message = strings.TrimSpace(rewrite.Message)
				return &rewrite, true
			}
		}
	}
	return nil, false
}
