package controller

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

type channelDiscoveryChoice struct {
	Source      string `json:"source"`
	Model       string `json:"model"`
	Target      string `json:"target"`
	Recommended bool   `json:"recommended"`
}

type channelDiscoveryMatch struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type channelDiscoveryRouteSelection map[string]string

var channelDiscoveryRouteDefaults = map[string]string{
	"models":            "/v1/models",
	"responses":         "/v1/responses",
	"compact":           "/v1/responses/compact",
	"chat":              "/v1/chat/completions",
	"messages":          "/v1/messages",
	"image_generations": "/v1/images/generations",
	"image_edits":       "/v1/images/edits",
}

const channelDiscoveryPreferredTestModel = "gpt-5.6-luna"

type channelDiscoveryDraft struct {
	Operation          string                         `json:"operation"`
	BlockIndex         int                            `json:"block_index"`
	ChannelID          int                            `json:"channel_id,omitempty"`
	BaseURL            string                         `json:"base_url"`
	AcceptedKeyIndexes []int                          `json:"accepted_key_indexes"`
	SelectedModels     []string                       `json:"selected_models"`
	Mapping            map[string]string              `json:"mapping"`
	Routes             channelDiscoveryRouteSelection `json:"routes"`
	Name               string                         `json:"name,omitempty"`
	Groups             []string                       `json:"groups,omitempty"`
	Tag                string                         `json:"tag,omitempty"`
	Priority           int64                          `json:"priority,omitempty"`
	Enabled            bool                           `json:"enabled"`
	KeyMode            string                         `json:"key_mode,omitempty"`
	SyncConfiguration  bool                           `json:"sync_configuration"`
	Reenable           bool                           `json:"reenable"`
}

type channelDiscoveryRoutePreview struct {
	Protocol     string `json:"protocol"`
	IncomingPath string `json:"incoming_path"`
	UpstreamPath string `json:"upstream_path"`
}

type channelDiscoveryPreview struct {
	Operation    string                         `json:"operation"`
	Name         string                         `json:"name"`
	TypeName     string                         `json:"type_name"`
	Status       int                            `json:"status"`
	BaseURL      string                         `json:"base_url"`
	Groups       []string                       `json:"groups"`
	Tag          string                         `json:"tag"`
	Priority     int64                          `json:"priority"`
	Models       []string                       `json:"models"`
	Mapping      map[string]string              `json:"mapping"`
	Routes       []channelDiscoveryRoutePreview `json:"routes"`
	KeyCount     int                            `json:"key_count"`
	Changes      []string                       `json:"changes"`
	SnapshotHash string                         `json:"snapshot_hash,omitempty"`
	PreviewHash  string                         `json:"preview_hash"`
}

type channelDiscoveryPlan struct {
	Channel           *model.Channel
	Operation         string
	SnapshotHash      string
	KeyCount          int
	Changes           []string
	Routes            []channelDiscoveryRoutePreview
	SyncConfiguration bool
	ReplaceKeys       bool
}

