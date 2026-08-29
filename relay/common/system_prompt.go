package common

import (
	"fmt"
	"io"

	common2 "github.com/QuantumNous/new-api/common"
	rootconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const codexResponsesLiteHeader = "X-OpenAI-Internal-Codex-Responses-Lite"

// RewriteResponsesSystemPromptBody applies the channel prompt to the final
// outbound Responses JSON body without consuming a replayable source body.
func RewriteResponsesSystemPromptBody(c *gin.Context, info *RelayInfo, requestBody io.Reader) (io.Reader, io.Closer, error) {
	if info == nil || info.ChannelMeta == nil || requestBody == nil {
		return requestBody, nil, nil
	}
	format := info.GetFinalRequestRelayFormat()
	if format != types.RelayFormatOpenAIResponses && format != types.RelayFormatOpenAIResponsesCompaction {
		return requestBody, nil, nil
	}
	setting := info.ChannelSetting
	mode := setting.EffectiveSystemPromptMode()
	if mode == dto.SystemPromptModeNone || setting.SystemPrompt == "" {
		return requestBody, nil, nil
	}

	readBody := requestBody
	var replay io.ReadCloser
	if replayable, ok := requestBody.(common2.ReplayableBody); ok {
		var err error
		replay, err = replayable.NewReader()
		if err != nil {
			return nil, nil, fmt.Errorf("replay responses request body: %w", err)
		}
		defer replay.Close()
		readBody = replay
	}
	// ponytail: request bodies are bounded by MAX_REQUEST_BODY_MB; use a streaming JSON rewriter if buffering becomes measurable.
	body, err := io.ReadAll(readBody)
	if err != nil {
		return nil, nil, fmt.Errorf("read responses request body: %w", err)
	}

	path := responsesSystemPromptPath(body, c != nil && c.GetHeader(codexResponsesLiteHeader) == "true")
	if path == "" {
		if c != nil && c.Request != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("responses system prompt rewrite skipped: mode=%s, reason=target_not_found", mode))
		}
		if replay != nil {
			return requestBody, nil, nil
		}
		body, closer, err := NewOutboundJSONBody(body)
		return body, closer, err
	}

	existing := gjson.GetBytes(body, path).String()
	rewritten, changed := setting.RewriteSystemPrompt(existing)
	if !changed {
		if replay != nil {
			return requestBody, nil, nil
		}
		body, closer, err := NewOutboundJSONBody(body)
		return body, closer, err
	}
	body, err = sjson.SetBytes(body, path, rewritten)
	if err != nil {
		return nil, nil, fmt.Errorf("rewrite responses system prompt: %w", err)
	}
	MarkSystemPromptRewrite(c, mode)
	rewrittenBody, closer, err := NewOutboundJSONBody(body)
	return rewrittenBody, closer, err
}

func responsesSystemPromptPath(body []byte, responsesLite bool) string {
	instructions := gjson.GetBytes(body, "instructions")
	if instructions.Type == gjson.String {
		return "instructions"
	}
	if !responsesLite {
		return "instructions"
	}

	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return ""
	}
	items := input.Array()
	for index, item := range items {
		if item.Get("role").String() != "developer" {
			continue
		}
		for _, kind := range item.Get("internal_chat_message_metadata_passthrough.content_item_kinds").Array() {
			if kind.String() == "model.base_instructions" {
				return responsesInputTextPath(items, index)
			}
		}
	}

	if len(items) >= 2 &&
		items[0].Get("type").String() == "additional_tools" && items[0].Get("role").String() == "developer" &&
		items[1].Get("type").String() == "message" && items[1].Get("role").String() == "developer" {
		return responsesInputTextPath(items, 1)
	}
	return ""
}

func responsesInputTextPath(items []gjson.Result, itemIndex int) string {
	for contentIndex, content := range items[itemIndex].Get("content").Array() {
		if content.Get("type").String() == "input_text" && content.Get("text").Type == gjson.String {
			return fmt.Sprintf("input.%d.content.%d.text", itemIndex, contentIndex)
		}
	}
	return ""
}

// MarkSystemPromptRewrite records an actual prompt replacement for usage logs.
func MarkSystemPromptRewrite(c *gin.Context, mode string) {
	if c != nil && mode == dto.SystemPromptModeOverride {
		common2.SetContextKey(c, rootconstant.ContextKeySystemPromptOverride, true)
	}
}
