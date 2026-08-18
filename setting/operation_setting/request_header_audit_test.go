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
	assert.Equal(t, RequestHeaderPolicy{Record: false, Forward: false}, ResolveRequestHeaderPolicy("Forwarded", nil))
	assert.Equal(t, RequestHeaderPolicy{Record: true, Forward: true}, ResolveRequestHeaderPolicy("X-OpenAI-Subagent", nil))
}

func TestParseRequestHeaderRulesValidatesEditableRules(t *testing.T) {
	valid := `[{"name":"X-Debug","record":true,"forward":false},{"name":"Sec-Fetch-*","record":false,"forward":false}]`
	rules, err := ParseRequestHeaderRules(valid)
	require.NoError(t, err)
	assert.Equal(t, []RequestHeaderRule{
		{Name: "X-Debug", Record: true, Forward: false},
		{Name: "Sec-Fetch-*", Record: false, Forward: false},
	}, rules)

	for _, value := range []string{
		"",
		`[{"name":"*debug*","record":false,"forward":false}]`,
		`[{"name":"Bad Header","record":false,"forward":false}]`,
		`[{"name":"Forwarded","record":false,"forward":false}]`,
		`[{"name":"X-Debug","record":false,"forward":false},{"name":"x-debug","record":true,"forward":true}]`,
		strings.Repeat("a", requestHeaderRulesMaxBytes+1),
	} {
		assert.Error(t, ValidateRequestHeaderRulesJSON(value), value)
	}
}

func TestConvertLegacyRequestHeaderRulesPreservesCombinedActions(t *testing.T) {
	converted, err := ConvertLegacyRequestHeaderRules(
		"Content-Length\nSec-Fetch-*",
		"Forwarded\nSec-Fetch-*\nX-Stainless-*",
	)
	require.NoError(t, err)
	rules, err := ParseRequestHeaderRules(converted)
	require.NoError(t, err)

	assert.Equal(t, []RequestHeaderRule{
		{Name: "Content-Length", Record: false, Forward: true},
		{Name: "Sec-Fetch-*", Record: false, Forward: false},
		{Name: "X-Stainless-*", Record: true, Forward: false},
	}, rules)
}