// buildChannelDiscoveryPlan combines the validated source block, the editable
// draft, and freshly discovered models into one write plan. Updates bind the
// plan to a database snapshot; the returned plan owns only fields this workflow
// is allowed to change.
func buildChannelDiscoveryPlan(block channelDiscoveryBlock, draft channelDiscoveryDraft, fetched channelDiscoveryFetchResult) (channelDiscoveryPlan, error) {
	if draft.Operation != "create" && draft.Operation != "update" {
		return channelDiscoveryPlan{}, errors.New("operation must be create or update")
	}
	baseBlock, err := newChannelDiscoveryBlock(draft.BaseURL)
	if err != nil {
		return channelDiscoveryPlan{}, err
	}
	if baseBlock.Origin != block.Origin {
		return channelDiscoveryPlan{}, errors.New("base URL must keep the connection block origin")
	}
	keys, err := selectedChannelDiscoveryKeys(block, draft.AcceptedKeyIndexes)
	if err != nil {
		return channelDiscoveryPlan{}, err
	}
	keyText := strings.Join(keys, "\n")

	var current *model.Channel
	snapshotHash := ""
	// Resolve updates against the persisted matching channel before copying any
	// draft values, so a stale match cannot redirect credentials to another row.
	if draft.Operation == "update" {
		if draft.ChannelID <= 0 {
			return channelDiscoveryPlan{}, errors.New("channel id is required")
		}
		if draft.KeyMode != "append" && draft.KeyMode != "replace" {
			return channelDiscoveryPlan{}, errors.New("key mode must be append or replace")
		}
		current, err = model.GetChannelById(draft.ChannelID, true)
		if err != nil {
			return channelDiscoveryPlan{}, err
		}
		currentBase, err := newChannelDiscoveryBlock(current.GetBaseURL())
		if err != nil || currentBase.BaseURL != block.BaseURL {
			return channelDiscoveryPlan{}, errors.New("selected channel no longer matches this connection")
		}
		snapshotHash, err = model.ChannelConfigurationSnapshot(current)
		if err != nil {
			return channelDiscoveryPlan{}, err
		}
	}

	channel := &model.Channel{}
	if current != nil {
		*channel = *current
	}
	// Build the desired row from the latest persisted state. Key-only updates
	// deliberately leave model, route, and Base URL configuration untouched.
	if current == nil || draft.SyncConfiguration {
		channel.BaseURL = common.GetPointer(baseBlock.BaseURL)
	}
	channel.Key = keyText
	if current != nil && draft.KeyMode != "replace" {
		channel.Key = appendChannelDiscoveryKeys(current.Key, keys)
	}
	channel.ChannelInfo = buildChannelDiscoveryInfo(current, channel.Key, draft.KeyMode == "replace")
	if current == nil {
		channel.Name = strings.TrimSpace(draft.Name)
		if channel.Name == "" {
			return channelDiscoveryPlan{}, errors.New("channel name is required")
		}
		groups := normalizeModelNames(draft.Groups)
		sort.Strings(groups)
		if len(groups) == 0 {
			return channelDiscoveryPlan{}, errors.New("at least one group is required")
		}
		channel.Group = strings.Join(groups, ",")
		channel.SetTag(strings.TrimSpace(draft.Tag))
		channel.Priority = common.GetPointer(draft.Priority)
		channel.Status = common.ChannelStatusManuallyDisabled
		if draft.Enabled {
			channel.Status = common.ChannelStatusEnabled
		}
		channel.CreatedTime = common.GetTimestamp()
	} else if draft.Reenable {
		channel.Status = common.ChannelStatusEnabled
	}

	routesPreview := []channelDiscoveryRoutePreview{}
	syncConfiguration := current == nil || draft.SyncConfiguration
	// Configuration is derived only after upstream validation, keeping every
	// selected model and mapping target bound to the refreshed model list.
	if syncConfiguration {
		models, mapping, err := buildChannelDiscoveryConfiguration(draft, fetched.Models)
		if err != nil {
			return channelDiscoveryPlan{}, err
		}
		channel.Models = strings.Join(models, ",")
		mappingText, err := marshalDiscoveryMapping(mapping)
		if err != nil {
			return channelDiscoveryPlan{}, err
		}
		channel.ModelMapping = common.GetPointer(mappingText)
		routes := make(channelDiscoveryRouteSelection, len(draft.Routes)+1)
		for protocol, upstreamPath := range draft.Routes {
			routes[protocol] = upstreamPath
		}
		channel.Type = inferChannelDiscoveryType(models, routes)
		routesPreview, err = applyChannelDiscoveryRoutes(channel, routes, fetched.ModelsAuthType)
		if err != nil {
			return channelDiscoveryPlan{}, err
		}
		channel.TestModel = nil
		_, upstreamProvidesTestModel := makeStringSet(fetched.Models)[channelDiscoveryPreferredTestModel]
		// A test model must be both selected and observed upstream; aliases cannot
		// prove that the upstream actually supports the preferred probe model.
		if upstreamProvidesTestModel && slices.Contains(models, channelDiscoveryPreferredTestModel) {
			channel.TestModel = common.GetPointer(channelDiscoveryPreferredTestModel)
		}
	}

	changes := channelDiscoveryChanges(current, channel, draft.KeyMode, syncConfiguration)
	return channelDiscoveryPlan{
		Channel:           channel,
		Operation:         draft.Operation,
		SnapshotHash:      snapshotHash,
		KeyCount:          len(channel.GetKeys()),
		Changes:           changes,
		Routes:            routesPreview,
		SyncConfiguration: syncConfiguration,
		ReplaceKeys:       current != nil && draft.KeyMode == "replace",
	}, nil
}

