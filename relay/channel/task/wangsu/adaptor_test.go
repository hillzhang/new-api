package wangsu

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

// 浮点数比较辅助函数
func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestGetVideoInputRatio(t *testing.T) {
	// 1. 测试 Doubao-Seedance-2.5
	ratio, ok := GetVideoInputRatio("doubao-seedance-2-5", "720p", false)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 1.0, 0.0001), "2.5 不含视频基准倍率应为 1.0")

	ratio, ok = GetVideoInputRatio("doubao-seedance-2-5", "720p", true)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 42.0/70.0, 0.0001), "2.5 含视频倍率应为 0.6")

	ratio, ok = GetVideoInputRatio("doubao-seedance-2-5", "1080p", false)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 77.0/70.0, 0.0001), "2.5 1080p 不含视频倍率应为 77/70")

	ratio, ok = GetVideoInputRatio("doubao-seedance-2-5", "1080p", true)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 46.0/70.0, 0.0001), "2.5 1080p 含视频倍率应为 46/70")

	// 带日期后缀匹配
	ratio, ok = GetVideoInputRatio("doubao-seedance-2-5-260628", "720p", true)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 0.6, 0.0001))

	ratio, ok = GetVideoInputRatio("doubao-seedance-2-5-260628", "1080p", false)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 77.0/70.0, 0.0001))

	ratio, ok = GetVideoInputRatio("doubao-seedance-2-5-260628", "1080p", true)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 46.0/70.0, 0.0001))

	// 2. 测试 Doubao-Seedance-2.0
	ratio, ok = GetVideoInputRatio("doubao-seedance-2-0-260128", "720p", false)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 1.0, 0.0001), "2.0 720p 不含视频基准应为 1.0")

	ratio, ok = GetVideoInputRatio("doubao-seedance-2-0-260128", "720p", true)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 28.0/46.0, 0.0001), "2.0 720p 含视频应为 28/46")

	ratio, ok = GetVideoInputRatio("doubao-seedance-2-0-260128", "1080p", false)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 51.0/46.0, 0.0001), "2.0 1080p 不含视频应为 51/46")

	ratio, ok = GetVideoInputRatio("doubao-seedance-2-0-260128", "1080p", true)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 31.0/46.0, 0.0001), "2.0 1080p 含视频应为 31/46")

	ratio, ok = GetVideoInputRatio("doubao-seedance-2-0-260128", "4k", false)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 26.0/46.0, 0.0001), "2.0 4K 不含视频应为 26/46")

	ratio, ok = GetVideoInputRatio("doubao-seedance-2-0-260128", "4k", true)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 16.0/46.0, 0.0001), "2.0 4K 含视频应为 16/46")

	// 3. 测试 Doubao-Seedance-2.0-fast
	ratio, ok = GetVideoInputRatio("doubao-seedance-2-0-fast-260128", "720p", false)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 1.0, 0.0001))

	ratio, ok = GetVideoInputRatio("doubao-seedance-2-0-fast-260128", "720p", true)
	assert.True(t, ok)
	assert.True(t, almostEqual(ratio, 16.50/27.75, 0.0001))
}

func TestParseTaskResult_RealWangsuResponse(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// 用户真实截图数据
	mockResp := `{
		"id": "video.31353.646f7562616f2d73656564616e63652d322d302d323630313238.766964656f2e323630323332e3634366637353632",
		"object": "video",
		"model": "doubao-seedance-2-0-260128",
		"created_at": 1789440905,
		"expires_at": 1789613705,
		"completed_at": 1789441021,
		"status": "completed",
		"size": "720p",
		"seconds": "6",
		"eca_audio": true,
		"eca_video_usage": {
			"completion_tokens": 130500,
			"total_tokens": 130500
		}
	}`

	taskInfo, err := adaptor.ParseTaskResult([]byte(mockResp))
	assert.NoError(t, err)
	assert.NotNil(t, taskInfo)

	assert.Equal(t, model.TaskStatusSuccess, taskInfo.Status)
	assert.Equal(t, "100%", taskInfo.Progress)
	assert.Equal(t, 130500, taskInfo.TotalTokens, "应成功提取网宿返回的 130500 Tokens")
	assert.Equal(t, 130500, taskInfo.CompletionTokens)
}

