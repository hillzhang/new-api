package ctyun

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func testRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeCtyunVideo,
			ChannelBaseUrl: "https://ai.ctaigw.cn",
			ApiKey:         "sk-test-ctyun-key",
		},
	}
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
		{"https://ai.ctaigw.cn///", "https://ai.ctaigw.cn"},
	}

	for _, tt := range tests {
		require.Equal(t, tt.expected, normalizeBaseURL(tt.input))
	}
}

func TestTaskAdaptorInitAndBuildRequestURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := testRelayInfo()
	info.ChannelBaseUrl = "https://ai.ctaigw.cn/v1/"
	adaptor.Init(info)

	url, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://ai.ctaigw.cn/v1/services/aigc/video-generation/video-synthesis", url)
}

func TestTaskAdaptorBuildRequestHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	adaptor := &TaskAdaptor{}
	info := testRelayInfo()
	adaptor.Init(info)

	httpReq, err := http.NewRequest(http.MethodPost, "https://ai.ctaigw.cn/v1/services/aigc/video-generation/video-synthesis", nil)
	require.NoError(t, err)

	err = adaptor.BuildRequestHeader(c, httpReq, info)
	require.NoError(t, err)
	require.Equal(t, "Bearer sk-test-ctyun-key", httpReq.Header.Get("Authorization"))
	require.Equal(t, "application/json", httpReq.Header.Get("Content-Type"))
	require.Equal(t, "enable", httpReq.Header.Get("X-DashScope-Async"))
}

func TestTaskAdaptorConvertToCtyunRequestHappyHorseT2V(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.Init(testRelayInfo())

	req := relaycommon.TaskSubmitReq{
		Model:    "happyhorse-1.1-t2v",
		Prompt:   "A running horse in cinematic sunlight",
		Size:     "1920*1080",
		Duration: 5,
		Metadata: map[string]interface{}{
			"parameters": map[string]interface{}{
				"audio": true,
			},
		},
	}

	ctyunReq, err := adaptor.convertToCtyunRequest(testRelayInfo(), req)
	require.NoError(t, err)
	require.Equal(t, "happyhorse-1.1-t2v", ctyunReq.Model)
	require.Equal(t, "A running horse in cinematic sunlight", ctyunReq.Input.Prompt)
	require.Equal(t, "1080P", ctyunReq.Parameters.Resolution)
	require.Equal(t, "16:9", ctyunReq.Parameters.Ratio)
	require.Equal(t, 5, ctyunReq.Parameters.Duration)
	require.NotNil(t, ctyunReq.Parameters.Audio)
	require.True(t, *ctyunReq.Parameters.Audio)
	require.Empty(t, ctyunReq.Input.Media)
}

func TestTaskAdaptorConvertToCtyunRequestHappyHorseI2V(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.Init(testRelayInfo())

	req := relaycommon.TaskSubmitReq{
		Model:    "happyhorse-1.1-i2v",
		Prompt:   "Make this portrait smile and wink",
		Image:    "https://example.com/input.jpg",
		Duration: 5,
		Size:     "720p",
	}

	ctyunReq, err := adaptor.convertToCtyunRequest(testRelayInfo(), req)
	require.NoError(t, err)
	require.Equal(t, "happyhorse-1.1-i2v", ctyunReq.Model)
	require.Equal(t, "720P", ctyunReq.Parameters.Resolution)
	require.Equal(t, []CtyunVideoMedia{
		{Type: "first_frame", URL: "https://example.com/input.jpg"},
	}, ctyunReq.Input.Media)
}

func TestTaskAdaptorConvertToCtyunRequestHappyHorseR2V(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.Init(testRelayInfo())

	req := relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.1-r2v",
		Prompt: "A dynamic video guided by reference frames",
		Images: []string{
			"https://example.com/ref1.jpg",
			"https://example.com/ref2.jpg",
		},
		Duration: 5,
		Size:     "16:9",
	}

	ctyunReq, err := adaptor.convertToCtyunRequest(testRelayInfo(), req)
	require.NoError(t, err)
	require.Equal(t, "happyhorse-1.1-r2v", ctyunReq.Model)
	require.Equal(t, "16:9", ctyunReq.Parameters.Ratio)
	require.Equal(t, []CtyunVideoMedia{
		{Type: "reference_image", URL: "https://example.com/ref1.jpg"},
		{Type: "reference_image", URL: "https://example.com/ref2.jpg"},
	}, ctyunReq.Input.Media)
}

