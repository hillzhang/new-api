package hailuo

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func init() {
	gin.SetMode(gin.TestMode)
	ratio_setting.InitRatioSettings()
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-4
}

func TestEstimateBilling_BasePriceAndResolution(t *testing.T) {
	adaptor := &TaskAdaptor{}

	tests := []struct {
		name             string
		model            string
		duration         int
		size             string
		metadata         map[string]any
		wantSeconds      float64
		wantResRatio     float64
		expectedBaseCost float64 // ModelPrice * seconds * resRatio (元)
	}{
		{
			name:             "MiniMax-H3 default 5s 768P",
			model:            ModelH3,
			duration:         5,
			size:             "768P",
			wantSeconds:      5.0,
			wantResRatio:     1.0,
			expectedBaseCost: 0.50 * 5.0 * 1.0, // 2.50 元
		},
		{
			name:             "MiniMax-H3 upgrade 2K 5s",
			model:            ModelH3,
			duration:         5,
			size:             "2048x1080",
			wantSeconds:      5.0,
			wantResRatio:     1.60,
			expectedBaseCost: 0.50 * 5.0 * 1.60, // 4.00 元
		},
		{
			name:             "MiniMax-H3-Max default 5s 480P",
			model:            ModelH3Max,
			duration:         5,
			size:             "480P",
			wantSeconds:      5.0,
			wantResRatio:     1.0,
			expectedBaseCost: 0.33 * 5.0 * 1.0, // 1.65 元
		},
		{
			name:             "MiniMax-H3-Max upgrade 768P 5s",
			model:            ModelH3Max,
			duration:         5,
			size:             "1280x768",
			wantSeconds:      5.0,
			wantResRatio:     0.50 / 0.33,
			expectedBaseCost: 0.33 * 5.0 * (0.50 / 0.33), // 2.50 元
		},
		{
			name:             "MiniMax-H3-Regeneration 5s 2K",
			model:            ModelH3Regeneration,
			duration:         5,
			size:             "2K",
			wantSeconds:      5.0,
			wantResRatio:     1.0,
			expectedBaseCost: 0.30 * 5.0 * 1.0, // 1.50 元
		},
		{
			name:             "MiniMax-H3 Auto-Detect Regeneration via source_task_id (0.30/0.50=0.60)",
			model:            ModelH3,
			duration:         5,
			metadata:         map[string]any{"source_task_id": "424010985738629"},
			wantSeconds:      5.0,
			wantResRatio:     0.30 / 0.50,         // 0.60 折扣
			expectedBaseCost: 0.50 * 5.0 * 0.60,   // 1.50 元 (精准对齐 0.30 元/秒)
		},
		{
			name:             "MiniMax-H3-Max 10s 480P",
			model:            ModelH3Max,
			duration:         10,
			size:             "480P",
			wantSeconds:      10.0,
			wantResRatio:     1.0,
			expectedBaseCost: 0.33 * 10.0 * 1.0, // 3.30 元
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			req := relaycommon.TaskSubmitReq{
				Duration: tt.duration,
				Size:     tt.size,
				Metadata: tt.metadata,
			}
			c.Set("task_request", req)

			info := &relaycommon.RelayInfo{
				OriginModelName: tt.model,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: tt.model,
				},
			}

			ratios := adaptor.EstimateBilling(c, info)
			assert.NotNil(t, ratios)
			assert.Equal(t, tt.wantSeconds, ratios["seconds"])

			resRatio := ratios["resolution"]
			if tt.wantResRatio == 1.0 {
				assert.Equal(t, 0.0, resRatio) // ratio == 1.0 is omitted
			} else {
				assert.True(t, almostEqual(tt.wantResRatio, resRatio), "want %f got %f", tt.wantResRatio, resRatio)
			}
		})
	}
}

