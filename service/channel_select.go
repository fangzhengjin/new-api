package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/gin-gonic/gin"
)

func GetChannelConstraints(c *gin.Context) *dto.ChannelConstraints {
	if c == nil {
		return &dto.ChannelConstraints{}
	}
	if existing, ok := common.GetContextKeyType[*dto.ChannelConstraints](c, constant.ContextKeyChannelConstraints); ok && existing != nil {
		return existing
	}
	constraints := &dto.ChannelConstraints{}
	common.SetContextKey(c, constant.ContextKeyChannelConstraints, constraints)
	return constraints
}

func AppendTaskPluginIdentityFilter(c *gin.Context, pluginKey string) {
	if c == nil {
		return
	}
	GetChannelConstraints(c).AddFilter(dto.ChannelFilter{
		Kind:                   dto.FilterTaskPluginIdentity,
		TaskPluginKey:          pluginKey,
		TaskPluginChannelTypes: pinnedTaskPluginChannelTypes(c, pluginKey),
	})
}

// RetryParam tracks request-local routing targets that have exhausted retries.
type RetryParam struct {
	Ctx              *gin.Context
	TokenGroup       string
	ModelName        string
	excludedChannels map[int]struct{}
	excludedKeys     map[int]map[int]struct{}
}

// ExcludeTarget prevents an exhausted channel or multi-key entry from being
// selected again during the current request.
func (p *RetryParam) ExcludeTarget(channelId int, isMultiKey bool, keyIndex int) {
	if !isMultiKey {
		p.ExcludeChannel(channelId)
		return
	}
	if p.excludedKeys == nil {
		p.excludedKeys = make(map[int]map[int]struct{})
	}
	if p.excludedKeys[channelId] == nil {
		p.excludedKeys[channelId] = make(map[int]struct{})
	}
	p.excludedKeys[channelId][keyIndex] = struct{}{}
}

// ExcludeChannel prevents a channel from being selected again during the
// current request.
func (p *RetryParam) ExcludeChannel(channelId int) {
	if p.excludedChannels == nil {
		p.excludedChannels = make(map[int]struct{})
	}
	p.excludedChannels[channelId] = struct{}{}
}

// ExcludedChannels returns the request-local set of exhausted channels.
func (p *RetryParam) ExcludedChannels() map[int]struct{} {
	return p.excludedChannels
}

// ExcludedKeyIndexes returns exhausted key indexes for one channel.
func (p *RetryParam) ExcludedKeyIndexes(channelId int) map[int]struct{} {
	return p.excludedKeys[channelId]
}

// CacheGetRandomSatisfiedChannel selects the highest-priority available
// channel after applying the current request's exhausted-channel exclusions.
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
	filters := GetChannelConstraints(param.Ctx).Filters
	if param.TokenGroup != "auto" {
		channel, err := model.GetRandomSatisfiedChannel(
			param.TokenGroup,
			param.ModelName,
			0,
			filters,
			param.ExcludedChannels(),
		)
		return channel, selectGroup, err
	}

	autoGroups := GetRequestAutoGroups(param.Ctx, userGroup)
	if len(autoGroups) == 0 {
		return nil, selectGroup, errors.New("auto groups is not enabled")
	}

	startGroupIndex := 0
	_, hasSelectedGroup := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex)
	if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
		if index, ok := lastGroupIndex.(int); ok {
			startGroupIndex = index
		}
	}
	crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
	for index := startGroupIndex; index < len(autoGroups); index++ {
		if hasSelectedGroup && !crossGroupRetry && index > startGroupIndex {
			break
		}
		autoGroup := autoGroups[index]
		logger.LogDebug(param.Ctx, "Auto selecting group: %s", autoGroup)
		channel, err := model.GetRandomSatisfiedChannel(
			autoGroup,
			param.ModelName,
			0,
			filters,
			param.ExcludedChannels(),
		)
		if err != nil {
			return nil, autoGroup, err
		}
		if channel == nil {
			logger.LogDebug(param.Ctx, "No available channel in group %s for model %s", autoGroup, param.ModelName)
			continue
		}
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, index)
		logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)
		return channel, autoGroup, nil
	}
	return nil, selectGroup, nil
}

func pinnedTaskPluginChannelTypes(c *gin.Context, expected string) []int {
	if c == nil || expected == "" {
		return nil
	}
	if value, exists := c.Get(jsplugin.ContextKeyPinnedEndpoint); exists {
		pinned, ok := value.(jsplugin.PinnedEndpoint)
		if ok && pinned.Generation != nil && len(pinned.Candidates) > 1 {
			expectedFound := false
			channelTypes := make([]int, 0, len(pinned.Candidates))
			seen := make(map[int]struct{}, len(pinned.Candidates))
			for _, candidate := range pinned.Candidates {
				if candidate.Plugin == nil {
					continue
				}
				if candidate.Plugin.Meta.Key == expected {
					expectedFound = true
				}
				for _, channelType := range candidate.Plugin.Meta.ChannelTypes {
					if channelType == 0 || channelType == constant.ChannelTypeTaskPlugin {
						continue
					}
					if _, duplicate := seen[channelType]; duplicate {
						continue
					}
					if plugin, indexed := pinned.Generation.GetByChannelType(channelType); indexed && plugin == candidate.Plugin {
						seen[channelType] = struct{}{}
						channelTypes = append(channelTypes, channelType)
					}
				}
			}
			if expectedFound {
				return channelTypes
			}
		}
	}
	value, exists := c.Get(jsplugin.ContextKeyPinnedPlugin)
	pinned, ok := value.(jsplugin.PinnedPlugin)
	if !exists || !ok || pinned.Generation == nil || pinned.Plugin == nil || pinned.Plugin.Meta.Key != expected {
		return nil
	}
	channelTypes := make([]int, 0, len(pinned.Plugin.Meta.ChannelTypes))
	for _, channelType := range pinned.Plugin.Meta.ChannelTypes {
		if channelType == 0 || channelType == constant.ChannelTypeTaskPlugin {
			continue
		}
		channelTypes = append(channelTypes, channelType)
	}
	if len(channelTypes) == 0 {
		return nil
	}
	return channelTypes
}
