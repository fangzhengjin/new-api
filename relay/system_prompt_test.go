package relay

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/vertex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplySystemPromptIfNeededCreatesMissingSystemMessage(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := &dto.GeneralOpenAIRequest{Messages: []dto.Message{{Role: "user", Content: "hello"}}}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{
		SystemPrompt:     "extra",
		SystemPromptMode: dto.SystemPromptModeOverride,
	}}}

	applySystemPromptIfNeeded(ctx, info, request)
	require.Len(t, request.Messages, 2)
	assert.Equal(t, request.GetSystemRoleName(), request.Messages[0].Role)
	assert.Equal(t, "extra", request.Messages[0].StringContent())
	assert.Equal(t, "user", request.Messages[1].Role)
	assert.False(t, common.GetContextKeyBool(ctx, constant.ContextKeySystemPromptOverride))
}

func TestApplySystemPromptIfNeededDoesNotAddSeparatorToEmptyMedia(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := &dto.GeneralOpenAIRequest{Messages: []dto.Message{{Role: "system"}}}
	request.Messages[0].SetMediaContent(nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{
		SystemPrompt:     "extra",
		SystemPromptMode: dto.SystemPromptModeAppend,
	}}}

	applySystemPromptIfNeeded(ctx, info, request)
	contents := request.Messages[0].ParseContent()
	require.Len(t, contents, 1)
	assert.Equal(t, "extra", contents[0].Text)
}

func TestApplyClaudeSystemPromptIfNeededAppends(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := &dto.ClaudeRequest{}
	request.SetStringSystem("base")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{
		SystemPrompt:     "extra",
		SystemPromptMode: dto.SystemPromptModeAppend,
	}}}

	applyClaudeSystemPromptIfNeeded(ctx, info, request)
	assert.Equal(t, "base\n\nextra", request.GetStringSystem())
}

func TestApplyGeminiSystemPromptIfNeededOverrides(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := &dto.GeminiChatRequest{SystemInstructions: &dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: "base"}, {Text: "keep"}}}}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{
		SystemPrompt:     "extra",
		SystemPromptMode: dto.SystemPromptModeOverride,
	}}}

	applyGeminiSystemPromptIfNeeded(ctx, info, request)
	require.Len(t, request.SystemInstructions.Parts, 1)
	assert.Equal(t, "extra", request.SystemInstructions.Parts[0].Text)
}

func TestApplyConvertedSystemPromptIfNeededDefersResponsesToSendLayer(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := &dto.OpenAIResponsesRequest{Instructions: json.RawMessage(`"base"`)}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{
		SystemPrompt:     "extra",
		SystemPromptMode: dto.SystemPromptModeAppend,
	}}}

	applyConvertedSystemPromptIfNeeded(ctx, info, request)
	assert.JSONEq(t, `"base"`, string(request.Instructions))
}

func TestVertexClaudeSystemPromptAfterProviderConversion(t *testing.T) {
	for _, test := range []struct {
		name   string
		mode   string
		system string
		want   string
	}{
		{name: "none", mode: dto.SystemPromptModeNone, system: `"base"`, want: `"base"`},
		{name: "prepend", mode: dto.SystemPromptModePrepend, system: `"base"`, want: `"extra\n\nbase"`},
		{name: "append", mode: dto.SystemPromptModeAppend, system: `"base"`, want: `"base\n\nextra"`},
		{name: "override", mode: dto.SystemPromptModeOverride, system: `"base"`, want: `"extra"`},
		{name: "missing", mode: dto.SystemPromptModeAppend, system: `null`, want: `"extra"`},
		{name: "cached blocks append", mode: dto.SystemPromptModeAppend, system: `[{"type":"text","text":"base","cache_control":{"type":"ephemeral"}}]`, want: `[{"type":"text","text":"base","cache_control":{"type":"ephemeral"}},{"type":"text","text":"\n\nextra"}]`},
		{name: "blocks override", mode: dto.SystemPromptModeOverride, system: `[{"type":"text","text":"base","cache_control":{"type":"ephemeral"}}]`, want: `[{"type":"text","text":"extra"}]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			info := &relaycommon.RelayInfo{
				RelayFormat:     types.RelayFormatClaude,
				OriginModelName: "claude-sonnet-4-5",
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:       constant.ChannelTypeVertexAi,
					UpstreamModelName: "claude-sonnet-4-5",
					ChannelSetting:    dto.ChannelSettings{SystemPrompt: "extra", SystemPromptMode: test.mode},
				},
			}
			var request dto.ClaudeRequest
			require.NoError(t, common.UnmarshalJsonStr(`{"model":"claude-sonnet-4-5","max_tokens":64,"messages":[{"role":"user","content":"hello"}],"system":`+test.system+`}`, &request))
			adaptor := &vertex.Adaptor{}
			converted, err := adaptor.ConvertClaudeRequest(ctx, info, &request)
			require.NoError(t, err)
			applyConvertedSystemPromptIfNeeded(ctx, info, converted)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			var outbound map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(body, &outbound))
			assert.JSONEq(t, test.want, string(outbound["system"]))
			assert.JSONEq(t, `64`, string(outbound["max_tokens"]))
			assert.JSONEq(t, `[{"role":"user","content":"hello"}]`, string(outbound["messages"]))
			assert.NotContains(t, outbound, "model")
			assert.NotEmpty(t, outbound["anthropic_version"])
		})
	}
}

type systemPromptCaptureAdaptor struct {
	channel.Adaptor
	converted any
	body      []byte
}

func (a *systemPromptCaptureAdaptor) ConvertOpenAIResponsesRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ dto.OpenAIResponsesRequest) (any, error) {
	return a.converted, nil
}

func (a *systemPromptCaptureAdaptor) DoRequest(_ *gin.Context, _ *relaycommon.RelayInfo, body io.Reader) (any, error) {
	var err error
	a.body, err = io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	return nil, errors.New("request captured")
}

func TestTextRequestViaResponsesAppliesPromptAfterFinalConversion(t *testing.T) {
	tests := []struct {
		name      string
		converted any
		assert    func(*testing.T, []byte)
	}{
		{
			name: "chat",
			converted: &dto.GeneralOpenAIRequest{Messages: []dto.Message{
				{Role: "system", Content: "base"},
			}},
			assert: func(t *testing.T, body []byte) {
				var request dto.GeneralOpenAIRequest
				require.NoError(t, common.Unmarshal(body, &request))
				require.Len(t, request.Messages, 1)
				assert.Equal(t, "base\n\nextra", request.Messages[0].StringContent())
			},
		},
		{
			name: "gemini",
			converted: &dto.GeminiChatRequest{SystemInstructions: &dto.GeminiChatContent{
				Parts: []dto.GeminiPart{{Text: "base"}},
			}},
			assert: func(t *testing.T, body []byte) {
				var request dto.GeminiChatRequest
				require.NoError(t, common.Unmarshal(body, &request))
				require.NotNil(t, request.SystemInstructions)
				require.Len(t, request.SystemInstructions.Parts, 1)
				assert.Equal(t, "base\n\nextra", request.SystemInstructions.Parts[0].Text)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{
					SystemPrompt:     "extra",
					SystemPromptMode: dto.SystemPromptModeAppend,
				}},
			}
			adaptor := &systemPromptCaptureAdaptor{converted: test.converted}

			_, newAPIError := textRequestViaResponses(ctx, info, adaptor, &dto.GeneralOpenAIRequest{
				Model:    "gpt-test",
				Messages: []dto.Message{{Role: "user", Content: "hello"}},
			})

			require.NotNil(t, newAPIError)
			test.assert(t, adaptor.body)
		})
	}
}
