package openai

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIRequest_GPT6AstraSanitization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-6-astra",
		},
	}

	req := &dto.GeneralOpenAIRequest{
		Model:       "gpt-6-astra",
		Temperature: lo.ToPtr(0.7),
		TopP:        lo.ToPtr(0.9),
		LogProbs:    lo.ToPtr(true),
		MaxTokens:   lo.ToPtr(uint(2048)),
		Messages: []dto.Message{
			{
				Role:    "system",
				Content: "You are a helpful assistant.",
			},
			{
				Role:    "user",
				Content: "Hello",
			},
		},
	}

	res, err := adaptor.ConvertOpenAIRequest(c, info, req)
	require.NoError(t, err)

	converted, ok := res.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)

	// Temperature, TopP, LogProbs must be stripped (set to nil) for GPT-6
	assert.Nil(t, converted.Temperature, "temperature should be nil for gpt-6")
	assert.Nil(t, converted.TopP, "top_p should be nil for gpt-6")
	assert.Nil(t, converted.LogProbs, "logprobs should be nil for gpt-6")

	// MaxTokens should be migrated to MaxCompletionTokens and cleared
	assert.Nil(t, converted.MaxTokens, "max_tokens should be nil after migration")
	require.NotNil(t, converted.MaxCompletionTokens, "max_completion_tokens should be set")
	assert.Equal(t, uint(2048), *converted.MaxCompletionTokens)

	// system message should be converted to developer
	assert.Equal(t, "developer", converted.Messages[0].Role, "system role should be converted to developer for gpt-6")
}

func TestConvertOpenAIRequest_GPT6_DualMaxTokensSanitization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-6-astra",
		},
	}

	// 模拟同时传递 max_tokens 与 max_completion_tokens 的场景
	req := &dto.GeneralOpenAIRequest{
		Model:               "gpt-6-astra",
		MaxTokens:           lo.ToPtr(uint(1000)),
		MaxCompletionTokens: lo.ToPtr(uint(4000)),
	}

	res, err := adaptor.ConvertOpenAIRequest(c, info, req)
	require.NoError(t, err)

	converted, ok := res.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)

	// max_tokens 必须被无条件置空，防止触发上游同时传递双参数的 400 报错
	assert.Nil(t, converted.MaxTokens, "max_tokens must be nil when max_completion_tokens is already set")
	require.NotNil(t, converted.MaxCompletionTokens)
	assert.Equal(t, uint(4000), *converted.MaxCompletionTokens)
}

func TestConvertOpenAIRequest_GPT6_CaseInsensitive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "GPT-6-Astra",
		},
	}

	req := &dto.GeneralOpenAIRequest{
		Model:       "GPT-6-Astra",
		Temperature: lo.ToPtr(0.5),
		MaxTokens:   lo.ToPtr(uint(1024)),
	}

	res, err := adaptor.ConvertOpenAIRequest(c, info, req)
	require.NoError(t, err)

	converted, ok := res.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)

	assert.Nil(t, converted.Temperature, "temperature must be stripped even if model name is uppercase")
	assert.Nil(t, converted.MaxTokens)
	require.NotNil(t, converted.MaxCompletionTokens)
	assert.Equal(t, uint(1024), *converted.MaxCompletionTokens)
}

func TestConvertOpenAIRequest_GPT6_FunctionToolsSetsReasoningEffortToNone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-6-astra",
		},
	}

	// 场景 1: 客户端传入了 tools 且显式传了 reasoning_effort: "medium"
	req := &dto.GeneralOpenAIRequest{
		Model:           "gpt-6-astra",
		ReasoningEffort: "medium",
		Tools: []dto.ToolCallRequest{
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name: "get_current_weather",
				},
			},
		},
	}

	res, err := adaptor.ConvertOpenAIRequest(c, info, req)
	require.NoError(t, err)

	converted, ok := res.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)

	// 必须将 reasoning_effort 强制置为 'none'，以满足 OpenAI /v1/chat/completions 的要求
	assert.Equal(t, "none", converted.ReasoningEffort, "reasoning_effort must be 'none' when tools are provided for gpt-6")
	assert.Equal(t, "none", info.ReasoningEffort, "relayInfo.ReasoningEffort must also be updated to 'none'")
	assert.Nil(t, converted.Reasoning, "reasoning raw message must be nil")

	// 场景 2: 客户端未传 reasoning_effort，但传了 tools，默认也必须补上 "none"
	req2 := &dto.GeneralOpenAIRequest{
		Model: "gpt-6-astra",
		Tools: []dto.ToolCallRequest{
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name: "execute_query",
				},
			},
		},
	}
	res2, err := adaptor.ConvertOpenAIRequest(c, info, req2)
	require.NoError(t, err)
	converted2, ok := res2.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	assert.Equal(t, "none", converted2.ReasoningEffort, "reasoning_effort must be set to 'none' even if not passed initially")

	// 场景 3: 命名空间前缀 openai/gpt-6-astra 且带 functions
	info3 := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "openai/gpt-6-astra",
		},
	}
	req3 := &dto.GeneralOpenAIRequest{
		Model:     "openai/gpt-6-astra",
		Functions: []byte(`[{"name":"test_fn"}]`),
	}
	res3, err := adaptor.ConvertOpenAIRequest(c, info3, req3)
	require.NoError(t, err)
	converted3, ok := res3.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	assert.Equal(t, "none", converted3.ReasoningEffort, "reasoning_effort must be 'none' when legacy functions are provided")

	// 场景 4: 无 tools 时正常保留 reasoning_effort
	req4 := &dto.GeneralOpenAIRequest{
		Model:           "gpt-6-astra",
		ReasoningEffort: "high",
	}
	res4, err := adaptor.ConvertOpenAIRequest(c, info, req4)
	require.NoError(t, err)
	converted4, ok := res4.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	assert.Equal(t, "high", converted4.ReasoningEffort, "reasoning_effort should be preserved when no tools are present")
}