// buildChannelDiscoveryConfiguration validates selected models and mappings
// against discoveredModels, derives the closed set of managed aliases, and
// returns the exposed model list plus upstream mapping.
func buildChannelDiscoveryConfiguration(draft channelDiscoveryDraft, discoveredModels []string) ([]string, map[string]string, error) {
	hasInferenceRoute := false
	for protocol := range draft.Routes {
		if protocol != "models" {
			hasInferenceRoute = true
			break
		}
	}
	if !hasInferenceRoute {
		return nil, nil, errors.New("at least one inference route is required")
	}
	upstream := makeStringSet(discoveredModels)
	choices := buildChannelDiscoveryChoices(discoveredModels)
	choiceByModel := make(map[string]channelDiscoveryChoice, len(choices))
	for _, choice := range choices {
		choiceByModel[choice.Model] = choice
	}

	models := makeStringSet(draft.SelectedModels)
	if len(models) == 0 {
		return nil, nil, errors.New("at least one model is required")
	}
	mapping := make(map[string]string, len(draft.Mapping)+2)
	// User mappings are accepted only when their source is exposed and their
	// target was observed upstream; this prevents preview-only arbitrary routes.
	for source, target := range draft.Mapping {
		source = strings.TrimSpace(source)
		target = strings.TrimSpace(target)
		if source == "" || target == "" {
			return nil, nil, errors.New("model mapping contains an empty value")
		}
		if _, selected := models[source]; !selected {
			return nil, nil, fmt.Errorf("mapping source %q is not selected", source)
		}
		if _, exists := upstream[target]; !exists {
			return nil, nil, fmt.Errorf("mapping target %q was not discovered upstream", target)
		}
		mapping[source] = target
	}
	for modelName := range models {
		if _, exists := upstream[modelName]; exists {
			continue
		}
		if choice, exists := choiceByModel[modelName]; exists && choice.Recommended {
			if choice.Target != modelName {
				mapping[modelName] = choice.Target
			}
			continue
		}
		if _, mapped := mapping[modelName]; !mapped {
			return nil, nil, fmt.Errorf("model %q was not discovered upstream", modelName)
		}
	}

	if _, compact := draft.Routes["compact"]; compact {
		if _, responses := draft.Routes["responses"]; !responses {
			return nil, nil, errors.New("compact requires the responses route")
		}
		if !hasChannelDiscoveryCompactModel(models) {
			return nil, nil, errors.New("compact requires a compatible GPT model")
		}
	}
	for source := range mapping {
		models[source] = struct{}{}
	}
	return sortedSetValues(models), mapping, nil
}

