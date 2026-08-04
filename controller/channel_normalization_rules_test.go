package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChannelNormalizationCandidateAppliesManagedRules(t *testing.T) {
	mapping := `{"claude-opus-4-7":"claude-opus-4-7-thinking","codex-auto-review":"gpt-5.4","gpt-5.4-mini":"gpt-5.4","gpt-5.5-openai-compact":"gpt-5.5"}`
	channel := &model.Channel{
		Id:           12,
		Type:         constant.ChannelTypeOpenAI,
		Models:       "gpt-4-turbo,claude-opus-4-7,codex-auto-review,gpt-5.4,gpt-5.4-mini,gpt-5.5,gpt-5.5-openai-compact",
		ModelMapping: &mapping,
	}
	candidate, err := buildChannelNormalizationCandidate(channel, []string{"gpt-5.4", "gpt-5.5", "claude-opus-4.7"}, true)
	require.NoError(t, err)
	assert.NotContains(t, candidate.Models, "gpt-4-turbo")
	assert.NotContains(t, candidate.Models, "gpt-5.4-mini")
	assert.NotContains(t, candidate.Models, "gpt-5.5-openai-compact")
	assert.NotContains(t, candidate.Models, "codex-auto-review")
	assert.Contains(t, candidate.Models, "claude-opus-4-7")
	assert.NotContains(t, candidate.Mapping, "gpt-5.4-mini")
	assert.NotContains(t, candidate.Mapping, "codex-auto-review")
	assert.Equal(t, "claude-opus-4.7", candidate.Mapping["claude-opus-4-7"])
	assert.NotContains(t, candidate.Mapping, "gpt-5.5-openai-compact")
}

func TestBuildChannelNormalizationCandidateCleansStoredConfigurationWithoutUpstream(t *testing.T) {
	mapping := `{"gpt-5.5-openai-compact":"gpt-5.5"}`
	channel := &model.Channel{
		Type:         constant.ChannelTypeOpenAI,
		Models:       "gpt-5.5,gpt-5.5-openai-compact",
		ModelMapping: &mapping,
	}

	candidate, err := buildChannelNormalizationCandidate(channel, channel.GetModels(), false)
	require.NoError(t, err)
	assert.Equal(t, []string{"gpt-5.5"}, candidate.Models)
	assert.Empty(t, candidate.Mapping)
}

func TestBuildChannelNormalizationCandidateKeepsUpstreamMiniWithoutMapping(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI}
	candidate, err := buildChannelNormalizationCandidate(channel, []string{"gpt-5.4-mini"}, true)
	require.NoError(t, err)
	assert.Contains(t, candidate.Models, "gpt-5.4-mini")
	assert.NotContains(t, candidate.Mapping, "gpt-5.4-mini")
}

func TestBuildChannelNormalizationCandidateKeepsUpstreamCodexAutoReviewWithoutMapping(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI}
	candidate, err := buildChannelNormalizationCandidate(channel, []string{"codex-auto-review"}, true)
	require.NoError(t, err)
	assert.Contains(t, candidate.Models, "codex-auto-review")
	assert.NotContains(t, candidate.Mapping, "codex-auto-review")
}

func TestValidateChannelNormalizationSelectionRequiresMappingDependencies(t *testing.T) {
	candidate := channelNormalizationItem{
		ChannelID:     12,
		SnapshotHash:  "snapshot",
		AddModels:     []string{"claude-opus-4-7"},
		RemoveModels:  []string{"legacy-alias"},
		MappingSet:    map[string]string{"claude-opus-4-7": "claude-opus-4.7"},
		MappingRemove: []string{"legacy-alias"},
	}

	_, err := validateChannelNormalizationSelection(candidate, applyChannelNormalizationSelection{
		ChannelID: 12,
		AddModels: []string{"claude-opus-4-7"},
	})
	require.ErrorContains(t, err, "requires its mapping")

	_, err = validateChannelNormalizationSelection(candidate, applyChannelNormalizationSelection{
		ChannelID:    12,
		RemoveModels: []string{"legacy-alias"},
	})
	require.ErrorContains(t, err, "requires removing its mapping")
}

func TestValidateChannelNormalizationSelectionForcesSortingWithModelChanges(t *testing.T) {
	tests := []struct {
		name      string
		candidate channelNormalizationItem
		selected  applyChannelNormalizationSelection
	}{
		{
			name:      "addition",
			candidate: channelNormalizationItem{ChannelID: 12, AddModels: []string{"gpt-5.6-sol"}},
			selected:  applyChannelNormalizationSelection{ChannelID: 12, AddModels: []string{"gpt-5.6-sol"}},
		},
		{
			name:      "removal",
			candidate: channelNormalizationItem{ChannelID: 12, RemoveModels: []string{"gpt-4-turbo"}},
			selected:  applyChannelNormalizationSelection{ChannelID: 12, RemoveModels: []string{"gpt-4-turbo"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutation, err := validateChannelNormalizationSelection(test.candidate, test.selected)
			require.NoError(t, err)
			assert.True(t, mutation.SortModels)
		})
	}
}

func TestAllowedOpenAIModelRejectsEmptyImageVersion(t *testing.T) {
	assert.False(t, isChannelNormalizationAllowedOpenAIModel("gpt-image-"))
}

func TestChannelNormalizationTargetsOnlySupportedChannelTypes(t *testing.T) {
	assert.False(t, isChannelNormalizationTarget(&model.Channel{
		Type:   constant.ChannelTypeGemini,
		Models: "gpt-4-turbo",
	}))
	assert.True(t, isChannelNormalizationTarget(&model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Models: "gpt-4-turbo",
	}))
}

func TestBuildChannelNormalizationCandidatePrefersCurrentDottedHaiku(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeAnthropic}
	candidate, err := buildChannelNormalizationCandidate(channel, []string{
		"claude-haiku-4-5",
		"claude-haiku-4.5-20251001",
	}, true)
	require.NoError(t, err)

	assert.Equal(t, "claude-haiku-4.5-20251001", candidate.Mapping["claude-haiku-4-5-20251001"])
}

func TestBuildChannelNormalizationCandidateWarnsAboutRetainedMissingTarget(t *testing.T) {
	mapping := `{"custom-alias":"missing-upstream"}`
	channel := &model.Channel{
		Type:         constant.ChannelTypeOpenAI,
		Models:       "custom-alias",
		ModelMapping: &mapping,
	}
	candidate, err := buildChannelNormalizationCandidate(channel, []string{"gpt-5.4"}, true)
	require.NoError(t, err)

	assert.Equal(t, "missing-upstream", candidate.MappingWarnings["custom-alias"])
}

func TestFetchChannelUpstreamModelIDsWithContextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	baseURL := "http://127.0.0.1:1"
	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		BaseURL: &baseURL,
		Key:     "test-key",
	}

	_, err := fetchChannelUpstreamModelIDsWithContext(ctx, channelDiscoveryMaxResponseBytes, channel)
	require.ErrorContains(t, err, "context canceled")
}

func TestFetchChannelUpstreamModelIDsWithContextLimitsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"data":[{"id":"gpt-5.4"}]}`))
	}))
	t.Cleanup(server.Close)
	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		BaseURL: &server.URL,
		Key:     "test-key",
	}

	_, err := fetchChannelUpstreamModelIDsWithContext(context.Background(), 8, channel)
	require.ErrorContains(t, err, "upstream response exceeds 8 bytes")
}