func TestEstimateBilling_ExtraImages(t *testing.T) {
	adaptor := &TaskAdaptor{}

	t.Run("MiniMax-H3 with 7 images (2 extra, default 0.20 each)", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		images := make([]string, 7)
		req := relaycommon.TaskSubmitReq{
			Duration: 5,
			Size:     "768P",
			Images:   images,
		}
		c.Set("task_request", req)

		info := &relaycommon.RelayInfo{
			OriginModelName: ModelH3,
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		ratios := adaptor.EstimateBilling(c, info)
		assert.NotNil(t, ratios)
		assert.Equal(t, 5.0, ratios["seconds"])

		// Base video cost: 0.50 * 5 = 2.50 元
		// Extra images: (7 - 5) * 0.20 = 0.40 元
		// extra_assets = 1.0 + 0.40 / 2.50 = 1.16
		// Total cost: 0.50 * 5.0 * 1.16 = 2.90 元 (2.50 + 0.40)
		assert.True(t, almostEqual(1.16, ratios["extra_assets"]))
	})

	t.Run("MiniMax-H3-Max with 4 images (2 extra, default 0.50 each)", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		images := make([]string, 4)
		req := relaycommon.TaskSubmitReq{
			Duration: 5,
			Size:     "480P",
			Images:   images,
		}
		c.Set("task_request", req)

		info := &relaycommon.RelayInfo{
			OriginModelName: ModelH3Max,
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3Max,
			},
		}

		ratios := adaptor.EstimateBilling(c, info)
		assert.NotNil(t, ratios)
		assert.Equal(t, 5.0, ratios["seconds"])

		// Base video cost: 0.33 * 5 = 1.65 元
		// Extra images: (4 - 2) * 0.50 = 1.00 元
		// extra_assets = 1.0 + 1.00 / 1.65 = 1.60606
		// Total cost: 0.33 * 5.0 * 1.60606 = 2.65 元 (1.65 + 1.00)
		wantExtraRatio := 1.0 + 1.00/1.65
		assert.True(t, almostEqual(wantExtraRatio, ratios["extra_assets"]))
	})

	t.Run("MiniMax-H3-Regeneration with 7 images (2 extra, default 0.15 each)", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		images := make([]string, 7)
		req := relaycommon.TaskSubmitReq{
			Duration: 5,
			Size:     "2K",
			Images:   images,
		}
		c.Set("task_request", req)

		info := &relaycommon.RelayInfo{
			OriginModelName: ModelH3Regeneration,
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3Regeneration,
			},
		}

		ratios := adaptor.EstimateBilling(c, info)
		assert.NotNil(t, ratios)
		assert.Equal(t, 5.0, ratios["seconds"])

		// Base video cost: 0.30 * 5 = 1.50 元
		// Extra images: (7 - 5) * 0.15 = 0.30 元
		// extra_assets = 1.0 + 0.30 / 1.50 = 1.20
		// Total cost: 0.30 * 5.0 * 1.20 = 1.80 元 (1.50 + 0.30)
		assert.True(t, almostEqual(1.20, ratios["extra_assets"]))
	})

	t.Run("Customized image price override", func(t *testing.T) {
		// Simulate admin overriding MiniMax-H3-image to 0.10 RMB/image
		_ = ratio_setting.UpdateModelPriceByJSONString(`{"MiniMax-H3-image": 0.10}`)
		defer func() {
			_ = ratio_setting.UpdateModelPriceByJSONString(`{"MiniMax-H3-image": 0.20}`)
		}()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		images := make([]string, 7) // 2 extra images
		req := relaycommon.TaskSubmitReq{
			Duration: 5,
			Size:     "768P",
			Images:   images,
		}
		c.Set("task_request", req)

		info := &relaycommon.RelayInfo{
			OriginModelName: ModelH3,
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		ratios := adaptor.EstimateBilling(c, info)
		assert.NotNil(t, ratios)
		// Base video cost: 0.50 * 5 = 2.50 元
		// Extra images at discounted rate: 2 * 0.10 = 0.20 元
		// extra_assets = 1.0 + 0.20 / 2.50 = 1.08
		assert.True(t, almostEqual(1.08, ratios["extra_assets"]))
	})
}

