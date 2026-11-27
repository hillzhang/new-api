package ali

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConvertOpenAIRequestFiltersThinkingBudgetByUpstreamModel(t *testing.T) {
	tests := []struct {
		name          string
		requestModel  string
		upstreamModel string
		budget        string
		wantBudget    bool
		wantValue     int64
	}{
		{
			name:          "qwen",
			requestModel:  "qwen-plus",
			upstreamModel: "qwen-plus",
			budget:        "128",
			wantBudget:    true,
			wantValue:     128,
		},
		{
			name:          "qwq explicit zero",
			requestModel:  "qwq-32b",
			upstreamModel: "qwq-32b",
			budget:        "0",
			wantBudget:    true,
			wantValue:     0,
		},
		{
			name:          "unsupported upstream overrides qwen request",
			requestModel:  "qwen-plus",
			upstreamModel: "deepseek-r1",
			budget:        "128",
			wantBudget:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &dto.GeneralOpenAIRequest{
				Model:          tt.requestModel,
				EnableThinking: json.RawMessage(`true`),
				ThinkingBudget: json.RawMessage(tt.budget),
			}
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: tt.upstreamModel,
				},
			}

			convertedValue, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)
			require.NoError(t, err)
			converted, ok := convertedValue.(*dto.GeneralOpenAIRequest)
			require.True(t, ok)

			if tt.wantBudget {
				assert.Equal(t, tt.budget, string(converted.ThinkingBudget))
			} else {
				assert.Nil(t, converted.ThinkingBudget)
			}

			encoded, err := common.Marshal(converted)
			require.NoError(t, err)

			assert.True(t, gjson.GetBytes(encoded, "enable_thinking").Bool())
			value := gjson.GetBytes(encoded, "thinking_budget")
			assert.Equal(t, tt.wantBudget, value.Exists())
			if tt.wantBudget {
				assert.Equal(t, tt.wantValue, value.Int())
			}
		})
	}
}

func TestConvertOpenAIRequestPreservesExplicitZeroForMappedQwenModel(t *testing.T) {
	const (
		clientModel   = "customer-model"
		upstreamModel = "Qwen/Qwen3-235B-A22B-Thinking-2507"
	)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("model_mapping", `{"customer-model":"Qwen/Qwen3-235B-A22B-Thinking-2507"}`)

	request := &dto.GeneralOpenAIRequest{
		Model:          clientModel,
		EnableThinking: json.RawMessage(`true`),
		ThinkingBudget: json.RawMessage(`0`),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: clientModel,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: clientModel,
		},
	}

	err := relayhelper.ModelMappedHelper(c, info, request)
	require.NoError(t, err)
	assert.True(t, info.IsModelMapped)
	assert.Equal(t, upstreamModel, info.UpstreamModelName)
	assert.Equal(t, upstreamModel, request.Model)

	convertedValue, err := (&Adaptor{}).ConvertOpenAIRequest(c, info, request)
	require.NoError(t, err)
	converted, ok := convertedValue.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	assert.Equal(t, json.RawMessage(`0`), converted.ThinkingBudget)

	encoded, err := common.Marshal(converted)
	require.NoError(t, err)

	value := gjson.GetBytes(encoded, "thinking_budget")
	assert.True(t, value.Exists())
	assert.Equal(t, int64(0), value.Int())
}

func TestMappedAliImageModelUsesUpstreamProtocol(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	info := &relaycommon.RelayInfo{
		RelayMode:       constant.RelayModeImagesGenerations,
		OriginModelName: "customer-image-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://dashscope.aliyuncs.com",
			UpstreamModelName: "qwen-image-3.0-pro",
		},
	}

	adaptor := &Adaptor{}
	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://dashscope.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation", url)

	header := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &header, info))
	assert.Empty(t, header.Get("X-DashScope-Async"))

	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  info.UpstreamModelName,
		Prompt: "poster",
	})
	require.NoError(t, err)
	assert.True(t, adaptor.IsSyncImageModel)
	assert.IsType(t, &AliImageRequest{}, converted)
}

