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

func TestConvertOpenAIRequest_GPT6_PreservesReasoningEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-6-astra",
		},
	}

	// 场景 1: gpt-6-astra 正常保留用户的 reasoning_effort（如 high），不能被设为 none
	req := &dto.GeneralOpenAIRequest{
		Model:           "gpt-6-astra",
		ReasoningEffort: "high",
		Temperature:     lo.ToPtr(0.7),
	}

	res, err := adaptor.ConvertOpenAIRequest(c, info, req)
	require.NoError(t, err)

	converted, ok := res.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)

	assert.Equal(t, "high", converted.ReasoningEffort, "gpt-6 does not support none, must preserve valid effort")
	assert.Nil(t, converted.Temperature, "temperature must be nil for gpt-6")

	// 场景 2: 模型后缀 gpt-6-astra-medium 自动转为 reasoning_effort
	info2 := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-6-astra-medium",
		},
	}
	req2 := &dto.GeneralOpenAIRequest{
		Model: "gpt-6-astra-medium",
	}
	res2, err := adaptor.ConvertOpenAIRequest(c, info2, req2)
	require.NoError(t, err)
	converted2, ok := res2.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	assert.Equal(t, "medium", converted2.ReasoningEffort)
	assert.Equal(t, "gpt-6-astra", converted2.Model)
}

func TestConvertOpenAIResponsesRequest_GPT6_SanitizesTemperatureAndTopP(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-6-astra",
		},
	}

	req := dto.OpenAIResponsesRequest{
		Model:       "gpt-6-astra",
		Temperature: lo.ToPtr(0.8),
		TopP:        lo.ToPtr(0.95),
		Reasoning: &dto.Reasoning{
			Effort: "high",
		},
	}

	res, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, req)
	require.NoError(t, err)

	converted, ok := res.(dto.OpenAIResponsesRequest)
	require.True(t, ok)

	assert.Nil(t, converted.Temperature, "temperature must be nil in responses for gpt-6")
	assert.Nil(t, converted.TopP, "top_p must be nil in responses for gpt-6")
	require.NotNil(t, converted.Reasoning)
	assert.Equal(t, "high", converted.Reasoning.Effort)
}


