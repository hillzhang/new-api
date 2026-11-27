package ctyundoubao

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVideoInputRatio(t *testing.T) {
	// 1. cdance2.5-0807
	ratio, ok := GetVideoInputRatio("cdance2.5-0807", "720p", false)
	assert.True(t, ok)
	assert.InDelta(t, 1.0, ratio, 0.0001)

	ratio, ok = GetVideoInputRatio("cdance2.5-0807", "720p", true)
	assert.True(t, ok)
	assert.InDelta(t, 42.0/70.0, ratio, 0.0001)

	ratio, ok = GetVideoInputRatio("cdance2.5-0807", "1080p", false)
	assert.True(t, ok)
	assert.InDelta(t, 77.0/70.0, ratio, 0.0001)

	ratio, ok = GetVideoInputRatio("cdance2.5-0807", "1080p", true)
	assert.True(t, ok)
	assert.InDelta(t, 46.0/70.0, ratio, 0.0001)

	// 2. cdance2.0-0807
	ratio, ok = GetVideoInputRatio("cdance2.0-0807", "720p", false)
	assert.True(t, ok)
	assert.InDelta(t, 1.0, ratio, 0.0001)

	ratio, ok = GetVideoInputRatio("cdance2.0-0807", "720p", true)
	assert.True(t, ok)
	assert.InDelta(t, 28.0/46.0, ratio, 0.0001)

	ratio, ok = GetVideoInputRatio("cdance2.0-0807", "1080p", false)
	assert.True(t, ok)
	assert.InDelta(t, 51.0/46.0, ratio, 0.0001)

	ratio, ok = GetVideoInputRatio("cdance2.0-0807", "4k", false)
	assert.True(t, ok)
	assert.InDelta(t, 26.0/46.0, ratio, 0.0001)

	// 3. cdance2.0-fast-0807 (天翼官方: 37元/22元)
	ratio, ok = GetVideoInputRatio("cdance2.0-fast-0807", "720p", false)
	assert.True(t, ok)
	assert.InDelta(t, 1.0, ratio, 0.0001)

	ratio, ok = GetVideoInputRatio("cdance2.0-fast-0807", "720p", true)
	assert.True(t, ok)
	assert.InDelta(t, 22.0/37.0, ratio, 0.0001)

	// 4. cdance2.0-mini-0807 (天翼官方: 23元/14元)
	ratio, ok = GetVideoInputRatio("cdance2.0-mini-0807", "720p", false)
	assert.True(t, ok)
	assert.InDelta(t, 1.0, ratio, 0.0001)

	ratio, ok = GetVideoInputRatio("cdance2.0-mini-0807", "720p", true)
	assert.True(t, ok)
	assert.InDelta(t, 14.0/23.0, ratio, 0.0001)

	// 5. 通用别名测试: doubao-seedance-2-5
	ratio, ok = GetVideoInputRatio("doubao-seedance-2-5-260628", "1080p", false)
	assert.True(t, ok)
	assert.InDelta(t, 77.0/70.0, ratio, 0.0001)
}

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://ai.ctaigw.cn", "https://ai.ctaigw.cn"},
		{"https://ai.ctaigw.cn/", "https://ai.ctaigw.cn"},
		{"https://ai.ctaigw.cn/v1", "https://ai.ctaigw.cn"},
		{"https://ai.ctaigw.cn/v1/", "https://ai.ctaigw.cn"},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.expected, normalizeBaseURL(tc.input))
	}
}

func TestTaskAdaptor_BuildRequestURLAndHeader(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeCtyunDoubaoVideo,
			ChannelBaseUrl: "https://ai.ctaigw.cn/v1",
			ApiKey:         "sk-test-tianyi-key",
		},
	}
	adaptor.Init(info)

	url, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://ai.ctaigw.cn/v1/contents/generations/tasks", url)

	req, _ := http.NewRequest(http.MethodPost, url, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	err = adaptor.BuildRequestHeader(c, req, info)
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-test-tianyi-key", req.Header.Get("Authorization"))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "application/json", req.Header.Get("Accept"))
}

