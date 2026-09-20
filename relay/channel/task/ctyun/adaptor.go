package ctyun

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// ============================
// Request / Response structures
// ============================

// CtyunVideoRequest 天翼云视频生成请求（兼容 DashScope 异步协议）
type CtyunVideoRequest struct {
	Model      string                `json:"model"`
	Input      CtyunVideoInput       `json:"input"`
	Parameters *CtyunVideoParameters `json:"parameters,omitempty"`
}

// CtyunVideoMedia 描述天翼云媒体输入
type CtyunVideoMedia struct {
	Type string `json:"type"` // reference_image, first_frame, last_frame, driving_audio, first_clip
	URL  string `json:"url"`
}

// CtyunVideoInput 视频输入参数
type CtyunVideoInput struct {
	Prompt         string            `json:"prompt,omitempty"`          // 文本提示词
	Media          []CtyunVideoMedia `json:"media,omitempty"`           // 媒体素材数组
	ImgURL         string            `json:"img_url,omitempty"`         // 兼容老版本图生视频
	FirstFrameURL  string            `json:"first_frame_url,omitempty"` // 首帧图片URL
	LastFrameURL   string            `json:"last_frame_url,omitempty"`  // 尾帧图片URL
	AudioURL       string            `json:"audio_url,omitempty"`       // 音频URL
	NegativePrompt string            `json:"negative_prompt,omitempty"` // 反向提示词
}

// CtyunVideoParameters 视频参数
type CtyunVideoParameters struct {
	Resolution   string `json:"resolution,omitempty"`    // 分辨率: 480P/720P/1080P
	Ratio        string `json:"ratio,omitempty"`         // 宽高比: 16:9/9:16/1:1等 (HappyHorse)
	Size         string `json:"size,omitempty"`          // 尺寸: 如 "1920*1080"
	Duration     int    `json:"duration,omitempty"`      // 时长: 2-15秒
	PromptExtend bool   `json:"prompt_extend,omitempty"` // 是否开启prompt智能改写
	Watermark    bool   `json:"watermark,omitempty"`     // 是否添加水印
	Audio        *bool  `json:"audio,omitempty"`         // 是否生成有声视频
	Seed         int    `json:"seed,omitempty"`          // 随机数种子
}

// CtyunVideoResponse 天翼云视频响应
type CtyunVideoResponse struct {
	Output    CtyunVideoOutput `json:"output"`
	RequestID string           `json:"request_id"`
	Code      string           `json:"code,omitempty"`
	Message   string           `json:"message,omitempty"`
	Usage     *CtyunUsage      `json:"usage,omitempty"`
	Error     *CtyunError      `json:"error,omitempty"`
}