func TestTaskAdaptorConvertToCtyunRequestWan27I2V(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.Init(testRelayInfo())

	req := relaycommon.TaskSubmitReq{
		Model:    "wan2.7-i2v",
		Prompt:   "smooth transitions between frames",
		Images:   []string{"https://example.com/start.png", "https://example.com/end.png"},
		Duration: 5,
		Size:     "1280*720",
	}

	ctyunReq, err := adaptor.convertToCtyunRequest(testRelayInfo(), req)
	require.NoError(t, err)
	require.Equal(t, "wan2.7-i2v", ctyunReq.Model)
	require.Equal(t, "720P", ctyunReq.Parameters.Resolution)
	require.Equal(t, []CtyunVideoMedia{
		{Type: "first_frame", URL: "https://example.com/start.png"},
		{Type: "last_frame", URL: "https://example.com/end.png"},
	}, ctyunReq.Input.Media)
}

func TestProcessCtyunOtherRatiosHappyHorse11(t *testing.T) {
	req720p := &CtyunVideoRequest{
		Model: "happyhorse-1.1-t2v",
		Parameters: &CtyunVideoParameters{
			Resolution: "720P",
			Duration:   5,
		},
	}
	ratios, err := ProcessCtyunOtherRatios(req720p)
	require.NoError(t, err)
	require.Equal(t, 1.0, ratios["resolution-720P"])

	req1080p := &CtyunVideoRequest{
		Model: "happyhorse-1.1-t2v",
		Parameters: &CtyunVideoParameters{
			Resolution: "1080P",
			Duration:   5,
		},
	}
	ratios, err = ProcessCtyunOtherRatios(req1080p)
	require.NoError(t, err)
	expected1080pRatio := 1.2 / 0.9
	require.InDelta(t, expected1080pRatio, ratios["resolution-1080P"], 0.0001)
}

func TestProcessCtyunOtherRatiosHappyHorse10(t *testing.T) {
	req1080p := &CtyunVideoRequest{
		Model: "happyhorse-1.0-i2v",
		Parameters: &CtyunVideoParameters{
			Resolution: "1080P",
			Duration:   5,
		},
	}
	ratios, err := ProcessCtyunOtherRatios(req1080p)
	require.NoError(t, err)
	expected1080pRatio := 1.6 / 0.9
	require.InDelta(t, expected1080pRatio, ratios["resolution-1080P"], 0.0001)

	// Test happyhorse-1.0-i2v-20260618
	reqDated := &CtyunVideoRequest{
		Model: "happyhorse-1.0-i2v-20260618",
		Parameters: &CtyunVideoParameters{
			Resolution: "1080P",
			Duration:   5,
		},
	}
	ratios, err = ProcessCtyunOtherRatios(reqDated)
	require.NoError(t, err)
	require.InDelta(t, expected1080pRatio, ratios["resolution-1080P"], 0.0001)
}

func TestProcessCtyunOtherRatiosWan27(t *testing.T) {
	req1080p := &CtyunVideoRequest{
		Model: "wan2.7-t2v",
		Parameters: &CtyunVideoParameters{
			Resolution: "1080P",
			Duration:   5,
		},
	}
	ratios, err := ProcessCtyunOtherRatios(req1080p)
	require.NoError(t, err)
	expected1080pRatio := 1.0 / 0.6
	require.InDelta(t, expected1080pRatio, ratios["resolution-1080P"], 0.0001)
}

func TestProcessCtyunOtherRatiosZeroDurationSafety(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.Init(testRelayInfo())

	// If metadata or user sets duration <= 0, convertToCtyunRequest clamps it to 5
	req := relaycommon.TaskSubmitReq{
		Model:    "happyhorse-1.1-t2v",
		Prompt:   "safely clamp duration",
		Duration: 0,
		Metadata: map[string]interface{}{
			"parameters": map[string]interface{}{
				"duration": 0,
			},
		},
	}

	ctyunReq, err := adaptor.convertToCtyunRequest(testRelayInfo(), req)
	require.NoError(t, err)
	require.Equal(t, 5, ctyunReq.Parameters.Duration)
}