func TestConvertToRequestPayload(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// 1. 文生视频
	reqT2V := &relaycommon.TaskSubmitReq{
		Model:   "doubao-seedance-2-5",
		Prompt:  "一只可爱的小猫在钢琴上演奏",
		Seconds: "5",
		Size:    "1280x720",
	}
	payload, err := adaptor.convertToRequestPayload(reqT2V)
	assert.NoError(t, err)
	assert.Equal(t, "doubao-seedance-2-5", payload.Model)
	assert.Equal(t, "一只可爱的小猫在钢琴上演奏", payload.Prompt)
	assert.Equal(t, "720p", payload.Size, "1280x720 应该被智能标准化为 720p")
	assert.NotNil(t, payload.Seconds)
	assert.Equal(t, 5, int(*payload.Seconds))

	// 2. 图生视频（单图，封装进 InputReference）
	reqI2V := &relaycommon.TaskSubmitReq{
		Model:   "doubao-seedance-2-0-260128",
		Prompt:  "让画面中的花朵绽放",
		Images:  []string{"https://example.com/cat.png"},
		Seconds: "6",
		Size:    "720p",
	}
	payloadI2V, err := adaptor.convertToRequestPayload(reqI2V)
	assert.NoError(t, err)
	assert.NotNil(t, payloadI2V.InputReference)
	refListI2V, ok := payloadI2V.InputReference.([]map[string]any)
	assert.True(t, ok)
	assert.Len(t, refListI2V, 1)
	assert.Equal(t, "https://example.com/cat.png", refListI2V[0]["image"])
	assert.Empty(t, payloadI2V.EcaFirstFrame)
	assert.Empty(t, payloadI2V.EcaLastFrame)

	// 3. 图生视频（多图参考，全部保留进 InputReference）
	reqFrames := &relaycommon.TaskSubmitReq{
		Model:   "doubao-seedance-2-0-260128",
		Prompt:  "多图参考生成",
		Images:  []string{"https://example.com/start.png", "https://example.com/end.png"},
		Seconds: "4",
	}
	payloadFrames, err := adaptor.convertToRequestPayload(reqFrames)
	assert.NoError(t, err)
	assert.NotNil(t, payloadFrames.InputReference)
	refListFrames, ok := payloadFrames.InputReference.([]map[string]any)
	assert.True(t, ok)
	assert.Len(t, refListFrames, 2)
	assert.Equal(t, "https://example.com/start.png", refListFrames[0]["image"])
	assert.Equal(t, "https://example.com/end.png", refListFrames[1]["image"])
	assert.Empty(t, payloadFrames.EcaFirstFrame)
	assert.Empty(t, payloadFrames.EcaLastFrame)

	// 验证序列化后包含 input_reference 数组
	jsonBytes, err := json.Marshal(payloadFrames)
	assert.NoError(t, err)
	assert.NotContains(t, string(jsonBytes), `"content"`)
	assert.Contains(t, string(jsonBytes), `"input_reference"`)
	assert.NotContains(t, string(jsonBytes), `"eca_first_frame"`)
}

func TestNormalizeWangsuResolution(t *testing.T) {
	assert.Equal(t, "720p", normalizeWangsuResolution(""))
	assert.Equal(t, "720p", normalizeWangsuResolution("720p"))
	assert.Equal(t, "720p", normalizeWangsuResolution("720P"))
	assert.Equal(t, "720p", normalizeWangsuResolution("1280x720"))
	assert.Equal(t, "720p", normalizeWangsuResolution("720*1280"))
	assert.Equal(t, "1080p", normalizeWangsuResolution("1080p"))
	assert.Equal(t, "1080p", normalizeWangsuResolution("1080P"))
	assert.Equal(t, "1080p", normalizeWangsuResolution("1920x1080"))
	assert.Equal(t, "480p", normalizeWangsuResolution("480p"))
	assert.Equal(t, "480p", normalizeWangsuResolution("854x480"))
	assert.Equal(t, "4k", normalizeWangsuResolution("4k"))
	assert.Equal(t, "4k", normalizeWangsuResolution("3840x2160"))
}

