package model_setting

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

var clientIdentityModelRegexCache sync.Map // pattern string -> *regexp.Regexp

// MatchesClientIdentityModel reports whether the mapped upstream model is in
// an administrator-configured client identity family.
func MatchesClientIdentityModel(patterns []string, upstreamModel string) bool {
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		cached, ok := clientIdentityModelRegexCache.Load(pattern)
		if !ok {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			cached, _ = clientIdentityModelRegexCache.LoadOrStore(pattern, compiled)
		}
		if cached.(*regexp.Regexp).MatchString(upstreamModel) {
			return true
		}
	}
	return false
}

// ValidateClientIdentityModelPatterns validates the JSON array persisted by
// the option API before the patterns enter the outbound request path.
func ValidateClientIdentityModelPatterns(value string) error {
	var patterns []string
	if err := common.UnmarshalJsonStr(value, &patterns); err != nil {
		return fmt.Errorf("client identity model patterns must be a JSON string array: %w", err)
	}
	if len(patterns) == 0 || len(patterns) > 64 {
		return fmt.Errorf("client identity model patterns must contain between 1 and 64 entries")
	}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" || len(pattern) > 256 {
			return fmt.Errorf("client identity model pattern must contain between 1 and 256 characters")
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("invalid client identity model pattern %q: %w", pattern, err)
		}
	}
	return nil
}
