package operation_setting

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"golang.org/x/net/http/httpguts"
)

const (
	RequestHeaderRulesOptionKey          = "RequestHeaderRules"
	RequestHeaderRulesDefaultOptionKey   = "RequestHeaderRulesDefault"
	RequestHeaderSystemRulesOptionKey    = "RequestHeaderSystemRules"
	LegacyRequestHeaderIgnoredHeadersKey = "RequestHeaderAuditIgnoredHeaders"
	LegacyRequestHeaderBlockedHeadersKey = "RequestHeaderForwardingBlockedHeaders"
	RequestHeaderAuditCapacityBytes      = 16 * 1024
	requestHeaderRulesMaxBytes           = 8 * 1024
	requestHeaderRulesMaxRules           = 200
)

// RequestHeaderRule controls whether one matched header can be recorded and
// forwarded. User rules support exact names and a trailing * wildcard.
type RequestHeaderRule struct {
	Name    string `json:"name"`
	Record  bool   `json:"record"`
	Forward bool   `json:"forward"`
}

// RequestHeaderPolicy is the effective action for one concrete header name.
type RequestHeaderPolicy struct {
	Record  bool
	Forward bool
}

var defaultRequestHeaderRules = []RequestHeaderRule{
	{Name: "Content-Length", Record: false, Forward: true},
	{Name: "Transfer-Encoding", Record: false, Forward: true},
	{Name: "Connection", Record: false, Forward: true},
	{Name: "Proxy-Connection", Record: false, Forward: true},
	{Name: "Keep-Alive", Record: false, Forward: true},
	{Name: "Host", Record: false, Forward: true},
	{Name: "Accept-Encoding", Record: false, Forward: true},
	{Name: "TE", Record: false, Forward: true},
	{Name: "Trailer", Record: false, Forward: true},
	{Name: "Upgrade", Record: false, Forward: true},
	{Name: "Expect", Record: false, Forward: true},
	{Name: "Sec-Fetch-*", Record: false, Forward: false},
	{Name: "Sec-CH-*", Record: false, Forward: false},
	{Name: "Access-Control-Request-*", Record: false, Forward: false},
	{Name: "Via", Record: true, Forward: false},
	{Name: "X-Forwarded-*", Record: true, Forward: false},
	{Name: "CF-*", Record: true, Forward: false},
	{Name: "CDN-Loop", Record: true, Forward: false},
	{Name: "X-Stainless-*", Record: true, Forward: false},
}

var systemRequestHeaderRules = []RequestHeaderRule{
	{Name: "Cookie", Record: false, Forward: true},
	{Name: "Set-Cookie", Record: false, Forward: true},
	{Name: "X-Amz-Security-Token", Record: false, Forward: true},
	{Name: "X-CSRF-Token", Record: false, Forward: true},
	{Name: "CF-Access-Jwt-Assertion", Record: false, Forward: true},
	{Name: "Forwarded", Record: false, Forward: false},
	{Name: "X-Forwarded-For", Record: false, Forward: true},
	{Name: "X-Real-IP", Record: false, Forward: false},
	{Name: "CF-Connecting-IP", Record: false, Forward: true},
	{Name: "True-Client-IP", Record: false, Forward: true},
	{Name: "Referer", Record: false, Forward: true},
	{Name: "Baggage", Record: false, Forward: true},
	{Name: "*authorization*", Record: false, Forward: true},
	{Name: "*password*", Record: false, Forward: true},
	{Name: "*passwd*", Record: false, Forward: true},
	{Name: "*secret*", Record: false, Forward: true},
	{Name: "*credential*", Record: false, Forward: true},
	{Name: "*private-key*", Record: false, Forward: true},
	{Name: "*api-key*", Record: false, Forward: true},
	{Name: "*apikey*", Record: false, Forward: true},
	{Name: "*access-token*", Record: false, Forward: true},
	{Name: "*refresh-token*", Record: false, Forward: true},
	{Name: "*auth-token*", Record: false, Forward: true},
	{Name: "*session-token*", Record: false, Forward: true},
	{Name: "*signature*", Record: false, Forward: true},
}

func requestHeaderAuditRuleMatches(name string, rule string) bool {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return false
	}
	if strings.HasPrefix(rule, "*") && strings.HasSuffix(rule, "*") {
		fragment := strings.Trim(rule, "*")
		return fragment != "" && strings.Contains(strings.ToLower(name), strings.ToLower(fragment))
	}
	if strings.HasSuffix(rule, "*") {
		return strings.HasPrefix(strings.ToLower(name), strings.ToLower(strings.TrimSuffix(rule, "*")))
	}
	return strings.EqualFold(name, rule)
}

func requestHeaderRulesJSON(rules []RequestHeaderRule) string {
	data, err := common.Marshal(rules)
	if err != nil {
		panic(fmt.Sprintf("marshal built-in request header rules: %v", err))
	}
	return string(data)
}

// DefaultRequestHeaderRulesJSON returns the restorable user rule preset.
func DefaultRequestHeaderRulesJSON() string {
	return requestHeaderRulesJSON(defaultRequestHeaderRules)
}

// SystemRequestHeaderRulesJSON returns the immutable rules shown to admins.
func SystemRequestHeaderRulesJSON() string {
	return requestHeaderRulesJSON(systemRequestHeaderRules)
}

func isSystemRequestHeaderRuleName(name string) bool {
	for _, rule := range systemRequestHeaderRules {
		if strings.EqualFold(name, rule.Name) {
			return true
		}
	}
	return false
}