func TestTaskAdaptor_ConvertToRequestPayload_T2V(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:    "cdance2.0-0807",
		Prompt:   "A cute cat walking in the garden",
		Size:     "1080p",
		Seconds:  "5",
		Duration: 5,
		Metadata: map[string]interface{}{
			"ratio":             "16:9",
			"return_last_frame": true,
			"generate_audio":    true,
			"watermark":         false,
		},
	}

	payload, err := adaptor.convertToRequestPayload(req)
	require.NoError(t, err)
	assert.Equal(t, "cdance2.0-0807", payload.Model)
	assert.Equal(t, "1080p", payload.Resolution)
	assert.Equal(t, "16:9", payload.Ratio)
	require.NotNil(t, payload.Duration)
	assert.Equal(t, dto.IntValue(5), *payload.Duration)
	require.NotNil(t, payload.ReturnLastFrame)
	assert.True(t, bool(*payload.ReturnLastFrame))
	require.NotNil(t, payload.GenerateAudio)
	assert.True(t, bool(*payload.GenerateAudio))
	require.NotNil(t, payload.Watermark)
	assert.False(t, bool(*payload.Watermark))

	require.Len(t, payload.Content, 1)
	assert.Equal(t, "text", payload.Content[0].Type)
	assert.Equal(t, "A cute cat walking in the garden", payload.Content[0].Text)
}

func TestTaskAdaptor_ConvertToRequestPayload_I2V_WithFirstAndLastFrame(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  "cdance2.5-0807",
		Prompt: "Camera moves forward into the hall",
		Size:   "1920*1080",
		Metadata: map[string]interface{}{
			"first_frame": "https://example.com/first.jpg",
			"last_frame":  "https://example.com/last.jpg",
		},
	}

	payload, err := adaptor.convertToRequestPayload(req)
	require.NoError(t, err)
	assert.Equal(t, "cdance2.5-0807", payload.Model)
	assert.Equal(t, "1080p", payload.Resolution)
	assert.Equal(t, "16:9", payload.Ratio)

	require.Len(t, payload.Content, 3)
	// First frame
	assert.Equal(t, "image_url", payload.Content[0].Type)
	assert.Equal(t, "https://example.com/first.jpg", payload.Content[0].ImageURL.URL)
	assert.Equal(t, "first_frame", payload.Content[0].Role)

	// Last frame
	assert.Equal(t, "image_url", payload.Content[1].Type)
	assert.Equal(t, "https://example.com/last.jpg", payload.Content[1].ImageURL.URL)
	assert.Equal(t, "last_frame", payload.Content[1].Role)

	// Prompt
	assert.Equal(t, "text", payload.Content[2].Type)
	assert.Equal(t, "Camera moves forward into the hall", payload.Content[2].Text)
}

func TestTaskAdaptor_ConvertToRequestPayload_R2V_MultipleReferences(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  "cdance2.0-0807",
		Prompt: "A character walking in anime style",
		Images: []string{
			"https://example.com/char.jpg",
			"https://example.com/ref1.jpg",
			"https://example.com/ref2.jpg",
		},
	}

	payload, err := adaptor.convertToRequestPayload(req)
	require.NoError(t, err)
	assert.Equal(t, "cdance2.0-0807", payload.Model)

	require.Len(t, payload.Content, 4)
	assert.Equal(t, "first_frame", payload.Content[0].Role)
	assert.Equal(t, "https://example.com/char.jpg", payload.Content[0].ImageURL.URL)

	assert.Equal(t, "reference_image", payload.Content[1].Role)
	assert.Equal(t, "https://example.com/ref1.jpg", payload.Content[1].ImageURL.URL)

	assert.Equal(t, "reference_image", payload.Content[2].Role)
	assert.Equal(t, "https://example.com/ref2.jpg", payload.Content[2].ImageURL.URL)

	assert.Equal(t, "text", payload.Content[3].Type)
}