type CtyunError struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// CtyunVideoOutput 输出信息
type CtyunVideoOutput struct {
	TaskID        string `json:"task_id"`
	TaskStatus    string `json:"task_status"`
	SubmitTime    string `json:"submit_time,omitempty"`
	ScheduledTime string `json:"scheduled_time,omitempty"`
	EndTime       string `json:"end_time,omitempty"`
	OrigPrompt    string `json:"orig_prompt,omitempty"`
	ActualPrompt  string `json:"actual_prompt,omitempty"`
	VideoURL      string `json:"video_url,omitempty"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

// CtyunUsage 使用统计
type CtyunUsage struct {
	Duration            dto.IntValue `json:"duration,omitempty"`
	InputVideoDuration  dto.IntValue `json:"input_video_duration,omitempty"`
	OutputVideoDuration dto.IntValue `json:"output_video_duration,omitempty"`
	VideoCount          dto.IntValue `json:"video_count,omitempty"`
	SR                  dto.IntValue `json:"SR,omitempty"`
	Ratio               string       `json:"ratio,omitempty"`
}

// ============================
// Model Helpers
// ============================

func isHappyHorseT2V(model string) bool {
	return strings.HasPrefix(model, "happyhorse") && strings.Contains(model, "t2v")
}

func isHappyHorseI2V(model string) bool {
	return strings.HasPrefix(model, "happyhorse") && strings.Contains(model, "i2v")
}

func isHappyHorseR2V(model string) bool {
	return strings.HasPrefix(model, "happyhorse") && strings.Contains(model, "r2v")
}

func isHappyHorse(model string) bool {
	return strings.HasPrefix(model, "happyhorse")
}

func isWanx27(model string) bool {
	return strings.HasPrefix(model, "wan2.7")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstTaskImage(req relaycommon.TaskSubmitReq) string {
	if image := strings.TrimSpace(req.Image); image != "" {
		return image
	}
	for _, image := range req.Images {
		if trimmed := strings.TrimSpace(image); trimmed != "" {
			return trimmed
		}
	}
	if inputReference := strings.TrimSpace(req.InputReference); inputReference != "" {
		return inputReference
	}
	return ""
}

func secondTaskImage(req relaycommon.TaskSubmitReq) string {
	nonEmptyImages := 0
	for _, image := range req.Images {
		trimmed := strings.TrimSpace(image)
		if trimmed == "" {
			continue
		}
		nonEmptyImages++
		if nonEmptyImages == 2 {
			return trimmed
		}
	}
	return ""
}

var (
	size480p = []string{
		"832*480", "480*832", "624*624",
		"832x480", "480x832", "624x624",
	}
	size720p = []string{
		"1280*720", "720*1280", "960*960", "1088*832", "832*1088",
		"1280x720", "720x1280", "960x960", "1088x832", "832x1088",
	}
	size1080p = []string{
		"1920*1080", "1080*1920", "1440*1440", "1632*1248", "1248*1632",
		"1920x1080", "1080x1920", "1440x1440", "1632x1248", "1248x1632",
	}
)

func sizeToResolution(size string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(size))
	if lo.Contains(size480p, s) {
		return "480P", nil
	} else if lo.Contains(size720p, s) {
		return "720P", nil
	} else if lo.Contains(size1080p, s) {
		return "1080P", nil
	}

	// 面积估算兜底
	var w, h int
	if strings.Contains(s, "*") {
		parts := strings.Split(s, "*")
		if len(parts) == 2 {
			w, _ = strconv.Atoi(parts[0])
			h, _ = strconv.Atoi(parts[1])
		}
	} else if strings.Contains(s, "x") {
		parts := strings.Split(s, "x")
		if len(parts) == 2 {
			w, _ = strconv.Atoi(parts[0])
			h, _ = strconv.Atoi(parts[1])
		}
	}
	if w > 0 && h > 0 {
		area := w * h
		if area <= 832*480*1.15 {
			return "480P", nil
		} else if area <= 1280*720*1.15 {
			return "720P", nil
		}
		return "1080P", nil
	}

	return "", fmt.Errorf("invalid size: %s", size)
}

func sizeToRatio(size string) string {
	s := strings.ToLower(strings.TrimSpace(size))
	var w, h int
	if strings.Contains(s, "*") {
		parts := strings.Split(s, "*")
		if len(parts) == 2 {
			w, _ = strconv.Atoi(parts[0])
			h, _ = strconv.Atoi(parts[1])
		}
	} else if strings.Contains(s, "x") {
		parts := strings.Split(s, "x")
		if len(parts) == 2 {
			w, _ = strconv.Atoi(parts[0])
			h, _ = strconv.Atoi(parts[1])
		}
	}
	if w > 0 && h > 0 {
		if w == h {
			return "1:1"
		}
		ratioVal := float64(w) / float64(h)
		if ratioVal > 1.6 && ratioVal < 1.9 {
			return "16:9"
		} else if ratioVal > 0.5 && ratioVal < 0.65 {
			return "9:16"
		} else if ratioVal > 1.25 && ratioVal < 1.4 {
			return "4:3"
		} else if ratioVal > 0.7 && ratioVal < 0.85 {
			return "3:4"
		}
	}
	// 如果本身就是形如 "16:9", "9:16"
	if strings.Contains(s, ":") {
		return strings.TrimSpace(size)
	}
	return "16:9"
}

func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	return baseURL
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = normalizeBaseURL(info.ChannelBaseUrl)
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	return relaycommon.ValidateMultipartDirect(c, info)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/services/aigc/video-generation/video-synthesis", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable") // 天翼云同样需要此异步Header
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_task_request_failed")
	}

	ctyunReq, err := a.convertToCtyunRequest(info, taskReq)
	if err != nil {
		return nil, errors.Wrap(err, "convert_to_ctyun_request_failed")
	}
	logger.LogJson(c, "ctyun video request body", ctyunReq)

	bodyBytes, err := common.Marshal(ctyunReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal_ctyun_request_failed")
	}
	return bytes.NewReader(bodyBytes), nil
}

func (a *TaskAdaptor) convertToCtyunRequest(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*CtyunVideoRequest, error) {
	upstreamModel := req.Model
	if info != nil && info.IsModelMapped {
		upstreamModel = info.UpstreamModelName
	}

	ctyunReq := &CtyunVideoRequest{
		Model: upstreamModel,
		Input: CtyunVideoInput{
			Prompt: req.Prompt,
		},
		Parameters: &CtyunVideoParameters{
			PromptExtend: true,
			Watermark:    false,
		},
	}

	// 分辨率和宽高比处理
	resolution := "1080P"
	ratio := "16:9"
	if req.Size != "" {
		if strings.Contains(req.Size, ":") {
			ratio = strings.TrimSpace(req.Size)
		} else if strings.Contains(req.Size, "*") || strings.Contains(req.Size, "x") || strings.Contains(req.Size, "X") {
			if res, err := sizeToResolution(req.Size); err == nil {
				resolution = res
			}
			ratio = sizeToRatio(req.Size)
		} else {
			normRes := strings.ToUpper(strings.TrimSpace(req.Size))
			if !strings.HasSuffix(normRes, "P") {
				normRes += "P"
			}
			resolution = normRes
		}
	}

	if isHappyHorseT2V(upstreamModel) {
		// HappyHorse 文生视频：仅需 input.prompt，无需 media
		ctyunReq.Parameters.Resolution = resolution
		ctyunReq.Parameters.Ratio = ratio
	} else if isHappyHorseI2V(upstreamModel) {
		// HappyHorse 图生视频：单首帧 input.media: [{"type": "first_frame", "url": "..."}]
		firstImg := firstTaskImage(req)
		if firstImg == "" {
			return nil, fmt.Errorf("%s requires image input", upstreamModel)
		}
		ctyunReq.Input.Media = []CtyunVideoMedia{
			{
				Type: "first_frame",
				URL:  firstImg,
			},
		}
		ctyunReq.Parameters.Resolution = resolution
		// 图生视频比例自动跟随输入首帧，不设置 ratio
	} else if isHappyHorseR2V(upstreamModel) {
		// HappyHorse 参考生视频：支持 1-9 张参考图像
		var refImages []string
		if img := strings.TrimSpace(req.Image); img != "" {
			refImages = append(refImages, img)
		}
		for _, img := range req.Images {
			if trimmed := strings.TrimSpace(img); trimmed != "" {
				refImages = append(refImages, trimmed)
			}
		}
		if len(refImages) == 0 && strings.TrimSpace(req.InputReference) != "" {
			refImages = append(refImages, strings.TrimSpace(req.InputReference))
		}
		if len(refImages) == 0 {
			return nil, fmt.Errorf("%s requires 1-9 reference images", upstreamModel)
		}
		if len(refImages) > 9 {
			refImages = refImages[:9]
		}
		for _, img := range refImages {
			ctyunReq.Input.Media = append(ctyunReq.Input.Media, CtyunVideoMedia{
				Type: "reference_image",
				URL:  img,
			})
		}
		ctyunReq.Parameters.Resolution = resolution
		ctyunReq.Parameters.Ratio = ratio
	} else if isWanx27(upstreamModel) {
		// 万相 2.7 系列
		ctyunReq.Parameters.Resolution = resolution
		firstImg := firstTaskImage(req)
		lastImg := secondTaskImage(req)
		if firstImg != "" {
			ctyunReq.Input.Media = append(ctyunReq.Input.Media, CtyunVideoMedia{
				Type: "first_frame",
				URL:  firstImg,
			})
		}
		if lastImg != "" {
			ctyunReq.Input.Media = append(ctyunReq.Input.Media, CtyunVideoMedia{
				Type: "last_frame",
				URL:  lastImg,
			})
		}
		if len(ctyunReq.Input.Media) == 0 && !strings.Contains(upstreamModel, "t2v") {
			return nil, fmt.Errorf("%s requires image", upstreamModel)
		}
	} else {
		// 默认通用万相模式
		if strings.Contains(upstreamModel, "t2v") {
			if req.Size != "" && strings.Contains(req.Size, "*") {
				ctyunReq.Parameters.Size = req.Size
			} else {
				ctyunReq.Parameters.Size = "1920*1080"
			}
		} else {
			ctyunReq.Parameters.Resolution = resolution
			ctyunReq.Input.ImgURL = firstTaskImage(req)
		}
	}

	// 时长处理
	if req.Duration > 0 {
		ctyunReq.Parameters.Duration = req.Duration
	} else if req.Seconds != "" {
		if seconds, err := strconv.Atoi(req.Seconds); err == nil {
			ctyunReq.Parameters.Duration = seconds
		}
	}
	if ctyunReq.Parameters.Duration <= 0 {
		ctyunReq.Parameters.Duration = 5 // 默认5秒
	}

	// 从 metadata 提取额外参数
	if req.Metadata != nil {
		if metadataBytes, err := common.Marshal(req.Metadata); err == nil {
			_ = common.Unmarshal(metadataBytes, ctyunReq)
		}
	}

	// 安全校验：防止 metadata 注入非法负数或零覆盖时长
	if ctyunReq.Parameters.Duration <= 0 {
		ctyunReq.Parameters.Duration = 5
	}

	return ctyunReq, nil
}

func ProcessCtyunOtherRatios(ctyunReq *CtyunVideoRequest) (map[string]float64, error) {
	otherRatios := make(map[string]float64)

	// 倍率字典
	// HappyHorse 官方标准：以 720P (0.9元/秒) 为基准 1.0
	// 1.1 系列 1080P: 1.2 / 0.9 ≈ 1.3333
	// 1.0 系列 1080P: 1.6 / 0.9 ≈ 1.7778
	ctyunRatios := map[string]map[string]float64{
		// HappyHorse 1.1
		"happyhorse-1.1-t2v": {
			"480P":  1,
			"720P":  1,
			"1080P": 1.2 / 0.9,
		},
		"happyhorse-1.1-i2v": {
			"480P":  1,
			"720P":  1,
			"1080P": 1.2 / 0.9,
		},
		"happyhorse-1.1-r2v": {
			"480P":  1,
			"720P":  1,
			"1080P": 1.2 / 0.9,
		},

		// HappyHorse 1.0
		"happyhorse-1.0-t2v": {
			"480P":  1,
			"720P":  1,
			"1080P": 1.6 / 0.9,
		},
		"happyhorse-1.0-i2v": {
			"480P":  1,
			"720P":  1,
			"1080P": 1.6 / 0.9,
		},
		"happyhorse-1.0-i2v-20260618": {
			"480P":  1,
			"720P":  1,
			"1080P": 1.6 / 0.9,
		},
		"happyhorse-1.0-r2v": {
			"480P":  1,
			"720P":  1,
			"1080P": 1.6 / 0.9,
		},
		"happyhorse-1.0-r2v-20260618": {
			"480P":  1,
			"720P":  1,
			"1080P": 1.6 / 0.9,
		},

		// 阿里万相原生定价倍率
		"wan2.7-i2v": {
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"wan2.7-t2v": {
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"wan2.6-i2v": {
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"wan2.5-t2v-preview": {
			"480P":  1,
			"720P":  2,
			"1080P": 1 / 0.3,
		},
		"wan2.5-i2v-preview": {
			"480P":  1,
			"720P":  2,
			"1080P": 1 / 0.3,
		},
		"wan2.2-t2v-plus": {
			"480P":  1,
			"720P":  2,
			"1080P": 0.7 / 0.14,
		},
		"wan2.2-i2v-plus": {
			"480P":  1,
			"720P":  2,
			"1080P": 0.7 / 0.14,
		},
		"wan2.2-kf2v-flash": {
			"480P":  1,
			"720P":  2,
			"1080P": 4.8,
		},
		"wan2.2-i2v-flash": {
			"480P": 1,
			"720P": 2,
		},
		"wan2.2-s2v": {
			"480P": 1,
			"720P": 0.9 / 0.5,
		},
		"wanx2.1-i2v-plus": {
			"480P":  1,
			"720P":  2,
			"1080P": 0.7 / 0.14,
		},
		"wanx2.1-i2v-turbo": {
			"480P": 1,
			"720P": 2,
		},
	}

	var resolution string
	if ctyunReq.Parameters.Size != "" {
		toRes, err := sizeToResolution(ctyunReq.Parameters.Size)
		if err != nil {
			return nil, err
		}
		resolution = toRes
	} else {
		resolution = strings.ToUpper(ctyunReq.Parameters.Resolution)
		if !strings.HasSuffix(resolution, "P") {
			resolution = resolution + "P"
		}
	}

	if otherRatio, ok := ctyunRatios[ctyunReq.Model]; ok {
		if ratio, ok := otherRatio[resolution]; ok {
			otherRatios[fmt.Sprintf("resolution-%s", resolution)] = ratio
		}
	}

	return otherRatios, nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	ctyunReq, err := a.convertToCtyunRequest(info, taskReq)
	if err != nil {
		return nil
	}

	otherRatios := map[string]float64{
		"seconds": float64(min(ctyunReq.Parameters.Duration, relaycommon.MaxTaskDurationSeconds)),
	}

	ratios, err := ProcessCtyunOtherRatios(ctyunReq)
	if err != nil {
		return otherRatios
	}
	for k, v := range ratios {
		otherRatios[k] = v
	}
	return otherRatios
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var ctyunResp CtyunVideoResponse
	if err := common.Unmarshal(responseBody, &ctyunResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if ctyunResp.Code != "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s: %s", ctyunResp.Code, ctyunResp.Message), "ctyun_api_error", resp.StatusCode)
		return
	}
	if ctyunResp.Error != nil && ctyunResp.Error.Code != "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s: %s", ctyunResp.Error.Code, ctyunResp.Error.Message), "ctyun_api_error", resp.StatusCode)
		return
	}

	if ctyunResp.Output.TaskID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 构造 OpenAI 视频响应
	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = info.PublicTaskID
	openAIResp.TaskID = info.PublicTaskID
	openAIResp.Model = c.GetString("model")
	if openAIResp.Model == "" && info != nil {
		openAIResp.Model = info.OriginModelName
	}
	openAIResp.Status = convertCtyunStatus(ctyunResp.Output.TaskStatus)
	openAIResp.CreatedAt = common.GetTimestamp()

	c.JSON(http.StatusOK, openAIResp)
	return ctyunResp.Output.TaskID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	base := normalizeBaseURL(baseUrl)
	uri := fmt.Sprintf("%s/v1/tasks/%s", base, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var ctyunResp CtyunVideoResponse
	if err := common.Unmarshal(respBody, &ctyunResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	switch ctyunResp.Output.TaskStatus {
	case "PENDING":
		taskResult.Status = model.TaskStatusQueued
	case "RUNNING":
		taskResult.Status = model.TaskStatusInProgress
	case "SUCCEEDED":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Url = ctyunResp.Output.VideoURL
	case "FAILED", "CANCELED", "UNKNOWN":
		taskResult.Status = model.TaskStatusFailure
		if ctyunResp.Message != "" {
			taskResult.Reason = ctyunResp.Message
		} else if ctyunResp.Output.Message != "" {
			taskResult.Reason = fmt.Sprintf("task failed, code: %s, message: %s", ctyunResp.Output.Code, ctyunResp.Output.Message)
		} else if ctyunResp.Error != nil && ctyunResp.Error.Message != "" {
			taskResult.Reason = fmt.Sprintf("task failed, code: %s, message: %s", ctyunResp.Error.Code, ctyunResp.Error.Message)
		} else {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusQueued
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var ctyunResp CtyunVideoResponse
	if err := common.Unmarshal(task.Data, &ctyunResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal ctyun response failed")
	}

	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = task.TaskID
	openAIResp.Status = convertCtyunStatus(ctyunResp.Output.TaskStatus)
	openAIResp.Model = task.Properties.OriginModelName
	openAIResp.SetProgressStr(task.Progress)
	openAIResp.CreatedAt = task.CreatedAt
	openAIResp.CompletedAt = task.UpdatedAt

	openAIResp.SetMetadata("url", ctyunResp.Output.VideoURL)

	if ctyunResp.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    ctyunResp.Code,
			Message: ctyunResp.Message,
		}
	} else if ctyunResp.Output.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    ctyunResp.Output.Code,
			Message: ctyunResp.Output.Message,
		}
	} else if ctyunResp.Error != nil && ctyunResp.Error.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    ctyunResp.Error.Code,
			Message: ctyunResp.Error.Message,
		}
	}

	return common.Marshal(openAIResp)
}

func convertCtyunStatus(ctyunStatus string) string {
	switch ctyunStatus {
	case "PENDING":
		return dto.VideoStatusQueued
	case "RUNNING":
		return dto.VideoStatusInProgress
	case "SUCCEEDED":
		return dto.VideoStatusCompleted
	case "FAILED", "CANCELED", "UNKNOWN":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}
