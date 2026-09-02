package setting

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRequestLimitErrorTemplate(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{
			name:  "accepts supported variables and line breaks",
			key:   ModelRequestRateLimitAccountTotalErrorTemplateOptionKey,
			value: "Retry in {{.RetryAfter}}\n请在 {{.RetryAfter}} 后重试",
		},
		{
			name:  "accepts an empty override",
			key:   ModelRequestConcurrencyAccountErrorTemplateOptionKey,
			value: "",
		},
		{
			name:    "rejects an unavailable variable",
			key:     ModelRequestConcurrencyAccountErrorTemplateOptionKey,
			value:   "{{.Period}}",
			wantErr: true,
		},
		{
			name:    "rejects an unexposed error code",
			key:     AccessSourceSwitchCooldownErrorTemplateOptionKey,
			value:   "{{.Code}}",
			wantErr: true,
		},
		{
			name:    "rejects template control flow",
			key:     ModelRequestRateLimitIPTotalErrorTemplateOptionKey,
			value:   "{{if .Limit}}{{.Limit}}{{end}}",
			wantErr: true,
		},
		{
			name:    "rejects associated templates",
			key:     ModelRequestRateLimitIPTotalErrorTemplateOptionKey,
			value:   `{{define "hidden"}}{{.Code}}{{end}}Retry after {{.RetryAfter}}`,
			wantErr: true,
		},
		{
			name:    "rejects invalid syntax",
			key:     ModelRequestRateLimitIPSuccessErrorTemplateOptionKey,
			value:   "{{.Limit",
			wantErr: true,
		},
		{
			name:    "rejects more than 2000 characters",
			key:     AccessSourceAccountIPLimitErrorTemplateOptionKey,
			value:   strings.Repeat("额", MaxRequestLimitErrorTemplateLength+1),
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRequestLimitErrorTemplate(test.key, test.value)
			if test.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestRequestLimitErrorTemplatesRenderDefaultsAndOverrides(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = make(map[string]string, len(requestLimitErrorTemplateSpecs))
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})

	values := RequestLimitErrorTemplateValues{
		Limit:      "10",
		Period:     "1m",
		RetryAfter: "2m 15s",
	}
	for key := range GetDefaultRequestLimitErrorTemplates() {
		message, err := RenderRequestLimitErrorTemplate(key, values)
		require.NoError(t, err)
		assert.Contains(t, message, "\n")
	}

	key := ModelRequestRateLimitAccountTotalErrorTemplateOptionKey
	common.OptionMapRWMutex.Lock()
	common.OptionMap[key] = "Retry in {{.RetryAfter}}\n请在 {{.RetryAfter}} 后重试"
	common.OptionMapRWMutex.Unlock()
	message, err := RenderRequestLimitErrorTemplate(key, values)
	require.NoError(t, err)
	assert.Equal(t, "Retry in 2m 15s\n请在 2m 15s 后重试", message)

	assert.Error(t, ValidateRequestLimitErrorTemplate(key, "{{.Code}}"))
	message, err = RenderRequestLimitErrorTemplate(key, values)
	require.NoError(t, err)
	assert.Equal(t, "Retry in 2m 15s\n请在 2m 15s 后重试", message)
}

func TestFormatRequestLimitDuration(t *testing.T) {
	tests := []struct {
		seconds int64
		want    string
	}{
		{seconds: -1, want: "0s"},
		{seconds: 0, want: "0s"},
		{seconds: 45, want: "45s"},
		{seconds: 60, want: "1m"},
		{seconds: 135, want: "2m 15s"},
		{seconds: 3599, want: "59m 59s"},
		{seconds: 3600, want: "1h"},
		{seconds: 3900, want: "1h 5m"},
		{seconds: 3930, want: "1h 5m"},
	}

	for _, test := range tests {
		assert.Equal(t, test.want, FormatRequestLimitDuration(test.seconds))
	}
}
