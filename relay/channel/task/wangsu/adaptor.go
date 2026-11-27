package wangsu

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
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

type requestPayload struct {
	Model                 string           `json:"model"`
	Prompt                string           `json:"prompt"`
	Seconds               *dto.IntValue    `json:"seconds,omitempty"`
	Size                  string           `json:"size,omitempty"`
	InputReference        any              `json:"input_reference,omitempty"`
	EcaFirstFrame         string           `json:"eca_first_frame,omitempty"`
	EcaLastFrame          string           `json:"eca_last_frame,omitempty"`
	EcaVideoReference     []map[string]any `json:"eca_video_reference,omitempty"`
	EcaAudioReference     []map[string]any `json:"eca_audio_reference,omitempty"`
	EcaAudio              *dto.BoolValue   `json:"eca_audio,omitempty"`
	EcaVoice              string           `json:"eca_voice,omitempty"`
	EcaAspectRatio        string           `json:"eca_aspect_ratio,omitempty"`
	EcaMode               string           `json:"eca_mode,omitempty"`
	EcaMultiShot          *dto.BoolValue   `json:"eca_multi_shot,omitempty"`
	EcaTools              []map[string]any `json:"eca_tools,omitempty"`
	Seed                  *dto.IntValue    `json:"seed,omitempty"`
	Watermark             *dto.BoolValue   `json:"watermark,omitempty"`
	ReturnLastFrame       *dto.BoolValue   `json:"return_last_frame,omitempty"`
	ExecutionExpiresAfter *dto.IntValue    `json:"execution_expires_after,omitempty"`
	CallbackUrl           string           `json:"callback_url,omitempty"`
	ServiceTier           string           `json:"service_tier,omitempty"`
}

type responsePayload struct {
	ID     string `json:"id"`
	Object string `json:"object,omitempty"`
	Status string `json:"status,omitempty"`
}

type EcaVideoUsage struct {
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type responseTask struct {
	ID            string        `json:"id"`
	Object        string        `json:"object"`
	Model         string        `json:"model"`
	Status        string        `json:"status"`
	Size          string        `json:"size"`
	Seconds       string        `json:"seconds"`
	CreatedAt     int64         `json:"created_at"`
	ExpiresAt     int64         `json:"expires_at"`
	CompletedAt   int64         `json:"completed_at"`
	EcaAudio      bool           `json:"eca_audio"`
	Audio         bool           `json:"audio,omitempty"`
	EcaVideoUsage EcaVideoUsage  `json:"eca_video_usage"`
	Usage         *EcaVideoUsage `json:"usage,omitempty"`
	Error         struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
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
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
// 放行 seconds/duration == -1（豆包智能自动时长），其余校验与 ValidateBasicTaskRequest 对齐。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	contentType := c.GetHeader("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
	}

	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}

	// 捕获并合并顶层扁平参数到 req.Metadata，支持无 metadata 嵌套的 REST 调用
	var rawBody map[string]any
	if err := common.UnmarshalBodyReusable(c, &rawBody); err == nil && rawBody != nil {
		nativeKeys := []string{
			"seed", "watermark", "draft", "return_last_frame", "first_frame", "last_frame",
			"aspect_ratio", "eca_aspect_ratio", "eca_audio", "audio", "eca_voice", "voice",
			"eca_mode", "mode", "eca_multi_shot", "multi_shot", "tools", "eca_tools",
			"video_reference", "eca_video_reference", "audio_reference", "eca_audio_reference",
			"input_reference", "execution_expires_after", "service_tier", "callback_url",
		}
		for _, key := range nativeKeys {
			if val, exists := rawBody[key]; exists && val != nil {
				if req.Metadata == nil {
					req.Metadata = make(map[string]interface{})
				}
				if _, set := req.Metadata[key]; !set {
					req.Metadata[key] = val
				}
			}
		}
	}

	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}

	seconds := req.Duration
	if seconds == 0 && req.Seconds != "" {
		seconds, _ = strconv.Atoi(req.Seconds)
	}
	if seconds < -1 || seconds > relaycommon.MaxTaskDurationSeconds {
		return service.TaskErrorWrapperLocal(fmt.Errorf("seconds must be -1 or between 1 and %d", relaycommon.MaxTaskDurationSeconds), "invalid_seconds", http.StatusBadRequest)
	}

	if len(req.Images) == 0 && strings.TrimSpace(req.Image) != "" {
		req.Images = []string{req.Image}
	}

	if info != nil {
		if info.TaskRelayInfo == nil {
			info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
		}
		info.Action = constant.TaskActionGenerate
	}
	c.Set("task_request", req)
	return nil
}