func TestResponsesURLAndSearchOptions(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: constant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://dashscope.aliyuncs.com",
			UpstreamModelName: "qwen-plus",
		},
	}
	url, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://dashscope.aliyuncs.com/compatible-mode/v1/responses", url)

	req := &dto.GeneralOpenAIRequest{
		Model:         "qwen-plus-internet",
		SearchOptions: json.RawMessage(`{"forced_search":true,"search_strategy":"agent"}`),
	}
	info.UpstreamModelName = "qwen-plus-internet"
	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, req)
	require.NoError(t, err)
	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.Equal(t, "qwen-plus", gjson.GetBytes(encoded, "model").String())
	assert.True(t, gjson.GetBytes(encoded, "enable_search").Bool())
	assert.True(t, gjson.GetBytes(encoded, "search_options.forced_search").Bool())
	assert.Equal(t, "agent", gjson.GetBytes(encoded, "search_options.search_strategy").String())
}

func TestEmbeddingRequestByModel(t *testing.T) {
	const (
		base       = "https://dashscope.invalid"
		nativePath = "/api/v1/services/embeddings/multimodal-embedding/multimodal-embedding"
	)
	tests := []struct {
		model    string
		request  string
		wantPath string
		wantBody string
	}{
		{
			model:    "qwen3-vl-embedding",
			request:  `"input":"a cat"`,
			wantPath: nativePath,
			wantBody: `{"model":"qwen3-vl-embedding","input":{"contents":[{"text":"a cat"}]}}`,
		},
		{
			model:    "tongyi-embedding-vision-plus-2026-03-06",
			request:  `"input":["a cat","a dog"],"dimensions":1024`,
			wantPath: nativePath,
			wantBody: `{"model":"tongyi-embedding-vision-plus-2026-03-06","input":{"contents":[{"text":"a cat"},{"text":"a dog"}]},"parameters":{"dimension":1024}}`,
		},
		{
			model: "multimodal-embedding-v1",
			request: `"input":[{"type":"text","text":"a cat"},` +
				`{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}},` +
				`{"type":"video_url","video_url":{"url":"https://example.com/cat.mp4"}}]`,
			wantPath: nativePath,
			wantBody: `{"model":"multimodal-embedding-v1","input":{"contents":[{"text":"a cat"},` +
				`{"image":"data:image/png;base64,iVBORw0KGgo="},{"video":"https://example.com/cat.mp4"}]}}`,
		},
		{
			model:    "qwen3-vl-embedding",
			request:  `"input":[{"type":"image_url","image_url":{"url":""}}]`,
			wantPath: nativePath,
		},
		{
			model:    "text-embedding-v4",
			request:  `"input":"a cat","dimensions":1024`,
			wantPath: "/compatible-mode/v1/embeddings",
			wantBody: `{"model":"text-embedding-v4","input":"a cat","dimensions":1024}`,
		},
	}
	for _, tt := range tests {
		info := &relaycommon.RelayInfo{
			RelayMode:   constant.RelayModeEmbeddings,
			RelayFormat: types.RelayFormatEmbedding,
			ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: base, UpstreamModelName: tt.model},
		}
		url, err := (&Adaptor{}).GetRequestURL(info)
		require.NoError(t, err, tt.request)
		assert.Equal(t, base+tt.wantPath, url, tt.request)

		var request dto.EmbeddingRequest
		require.NoError(t, common.UnmarshalJsonStr(`{"model":"`+tt.model+`",`+tt.request+`}`, &request))
		converted, err := (&Adaptor{}).ConvertEmbeddingRequest(nil, info, request)
		if tt.wantBody == "" {
			assert.Error(t, err, tt.request)
			continue
		}
		require.NoError(t, err, tt.request)
		wire, err := common.Marshal(converted)
		require.NoError(t, err)
		assert.JSONEq(t, tt.wantBody, string(wire), tt.request)
	}
}

