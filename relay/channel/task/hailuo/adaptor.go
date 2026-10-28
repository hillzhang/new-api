package hailuo

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// https://platform.minimaxi.com/docs/api-reference/video-generation-intro
type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
	isRegen     bool
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	if info != nil && info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	taskErr = relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
	if taskErr != nil {
		// 若因未传 prompt 校验失败，检查是否为基于 source_task_id 的再生成（官方无需 prompt）
		if strings.Contains(taskErr.Message, "prompt is required") {
			var rawMap map[string]interface{}
			if err := common.UnmarshalBodyReusable(c, &rawMap); err == nil && rawMap != nil {
				sourceTaskID := ""
				if stID, ok := rawMap["source_task_id"].(string); ok && stID != "" {
					sourceTaskID = stID
				} else if meta, ok := rawMap["metadata"].(map[string]interface{}); ok {
					if stID, ok := meta["source_task_id"].(string); ok {
						sourceTaskID = stID
					}
				}
				if sourceTaskID != "" {
					taskReq := relaycommon.TaskSubmitReq{
						Model:  info.OriginModelName,
						Prompt: "video regeneration",
						Metadata: map[string]interface{}{
							"source_task_id": sourceTaskID,
						},
					}
					if cb, ok := rawMap["callback_url"].(string); ok && cb != "" {
						taskReq.Metadata["callback_url"] = cb
					}
					if wm, ok := rawMap["aigc_watermark"].(bool); ok {
						taskReq.Metadata["aigc_watermark"] = wm
					}
					if info != nil {
						if info.TaskRelayInfo == nil {
							info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
						}
						info.Action = constant.TaskActionGenerate
					}
					c.Set("task_request", taskReq)
					a.isRegen = true
					return nil
				}
			}
		}
		return taskErr
	}

	if taskReq, err := relaycommon.GetTaskRequest(c); err == nil {
		a.supplementMetadataFromRawBody(c, &taskReq)
		a.isRegen = a.checkIsRegen(&taskReq, info)
		c.Set("task_request", taskReq)
	}
	return nil
}

func (a *TaskAdaptor) supplementMetadataFromRawBody(c *gin.Context, req *relaycommon.TaskSubmitReq) {
	var rawMap map[string]interface{}
	if err := common.UnmarshalBodyReusable(c, &rawMap); err == nil && rawMap != nil {
		if req.Metadata == nil {
			req.Metadata = make(map[string]interface{})
		}
		if stID, ok := rawMap["source_task_id"].(string); ok && stID != "" {
			if _, exists := req.Metadata["source_task_id"]; !exists {
				req.Metadata["source_task_id"] = stID
			}
		}
		if bv, ok := rawMap["base_video"].(string); ok && bv != "" {
			if _, exists := req.Metadata["base_video"]; !exists {
				req.Metadata["base_video"] = bv
			}
		}
		if cb, ok := rawMap["callback_url"].(string); ok && cb != "" {
			if _, exists := req.Metadata["callback_url"]; !exists {
				req.Metadata["callback_url"] = cb
			}
		}
		if wm, ok := rawMap["aigc_watermark"].(bool); ok {
			if _, exists := req.Metadata["aigc_watermark"]; !exists {
				req.Metadata["aigc_watermark"] = wm
			}
		}
	}
}

