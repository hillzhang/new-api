package hailuo

import "strings"

const (
	ChannelName = "hailuo-video"
)

var ModelList = []string{
	"MiniMax-Hailuo-2.3",
	"MiniMax-Hailuo-2.3-Fast",
	"MiniMax-Hailuo-02",
	"T2V-01-Director",
	"T2V-01",
	"I2V-01-Director",
	"I2V-01-live",
	"I2V-01",
	"S2V-01",
	"MiniMax-H3",
	"MiniMax-H3-Max",
	"MiniMax-H3-Regeneration",
}

const (
	ModelH3             = "MiniMax-H3"
	ModelH3Max          = "MiniMax-H3-Max"
	ModelH3Regeneration = "MiniMax-H3-Regeneration"
)

const (
	TextToVideoEndpoint       = "/v1/video_generation"
	QueryTaskEndpoint         = "/v1/query/video_generation"
	TextToVideoV2Endpoint     = "/v2/video_generation"
	VideoRegenerationEndpoint = "/v2/video_regeneration"
	QueryTaskV2Endpoint       = "/v2/query/video_generation"
)

const (
	StatusSuccess    = 0
	StatusRateLimit  = 1002
	StatusAuthFailed = 1004
	StatusNoBalance  = 1008
	StatusSensitive  = 1026
	StatusParamError = 2013
	StatusInvalidKey = 2049
)

const (
	TaskStatusPreparing  = "Preparing"
	TaskStatusQueueing   = "Queueing"
	TaskStatusProcessing = "Processing"
	TaskStatusSuccess    = "Success"
	TaskStatusFailed     = "Fail"

	TaskV2StatusQueued    = "queued"
	TaskV2StatusRunning   = "running"
	TaskV2StatusSucceeded = "succeeded"
	TaskV2StatusFailed    = "failed"
	TaskV2StatusCancelled = "cancelled"
)

const (
	Resolution480P  = "480P"
	Resolution512P  = "512P"
	Resolution720P  = "720P"
	Resolution768P  = "768P"
	Resolution1080P = "1080P"
	Resolution2K    = "2K"
)

const (
	DefaultDuration   = 6
	DefaultResolution = Resolution720P
)

// h3PriceKey 针对不同模型高规格升级的键 (零值键 {} 即为各自默认基准)
type h3PriceKey struct {
	is768p bool // MiniMax-H3-Max 升级到 768P
	is2k   bool // MiniMax-H3 升级到 2K
}

// h3PriceTable 各模型每秒基准与升级单价（单位：元/秒）
// 零值键 {} 统一对应各自的默认起步规格 (H3: 768P, H3-Max: 480P, Regen: 2K)
var h3PriceTable = map[string]map[h3PriceKey]float64{
	ModelH3: {
		{}:           0.50, // 默认 768P: 0.50 元/秒 (基准单价)
		{is2k: true}: 0.80, // 升级 2K: 0.80 元/秒 (1.60 倍)
	},
	ModelH3Max: {
		{}:             0.33, // 默认 480P: 0.33 元/秒 (基准单价)
		{is768p: true}: 0.50, // 升级 768P: 0.50 元/秒 (1.515152 倍)
	},
	ModelH3Regeneration: {
		{}: 0.30, // 默认 2K: 0.30 元/秒 (基准单价)
	},
}

// h3FreeImageCount 各模型免费包含的图片素材上限
var h3FreeImageCount = map[string]int{
	ModelH3:             5,
	ModelH3Max:          2,
	ModelH3Regeneration: 5,
}

// h3DefaultImagePrice 各模型超额图片默认单价（元/张）
var h3DefaultImagePrice = map[string]float64{
	ModelH3:             0.20,
	ModelH3Max:          0.50,
	ModelH3Regeneration: 0.15,
}

// GetH3VideoInputPricePerSec 根据模型及【生成视频目标分辨率】获取输入参考视频动作单价（元/秒）
// 官方计费规则：“按输入视频时长及生成视频分辨率计费”
// - MiniMax-H3: 768P 为 0.50 元/秒，2K 为 0.80 元/秒
// - MiniMax-H3-Max: 480P 为 0.37 元/秒，768P 为 0.97 元/秒
// - MiniMax-H3-Regeneration: 固定 2K 为 0.30 元/秒
func GetH3VideoInputPricePerSec(modelName, resolution string) float64 {
	res := strings.ToUpper(strings.TrimSpace(resolution))
	switch modelName {
	case ModelH3:
		if res == Resolution2K {
			return 0.80
		}
		return 0.50
	case ModelH3Max:
		if res == Resolution768P {
			return 0.97
		}
		return 0.37
	case ModelH3Regeneration:
		return 0.30
	default:
		return 0.50
	}
}

// IsH3Model 判断是否为 H3 全模态系列模型
func IsH3Model(modelName string) bool {
	switch modelName {
	case ModelH3, ModelH3Max, ModelH3Regeneration:
		return true
	default:
		return false
	}
}

// GetH3ResolutionRatio 返回 H3 系列在特定分辨率下的相对基准单价倍率
func GetH3ResolutionRatio(modelName, resolution string) (float64, bool) {
	prices, ok := h3PriceTable[modelName]
	if !ok {
		return 1.0, false
	}
	base := prices[h3PriceKey{}]
	if base <= 0 {
		return 1.0, false
	}
	res := strings.ToUpper(strings.TrimSpace(resolution))
	key := h3PriceKey{
		is768p: res == Resolution768P,
		is2k:   res == Resolution2K,
	}
	price, exists := prices[key]
	if !exists {
		return 1.0, true
	}
	return price / base, true
}
