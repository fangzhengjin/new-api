package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type channelDiscoveryRequest struct {
	Text string `json:"text"`
}

type channelDiscoveryConnection struct {
	BlockIndex       int                      `json:"block_index"`
	BaseURL          string                   `json:"base_url"`
	SuggestedName    string                   `json:"suggested_name"`
	Usable           bool                     `json:"usable"`
	Models           []string                 `json:"models"`
	Choices          []channelDiscoveryChoice `json:"choices"`
	UsableKeyIndexes []int                    `json:"usable_key_indexes"`
	RejectedKeyCount int                      `json:"rejected_key_count"`
	Matches          []channelDiscoveryMatch  `json:"matches"`
	ModelsPath       string                   `json:"models_path,omitempty"`
	ErrorMessage     string                   `json:"error_message,omitempty"`
}

// DiscoverChannels reads pasted connection text from c, validates every URL/key
// block against its upstream, and writes redacted configuration candidates to
// the Gin response. The handler returns no Go value.
func DiscoverChannels(c *gin.Context) {
	req := channelDiscoveryRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	blocks, err := parseChannelDiscoveryBlocks(req.Text)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	rows, err := channelDiscoveryMatchRows()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), channelDiscoveryOperationTimeout)
	defer cancel()
	connections := make([]channelDiscoveryConnection, 0, len(blocks))
	for index, block := range blocks {
		fetched := discoverChannelBlock(ctx, block)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			c.JSON(http.StatusGatewayTimeout, gin.H{"success": false, "message": "channel discovery timed out"})
			return
		}
		connection := channelDiscoveryConnection{
			BlockIndex:       index,
			BaseURL:          block.BaseURL,
			SuggestedName:    channelDiscoverySuggestedName(block),
			Usable:           len(fetched.UsableKeyIndexes) > 0,
			Models:           fetched.Models,
			Choices:          buildChannelDiscoveryChoices(fetched.Models),
			UsableKeyIndexes: fetched.UsableKeyIndexes,
			RejectedKeyCount: len(fetched.RejectedKeyIndexes),
			Matches:          matchingChannelDiscoveryRows(block, rows),
			ModelsPath:       fetched.ModelsPath,
		}
		if fetched.Error != nil {
			connection.ErrorMessage = truncateChannelDiscoveryMessage(fetched.Error.Error())
		}
		connections = append(connections, connection)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": connections})
}

type channelDiscoveryProbeRequest struct {
	Text       string `json:"text"`
	BlockIndex int    `json:"block_index"`
	KeyIndex   int    `json:"key_index"`
	BaseURL    string `json:"base_url"`
	Model      string `json:"model"`
	ModelsPath string `json:"models_path"`
}