func TestConvertToRequestPayload_VideoAndAudioReference(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// 1. 测试 Metadata 中的 video_reference 字符串自动封装为 eca_video_reference
	reqV2V := &relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-5-260628",
		Prompt: "猫咪在奔跑",
		Metadata: map[string]any{
			"video_reference": "https://example.com/cat.mp4",
		},
	}
	payloadV2V, err := adaptor.convertToRequestPayload(reqV2V)
	assert.NoError(t, err)
	assert.Len(t, payloadV2V.EcaVideoReference, 1)
	assert.Equal(t, "https://example.com/cat.mp4", payloadV2V.EcaVideoReference[0]["video"])

	// 2. 测试 Metadata 中的 video_reference (含音频)
	reqMetaVideo := &relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "人物在演讲",
		Metadata: map[string]any{
			"video_reference": []any{
				map[string]any{
					"video": "https://example.com/speech.mp4",
					"audio": "https://example.com/voice.mp3",
				},
			},
		},
	}
	payloadMetaVideo, err := adaptor.convertToRequestPayload(reqMetaVideo)
	assert.NoError(t, err)
	assert.Len(t, payloadMetaVideo.EcaVideoReference, 1)
	assert.Equal(t, "https://example.com/speech.mp4", payloadMetaVideo.EcaVideoReference[0]["video"])
	assert.Equal(t, "https://example.com/voice.mp3", payloadMetaVideo.EcaVideoReference[0]["audio"])

	// 3. 测试 2.5 独有的 eca_audio_reference
	reqAudioRef := &relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-5-260628",
		Prompt: "带背景音乐生成",
		Metadata: map[string]any{
			"audio_reference": []any{
				map[string]any{"audio": "https://example.com/bgm.mp3"},
			},
		},
	}
	payloadAudioRef, err := adaptor.convertToRequestPayload(reqAudioRef)
	assert.NoError(t, err)
	assert.Len(t, payloadAudioRef.EcaAudioReference, 1)
	assert.Equal(t, "https://example.com/bgm.mp3", payloadAudioRef.EcaAudioReference[0]["audio"])
}

func TestConvertToRequestPayload_AdvancedMetadata(t *testing.T) {
	adaptor := &TaskAdaptor{}

	req := &relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "高级参数测试",
		Metadata: map[string]any{
			"eca_aspect_ratio":       "16:9",
			"eca_audio":              true,
			"eca_mode":               "pro",
			"eca_multi_shot":         true,
			"eca_tools":              []any{map[string]any{"type": "web_search"}},
			"seed":                   12345,
			"watermark":              false,
			"return_last_frame":      true,
			"execution_expires_after": 7200,
			"callback_url":           "https://example.com/callback",
		},
	}

	payload, err := adaptor.convertToRequestPayload(req)
	assert.NoError(t, err)
	assert.Equal(t, "16:9", payload.EcaAspectRatio)
	assert.NotNil(t, payload.EcaAudio)
	assert.True(t, bool(*payload.EcaAudio))
	assert.Equal(t, "pro", payload.EcaMode)
	assert.NotNil(t, payload.EcaMultiShot)
	assert.True(t, bool(*payload.EcaMultiShot))
	assert.Len(t, payload.EcaTools, 1)
	assert.Equal(t, "web_search", payload.EcaTools[0]["type"])
	assert.NotNil(t, payload.Seed)
	assert.Equal(t, 12345, int(*payload.Seed))
	assert.NotNil(t, payload.Watermark)
	assert.False(t, bool(*payload.Watermark))
	assert.NotNil(t, payload.ReturnLastFrame)
	assert.True(t, bool(*payload.ReturnLastFrame))
	assert.NotNil(t, payload.ExecutionExpiresAfter)
	assert.Equal(t, 7200, int(*payload.ExecutionExpiresAfter))
	assert.Equal(t, "https://example.com/callback", payload.CallbackUrl)

	// 序列化后字段名必须精确匹配网宿接口规范
	jsonBytes, err := json.Marshal(payload)
	assert.NoError(t, err)
	jsonStr := string(jsonBytes)
	assert.Contains(t, jsonStr, `"eca_aspect_ratio":"16:9"`)
	assert.Contains(t, jsonStr, `"eca_audio":true`)
	assert.Contains(t, jsonStr, `"eca_mode":"pro"`)
	assert.Contains(t, jsonStr, `"eca_multi_shot":true`)
	assert.Contains(t, jsonStr, `"eca_tools":[{"type":"web_search"}]`)
	assert.Contains(t, jsonStr, `"seed":12345`)
	assert.Contains(t, jsonStr, `"watermark":false`)
	assert.Contains(t, jsonStr, `"return_last_frame":true`)
	assert.Contains(t, jsonStr, `"execution_expires_after":7200`)
	assert.Contains(t, jsonStr, `"callback_url":"https://example.com/callback"`)
}

