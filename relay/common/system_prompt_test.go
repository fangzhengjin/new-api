package common

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	common2 "github.com/QuantumNous/new-api/common"
	rootconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewriteResponsesSystemPromptBodyUsesFinalFormat(t *testing.T) {
	tests := []struct {
		name   string
		format types.RelayFormat
		mode   string
		want   string
	}{
		{name: "disabled", format: types.RelayFormatOpenAIResponses, mode: dto.SystemPromptModeNone, want: "base"},
		{name: "prepend", format: types.RelayFormatOpenAIResponses, mode: dto.SystemPromptModePrepend, want: "extra\n\nbase"},
		{name: "append", format: types.RelayFormatOpenAIResponses, mode: dto.SystemPromptModeAppend, want: "base\n\nextra"},
		{name: "override", format: types.RelayFormatOpenAIResponses, mode: dto.SystemPromptModeOverride, want: "extra"},
		{name: "compaction", format: types.RelayFormatOpenAIResponsesCompaction, mode: dto.SystemPromptModeAppend, want: "base\n\nextra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/source", nil)
			info := &RelayInfo{
				RelayFormat:            types.RelayFormatClaude,
				RequestConversionChain: []types.RelayFormat{types.RelayFormatClaude, tt.format},
				ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{
					SystemPrompt:     "extra",
					SystemPromptMode: tt.mode,
				}},
			}

			body, closer, err := RewriteResponsesSystemPromptBody(ctx, info, strings.NewReader(`{"instructions":"base"}`))
			require.NoError(t, err)
			if closer != nil {
				defer closer.Close()
			}
			payload, err := io.ReadAll(body)
			require.NoError(t, err)
			var got struct {
				Instructions string `json:"instructions"`
			}
			require.NoError(t, common2.Unmarshal(payload, &got))
			assert.Equal(t, tt.want, got.Instructions)
			assert.Equal(t, tt.mode == dto.SystemPromptModeOverride, common2.GetContextKeyBool(ctx, rootconstant.ContextKeySystemPromptOverride))
		})
	}
}

func TestRewriteResponsesSystemPromptBodyCreatesMissingInstructions(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIResponsesCompaction} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		info := &RelayInfo{
			RelayFormat: format,
			ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{
				SystemPrompt:     "extra",
				SystemPromptMode: dto.SystemPromptModeAppend,
			}},
		}

		body, closer, err := RewriteResponsesSystemPromptBody(ctx, info, strings.NewReader(`{"input":"hello"}`))
		require.NoError(t, err)
		require.NotNil(t, closer)
		payload, err := io.ReadAll(body)
		require.NoError(t, err)
		require.NoError(t, closer.Close())
		assert.JSONEq(t, `{"input":"hello","instructions":"extra"}`, string(payload))
	}
}

func TestRewriteResponsesSystemPromptBodyLeavesNonResponsesUnread(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	original := strings.NewReader(`{"instructions":"base"}`)
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAIResponses,
		FinalRequestRelayFormat: types.RelayFormatClaude,
		ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{
			SystemPrompt:     "extra",
			SystemPromptMode: dto.SystemPromptModeOverride,
		}},
	}

	body, closer, err := RewriteResponsesSystemPromptBody(ctx, info, original)
	require.NoError(t, err)
	assert.Nil(t, closer)
	assert.Same(t, original, body)
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"instructions":"base"}`, string(payload))
}

func TestRewriteResponsesSystemPromptBodyRewritesReplayableLiteBody(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set(codexResponsesLiteHeader, "true")
	original := []byte(`{"input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"base","annotations":["keep"]}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["model.base_instructions"]},"unknown":{"keep":true}},{"type":"message","role":"developer","content":[{"type":"input_text","text":"skills and tools"}],"tools":[{"type":"function","name":"keep"}]}]}`)
	storage, err := common2.CreateBodyStorage(original)
	require.NoError(t, err)
	defer storage.Close()
	source := common2.NewReplayableBodyReader(storage)
	info := &RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{
			SystemPrompt:     "extra",
			SystemPromptMode: dto.SystemPromptModeOverride,
		}},
	}

	body, closer, err := RewriteResponsesSystemPromptBody(ctx, info, source)
	require.NoError(t, err)
	require.NotNil(t, closer)
	defer closer.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"extra","annotations":["keep"]}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["model.base_instructions"]},"unknown":{"keep":true}},{"type":"message","role":"developer","content":[{"type":"input_text","text":"skills and tools"}],"tools":[{"type":"function","name":"keep"}]}]}`, string(payload))

	unconsumedSource, err := io.ReadAll(source)
	require.NoError(t, err)
	assert.Equal(t, original, unconsumedSource)
	replayable, ok := body.(common2.ReplayableBody)
	require.True(t, ok)
	replay, err := replayable.NewReader()
	require.NoError(t, err)
	defer replay.Close()
	replayedPayload, err := io.ReadAll(replay)
	require.NoError(t, err)
	assert.Equal(t, payload, replayedPayload)
}

func TestRewriteResponsesSystemPromptBodyLeavesUnidentifiedReplayableBodyUnchanged(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set(codexResponsesLiteHeader, "true")
	original := []byte(`{"input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"skills"}]}]}`)
	storage, err := common2.CreateBodyStorage(original)
	require.NoError(t, err)
	defer storage.Close()
	source := common2.NewReplayableBodyReader(storage)
	info := &RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{
			SystemPrompt:     "extra",
			SystemPromptMode: dto.SystemPromptModeOverride,
		}},
	}

	body, closer, err := RewriteResponsesSystemPromptBody(ctx, info, source)
	require.NoError(t, err)
	assert.Nil(t, closer)
	assert.Equal(t, source, body)
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, original, payload)
}

func TestRewriteResponsesSystemPromptBodyPrefersEmptyInstructions(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set(codexResponsesLiteHeader, "true")
	input := `[{"type":"message","role":"developer","content":[{"type":"input_text","text":"base"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["model.base_instructions"]}}]`
	info := &RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{
			SystemPrompt:     "extra",
			SystemPromptMode: dto.SystemPromptModeAppend,
		}},
	}

	body, closer, err := RewriteResponsesSystemPromptBody(ctx, info, strings.NewReader(`{"instructions":"","input":`+input+`}`))
	require.NoError(t, err)
	require.NotNil(t, closer)
	defer closer.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"instructions":"extra","input":`+input+`}`, string(payload))
}

func TestRewriteResponsesSystemPromptBodyUsesLiteLegacyPrefix(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set(codexResponsesLiteHeader, "true")
	info := &RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{
			SystemPrompt:     "extra",
			SystemPromptMode: dto.SystemPromptModeAppend,
		}},
	}

	body, closer, err := RewriteResponsesSystemPromptBody(ctx, info, strings.NewReader(`{"input":[{"type":"additional_tools","role":"developer","tools":[]},{"type":"message","role":"developer","content":[{"type":"input_text","text":"base"}]},{"type":"message","role":"developer","content":[{"type":"input_text","text":"skills"}]}]}`))
	require.NoError(t, err)
	require.NotNil(t, closer)
	defer closer.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"input":[{"type":"additional_tools","role":"developer","tools":[]},{"type":"message","role":"developer","content":[{"type":"input_text","text":"base\n\nextra"}]},{"type":"message","role":"developer","content":[{"type":"input_text","text":"skills"}]}]}`, string(payload))
}
