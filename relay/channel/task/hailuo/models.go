package hailuo

type SubjectReference struct {
	Type  string   `json:"type"`  // Subject type, currently only supports "character"
	Image []string `json:"image"` // Array of subject reference images (currently only supports single image)
}

type AudioReference struct {
	Type string `json:"type,omitempty"`
	URL  string `json:"url,omitempty"`
}

type VideoReference struct {
	Type string `json:"type,omitempty"`
	URL  string `json:"url,omitempty"`
}

type VideoRequest struct {
	Model            string             `json:"model"`
	Prompt           string             `json:"prompt,omitempty"`
	PromptOptimizer  *bool              `json:"prompt_optimizer,omitempty"`
	FastPretreatment *bool              `json:"fast_pretreatment,omitempty"`
	Duration         *int               `json:"duration,omitempty"`
	Resolution       string             `json:"resolution,omitempty"`
	CallbackURL      string             `json:"callback_url,omitempty"`
	AigcWatermark    *bool              `json:"aigc_watermark,omitempty"`
	FirstFrameImage  string             `json:"first_frame_image,omitempty"` // For image-to-video and start-end-to-video
	LastFrameImage   string             `json:"last_frame_image,omitempty"`  // For start-end-to-video
	SubjectReference []SubjectReference `json:"subject_reference,omitempty"` // For subject-reference-to-video
	AudioReference   []AudioReference   `json:"audio_reference,omitempty"`   // For audio reference
	VideoReference   []VideoReference   `json:"video_reference,omitempty"`   // For video reference
	WithAudio        *bool              `json:"with_audio,omitempty"`        // Whether to generate audio
}

type VideoResponse struct {
	TaskID   string   `json:"task_id"`
	BaseResp BaseResp `json:"base_resp"`
}

type BaseResp struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

type QueryTaskRequest struct {
	TaskID string `json:"task_id"`
}

type QueryTaskResponse struct {
	TaskID      string   `json:"task_id"`
	Status      string   `json:"status"`
	FileID      string   `json:"file_id,omitempty"`
	VideoWidth  int      `json:"video_width,omitempty"`
	VideoHeight int      `json:"video_height,omitempty"`
	BaseResp    BaseResp `json:"base_resp"`
}

type ErrorInfo struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

type TaskStatusInfo struct {
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	FileID    string `json:"file_id,omitempty"`
	VideoURL  string `json:"video_url,omitempty"`
	ErrorCode int    `json:"error_code,omitempty"`
	ErrorMsg  string `json:"error_msg,omitempty"`
}

type ModelConfig struct {
	Name                 string
	DefaultResolution    string
	SupportedDurations   []int
	SupportedResolutions []string
	HasPromptOptimizer   bool
	HasFastPretreatment  bool
}

type RetrieveFileResponse struct {
	File     FileObject `json:"file"`
	BaseResp BaseResp   `json:"base_resp"`
}

type FileObject struct {
	FileID      int64  `json:"file_id"`
	Bytes       int64  `json:"bytes"`
	CreatedAt   int64  `json:"created_at"`
	Filename    string `json:"filename"`
	Purpose     string `json:"purpose"`
	DownloadURL string `json:"download_url"`
}

