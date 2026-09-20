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
