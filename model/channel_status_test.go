package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelStatusPersistenceTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	memoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = memoryCacheEnabled
	})
}

func setupChannelStatusCacheTest(t *testing.T, migrateAbility bool) {
	t.Helper()
	originalDB := DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	channelSyncLock.Lock()
	originalChannels := channelsIDM
	originalRoutes := group2model2channels
	originalAdvancedConfig := channel2advancedCustomConfig
	channelsIDM = make(map[int]*Channel)
	group2model2channels = make(map[string]map[string][]int)
	channel2advancedCustomConfig = make(map[int]*dto.AdvancedCustomConfig)
	channelSyncLock.Unlock()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	if migrateAbility {
		require.NoError(t, db.AutoMigrate(&Ability{}))
	}
	DB = db
	common.MemoryCacheEnabled = true

	t.Cleanup(func() {
		DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		channelSyncLock.Lock()
		channelsIDM = originalChannels
		group2model2channels = originalRoutes
		channel2advancedCustomConfig = originalAdvancedConfig
		channelSyncLock.Unlock()
	})
}

func TestUpdateChannelStatusPersistsMultiKeyState(t *testing.T) {
	setupChannelStatusPersistenceTest(t)

	channel := Channel{
		Name:   "multi-key-status",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         2,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 1,
		},
	}
	require.NoError(t, DB.Create(&channel).Error)

	changed := UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "provider rejected key")
	require.True(t, changed)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, "provider rejected key", stored.ChannelInfo.MultiKeyDisabledReason[0])
	assert.NotZero(t, stored.ChannelInfo.MultiKeyDisabledTime[0])
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)
}

func TestGetNextEnabledKeyExcludingSkipsExhaustedTarget(t *testing.T) {
	setupChannelStatusCacheTest(t, false)
	channel := &Channel{
		Id:     1,
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModePolling,
		},
	}
	CacheUpdateChannel(channel)

	key, index, err := channel.GetNextEnabledKeyExcluding(map[int]struct{}{0: {}})
	require.Nil(t, err)
	assert.Equal(t, "key-b", key)
	assert.Equal(t, 1, index)
}

func TestGetRandomSatisfiedChannelWithoutCacheSelectsNextPriorityAfterExclusion(t *testing.T) {
	setupChannelStatusCacheTest(t, true)
	initCol()
	common.MemoryCacheEnabled = false
	highPriority := int64(100)
	lowPriority := int64(0)
	for _, fixture := range []struct {
		channelID int
		priority  *int64
	}{
		{channelID: 1, priority: &highPriority},
		{channelID: 2, priority: &lowPriority},
	} {
		channel := Channel{Id: fixture.channelID, Key: "key", Status: common.ChannelStatusEnabled}
		require.NoError(t, DB.Create(&channel).Error)
		require.NoError(t, DB.Create(&Ability{
			Group:     "default",
			Model:     "model-a",
			ChannelId: fixture.channelID,
			Enabled:   true,
			Priority:  fixture.priority,
		}).Error)
	}

	channel, err := GetRandomSatisfiedChannel("default", "model-a", 0, nil, map[int]struct{}{1: {}})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2, channel.Id)
}