func GetModelConfig(model string) ModelConfig {
	configs := map[string]ModelConfig{
		"MiniMax-Hailuo-2.3": {
			Name:                 "MiniMax-Hailuo-2.3",
			DefaultResolution:    Resolution768P,
			SupportedDurations:   []int{6, 10},
			SupportedResolutions: []string{Resolution768P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  true,
		},
		"MiniMax-Hailuo-2.3-Fast": {
			Name:                 "MiniMax-Hailuo-2.3-Fast",
			DefaultResolution:    Resolution768P,
			SupportedDurations:   []int{6, 10},
			SupportedResolutions: []string{Resolution768P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  true,
		},
		"MiniMax-Hailuo-02": {
			Name:                 "MiniMax-Hailuo-02",
			DefaultResolution:    Resolution768P,
			SupportedDurations:   []int{6, 10},
			SupportedResolutions: []string{Resolution512P, Resolution768P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  true,
		},
		"T2V-01-Director": {
			Name:                 "T2V-01-Director",
			DefaultResolution:    Resolution768P,
			SupportedDurations:   []int{6},
			SupportedResolutions: []string{Resolution768P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  false,
		},
		"T2V-01": {
			Name:                 "T2V-01",
			DefaultResolution:    Resolution720P,
			SupportedDurations:   []int{6},
			SupportedResolutions: []string{Resolution720P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  false,
		},
		"I2V-01-Director": {
			Name:                 "I2V-01-Director",
			DefaultResolution:    Resolution720P,
			SupportedDurations:   []int{6},
			SupportedResolutions: []string{Resolution720P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  false,
		},
		"I2V-01-live": {
			Name:                 "I2V-01-live",
			DefaultResolution:    Resolution720P,
			SupportedDurations:   []int{6},
			SupportedResolutions: []string{Resolution720P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  false,
		},
		"I2V-01": {
			Name:                 "I2V-01",
			DefaultResolution:    Resolution720P,
			SupportedDurations:   []int{6},
			SupportedResolutions: []string{Resolution720P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  false,
		},
		"S2V-01": {
			Name:                 "S2V-01",
			DefaultResolution:    Resolution720P,
			SupportedDurations:   []int{6},
			SupportedResolutions: []string{Resolution720P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  false,
		},
		ModelH3: {
			Name:                 ModelH3,
			DefaultResolution:    Resolution768P,
			SupportedDurations:   []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			SupportedResolutions: []string{Resolution768P, Resolution2K},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  true,
		},
		ModelH3Max: {
			Name:                 ModelH3Max,
			DefaultResolution:    Resolution480P,
			SupportedDurations:   []int{5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			SupportedResolutions: []string{Resolution480P, Resolution768P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  true,
		},
		ModelH3Regeneration: {
			Name:                 ModelH3Regeneration,
			DefaultResolution:    Resolution2K,
			SupportedDurations:   []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			SupportedResolutions: []string{Resolution2K},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  true,
		},
	}

	if config, exists := configs[model]; exists {
		return config
	}

	return ModelConfig{
		Name:                 model,
		DefaultResolution:    DefaultResolution,
		SupportedDurations:   []int{6},
		SupportedResolutions: []string{DefaultResolution},
		HasPromptOptimizer:   true,
		HasFastPretreatment:  false,
	}
}

// --- MiniMax Video V2 (H3) 数据结构 ---

type ContentItem struct {
	Type     string    `json:"type"`                // text, image_url, video_url, audio_url
	Text     string    `json:"text,omitempty"`
	ImageURL *MediaURL `json:"image_url,omitempty"`
	VideoURL *MediaURL `json:"video_url,omitempty"`
	AudioURL *MediaURL `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"`      // first_frame, last_frame, reference_image, reference_video, reference_audio
}

type MediaURL struct {
	URL string `json:"url"`
}

type VideoGenerationV2Req struct {
	Model         string        `json:"model"`
	Content       []ContentItem `json:"content,omitempty"`
	SourceTaskID  string        `json:"source_task_id,omitempty"`
	Resolution    string        `json:"resolution"`
	Duration      int           `json:"duration,omitempty"`
	Ratio         string        `json:"ratio,omitempty"`
	Extra         any           `json:"extra,omitempty"`
	CallbackURL   string        `json:"callback_url,omitempty"`
	AigcWatermark *bool         `json:"aigc_watermark,omitempty"`
}

type VideoGenerationV2Resp struct {
	TaskID   string    `json:"task_id"`
	BaseResp *BaseResp `json:"base_resp,omitempty"`
	Error    *struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type QueryTaskV2Resp struct {
	Task     *TaskV2Detail `json:"task,omitempty"`
	Error    *TaskV2Error  `json:"error,omitempty"`
	BaseResp *BaseResp     `json:"base_resp,omitempty"`
}

type TaskV2Detail struct {
	ID         string        `json:"id"`
	Model      string        `json:"model"`
	Status     string        `json:"status"` // queued, running, succeeded, failed, cancelled
	CreatedAt  int64         `json:"created_at"`
	UpdatedAt  int64         `json:"updated_at"`
	Content    TaskV2Content `json:"content"`
	Resolution string        `json:"resolution"`
	Duration   int           `json:"duration"`
	Ratio      string        `json:"ratio"`
	Error      *TaskV2Error  `json:"error,omitempty"`
	Usage      *TaskV2Usage  `json:"usage,omitempty"`
}

type TaskV2Content struct {
	URL    string `json:"url,omitempty"`
	Prompt string `json:"prompt,omitempty"`
}

type TaskV2Error struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
}

type TaskV2Usage struct {
	TotalSeconds      int `json:"total_seconds"`
	InputSeconds      int `json:"input_seconds"`
	OutputSeconds     int `json:"output_seconds"`
	InputImageCount   int `json:"input_image_count"`
	InputAudioSeconds int `json:"input_audio_seconds"`
}
