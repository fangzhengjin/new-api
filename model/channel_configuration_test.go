package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyDiscoveredChannelMergesLatestRuntimeState(t *testing.T) {
	truncateTables(t)
	baseURL := "https://api.example.test"
	mapping := ""
	testModel := "gpt-5.4"
	priority := int64(1)
	weight := uint(2)
	tag := "old-tag"
	channel := Channel{
		Type:         constant.ChannelTypeOpenAI,
		Key:          "first-key\nsecond-key",
		Status:       common.ChannelStatusEnabled,
		Name:         "example",
		BaseURL:      &baseURL,
		Models:       "gpt-5.4",
		Group:        "default",
		ModelMapping: &mapping,
		TestModel:    &testModel,
		Priority:     &priority,
		Weight:       &weight,
		Tag:          &tag,
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AllowSpeed:                       true,
		UpstreamModelUpdateLastCheckTime: 10,
	})
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	snapshotHash, err := ChannelConfigurationSnapshot(&channel)
	require.NoError(t, err)

	desired := channel
	desired.Type = constant.ChannelTypeAdvancedCustom
	desired.Key += "\nthird-key"
	desired.Models = "gpt-5.4,gpt-5.6-luna"
	desiredTestModel := "gpt-5.6-luna"
	desired.TestModel = &desiredTestModel
	desired.ChannelInfo = ChannelInfo{
		IsMultiKey:   true,
		MultiKeySize: 3,
		MultiKeyMode: constant.MultiKeyModeRandom,
	}
	desired.SetOtherSettings(dto.ChannelOtherSettings{
		AllowSpeed:                       true,
		UpstreamModelUpdateLastCheckTime: 10,
		AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{
			{IncomingPath: "/v1/responses", UpstreamPath: "/vendor/responses"},
		}},
	})

	latestInfo := ChannelInfo{
		IsMultiKey:             true,
		MultiKeySize:           2,
		MultiKeyMode:           constant.MultiKeyModePolling,
		MultiKeyStatusList:     map[int]int{1: common.ChannelStatusManuallyDisabled},
		MultiKeyDisabledReason: map[int]string{1: "manual"},
		MultiKeyDisabledTime:   map[int]int64{1: 123},
		MultiKeyPollingIndex:   1,
	}
	latest := channel.GetOtherSettings()
	latest.UpstreamModelUpdateCheckEnabled = true
	latest.UpstreamModelUpdateAutoSyncEnabled = true
	latest.UpstreamModelUpdateLastCheckTime = 20
	latest.UpstreamModelUpdateLastDetectedModels = []string{"gpt-5.5"}
	channel.SetOtherSettings(latest)
	latestPriority := int64(9)
	latestWeight := uint(7)
	latestTag := "latest-tag"
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"channel_info": latestInfo,
		"group":        "vip",
		"priority":     &latestPriority,
		"settings":     channel.OtherSettings,
		"tag":          &latestTag,
		"weight":       &latestWeight,
	}).Error)
	latestChannel := channel
	latestChannel.Group = "vip"
	latestChannel.Priority = &latestPriority
	latestChannel.Tag = &latestTag
	latestChannel.Weight = &latestWeight
	require.NoError(t, latestChannel.UpdateAbilities(nil))

	_, err = ApplyDiscoveredChannel(&desired, snapshotHash, true, false)
	require.NoError(t, err)

	var saved Channel
	require.NoError(t, DB.First(&saved, channel.Id).Error)
	assert.Equal(t, latestInfo.MultiKeyMode, saved.ChannelInfo.MultiKeyMode)
	assert.Equal(t, latestInfo.MultiKeyStatusList, saved.ChannelInfo.MultiKeyStatusList)
	assert.Equal(t, latestInfo.MultiKeyPollingIndex, saved.ChannelInfo.MultiKeyPollingIndex)
	assert.Equal(t, 3, saved.ChannelInfo.MultiKeySize)
	settings := saved.GetOtherSettings()
	assert.Equal(t, int64(20), settings.UpstreamModelUpdateLastCheckTime)
	assert.Equal(t, []string{"gpt-5.5"}, settings.UpstreamModelUpdateLastDetectedModels)
	assert.NotNil(t, settings.AdvancedCustom)
	assert.False(t, settings.UpstreamModelUpdateCheckEnabled)
	assert.False(t, settings.UpstreamModelUpdateAutoSyncEnabled)
	assert.Equal(t, desiredTestModel, *saved.TestModel)
	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Order("model asc").Find(&abilities).Error)
	require.Len(t, abilities, 2)
	for _, ability := range abilities {
		assert.Equal(t, "vip", ability.Group)
		assert.Equal(t, latestPriority, *ability.Priority)
		assert.Equal(t, latestWeight, ability.Weight)
		assert.Equal(t, latestTag, *ability.Tag)
	}
}