func TestConvertToRequestPayload_UniversalMetadataWithoutEca(t *testing.T) {
	adaptor := &TaskAdaptor{}

	req := &relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "通用无 eca 前缀参数测试",
		Metadata: map[string]any{
			"aspect_ratio": "16:9",
			"audio":        true,
			"first_frame":  "https://example.com/first.png",
			"last_frame":   "https://example.com/last.png",
			"mode":         "pro",
			"multi_shot":   true,
			"tools":        []any{map[string]any{"type": "web_search"}},
		},
	}

	payload, err := adaptor.convertToRequestPayload(req)
	assert.NoError(t, err)
	assert.Equal(t, "16:9", payload.EcaAspectRatio)
	assert.NotNil(t, payload.EcaAudio)
	assert.True(t, bool(*payload.EcaAudio))
	assert.Equal(t, "https://example.com/first.png", payload.EcaFirstFrame)
	assert.Equal(t, "https://example.com/last.png", payload.EcaLastFrame)
	assert.Equal(t, "pro", payload.EcaMode)
	assert.NotNil(t, payload.EcaMultiShot)
	assert.True(t, bool(*payload.EcaMultiShot))
	assert.Len(t, payload.EcaTools, 1)
	assert.Equal(t, "web_search", payload.EcaTools[0]["type"])
}

func TestParseTaskResult_Expired(t *testing.T) {
	adaptor := &TaskAdaptor{}

	resp := `{
		"id": "video.12345",
		"status": "expired"
	}`

	taskInfo, err := adaptor.ParseTaskResult([]byte(resp))
	assert.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, taskInfo.Status)
	assert.Equal(t, "task execution expired", taskInfo.Reason)
}

func TestConvertToRequestPayload_MetadataFirstAndLastFrame(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// 用户场景：eca_first_frame 和 eca_last_frame 明确放在 metadata 中传递
	req := &relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-5-260628",
		Prompt: "结合首尾帧生成自然转场视频",
		Size:   "720p",
		Metadata: map[string]any{
			"eca_first_frame": "https://example.com/first.png",
			"eca_last_frame":  "https://example.com/last.png",
		},
	}

	payload, err := adaptor.convertToRequestPayload(req)
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com/first.png", payload.EcaFirstFrame)
	assert.Equal(t, "https://example.com/last.png", payload.EcaLastFrame)
}

func TestConvertToRequestPayload_UserDemoJson(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// 用户场景：渠道专有参数统一放置于 metadata 中
	rawJSON := `{
		"model": "doubao-seedance-2-5-260628",
		"prompt": "结合参考图生成一个简单安静的视频",
		"seconds": 5,
		"size": "720p",
		"metadata": {
			"eca_aspect_ratio": "16:9",
			"input_reference": [
				{
					"image": "https://arkdocs.tos-cn-beijing.volces.com/images/video-generation/seedance2.5_30s_input.png"
				},
				{
					"image": "https://arkdocs.tos-cn-beijing.volces.com/images/video-generation/seedance2.5_reference1.png"
				}
			]
		}
	}`

	var req relaycommon.TaskSubmitReq
	err := json.Unmarshal([]byte(rawJSON), &req)
	assert.NoError(t, err)

	payload, err := adaptor.convertToRequestPayload(&req)
	assert.NoError(t, err)
	assert.Equal(t, "doubao-seedance-2-5-260628", payload.Model)
	assert.Equal(t, "结合参考图生成一个简单安静的视频", payload.Prompt)
	assert.Equal(t, "720p", payload.Size)
	assert.Equal(t, "16:9", payload.EcaAspectRatio)
	// 原样透传 input_reference 数组，支持多参考图，不强行塞给首尾帧
	assert.NotNil(t, payload.InputReference)
	inputRefList, ok := payload.InputReference.([]any)
	assert.True(t, ok)
	assert.Len(t, inputRefList, 2)
	assert.Empty(t, payload.EcaFirstFrame)
	assert.Empty(t, payload.EcaLastFrame)
}