// ProbeChannelDiscovery reads one discovered key/model selection from c, tests
// its supported inference protocols, and writes the successful route paths to
// the Gin response. The handler returns no Go value.
func ProbeChannelDiscovery(c *gin.Context) {
	req := channelDiscoveryProbeRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	blocks, err := parseChannelDiscoveryBlocks(req.Text)
	if err != nil || req.BlockIndex < 0 || req.BlockIndex >= len(blocks) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "connection block is invalid"})
		return
	}
	sourceBlock := blocks[req.BlockIndex]
	block, err := newChannelDiscoveryBlock(req.BaseURL)
	if err != nil || block.Origin != sourceBlock.Origin {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "probe Base URL must keep the connection block origin"})
		return
	}
	block.Keys = sourceBlock.Keys
	block.ModelsPath = req.ModelsPath
	modelName := strings.TrimSpace(req.Model)
	if req.KeyIndex < 0 || req.KeyIndex >= len(block.Keys) || modelName == "" || len(modelName) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "probe key or model is invalid"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), channelDiscoveryOperationTimeout)
	defer cancel()
	endpoints := map[string]string{}
	protocols := channelDiscoveryProbeProtocols(modelName)
	for _, protocol := range protocols {
		endpoint, err := probeChannelDiscoveryEndpoint(ctx, block, block.Keys[req.KeyIndex], protocol, modelName)
		if err != nil {
			continue
		}
		endpoints[protocol] = endpoint
	}
	if slices.Contains(protocols, "responses") && endpoints["responses"] == "" {
		endpoint, err := probeChannelDiscoveryEndpoint(ctx, block, block.Keys[req.KeyIndex], "chat", modelName)
		if err == nil {
			endpoints["chat"] = endpoint
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		c.JSON(http.StatusGatewayTimeout, gin.H{"success": false, "message": "channel protocol probe timed out"})
		return
	}
	if block.ModelsPath != "" {
		endpoints["models"] = channelDiscoveryRouteTarget(block, block.ModelsPath)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": endpoints})
}

// channelDiscoveryProbeProtocols avoids unnecessary inference calls for known
// native families. Unknown names try both protocols because model discovery
// returns identifiers, not protocol capabilities; image routes are inferred
// without generating media.
func channelDiscoveryProbeProtocols(modelName string) []string {
	switch {
	case isChannelDiscoveryImageModel(modelName):
		return nil
	case strings.HasPrefix(modelName, "claude-"):
		return []string{"messages"}
	case strings.HasPrefix(modelName, "gpt-"), strings.HasPrefix(modelName, "codex-"), strings.HasPrefix(modelName, "grok-"):
		return []string{"responses"}
	default:
		return []string{"responses", "messages"}
	}
}

type channelDiscoveryPreviewRequest struct {
	Text  string                `json:"text"`
	Draft channelDiscoveryDraft `json:"draft"`
}

// PreviewChannelDiscovery reads a draft from c, revalidates it against the
// current upstream, and writes the exact immutable write preview to the Gin
// response. The handler returns no Go value.
func PreviewChannelDiscovery(c *gin.Context) {
	req := channelDiscoveryPreviewRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	plan, err := prepareChannelDiscoveryPlan(c.Request.Context(), req.Text, req.Draft)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	preview, err := channelDiscoveryPreviewFromPlan(plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": preview})
}

type applyChannelDiscoveryRequest struct {
	Text         string                `json:"text"`
	Draft        channelDiscoveryDraft `json:"draft"`
	PreviewHash  string                `json:"preview_hash"`
	SnapshotHash string                `json:"snapshot_hash"`
}

// ApplyChannelDiscovery reads a reviewed preview from c, revalidates its hashes
// and upstream keys, then creates or atomically updates the channel. The result
// channel ID is written to the Gin response; the handler returns no Go value.
func ApplyChannelDiscovery(c *gin.Context) {
	req := applyChannelDiscoveryRequest{}
	if err := c.ShouldBindJSON(&req); err != nil || req.PreviewHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	plan, err := prepareChannelDiscoveryPlan(c.Request.Context(), req.Text, req.Draft)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	preview, err := channelDiscoveryPreviewFromPlan(plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// PreviewHash binds the newly rebuilt write plan; SnapshotHash is checked
	// again under the database row lock to close concurrent configuration edits.
	if preview.PreviewHash != req.PreviewHash || preview.SnapshotHash != req.SnapshotHash {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "channel preview changed; generate a new preview"})
		return
	}
	id, err := model.ApplyDiscoveredChannel(
		plan.Channel,
		req.SnapshotHash,
		plan.SyncConfiguration,
		plan.ReplaceKeys,
	)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrChannelConfigurationConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "channel.discovery_apply", map[string]any{
		"id":        id,
		"operation": plan.Operation,
		"changes":   plan.Changes,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"id": id}})
}

// prepareChannelDiscoveryPlan rebuilds a draft from its raw source and current
// upstream state. ctx bounds the whole revalidation; text and draft identify the
// selected block and edits. It returns only a fully validated write plan.
func prepareChannelDiscoveryPlan(ctx context.Context, text string, draft channelDiscoveryDraft) (channelDiscoveryPlan, error) {
	ctx, cancel := context.WithTimeout(ctx, channelDiscoveryOperationTimeout)
	defer cancel()
	// Browser discovery results are advisory. Preview and apply both refetch the
	// selected keys so a stale model list can never be written silently.
	block, err := channelDiscoveryRequestBlock(text, draft)
	if err != nil {
		return channelDiscoveryPlan{}, err
	}
	fetched, err := validateChannelDiscoveryDraft(ctx, block, draft)
	if err != nil {
		return channelDiscoveryPlan{}, err
	}
	plan, err := buildChannelDiscoveryPlan(block, draft, fetched)
	if err != nil {
		return channelDiscoveryPlan{}, err
	}
	if err := validateChannelDiscoveryPlan(plan); err != nil {
		return channelDiscoveryPlan{}, err
	}
	return plan, nil
}