// BuildRequestURL constructs the upstream URL: POST ${BASE_URL}/videos
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/videos", strings.TrimRight(a.baseURL, "/")), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// EstimateBilling 根据请求中的分辨率与是否包含视频输入，返回相对基准价的计费 OtherRatio。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	hasVideo := hasVideoInReq(&req)
	resolution := extractResolution(&req)
	ratio, ok := GetVideoInputRatio(info.OriginModelName, resolution, hasVideo)
	if !ok && info.ChannelMeta != nil && info.UpstreamModelName != "" && info.UpstreamModelName != info.OriginModelName {
		ratio, ok = GetVideoInputRatio(info.UpstreamModelName, resolution, hasVideo)
	}
	if !ok || ratio == 1.0 {
		return nil
	}
	return map[string]float64{"video_input": ratio}
}

// normalizeWangsuResolution 标准化分辨率档位。
// 网宿 / 豆包等上游仅接受标准档位（如 "720p", "1080p", "480p", "4k"），不接受具体的像素格式（如 "1280x720"）。
func normalizeWangsuResolution(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "720p"
	}
	switch s {
	case "480p", "720p", "1080p", "4k":
		return s
	}
	if strings.Contains(s, "1080") {
		return "1080p"
	}
	if strings.Contains(s, "4k") || strings.Contains(s, "2160") {
		return "4k"
	}
	if strings.Contains(s, "720") {
		return "720p"
	}
	if strings.Contains(s, "480") {
		return "480p"
	}
	return s
}

func extractResolution(req *relaycommon.TaskSubmitReq) string {
	var res = req.Size
	if req.Metadata != nil {
		if size, ok := req.Metadata["size"].(string); ok && size != "" {
			res = size
		} else if r, ok := req.Metadata["resolution"].(string); ok && r != "" {
			res = r
		}
	}
	return normalizeWangsuResolution(res)
}

var videoExtensions = map[string]bool{
	".mp4":  true,
	".mov":  true,
	".avi":  true,
	".mkv":  true,
	".webm": true,
	".flv":  true,
	".wmv":  true,
	".m4v":  true,
	".3gp":  true,
	".ts":   true,
	".mpeg": true,
	".mpg":  true,
}

var audioExtensions = map[string]bool{
	".mp3":  true,
	".wav":  true,
	".aac":  true,
	".m4a":  true,
	".ogg":  true,
	".flac": true,
	".wma":  true,
	".opus": true,
	".mid":  true,
	".midi": true,
}

var imageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".bmp":  true,
	".gif":  true,
	".tiff": true,
}

func getURLExtension(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if idx := strings.IndexAny(rawURL, "?#"); idx != -1 {
		rawURL = rawURL[:idx]
	}
	return strings.ToLower(path.Ext(rawURL))
}

func isImageURL(rawURL string) bool {
	if strings.HasPrefix(rawURL, "data:image/") {
		return true
	}
	ext := getURLExtension(rawURL)
	return imageExtensions[ext]
}

func isAudioURL(rawURL string) bool {
	if strings.HasPrefix(rawURL, "data:audio/") {
		return true
	}
	ext := getURLExtension(rawURL)
	return audioExtensions[ext]
}

func isVideoURL(rawURL string) bool {
	if strings.HasPrefix(rawURL, "data:video/") {
		return true
	}
	if isImageURL(rawURL) || isAudioURL(rawURL) {
		return false
	}
	ext := getURLExtension(rawURL)
	if videoExtensions[ext] {
		return true
	}
	lower := strings.ToLower(rawURL)
	if strings.Contains(lower, "video") && !strings.Contains(lower, "audio") && !strings.Contains(lower, "image") {
		return true
	}
	return false
}

func refItemContainsVideo(item any) bool {
	if item == nil {
		return false
	}
	switch m := item.(type) {
	case string:
		if isAudioURL(m) {
			return false
		}
		if isVideoURL(m) {
			return true
		}
		return strings.TrimSpace(m) != ""
	case map[string]any:
		return checkMapContainsVideo(m)
	}
	return false
}