func TestConvertToRequestPayload_InputReferenceMultipleImages(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// 支持上传多张（>2张）参考图，例如3张参考图指导人物与风格
	rawJSON := `{
		"model": "doubao-seedance-2-5-260628",
		"prompt": "结合多张参考图生成视频",
		"metadata": {
			"input_reference": [
				{"image": "https://example.com/face_angle1.png"},
				{"image": "https://example.com/face_angle2.png"},
				{"image": "https://example.com/costume.png"}
			]
		}
	}`

	var req relaycommon.TaskSubmitReq
	err := json.Unmarshal([]byte(rawJSON), &req)
	assert.NoError(t, err)

	payload, err := adaptor.convertToRequestPayload(&req)
	assert.NoError(t, err)
	assert.NotNil(t, payload.InputReference)
	refList, ok := payload.InputReference.([]any)
	assert.True(t, ok)
	assert.Len(t, refList, 3, "应该完整保留3张参考图")
	assert.Empty(t, payload.EcaFirstFrame)
	assert.Empty(t, payload.EcaLastFrame)
}

func TestConvertToRequestPayload_InputReferenceSingleImage(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// OpenAI 规范中常用的单字符串形式
	rawJSON := `{
		"model": "doubao-seedance-2-5-260628",
		"prompt": "单图生视频",
		"input_reference": "https://example.com/single_image.jpg"
	}`

	var req relaycommon.TaskSubmitReq
	err := json.Unmarshal([]byte(rawJSON), &req)
	assert.NoError(t, err)

	payload, err := adaptor.convertToRequestPayload(&req)
	assert.NoError(t, err)
	assert.NotNil(t, payload.InputReference)
	refList, ok := payload.InputReference.([]map[string]any)
	assert.True(t, ok)
	assert.Len(t, refList, 1)
	assert.Equal(t, "https://example.com/single_image.jpg", refList[0]["image"])
	assert.Empty(t, payload.EcaVideoReference)
}

func TestConvertToRequestPayload_VideoReferenceInMetadata(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// 视频生视频：通过专有的 video_reference 参数传递
	rawJSON := `{
		"model": "doubao-seedance-2-5-260628",
		"prompt": "视频风格重绘",
		"metadata": {
			"video_reference": "https://example.com/reference_motion.mp4"
		}
	}`

	var req relaycommon.TaskSubmitReq
	err := json.Unmarshal([]byte(rawJSON), &req)
	assert.NoError(t, err)

	payload, err := adaptor.convertToRequestPayload(&req)
	assert.NoError(t, err)
	assert.Nil(t, payload.InputReference)
	assert.Len(t, payload.EcaVideoReference, 1)
	assert.Equal(t, "https://example.com/reference_motion.mp4", payload.EcaVideoReference[0]["video"])
}