func TestTaskAdaptor_ParseTaskResult_Succeeded(t *testing.T) {
	adaptor := &TaskAdaptor{}
	respJSON := `{
		"id": "cgt-20260716112202-9h6zh",
		"model": "cdance2.0-0807",
		"status": "succeeded",
		"content": {
			"video_url": "https://tos.ctyun.cn/doubao/video.mp4",
			"last_frame_url": "https://tos.ctyun.cn/doubao/last.jpg"
		},
		"usage": {
			"completion_tokens": 238500,
			"total_tokens": 238500
		},
		"resolution": "720p",
		"duration": 5
	}`

	info, err := adaptor.ParseTaskResult([]byte(respJSON))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, info.Status)
	assert.Equal(t, "100%", info.Progress)
	assert.Equal(t, "https://tos.ctyun.cn/doubao/video.mp4", info.Url)
	assert.Equal(t, 238500, info.CompletionTokens)
	assert.Equal(t, 238500, info.TotalTokens)
}

func TestTaskAdaptor_ParseTaskResult_Failed(t *testing.T) {
	adaptor := &TaskAdaptor{}
	respJSON := `{
		"id": "cgt-failed-123",
		"model": "cdance2.0-0807",
		"status": "failed",
		"error": {
			"code": "InvalidParameter",
			"message": "input reference image is invalid"
		}
	}`

	info, err := adaptor.ParseTaskResult([]byte(respJSON))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, info.Status)
	assert.Equal(t, "100%", info.Progress)
	assert.Equal(t, "input reference image is invalid", info.Reason)
}

func TestTaskAdaptor_ConvertToOpenAIVideo(t *testing.T) {
	adaptor := &TaskAdaptor{}
	taskData := `{
		"id": "cgt-20260716112202-9h6zh",
		"model": "cdance2.0-0807",
		"status": "succeeded",
		"content": {
			"video_url": "https://tos.ctyun.cn/doubao/video.mp4",
			"last_frame_url": "https://tos.ctyun.cn/doubao/last.jpg"
		}
	}`

	mTask := &model.Task{
		TaskID:   "public-task-123",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Data:     []byte(taskData),
		Properties: model.Properties{
			OriginModelName: "cdance2.0-0807",
		},
	}

	bytes, err := adaptor.ConvertToOpenAIVideo(mTask)
	require.NoError(t, err)

	var oaiVideo dto.OpenAIVideo
	err = json.Unmarshal(bytes, &oaiVideo)
	require.NoError(t, err)
	assert.Equal(t, "public-task-123", oaiVideo.ID)
	assert.Equal(t, "completed", oaiVideo.Status)
	assert.Equal(t, "https://tos.ctyun.cn/doubao/video.mp4", oaiVideo.Metadata["url"])
	assert.Equal(t, "https://tos.ctyun.cn/doubao/last.jpg", oaiVideo.Metadata["last_frame_url"])
}

func TestTaskAdaptor_EstimateBilling(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		OriginModelName: "cdance2.5-0807",
	}

	// 1. 720p, no video -> ratio 1.0 -> should return nil
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Set("task_request", relaycommon.TaskSubmitReq{
		Model: "cdance2.5-0807",
		Size:  "720p",
	})
	res1 := adaptor.EstimateBilling(c1, info)
	assert.Nil(t, res1)

	// 2. 1080p, no video -> ratio 1.1 -> should return map
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Set("task_request", relaycommon.TaskSubmitReq{
		Model: "cdance2.5-0807",
		Size:  "1080p",
	})
	res2 := adaptor.EstimateBilling(c2, info)
	require.NotNil(t, res2)
	assert.InDelta(t, 1.1, res2["video_input"], 0.0001)

	// 3. 720p, has video -> ratio 0.6 -> should return map
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Set("task_request", relaycommon.TaskSubmitReq{
		Model: "cdance2.5-0807",
		Size:  "720p",
		Metadata: map[string]interface{}{
			"has_video": true,
		},
	})
	res3 := adaptor.EstimateBilling(c3, info)
	require.NotNil(t, res3)
	assert.InDelta(t, 0.6, res3["video_input"], 0.0001)
}