func checkMapContainsVideo(m map[string]any) bool {
	vidVal, ok := m["video"]
	if !ok || vidVal == nil {
		// 没有 video 字段（例如只有 audio 字段），判定为纯音频，不含视频
		return false
	}
	vidStr, ok := vidVal.(string)
	if !ok || strings.TrimSpace(vidStr) == "" {
		return false
	}
	if isAudioURL(vidStr) {
		return false
	}
	return true
}

func referenceContainsVideo(vRef any) bool {
	if vRef == nil {
		return false
	}
	switch v := vRef.(type) {
	case string:
		return refItemContainsVideo(v)
	case map[string]any:
		return checkMapContainsVideo(v)
	case []any:
		for _, item := range v {
			if refItemContainsVideo(item) {
				return true
			}
		}
	case []map[string]any:
		for _, m := range v {
			if checkMapContainsVideo(m) {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if refItemContainsVideo(s) {
				return true
			}
		}
	}
	return false
}

func hasVideoInMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	if v, ok := metadata["has_video"].(bool); ok && v {
		return true
	}
	// eca_video_reference / video_reference 可能包含音频（如 {"audio": "..."} 或 .mp3 后缀）
	// 只有确认其中包含有效视频时才判定为含视频
	if vRef, ok := metadata["eca_video_reference"]; ok && referenceContainsVideo(vRef) {
		return true
	}
	if vRef, ok := metadata["video_reference"]; ok && referenceContainsVideo(vRef) {
		return true
	}
	if vRef, ok := metadata["video"]; ok && referenceContainsVideo(vRef) {
		return true
	}
	contentRaw, ok := metadata["content"]
	if !ok {
		return false
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "video_url" {
			if urlMap, ok := itemMap["video_url"].(map[string]any); ok {
				if u, ok := urlMap["url"].(string); ok && isAudioURL(u) {
					continue
				}
			} else if u, ok := itemMap["video_url"].(string); ok && isAudioURL(u) {
				continue
			}
			return true
		}
		if _, has := itemMap["video_url"]; has {
			return true
		}
	}
	return false
}

func isVideoInputReference(inputRef any) bool {
	if inputRef == nil {
		return false
	}
	switch v := inputRef.(type) {
	case string:
		return isVideoURL(v)
	case map[string]any:
		if vStr, ok := v["video"].(string); ok && isVideoURL(vStr) {
			return true
		}
		if uStr, ok := v["url"].(string); ok && isVideoURL(uStr) {
			return true
		}
		return false
	case []string:
		for _, s := range v {
			if isVideoURL(s) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if isVideoInputReference(item) {
				return true
			}
		}
	}
	return false
}

func hasVideoInReq(req *relaycommon.TaskSubmitReq) bool {
	if req == nil {
		return false
	}
	if req.InputReference != "" && isVideoURL(req.InputReference) {
		return true
	}
	for _, img := range req.Images {
		if isVideoURL(img) {
			return true
		}
	}
	if req.Image != "" && isVideoURL(req.Image) {
		return true
	}
	if req.Metadata != nil {
		if inputRef, ok := req.Metadata["input_reference"]; ok && inputRef != nil {
			if isVideoInputReference(inputRef) {
				return true
			}
		}
	}
	return hasVideoInMetadata(req.Metadata)
}