func TestEstimateBilling_InputVideoReference(t *testing.T) {
	adaptor := &TaskAdaptor{}

	t.Run("MiniMax-H3 generating 768P: input video 0.50/s", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := relaycommon.TaskSubmitReq{
			Duration: 5,
			Size:     "768P",
			Metadata: map[string]any{
				"video_seconds": 3, // 3s reference video
			},
		}
		c.Set("task_request", req)

		info := &relaycommon.RelayInfo{
			OriginModelName: ModelH3,
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		ratios := adaptor.EstimateBilling(c, info)
		assert.NotNil(t, ratios)
		assert.Equal(t, 5.0, ratios["seconds"])

		// Base video cost: 0.50 * 5 = 2.50 元
		// Input video cost (768P): 3 * 0.50 = 1.50 元
		// extra_assets = 1.0 + 1.50 / 2.50 = 1.60
		assert.True(t, almostEqual(1.60, ratios["extra_assets"]))
	})

	t.Run("MiniMax-H3 generating 2K: input video 0.80/s", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := relaycommon.TaskSubmitReq{
			Duration: 5,
			Size:     "2K",
			Metadata: map[string]any{
				"video_seconds": 3, // 3s reference video
			},
		}
		c.Set("task_request", req)

		info := &relaycommon.RelayInfo{
			OriginModelName: ModelH3,
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		ratios := adaptor.EstimateBilling(c, info)
		assert.NotNil(t, ratios)
		assert.Equal(t, 5.0, ratios["seconds"])
		assert.Equal(t, 1.60, ratios["resolution"]) // 2K ratio = 1.60

		// Base video cost: 0.50 * 5 * 1.60 = 4.00 元
		// Input video cost (2K): 3 * 0.80 = 2.40 元
		// extra_assets = 1.0 + 2.40 / 4.00 = 1.60
		assert.True(t, almostEqual(1.60, ratios["extra_assets"]))
	})

	t.Run("MiniMax-H3-Max generating 480P: input video 0.37/s", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := relaycommon.TaskSubmitReq{
			Duration: 5,
			Size:     "480P",
			Metadata: map[string]any{
				"video_seconds": 3, // 3s reference video
			},
		}
		c.Set("task_request", req)

		info := &relaycommon.RelayInfo{
			OriginModelName: ModelH3Max,
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3Max,
			},
		}

		ratios := adaptor.EstimateBilling(c, info)
		assert.NotNil(t, ratios)
		assert.Equal(t, 5.0, ratios["seconds"])

		// Base video cost: 0.33 * 5 = 1.65 元
		// Input video cost (480P): 3 * 0.37 = 1.11 元
		// extra_assets = 1.0 + 1.11 / 1.65 = 1.672727
		wantExtraRatio := 1.0 + 1.11/1.65
		assert.True(t, almostEqual(wantExtraRatio, ratios["extra_assets"]))
	})

	t.Run("MiniMax-H3-Max generating 768P: input video 0.97/s", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := relaycommon.TaskSubmitReq{
			Duration: 5,
			Size:     "768P",
			Metadata: map[string]any{
				"video_seconds": 3, // 3s reference video
			},
		}
		c.Set("task_request", req)

		info := &relaycommon.RelayInfo{
			OriginModelName: ModelH3Max,
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3Max,
			},
		}

		ratios := adaptor.EstimateBilling(c, info)
		assert.NotNil(t, ratios)
		assert.Equal(t, 5.0, ratios["seconds"])

		// Base video cost: 0.33 * 5 * (0.50 / 0.33) = 2.50 元
		// Input video cost (768P): 3 * 0.97 = 2.91 元
		// extra_assets = 1.0 + 2.91 / 2.50 = 2.164
		wantExtraRatio := 1.0 + 2.91/2.50
		assert.True(t, almostEqual(wantExtraRatio, ratios["extra_assets"]))
	})
}

