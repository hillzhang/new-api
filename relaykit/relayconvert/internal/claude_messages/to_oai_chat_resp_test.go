package claudemessages

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseClaude2OpenAI_ConcatenatesMultipleTextBlocks(t *testing.T) {
	// 验证多个 Text 块能够正确拼接合并，不会出现后一块覆盖前一块导致文本丢失的缺陷
	claudeResp := &dto.ClaudeResponse{
		Id:    "msg_test_multi_text",
		Model: "claude-fable-5-1",
		Content: []dto.ClaudeMediaMessage{
			{
				Type: "text",
				Text: kitutil.GetPointer("Hello, "),
			},
			{
				Type: "text",
				Text: kitutil.GetPointer("World! Welcome to project-newapi."),
			},
		},
		StopReason: "end_turn",
	}

	oaiResp := ResponseClaude2OpenAI(claudeResp)
	require.NotNil(t, oaiResp)
	assert.Equal(t, "msg_test_multi_text", oaiResp.Id)
	assert.Equal(t, "claude-fable-5-1", oaiResp.Model)
	require.Len(t, oaiResp.Choices, 1)

	choice := oaiResp.Choices[0]
	assert.Equal(t, "Hello, World! Welcome to project-newapi.", choice.Message.StringContent())
}

func TestResponseClaude2OpenAI_ThinkingExposedDual(t *testing.T) {
	// 验证自适应思考内容能够双重暴露（choice.ReasoningContent 与 choice.Message.ReasoningContent），兼容各类客户端
	thought1 := "First let me analyze the query.\n"
	thought2 := "Then provide the solution.\n"

	claudeResp := &dto.ClaudeResponse{
		Id:    "msg_test_thinking",
		Model: "claude-fable-5-1",
		Content: []dto.ClaudeMediaMessage{
			{
				Type:     "thinking",
				Thinking: &thought1,
			},
			{
				Type:     "thinking",
				Thinking: &thought2,
			},
			{
				Type: "text",
				Text: kitutil.GetPointer("The answer is 42."),
			},
		},
		StopReason: "end_turn",
	}

	oaiResp := ResponseClaude2OpenAI(claudeResp)
	require.NotNil(t, oaiResp)
	require.Len(t, oaiResp.Choices, 1)

	choice := oaiResp.Choices[0]
	assert.Equal(t, "The answer is 42.", choice.Message.StringContent())

	expectedThinking := "First let me analyze the query.\nThen provide the solution.\n"
	require.NotNil(t, choice.ReasoningContent)
	assert.Equal(t, expectedThinking, *choice.ReasoningContent)

	require.NotNil(t, choice.Message.ReasoningContent)
	assert.Equal(t, expectedThinking, *choice.Message.ReasoningContent)
}

func TestResponseClaude2OpenAI_ToolUseAndThinking(t *testing.T) {
	thought := "I need to call the weather tool."
	claudeResp := &dto.ClaudeResponse{
		Id:    "msg_test_tool",
		Model: "claude-fable-5-1",
		Content: []dto.ClaudeMediaMessage{
			{
				Type:     "thinking",
				Thinking: &thought,
			},
			{
				Type: "tool_use",
				Id:   "call_weather_123",
				Name: "get_weather",
				Input: map[string]any{
					"location": "Beijing",
				},
			},
		},
		StopReason: "tool_use",
	}

	oaiResp := ResponseClaude2OpenAI(claudeResp)
	require.NotNil(t, oaiResp)
	require.Len(t, oaiResp.Choices, 1)

	choice := oaiResp.Choices[0]
	assert.Equal(t, "tool_calls", choice.FinishReason)
	assert.Equal(t, thought, *choice.Message.ReasoningContent)

	toolCalls := choice.Message.ParseToolCalls()
	require.Len(t, toolCalls, 1)
	tc := toolCalls[0]
	assert.Equal(t, "call_weather_123", tc.ID)
	assert.Equal(t, "get_weather", tc.Function.Name)
	assert.JSONEq(t, `{"location":"Beijing"}`, tc.Function.Arguments)
}

func TestStreamResponseClaude2OpenAI_ThinkingDelta(t *testing.T) {
	// 验证流式 thinking_delta 转换为 reasoning_content
	thoughtChunk := "Analyzing the problem..."
	claudeResp := &dto.ClaudeResponse{
		Type: "content_block_delta",
		Delta: &dto.ClaudeMediaMessage{
			Type:     "thinking_delta",
			Thinking: &thoughtChunk,
		},
	}

	streamResp := StreamResponseClaude2OpenAI(claudeResp)
	require.NotNil(t, streamResp)
	require.Len(t, streamResp.Choices, 1)

	choice := streamResp.Choices[0]
	assert.Nil(t, choice.Delta.Content)
	require.NotNil(t, choice.Delta.ReasoningContent)
	assert.Equal(t, "Analyzing the problem...", *choice.Delta.ReasoningContent)
}

func TestStreamResponseClaude2OpenAI_TextDelta(t *testing.T) {
	// 验证流式 text_delta 转换为 content
	textChunk := "Hello world"
	claudeResp := &dto.ClaudeResponse{
		Type: "content_block_delta",
		Delta: &dto.ClaudeMediaMessage{
			Type: "text_delta",
			Text: &textChunk,
		},
	}

	streamResp := StreamResponseClaude2OpenAI(claudeResp)
	require.NotNil(t, streamResp)
	require.Len(t, streamResp.Choices, 1)

	choice := streamResp.Choices[0]
	assert.Nil(t, choice.Delta.ReasoningContent)
	require.NotNil(t, choice.Delta.Content)
	assert.Equal(t, "Hello world", *choice.Delta.Content)
}
