package operation_setting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRequestHeaderPolicyUsesSpecificUserRuleThenSystemConstraints(t *testing.T) {
	rules := []RequestHeaderRule{
		{Name: "X-*", Record: false, Forward: false},
		{Name: "X-Trace-*", Record: true, Forward: false},
		{Name: "X-Trace-Allowed", Record: true, Forward: true},
		{Name: "Accept*", Record: false, Forward: true},
		{Name: "Authorization", Record: true, Forward: false},
	}

	assert.Equal(t, RequestHeaderPolicy{Record: true, Forward: true}, ResolveRequestHeaderPolicy("X-Trace-Allowed", rules))
	assert.Equal(t, RequestHeaderPolicy{Record: true, Forward: false}, ResolveRequestHeaderPolicy("X-Trace-Other", rules))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: false}, ResolveRequestHeaderPolicy("X-Other", rules))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: true}, ResolveRequestHeaderPolicy("Accept", rules))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: false}, ResolveRequestHeaderPolicy("Authorization", rules))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: false}, ResolveRequestHeaderPolicy("Connection", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: false}, ResolveRequestHeaderPolicy("Content-Length", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: true}, ResolveRequestHeaderPolicy("Accept", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: true}, ResolveRequestHeaderPolicy("Content-Type", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: false}, ResolveRequestHeaderPolicy("Accept-Encoding", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: false}, ResolveRequestHeaderPolicy("Cache-Control", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: false}, ResolveRequestHeaderPolicy("If-None-Match", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: true}, ResolveRequestHeaderPolicy("Sec-WebSocket-Protocol", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: false}, ResolveRequestHeaderPolicy("Sec-WebSocket-Key", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: true}, ResolveRequestHeaderPolicy("X-Webhook-Signature", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: true, Forward: true}, ResolveRequestHeaderPolicy("Signature-Algorithm", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: false}, ResolveRequestHeaderPolicy("Forwarded", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: true, Forward: true}, ResolveRequestHeaderPolicy("X-Request-ID", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: true, Forward: true}, ResolveRequestHeaderPolicy("X-OpenAI-Subagent", nil))
}

func TestParseRequestHeaderRulesValidatesEditableRules(t *testing.T) {
	valid := `[{"name":"X-Debug","record":true,"forward":false},{"name":"X-Trace-*","record":false,"forward":false}]`
	rules, err := ParseRequestHeaderRules(valid)
	require.NoError(t, err)
	assert.Equal(t, []RequestHeaderRule{
		{Name: "X-Debug", Record: true, Forward: false},
		{Name: "X-Trace-*", Record: false, Forward: false},
	}, rules)

	for _, value := range []string{
		"",
		`[{"name":"*debug*","record":false,"forward":false}]`,
		`[{"name":"Bad Header","record":false,"forward":false}]`,
		`[{"name":"Connection","record":false,"forward":false}]`,
		`[{"name":"Authorization","record":false,"forward":true}]`,
		`[{"name":"X-Debug","record":false,"forward":false},{"name":"x-debug","record":true,"forward":true}]`,
		strings.Repeat("a", requestHeaderRulesMaxBytes+1),
	} {
		assert.Error(t, ValidateRequestHeaderRulesJSON(value), value)
	}
}

func TestConvertLegacyRequestHeaderRulesPreservesCombinedActions(t *testing.T) {
	converted, err := ConvertLegacyRequestHeaderRules(
		"Content-Length\nBaggage",
		"Forwarded\nBaggage\nX-Debug",
	)
	require.NoError(t, err)
	rules, err := ParseRequestHeaderRules(converted)
	require.NoError(t, err)

	assert.Equal(t, []RequestHeaderRule{
		{Name: "Baggage", Record: false, Forward: false},
		{Name: "X-Debug", Record: true, Forward: false},
		{Name: "CF-*", Record: false, Forward: false},
		{Name: "EO-*", Record: false, Forward: false},
		{Name: "Ali-*", Record: false, Forward: false},
		{Name: "ESA-*", Record: false, Forward: false},
		{Name: "TLS-Hash", Record: false, Forward: false},
		{Name: "TLS-JA3", Record: false, Forward: false},
		{Name: "TLS-JA4", Record: false, Forward: false},
	}, rules)
}

func TestValidateRequestHeaderRulesRequiresManagedCDNRules(t *testing.T) {
	require.NoError(t, ValidateRequestHeaderRulesJSON(DefaultRequestHeaderRulesJSON()))
	require.EqualError(
		t,
		ValidateRequestHeaderRulesJSON(`[{"name":"X-Request-ID","record":false,"forward":false}]`),
		"缺少系统管理的 CDN 请求头规则: CF-*",
	)
}

func TestDefaultRequestHeaderRulesClassifyCommonHeaders(t *testing.T) {
	rules, err := ParseRequestHeaderRules(DefaultRequestHeaderRulesJSON())
	require.NoError(t, err)

	blockedHeaders := []string{
		"Sec-Fetch-Site",
		"Sec-CH-UA",
		"Access-Control-Request-Method",
		"Origin",
		"Referer",
		"Accept-Language",
		"Cache-Control",
		"Pragma",
		"Priority",
		"Range",
		"If-None-Match",
		"Last-Event-ID",
		"DNT",
		"Sec-GPC",
		"X-Requested-With",
		"Ali-Real-Client-IP",
		"Ali-IP-Country",
		"Ali-IP-City",
		"CDN-Loop",
		"CF-IPCountry",
		"CF-Ray",
		"CF-Visitor",
		"EO-Connecting-IP",
		"EO-Log-UUID",
		"ESA-User-Risk",
		"Forwarded",
		"TLS-Hash",
		"TLS-JA3",
		"TLS-JA4",
		"X-Forwarded-Proto",
		"X-Real-IP",
		"X-Stainless-Runtime",
		"Baggage",
		"traceparent",
		"tracestate",
		"X-Request-ID",
		"X-Correlation-ID",
	}
	for _, name := range blockedHeaders {
		assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: false}, ResolveRequestHeaderPolicy(name, rules), name)
	}

	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: true}, ResolveRequestHeaderPolicy("Accept", rules))
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: true}, ResolveRequestHeaderPolicy("Content-Type", rules))
	assert.Equal(t, RequestHeaderPolicy{Record: true, Forward: true}, ResolveRequestHeaderPolicy("X-Custom-Header", rules))
}