func TestEstimateBilling_LegacyModelFallback(t *testing.T) {
	adaptor := &TaskAdaptor{}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := relaycommon.TaskSubmitReq{
		Duration: 6,
		Size:     "720P",
	}
	c.Set("task_request", req)

	info := &relaycommon.RelayInfo{
		OriginModelName: "MiniMax-Hailuo-2.3",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "MiniMax-Hailuo-2.3",
		},
	}

	ratios := adaptor.EstimateBilling(c, info)
	assert.Nil(t, ratios, "Legacy models should return nil to preserve existing BaseBilling behavior")
}

func TestModelConfig_H3Specifications(t *testing.T) {
	// 验证 H3 模型配置
	h3Config := GetModelConfig(ModelH3)
	assert.Equal(t, Resolution768P, h3Config.DefaultResolution)
	assert.Equal(t, []string{Resolution768P, Resolution2K}, h3Config.SupportedResolutions)
	assert.Equal(t, []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, h3Config.SupportedDurations)

	// 验证 H3-Max 模型配置
	h3MaxConfig := GetModelConfig(ModelH3Max)
	assert.Equal(t, Resolution480P, h3MaxConfig.DefaultResolution)
	assert.Equal(t, []string{Resolution480P, Resolution768P}, h3MaxConfig.SupportedResolutions)
	assert.Equal(t, []int{5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, h3MaxConfig.SupportedDurations)

	// 验证 H3-Regeneration 模型配置
	regenConfig := GetModelConfig(ModelH3Regeneration)
	assert.Equal(t, Resolution2K, regenConfig.DefaultResolution)
	assert.Equal(t, []string{Resolution2K}, regenConfig.SupportedResolutions)
}

func TestConvertToV2RequestPayload(t *testing.T) {
	adaptor := &TaskAdaptor{}

	t.Run("Text to Video (t2va) auto sets ratio 16:9", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Prompt:   "A beautiful mountain sunrise",
			Duration: 5,
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, ModelH3, v2Req.Model)
		assert.Equal(t, 5, v2Req.Duration)
		assert.Equal(t, Resolution768P, v2Req.Resolution)
		assert.Equal(t, "16:9", v2Req.Ratio)
		assert.Len(t, v2Req.Content, 1)
		assert.Equal(t, "text", v2Req.Content[0].Type)
		assert.Equal(t, "A beautiful mountain sunrise", v2Req.Content[0].Text)
	})

	t.Run("Image to Video (i2va) with first and last frame sets ratio adaptive", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Prompt: "A car speeding on highway",
			Images: []string{"https://img.com/first.png", "https://img.com/last.png"},
			Size:   "2K",
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, Resolution2K, v2Req.Resolution)
		assert.Equal(t, "adaptive", v2Req.Ratio)
		assert.Len(t, v2Req.Content, 3) // text + first_frame + last_frame
		assert.Equal(t, "first_frame", v2Req.Content[1].Role)
		assert.Equal(t, "https://img.com/first.png", v2Req.Content[1].ImageURL.URL)
		assert.Equal(t, "last_frame", v2Req.Content[2].Role)
		assert.Equal(t, "https://img.com/last.png", v2Req.Content[2].ImageURL.URL)
	})

	t.Run("H3-Max duration clamp and resolution enforcement", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Prompt:   "fast car",
			Duration: 3, // below min 5
			Size:     "1080P", // unsupported for Max, should fallback to 480P
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3Max,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, 5, v2Req.Duration, "H3-Max duration should be clamped to at least 5")
		assert.Equal(t, Resolution480P, v2Req.Resolution, "H3-Max resolution should fallback to 480P")
	})

	t.Run("Multi-reference images and videos payload assembly", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Prompt: "Character performing action based on references",
			Metadata: map[string]any{
				"reference_images": []any{
					"https://img.com/ref1.png",
					"https://img.com/ref2.png",
					"https://img.com/ref3.png",
				},
				"reference_videos": []any{
					"https://vid.com/ref1.mp4",
					"https://vid.com/ref2.mp4",
				},
			},
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		// 1 text + 3 reference images + 2 reference videos = 6 content items
		assert.Len(t, v2Req.Content, 6)
		assert.Equal(t, "text", v2Req.Content[0].Type)

		imageRefCount := 0
		videoRefCount := 0
		for _, item := range v2Req.Content {
			if item.Role == "reference_image" {
				assert.Equal(t, "image_url", item.Type)
				assert.NotEmpty(t, item.ImageURL.URL)
				imageRefCount++
			}
			if item.Role == "reference_video" {
				assert.Equal(t, "video_url", item.Type)
				assert.NotEmpty(t, item.VideoURL.URL)
				videoRefCount++
			}
		}
		assert.Equal(t, 3, imageRefCount)
		assert.Equal(t, 2, videoRefCount)
	})

	t.Run("H3-Regeneration with source_task_id", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Metadata: map[string]any{
				"source_task_id": "424010985738629",
			},
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3Regeneration,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, ModelH3, v2Req.Model)
		assert.Equal(t, "424010985738629", v2Req.SourceTaskID)
		assert.Equal(t, Resolution2K, v2Req.Resolution)
		assert.Empty(t, v2Req.Content)
	})

	t.Run("H3-Regeneration with reference video mapped to base_video", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Metadata: map[string]any{
				"reference_video": "https://vid.com/source_768p.mp4",
			},
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3Regeneration,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, ModelH3, v2Req.Model)
		assert.Equal(t, Resolution2K, v2Req.Resolution)
		hasBaseVideo := false
		for _, item := range v2Req.Content {
			if item.Role == "base_video" {
				hasBaseVideo = true
				assert.Equal(t, "https://vid.com/source_768p.mp4", item.VideoURL.URL)
			}
		}
		assert.True(t, hasBaseVideo)
	})

	t.Run("H3-Regeneration metadata base_video and omitempty duration/ratio", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Prompt:   "Original prompt used during 768P generation",
			Duration: 5,
			Size:     "16:9",
			Metadata: map[string]any{
				"base_video": "https://example.com/h3_768p.mp4",
			},
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3Regeneration,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, ModelH3, v2Req.Model)
		assert.Equal(t, Resolution2K, v2Req.Resolution)
		assert.Equal(t, 0, v2Req.Duration, "Regeneration duration must be 0 to trigger omitempty")
		assert.Equal(t, "", v2Req.Ratio, "Regeneration ratio must be empty string to trigger omitempty")

		// Verify JSON serialization does not contain duration or ratio
		data, err := json.Marshal(v2Req)
		assert.NoError(t, err)
		jsonStr := string(data)
		assert.NotContains(t, jsonStr, `"duration"`)
		assert.NotContains(t, jsonStr, `"ratio"`)
		assert.Contains(t, jsonStr, `"base_video"`)
		assert.Contains(t, jsonStr, `"Original prompt used during 768P generation"`)
	})

	t.Run("H3-Regeneration metadata source_task_id does not contain duration/ratio/content", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Metadata: map[string]any{
				"source_task_id": "424010985738629",
			},
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3Regeneration,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, ModelH3, v2Req.Model)
		assert.Equal(t, "424010985738629", v2Req.SourceTaskID)
		assert.Equal(t, Resolution2K, v2Req.Resolution)

		data, err := json.Marshal(v2Req)
		assert.NoError(t, err)
		jsonStr := string(data)
		assert.NotContains(t, jsonStr, `"duration"`)
		assert.NotContains(t, jsonStr, `"ratio"`)
		assert.NotContains(t, jsonStr, `"content"`)
		assert.Contains(t, jsonStr, `"source_task_id":"424010985738629"`)
		assert.Contains(t, jsonStr, `"resolution":"2K"`)
	})

	t.Run("MiniMax-H3 native model with source_task_id auto-detected as regeneration", func(t *testing.T) {
		adaptor := &TaskAdaptor{
			isRegen: true,
		}
		req := &relaycommon.TaskSubmitReq{
			Model: ModelH3,
			Metadata: map[string]any{
				"source_task_id": "424010985738629",
			},
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, ModelH3, v2Req.Model)
		assert.Equal(t, "424010985738629", v2Req.SourceTaskID)
		assert.Equal(t, Resolution2K, v2Req.Resolution)
		assert.Equal(t, 0, v2Req.Duration)
		assert.Equal(t, "", v2Req.Ratio)

		u, err := adaptor.BuildRequestURL(info)
		assert.NoError(t, err)
		assert.Contains(t, u, VideoRegenerationEndpoint)
	})

	t.Run("H3-Max prompt_expansion_mode in metadata", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Prompt: "A cyberpunk city street at night",
			Metadata: map[string]any{
				"prompt_expansion_mode": "quality",
			},
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3Max,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, ModelH3Max, v2Req.Model)
		assert.Equal(t, map[string]any{"prompt_expansion_mode": "quality"}, v2Req.Extra)
	})

	t.Run("Pure text video ignores accidental adaptive ratio and forces 16:9", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Prompt: "A serene lake surrounded by pine trees",
			Metadata: map[string]any{
				"ratio": "adaptive",
			},
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, "16:9", v2Req.Ratio, "pure text video must not be adaptive, must fallback to 16:9")
	})

	t.Run("Multi-modal reference video defaults to adaptive ratio", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Prompt: "Follow the camera movement",
			Metadata: map[string]any{
				"reference_video": "https://example.com/ref.mp4",
			},
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, "adaptive", v2Req.Ratio, "reference video without explicit ratio should default to adaptive")
	})

	t.Run("Pixel dimensions like 768x768 and 768×768 are parsed to 768P and 1:1", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Prompt: "A cat playing piano",
			Size:   "768×768", // Unicode math multiplication symbol
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, Resolution768P, v2Req.Resolution)
		assert.Equal(t, "1:1", v2Req.Ratio)
	})

	t.Run("Pixel dimensions 1344x768 parsed to 768P and 16:9", func(t *testing.T) {
		req := &relaycommon.TaskSubmitReq{
			Prompt: "Cinematic landscape",
			Size:   "1344x768",
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		v2Req, err := adaptor.convertToV2RequestPayload(req, info)
		assert.NoError(t, err)
		assert.Equal(t, Resolution768P, v2Req.Resolution)
		assert.Equal(t, "16:9", v2Req.Ratio)
	})
}