func (a *TaskAdaptor) checkIsRegen(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) bool {
	if info.UpstreamModelName == ModelH3Regeneration {
		return true
	}
	if req.Mode == "regeneration" || req.Mode == "regen" {
		return true
	}
	if req.Metadata != nil {
		if stID, ok := req.Metadata["source_task_id"].(string); ok && stID != "" {
			return true
		}
		if bv, ok := req.Metadata["base_video"].(string); ok && bv != "" {
			return true
		}
		if m, ok := req.Metadata["mode"].(string); ok && (m == "regeneration" || m == "regen") {
			return true
		}
		if tt, ok := req.Metadata["task_type"].(string); ok && (tt == "regeneration" || tt == "regen") {
			return true
		}
	}
	return false
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if a.isRegen || info.UpstreamModelName == ModelH3Regeneration {
		return fmt.Sprintf("%s%s", a.baseURL, VideoRegenerationEndpoint), nil
	}
	if IsH3Model(info.UpstreamModelName) {
		return fmt.Sprintf("%s%s", a.baseURL, TextToVideoV2Endpoint), nil
	}
	return fmt.Sprintf("%s%s", a.baseURL, TextToVideoEndpoint), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil, fmt.Errorf("invalid request type in context")
	}

	var data []byte
	if IsH3Model(info.UpstreamModelName) {
		v2Payload, err := a.convertToV2RequestPayload(&req, info)
		if err != nil {
			return nil, errors.Wrap(err, "convert v2 request payload failed")
		}
		data, err = common.Marshal(v2Payload)
		if err != nil {
			return nil, err
		}
	} else {
		body, err := a.convertToRequestPayload(&req, info)
		if err != nil {
			return nil, errors.Wrap(err, "convert request payload failed")
		}
		data, err = common.Marshal(body)
		if err != nil {
			return nil, err
		}
	}

	return bytes.NewReader(data), nil
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

	if IsH3Model(info.UpstreamModelName) {
		var v2Resp VideoGenerationV2Resp
		if err := common.Unmarshal(responseBody, &v2Resp); err != nil {
			taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
			return
		}
		if v2Resp.TaskID == "" {
			errMsg := "minimax v2 api error"
			if v2Resp.Error != nil && v2Resp.Error.Message != "" {
				errMsg = fmt.Sprintf("minimax v2 api error: %s", v2Resp.Error.Message)
			} else if v2Resp.BaseResp != nil && v2Resp.BaseResp.StatusMsg != "" {
				errMsg = fmt.Sprintf("minimax v2 api error: %s", v2Resp.BaseResp.StatusMsg)
			} else {
				errMsg = fmt.Sprintf("minimax v2 api error: %s", responseBody)
			}
			taskErr = service.TaskErrorWrapper(errors.New(errMsg), "minimax_v2_error", http.StatusBadRequest)
			return
		}

		ov := dto.NewOpenAIVideo()
		ov.ID = info.PublicTaskID
		ov.TaskID = info.PublicTaskID
		ov.CreatedAt = time.Now().Unix()
		ov.Model = info.OriginModelName

		c.JSON(http.StatusOK, ov)
		return v2Resp.TaskID, responseBody, nil
	}

	var hResp VideoResponse
	if err := common.Unmarshal(responseBody, &hResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if hResp.BaseResp.StatusCode != StatusSuccess {
		taskErr = service.TaskErrorWrapper(
			fmt.Errorf("hailuo api error: %s", hResp.BaseResp.StatusMsg),
			strconv.Itoa(hResp.BaseResp.StatusCode),
			http.StatusBadRequest,
		)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return hResp.TaskID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	var uri string
	if isV2Task(body) {
		uri = fmt.Sprintf("%s%s/%s", baseUrl, QueryTaskV2Endpoint, taskID)
	} else {
		uri = fmt.Sprintf("%s%s?task_id=%s", baseUrl, QueryTaskEndpoint, taskID)
	}

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func isV2Task(body map[string]any) bool {
	if m, ok := body["model"].(string); ok && IsH3Model(m) {
		return true
	}
	if _, ok := body["task"]; ok {
		return true
	}
	return false
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// EstimateBilling 根据用户请求的时长、分辨率、输入参考视频与超额图片计算 OtherRatios。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if !IsH3Model(info.UpstreamModelName) {
		return nil
	}

	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	modelConfig := GetModelConfig(info.UpstreamModelName)

	// 1. 确定输出时长 (秒)
	duration := 5
	if taskReq.Duration > 0 {
		duration = taskReq.Duration
	} else if taskReq.Seconds != "" {
		if s, err := strconv.Atoi(taskReq.Seconds); err == nil && s > 0 {
			duration = s
		}
	}
	duration = min(duration, relaycommon.MaxTaskDurationSeconds)

	// 2. 确定输出分辨率与分辨率倍率
	isRegen := a.isRegen || a.checkIsRegen(&taskReq, info)
	var resRatio float64
	resolution := modelConfig.DefaultResolution
	if isRegen {
		resolution = Resolution2K
		// 视频再生成固定输出 2K，官方实际价格 0.30 元/秒
		if info.UpstreamModelName == ModelH3Regeneration {
			resRatio = 1.0 // 该虚拟模型在系统后台基准价即为 0.30 元/秒
		} else {
			// 用户直接使用 MiniMax-H3 原生模型（基准价 0.50 元/秒），按 0.30 / 0.50 = 0.60 折算，确保扣费分文不差
			baseModelPrice := h3PriceTable[info.UpstreamModelName][h3PriceKey{}]
			if baseModelPrice > 0 {
				resRatio = 0.30 / baseModelPrice
			} else {
				resRatio = 0.60
			}
		}
	} else {
		if taskReq.Size != "" {
			resolution = a.parseResolutionFromSize(taskReq.Size, modelConfig)
		}
		if taskReq.Metadata != nil {
			if res, ok := taskReq.Metadata["resolution"].(string); ok && res != "" {
				resolution = a.parseResolutionFromSize(res, modelConfig)
			}
		}
		resRatio, _ = GetH3ResolutionRatio(info.UpstreamModelName, resolution)
	}

	// 3. 统计图片数量
	imageCount := a.countRequestImages(&taskReq)

	// 4. 统计输入参考视频时长 (秒)
	inputVideoSec := a.extractInputVideoSeconds(&taskReq)

	// 5. 基础视频费用 (基准单价 * 秒数 * 分辨率倍率)
	basePrice := h3PriceTable[info.UpstreamModelName][h3PriceKey{}]
	baseVideoCost := basePrice * float64(duration) * resRatio

	// 6. 计算超额图片费用 (原 768P 生成任务中使用的输入图片重新计费：5张内免费，超出 0.15 元/张)
	freeImages := h3FreeImageCount[info.UpstreamModelName]
	extraImages := 0
	if imageCount > freeImages {
		extraImages = imageCount - freeImages
	}
	var extraImageCost float64
	if extraImages > 0 {
		imageModelName := fmt.Sprintf("%s-image", info.OriginModelName)
		imagePrice, ok := ratio_setting.GetModelPrice(imageModelName, false)
		if !ok || imagePrice <= 0 {
			imagePrice = h3DefaultImagePrice[info.UpstreamModelName]
		}
		extraImageCost = float64(extraImages) * imagePrice
	}

	// 7. 计算输入参考视频动作费用 (官方规则：按输入视频时长及生成视频目标分辨率计费)
	var inputVideoCost float64
	if inputVideoSec > 0 {
		videoInputPrice := GetH3VideoInputPricePerSec(info.UpstreamModelName, resolution)
		inputVideoCost = float64(inputVideoSec) * videoInputPrice
	}

	// 8. 组装 OtherRatios
	otherRatios := map[string]float64{
		"seconds": float64(duration),
	}
	if resRatio != 1.0 {
		otherRatios["resolution"] = resRatio
	}
	if (extraImageCost+inputVideoCost) > 0 && baseVideoCost > 0 {
		extraRatio := 1.0 + (extraImageCost+inputVideoCost)/baseVideoCost
		otherRatios["extra_assets"] = extraRatio
	}

	return otherRatios
}

func (a *TaskAdaptor) countRequestImages(taskReq *relaycommon.TaskSubmitReq) int {
	count := 0
	if taskReq.Image != "" {
		count++
	}
	count += len(taskReq.Images)
	if taskReq.Metadata != nil {
		if v, ok := taskReq.Metadata["subject_reference"]; ok {
			if subRefs, ok := v.([]any); ok {
				for _, sub := range subRefs {
					if subMap, ok := sub.(map[string]any); ok {
						if imgs, ok := subMap["image"].([]any); ok {
							count += len(imgs)
						}
					}
				}
			}
		}
		if v, ok := taskReq.Metadata["first_frame_image"].(string); ok && v != "" && taskReq.Image == "" {
			count++
		}
		if v, ok := taskReq.Metadata["last_frame_image"].(string); ok && v != "" {
			count++
		}
		for _, key := range []string{"reference_image", "reference_images"} {
			if v, ok := taskReq.Metadata[key]; ok {
				if s, ok := v.(string); ok && s != "" {
					count++
				} else if list, ok := v.([]any); ok {
					count += len(list)
				} else if list, ok := v.([]string); ok {
					count += len(list)
				}
			}
		}
	}
	return count
}

func (a *TaskAdaptor) extractInputVideoSeconds(taskReq *relaycommon.TaskSubmitReq) int {
	if taskReq.Metadata == nil {
		return 0
	}
	if v, ok := taskReq.Metadata["video_seconds"]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case string:
			if s, err := strconv.Atoi(n); err == nil {
				return s
			}
		}
	}
	if v, ok := taskReq.Metadata["input_video_seconds"]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case string:
			if s, err := strconv.Atoi(n); err == nil {
				return s
			}
		}
	}
	return 0
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*VideoRequest, error) {
	modelConfig := GetModelConfig(info.UpstreamModelName)
	duration := DefaultDuration
	if req.Duration > 0 {
		duration = req.Duration
	} else if req.Seconds != "" {
		if s, err := strconv.Atoi(req.Seconds); err == nil && s > 0 {
			duration = s
		}
	} else if IsH3Model(info.UpstreamModelName) {
		duration = 5
	}
	resolution := modelConfig.DefaultResolution
	if req.Size != "" {
		resolution = a.parseResolutionFromSize(req.Size, modelConfig)
	}

	videoRequest := &VideoRequest{
		Model:      info.UpstreamModelName,
		Prompt:     req.Prompt,
		Duration:   &duration,
		Resolution: resolution,
	}
	if req.Image != "" {
		videoRequest.FirstFrameImage = req.Image
	}
	if len(req.Images) > 0 {
		if videoRequest.FirstFrameImage == "" {
			videoRequest.FirstFrameImage = req.Images[0]
			if len(req.Images) > 1 {
				videoRequest.LastFrameImage = req.Images[1]
			}
		} else if videoRequest.LastFrameImage == "" {
			videoRequest.LastFrameImage = req.Images[0]
		}
	}

	if err := req.UnmarshalMetadata(&videoRequest); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata to video request failed")
	}

	return videoRequest, nil
}