func channelDiscoveryRequestBlock(text string, draft channelDiscoveryDraft) (channelDiscoveryBlock, error) {
	blocks, err := parseChannelDiscoveryBlocks(text)
	if err != nil {
		return channelDiscoveryBlock{}, err
	}
	if draft.BlockIndex < 0 || draft.BlockIndex >= len(blocks) {
		return channelDiscoveryBlock{}, errors.New("connection block is invalid")
	}
	return blocks[draft.BlockIndex], nil
}

// validateChannelDiscoveryDraft restricts discovery to the selected keys and to
// the Base URL that the operation will actually persist. It returns verified
// models and model-list route metadata or an error when any selected key fails.
func validateChannelDiscoveryDraft(ctx context.Context, block channelDiscoveryBlock, draft channelDiscoveryDraft) (channelDiscoveryFetchResult, error) {
	keys, err := selectedChannelDiscoveryKeys(block, draft.AcceptedKeyIndexes)
	if err != nil {
		return channelDiscoveryFetchResult{}, err
	}
	selectedBlock, err := channelDiscoveryValidationBlock(block, draft)
	if err != nil {
		return channelDiscoveryFetchResult{}, err
	}
	selectedBlock.Keys = keys
	fetched := discoverChannelBlock(ctx, selectedBlock)
	if fetched.Error != nil {
		return channelDiscoveryFetchResult{}, fetched.Error
	}
	if len(fetched.RejectedKeyIndexes) > 0 {
		return channelDiscoveryFetchResult{}, fmt.Errorf("%d selected API keys failed model discovery", len(fetched.RejectedKeyIndexes))
	}
	return fetched, nil
}

func channelDiscoveryValidationBlock(block channelDiscoveryBlock, draft channelDiscoveryDraft) (channelDiscoveryBlock, error) {
	// A key-only update keeps the stored Base URL, so validate its keys against
	// that connection. Create and configuration-sync operations validate the
	// editable Base URL because that is the address they will persist.
	if draft.Operation == "update" && !draft.SyncConfiguration {
		return block, nil
	}
	validationBlock, err := newChannelDiscoveryBlock(draft.BaseURL)
	if err != nil {
		return channelDiscoveryBlock{}, err
	}
	if validationBlock.Origin != block.Origin {
		return channelDiscoveryBlock{}, errors.New("base URL must keep the connection block origin")
	}
	return validationBlock, nil
}

func validateChannelDiscoveryPlan(plan channelDiscoveryPlan) error {
	isAdd := plan.Operation == "create"
	if err := validateChannel(plan.Channel, isAdd); err != nil {
		return err
	}
	if strings.TrimSpace(plan.Channel.Key) == "" {
		return fmt.Errorf("channel key is required")
	}
	for _, modelName := range plan.Channel.GetModels() {
		if len(modelName) > 255 {
			return fmt.Errorf("model name is too long: %s", modelName)
		}
	}
	return nil
}

func channelDiscoveryMatchRows() ([]*model.Channel, error) {
	var channels []*model.Channel
	err := model.DB.Select("id", "name", "base_url").Order("id asc").Find(&channels).Error
	return channels, err
}

func matchingChannelDiscoveryRows(block channelDiscoveryBlock, rows []*model.Channel) []channelDiscoveryMatch {
	matches := make([]channelDiscoveryMatch, 0)
	for _, row := range rows {
		base, err := newChannelDiscoveryBlock(row.GetBaseURL())
		if err != nil || base.BaseURL != block.BaseURL {
			continue
		}
		matches = append(matches, channelDiscoveryMatch{
			ID: row.Id, Name: row.Name,
		})
	}
	return matches
}

func channelDiscoverySuggestedName(block channelDiscoveryBlock) string {
	parsed, _ := url.Parse(block.BaseURL)
	hostname := strings.TrimPrefix(parsed.Hostname(), "www.")
	parts := strings.Split(hostname, ".")
	if len(parts) > 0 && parts[0] != "api" {
		return parts[0]
	}
	return hostname
}

func truncateChannelDiscoveryMessage(message string) string {
	runes := []rune(message)
	if len(runes) <= 512 {
		return message
	}
	return string(append(runes[:511], '…'))
}