// ParseRequestHeaderRules validates and normalizes editable request header rules.
func ParseRequestHeaderRules(value string) ([]RequestHeaderRule, error) {
	if len(value) > requestHeaderRulesMaxBytes {
		return nil, fmt.Errorf("请求头规则不能超过 %d 字节", requestHeaderRulesMaxBytes)
	}

	var rules []RequestHeaderRule
	if err := common.UnmarshalJsonStr(value, &rules); err != nil {
		return nil, fmt.Errorf("请求头规则格式无效: %w", err)
	}
	if rules == nil {
		return nil, fmt.Errorf("请求头规则必须是 JSON 数组")
	}
	if len(rules) > requestHeaderRulesMaxRules {
		return nil, fmt.Errorf("请求头规则不能超过 %d 条", requestHeaderRulesMaxRules)
	}

	seen := make(map[string]struct{}, len(rules))
	for index := range rules {
		rules[index].Name = strings.TrimSpace(rules[index].Name)
		name := strings.TrimSuffix(rules[index].Name, "*")
		if strings.Contains(name, "*") || name == "" || !httpguts.ValidHeaderFieldName(name) {
			return nil, fmt.Errorf("无效的请求头规则: %s", rules[index].Name)
		}
		key := strings.ToLower(rules[index].Name)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("重复的请求头规则: %s", rules[index].Name)
		}
		if isSystemRequestHeaderRuleName(rules[index].Name) {
			return nil, fmt.Errorf("请求头规则由系统管理: %s", rules[index].Name)
		}
		seen[key] = struct{}{}
	}
	return rules, nil
}

// ValidateRequestHeaderRulesJSON validates the persisted editable rule set.
func ValidateRequestHeaderRulesJSON(value string) error {
	_, err := ParseRequestHeaderRules(value)
	return err
}

// DefaultLegacyRequestHeaderRuleLists returns the former two-list view of the
// current preset for installations that have only one legacy option stored.
func DefaultLegacyRequestHeaderRuleLists() (string, string) {
	ignored := make([]string, 0)
	blocked := make([]string, 0)
	for _, rule := range defaultRequestHeaderRules {
		if !rule.Record {
			ignored = append(ignored, rule.Name)
		}
		if !rule.Forward {
			blocked = append(blocked, rule.Name)
		}
	}
	return strings.Join(ignored, "\n"), strings.Join(blocked, "\n")
}

// ConvertLegacyRequestHeaderRules converts the former ignore/block lists into
// one rule set without writing either legacy option.
func ConvertLegacyRequestHeaderRules(ignored string, blocked string) (string, error) {
	rules := make([]RequestHeaderRule, 0)
	indexes := make(map[string]int)
	appendRules := func(value string, record bool, forward bool) {
		for _, rawName := range strings.Split(value, "\n") {
			name := strings.TrimSpace(rawName)
			if name == "" || isSystemRequestHeaderRuleName(name) {
				continue
			}
			key := strings.ToLower(name)
			if index, exists := indexes[key]; exists {
				rules[index].Record = rules[index].Record && record
				rules[index].Forward = rules[index].Forward && forward
				continue
			}
			indexes[key] = len(rules)
			rules = append(rules, RequestHeaderRule{Name: name, Record: record, Forward: forward})
		}
	}
	appendRules(ignored, false, true)
	appendRules(blocked, true, false)

	value := requestHeaderRulesJSON(rules)
	if err := ValidateRequestHeaderRulesJSON(value); err != nil {
		return "", fmt.Errorf("转换旧请求头规则: %w", err)
	}
	return value, nil
}

// GetRequestHeaderRules returns the validated active user rules. Invalid
// runtime state falls back to the built-in preset and is reported server-side.
func GetRequestHeaderRules() []RequestHeaderRule {
	common.OptionMapRWMutex.RLock()
	value := common.OptionMap[RequestHeaderRulesOptionKey]
	common.OptionMapRWMutex.RUnlock()

	rules, err := ParseRequestHeaderRules(value)
	if err == nil {
		return rules
	}
	common.SysError("invalid request header rules option: " + err.Error())
	return append([]RequestHeaderRule(nil), defaultRequestHeaderRules...)
}

func mostSpecificRequestHeaderRule(name string, rules []RequestHeaderRule) (RequestHeaderRule, bool) {
	for _, rule := range rules {
		if !strings.HasSuffix(rule.Name, "*") && strings.EqualFold(name, rule.Name) {
			return rule, true
		}
	}

	bestPrefixLength := -1
	var best RequestHeaderRule
	for _, rule := range rules {
		if !strings.HasSuffix(rule.Name, "*") || !requestHeaderAuditRuleMatches(name, rule.Name) {
			continue
		}
		prefixLength := len(strings.TrimSuffix(rule.Name, "*"))
		if prefixLength > bestPrefixLength {
			bestPrefixLength = prefixLength
			best = rule
		}
	}
	return best, bestPrefixLength >= 0
}

// ResolveRequestHeaderPolicy applies the most specific user rule, then the
// immutable system constraints. System rules can only remove capabilities.
func ResolveRequestHeaderPolicy(name string, rules []RequestHeaderRule) RequestHeaderPolicy {
	policy := RequestHeaderPolicy{Record: true, Forward: true}
	if rule, ok := mostSpecificRequestHeaderRule(name, rules); ok {
		policy.Record = rule.Record
		policy.Forward = rule.Forward
	}
	for _, rule := range systemRequestHeaderRules {
		if requestHeaderAuditRuleMatches(name, rule.Name) {
			policy.Record = policy.Record && rule.Record
			policy.Forward = policy.Forward && rule.Forward
		}
	}
	return policy
}
