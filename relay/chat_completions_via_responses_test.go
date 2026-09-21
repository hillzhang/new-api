package relay

import (
	"math"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsResponsesEventStreamContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "plain", contentType: "text/event-stream", want: true},
		{name: "mixed case with charset", contentType: "Text/Event-Stream; charset=utf-8", want: true},
		{name: "json", contentType: "application/json", want: false},
		{name: "empty", contentType: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isResponsesEventStreamContentType(tt.contentType))
		})
	}
}

func TestRecalcQuotaFromRatiosIgnoresInvalidMultipliers(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota: 100,
		},
	}
	info.PriceData.AddOtherRatio("duration", 2)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{
		"duration": 3,
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	})

	require.True(t, ok)
	assert.Equal(t, 150, quota)
	assert.True(t, info.PriceData.HasOtherRatio("duration"))
}

func TestRecalcQuotaFromRatiosRejectsAllInvalidAdjustedRatios(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota: 100,
		},
	}
	info.PriceData.AddOtherRatio("duration", 2)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	})

	require.False(t, ok)
	assert.Equal(t, 0, quota)
	assert.True(t, info.PriceData.HasOtherRatio("duration"))
}

func TestShouldUseResponsesForGPT6Tools(t *testing.T) {
	// 场景 1: OpenAI 渠道, gpt-6-astra, 带有 tools -> 必须为 true (走 responses)
	assert.True(t, ShouldUseResponsesForGPT6Tools(1, "gpt-6-astra", "gpt-6-astra", true))

	// 场景 2: OpenAI 渠道, gpt-6-astra, 不带 tools -> 为 false (走正常 chat/completions)
	assert.False(t, ShouldUseResponsesForGPT6Tools(1, "gpt-6-astra", "gpt-6-astra", false))

	// 场景 3: Azure 渠道, openai/gpt-6-astra, 带有 tools -> 为 true
	assert.True(t, ShouldUseResponsesForGPT6Tools(3, "openai/gpt-6-astra", "gpt-6-astra", true))

	// 场景 4: 其它模型如 gpt-4o, 即使带 tools 也不强制走 responses
	assert.False(t, ShouldUseResponsesForGPT6Tools(1, "gpt-4o", "gpt-4o", true))

	// 场景 5: 非 OpenAI/Azure 渠道 (如 Anthropic 渠道), 即使带 tools 也不走 responses
	assert.False(t, ShouldUseResponsesForGPT6Tools(14, "gpt-6-astra", "gpt-6-astra", true))
}