func TestTaskAdaptor_GetModelListAndName(t *testing.T) {
	adaptor := &TaskAdaptor{}
	assert.Equal(t, ChannelName, adaptor.GetChannelName())
	models := adaptor.GetModelList()
	assert.Contains(t, models, "cdance2.0-0807")
	assert.Contains(t, models, "cdance2.5-0807")
	assert.Contains(t, models, "cdance2.0-0813")
	assert.Contains(t, models, "doubao-seedance-2-5")
}

func TestTaskAdaptor_ValidateRequestAndSetAction_NativeContent(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	adaptor := &TaskAdaptor{}
	rawJSON := `{
		"model": "cdance2.0-0807",
		"content": [
			{"type": "text", "text": "镜头固定微动，特写展示面霜质地"},
			{"type": "image_url", "image_url": {"url": "https://example.com/first.jpg"}, "role": "first_frame"}
		],
		"resolution": "1080p",
		"ratio": "16:9",
		"duration": -1,
		"generate_audio": true,
		"watermark": false
	}`

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewBufferString(rawJSON))
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{OriginModelName: "cdance2.0-0807"}
	taskErr := adaptor.ValidateRequestAndSetAction(c, info)
	require.Nil(t, taskErr)
	assert.Equal(t, constant.TaskActionGenerate, info.Action)

	req, err := relaycommon.GetTaskRequest(c)
	require.NoError(t, err)
	assert.Equal(t, "镜头固定微动，特写展示面霜质地", req.Prompt)
	assert.Equal(t, -1, req.Duration)
	assert.Equal(t, "1080p", req.Metadata["resolution"])
	assert.Equal(t, "16:9", req.Metadata["ratio"])
	assert.Equal(t, true, req.Metadata["generate_audio"])
	assert.Equal(t, false, req.Metadata["watermark"])

	payload, err := adaptor.convertToRequestPayload(&req)
	require.NoError(t, err)
	assert.Equal(t, "1080p", payload.Resolution)
	assert.Equal(t, "16:9", payload.Ratio)
	require.NotNil(t, payload.Duration)
	assert.Equal(t, -1, int(*payload.Duration))
	require.NotNil(t, payload.GenerateAudio)
	assert.True(t, bool(*payload.GenerateAudio))
	require.Len(t, payload.Content, 2)
	assert.Equal(t, "text", payload.Content[0].Type)
	assert.Equal(t, "image_url", payload.Content[1].Type)
	assert.Equal(t, "first_frame", payload.Content[1].Role)

	// 验证 EstimateBilling 也能正确读取注入到 Metadata 的 1080p 分辨率
	ratios := adaptor.EstimateBilling(c, info)
	require.NotNil(t, ratios)
	assert.InDelta(t, 51.0/46.0, ratios["video_input"], 0.0001)
}