func (a *TaskAdaptor) convertToV2RequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*VideoGenerationV2Req, error) {
	modelConfig := GetModelConfig(info.UpstreamModelName)

	// 1. duration
	duration := 5
	if req.Duration > 0 {
		duration = req.Duration
	} else if req.Seconds != "" {
		if s, err := strconv.Atoi(req.Seconds); err == nil && s > 0 {
			duration = s
		}
	}
	if info.UpstreamModelName == ModelH3Max {
		if duration < 5 {
			duration = 5
		}
	} else {
		if duration < 4 {
			duration = 4
		}
	}
	if duration > 15 {
		duration = 15
	}

	// 2. resolution
	resolution := modelConfig.DefaultResolution
	if req.Size != "" {
		resolution = a.parseResolutionFromSize(req.Size, modelConfig)
	}
	if req.Metadata != nil {
		if res, ok := req.Metadata["resolution"].(string); ok && res != "" {
			resolution = a.parseResolutionFromSize(res, modelConfig)
		}
	}
	if info.UpstreamModelName == ModelH3 {
		if resolution != Resolution2K {
			resolution = Resolution768P
		}
	} else if info.UpstreamModelName == ModelH3Max {
		if resolution != Resolution768P {
			resolution = Resolution480P
		}
	} else if info.UpstreamModelName == ModelH3Regeneration {
		resolution = Resolution2K
	}

	// 3. content 多模态数组
	var content []ContentItem

	// 文本提示词 (必填项)
	promptText := req.Prompt
	if promptText == "" {
		promptText = "generate video"
	}
	content = append(content, ContentItem{
		Type: "text",
		Text: promptText,
	})

	hasFrameImages := false

	// 首帧
	firstFrame := req.Image
	if len(req.Images) > 0 && firstFrame == "" {
		firstFrame = req.Images[0]
	}
	if req.Metadata != nil {
		if ff, ok := req.Metadata["first_frame_image"].(string); ok && ff != "" {
			firstFrame = ff
		}
	}
	if firstFrame != "" {
		hasFrameImages = true
		content = append(content, ContentItem{
			Type:     "image_url",
			ImageURL: &MediaURL{URL: firstFrame},
			Role:     "first_frame",
		})
	}

	// 尾帧
	var lastFrame string
	if len(req.Images) > 1 {
		lastFrame = req.Images[1]
	}
	if req.Metadata != nil {
		if lf, ok := req.Metadata["last_frame_image"].(string); ok && lf != "" {
			lastFrame = lf
		}
	}
	if lastFrame != "" {
		hasFrameImages = true
		content = append(content, ContentItem{
			Type:     "image_url",
			ImageURL: &MediaURL{URL: lastFrame},
			Role:     "last_frame",
		})
	}

	// 多模态参考生视频 (仅在未传首尾帧时生效，二者互斥)
	if !hasFrameImages && req.Metadata != nil {
		// 参考图片 (最多 9 张)
		for _, key := range []string{"reference_image", "reference_images"} {
			if v, ok := req.Metadata[key]; ok {
				if s, ok := v.(string); ok && s != "" {
					content = append(content, ContentItem{
						Type:     "image_url",
						ImageURL: &MediaURL{URL: s},
						Role:     "reference_image",
					})
				} else if list, ok := v.([]any); ok {
					for _, item := range list {
						if imgStr, ok := item.(string); ok && imgStr != "" {
							content = append(content, ContentItem{
								Type:     "image_url",
								ImageURL: &MediaURL{URL: imgStr},
								Role:     "reference_image",
							})
						}
					}
				} else if list, ok := v.([]string); ok {
					for _, imgStr := range list {
						if imgStr != "" {
							content = append(content, ContentItem{
								Type:     "image_url",
								ImageURL: &MediaURL{URL: imgStr},
								Role:     "reference_image",
							})
						}
					}
				}
			}
		}

		// 参考视频 (最多 3 个)
		for _, key := range []string{"reference_video", "reference_videos"} {
			if v, ok := req.Metadata[key]; ok {
				if s, ok := v.(string); ok && s != "" {
					content = append(content, ContentItem{
						Type:     "video_url",
						VideoURL: &MediaURL{URL: s},
						Role:     "reference_video",
					})
				} else if list, ok := v.([]any); ok {
					for _, item := range list {
						if videoStr, ok := item.(string); ok && videoStr != "" {
							content = append(content, ContentItem{
								Type:     "video_url",
								VideoURL: &MediaURL{URL: videoStr},
								Role:     "reference_video",
							})
						}
					}
				} else if list, ok := v.([]string); ok {
					for _, videoStr := range list {
						if videoStr != "" {
							content = append(content, ContentItem{
								Type:     "video_url",
								VideoURL: &MediaURL{URL: videoStr},
								Role:     "reference_video",
							})
						}
					}
				}
			}
		}

		// 参考音频 (最多 3 个)
		for _, key := range []string{"reference_audio", "reference_audios"} {
			if v, ok := req.Metadata[key]; ok {
				if s, ok := v.(string); ok && s != "" {
					content = append(content, ContentItem{
						Type:     "audio_url",
						AudioURL: &MediaURL{URL: s},
						Role:     "reference_audio",
					})
				} else if list, ok := v.([]any); ok {
					for _, item := range list {
						if audioStr, ok := item.(string); ok && audioStr != "" {
							content = append(content, ContentItem{
								Type:     "audio_url",
								AudioURL: &MediaURL{URL: audioStr},
								Role:     "reference_audio",
							})
						}
					}
				} else if list, ok := v.([]string); ok {
					for _, audioStr := range list {
						if audioStr != "" {
							content = append(content, ContentItem{
								Type:     "audio_url",
								AudioURL: &MediaURL{URL: audioStr},
								Role:     "reference_audio",
							})
						}
					}
				}
			}
		}
	}

	// 4. ratio 宽高比
	// 官方规范：
	// - 文生视频 (content 仅含 text): ratio 必填且不可为 adaptive，默认为 16:9
	// - 图生视频 (content 含 first_frame / last_frame): 恒为 adaptive
	// - 多模态参考生视频 (content 含 reference_*): 默认 adaptive，可自定义具体比例
	isPureText := len(content) == 1 && content[0].Type == "text"
	ratio := "adaptive"
	if isPureText {
		ratio = "16:9"
		if req.Size != "" {
			ratio = a.parseRatioFromSize(req.Size)
		}
	}
	if req.Metadata != nil {
		if r, ok := req.Metadata["ratio"].(string); ok && r != "" {
			ratio = r
		}
	}
	// 纯文生视频强制不允许 adaptive，若为空或误传 adaptive 则兜底回 16:9
	if isPureText && (ratio == "" || ratio == "adaptive") {
		ratio = "16:9"
	}

	upstreamModel := info.UpstreamModelName
	if info.UpstreamModelName == ModelH3Regeneration {
		upstreamModel = ModelH3 // MiniMax 官方再生成接口 model 字段固定为 MiniMax-H3
	}

	isRegen := a.isRegen || a.checkIsRegen(req, info)

	// 针对 ModelH3Regeneration 或自动识别的视频再生成专用接口 (POST /v2/video_regeneration)
	if isRegen || info.UpstreamModelName == ModelH3Regeneration {
		// 方式 A：按任务 ID 再生成 (从 metadata.source_task_id 获取)
		var sourceTaskID string
		if req.Metadata != nil {
			if stID, ok := req.Metadata["source_task_id"].(string); ok && stID != "" {
				sourceTaskID = stID
			}
		}
		if sourceTaskID != "" {
			v2Req := &VideoGenerationV2Req{
				Model:        ModelH3, // 官方再生成接口 model 字段固定为 MiniMax-H3
				SourceTaskID: sourceTaskID,
				Resolution:   Resolution2K, // 固定输出 2K
			}
			if req.Metadata != nil {
				if cb, ok := req.Metadata["callback_url"].(string); ok && cb != "" {
					v2Req.CallbackURL = cb
				}
				if wm, ok := req.Metadata["aigc_watermark"].(bool); ok {
					v2Req.AigcWatermark = &wm
				}
			}
			return v2Req, nil
		}

		// 方式 B：按源视频再生成 (从 metadata.base_video 或 input_reference 获取)
		var baseVideo string
		if req.Metadata != nil {
			if bv, ok := req.Metadata["base_video"].(string); ok && bv != "" {
				baseVideo = bv
			}
		}
		if baseVideo == "" && req.InputReference != "" {
			baseVideo = req.InputReference
		}

		hasBaseVideo := false
		for _, item := range content {
			if item.Role == "base_video" {
				hasBaseVideo = true
				break
			}
		}

		if !hasBaseVideo && baseVideo != "" {
			content = append(content, ContentItem{
				Type:     "video_url",
				VideoURL: &MediaURL{URL: baseVideo},
				Role:     "base_video",
			})
			hasBaseVideo = true
		}

		// 容错：如果用户未指定 base_video 字段，但在 reference_videos 传入了源视频，将最后一个 reference_video 转为 base_video
		if !hasBaseVideo {
			for i := len(content) - 1; i >= 0; i-- {
				if content[i].Role == "reference_video" {
					content[i].Role = "base_video"
					break
				}
			}
		}

		// 官方 /v2/video_regeneration Schema 严格不允许 duration 和 ratio 字段（由源视频决定）
		v2Req := &VideoGenerationV2Req{
			Model:      ModelH3,
			Content:    content,
			Resolution: Resolution2K,
		}
		if req.Metadata != nil {
			if cb, ok := req.Metadata["callback_url"].(string); ok && cb != "" {
				v2Req.CallbackURL = cb
			}
			if wm, ok := req.Metadata["aigc_watermark"].(bool); ok {
				v2Req.AigcWatermark = &wm
			}
		}
		return v2Req, nil
	}

	v2Req := &VideoGenerationV2Req{
		Model:      upstreamModel,
		Content:    content,
		Resolution: resolution,
		Duration:   duration,
		Ratio:      ratio,
	}

	if req.Metadata != nil {
		if extra, ok := req.Metadata["extra"]; ok && extra != nil {
			v2Req.Extra = extra
		} else if pem, ok := req.Metadata["prompt_expansion_mode"].(string); ok && pem != "" {
			v2Req.Extra = map[string]any{"prompt_expansion_mode": pem}
		}
		if cb, ok := req.Metadata["callback_url"].(string); ok && cb != "" {
			v2Req.CallbackURL = cb
		}
		if wm, ok := req.Metadata["aigc_watermark"].(bool); ok {
			v2Req.AigcWatermark = &wm
		}
	}

	return v2Req, nil
}

