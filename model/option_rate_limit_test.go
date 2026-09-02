/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateModelRequestRateLimitOptions(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		valid bool
	}{
		{name: "zero IP limit disables it", key: "ModelRequestIPRateLimitCount", value: "0", valid: true},
		{name: "positive IP limit", key: "ModelRequestIPRateLimitCount", value: "100000000", valid: true},
		{name: "negative IP limit", key: "ModelRequestIPRateLimitCount", value: "-1", valid: false},
		{name: "fractional IP limit", key: "ModelRequestIPRateLimitCount", value: "1.5", valid: false},
		{name: "overflowing IP limit", key: "ModelRequestIPRateLimitCount", value: "9223372036854775807", valid: false},
		{name: "minimum duration", key: "ModelRequestRateLimitDurationMinutes", value: "1", valid: true},
		{name: "maximum duration", key: "ModelRequestRateLimitDurationMinutes", value: "1440", valid: true},
		{name: "zero duration", key: "ModelRequestRateLimitDurationMinutes", value: "0", valid: false},
		{name: "duration above one day", key: "ModelRequestRateLimitDurationMinutes", value: "1441", valid: false},
		{name: "zero total limit", key: "ModelRequestRateLimitCount", value: "0", valid: true},
		{name: "negative total limit", key: "ModelRequestRateLimitCount", value: "-1", valid: false},
		{name: "minimum success limit", key: "ModelRequestRateLimitSuccessCount", value: "1", valid: true},
		{name: "zero success limit disables it", key: "ModelRequestRateLimitSuccessCount", value: "0", valid: true},
		{name: "zero IP success limit disables it", key: "ModelRequestIPRateLimitSuccessCount", value: "0", valid: true},
		{name: "positive IP success limit", key: "ModelRequestIPRateLimitSuccessCount", value: "100000000", valid: true},
		{name: "negative IP success limit", key: "ModelRequestIPRateLimitSuccessCount", value: "-1", valid: false},
		{name: "zero account concurrency disables it", key: setting.ModelRequestConcurrencyLimitOptionKey, value: "0", valid: true},
		{name: "maximum account concurrency", key: setting.ModelRequestConcurrencyLimitOptionKey, value: "10000", valid: true},
		{name: "account concurrency above maximum", key: setting.ModelRequestConcurrencyLimitOptionKey, value: "10001", valid: false},
		{name: "negative IP concurrency", key: setting.ModelRequestIPConcurrencyLimitOptionKey, value: "-1", valid: false},
		{name: "valid concurrency switch", key: setting.ModelRequestConcurrencyLimitEnabledOptionKey, value: "true", valid: true},
		{name: "invalid concurrency switch", key: setting.ModelRequestConcurrencyLimitEnabledOptionKey, value: "enabled", valid: false},
		{name: "maximum source window", key: setting.AccessSourceAssociationWindowHoursOptionKey, value: "168", valid: true},
		{name: "source window above seven days", key: setting.AccessSourceAssociationWindowHoursOptionKey, value: "169", valid: false},
		{name: "maximum account IP associations", key: setting.AccessSourceMaxIPsPerUserOptionKey, value: "1000", valid: true},
		{name: "too many account IP associations", key: setting.AccessSourceMaxIPsPerUserOptionKey, value: "1001", valid: false},
		{name: "too many accounts per IP", key: setting.AccessSourceMaxUsersPerIPOptionKey, value: "1001", valid: false},
		{name: "maximum source switch wait", key: setting.AccessSourceSwitchCooldownMinutesOptionKey, value: "1440", valid: true},
		{name: "source switch wait above one day", key: setting.AccessSourceSwitchCooldownMinutesOptionKey, value: "1441", valid: false},
		{name: "valid source limiter switch", key: setting.AccessSourceLimitEnabledOptionKey, value: "true", valid: true},
		{name: "invalid source limiter switch", key: setting.AccessSourceLimitEnabledOptionKey, value: "enabled", valid: false},
		{name: "valid request limit error template", key: setting.ModelRequestRateLimitAccountTotalErrorTemplateOptionKey, value: "Retry in {{.RetryAfter}}\n请在 {{.RetryAfter}} 后重试", valid: true},
		{name: "unsupported request limit error variable", key: setting.ModelRequestConcurrencyAccountErrorTemplateOptionKey, value: "{{.Period}}", valid: false},
		{name: "request limit error defaults are read only", key: setting.RequestLimitErrorTemplateDefaultsOptionKey, value: "{}", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOptionValue(test.key, test.value)
			if test.valid {
				assert.NoError(t, err)
				return
			}
			assert.Error(t, err)
		})
	}
}