func TestBuildRequestURL(t *testing.T) {
	adaptor := &TaskAdaptor{
		baseURL: "https://api.minimax.cn",
	}

	tests := []struct {
		model   string
		wantURL string
	}{
		{model: ModelH3, wantURL: "https://api.minimax.cn/v2/video_generation"},
		{model: ModelH3Max, wantURL: "https://api.minimax.cn/v2/video_generation"},
		{model: ModelH3Regeneration, wantURL: "https://api.minimax.cn/v2/video_regeneration"},
		{model: "MiniMax-Hailuo-2.3", wantURL: "https://api.minimax.cn/v1/video_generation"},
	}

	for _, tt := range tests {
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: tt.model,
			},
		}
		u, err := adaptor.BuildRequestURL(info)
		assert.NoError(t, err)
		assert.Equal(t, tt.wantURL, u)
	}
}

func TestParseTaskResult_V2(t *testing.T) {
	adaptor := &TaskAdaptor{}

	t.Run("V2 succeeded returns direct URL", func(t *testing.T) {
		v2SuccessResp := []byte(`{
			"task": {
				"id": "424010985738629",
				"model": "MiniMax-H3",
				"status": "succeeded",
				"content": {
					"url": "https://cdn.example.com/h3_video.mp4"
				},
				"resolution": "2K",
				"duration": 5
			}
		}`)

		res, err := adaptor.ParseTaskResult(v2SuccessResp)
		assert.NoError(t, err)
		assert.Equal(t, "SUCCESS", string(res.Status))
		assert.Equal(t, "100%", res.Progress)
		assert.Equal(t, "https://cdn.example.com/h3_video.mp4", res.Url)
	})

	t.Run("V2 failed returns error message", func(t *testing.T) {
		v2FailResp := []byte(`{
			"task": {
				"id": "424010985738630",
				"model": "MiniMax-H3",
				"status": "failed",
				"error": {
					"code": "1026",
					"message": "sensitive content detected"
				}
			}
		}`)

		res, err := adaptor.ParseTaskResult(v2FailResp)
		assert.NoError(t, err)
		assert.Equal(t, "FAILURE", string(res.Status))
		assert.Equal(t, "sensitive content detected", res.Reason)
	})
}