func TestParseTaskResult(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// PENDING
	pendingJSON := `{"output":{"task_id":"123","task_status":"PENDING"}}`
	res, err := adaptor.ParseTaskResult([]byte(pendingJSON))
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusQueued, res.Status)

	// RUNNING
	runningJSON := `{"output":{"task_id":"123","task_status":"RUNNING"}}`
	res, err = adaptor.ParseTaskResult([]byte(runningJSON))
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusInProgress, res.Status)

	// SUCCEEDED
	successJSON := `{"output":{"task_id":"123","task_status":"SUCCEEDED","video_url":"https://video.mp4"}}`
	res, err = adaptor.ParseTaskResult([]byte(successJSON))
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusSuccess, res.Status)
	require.Equal(t, "https://video.mp4", res.Url)

	// FAILED
	failJSON := `{"output":{"task_id":"123","task_status":"FAILED","code":"ContentModerationViolated","message":"Sensitive content"}}`
	res, err = adaptor.ParseTaskResult([]byte(failJSON))
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusFailure, res.Status)
	require.Contains(t, res.Reason, "ContentModerationViolated")
}

func TestHappyHorseVideoEditRequestAndBilling(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.Init(testRelayInfo())

	req := relaycommon.TaskSubmitReq{
		Model:          "happyhorse-1.0-video-edit-20260618",
		Prompt:         "change background to cyberpunk city",
		InputReference: "https://example.com/source.mp4",
		Images:         []string{"https://example.com/ref1.png", "https://example.com/ref2.png"},
		Size:           "1080p",
		Duration:       6,
	}

	ctyunReq, err := adaptor.convertToCtyunRequest(testRelayInfo(), req)
	require.NoError(t, err)
	require.Len(t, ctyunReq.Input.Media, 3)
	require.Equal(t, CtyunVideoMedia{Type: "video", URL: "https://example.com/source.mp4"}, ctyunReq.Input.Media[0])
	require.Equal(t, CtyunVideoMedia{Type: "reference_image", URL: "https://example.com/ref1.png"}, ctyunReq.Input.Media[1])
	require.Equal(t, CtyunVideoMedia{Type: "reference_image", URL: "https://example.com/ref2.png"}, ctyunReq.Input.Media[2])
	require.Equal(t, "1080P", ctyunReq.Parameters.Resolution)

	ratios, err := ProcessCtyunOtherRatios(ctyunReq)
	require.NoError(t, err)
	require.InDelta(t, 1.6/0.9, ratios["resolution-1080P"], 0.0001)

	// Missing source video should fail
	_, err = adaptor.convertToCtyunRequest(testRelayInfo(), relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.0-video-edit-20260618",
		Prompt: "missing video",
	})
	require.Error(t, err)
}

func TestWanx21AndWan26BillingAndSize(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.Init(testRelayInfo())

	// wan2.6-t2v with 720p maps to 1280*720
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.6-t2v",
		Prompt: "a running horse",
		Size:   "720p",
	}
	ctyunReq, err := adaptor.convertToCtyunRequest(testRelayInfo(), req)
	require.NoError(t, err)
	require.Equal(t, "1280*720", ctyunReq.Parameters.Size)

	// wanx2.1-i2v-plus 720P ratio should be 1.0 (not 2.0)
	ratios, err := ProcessCtyunOtherRatios(&CtyunVideoRequest{
		Model:      "wanx2.1-i2v-plus",
		Parameters: &CtyunVideoParameters{Resolution: "720P"},
	})
	require.NoError(t, err)
	require.InDelta(t, 1.0, ratios["resolution-720P"], 0.0001)

	// happyhorse-1.0-t2v-20260618 1080P ratio
	ratios, err = ProcessCtyunOtherRatios(&CtyunVideoRequest{
		Model:      "happyhorse-1.0-t2v-20260618",
		Parameters: &CtyunVideoParameters{Resolution: "1080P"},
	})
	require.NoError(t, err)
	require.InDelta(t, 1.6/0.9, ratios["resolution-1080P"], 0.0001)
}

