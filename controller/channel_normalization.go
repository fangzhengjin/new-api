package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// MySQL TEXT is limited to 64 KiB. Keeping only compact diffs below 60 KiB
// leaves serialization headroom without changing the shared task schema.
const (
	channelNormalizationResultMaxBytes = 60 * 1024
	channelNormalizationFetchTimeout   = 30 * time.Second
)

type channelNormalizationPayload struct {
	IncludeDisabled bool `json:"include_disabled"`
}

type channelNormalizationState struct {
	Processed        int `json:"processed"`
	Total            int `json:"total"`
	CurrentChannelID int `json:"current_channel_id,omitempty"`
}

type channelNormalizationSummary struct {
	Scanned int `json:"scanned"`
	Changed int `json:"changed"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

type channelNormalizationItem struct {
	ChannelID       int               `json:"channel_id"`
	SnapshotHash    string            `json:"snapshot_hash"`
	AddModels       []string          `json:"add_models,omitempty"`
	RemoveModels    []string          `json:"remove_models,omitempty"`
	MappingSet      map[string]string `json:"mapping_set,omitempty"`
	MappingRemove   []string          `json:"mapping_remove,omitempty"`
	MappingWarnings map[string]string `json:"mapping_warnings,omitempty"`
	SortChanged     bool              `json:"sort_changed,omitempty"`
}

type channelNormalizationFailure struct {
	ChannelID    int    `json:"channel_id"`
	ErrorMessage string `json:"error_message"`
}

type channelNormalizationResult struct {
	Summary   channelNormalizationSummary   `json:"summary"`
	Items     []channelNormalizationItem    `json:"items,omitempty"`
	Failures  []channelNormalizationFailure `json:"failures,omitempty"`
	AppliedAt int64                         `json:"applied_at,omitempty"`
}

type channelNormalizationHandler struct{}

// Type returns the persistent system-task type handled by Run.
func (channelNormalizationHandler) Type() string {
	return model.SystemTaskTypeChannelNormalize
}

// Run scans supported channels for task while ctx and runnerID keep the work
// cancellable and lease-owned. It persists compact preview diffs or records a
// terminal failure; results are returned through the system-task row.
func (channelNormalizationHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelNormalizationPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}

	var allChannels []*model.Channel
	if err := model.DB.Select(channelUpstreamModelUpdateSelectFields).Order("id asc").Find(&allChannels).Error; err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	channels := make([]*model.Channel, 0, len(allChannels))
	for _, channel := range allChannels {
		if isChannelNormalizationTarget(channel) {
			channels = append(channels, channel)
		}
	}

	state := channelNormalizationState{Total: len(channels)}
	if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
		common.SysLog(fmt.Sprintf("channel normalization task %s failed to save state: %v", task.TaskID, err))
		return
	}
	result := channelNormalizationResult{}

	for _, channel := range channels {
		select {
		case <-ctx.Done():
			finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, ctx.Err())
			return
		default:
		}

		state.CurrentChannelID = channel.Id
		upstreamModels := normalizeModelNames(channel.GetModels())
		upstreamAccessible := false
		var fetchFailure *channelNormalizationFailure
		if channel.Status != common.ChannelStatusEnabled && !payload.IncludeDisabled {
			result.Summary.Skipped++
		} else {
			models, err := fetchChannelNormalizationModels(ctx, channel)
			if err != nil {
				common.SysLog(fmt.Sprintf("channel normalization fetch failed: channel_id=%d name=%s err=%v", channel.Id, channel.Name, err))
				failure := newChannelNormalizationFailure(channel.Id, err)
				fetchFailure = &failure
			} else {
				upstreamModels = models
				upstreamAccessible = true
			}
		}

		// A skipped or failed fetch does not abort deterministic cleanup based on
		// stored models; destructive fallback changes stay unselected in the UI.
		candidate, err := buildChannelNormalizationCandidate(channel, upstreamModels, upstreamAccessible)
		if err != nil {
			common.SysLog(fmt.Sprintf("channel normalization failed: channel_id=%d name=%s err=%v", channel.Id, channel.Name, err))
			result.Failures = append(result.Failures, newChannelNormalizationFailure(channel.Id, err))
		} else {
			if fetchFailure != nil {
				result.Failures = append(result.Failures, *fetchFailure)
			}
			item, changed, err := buildChannelNormalizationItem(channel, candidate)
			if err != nil {
				result.Failures = append(result.Failures, newChannelNormalizationFailure(channel.Id, err))
			} else if changed {
				result.Items = append(result.Items, item)
			}
		}

		state.Processed++
		if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
			common.SysLog(fmt.Sprintf("channel normalization task %s failed to save state: %v", task.TaskID, err))
			return
		}
	}

	state.CurrentChannelID = 0
	result.Summary.Scanned = len(channels)
	result.Summary.Changed = len(result.Items)
	result.Summary.Failed = len(result.Failures)
	encoded, err := common.Marshal(result)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	if len(encoded) > channelNormalizationResultMaxBytes {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, fmt.Errorf("channel normalization preview exceeds %d KiB", channelNormalizationResultMaxBytes/1024))
		return
	}
	if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
		common.SysLog(fmt.Sprintf("channel normalization task %s failed to save final state: %v", task.TaskID, err))
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, result, nil)
}

// fetchChannelNormalizationModels fetches one channel under a per-channel child
// deadline. Advanced Custom uses its configured model-list route when present,
// otherwise it falls back to bounded discovery. It returns normalized model IDs.
func fetchChannelNormalizationModels(ctx context.Context, channel *model.Channel) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, channelNormalizationFetchTimeout)
	defer cancel()
	if channel.Type != constant.ChannelTypeAdvancedCustom {
		return fetchChannelUpstreamModelIDsWithContext(ctx, channelDiscoveryMaxResponseBytes, channel)
	}

	settings := dto.ChannelOtherSettings{}
	if strings.TrimSpace(channel.OtherSettings) != "" {
		if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
			return nil, fmt.Errorf("channel %d has invalid settings: %w", channel.Id, err)
		}
	}
	if settings.AdvancedCustom != nil {
		if _, exists := settings.AdvancedCustom.ModelListRoute(); exists {
			return fetchChannelUpstreamModelIDsWithContext(ctx, channelDiscoveryMaxResponseBytes, channel)
		}
	}

	block, err := newChannelDiscoveryBlock(channel.GetBaseURL())
	if err != nil {
		return nil, err
	}
	key, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return nil, fmt.Errorf("failed to get channel key: %w", apiErr)
	}
	block.Keys = []string{strings.TrimSpace(key)}
	result := discoverChannelBlock(ctx, block)
	if result.Error != nil {
		return nil, result.Error
	}
	return result.Models, nil
}

// buildChannelNormalizationItem diffs candidate against the persisted channel.
// It returns the selectable preview item, whether any change exists, and any
// mapping or snapshot error that makes the channel unsafe to preview.
func buildChannelNormalizationItem(channel *model.Channel, candidate channelNormalizationCandidate) (channelNormalizationItem, bool, error) {
	currentModels := normalizeModelNames(channel.GetModels())
	currentModelSet := makeStringSet(currentModels)
	candidateModelSet := makeStringSet(candidate.Models)
	addModels := setDifference(candidateModelSet, currentModelSet)
	removeModels := setDifference(currentModelSet, candidateModelSet)

	currentMapping, err := parseStrictChannelMapping(channel)
	if err != nil {
		return channelNormalizationItem{}, false, err
	}
	mappingSet := map[string]string{}
	for source, target := range candidate.Mapping {
		if currentMapping[source] != target {
			mappingSet[source] = target
		}
	}
	mappingRemove := make([]string, 0)
	for source := range currentMapping {
		if _, exists := candidate.Mapping[source]; !exists {
			mappingRemove = append(mappingRemove, source)
		}
	}
	sort.Strings(mappingRemove)

	sortedCurrent := append([]string(nil), currentModels...)
	sort.Strings(sortedCurrent)
	sortChanged := !slices.Equal(currentModels, sortedCurrent) || strings.Join(currentModels, ",") != channel.Models
	changed := len(addModels) > 0 || len(removeModels) > 0 || len(mappingSet) > 0 || len(mappingRemove) > 0 || sortChanged
	if !changed {
		return channelNormalizationItem{}, false, nil
	}
	snapshotHash, err := model.ChannelConfigurationSnapshot(channel)
	if err != nil {
		return channelNormalizationItem{}, false, err
	}
	return channelNormalizationItem{
		ChannelID:       channel.Id,
		SnapshotHash:    snapshotHash,
		AddModels:       addModels,
		RemoveModels:    removeModels,
		MappingSet:      mappingSet,
		MappingRemove:   mappingRemove,
		MappingWarnings: candidate.MappingWarnings,
		SortChanged:     sortChanged,
	}, true, nil
}

// StartChannelNormalization reads scan options from c, enqueues the single
// deduplicated background scan, and writes its task response to c. The handler
// returns no Go value.
func StartChannelNormalization(c *gin.Context) {
	payload := channelNormalizationPayload{}
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	task, _, err := service.EnqueueSystemTask(model.SystemTaskTypeChannelNormalize, payload)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondChannelNormalizationTask(c, task)
}

// GetCurrentChannelNormalization reads no request body from c and writes the
// active or most recent normalization task plus current channel metadata. The
// handler returns no Go value.
func GetCurrentChannelNormalization(c *gin.Context) {
	task, err := model.GetActiveSystemTask(model.SystemTaskTypeChannelNormalize)
	if err == nil && task == nil {
		task, err = model.GetLatestSystemTask(model.SystemTaskTypeChannelNormalize)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondChannelNormalizationTask(c, task)
}

type applyChannelNormalizationRequest struct {
	TaskID   string                               `json:"task_id"`
	Channels []applyChannelNormalizationSelection `json:"channels"`
}

type applyChannelNormalizationSelection struct {
	ChannelID     int               `json:"channel_id"`
	AddModels     []string          `json:"add_models"`
	RemoveModels  []string          `json:"remove_models"`
	MappingSet    map[string]string `json:"mapping_set"`
	MappingRemove []string          `json:"mapping_remove"`
	SortModels    bool              `json:"sort_models"`
}

// ApplyChannelNormalization reads reviewed subitems from c, validates them
// against the completed task preview, and atomically applies all selected
// channels. The updated count is written to c; the handler returns no Go value.
func ApplyChannelNormalization(c *gin.Context) {
	req := applyChannelNormalizationRequest{}
	if err := c.ShouldBindJSON(&req); err != nil || req.TaskID == "" || len(req.Channels) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	task, err := model.GetSystemTaskByTaskID(req.TaskID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if task == nil || task.Type != model.SystemTaskTypeChannelNormalize || task.Status != model.SystemTaskStatusSucceeded {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "normalization task is not ready"})
		return
	}
	result := channelNormalizationResult{}
	if err := common.UnmarshalJsonStr(task.Result, &result); err != nil {
		common.ApiError(c, err)
		return
	}
	if result.AppliedAt != 0 {
		// AppliedAt is persisted in the same transaction as channel mutations, so
		// retries cannot apply a completed preview twice.
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "normalization task was already applied"})
		return
	}

	candidates := make(map[int]channelNormalizationItem, len(result.Items))
	for _, item := range result.Items {
		candidates[item.ChannelID] = item
	}
	mutations := make([]model.ChannelNormalizationMutation, 0, len(req.Channels))
	seen := map[int]struct{}{}
	for _, selected := range req.Channels {
		candidate, exists := candidates[selected.ChannelID]
		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("channel %d is not in this preview", selected.ChannelID)})
			return
		}
		if _, duplicate := seen[selected.ChannelID]; duplicate {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("duplicate channel %d", selected.ChannelID)})
			return
		}
		seen[selected.ChannelID] = struct{}{}
		mutation, err := validateChannelNormalizationSelection(candidate, selected)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
			return
		}
		mutations = append(mutations, mutation)
	}

	result.AppliedAt = common.GetTimestamp()
	updated, err := model.ApplyChannelNormalizations(task.TaskID, task.Result, result, mutations)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrChannelConfigurationConflict) || errors.Is(err, model.ErrChannelNormalizationApplied) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "channel.normalize", map[string]any{"task_id": task.TaskID, "updated_channels": updated})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"updated": updated}})
}

type channelNormalizationMetadata struct {
	Name         string            `json:"name"`
	Models       []string          `json:"models"`
	ModelMapping map[string]string `json:"model_mapping"`
}

func respondChannelNormalizationTask(c *gin.Context, task *model.SystemTask) {
	if task == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": nil})
		return
	}
	metadata, err := getChannelNormalizationMetadata(task)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{
		"task":     task.ToResponse(),
		"channels": metadata,
	}})
}

// getChannelNormalizationMetadata loads display-only channel fields referenced
// by task. It returns metadata keyed by channel ID without expanding the durable
// task result stored in the cross-database TEXT column.
func getChannelNormalizationMetadata(task *model.SystemTask) (map[int]channelNormalizationMetadata, error) {
	// Task results store diffs only. Current names/models are loaded on demand so
	// the durable result remains safely below the cross-database TEXT limit.
	metadata := map[int]channelNormalizationMetadata{}
	if task == nil || strings.TrimSpace(task.Result) == "" {
		return metadata, nil
	}
	result := channelNormalizationResult{}
	if err := common.UnmarshalJsonStr(task.Result, &result); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(result.Items)+len(result.Failures))
	seen := map[int]struct{}{}
	for _, item := range result.Items {
		if _, exists := seen[item.ChannelID]; !exists {
			seen[item.ChannelID] = struct{}{}
			ids = append(ids, item.ChannelID)
		}
	}
	for _, failure := range result.Failures {
		if _, exists := seen[failure.ChannelID]; !exists {
			seen[failure.ChannelID] = struct{}{}
			ids = append(ids, failure.ChannelID)
		}
	}
	if len(ids) == 0 {
		return metadata, nil
	}
	var channels []*model.Channel
	if err := model.DB.Select("id", "name", "models", "model_mapping").Where("id IN ?", ids).Find(&channels).Error; err != nil {
		return nil, err
	}
	for _, channel := range channels {
		mapping, err := parseStrictChannelMapping(channel)
		if err != nil {
			mapping = map[string]string{}
		}
		metadata[channel.Id] = channelNormalizationMetadata{
			Name:         channel.Name,
			Models:       normalizeModelNames(channel.GetModels()),
			ModelMapping: mapping,
		}
	}
	return metadata, nil
}

// validateChannelNormalizationSelection treats candidate as an allowlist for
// selected, enforces model/mapping dependencies and deterministic sorting, then
// returns the exact mutation safe for the transactional model layer.
func validateChannelNormalizationSelection(candidate channelNormalizationItem, selected applyChannelNormalizationSelection) (model.ChannelNormalizationMutation, error) {
	// First reject every model or mapping-removal value that was not present in
	// the immutable preview; later checks only need to validate dependencies.
	addModels, err := validateSelectedStrings(selected.AddModels, candidate.AddModels, "add model")
	if err != nil {
		return model.ChannelNormalizationMutation{}, fmt.Errorf("channel %d: %w", selected.ChannelID, err)
	}
	removeModels, err := validateSelectedStrings(selected.RemoveModels, candidate.RemoveModels, "remove model")
	if err != nil {
		return model.ChannelNormalizationMutation{}, fmt.Errorf("channel %d: %w", selected.ChannelID, err)
	}
	mappingRemove, err := validateSelectedStrings(selected.MappingRemove, candidate.MappingRemove, "remove mapping")
	if err != nil {
		return model.ChannelNormalizationMutation{}, fmt.Errorf("channel %d: %w", selected.ChannelID, err)
	}
	mappingSet := make(map[string]string, len(selected.MappingSet))
	for source, target := range selected.MappingSet {
		source = strings.TrimSpace(source)
		target = strings.TrimSpace(target)
		if _, exists := candidate.MappingSet[source]; !exists {
			return model.ChannelNormalizationMutation{}, fmt.Errorf("channel %d: mapping %q is not in this preview", selected.ChannelID, source)
		}
		if target == "" {
			return model.ChannelNormalizationMutation{}, fmt.Errorf("channel %d: mapping %q target is empty", selected.ChannelID, source)
		}
		mappingSet[source] = target
	}
	addModelSet := makeStringSet(addModels)
	candidateAddModelSet := makeStringSet(candidate.AddModels)
	// A new alias and its mapping form one usable unit: neither side can be
	// applied without the other.
	for source := range mappingSet {
		if _, requiresAdd := candidateAddModelSet[source]; requiresAdd {
			if _, selectedAdd := addModelSet[source]; !selectedAdd {
				return model.ChannelNormalizationMutation{}, fmt.Errorf("channel %d: mapping %q requires adding its model", selected.ChannelID, source)
			}
		}
	}
	for modelName := range addModelSet {
		if _, requiresMapping := candidate.MappingSet[modelName]; requiresMapping {
			if _, selectedMapping := mappingSet[modelName]; !selectedMapping {
				return model.ChannelNormalizationMutation{}, fmt.Errorf("channel %d: model %q requires its mapping", selected.ChannelID, modelName)
			}
		}
	}
	removeMappingSet := makeStringSet(mappingRemove)
	candidateMappingRemoveSet := makeStringSet(candidate.MappingRemove)
	// Removing an exposed alias also removes its now-orphaned mapping; keeping
	// either side would leave an inconsistent channel configuration.
	for _, modelName := range removeModels {
		if _, requiresMappingRemoval := candidateMappingRemoveSet[modelName]; requiresMappingRemoval {
			if _, selectedRemoval := removeMappingSet[modelName]; !selectedRemoval {
				return model.ChannelNormalizationMutation{}, fmt.Errorf("channel %d: model %q requires removing its mapping", selected.ChannelID, modelName)
			}
		}
	}
	for source := range mappingSet {
		if _, removed := removeMappingSet[source]; removed {
			return model.ChannelNormalizationMutation{}, fmt.Errorf("channel %d: mapping %q cannot be set and removed", selected.ChannelID, source)
		}
	}
	// Any selected model-set change must produce a deterministic final order.
	// Enforce it server-side rather than trusting the UI to send sort_models.
	modelChangesSelected := len(addModels) > 0 || len(removeModels) > 0
	sortModels := selected.SortModels || modelChangesSelected
	if sortModels && !candidate.SortChanged && !modelChangesSelected {
		return model.ChannelNormalizationMutation{}, fmt.Errorf("channel %d: sorting is not in this preview", selected.ChannelID)
	}
	if len(addModels) == 0 && len(removeModels) == 0 && len(mappingSet) == 0 && len(mappingRemove) == 0 && !sortModels {
		return model.ChannelNormalizationMutation{}, fmt.Errorf("channel %d: no changes selected", selected.ChannelID)
	}
	return model.ChannelNormalizationMutation{
		ChannelID:     selected.ChannelID,
		SnapshotHash:  candidate.SnapshotHash,
		AddModels:     addModels,
		RemoveModels:  removeModels,
		MappingSet:    mappingSet,
		MappingRemove: mappingRemove,
		SortModels:    sortModels,
	}, nil
}

func validateSelectedStrings(selected []string, candidates []string, label string) ([]string, error) {
	allowed := makeStringSet(candidates)
	result := make([]string, 0, len(selected))
	seen := map[string]struct{}{}
	for _, value := range selected {
		value = strings.TrimSpace(value)
		if _, exists := allowed[value]; !exists {
			return nil, fmt.Errorf("%s %q is not in this preview", label, value)
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func setDifference(left map[string]struct{}, right map[string]struct{}) []string {
	result := make([]string, 0)
	for value := range left {
		if _, exists := right[value]; !exists {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func newChannelNormalizationFailure(channelID int, err error) channelNormalizationFailure {
	message := []rune(err.Error())
	if len(message) > 512 {
		message = append(message[:511], '…')
	}
	return channelNormalizationFailure{
		ChannelID:    channelID,
		ErrorMessage: string(message),
	}
}