func TestHasVideoInMetadata_DistinguishesImageAndVideo(t *testing.T) {
	// 1. input_reference 专用于参考图，不能被判定为 hasVideo
	metaImageArray := map[string]any{
		"input_reference": []any{
			map[string]any{"image": "https://example.com/frame1.png"},
			map[string]any{"image": "https://example.com/frame2.png"},
		},
	}
	assert.False(t, hasVideoInMetadata(metaImageArray), "input_reference 数组不能被判定为包含视频")

	metaImageStr := map[string]any{
		"input_reference": "https://example.com/cat.png",
	}
	assert.False(t, hasVideoInMetadata(metaImageStr), "input_reference 单图不能被判定为包含视频")

	// 2. 视频参考必须通过专有的 video_reference / eca_video_reference 传递
	metaVideoRef := map[string]any{
		"video_reference": []any{
			map[string]any{"video": "https://example.com/speech.mp4"},
		},
	}
	assert.True(t, hasVideoInMetadata(metaVideoRef), "video_reference 应判定为包含视频")

	metaEcaVideoRef := map[string]any{
		"eca_video_reference": []any{
			map[string]any{"video": "https://example.com/dance.mov"},
		},
	}
	assert.True(t, hasVideoInMetadata(metaEcaVideoRef), "eca_video_reference 应判定为包含视频")

	// 3. 用户实际场景：eca_video_reference 同时包含 video 和 audio
	metaVideoAndAudio := map[string]any{
		"eca_video_reference": []any{
			map[string]any{
				"video": "https://wcs-upload.edgecloudapp.com/video_doubao-seedance-2-0-fast-260128_cg.mp4",
				"audio": "https://wcs-upload.edgecloudapp.com/audios/2016ce53-ecce-4973-992d.mp3",
			},
		},
	}
	assert.True(t, hasVideoInMetadata(metaVideoAndAudio), "同时包含 video 与 audio 应判定为包含视频")

	// 4. 用户实际场景：eca_video_reference 仅支持/传递了音频，无视频
	metaAudioOnlyInEcaVideoRef := map[string]any{
		"eca_video_reference": []any{
			map[string]any{
				"audio": "https://wcs-upload.edgecloudapp.com/audios/2016ce53-ecce-4973-992d.mp3",
			},
		},
	}
	assert.False(t, hasVideoInMetadata(metaAudioOnlyInEcaVideoRef), "eca_video_reference 中只有 audio 时不能判定为包含视频")

	// 5. 后缀判断：video_reference 传入纯音频链接（.mp3 / .wav）不能判定为含视频
	metaAudioURLStr := map[string]any{
		"video_reference": "https://example.com/bgm.mp3",
	}
	assert.False(t, hasVideoInMetadata(metaAudioURLStr), "传入 .mp3 后缀的 audio 不能判定为包含视频")

	metaWavURLStr := map[string]any{
		"video_reference": "https://example.com/voice.wav?token=abc",
	}
	assert.False(t, hasVideoInMetadata(metaWavURLStr), "传入 .wav 后缀的 audio 不能判定为包含视频")

	// 6. 后缀判断：video_reference 传入纯视频链接（.mp4）判定为含视频
	metaVideoURLStr := map[string]any{
		"video_reference": "https://example.com/test_video.mp4?auth=xyz",
	}
	assert.True(t, hasVideoInMetadata(metaVideoURLStr), "传入 .mp4 后缀的链接应判定为包含视频")

	// 7. 用户明确规则：只有 video 参数、没有 audio 参数的时候 hasVideo = true
	metaOnlyVideoInMap := map[string]any{
		"eca_video_reference": []any{
			map[string]any{
				"video": "https://wcs-upload.edgecloudapp.com/video_doubao-seedance-2-0-fast-260128_cg.mp4",
			},
		},
	}
	assert.True(t, hasVideoInMetadata(metaOnlyVideoInMap), "只有 video 参数没有 audio 参数时应判定为包含视频 hasVideo = true")

	// 8. metadata["video"] 直接传入视频
	metaDirectVideo := map[string]any{
		"video": "https://example.com/test.mp4",
	}
	assert.True(t, hasVideoInMetadata(metaDirectVideo), "metadata 中直接传 video 视频参数时 hasVideo = true")
}

func TestParseVideoReference_HandlesAudioAndVideo(t *testing.T) {
	// 1. 传入纯音频 URL 字符串，智能转为 audio 结构
	resAudio := parseVideoReference("https://example.com/voice.mp3")
	assert.Len(t, resAudio, 1)
	assert.Equal(t, "https://example.com/voice.mp3", resAudio[0]["audio"])
	assert.Nil(t, resAudio[0]["video"])

	// 2. 传入纯视频 URL 字符串，智能转为 video 结构
	resVideo := parseVideoReference("https://example.com/motion.mp4")
	assert.Len(t, resVideo, 1)
	assert.Equal(t, "https://example.com/motion.mp4", resVideo[0]["video"])
	assert.Nil(t, resVideo[0]["audio"])

	// 3. 传入同时包含 video 和 audio 的对象
	mixed := []any{
		map[string]any{
			"video": "https://example.com/test.mp4",
			"audio": "https://example.com/test.mp3",
		},
	}
	resMixed := parseVideoReference(mixed)
	assert.Len(t, resMixed, 1)
	assert.Equal(t, "https://example.com/test.mp4", resMixed[0]["video"])
	assert.Equal(t, "https://example.com/test.mp3", resMixed[0]["audio"])

	// 4. 传入仅包含 audio 的对象
	audioOnly := []any{
		map[string]any{
			"audio": "https://example.com/test.mp3",
		},
	}
	resAudioOnly := parseVideoReference(audioOnly)
	assert.Len(t, resAudioOnly, 1)
	assert.Equal(t, "https://example.com/test.mp3", resAudioOnly[0]["audio"])
	assert.Nil(t, resAudioOnly[0]["video"])
}