func TestNewWanModelsConversionAndBilling(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.Init(testRelayInfo())

	// 1. wan3.0-video multimodal + 1080P (4x)
	req30 := relaycommon.TaskSubmitReq{
		Model:          "wan3.0-video",
		Prompt:         "cinematic scene",
		Images:         []string{"https://example.com/a.png", "https://example.com/b.mp3"},
		InputReference: "https://example.com/ref.mp4",
		Size:           "1080p",
	}
	ctyun30, err := adaptor.convertToCtyunRequest(testRelayInfo(), req30)
	require.NoError(t, err)
	require.Len(t, ctyun30.Input.Media, 3)
	require.Equal(t, CtyunVideoMedia{Type: "reference_image", URL: "https://example.com/a.png"}, ctyun30.Input.Media[0])
	require.Equal(t, CtyunVideoMedia{Type: "reference_audio", URL: "https://example.com/b.mp3"}, ctyun30.Input.Media[1])
	require.Equal(t, CtyunVideoMedia{Type: "reference_video", URL: "https://example.com/ref.mp4"}, ctyun30.Input.Media[2])
	r30, err := ProcessCtyunOtherRatios(ctyun30)
	require.NoError(t, err)
	require.InDelta(t, 4.0, r30["resolution-1080P"], 0.0001)

	// 2. wan2.6-i2v-flash with audio=false -> 0.5x
	silent := false
	rFlash, err := ProcessCtyunOtherRatios(&CtyunVideoRequest{
		Model: "wan2.6-i2v-flash",
		Parameters: &CtyunVideoParameters{
			Resolution: "1080P",
			Audio:      &silent,
		},
	})
	require.NoError(t, err)
	require.InDelta(t, 0.5/0.3, rFlash["resolution-1080P"], 0.0001)
	require.InDelta(t, 0.5, rFlash["audio"], 0.0001)

	// 3. wan2.2-kf2v-flash first & last frame
	ctyunKF, err := adaptor.convertToCtyunRequest(testRelayInfo(), relaycommon.TaskSubmitReq{
		Model:  "wan2.2-kf2v-flash",
		Prompt: "smooth transition",
		Images: []string{"https://example.com/f1.png", "https://example.com/f2.png"},
		Size:   "1080p",
	})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/f1.png", ctyunKF.Input.FirstFrameURL)
	require.Equal(t, "https://example.com/f2.png", ctyunKF.Input.LastFrameURL)

	// 4. wan2.2-animate-mix image_url + video_url + image2video endpoint + results.video_url
	mixInfo := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://ai.ctaigw.cn/v1",
			UpstreamModelName: "wan2.2-animate-mix",
		},
	}
	adaptor.Init(mixInfo)
	mixURL, err := adaptor.BuildRequestURL(mixInfo)
	require.NoError(t, err)
	require.Equal(t, "https://ai.ctaigw.cn/v1/services/aigc/image2video/video-synthesis", mixURL)

	ctyunMix, err := adaptor.convertToCtyunRequest(testRelayInfo(), relaycommon.TaskSubmitReq{
		Model:          "wan2.2-animate-mix",
		Prompt:         "swap character",
		Image:          "https://example.com/char.png",
		InputReference: "https://example.com/dance.mp4",
		Metadata: map[string]any{
			"parameters": map[string]any{"mode": "wan-pro"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/char.png", ctyunMix.Input.ImageURL)
	require.Equal(t, "https://example.com/dance.mp4", ctyunMix.Input.VideoURL)
	require.Equal(t, "wan-pro", ctyunMix.Parameters.Mode)
	rMix, err := ProcessCtyunOtherRatios(ctyunMix)
	require.NoError(t, err)
	require.InDelta(t, 1.5, rMix["mode"], 0.0001)

	mixRes, err := adaptor.ParseTaskResult([]byte(`{"output":{"task_id":"t1","task_status":"SUCCEEDED","results":{"video_url":"https://example.com/out.mp4"}}}`))
	require.NoError(t, err)
	require.Equal(t, "https://example.com/out.mp4", mixRes.Url)

	// 5. wan2.6-r2v uses input.reference_urls and parameters.size
	ctyun26R2V, err := adaptor.convertToCtyunRequest(testRelayInfo(), relaycommon.TaskSubmitReq{
		Model:          "wan2.6-r2v",
		Prompt:         "character1 running",
		InputReference: "https://example.com/ref.mp4",
		Size:           "720p",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"https://example.com/ref.mp4"}, ctyun26R2V.Input.ReferenceURLs)
	require.Equal(t, "1280*720", ctyun26R2V.Parameters.Size)
}
