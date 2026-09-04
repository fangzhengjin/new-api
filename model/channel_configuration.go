package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"gorm.io/gorm"
)

var (
	ErrChannelConfigurationConflict = errors.New("channel configuration changed")
	ErrChannelNormalizationApplied  = errors.New("channel normalization task already applied")
	ErrChannelNormalizationNoChange = errors.New("channel normalization selection has no changes")
)

// ChannelNormalizationMutation describes one reviewed models/mapping change.
// SnapshotHash binds it to the previewed channel configuration; the controller
// validates all selected fields before this mutation reaches the model layer.
type ChannelNormalizationMutation struct {
	ChannelID     int
	SnapshotHash  string
	AddModels     []string
	RemoveModels  []string
	MappingSet    map[string]string
	MappingRemove []string
	SortModels    bool
}

type channelConfigurationSnapshot struct {
	Type           int                       `json:"type"`
	Status         int                       `json:"status"`
	BaseURL        string                    `json:"base_url"`
	Key            string                    `json:"key"`
	Models         string                    `json:"models"`
	ModelMapping   string                    `json:"model_mapping"`
	Proxy          string                    `json:"proxy"`
	HeaderOverride string                    `json:"header_override"`
	AdvancedCustom *dto.AdvancedCustomConfig `json:"advanced_custom"`
}