// BuildRequestBody converts request into Wangsu /videos specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	if info.IsModelMapped {
		body.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = body.Model
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{
		Model:   req.Model,
		Prompt:  req.Prompt,
		EcaMode: req.Mode,
	}

	// 1. 分辨率
	if req.Size != "" {
		r.Size = req.Size
	}

	// 2. 秒数 (支持正数及 -1 自动时长)
	if sec, err := strconv.Atoi(req.Seconds); err == nil && (sec > 0 || sec == -1) {
		r.Seconds = lo.ToPtr(dto.IntValue(sec))
	} else if req.Duration > 0 || req.Duration == -1 {
		r.Seconds = lo.ToPtr(dto.IntValue(req.Duration))
	}

	// 3. 处理参考图 / 视频输入（支持 input_reference 中传入视频或图片）
	if req.InputReference != "" {
		if isVideoURL(req.InputReference) {
			r.EcaVideoReference = append(r.EcaVideoReference, map[string]any{"video": req.InputReference})
		} else {
			r.InputReference = []map[string]any{
				{"image": req.InputReference},
			}
		}
	} else if req.HasImage() {
		var refs []map[string]any
		for _, img := range req.Images {
			trimmed := strings.TrimSpace(img)
			if trimmed == "" {
				continue
			}
			if isVideoURL(trimmed) {
				r.EcaVideoReference = append(r.EcaVideoReference, map[string]any{"video": trimmed})
			} else {
				refs = append(refs, map[string]any{"image": trimmed})
			}
		}
		if len(refs) > 0 {
			r.InputReference = refs
		}
	} else if strings.TrimSpace(req.Image) != "" {
		trimmed := strings.TrimSpace(req.Image)
		if isVideoURL(trimmed) {
			r.EcaVideoReference = append(r.EcaVideoReference, map[string]any{"video": trimmed})
		} else {
			r.InputReference = []map[string]any{
				{"image": trimmed},
			}
		}
	}

	// 5. 解析透传 Metadata（先剔除支持多态简写的字段，避免 UnmarshalMetadata 因类型不匹配失败）
	if req.Metadata != nil {
		filteredMeta := make(map[string]interface{}, len(req.Metadata))
		for k, v := range req.Metadata {
			switch k {
			case "eca_video_reference", "eca_audio_reference", "input_reference", "eca_tools":
				continue
			default:
				filteredMeta[k] = v
			}
		}
		if err := taskcommon.UnmarshalMetadata(filteredMeta, &r); err != nil {
			return nil, errors.Wrap(err, "unmarshal metadata failed")
		}
		// 如果 metadata 显式覆盖了 size，优先使用
		if sizeStr, ok := req.Metadata["size"].(string); ok && sizeStr != "" {
			r.Size = sizeStr
		} else if resStr, ok := req.Metadata["resolution"].(string); ok && resStr != "" {
			r.Size = resStr
		}
		// 宽高比：支持通用别名 aspect_ratio -> eca_aspect_ratio
		if ar, ok := req.Metadata["aspect_ratio"].(string); ok && ar != "" && r.EcaAspectRatio == "" {
			r.EcaAspectRatio = ar
		}
		// 音频：支持 audio -> eca_audio (支持布尔及字符串 "true"/"false")
		if b := parseBoolPtr(req.Metadata["audio"]); b != nil && r.EcaAudio == nil {
			r.EcaAudio = b
		}
		// 首尾帧兼容：支持 first_frame / last_frame 别名
		if ff, ok := req.Metadata["first_frame"].(string); ok && ff != "" && r.EcaFirstFrame == "" {
			r.EcaFirstFrame = ff
		}
		if lf, ok := req.Metadata["last_frame"].(string); ok && lf != "" && r.EcaLastFrame == "" {
			r.EcaLastFrame = lf
		}
		// 参考输入（多参考图/视频）：直接透传 input_reference，若是视频则转入 eca_video_reference，若是图片则智能规整为标准 [{"image": "url"}] 数组
		if inputRefRaw, ok := req.Metadata["input_reference"]; ok && inputRefRaw != nil {
			if isVideoInputReference(inputRefRaw) {
				if refs := parseVideoReference(inputRefRaw); len(refs) > 0 {
					r.EcaVideoReference = append(r.EcaVideoReference, refs...)
				}
			} else {
				if refs := parseInputReference(inputRefRaw); len(refs) > 0 {
					r.InputReference = refs
				}
			}
		}
		// 视频参考兼容：支持 eca_video_reference / video_reference / video（兼容字符串、单对象、数组）
		for _, key := range []string{"eca_video_reference", "video_reference", "video"} {
			if vRefRaw, ok := req.Metadata[key]; ok && vRefRaw != nil {
				if refs := parseVideoReference(vRefRaw); len(refs) > 0 {
					r.EcaVideoReference = refs
					break
				}
			}
		}
		// 音频参考兼容：支持 eca_audio_reference / audio_reference / 非布尔的 audio
		for _, key := range []string{"eca_audio_reference", "audio_reference", "audio"} {
			if aRefRaw, ok := req.Metadata[key]; ok && aRefRaw != nil {
				if key == "audio" {
					if _, isBool := aRefRaw.(bool); isBool || parseBoolPtr(aRefRaw) != nil {
						continue
					}
				}
				if refs := parseAudioReference(aRefRaw); len(refs) > 0 {
					r.EcaAudioReference = refs
					break
				}
			}
		}
		// 模式兼容：支持 mode -> eca_mode
		if m, ok := req.Metadata["mode"].(string); ok && m != "" {
			r.EcaMode = m
		}
		// 多镜头兼容：支持 multi_shot -> eca_multi_shot
		if r.EcaMultiShot == nil {
			if b := parseBoolPtr(req.Metadata["multi_shot"]); b != nil {
				r.EcaMultiShot = b
			}
		}
		// 水印与尾帧弱类型兼容
		if r.Watermark == nil {
			if b := parseBoolPtr(req.Metadata["watermark"]); b != nil {
				r.Watermark = b
			}
		}
		if r.ReturnLastFrame == nil {
			if b := parseBoolPtr(req.Metadata["return_last_frame"]); b != nil {
				r.ReturnLastFrame = b
			}
		}
		// 工具兼容：支持 eca_tools / tools
		if len(r.EcaTools) == 0 {
			for _, key := range []string{"eca_tools", "tools"} {
				if tList, ok := req.Metadata[key].([]map[string]any); ok && len(tList) > 0 {
					r.EcaTools = tList
					break
				} else if tRaw, ok := req.Metadata[key].([]any); ok && len(tRaw) > 0 {
					var tools []map[string]any
					for _, item := range tRaw {
						if m, ok := item.(map[string]any); ok {
							tools = append(tools, m)
						}
					}
					if len(tools) > 0 {
						r.EcaTools = tools
						break
					}
				}
			}
		}
	}

	// 统一标准化分辨率档位 (将 1280x720 等智能转为 720p)
	r.Size = normalizeWangsuResolution(r.Size)

	return &r, nil
}

