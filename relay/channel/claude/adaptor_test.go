package claude

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestGetRequestURL_SmartRouting(t *testing.T) {
	adaptor := &Adaptor{}

	tests := []struct {
		name        string
		baseURL     string
		expectedURL string
	}{
		{
			name:        "官方标准 Anthropic 地址（无结尾斜杠）",
			baseURL:     "https://api.anthropic.com",
			expectedURL: "https://api.anthropic.com/v1/messages",
		},
		{
			name:        "官方标准 Anthropic 地址（带结尾斜杠）",
			baseURL:     "https://api.anthropic.com/",
			expectedURL: "https://api.anthropic.com/v1/messages",
		},
		{
			name:        "网宿智算网关 BaseURL 前缀",
			baseURL:     "https://api.model-store.ai/v2/llm/anthropic",
			expectedURL: "https://api.model-store.ai/v2/llm/anthropic/messages",
		},
		{
			name:        "网宿智算网关 BaseURL 前缀（带斜杠）",
			baseURL:     "https://api.model-store.ai/v2/llm/anthropic/",
			expectedURL: "https://api.model-store.ai/v2/llm/anthropic/messages",
		},
		{
			name:        "网宿统一网关 BaseURL 前缀（/v2/llm）",
			baseURL:     "https://api.model-store.ai/v2/llm",
			expectedURL: "https://api.model-store.ai/v2/llm/anthropic/messages",
		},
		{
			name:        "自建反代 BaseURL 前缀（/v1）",
			baseURL:     "https://custom-proxy.com/v1",
			expectedURL: "https://custom-proxy.com/v1/messages",
		},
		{
			name:        "用户直接配置完整 /messages 端点",
			baseURL:     "https://custom-gateway.com/v2/llm/anthropic/messages",
			expectedURL: "https://custom-gateway.com/v2/llm/anthropic/messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl: tt.baseURL,
				},
			}
			actualURL, err := adaptor.GetRequestURL(info)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedURL, actualURL)
		})
	}
}
