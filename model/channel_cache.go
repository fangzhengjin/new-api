package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var group2model2channels map[string]map[string][]int // enabled channel
var channelsIDM map[int]*Channel                     // all channels include disabled
// channel2advancedCustomConfig caches parsed Advanced Custom (type 58) configs so
// path-aware selection avoids re-parsing JSON per request. Refreshed on full sync.
var channel2advancedCustomConfig map[int]*kitdto.AdvancedCustomConfig
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		InvalidatePricingCache()
		rebuildTaskAliasView()
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	newChannel2advancedCustomConfig := make(map[int]*kitdto.AdvancedCustomConfig)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
		if channel.Type == constant.ChannelTypeAdvancedCustom {
			if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
				newChannel2advancedCustomConfig[channel.Id] = config
			}
		}
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		groups := strings.Split(channel.Group, ",")
		for _, group := range groups {
			models := strings.Split(channel.Models, ",")
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channel2advancedCustomConfig = newChannel2advancedCustomConfig
	channelSyncLock.Unlock()
	// Lock ordering: InvalidatePricingCache acquires updatePricingLock, and
	// GetPricing (holding updatePricingLock) nests channelSyncLock.RLock via
	// loadPricingAdvancedCustomConfigs. channelSyncLock MUST be released before
	// invalidating the pricing cache, otherwise the reversed order deadlocks.
	InvalidatePricingCache()
	rebuildTaskAliasView()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func GetRandomSatisfiedChannel(
	group string,
	model string,
	retry int,
	filters []dto.ChannelFilter,
	excludedChannels map[int]struct{},
) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannel(group, model, retry, filters, excludedChannels)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// First, try to find channels with the exact model name.
	channels, _ := filterCandidateIDs(group2model2channels[group][model], model, filters)

	// If no channels found, try to find channels with the normalized model name.
	if len(channels) == 0 {
		normalizedModel := ratio_setting.RoutingMatchModelName(model)
		channels, _ = filterCandidateIDs(group2model2channels[group][normalizedModel], model, filters)
	}

	if len(channels) == 0 {
		return nil, nil
	}
	if len(excludedChannels) > 0 {
		filtered := make([]int, 0, len(channels))
		for _, channelId := range channels {
			if _, ok := excludedChannels[channelId]; !ok {
				filtered = append(filtered, channelId)
			}
		}
		channels = filtered
		if len(channels) == 0 {
			return nil, nil
		}
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return copyChannelSnapshot(channel), nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(channel.GetPriority())] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := int64(sortedUniquePriorities[retry])

	// get the priority for the given retry number
	var sumWeight = 0
	var targetChannels []*Channel
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				sumWeight += channel.GetWeight()
				targetChannels = append(targetChannels, channel)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority))
	}

	// smoothing factor and adjustment
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		// when all channels have weight 0, set sumWeight to the number of channels and set smoothing adjustment to 100
		// each channel's effective weight = 100
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		// when the average weight is less than 10, set smoothing factor to 100
		smoothingFactor = 100
	}

	// Calculate the total weight of all channels up to endIdx
	totalWeight := sumWeight * smoothingFactor

	// Generate a random value in the range [0, totalWeight)
	randomWeight := rand.Intn(totalWeight)

	// Find a channel based on its weight
	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return copyChannelSnapshot(channel), nil
		}
	}
	// return null if no channel is not found
	return nil, errors.New("channel not found")
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return copyChannelSnapshot(c), nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		info := copyChannelInfoSnapshot(channel.ChannelInfo)
		return &info, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	info := copyChannelInfoSnapshot(c.ChannelInfo)
	return &info, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
		rebuildChannelRoutesLocked()
	}
}