func (a *TaskAdaptor) parseRatioFromSize(size string) string {
	s := strings.ToLower(strings.TrimSpace(size))
	s = strings.ReplaceAll(s, "×", "x")
	s = strings.ReplaceAll(s, "*", "x")

	// 1. 若传入具体像素宽高 (如 768x768, 768×768, 1344x768)
	if parts := strings.Split(s, "x"); len(parts) == 2 {
		w, errW := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		h, errH := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if errW == nil && errH == nil && w > 0 && h > 0 {
			ratioVal := w / h
			switch {
			case math.Abs(ratioVal-1.0) < 0.05:
				return "1:1"
			case math.Abs(ratioVal-16.0/9.0) < 0.12:
				return "16:9"
			case math.Abs(ratioVal-9.0/16.0) < 0.12:
				return "9:16"
			case math.Abs(ratioVal-4.0/3.0) < 0.08:
				return "4:3"
			case math.Abs(ratioVal-3.0/4.0) < 0.08:
				return "3:4"
			case math.Abs(ratioVal-21.0/9.0) < 0.15:
				return "21:9"
			}
		}
	}

	switch {
	case strings.Contains(s, "9:16") || s == "720x1280" || s == "1080x1920":
		return "9:16"
	case strings.Contains(s, "16:9") || s == "1280x720" || s == "1920x1080":
		return "16:9"
	case strings.Contains(s, "1:1") || s == "1024x1024" || s == "768x768":
		return "1:1"
	case strings.Contains(s, "4:3") || s == "1024x768":
		return "4:3"
	case strings.Contains(s, "3:4") || s == "768x1024":
		return "3:4"
	case strings.Contains(s, "21:9"):
		return "21:9"
	default:
		return "16:9"
	}
}