func TestParseTaskResult_UniversalUsage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	mockResp := `{
		"id": "video.31353.646f7562616f2d73656564616e6365...",
		"object": "video",
		"model": "doubao-seedance-2-5-260628",
		"status": "completed",
		"usage": {
			"completion_tokens": 150000,
			"total_tokens": 150000
		}
	}`

	taskInfo, err := adaptor.ParseTaskResult([]byte(mockResp))
	assert.NoError(t, err)
	assert.NotNil(t, taskInfo)
	assert.Equal(t, model.TaskStatusSuccess, taskInfo.Status)
	assert.Equal(t, 150000, taskInfo.TotalTokens)
	assert.Equal(t, 150000, taskInfo.CompletionTokens)
}

func TestConvertToOpenAIVideo_Usage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	originTask := &model.Task{
		TaskID: "task_123456",
		Status: model.TaskStatusSuccess,
		Data: []byte(`{
			"id": "video.31353",
			"status": "completed",
			"usage": {
				"completion_tokens": 130500,
				"total_tokens": 130500
			}
		}`),
	}

	resBytes, err := adaptor.ConvertToOpenAIVideo(originTask)
	assert.NoError(t, err)
	var video dto.OpenAIVideo
	err = common.Unmarshal(resBytes, &video)
	assert.NoError(t, err)
	assert.Equal(t, "task_123456", video.ID)
	assert.NotNil(t, video.Metadata["usage"])
}

func TestPrintWangsuRequestPayloads(t *testing.T) {
	adaptor := &TaskAdaptor{}

	scenarios := []struct {
		Name string
		Req  *relaycommon.TaskSubmitReq
	}{
		{
			Name: "1. 豆包 2.5 最简文生视频",
			Req: &relaycommon.TaskSubmitReq{
				Model:   "doubao-seedance-2-5-260628",
				Prompt:  "黄昏时分，橘猫在窗台上伸懒腰",
				Size:    "720p",
				Seconds: "5",
			},
		},
		{
			Name: "2. 豆包 2.5 独立音频驱动 + 多参考图",
			Req: &relaycommon.TaskSubmitReq{
				Model:   "doubao-seedance-2-5-260628",
				Prompt:  "赛博朋克舞者随音乐节拍律动",
				Size:    "720p",
				Seconds: "10",
				Metadata: map[string]any{
					"aspect_ratio": "16:9",
					"input_reference": []map[string]any{
						{"image": "https://example.com/character.png"},
						{"image": "https://example.com/costume.png"},
					},
					"audio_reference": []map[string]any{
						{"audio": "https://example.com/beat.mp3"},
					},
				},
			},
		},
		{
			Name: "3. 豆包 2.0 旗舰：4K专业模式 + 多镜头运镜 + 联网搜索",
			Req: &relaycommon.TaskSubmitReq{
				Model:   "doubao-seedance-2-0-260128",
				Prompt:  "神舟飞船返回舱降落",
				Size:    "4k",
				Seconds: "12",
				Metadata: map[string]any{
					"mode":         "pro",
					"multi_shot":   true,
					"aspect_ratio": "16:9",
					"tools": []map[string]any{
						{"type": "web_search"},
					},
				},
			},
		},
		{
			Name: "4. 首尾双帧插值平滑过渡",
			Req: &relaycommon.TaskSubmitReq{
				Model:   "doubao-seedance-2-0-260128",
				Prompt:  "从白天到黑夜的城市霓虹变幻",
				Size:    "720p",
				Seconds: "6",
				Metadata: map[string]any{
					"first_frame": "https://example.com/city_day.png",
					"last_frame":  "https://example.com/city_night.png",
				},
			},
		},
		{
			Name: "5. 视频生视频 / 动作重绘 (带伴随音频)",
			Req: &relaycommon.TaskSubmitReq{
				Model:   "doubao-seedance-2-0-260128",
				Prompt:  "将视频中的人物替换为宇航员",
				Size:    "720p",
				Seconds: "5",
				Metadata: map[string]any{
					"video_reference": []map[string]any{
						{
							"video": "https://example.com/actor.mp4",
							"audio": "https://example.com/speech.mp3",
						},
					},
				},
			},
		},
	}

	for _, s := range scenarios {
		payload, err := adaptor.convertToRequestPayload(s.Req)
		assert.NoError(t, err)

		jsonBytes, err := json.MarshalIndent(payload, "", "  ")
		assert.NoError(t, err)

		t.Logf("\n==============================\n[测试场景]: %s\n[上游 Request Payload JSON]:\n%s\n==============================", s.Name, string(jsonBytes))
	}
}