// ChannelConfigurationSnapshot hashes configuration fields read from channel.
// It returns a stable hexadecimal digest or a serialization error. Runtime
// health/polling state and the derived test model are excluded so ordinary relay
// traffic does not invalidate a reviewed preview; persistent configuration
// inputs remain protected by the hash.
func ChannelConfigurationSnapshot(channel *Channel) (string, error) {
	if channel == nil {
		return "", errors.New("channel is nil")
	}

	setting := dto.ChannelSettings{}
	if channel.Setting != nil && strings.TrimSpace(*channel.Setting) != "" {
		if err := common.UnmarshalJsonStr(*channel.Setting, &setting); err != nil {
			return "", fmt.Errorf("invalid channel setting: %w", err)
		}
	}
	otherSettings := dto.ChannelOtherSettings{}
	if strings.TrimSpace(channel.OtherSettings) != "" {
		if err := common.UnmarshalJsonStr(channel.OtherSettings, &otherSettings); err != nil {
			return "", fmt.Errorf("invalid channel settings: %w", err)
		}
	}

	snapshot := channelConfigurationSnapshot{
		Type:           channel.Type,
		Status:         channel.Status,
		BaseURL:        pointerString(channel.BaseURL),
		Key:            channel.Key,
		Models:         channel.Models,
		ModelMapping:   pointerString(channel.ModelMapping),
		Proxy:          setting.Proxy,
		HeaderOverride: pointerString(channel.HeaderOverride),
		AdvancedCustom: otherSettings.AdvancedCustom,
	}
	data, err := common.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// ApplyDiscoveredChannel creates or atomically updates a reviewed channel draft.
// For updates, snapshotHash is checked while the row is locked, then runtime
// state not owned by discovery is merged before fields and abilities are written
// in the same transaction. syncConfiguration controls model/route fields, while
// replaceKeys controls whether key runtime state is reset. It returns the channel ID.
func ApplyDiscoveredChannel(channel *Channel, snapshotHash string, syncConfiguration bool, replaceKeys bool) (int, error) {
	if channel == nil {
		return 0, errors.New("channel is nil")
	}
	created := channel.Id == 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		if created {
			if _, err := channel.ReconcileUserHiddenModelMappings(); err != nil {
				return err
			}
			if err := tx.Create(channel).Error; err != nil {
				return err
			}
			return channel.AddAbilities(tx)
		}

		var current Channel
		if err := lockForUpdate(tx).Where("id = ?", channel.Id).First(&current).Error; err != nil {
			return err
		}
		currentHash, err := ChannelConfigurationSnapshot(&current)
		if err != nil {
			return err
		}
		if currentHash != snapshotHash {
			return ErrChannelConfigurationConflict
		}

		// Key health and polling state can change between preview preparation and
		// this lock. Append keeps that latest state; replace intentionally resets it.
		channel.Keys = nil
		keyCount := len(channel.GetKeys())
		channel.ChannelInfo = current.ChannelInfo
		if replaceKeys {
			channel.ChannelInfo.MultiKeyStatusList = nil
			channel.ChannelInfo.MultiKeyDisabledReason = nil
			channel.ChannelInfo.MultiKeyDisabledTime = nil
			channel.ChannelInfo.MultiKeyPollingIndex = 0
		}
		channel.ChannelInfo.IsMultiKey = keyCount > 1
		channel.ChannelInfo.MultiKeySize = 0
		if channel.ChannelInfo.IsMultiKey {
			channel.ChannelInfo.MultiKeySize = keyCount
			if channel.ChannelInfo.MultiKeyMode == "" {
				channel.ChannelInfo.MultiKeyMode = constant.MultiKeyModeRandom
			}
		} else {
			channel.ChannelInfo.MultiKeyMode = ""
		}
		if channel.ChannelInfo.MultiKeyPollingIndex < 0 || channel.ChannelInfo.MultiKeyPollingIndex >= keyCount {
			channel.ChannelInfo.MultiKeyPollingIndex = 0
		}

		updates := map[string]any{
			"key":          channel.Key,
			"status":       channel.Status,
			"channel_info": channel.ChannelInfo,
		}
		if syncConfiguration {
			channel.Setting = current.Setting
			settingChanged, err := channel.ReconcileUserHiddenModelMappings()
			if err != nil {
				return err
			}
			latestSettings := current.GetOtherSettings()
			desiredSettings := channel.GetOtherSettings()
			latestSettings.AdvancedCustom = desiredSettings.AdvancedCustom
			if channel.Type == constant.ChannelTypeAdvancedCustom {
				latestSettings.UpstreamModelUpdateCheckEnabled = false
				latestSettings.UpstreamModelUpdateAutoSyncEnabled = false
			}
			settingsJSON, err := common.Marshal(latestSettings)
			if err != nil {
				return err
			}
			channel.OtherSettings = string(settingsJSON)
			updates["type"] = channel.Type
			updates["base_url"] = channel.BaseURL
			updates["models"] = channel.Models
			updates["model_mapping"] = channel.ModelMapping
			updates["test_model"] = channel.TestModel
			updates["settings"] = channel.OtherSettings
			if settingChanged {
				updates["setting"] = channel.Setting
			}
		}
		if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Updates(updates).Error; err != nil {
			return err
		}
		if syncConfiguration {
			// Ability metadata is not owned by discovery, so rebuild from the
			// values read under the same row lock rather than the stale preview.
			channel.Group = current.Group
			channel.Priority = current.Priority
			channel.Weight = current.Weight
			channel.Tag = current.Tag
			return channel.UpdateAbilities(tx)
		}
		if channel.Status != current.Status {
			return tx.Model(&Ability{}).Where("channel_id = ?", channel.Id).
				Update("enabled", channel.Status == common.ChannelStatusEnabled).Error
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	InitChannelCache()
	return channel.Id, nil
}

// ApplyChannelNormalizations applies mutations for taskID and replaces the task
// result with updatedTaskResult in one transaction. expectedTaskResult prevents
// replay. It returns the number of changed channels or an error without leaving
// partial channel updates.
func ApplyChannelNormalizations(taskID string, expectedTaskResult string, updatedTaskResult any, mutations []ChannelNormalizationMutation) (int, error) {
	if len(mutations) == 0 {
		return 0, ErrChannelNormalizationNoChange
	}
	resultText, err := marshalSystemTaskJSON(updatedTaskResult)
	if err != nil {
		return 0, err
	}

	sortedMutations := append([]ChannelNormalizationMutation(nil), mutations...)
	// Every transaction locks channels in ID order to avoid cross-request
	// deadlocks while preserving all-or-nothing batch semantics.
	sort.Slice(sortedMutations, func(i, j int) bool {
		return sortedMutations[i].ChannelID < sortedMutations[j].ChannelID
	})
	for index := 1; index < len(sortedMutations); index++ {
		if sortedMutations[index-1].ChannelID == sortedMutations[index].ChannelID {
			return 0, fmt.Errorf("duplicate channel id: %d", sortedMutations[index].ChannelID)
		}
	}

	updated := 0
	err = DB.Transaction(func(tx *gorm.DB) error {
		var task SystemTask
		if err := lockForUpdate(tx).Where("task_id = ? AND type = ? AND status = ?", taskID, SystemTaskTypeChannelNormalize, SystemTaskStatusSucceeded).First(&task).Error; err != nil {
			return err
		}
		if task.Result != expectedTaskResult {
			return ErrChannelNormalizationApplied
		}

		for _, mutation := range sortedMutations {
			var channel Channel
			if err := lockForUpdate(tx).Where("id = ?", mutation.ChannelID).First(&channel).Error; err != nil {
				return err
			}
			snapshotHash, err := ChannelConfigurationSnapshot(&channel)
			if err != nil {
				return err
			}
			if snapshotHash != mutation.SnapshotHash {
				return fmt.Errorf("%w: channel %d", ErrChannelConfigurationConflict, mutation.ChannelID)
			}

			models := applyModelMutation(channel.GetModels(), mutation)
			mapping, err := ParseChannelModelMapping(&channel)
			if err != nil {
				return err
			}
			for _, source := range mutation.MappingRemove {
				delete(mapping, strings.TrimSpace(source))
			}
			for source, target := range mutation.MappingSet {
				source = strings.TrimSpace(source)
				target = strings.TrimSpace(target)
				if source == "" || target == "" {
					return fmt.Errorf("channel %d has an empty mapping", mutation.ChannelID)
				}
				mapping[source] = target
			}

			modelSet := make(map[string]struct{}, len(models))
			for _, modelName := range models {
				modelSet[modelName] = struct{}{}
			}
			for source := range mapping {
				if _, ok := modelSet[source]; !ok {
					return fmt.Errorf("channel %d mapping source %q is not exposed", mutation.ChannelID, source)
				}
			}

			modelsText := strings.Join(models, ",")
			mappingText, err := MarshalChannelModelMapping(mapping)
			if err != nil {
				return err
			}
			if modelsText == channel.Models && mappingText == pointerString(channel.ModelMapping) {
				continue
			}

			channel.Models = modelsText
			channel.ModelMapping = common.GetPointer(mappingText)
			settingChanged, err := channel.ReconcileUserHiddenModelMappings()
			if err != nil {
				return err
			}
			updates := map[string]any{
				"models":        modelsText,
				"model_mapping": mappingText,
			}
			if settingChanged {
				updates["setting"] = channel.Setting
			}
			if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Updates(updates).Error; err != nil {
				return err
			}
			if err := channel.UpdateAbilities(tx); err != nil {
				return err
			}
			updated++
		}

		if updated == 0 {
			return ErrChannelNormalizationNoChange
		}
		return tx.Model(&SystemTask{}).Where("id = ?", task.ID).Updates(map[string]any{
			"result":     resultText,
			"updated_at": common.GetTimestamp(),
		}).Error
	})
	if err != nil {
		return 0, err
	}
	InitChannelCache()
	return updated, nil
}

func applyModelMutation(current []string, mutation ChannelNormalizationMutation) []string {
	removeSet := make(map[string]struct{}, len(mutation.RemoveModels))
	for _, modelName := range mutation.RemoveModels {
		if trimmed := strings.TrimSpace(modelName); trimmed != "" {
			removeSet[trimmed] = struct{}{}
		}
	}
	models := make([]string, 0, len(current)+len(mutation.AddModels))
	seen := make(map[string]struct{}, cap(models))
	for _, modelName := range append(current, mutation.AddModels...) {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, removed := removeSet[modelName]; removed {
			continue
		}
		if _, exists := seen[modelName]; exists {
			continue
		}
		seen[modelName] = struct{}{}
		models = append(models, modelName)
	}
	if mutation.SortModels {
		sort.Strings(models)
	}
	return models
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