// buildChannelDiscoveryChoices converts upstream IDs into exposed candidates.
// It recommends only the feature-owned model surface and preserves every raw ID
// as the mapping target when an alias is required.
func buildChannelDiscoveryChoices(models []string) []channelDiscoveryChoice {
	upstream := makeStringSet(models)
	choices := make([]channelDiscoveryChoice, 0, len(models))
	for _, source := range sortedSetValues(upstream) {
		if _, compact := channelNormalizationLegacyCompactModels[source]; compact || channelNormalizationThinkingBase(source) != "" {
			continue
		}
		modelName := source
		target := source
		recommended := isChannelNormalizationAllowedOpenAIModel(source)
		if strings.HasPrefix(source, "claude-") {
			var ok bool
			modelName, target, ok = exposedClaudeModel(source, upstream)
			recommended = recommended && ok
			if !ok {
				modelName = strings.ReplaceAll(source, ".", "-")
				target = source
			}
		}
		choice := channelDiscoveryChoice{
			Source:      source,
			Model:       modelName,
			Target:      target,
			Recommended: recommended,
		}
		choices = append(choices, choice)
	}
	return choices
}

func selectedChannelDiscoveryKeys(block channelDiscoveryBlock, indexes []int) ([]string, error) {
	if len(indexes) == 0 {
		return nil, errors.New("at least one usable key is required")
	}
	keys := make([]string, 0, len(indexes))
	seen := map[int]struct{}{}
	for _, index := range indexes {
		if index < 0 || index >= len(block.Keys) {
			return nil, errors.New("usable key selection is invalid")
		}
		if _, duplicate := seen[index]; duplicate {
			continue
		}
		seen[index] = struct{}{}
		keys = append(keys, block.Keys[index])
	}
	return keys, nil
}