func TestValidateRequestAndSetAction_AutoRegen(t *testing.T) {
	adaptor := &TaskAdaptor{}

	t.Run("top-level source_task_id without prompt succeeds and marks isRegen", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		bodyJSON := `{"model":"MiniMax-H3","source_task_id":"424010985738629"}`
		c.Request = httptest.NewRequest("POST", "/v1/video/generations", bytes.NewBufferString(bodyJSON))
		c.Request.Header.Set("Content-Type", "application/json")

		info := &relaycommon.RelayInfo{
			OriginModelName: ModelH3,
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		taskErr := adaptor.ValidateRequestAndSetAction(c, info)
		assert.Nil(t, taskErr)
		assert.True(t, adaptor.isRegen)

		req, err := relaycommon.GetTaskRequest(c)
		assert.NoError(t, err)
		assert.Equal(t, "424010985738629", req.Metadata["source_task_id"])
	})

	t.Run("metadata source_task_id with prompt succeeds and marks isRegen", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		bodyJSON := `{"model":"MiniMax-H3","prompt":"any prompt","metadata":{"source_task_id":"424010985738629"}}`
		c.Request = httptest.NewRequest("POST", "/v1/video/generations", bytes.NewBufferString(bodyJSON))
		c.Request.Header.Set("Content-Type", "application/json")

		info := &relaycommon.RelayInfo{
			OriginModelName: ModelH3,
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		taskErr := adaptor.ValidateRequestAndSetAction(c, info)
		assert.Nil(t, taskErr)
		assert.True(t, adaptor.isRegen)

		req, err := relaycommon.GetTaskRequest(c)
		assert.NoError(t, err)
		assert.Equal(t, "424010985738629", req.Metadata["source_task_id"])
	})

	t.Run("standard MiniMax-H3 text-to-video request marks isRegen false", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		bodyJSON := `{"model":"MiniMax-H3","prompt":"a running puppy in the snow","size":"768P"}`
		c.Request = httptest.NewRequest("POST", "/v1/video/generations", bytes.NewBufferString(bodyJSON))
		c.Request.Header.Set("Content-Type", "application/json")

		info := &relaycommon.RelayInfo{
			OriginModelName: ModelH3,
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: ModelH3,
			},
		}

		taskErr := adaptor.ValidateRequestAndSetAction(c, info)
		assert.Nil(t, taskErr)
		assert.False(t, adaptor.isRegen)
	})
}

func TestConvertToOpenAIVideo_V2_Usage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	originTask := &model.Task{
		TaskID: "task_h3_test_1",
		Status: model.TaskStatusSuccess,
		Data: []byte(`{
			"task": {
				"id": "424010985738629",
				"model": "MiniMax-H3",
				"status": "succeeded",
				"content": {
					"url": "https://cdn.example.com/h3.mp4"
				},
				"resolution": "2K",
				"duration": 5,
				"usage": {
					"total_seconds": 5,
					"input_seconds": 3,
					"output_seconds": 5,
					"input_image_count": 1,
					"input_audio_seconds": 6
				}
			}
		}`),
	}

	bytesData, err := adaptor.ConvertToOpenAIVideo(originTask)
	assert.NoError(t, err)

	var video dto.OpenAIVideo
	err = common.Unmarshal(bytesData, &video)
	assert.NoError(t, err)
	assert.Equal(t, "task_h3_test_1", video.ID)
	assert.NotNil(t, video.Metadata["usage"])

	usageMap, ok := video.Metadata["usage"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, float64(3), usageMap["input_seconds"])
	assert.Equal(t, float64(5), usageMap["output_seconds"])
	assert.Equal(t, float64(1), usageMap["input_image_count"])
}