func (a *TaskAdaptor) parseResolutionFromSize(size string, modelConfig ModelConfig) string {
	s := strings.ToLower(strings.TrimSpace(size))
	s = strings.ReplaceAll(s, "×", "x")
	s = strings.ReplaceAll(s, "*", "x")

	// 1. 若传入具体像素宽高 (如 768x768, 768×768, 2048x1080)
	if parts := strings.Split(s, "x"); len(parts) == 2 {
		w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
		h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
		if errW == nil && errH == nil && w > 0 && h > 0 {
			minDim := min(w, h)
			maxDim := max(w, h)
			if maxDim >= 2048 || minDim >= 1440 {
				return Resolution2K
			}
			if minDim >= 1080 {
				return Resolution1080P
			}
			if minDim >= 768 {
				return Resolution768P
			}
			if minDim >= 720 {
				return Resolution720P
			}
			if minDim >= 512 {
				return Resolution512P
			}
			if minDim >= 480 {
				return Resolution480P
			}
		}
	}

	switch {
	case strings.Contains(s, "2k") || strings.Contains(s, "2048") || strings.Contains(s, "2560"):
		return Resolution2K
	case strings.Contains(s, "1080"):
		return Resolution1080P
	case strings.Contains(s, "768"):
		return Resolution768P
	case strings.Contains(s, "720"):
		return Resolution720P
	case strings.Contains(s, "480"):
		return Resolution480P
	case strings.Contains(s, "512"):
		return Resolution512P
	default:
		return modelConfig.DefaultResolution
	}
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	// 1. 优先尝试解析 V2 任务结果
	var v2Resp QueryTaskV2Resp
	if err := common.Unmarshal(respBody, &v2Resp); err == nil && v2Resp.Task != nil {
		taskResult := relaycommon.TaskInfo{}
		switch v2Resp.Task.Status {
		case TaskV2StatusQueued:
			taskResult.Status = model.TaskStatusInProgress
			taskResult.Progress = "10%"
		case TaskV2StatusRunning:
			taskResult.Status = model.TaskStatusInProgress
			taskResult.Progress = "50%"
		case TaskV2StatusSucceeded:
			taskResult.Status = model.TaskStatusSuccess
			taskResult.Progress = "100%"
			taskResult.Url = v2Resp.Task.Content.URL
		case TaskV2StatusFailed, TaskV2StatusCancelled:
			taskResult.Status = model.TaskStatusFailure
			taskResult.Progress = "100%"
			if v2Resp.Task.Error != nil {
				taskResult.Reason = v2Resp.Task.Error.Message
			} else {
				taskResult.Reason = "task failed"
			}
		default:
			taskResult.Status = model.TaskStatusInProgress
			taskResult.Progress = "30%"
		}
		return &taskResult, nil
	}

	// 2. 否则走原有的 V1 解析逻辑
	resTask := QueryTaskResponse{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{}

	if resTask.BaseResp.StatusCode == StatusSuccess {
		taskResult.Code = 0
	} else {
		taskResult.Code = resTask.BaseResp.StatusCode
		taskResult.Reason = resTask.BaseResp.StatusMsg
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
	}

	switch resTask.Status {
	case TaskStatusPreparing, TaskStatusQueueing, TaskStatusProcessing:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
		if resTask.Status == TaskStatusProcessing {
			taskResult.Progress = "50%"
		}
	case TaskStatusSuccess:
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = a.buildVideoURL(resTask.TaskID, resTask.FileID)
	case TaskStatusFailed:
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		if taskResult.Reason == "" {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	// 1. 优先尝试解析 V2 任务结果
	var v2Resp QueryTaskV2Resp
	if err := common.Unmarshal(originTask.Data, &v2Resp); err == nil && v2Resp.Task != nil {
		openAIVideo := originTask.ToOpenAIVideo()
		if (v2Resp.Task.Status == TaskV2StatusFailed || v2Resp.Task.Status == TaskV2StatusCancelled) && v2Resp.Task.Error != nil {
			openAIVideo.Error = &dto.OpenAIVideoError{
				Message: v2Resp.Task.Error.Message,
				Code:    fmt.Sprintf("%v", v2Resp.Task.Error.Code),
			}
		}
		if v2Resp.Task.Usage != nil {
			openAIVideo.SetMetadata("usage", v2Resp.Task.Usage)
		}
		jsonData, err := common.Marshal(openAIVideo)
		if err != nil {
			return nil, errors.Wrap(err, "marshal openai video failed")
		}
		return jsonData, nil
	}

	// 2. 否则按原有的 V1 格式解析
	var hailuoResp QueryTaskResponse
	if err := common.Unmarshal(originTask.Data, &hailuoResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal hailuo task data failed")
	}

	openAIVideo := originTask.ToOpenAIVideo()
	if hailuoResp.BaseResp.StatusCode != StatusSuccess {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: hailuoResp.BaseResp.StatusMsg,
			Code:    strconv.Itoa(hailuoResp.BaseResp.StatusCode),
		}
	}

	jsonData, err := common.Marshal(openAIVideo)
	if err != nil {
		return nil, errors.Wrap(err, "marshal openai video failed")
	}

	return jsonData, nil
}

func (a *TaskAdaptor) buildVideoURL(_, fileID string) string {
	if a.apiKey == "" || a.baseURL == "" {
		return ""
	}

	url := fmt.Sprintf("%s/v1/files/retrieve?file_id=%s", a.baseURL, fileID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ""
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var retrieveResp RetrieveFileResponse
	if err := common.Unmarshal(responseBody, &retrieveResp); err != nil {
		return ""
	}

	if retrieveResp.BaseResp.StatusCode != StatusSuccess {
		return ""
	}

	return retrieveResp.File.DownloadURL
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsInt(slice []int, item int) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
