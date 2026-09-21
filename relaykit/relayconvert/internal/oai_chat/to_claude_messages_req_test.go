package oaichat

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIChatRequestToClaudeMessagesNormalizesToolInputSchema(t *testing.T) {
	tests := []struct {
		name       string
		parameters any
		wantSchema map[string]any
	}{
		{
			name:       "omitted parameters",
			parameters: nil,
			wantSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			name: "missing type and properties",
			parameters: map[string]any{
				"additionalProperties": false,
			},
			wantSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			name: "non-string type",
			parameters: map[string]any{
				"type":       123,
				"properties": map[string]any{},
			},
			wantSchema: map[string]any{
				"type":       123,
				"properties": map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxTokens := uint(1024)
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model:     "claude-test",
				MaxTokens: &maxTokens,
				Messages: []dto.Message{
					{Role: "user", Content: "Call the tool."},
				},
				Tools: []dto.ToolCallRequest{
					{
						Type: "function",
						Function: dto.FunctionRequest{
							Name:        "get_current_time",
							Description: "Get the current time",
							Parameters:  tt.parameters,
						},
					},
				},
			})

			require.NoError(t, err)
			tools, ok := got.Tools.([]any)
			require.True(t, ok)
			require.Len(t, tools, 1)
			tool, ok := tools[0].(*dto.Tool)
			require.True(t, ok)
			assert.Equal(t, "get_current_time", tool.Name)
			assert.Equal(t, tt.wantSchema, tool.InputSchema)
		})
	}
}

// TestOpenAIChatRequestToClaudeMessages_ClaudeFableAdaptiveThinking 验证网宿智算平台 Claude Fable 系列模型转译：
// 1. 自动开启自适应思考 (adaptive thinking)
// 2. 彻底置空采样参数 (temperature, top_p, top_k 设为 nil)，避免触发网关 503 报错
// 3. 将 OpenAI 的 reasoning_effort 自动映射为 output_config.effort
// 4. 将 OpenAI 的 user 字段映射为 metadata.user_id
func TestOpenAIChatRequestToClaudeMessages_ClaudeFableAdaptiveThinking(t *testing.T) {
	temp := 0.3
	topP := 0.9
	topK := 50
	maxTokens := uint(2048)

	req := dto.GeneralOpenAIRequest{
		Model:           "claude-fable-5-1",
		MaxTokens:       &maxTokens,
		Temperature:     &temp,
		TopP:            &topP,
		TopK:            &topK,
		ReasoningEffort: "high",
		User:            []byte(`"usr-fable-test-123"`),
		Messages: []dto.Message{
			{Role: "user", Content: "Hello Fable"},
		},
	}

	got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, req)
	require.NoError(t, err)
	require.NotNil(t, got)

	// 1. 验证采样参数已被强制置空（JSON 序列化时将自动省略）
	assert.Nil(t, got.Temperature, "Fable 思考模型必须清除 temperature")
	assert.Nil(t, got.TopP, "Fable 思考模型必须清除 top_p")
	assert.Nil(t, got.TopK, "Fable 思考模型必须清除 top_k")

	// 2. 验证 thinking 模式为 adaptive
	require.NotNil(t, got.Thinking)
	assert.Equal(t, "adaptive", got.Thinking.Type)
	assert.Nil(t, got.Thinking.BudgetTokens, "adaptive 模式严禁传递 budget_tokens")

	// 3. 验证 reasoning_effort 映射为 output_config.effort
	assert.JSONEq(t, `{"effort":"high"}`, string(got.OutputConfig))

	// 4. 验证 user 映射为 metadata.user_id
	assert.JSONEq(t, `{"user_id":"usr-fable-test-123"}`, string(got.Metadata))
}

// TestOpenAIChatRequestToClaudeMessages_ClaudeFable_DefaultMaxTokens 验证未传递 max_tokens 时的默认兜底行为 (4096)
func TestOpenAIChatRequestToClaudeMessages_ClaudeFable_DefaultMaxTokens(t *testing.T) {
	req := dto.GeneralOpenAIRequest{
		Model: "claude-fable-5-1",
		Messages: []dto.Message{
			{Role: "user", Content: "Hello Fable without max_tokens"},
		},
	}

	got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, req)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.MaxTokens)
	assert.Equal(t, uint(4096), *got.MaxTokens, "未传递 max_tokens 时应自动兜底为 4096")
}

// TestOpenAIChatRequestToClaudeMessages_ClaudeFable_ProtectsFromOpenRouterReasoning 验证 OpenRouter reasoning 格式不会破坏 Fable 自适应思考
func TestOpenAIChatRequestToClaudeMessages_ClaudeFable_ProtectsFromOpenRouterReasoning(t *testing.T) {
	req := dto.GeneralOpenAIRequest{
		Model:     "claude-fable-5-1",
		Reasoning: json.RawMessage(`{"max_tokens": 2048}`),
		Messages: []dto.Message{
			{Role: "user", Content: "Hello OpenRouter Client"},
		},
	}

	got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, req)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Thinking)
	assert.Equal(t, "adaptive", got.Thinking.Type, "自适应思考模型不得被 OpenRouter reasoning 覆写为 enabled")
	assert.Nil(t, got.Thinking.BudgetTokens, "自适应思考模型严禁 budget_tokens")
}