func parseVideoReference(val any) []map[string]any {
	switch v := val.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			if isAudioURL(trimmed) {
				return []map[string]any{{"audio": trimmed}}
			}
			return []map[string]any{{"video": trimmed}}
		}
	case map[string]any:
		vid, hasVid := v["video"].(string)
		aud, hasAud := v["audio"].(string)
		if (hasVid && strings.TrimSpace(vid) != "") || (hasAud && strings.TrimSpace(aud) != "") {
			return []map[string]any{v}
		}
	case []string:
		var result []map[string]any
		for _, s := range v {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				if isAudioURL(trimmed) {
					result = append(result, map[string]any{"audio": trimmed})
				} else {
					result = append(result, map[string]any{"video": trimmed})
				}
			}
		}
		return result
	case []any:
		var result []map[string]any
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				vid, hasVid := m["video"].(string)
				aud, hasAud := m["audio"].(string)
				if (hasVid && strings.TrimSpace(vid) != "") || (hasAud && strings.TrimSpace(aud) != "") {
					result = append(result, m)
				}
			} else if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				trimmed := strings.TrimSpace(str)
				if isAudioURL(trimmed) {
					result = append(result, map[string]any{"audio": trimmed})
				} else {
					result = append(result, map[string]any{"video": trimmed})
				}
			}
		}
		return result
	case []map[string]any:
		var result []map[string]any
		for _, m := range v {
			vid, hasVid := m["video"].(string)
			aud, hasAud := m["audio"].(string)
			if (hasVid && strings.TrimSpace(vid) != "") || (hasAud && strings.TrimSpace(aud) != "") {
				result = append(result, m)
			}
		}
		return result
	}
	return nil
}

func parseAudioReference(val any) []map[string]any {
	switch v := val.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			return []map[string]any{{"audio": strings.TrimSpace(v)}}
		}
	case map[string]any:
		if aud, ok := v["audio"].(string); ok && strings.TrimSpace(aud) != "" {
			return []map[string]any{v}
		}
	case []string:
		var result []map[string]any
		for _, s := range v {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				result = append(result, map[string]any{"audio": trimmed})
			}
		}
		return result
	case []any:
		var result []map[string]any
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if aud, ok := m["audio"].(string); ok && strings.TrimSpace(aud) != "" {
					result = append(result, m)
				}
			} else if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				result = append(result, map[string]any{"audio": strings.TrimSpace(str)})
			}
		}
		return result
	case []map[string]any:
		var result []map[string]any
		for _, m := range v {
			if aud, ok := m["audio"].(string); ok && strings.TrimSpace(aud) != "" {
				result = append(result, m)
			}
		}
		return result
	}
	return nil
}