func TestUpdateOptionMapKeepsValidRequestLimitErrorTemplateWhenInvalidUpdateIsRejected(t *testing.T) {
	const key = setting.ModelRequestRateLimitAccountTotalErrorTemplateOptionKey
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	validTemplate := "Retry in {{.RetryAfter}}\n请在 {{.RetryAfter}} 后重试"
	require.NoError(t, updateOptionMap(key, validTemplate))
	assert.Equal(t, validTemplate, common.OptionMap[key])

	assert.Error(t, updateOptionMap(key, "{{.Code}}"))
	assert.Equal(t, validTemplate, common.OptionMap[key])
}

func TestUpdateOptionMapKeepsValidIPLimitsWhenInvalidUpdatesAreRejected(t *testing.T) {
	previousIPLimit := setting.ModelRequestIPRateLimitCount
	previousIPSuccessLimit := setting.ModelRequestIPRateLimitSuccessCount
	common.OptionMapRWMutex.Lock()
	optionMapWasNil := common.OptionMap == nil
	if optionMapWasNil {
		common.OptionMap = map[string]string{}
	}
	previousValue, hadPreviousValue := common.OptionMap["ModelRequestIPRateLimitCount"]
	previousSuccessValue, hadPreviousSuccessValue := common.OptionMap["ModelRequestIPRateLimitSuccessCount"]
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		setting.ModelRequestIPRateLimitCount = previousIPLimit
		setting.ModelRequestIPRateLimitSuccessCount = previousIPSuccessLimit
		common.OptionMapRWMutex.Lock()
		if hadPreviousValue {
			common.OptionMap["ModelRequestIPRateLimitCount"] = previousValue
		} else {
			delete(common.OptionMap, "ModelRequestIPRateLimitCount")
		}
		if hadPreviousSuccessValue {
			common.OptionMap["ModelRequestIPRateLimitSuccessCount"] = previousSuccessValue
		} else {
			delete(common.OptionMap, "ModelRequestIPRateLimitSuccessCount")
		}
		if optionMapWasNil {
			common.OptionMap = nil
		}
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, updateOptionMap("ModelRequestIPRateLimitCount", "25"))
	require.NoError(t, updateOptionMap("ModelRequestIPRateLimitSuccessCount", "10"))
	assert.Equal(t, 25, setting.ModelRequestIPRateLimitCount)
	assert.Equal(t, "25", common.OptionMap["ModelRequestIPRateLimitCount"])
	assert.Equal(t, 10, setting.ModelRequestIPRateLimitSuccessCount)
	assert.Equal(t, "10", common.OptionMap["ModelRequestIPRateLimitSuccessCount"])

	assert.Error(t, updateOptionMap("ModelRequestIPRateLimitCount", "-1"))
	assert.Error(t, updateOptionMap("ModelRequestIPRateLimitSuccessCount", "-1"))
	assert.Equal(t, 25, setting.ModelRequestIPRateLimitCount)
	assert.Equal(t, "25", common.OptionMap["ModelRequestIPRateLimitCount"])
	assert.Equal(t, 10, setting.ModelRequestIPRateLimitSuccessCount)
	assert.Equal(t, "10", common.OptionMap["ModelRequestIPRateLimitSuccessCount"])
}

func TestUpdateOptionMapKeepsValidConcurrencyLimitsWhenInvalidUpdatesAreRejected(t *testing.T) {
	previousEnabled := setting.ModelRequestConcurrencyLimitEnabled
	previousAccountLimit := setting.ModelRequestConcurrencyLimit
	previousIPLimit := setting.ModelRequestIPConcurrencyLimit
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		setting.ModelRequestConcurrencyLimitEnabled = previousEnabled
		setting.ModelRequestConcurrencyLimit = previousAccountLimit
		setting.ModelRequestIPConcurrencyLimit = previousIPLimit
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, updateOptionMap(setting.ModelRequestConcurrencyLimitEnabledOptionKey, "true"))
	require.NoError(t, updateOptionMap(setting.ModelRequestConcurrencyLimitOptionKey, "3"))
	require.NoError(t, updateOptionMap(setting.ModelRequestIPConcurrencyLimitOptionKey, "5"))
	assert.True(t, setting.ModelRequestConcurrencyLimitEnabled)
	assert.Equal(t, 3, setting.ModelRequestConcurrencyLimit)
	assert.Equal(t, 5, setting.ModelRequestIPConcurrencyLimit)

	assert.Error(t, updateOptionMap(setting.ModelRequestConcurrencyLimitOptionKey, "10001"))
	assert.Error(t, updateOptionMap(setting.ModelRequestIPConcurrencyLimitOptionKey, "-1"))
	assert.Equal(t, 3, setting.ModelRequestConcurrencyLimit)
	assert.Equal(t, 5, setting.ModelRequestIPConcurrencyLimit)
}