func TestSaveStatusStateFromSingleKeySnapshotPreservesUnownedColumns(t *testing.T) {
	setupChannelStatusPersistenceTest(t)

	channel := Channel{
		Name:        "single-key-status",
		Key:         "original-key",
		Status:      common.ChannelStatusEnabled,
		Models:      "original-model",
		Group:       "default",
		UsedQuota:   100,
		ChannelInfo: ChannelInfo{},
	}
	require.NoError(t, DB.Create(&channel).Error)

	stale, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)

	concurrentChannelInfo := ChannelInfo{
		IsMultiKey:           true,
		MultiKeySize:         2,
		MultiKeyMode:         constant.MultiKeyModePolling,
		MultiKeyPollingIndex: 1,
	}
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"key":          "rotated-key",
		"used_quota":   gorm.Expr("used_quota + ?", 250),
		"models":       "concurrent-model",
		"channel_info": concurrentChannelInfo,
	}).Error)

	stale.Status = common.ChannelStatusManuallyDisabled
	stale.SetOtherInfo(map[string]interface{}{
		"status_reason": "manual operation",
		"status_time":   int64(1234),
	})
	require.NoError(t, stale.saveStatusState(DB))

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.Equal(t, "rotated-key", stored.Key)
	assert.Equal(t, int64(350), stored.UsedQuota)
	assert.Equal(t, "concurrent-model", stored.Models)
	assert.Equal(t, concurrentChannelInfo, stored.ChannelInfo)

	otherInfo := stored.GetOtherInfo()
	assert.Equal(t, "manual operation", otherInfo["status_reason"])
	assert.Equal(t, float64(1234), otherInfo["status_time"])
}

func TestUpdateChannelStatusPersistsMultiKeyEnableWhenChannelAlreadyEnabled(t *testing.T) {
	setupChannelStatusCacheTest(t, true)
	channel := &Channel{
		Id:     1,
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		Name:   "multi",
		Models: "model-a",
		Group:  "default",
		ChannelInfo: ChannelInfo{
			IsMultiKey:             true,
			MultiKeySize:           2,
			MultiKeyStatusList:     map[int]int{0: common.ChannelStatusAutoDisabled},
			MultiKeyDisabledReason: map[int]string{0: "timeout"},
			MultiKeyDisabledTime:   map[int]int64{0: 1},
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	CacheUpdateChannel(channel)

	require.True(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusEnabled, ""))

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.NotContains(t, stored.ChannelInfo.MultiKeyStatusList, 0)
	assert.NotContains(t, stored.ChannelInfo.MultiKeyDisabledReason, 0)
	assert.NotContains(t, stored.ChannelInfo.MultiKeyDisabledTime, 0)
	cached, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Empty(t, cached.ChannelInfo.MultiKeyStatusList)
}

func TestUpdateChannelStatusRollsBackBeforeUpdatingCache(t *testing.T) {
	setupChannelStatusCacheTest(t, false)
	channel := &Channel{Id: 1, Key: "key", Status: common.ChannelStatusEnabled, Name: "single", Models: "model-a", Group: "default"}
	require.NoError(t, DB.Create(channel).Error)
	CacheUpdateChannel(channel)

	assert.False(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusAutoDisabled, "timeout"))

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	cached, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, cached.Status)
	selected, err := GetRandomSatisfiedChannel("default", "model-a", 0, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, selected)
}

func TestChannelCacheReturnsSnapshotsAndTracksStatusRoutes(t *testing.T) {
	setupChannelStatusCacheTest(t, true)
	priority := int64(10)
	channel := &Channel{
		Id:       1,
		Key:      "key-a\nkey-b",
		Status:   common.ChannelStatusEnabled,
		Name:     "cached",
		Models:   "model-a",
		Group:    "default",
		Priority: &priority,
		ChannelInfo: ChannelInfo{
			IsMultiKey:         true,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled},
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{ChannelId: channel.Id, Group: "default", Model: "model-a", Enabled: true}).Error)
	CacheUpdateChannel(channel)

	first, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	first.Status = common.ChannelStatusAutoDisabled
	first.ChannelInfo.MultiKeyStatusList[1] = common.ChannelStatusAutoDisabled
	*first.Priority = 99
	second, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, second.Status)
	assert.NotContains(t, second.ChannelInfo.MultiKeyStatusList, 1)
	assert.EqualValues(t, 10, *second.Priority)

	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual"))
	selected, err := GetRandomSatisfiedChannel("default", "model-a", 0, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, selected)
	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusEnabled, "manual"))
	selected, err = GetRandomSatisfiedChannel("default", "model-a", 0, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, selected)
}
