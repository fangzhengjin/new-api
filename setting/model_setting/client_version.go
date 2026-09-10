package model_setting

import (
	"fmt"
	"regexp"
	"strings"

	version "github.com/hashicorp/go-version"
)

const (
	DefaultCodexClientVersion        = "0.147.0"
	DefaultCodexDesktopClientVersion = "26.810.52044"
	DefaultClaudeCodeVersion         = "2.1.233"
)

var clientVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func isCodexCLIProduct(productName string) bool {
	switch strings.ToLower(productName) {
	case "codex-tui", "codex_cli_rs", "codex-cli":
		return true
	default:
		return false
	}
}

// IsCodexUserAgent reports whether the User-Agent starts with a supported
// Codex CLI or Codex Desktop product token.
func IsCodexUserAgent(userAgent string) bool {
	userAgent = strings.TrimSpace(userAgent)
	const desktopPrefix = "codex desktop/"
	if strings.HasPrefix(strings.ToLower(userAgent), desktopPrefix) {
		versionText := userAgent[len(desktopPrefix):]
		versionText, _, _ = strings.Cut(versionText, " ")
		return versionText != ""
	}

	product, _, _ := strings.Cut(userAgent, " ")
	productName, versionText, ok := strings.Cut(product, "/")
	return ok && versionText != "" && isCodexCLIProduct(productName)
}

// OutdatedClientVersion describes a recognized client whose version is below
// the configured minimum.
type OutdatedClientVersion struct {
	ClientName     string
	CurrentVersion string
	MinimumVersion string
}

// Message returns the client-facing upgrade instruction.
func (v *OutdatedClientVersion) Message() string {
	return fmt.Sprintf("当前 %s 客户端版本 %s 过低，请升级至 %s 或更高版本后重试", v.ClientName, v.CurrentVersion, v.MinimumVersion)
}

// CheckClientVersion recognizes supported client User-Agent product tokens and
// reports only versions that violate an enabled minimum-version policy.
func CheckClientVersion(userAgent string, checkClaudeCode, checkCodex bool) *OutdatedClientVersion {
	userAgent = strings.TrimSpace(userAgent)
	clientName := ""
	minimumVersion := ""
	rawVersion := ""
	if strings.HasPrefix(strings.ToLower(userAgent), "codex desktop/") {
		if !checkCodex {
			return nil
		}
		settings := GetCodexSettings()
		if !settings.DesktopClientVersionCheckEnabled {
			return nil
		}
		const desktopVersionMarker = "(codex desktop;"
		markerIndex := strings.LastIndex(strings.ToLower(userAgent), desktopVersionMarker)
		if markerIndex < 0 {
			return nil
		}
		versionText := strings.TrimSpace(userAgent[markerIndex+len(desktopVersionMarker):])
		var ok bool
		rawVersion, _, ok = strings.Cut(versionText, ")")
		if !ok {
			return nil
		}
		rawVersion = strings.TrimSpace(rawVersion)
		clientName = "Codex Desktop"
		minimumVersion = settings.MinimumDesktopClientVersion
	} else {
		product, _, _ := strings.Cut(userAgent, " ")
		productName, versionText, ok := strings.Cut(product, "/")
		if !ok || versionText == "" {
			return nil
		}
		rawVersion = versionText
		if isCodexCLIProduct(productName) {
			if !checkCodex {
				return nil
			}
			settings := GetCodexSettings()
			if !settings.ClientVersionCheckEnabled {
				return nil
			}
			clientName = "Codex CLI"
			minimumVersion = settings.MinimumClientVersion
		} else if strings.EqualFold(productName, "claude-cli") {
			if !checkClaudeCode {
				return nil
			}
			settings := GetClaudeSettings()
			if !settings.ClientVersionCheckEnabled {
				return nil
			}
			clientName = "Claude Code"
			minimumVersion = settings.MinimumClientVersion
		} else {
			return nil
		}
	}

	current, currentErr := version.NewVersion(rawVersion)
	minimum, minimumErr := version.NewVersion(strings.TrimSpace(minimumVersion))
	if currentErr != nil || minimumErr != nil || !current.LessThan(minimum) {
		return nil
	}
	return &OutdatedClientVersion{
		ClientName:     clientName,
		CurrentVersion: rawVersion,
		MinimumVersion: minimumVersion,
	}
}

// ValidateClientVersion validates a configurable minimum or simulated client
// version before it is persisted.
func ValidateClientVersion(value string) error {
	value = strings.TrimSpace(value)
	if !clientVersionPattern.MatchString(value) {
		return fmt.Errorf("invalid client version %q", value)
	}
	if _, err := version.NewVersion(value); err != nil {
		return fmt.Errorf("invalid client version %q", value)
	}
	return nil
}