func TestConvertToRequestPayload_SecondsMinusOne(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// 1. 测试 seconds: "-1" 自动时长
	req1 := &relaycommon.TaskSubmitReq{
		Model:   "doubao-seedance-2-5-260628",
		Prompt:  "自动时长测试",
		Seconds: "-1",
	}
	payload1, err := adaptor.convertToRequestPayload(req1)
	assert.NoError(t, err)
	assert.NotNil(t, payload1.Seconds)
	assert.Equal(t, -1, int(*payload1.Seconds))

	// 2. 测试 duration: -1 自动时长
	req2 := &relaycommon.TaskSubmitReq{
		Model:    "doubao-seedance-2-0-260128",
		Prompt:   "duration 自动时长测试",
		Duration: -1,
	}
	payload2, err := adaptor.convertToRequestPayload(req2)
	assert.NoError(t, err)
	assert.NotNil(t, payload2.Seconds)
	assert.Equal(t, -1, int(*payload2.Seconds))
}

func TestConvertToRequestPayload_TopLevelModeWithoutMetadata(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// 顶级 mode 在无 metadata 时应成功透传为 eca_mode
	req := &relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "专业模式测试",
		Mode:   "pro",
	}
	payload, err := adaptor.convertToRequestPayload(req)
	assert.NoError(t, err)
	assert.Equal(t, "pro", payload.EcaMode)
}

func TestConvertToRequestPayload_WeakTypeBools(t *testing.T) {
	adaptor := &TaskAdaptor{}

	req := &relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "弱类型布尔测试",
		Metadata: map[string]any{
			"audio":             "false",
			"multi_shot":        "true",
			"watermark":         "true",
			"return_last_frame": "false",
		},
	}
	payload, err := adaptor.convertToRequestPayload(req)
	assert.NoError(t, err)
	assert.NotNil(t, payload.EcaAudio)
	assert.False(t, bool(*payload.EcaAudio))
	assert.NotNil(t, payload.EcaMultiShot)
	assert.True(t, bool(*payload.EcaMultiShot))
	assert.NotNil(t, payload.Watermark)
	assert.True(t, bool(*payload.Watermark))
	assert.NotNil(t, payload.ReturnLastFrame)
	assert.False(t, bool(*payload.ReturnLastFrame))
}

func TestConvertToRequestPayload_InputReferenceStringArray(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// 传入纯字符串 URL 数组，应自动规整为标准对象数组
	req := &relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-5-260628",
		Prompt: "纯 URL 数组输入测试",
		Metadata: map[string]any{
			"input_reference": []any{
				"https://example.com/face.png",
				"https://example.com/body.png",
			},
		},
	}
	payload, err := adaptor.convertToRequestPayload(req)
	assert.NoError(t, err)
	assert.NotNil(t, payload.InputReference)
	refList, ok := payload.InputReference.([]any)
	assert.True(t, ok)
	assert.Len(t, refList, 2)
	assert.Equal(t, map[string]any{"image": "https://example.com/face.png"}, refList[0])
	assert.Equal(t, map[string]any{"image": "https://example.com/body.png"}, refList[1])
}

func TestExtractResolution_Priority(t *testing.T) {
	// 1. metadata 中的 size 优先于外层 req.Size
	req1 := &relaycommon.TaskSubmitReq{
		Size: "720p",
		Metadata: map[string]any{
			"size": "1080p",
		},
	}
	assert.Equal(t, "1080p", extractResolution(req1))

	// 2. 没有 metadata 时使用外层 req.Size
	req2 := &relaycommon.TaskSubmitReq{
		Size: "480p",
	}
	assert.Equal(t, "480p", extractResolution(req2))
}

func TestMatchPriceTable_LongestPrefixMatch(t *testing.T) {
	// 1. 精确匹配标准别名 doubao-seedance-2-0
	prices, ok := matchPriceTable("doubao-seedance-2-0")
	assert.True(t, ok)
	assert.Equal(t, 46.0, prices[videoPriceKey{}])

	// 2. 最长前缀匹配：doubao-seedance-2-0-fast 应精准匹配 fast 版价格
	prices, ok = matchPriceTable("doubao-seedance-2-0-fast-custom-suffix")
	assert.True(t, ok)
	assert.Equal(t, 27.75, prices[videoPriceKey{}])

	// 3. 不允许短名反向吞噬长名：只传 "doubao" 应返回 false，避免随机错误计费
	_, ok = matchPriceTable("doubao")
	assert.False(t, ok)
}