func TestTaskAdaptor_ValidateRequestAndSetAction_EmptyRejected(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	adaptor := &TaskAdaptor{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewBufferString(`{"model":"cdance2.0-0807"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	taskErr := adaptor.ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{})
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestTaskAdaptor_ConvertToRequestPayload_VideoAndInputReference(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  "cdance2.0-0807",
		Prompt: "根据参考视频与人物图重绘",
		Metadata: map[string]interface{}{
			"input_reference": []any{
				map[string]any{"image": "https://example.com/char1.png"},
			},
			"video_reference": []any{
				map[string]any{
					"video": "https://example.com/ref.mp4",
					"audio": "https://example.com/bgm.mp3",
				},
			},
		},
	}

	payload, err := adaptor.convertToRequestPayload(req)
	require.NoError(t, err)
	// Expected: 1 reference_image + 1 video_url + 1 audio_url + 1 text = 4 items
	require.Len(t, payload.Content, 4)
	assert.Equal(t, "image_url", payload.Content[0].Type)
	assert.Equal(t, "reference_image", payload.Content[0].Role)
	assert.Equal(t, "https://example.com/char1.png", payload.Content[0].ImageURL.URL)
	assert.Equal(t, "video_url", payload.Content[1].Type)
	assert.Equal(t, "https://example.com/ref.mp4", payload.Content[1].VideoURL.URL)
	assert.Equal(t, "audio_url", payload.Content[2].Type)
	assert.Equal(t, "https://example.com/bgm.mp3", payload.Content[2].AudioURL.URL)
	assert.Equal(t, "text", payload.Content[3].Type)
}

func TestTaskAdaptor_ParseTaskResult_CancelledAndExpired(t *testing.T) {
	adaptor := &TaskAdaptor{}
	for _, status := range []string{"cancelled", "expired"} {
		respJSON := `{"id":"cgt-1","status":"` + status + `"}`
		info, err := adaptor.ParseTaskResult([]byte(respJSON))
		require.NoError(t, err)
		assert.Equal(t, model.TaskStatusFailure, info.Status)
		assert.Equal(t, "100%", info.Progress)
		assert.Equal(t, "task "+status, info.Reason)
	}
}

func TestTaskAdaptor_EstimateBilling_UpstreamFallback(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		OriginModelName: "custom-alias-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "cdance2.5-0807",
		},
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model: "custom-alias-model",
		Size:  "1080p",
	})
	res := adaptor.EstimateBilling(c, info)
	require.NotNil(t, res)
	assert.InDelta(t, 1.1, res["video_input"], 0.0001)
}

func TestTaskAdaptor_ConvertToRequestPayload_InputReferenceVideoURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:          "cdance2.5-0807",
		Prompt:         "视频生视频",
		InputReference: "https://example.com/source.mp4",
	}

	payload, err := adaptor.convertToRequestPayload(req)
	require.NoError(t, err)
	require.Len(t, payload.Content, 2)
	assert.Equal(t, "video_url", payload.Content[0].Type)
	assert.Equal(t, "reference_video", payload.Content[0].Role)
	assert.Equal(t, "https://example.com/source.mp4", payload.Content[0].VideoURL.URL)
	assert.Equal(t, "text", payload.Content[1].Type)

	// 计费检测应感知到视频输入（享受折扣）
	assert.True(t, hasVideoInReq(req))
}

func TestTaskAdaptor_ConvertToRequestPayload_PreservesMultipleNativeTexts(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  "cdance2.0-0807",
		Prompt: "第一段描述",
		Metadata: map[string]interface{}{
			"content": []any{
				map[string]any{"type": "text", "text": "第一段描述"},
				map[string]any{"type": "text", "text": "第二段补充描述"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/img.png"}},
			},
		},
	}

	payload, err := adaptor.convertToRequestPayload(req)
	require.NoError(t, err)
	// 应保留两个文本项和一个图片项，不被抹除
	require.Len(t, payload.Content, 3)
	assert.Equal(t, "第一段描述", payload.Content[0].Text)
	assert.Equal(t, "第二段补充描述", payload.Content[1].Text)
	assert.Equal(t, "image_url", payload.Content[2].Type)
}

func TestTaskAdaptor_ConvertToOpenAIVideo_FallbackToResultURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	originTask := &model.Task{
		TaskID:   "task_ctyun_123",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Data:     []byte(`{"id":"cgt-test","status":"succeeded"}`), // Content.VideoURL is empty in Data
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://tos.ctyun.cn/video/result.mp4",
		},
	}

	bytesData, err := adaptor.ConvertToOpenAIVideo(originTask)
	require.NoError(t, err)

	var video dto.OpenAIVideo
	err = json.Unmarshal(bytesData, &video)
	require.NoError(t, err)
	assert.Equal(t, "https://tos.ctyun.cn/video/result.mp4", video.Metadata["url"])
}