func TestMultimodalEmbeddingResponse(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		body      string
		estimate  int
		wantData  []dto.EmbeddingResponseItem
		wantUsage *dto.Usage
	}{
		{
			name:  "visual tokens reported apart",
			model: "qwen3-vl-embedding",
			body: `{"output":{"embeddings":[{"index":0,"embedding":[0.1,-0.2],"type":"text"},{"index":1,"embedding":[0.3,0.4],"type":"image"}]},` +
				`"usage":{"input_tokens":43,"image_tokens":1247,"total_tokens":1290},"request_id":"r1"}`,
			wantData: []dto.EmbeddingResponseItem{
				{Object: "embedding", Index: 0, Embedding: []float64{0.1, -0.2}},
				{Object: "embedding", Index: 1, Embedding: []float64{0.3, 0.4}},
			},
			wantUsage: &dto.Usage{PromptTokens: 1290, TotalTokens: 1290,
				PromptTokensDetails: dto.InputTokenDetails{TextTokens: 43, ImageTokens: 1247}},
		},
		{
			name:  "input_tokens split in input_tokens_details",
			model: "tongyi-embedding-vision-plus",
			body: `{"output":{"embeddings":[{"index":0,"embedding":[0.5],"type":"fused"}]},` +
				`"usage":{"input_tokens":903,"input_tokens_details":{"image_tokens":896,"text_tokens":7},"output_tokens":3,"total_tokens":906},"request_id":"r2"}`,
			wantData: []dto.EmbeddingResponseItem{{Object: "embedding", Index: 0, Embedding: []float64{0.5}}},
			wantUsage: &dto.Usage{PromptTokens: 903, TotalTokens: 903,
				PromptTokensDetails: dto.InputTokenDetails{TextTokens: 7, ImageTokens: 896}},
		},
		{
			name:      "no usage bills the estimate",
			model:     "qwen3-vl-embedding",
			body:      `{"output":{"embeddings":[{"index":0,"embedding":[0.5],"type":"text"}]},"request_id":"r3"}`,
			estimate:  12,
			wantData:  []dto.EmbeddingResponseItem{{Object: "embedding", Index: 0, Embedding: []float64{0.5}}},
			wantUsage: &dto.Usage{PromptTokens: 12, TotalTokens: 12},
		},
		{
			name:  "error code in a 200 body",
			model: "qwen3-vl-embedding",
			body:  `{"code":"InvalidParameter","message":"The input is invalid.","request_id":"r4"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			info := &relaycommon.RelayInfo{
				RelayMode:   constant.RelayModeEmbeddings,
				RelayFormat: types.RelayFormatEmbedding,
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tt.model},
			}
			info.SetEstimatePromptTokens(tt.estimate)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tt.body))}

			usage, apiErr := (&Adaptor{}).DoResponse(c, resp, info)
			if tt.wantUsage == nil {
				require.NotNil(t, apiErr)
				assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
				assert.Equal(t, "InvalidParameter", string(apiErr.GetErrorCode()))
				assert.Empty(t, w.Body.String())
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, tt.wantUsage, usage)

			var got dto.EmbeddingResponse
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &got))
			assert.Equal(t, dto.EmbeddingResponse{Object: "list", Data: tt.wantData, Model: tt.model, Usage: *tt.wantUsage}, got)
		})
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		rawBaseURL string
		wantURL    string
	}{
		{"https://dashscope.aliyuncs.com", "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"},
		{"https://dashscope.aliyuncs.com/", "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"},
		{"https://dashscope.aliyuncs.com/compatible-mode/v1", "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"},
		{"https://dashscope.aliyuncs.com/compatible-mode/v1/", "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"},
		{"https://ws123.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", "https://ws123.cn-beijing.maas.aliyuncs.com/compatible-mode/v1/chat/completions"},
	}

	for _, tt := range tests {
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl:    tt.rawBaseURL,
				UpstreamModelName: "qwen-plus",
			},
		}
		got, err := (&Adaptor{}).GetRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, tt.wantURL, got)
	}
}