func TestApplyDiscoveredChannelReplaceWithoutSyncPreservesLatestConfiguration(t *testing.T) {
	truncateTables(t)
	baseURL := "https://api.example.test"
	mapping := ""
	testModel := "gpt-5.4"
	channel := Channel{
		Type:         constant.ChannelTypeOpenAI,
		Key:          "first-key\nsecond-key",
		Status:       common.ChannelStatusEnabled,
		Name:         "example",
		BaseURL:      &baseURL,
		Models:       "gpt-5.4",
		Group:        "default",
		ModelMapping: &mapping,
		TestModel:    &testModel,
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{UpstreamModelUpdateLastCheckTime: 10})
	require.NoError(t, DB.Create(&channel).Error)
	snapshotHash, err := ChannelConfigurationSnapshot(&channel)
	require.NoError(t, err)

	desired := channel
	desired.Key = "replacement-one\nreplacement-two"
	desired.ChannelInfo = ChannelInfo{
		IsMultiKey:   true,
		MultiKeySize: 2,
		MultiKeyMode: constant.MultiKeyModeRandom,
	}
	latestInfo := ChannelInfo{
		IsMultiKey:             true,
		MultiKeySize:           2,
		MultiKeyMode:           constant.MultiKeyModePolling,
		MultiKeyStatusList:     map[int]int{0: common.ChannelStatusManuallyDisabled},
		MultiKeyDisabledReason: map[int]string{0: "manual"},
		MultiKeyDisabledTime:   map[int]int64{0: 456},
		MultiKeyPollingIndex:   1,
	}
	latestTestModel := "concurrent-test-model"
	latestSettings := channel.GetOtherSettings()
	latestSettings.UpstreamModelUpdateLastCheckTime = 99
	channel.SetOtherSettings(latestSettings)
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"channel_info": latestInfo,
		"settings":     channel.OtherSettings,
		"test_model":   &latestTestModel,
	}).Error)

	_, err = ApplyDiscoveredChannel(&desired, snapshotHash, false, true)
	require.NoError(t, err)

	var saved Channel
	require.NoError(t, DB.First(&saved, channel.Id).Error)
	assert.Equal(t, constant.MultiKeyModePolling, saved.ChannelInfo.MultiKeyMode)
	assert.Empty(t, saved.ChannelInfo.MultiKeyStatusList)
	assert.Empty(t, saved.ChannelInfo.MultiKeyDisabledReason)
	assert.Empty(t, saved.ChannelInfo.MultiKeyDisabledTime)
	assert.Zero(t, saved.ChannelInfo.MultiKeyPollingIndex)
	assert.Equal(t, int64(99), saved.GetOtherSettings().UpstreamModelUpdateLastCheckTime)
	assert.Equal(t, latestTestModel, *saved.TestModel)
}

func TestApplyChannelNormalizationsRollsBackOnSnapshotConflict(t *testing.T) {
	truncateTables(t)
	baseURL := "https://api.example.test"
	mapping := ""
	channels := []Channel{
		{Type: 1, Key: "first-key", Status: common.ChannelStatusEnabled, Name: "first", BaseURL: &baseURL, Models: "gpt-5.4", Group: "default", ModelMapping: &mapping},
		{Type: 1, Key: "second-key", Status: common.ChannelStatusEnabled, Name: "second", BaseURL: &baseURL, Models: "gpt-5.4", Group: "default", ModelMapping: &mapping},
	}
	require.NoError(t, DB.Create(&channels).Error)
	firstHash, err := ChannelConfigurationSnapshot(&channels[0])
	require.NoError(t, err)

	task := SystemTask{
		TaskID: "normalize-conflict",
		Type:   SystemTaskTypeChannelNormalize,
		Status: SystemTaskStatusSucceeded,
		Result: `{"summary":{"scanned":2}}`,
	}
	require.NoError(t, DB.Create(&task).Error)

	mutations := []ChannelNormalizationMutation{
		{
			ChannelID:    channels[0].Id,
			SnapshotHash: firstHash,
			AddModels:    []string{"manual-alias"},
			MappingSet:   map[string]string{"manual-alias": "gpt-5.4"},
			SortModels:   true,
		},
		{
			ChannelID:    channels[1].Id,
			SnapshotHash: "stale",
			AddModels:    []string{"manual-alias"},
			MappingSet:   map[string]string{"manual-alias": "gpt-5.4"},
			SortModels:   true,
		},
	}
	_, err = ApplyChannelNormalizations(task.TaskID, task.Result, map[string]any{"applied_at": 1}, mutations)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrChannelConfigurationConflict))

	var first Channel
	require.NoError(t, DB.First(&first, channels[0].Id).Error)
	assert.Equal(t, "gpt-5.4", first.Models)
	var savedTask SystemTask
	require.NoError(t, DB.First(&savedTask, task.ID).Error)
	assert.Equal(t, task.Result, savedTask.Result)
}