func cacheUpdateChannelStatusState(state *Channel) {
	if !common.MemoryCacheEnabled || state == nil {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	cached, ok := channelsIDM[state.Id]
	if !ok {
		return
	}
	updated := copyChannelSnapshot(cached)
	updated.Status = state.Status
	updated.OtherInfo = state.OtherInfo
	if state.ChannelInfo.IsMultiKey {
		pollingIndex := updated.ChannelInfo.MultiKeyPollingIndex
		updated.ChannelInfo = copyChannelInfoSnapshot(state.ChannelInfo)
		updated.ChannelInfo.MultiKeyPollingIndex = pollingIndex
	}
	channelsIDM[state.Id] = updated
	rebuildChannelRoutesLocked()
}

func cacheUpdateChannelPollingIndex(id int, index int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.ChannelInfo.MultiKeyPollingIndex = index
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	if channel == nil {
		channelSyncLock.Unlock()
		return
	}

	if channelsIDM == nil {
		channelsIDM = make(map[int]*Channel)
	}
	cached := copyChannelSnapshot(channel)
	if oldChannel, ok := channelsIDM[channel.Id]; ok {
		logger.LogDebug(nil, "CacheUpdateChannel before: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, oldChannel.ChannelInfo.MultiKeyPollingIndex)
		if cached.ChannelInfo.IsMultiKey && cached.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling &&
			oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
			cached.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
		}
	}
	channelsIDM[channel.Id] = cached
	if channel2advancedCustomConfig == nil {
		channel2advancedCustomConfig = make(map[int]*kitdto.AdvancedCustomConfig)
	}
	delete(channel2advancedCustomConfig, channel.Id)
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
			channel2advancedCustomConfig[channel.Id] = config
		}
	}
	rebuildChannelRoutesLocked()
	logger.LogDebug(nil, "CacheUpdateChannel after: id=%d, name=%s, status=%d, polling_index=%d", cached.Id, cached.Name, cached.Status, cached.ChannelInfo.MultiKeyPollingIndex)
	// Lock ordering: do NOT hold channelSyncLock while calling
	// InvalidatePricingCache. GetPricing acquires updatePricingLock first and then
	// channelSyncLock.RLock (via loadPricingAdvancedCustomConfigs); acquiring
	// updatePricingLock while holding channelSyncLock would be an AB-BA deadlock.
	channelSyncLock.Unlock()
	InvalidatePricingCache()
}

func rebuildChannelRoutesLocked() {
	routes := make(map[string]map[string][]int)
	for id, channel := range channelsIDM {
		if channel.Status != common.ChannelStatusEnabled {
			continue
		}
		for _, group := range strings.Split(channel.Group, ",") {
			if routes[group] == nil {
				routes[group] = make(map[string][]int)
			}
			for _, model := range strings.Split(channel.Models, ",") {
				routes[group][model] = append(routes[group][model], id)
			}
		}
	}
	for _, models := range routes {
		for _, ids := range models {
			sort.Slice(ids, func(i, j int) bool {
				return channelsIDM[ids[i]].GetPriority() > channelsIDM[ids[j]].GetPriority()
			})
		}
	}
	group2model2channels = routes
}

func copyChannelSnapshot(channel *Channel) *Channel {
	if channel == nil {
		return nil
	}
	copied := *channel
	copied.OpenAIOrganization = copyPtr(channel.OpenAIOrganization)
	copied.TestModel = copyPtr(channel.TestModel)
	copied.Weight = copyPtr(channel.Weight)
	copied.BaseURL = copyPtr(channel.BaseURL)
	copied.ModelMapping = copyPtr(channel.ModelMapping)
	copied.StatusCodeMapping = copyPtr(channel.StatusCodeMapping)
	copied.Priority = copyPtr(channel.Priority)
	copied.AutoBan = copyPtr(channel.AutoBan)
	copied.Tag = copyPtr(channel.Tag)
	copied.Setting = copyPtr(channel.Setting)
	copied.ParamOverride = copyPtr(channel.ParamOverride)
	copied.HeaderOverride = copyPtr(channel.HeaderOverride)
	copied.Remark = copyPtr(channel.Remark)
	copied.ChannelInfo = copyChannelInfoSnapshot(channel.ChannelInfo)
	copied.Keys = append([]string(nil), channel.Keys...)
	return &copied
}

func copyChannelInfoSnapshot(info ChannelInfo) ChannelInfo {
	info.MultiKeyStatusList = copyMap(info.MultiKeyStatusList)
	info.MultiKeyDisabledReason = copyMap(info.MultiKeyDisabledReason)
	info.MultiKeyDisabledTime = copyMap(info.MultiKeyDisabledTime)
	return info
}

func copyPtr[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func copyMap[K comparable, V any](source map[K]V) map[K]V {
	if source == nil {
		return nil
	}
	copied := make(map[K]V, len(source))
	for key, value := range source {
		copied[key] = value
	}
	return copied
}