func appendChannelDiscoveryKeys(current string, appended []string) string {
	keys := normalizeModelNames(strings.Split(current, "\n"))
	seen := makeStringSet(keys)
	for _, key := range appended {
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return strings.Join(keys, "\n")
}

func buildChannelDiscoveryInfo(current *model.Channel, keyText string, reset bool) model.ChannelInfo {
	keys := normalizeModelNames(strings.Split(keyText, "\n"))
	info := model.ChannelInfo{}
	if current != nil && !reset {
		info = current.ChannelInfo
	}
	info.IsMultiKey = len(keys) > 1
	info.MultiKeySize = 0
	if info.IsMultiKey {
		info.MultiKeySize = len(keys)
		if info.MultiKeyMode == "" {
			info.MultiKeyMode = constant.MultiKeyModeRandom
		}
	} else {
		info.MultiKeyMode = ""
	}
	if reset {
		info.MultiKeyStatusList = nil
		info.MultiKeyDisabledReason = nil
		info.MultiKeyDisabledTime = nil
		info.MultiKeyPollingIndex = 0
	}
	if info.MultiKeyPollingIndex >= len(keys) {
		info.MultiKeyPollingIndex = 0
	}
	return info
}

func inferChannelDiscoveryType(models []string, routes channelDiscoveryRouteSelection) int {
	// Routes, rather than model-name prefixes, determine the channel type because
	// one advanced upstream may expose several inference protocols together.
	for protocol, upstreamPath := range routes {
		if strings.TrimSpace(upstreamPath) != channelDiscoveryRouteDefaults[protocol] {
			return constant.ChannelTypeAdvancedCustom
		}
	}
	_, messages := routes["messages"]
	_, responses := routes["responses"]
	_, chat := routes["chat"]
	_, imageGenerations := routes["image_generations"]
	_, imageEdits := routes["image_edits"]
	if messages && (responses || chat || imageGenerations || imageEdits) {
		return constant.ChannelTypeAdvancedCustom
	}
	if messages {
		return constant.ChannelTypeAnthropic
	}
	allGrok := len(models) > 0
	for _, modelName := range models {
		if !strings.HasPrefix(modelName, "grok-") {
			allGrok = false
			break
		}
	}
	if allGrok {
		return constant.ChannelTypeXai
	}
	return constant.ChannelTypeOpenAI
}

// applyChannelDiscoveryRoutes validates selected paths and writes the minimal
// Advanced Custom routing settings when native channel types cannot represent
// them. It returns the normalized routes shown in the preview.
func applyChannelDiscoveryRoutes(channel *model.Channel, selected channelDiscoveryRouteSelection, modelsAuthType string) ([]channelDiscoveryRoutePreview, error) {
	previews := make([]channelDiscoveryRoutePreview, 0, len(selected))
	for protocol, upstreamPath := range selected {
		incomingPath, exists := channelDiscoveryRouteDefaults[protocol]
		if !exists {
			return nil, fmt.Errorf("unsupported route protocol %q", protocol)
		}
		upstreamPath = strings.TrimSpace(upstreamPath)
		if upstreamPath == "" {
			return nil, fmt.Errorf("route %s has an invalid upstream path", protocol)
		}
		if !strings.HasPrefix(upstreamPath, "/") {
			target, err := url.Parse(upstreamPath)
			base, _ := url.Parse(channel.GetBaseURL())
			if err != nil || target.User != nil || target.Scheme == "" || target.Host == "" ||
				!strings.EqualFold(target.Scheme, base.Scheme) || !strings.EqualFold(target.Host, base.Host) {
				return nil, fmt.Errorf("route %s must keep the channel origin", protocol)
			}
		}
		previews = append(previews, channelDiscoveryRoutePreview{Protocol: protocol, IncomingPath: incomingPath, UpstreamPath: upstreamPath})
	}
	sort.Slice(previews, func(i, j int) bool { return previews[i].IncomingPath < previews[j].IncomingPath })

	settings := dto.ChannelOtherSettings{}
	if strings.TrimSpace(channel.OtherSettings) != "" {
		if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
			return nil, err
		}
	}
	settings.AdvancedCustom = nil
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		routes := make([]dto.AdvancedCustomRoute, 0, len(previews))
		for _, preview := range previews {
			auth := &dto.AdvancedCustomRouteAuth{Type: dto.AdvancedCustomAuthTypeHeader, Name: "Authorization", Value: "Bearer {api_key}"}
			if preview.Protocol == "messages" || (preview.Protocol == "models" && modelsAuthType == "anthropic") {
				auth = &dto.AdvancedCustomRouteAuth{Type: dto.AdvancedCustomAuthTypeHeader, Name: "x-api-key", Value: "{api_key}"}
			}
			routes = append(routes, dto.AdvancedCustomRoute{IncomingPath: preview.IncomingPath, UpstreamPath: preview.UpstreamPath, Converter: "none", Auth: auth})
		}
		settings.AdvancedCustom = &dto.AdvancedCustomConfig{Routes: routes}
		// Existing upstream-model sync assumes one provider route and must not
		// rewrite a mixed/custom routing configuration behind this preview.
		settings.UpstreamModelUpdateCheckEnabled = false
		settings.UpstreamModelUpdateAutoSyncEnabled = false
	}
	channel.SetOtherSettings(settings)
	return previews, nil
}

func channelDiscoveryChanges(current *model.Channel, next *model.Channel, keyMode string, syncConfiguration bool) []string {
	if current == nil {
		return []string{"create", "keys", "models", "mapping", "routes"}
	}
	changes := make([]string, 0)
	if keyMode == "replace" || next.Key != current.Key {
		changes = append(changes, "keys")
	}
	if next.Status != current.Status {
		changes = append(changes, "status")
	}
	if syncConfiguration {
		if next.Type != current.Type {
			changes = append(changes, "type")
		}
		if next.GetBaseURL() != current.GetBaseURL() {
			changes = append(changes, "base_url")
		}
		if next.Models != current.Models {
			changes = append(changes, "models")
		}
		if next.GetModelMapping() != current.GetModelMapping() {
			changes = append(changes, "mapping")
		}
		if next.OtherSettings != current.OtherSettings {
			changes = append(changes, "routes")
		}
	}
	return changes
}

