package ctyundoubao

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

type ContentItem struct {
	Type     string    `json:"type,omitempty"`
	Text     string    `json:"text,omitempty"`
	ImageURL *MediaURL `json:"image_url,omitempty"`
	VideoURL *MediaURL `json:"video_url,omitempty"`
	AudioURL *MediaURL `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"`
}

type MediaURL struct {
	URL string `json:"url,omitempty"`
}

type requestPayload struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content,omitempty"`
	CallbackURL           string         `json:"callback_url,omitempty"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue `json:"draft,omitempty"`
	Tools                 []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	SafetyIdentifier string         `json:"safety_identifier,omitempty"`
	Priority         *dto.IntValue  `json:"priority,omitempty"`
	Resolution       string         `json:"resolution,omitempty"`
	Ratio            string         `json:"ratio,omitempty"`
	Duration         *dto.IntValue  `json:"duration,omitempty"`
	Frames           *dto.IntValue  `json:"frames,omitempty"`
	Seed             *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed      *dto.BoolValue `json:"camera_fixed,omitempty"`
	Watermark        *dto.BoolValue `json:"watermark,omitempty"`
}

type responsePayload struct {
	ID string `json:"id"` // task_id: e.g. "cgt-xxx"
}

type responseTask struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL     string `json:"video_url"`
		LastFrameURL string `json:"last_frame_url,omitempty"`
	} `json:"content"`
	Seed            int    `json:"seed"`
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	Ratio           string `json:"ratio"`
	FramesPerSecond int    `json:"framespersecond"`
	ServiceTier     string `json:"service_tier"`
	Tools           []struct {
		Type string `json:"type"`
	} `json:"tools"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		ToolUsage        struct {
			WebSearch int `json:"web_search"`
		} `json:"tool_usage"`
	} `json:"usage"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
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

func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	return baseURL
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = normalizeBaseURL(info.ChannelBaseUrl)
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
// 兼容两种请求格式：
// 1. OpenAI 视频标准格式（含 prompt / size / seconds / image / metadata）
// 2. 豆包 / 天翼云原生格式（顶层直接传 content 数组、resolution、ratio、duration、generate_audio 等）
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	contentType := c.GetHeader("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
	}

	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}

	var rawBody map[string]any
	if err := common.UnmarshalBodyReusable(c, &rawBody); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}

	// 将顶层豆包/天翼原生字段合并注入到 req.Metadata（不覆盖 metadata 中已显式传的值）
	nativeKeys := []string{
		"content",
		"resolution",
		"ratio",
		"generate_audio",
		"return_last_frame",
		"watermark",
		"draft",
		"seed",
		"tools",
		"camera_fixed",
		"execution_expires_after",
		"service_tier",
		"safety_identifier",
		"priority",
		"frames",
		"callback_url",
	}
	for _, key := range nativeKeys {
		if val, exists := rawBody[key]; exists && val != nil {
			if req.Metadata == nil {
				req.Metadata = make(map[string]interface{})
			}
			if _, alreadySet := req.Metadata[key]; !alreadySet {
				req.Metadata[key] = val
			}
		}
	}

	// 若使用原生 content 数组且顶层未传 prompt，从 content 中提取文本或确认存在有效媒体内容
	hasNativeContent := false
	if req.Metadata != nil {
		if contentSlice, ok := req.Metadata["content"].([]interface{}); ok && len(contentSlice) > 0 {
			hasNativeContent = true
			if strings.TrimSpace(req.Prompt) == "" {
				for _, item := range contentSlice {
					if m, ok := item.(map[string]interface{}); ok && m["type"] == "text" {
						if txt, ok := m["text"].(string); ok && strings.TrimSpace(txt) != "" {
							req.Prompt = strings.TrimSpace(txt)
							break
						}
					}
				}
			}
		}
	}

	if strings.TrimSpace(req.Prompt) == "" && !hasNativeContent {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}

	// 校验时长范围：支持 -1（模型自动决定时长）或 0~MaxTaskDurationSeconds
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

// BuildRequestURL constructs upstream URL: POST ${BASE_URL}/v1/contents/generations/tasks
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/contents/generations/tasks", a.baseURL), nil
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

func normalizeResolution(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "720p"
	}
	switch s {
	case "480p", "720p", "1080p", "4k":
		return s
	}
	if strings.Contains(s, "4k") || strings.Contains(s, "2160") {
		return "4k"
	}
	if strings.Contains(s, "1080") {
		return "1080p"
	}
	if strings.Contains(s, "720") {
		return "720p"
	}
	if strings.Contains(s, "480") {
		return "480p"
	}
	return s
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
		} else if ratioVal > 2.0 && ratioVal < 2.5 {
			return "21:9"
		}
	}
	if strings.Contains(s, ":") {
		return strings.TrimSpace(size)
	}
	return "16:9"
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
	return normalizeResolution(res)
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
	if vRef, ok := metadata["video_reference"]; ok && referenceContainsVideo(vRef) {
		return true
	}
	if vRef, ok := metadata["eca_video_reference"]; ok && referenceContainsVideo(vRef) {
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

// BuildRequestBody converts request into Tianyi Cloud Seedance format.
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

// extractRefImagesFromAny extracts image URLs from string, []string, []any, or []map[string]any.
func extractRefImagesFromAny(val any) []string {
	var urls []string
	switch v := val.(type) {
	case string:
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			urls = append(urls, trimmed)
		}
	case []string:
		for _, item := range v {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				urls = append(urls, trimmed)
			}
		}
	case []map[string]any:
		for _, m := range v {
			urls = append(urls, extractImageFromMap(m)...)
		}
	case []any:
		for _, item := range v {
			switch elem := item.(type) {
			case string:
				if trimmed := strings.TrimSpace(elem); trimmed != "" {
					urls = append(urls, trimmed)
				}
			case map[string]any:
				urls = append(urls, extractImageFromMap(elem)...)
			}
		}
	case map[string]any:
		urls = append(urls, extractImageFromMap(v)...)
	}
	return urls
}

func extractImageFromMap(m map[string]any) []string {
	for _, key := range []string{"image", "url", "image_url"} {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return []string{strings.TrimSpace(s)}
		}
		if sub, ok := m[key].(map[string]any); ok {
			if u, ok := sub["url"].(string); ok && strings.TrimSpace(u) != "" {
				return []string{strings.TrimSpace(u)}
			}
		}
	}
	return nil
}

// extractMediaItemsFromRef converts video_reference / audio_reference values into ContentItems.
func extractMediaItemsFromRef(val any) []ContentItem {
	var items []ContentItem
	appendURL := func(rawURL string, forceType string) {
		u := strings.TrimSpace(rawURL)
		if u == "" {
			return
		}
		if forceType == "audio" || (forceType == "" && isAudioURL(u)) {
			items = append(items, ContentItem{
				Type:     "audio_url",
				AudioURL: &MediaURL{URL: u},
				Role:     "reference_audio",
			})
		} else {
			items = append(items, ContentItem{
				Type:     "video_url",
				VideoURL: &MediaURL{URL: u},
				Role:     "reference_video",
			})
		}
	}
	handleMap := func(m map[string]any) {
		if v, ok := m["video"].(string); ok && strings.TrimSpace(v) != "" {
			appendURL(v, "video")
		}
		if a, ok := m["audio"].(string); ok && strings.TrimSpace(a) != "" {
			appendURL(a, "audio")
		}
		if u, ok := m["url"].(string); ok && strings.TrimSpace(u) != "" {
			appendURL(u, "")
		}
	}

	switch v := val.(type) {
	case string:
		appendURL(v, "")
	case map[string]any:
		handleMap(v)
	case []string:
		for _, s := range v {
			appendURL(s, "")
		}
	case []map[string]any:
		for _, m := range v {
			handleMap(m)
		}
	case []any:
		for _, item := range v {
			switch elem := item.(type) {
			case string:
				appendURL(elem, "")
			case map[string]any:
				handleMap(elem)
			}
		}
	}
	return items
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{
		Model:   req.Model,
		Content: []ContentItem{},
	}

	// 1. 分辨率与宽高比标准化
	if req.Size != "" {
		if strings.Contains(req.Size, ":") {
			r.Ratio = strings.TrimSpace(req.Size)
		} else if strings.Contains(req.Size, "*") || strings.Contains(req.Size, "x") || strings.Contains(req.Size, "X") {
			r.Resolution = normalizeResolution(req.Size)
			r.Ratio = sizeToRatio(req.Size)
		} else {
			r.Resolution = normalizeResolution(req.Size)
		}
	}

	// 2. 时长（秒数，支持正数以及 -1 智能时长）
	if sec, err := strconv.Atoi(req.Seconds); err == nil && (sec > 0 || sec == -1) {
		r.Duration = lo.ToPtr(dto.IntValue(sec))
	} else if req.Duration > 0 || req.Duration == -1 {
		r.Duration = lo.ToPtr(dto.IntValue(req.Duration))
	}

	// 3. 处理图片/视频/音频输入
	var firstFrameURL string
	var lastFrameURL string
	var refImageURLs []string
	var refVideoURLs []string
	var refAudioURLs []string

	appendRefMedia := func(rawURL string) {
		u := strings.TrimSpace(rawURL)
		if u == "" {
			return
		}
		if isAudioURL(u) {
			refAudioURLs = append(refAudioURLs, u)
		} else if isVideoURL(u) {
			refVideoURLs = append(refVideoURLs, u)
		} else {
			refImageURLs = append(refImageURLs, u)
		}
	}

	if req.Metadata != nil {
		if ff, ok := req.Metadata["first_frame"].(string); ok && strings.TrimSpace(ff) != "" {
			firstFrameURL = strings.TrimSpace(ff)
		} else if ff, ok := req.Metadata["eca_first_frame"].(string); ok && strings.TrimSpace(ff) != "" {
			firstFrameURL = strings.TrimSpace(ff)
		}
		if lf, ok := req.Metadata["last_frame"].(string); ok && strings.TrimSpace(lf) != "" {
			lastFrameURL = strings.TrimSpace(lf)
		} else if lf, ok := req.Metadata["eca_last_frame"].(string); ok && strings.TrimSpace(lf) != "" {
			lastFrameURL = strings.TrimSpace(lf)
		}
		if inputRef, ok := req.Metadata["input_reference"]; ok {
			for _, refURL := range extractRefImagesFromAny(inputRef) {
				appendRefMedia(refURL)
			}
		}
	}

	// 参考输入/常规图片/视频收集
	if req.InputReference != "" {
		appendRefMedia(req.InputReference)
	}
	if req.HasImage() {
		for i, img := range req.Images {
			trimmed := strings.TrimSpace(img)
			if trimmed == "" {
				continue
			}
			if i == 0 && firstFrameURL == "" && len(refImageURLs) == 0 && !isVideoURL(trimmed) && !isAudioURL(trimmed) {
				firstFrameURL = trimmed
			} else {
				appendRefMedia(trimmed)
			}
		}
	} else if strings.TrimSpace(req.Image) != "" && firstFrameURL == "" && len(refImageURLs) == 0 {
		trimmed := strings.TrimSpace(req.Image)
		if isVideoURL(trimmed) || isAudioURL(trimmed) {
			appendRefMedia(trimmed)
		} else {
			firstFrameURL = trimmed
		}
	}

	// 4. 解析 Metadata 覆盖与高级配置
	if req.Metadata != nil {
		if err := taskcommon.UnmarshalMetadata(req.Metadata, &r); err != nil {
			return nil, errors.Wrap(err, "unmarshal metadata failed")
		}

		if sizeStr, ok := req.Metadata["size"].(string); ok && sizeStr != "" {
			if strings.Contains(sizeStr, ":") {
				r.Ratio = strings.TrimSpace(sizeStr)
			} else {
				r.Resolution = normalizeResolution(sizeStr)
			}
		}
		if resStr, ok := req.Metadata["resolution"].(string); ok && resStr != "" {
			r.Resolution = normalizeResolution(resStr)
		}
		if ar, ok := req.Metadata["ratio"].(string); ok && ar != "" {
			r.Ratio = ar
		} else if ar, ok := req.Metadata["aspect_ratio"].(string); ok && ar != "" {
			r.Ratio = ar
		} else if ar, ok := req.Metadata["eca_aspect_ratio"].(string); ok && ar != "" {
			r.Ratio = ar
		}

		if b := parseBoolPtr(req.Metadata["generate_audio"]); b != nil {
			r.GenerateAudio = b
		} else if b := parseBoolPtr(req.Metadata["audio"]); b != nil {
			r.GenerateAudio = b
		} else if b := parseBoolPtr(req.Metadata["eca_audio"]); b != nil {
			r.GenerateAudio = b
		}

		if b := parseBoolPtr(req.Metadata["return_last_frame"]); b != nil {
			r.ReturnLastFrame = b
		}
		if b := parseBoolPtr(req.Metadata["watermark"]); b != nil {
			r.Watermark = b
		}
		if b := parseBoolPtr(req.Metadata["draft"]); b != nil {
			r.Draft = b
		}
	}

	// 5. 如果 Metadata 显式传入了 content 数组且已有内容，直接使用；否则按 OpenAI 参数组装多模态列表
	if len(r.Content) == 0 {
		// 组装首帧
		if firstFrameURL != "" {
			r.Content = append(r.Content, ContentItem{
				Type: "image_url",
				ImageURL: &MediaURL{
					URL: firstFrameURL,
				},
				Role: "first_frame",
			})
		}
		// 组装参考图
		for _, refURL := range refImageURLs {
			r.Content = append(r.Content, ContentItem{
				Type: "image_url",
				ImageURL: &MediaURL{
					URL: refURL,
				},
				Role: "reference_image",
			})
		}
		// 组装尾帧
		if lastFrameURL != "" {
			r.Content = append(r.Content, ContentItem{
				Type: "image_url",
				ImageURL: &MediaURL{
					URL: lastFrameURL,
				},
				Role: "last_frame",
			})
		}
		// 组装参考视频（来自 input_reference 或 images）
		for _, refURL := range refVideoURLs {
			r.Content = append(r.Content, ContentItem{
				Type: "video_url",
				VideoURL: &MediaURL{
					URL: refURL,
				},
				Role: "reference_video",
			})
		}
		// 组装参考音频（来自 input_reference 或 images）
		for _, refURL := range refAudioURLs {
			r.Content = append(r.Content, ContentItem{
				Type: "audio_url",
				AudioURL: &MediaURL{
					URL: refURL,
				},
				Role: "reference_audio",
			})
		}
		// 组装参考视频 / 参考音频 (来自 metadata 扩展配置)
		if req.Metadata != nil {
			for _, key := range []string{"video_reference", "eca_video_reference", "video", "audio_reference", "audio"} {
				if val, exists := req.Metadata[key]; exists && val != nil {
					if key == "audio" {
						// metadata["audio"] 可能是 bool 开关，仅当是字符串/数组/map 时解析为媒体引用
						if _, isBool := val.(bool); isBool {
							continue
						}
					}
					r.Content = append(r.Content, extractMediaItemsFromRef(val)...)
				}
			}
		}
	}

	// 6. 追加/同步 Prompt 文本（仅在 content 数组中尚无文本项时追加，避免抹除原生 content 中的多段 text）
	if req.Prompt != "" {
		hasTextItem := false
		for _, item := range r.Content {
			if item.Type == "text" {
				hasTextItem = true
				break
			}
		}
		if !hasTextItem {
			r.Content = append(r.Content, ContentItem{
				Type: "text",
				Text: req.Prompt,
			})
		}
	}

	return &r, nil
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

	// Parse Tianyi response: {"id": "cgt-xxx"}
	var dResp responsePayload
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if dResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty: %s", responseBody), "invalid_response", http.StatusInternalServerError)
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

// FetchTask fetch task status: GET ${BASE_URL}/v1/contents/generations/tasks/${task_id}
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	normalized := normalizeBaseURL(baseUrl)
	uri := fmt.Sprintf("%s/v1/contents/generations/tasks/%s", normalized, taskID)

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

	// Map Doubao / Tianyi status to internal status
	switch resTask.Status {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resTask.Content.VideoURL
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
		if taskResult.TotalTokens == 0 && taskResult.CompletionTokens > 0 {
			taskResult.TotalTokens = taskResult.CompletionTokens
		}
	case "failed", "cancelled", "expired":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
		if taskResult.Reason == "" {
			taskResult.Reason = fmt.Sprintf("task %s", resTask.Status)
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
		return nil, errors.Wrap(err, "unmarshal task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	videoURL := dResp.Content.VideoURL
	if videoURL == "" {
		videoURL = originTask.GetResultURL()
	}
	if videoURL != "" {
		openAIVideo.SetMetadata("url", videoURL)
	}
	if dResp.Content.LastFrameURL != "" {
		openAIVideo.SetMetadata("last_frame_url", dResp.Content.LastFrameURL)
	}
	if dResp.Usage.TotalTokens > 0 || dResp.Usage.CompletionTokens > 0 {
		openAIVideo.SetMetadata("usage", dResp.Usage)
	}
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName
	if openAIVideo.Model == "" {
		openAIVideo.Model = dResp.Model
	}

	if dResp.Status == "failed" || dResp.Status == "cancelled" || dResp.Status == "expired" {
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