func parseInputReference(val any) []any {
	switch v := val.(type) {
	case string:
		if s := strings.TrimSpace(v); s != "" {
			return []any{map[string]any{"image": s}}
		}
	case map[string]any:
		if img, ok := v["image"].(string); ok && strings.TrimSpace(img) != "" {
			return []any{map[string]any{"image": strings.TrimSpace(img)}}
		}
	case []any:
		var result []any
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if img, ok := m["image"].(string); ok && strings.TrimSpace(img) != "" {
					result = append(result, map[string]any{"image": strings.TrimSpace(img)})
				}
			} else if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				result = append(result, map[string]any{"image": strings.TrimSpace(str)})
			}
		}
		return result
	case []map[string]any:
		var result []any
		for _, m := range v {
			if img, ok := m["image"].(string); ok && strings.TrimSpace(img) != "" {
				result = append(result, map[string]any{"image": strings.TrimSpace(img)})
			}
		}
		return result
	case []string:
		var result []any
		for _, s := range v {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				result = append(result, map[string]any{"image": trimmed})
			}
		}
		return result
	}
	return nil
}

func parseBoolPtr(val any) *dto.BoolValue {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case bool:
		return lo.ToPtr(dto.BoolValue(v))
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		if s == "true" || s == "1" {
			return lo.ToPtr(dto.BoolValue(true))
		} else if s == "false" || s == "0" {
			return lo.ToPtr(dto.BoolValue(false))
		}
	}
	return nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Wangsu response: {"id": "video.xxx", "status": "queued"}
	var dResp responsePayload
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if dResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.Status = "queued"
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return dResp.ID, responseBody, nil
}

// FetchTask fetch task status: GET ${BASE_URL}/videos/${task_id}
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/videos/%s", strings.TrimRight(baseUrl, "/"), taskID)

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

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// Map Wangsu status to internal status
	switch resTask.Status {
	case "pending", "queued", "created":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running", "in_progress":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "completed", "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		// 解析上游返回的真实 Token 消耗（优先通用 usage，兼容 eca_video_usage）
		if resTask.Usage != nil && (resTask.Usage.TotalTokens > 0 || resTask.Usage.CompletionTokens > 0) {
			taskResult.CompletionTokens = resTask.Usage.CompletionTokens
			taskResult.TotalTokens = resTask.Usage.TotalTokens
		} else {
			taskResult.CompletionTokens = resTask.EcaVideoUsage.CompletionTokens
			taskResult.TotalTokens = resTask.EcaVideoUsage.TotalTokens
		}
		if taskResult.TotalTokens == 0 && taskResult.CompletionTokens > 0 {
			taskResult.TotalTokens = taskResult.CompletionTokens
		}
	case "failed", "expired", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		if resTask.Error.Message != "" {
			taskResult.Reason = resTask.Error.Message
		} else if resTask.Status == "expired" {
			taskResult.Reason = "task execution expired"
		} else if resTask.Status == "cancelled" {
			taskResult.Reason = "task cancelled"
		} else {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var dResp responseTask
	if err := common.Unmarshal(originTask.Data, &dResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal wangsu task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)

	// 设置平台视频流代理地址，使得客户端可直接通过 /v1/videos/:task_id/content 播放和下载
	videoURL := originTask.GetResultURL()
	if videoURL == "" {
		videoURL = taskcommon.BuildProxyURL(originTask.TaskID)
	}
	openAIVideo.SetMetadata("url", videoURL)
	if dResp.Usage != nil && (dResp.Usage.TotalTokens > 0 || dResp.Usage.CompletionTokens > 0) {
		openAIVideo.SetMetadata("usage", dResp.Usage)
	} else if dResp.EcaVideoUsage.TotalTokens > 0 || dResp.EcaVideoUsage.CompletionTokens > 0 {
		openAIVideo.SetMetadata("usage", dResp.EcaVideoUsage)
	}
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName
	if openAIVideo.Model == "" {
		openAIVideo.Model = dResp.Model
	}

	if dResp.Status == "failed" || dResp.Status == "expired" || dResp.Status == "cancelled" {
		msg := dResp.Error.Message
		if msg == "" {
			msg = fmt.Sprintf("task %s", dResp.Status)
		}
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: msg,
			Code:    dResp.Error.Code,
		}
	}

	return common.Marshal(openAIVideo)
}