// channelDiscoveryPreviewFromPlan renders plan and hashes every field that can
// affect the eventual write. It returns the immutable preview used to reject
// edited or stale apply requests.
func channelDiscoveryPreviewFromPlan(plan channelDiscoveryPlan) (channelDiscoveryPreview, error) {
	channel := plan.Channel
	mapping, err := parseStrictChannelMapping(channel)
	if err != nil {
		return channelDiscoveryPreview{}, err
	}
	settings := dto.ChannelOtherSettings{}
	if strings.TrimSpace(channel.OtherSettings) != "" {
		if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
			return channelDiscoveryPreview{}, err
		}
	}
	digest := struct {
		Operation         string                    `json:"operation"`
		SnapshotHash      string                    `json:"snapshot_hash"`
		ID                int                       `json:"id"`
		Type              int                       `json:"type"`
		Key               string                    `json:"key"`
		Status            int                       `json:"status"`
		Name              string                    `json:"name"`
		BaseURL           string                    `json:"base_url"`
		Models            string                    `json:"models"`
		Group             string                    `json:"group"`
		ModelMapping      string                    `json:"model_mapping"`
		Priority          int64                     `json:"priority"`
		Tag               string                    `json:"tag"`
		AdvancedCustom    *dto.AdvancedCustomConfig `json:"advanced_custom"`
		TestModel         *string                   `json:"test_model"`
		SyncConfiguration bool                      `json:"sync_configuration"`
		ReplaceKeys       bool                      `json:"replace_keys"`
	}{
		Operation:         plan.Operation,
		SnapshotHash:      plan.SnapshotHash,
		ID:                channel.Id,
		Type:              channel.Type,
		Key:               channel.Key,
		Status:            channel.Status,
		Name:              channel.Name,
		BaseURL:           channel.GetBaseURL(),
		Models:            channel.Models,
		Group:             channel.Group,
		ModelMapping:      channel.GetModelMapping(),
		Tag:               channel.GetTag(),
		AdvancedCustom:    settings.AdvancedCustom,
		TestModel:         channel.TestModel,
		SyncConfiguration: plan.SyncConfiguration,
		ReplaceKeys:       plan.ReplaceKeys,
	}
	if channel.Priority != nil {
		digest.Priority = *channel.Priority
	}
	data, err := common.Marshal(digest)
	if err != nil {
		return channelDiscoveryPreview{}, err
	}
	priority := int64(0)
	if channel.Priority != nil {
		priority = *channel.Priority
	}
	return channelDiscoveryPreview{
		Operation:    plan.Operation,
		Name:         channel.Name,
		TypeName:     constant.GetChannelTypeName(channel.Type),
		Status:       channel.Status,
		BaseURL:      channel.GetBaseURL(),
		Groups:       channel.GetGroups(),
		Tag:          channel.GetTag(),
		Priority:     priority,
		Models:       normalizeModelNames(channel.GetModels()),
		Mapping:      mapping,
		Routes:       plan.Routes,
		KeyCount:     plan.KeyCount,
		Changes:      plan.Changes,
		SnapshotHash: plan.SnapshotHash,
		PreviewHash:  fmt.Sprintf("%x", sha256.Sum256(data)),
	}, nil
}

func marshalDiscoveryMapping(mapping map[string]string) (string, error) {
	if len(mapping) == 0 {
		return "", nil
	}
	data, err := common.Marshal(mapping)
	return string(data), err
}

func hasChannelDiscoveryCompactModel(models map[string]struct{}) bool {
	for _, base := range channelNormalizationLegacyCompactModels {
		if _, exists := models[base]; exists {
			return true
		}
	}
	return false
}

func isChannelDiscoveryImageModel(modelName string) bool {
	return strings.HasPrefix(modelName, "gpt-image-") ||
		strings.HasPrefix(modelName, "imagen-") ||
		strings.HasPrefix(modelName, "dall-e-")
}
