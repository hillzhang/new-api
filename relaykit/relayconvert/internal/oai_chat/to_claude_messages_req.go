package oaichat

import (
	"encoding/json"
	"fmt"
	"strings"

	"context"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	relaymedia "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	sharedclaude "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/claude"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
)

const (
	webSearchMaxUsesLow    = 1
	webSearchMaxUsesMedium = 5
	webSearchMaxUsesHigh   = 10
)

type openRouterRequestReasoning struct {
	Enabled   bool   `json:"enabled"`
	Effort    string `json:"effort,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
	Exclude   bool   `json:"exclude,omitempty"`
}

func OpenAIChatRequestToClaudeMessages(c context.Context, info convmeta.Meta, textRequest dto.GeneralOpenAIRequest) (*dto.ClaudeRequest, error) {
	opts := convmeta.OptionsOf(info)
	claudeTools := make([]any, 0, len(textRequest.Tools))

	for _, tool := range textRequest.Tools {
		if _, ok := tool.Function.Parameters.(map[string]any); !ok && tool.Type != "function" {
			continue
		}
		claudeTools = append(claudeTools, &dto.Tool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: sharedclaude.FunctionParametersToInputSchema(tool.Function.Parameters),
		})
	}

	if textRequest.WebSearchOptions != nil {
		webSearchTool := dto.ClaudeWebSearchTool{
			Type: "web_search_20250305",
			Name: "web_search",
		}

		if textRequest.WebSearchOptions.UserLocation != nil {
			anthropicUserLocation := &dto.ClaudeWebSearchUserLocation{
				Type: "approximate",
			}

			var userLocationMap map[string]interface{}
			if err := kitutil.Unmarshal(textRequest.WebSearchOptions.UserLocation, &userLocationMap); err == nil {
				if approximateData, ok := userLocationMap["approximate"].(map[string]interface{}); ok {
					if timezone, ok := approximateData["timezone"].(string); ok && timezone != "" {
						anthropicUserLocation.Timezone = timezone
					}
					if country, ok := approximateData["country"].(string); ok && country != "" {
						anthropicUserLocation.Country = country
					}
					if region, ok := approximateData["region"].(string); ok && region != "" {
						anthropicUserLocation.Region = region
					}
					if city, ok := approximateData["city"].(string); ok && city != "" {
						anthropicUserLocation.City = city
					}
				}
			}

			webSearchTool.UserLocation = anthropicUserLocation
		}

		switch textRequest.WebSearchOptions.SearchContextSize {
		case "low":
			webSearchTool.MaxUses = webSearchMaxUsesLow
		case "medium":
			webSearchTool.MaxUses = webSearchMaxUsesMedium
		case "high":
			webSearchTool.MaxUses = webSearchMaxUsesHigh
		}

		claudeTools = append(claudeTools, &webSearchTool)
	}

	claudeRequest := dto.ClaudeRequest{
		Model:         textRequest.Model,
		StopSequences: nil,
		Temperature:   textRequest.Temperature,
	}
	if len(claudeTools) > 0 {
		claudeRequest.Tools = claudeTools
	}
	if maxTokens := textRequest.GetMaxTokens(); maxTokens > 0 {
		claudeRequest.MaxTokens = kitutil.GetPointer(maxTokens)
	}
	if textRequest.TopP != nil {
		claudeRequest.TopP = kitutil.GetPointer(*textRequest.TopP)
	}
	if textRequest.TopK != nil {
		claudeRequest.TopK = kitutil.GetPointer(*textRequest.TopK)
	}
	if textRequest.IsStream(nil) {
		claudeRequest.Stream = kitutil.GetPointer(true)
	}

	if textRequest.ToolChoice != nil || textRequest.ParallelTooCalls != nil {
		claudeToolChoice := sharedclaude.MapOpenAIToolChoice(textRequest.ToolChoice, textRequest.ParallelTooCalls)
		if claudeToolChoice != nil {
			claudeRequest.ToolChoice = claudeToolChoice
		}
	}

	if claudeRequest.MaxTokens == nil || *claudeRequest.MaxTokens == 0 {
		if defaultMaxTokens, configured := opts.Claude.DefaultMaxTokensFor(textRequest.Model); configured {
			value := uint(defaultMaxTokens)
			claudeRequest.MaxTokens = &value
		}
	}

	if baseModel, effortLevel, ok := reasoning.TrimEffortSuffix(textRequest.Model); ok && effortLevel != "" &&
		(strings.HasPrefix(textRequest.Model, "claude-opus-4-6") ||
			strings.HasPrefix(textRequest.Model, "claude-opus-4-7") ||
			strings.HasPrefix(textRequest.Model, "claude-opus-4-8") ||
			strings.HasPrefix(textRequest.Model, "claude-fable-")) {
		claudeRequest.Model = baseModel
		claudeRequest.Thinking = &dto.Thinking{
			Type: "adaptive",
		}
		claudeRequest.OutputConfig = json.RawMessage(fmt.Sprintf(`{"effort":"%s"}`, effortLevel))
		if strings.HasPrefix(baseModel, "claude-opus-4-7") ||
			strings.HasPrefix(baseModel, "claude-opus-4-8") ||
			strings.HasPrefix(baseModel, "claude-fable-") {
			claudeRequest.Thinking.Display = "summarized"
			claudeRequest.Temperature = nil
			claudeRequest.TopP = nil
			claudeRequest.TopK = nil
		} else {
			claudeRequest.TopP = nil
			claudeRequest.Temperature = kitutil.GetPointer[float64](1.0)
		}
	} else if opts.Claude.ThinkingAdapterEnabled &&
		strings.HasSuffix(textRequest.Model, "-thinking") {

		trimmedModel := strings.TrimSuffix(textRequest.Model, "-thinking")
		if strings.HasPrefix(trimmedModel, "claude-opus-4-7") ||
			strings.HasPrefix(trimmedModel, "claude-opus-4-8") ||
			strings.HasPrefix(trimmedModel, "claude-fable-") {
			claudeRequest.Thinking = &dto.Thinking{Type: "adaptive", Display: "summarized"}
			claudeRequest.OutputConfig = json.RawMessage(`{"effort":"high"}`)
			claudeRequest.Temperature = nil
			claudeRequest.TopP = nil
			claudeRequest.TopK = nil
		} else {
			if claudeRequest.MaxTokens == nil || *claudeRequest.MaxTokens < 1280 {
				claudeRequest.MaxTokens = kitutil.GetPointer[uint](1280)
			}

			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: kitutil.GetPointer[int](int(float64(*claudeRequest.MaxTokens) * opts.Claude.ThinkingAdapterBudgetTokensPercentage)),
			}
			claudeRequest.TopP = nil
			claudeRequest.Temperature = kitutil.GetPointer[float64](1.0)
		}
		if !opts.ShouldPreserveThinkingSuffix(textRequest.Model) {
			claudeRequest.Model = trimmedModel
		}
	}

	// 针对新一代自适应思考模型（如网宿智算平台 Claude Fable 5.1 系列）进行协议规范化处理：
	// 1. 自适应思考模式：Thinking 模式固定为 adaptive（思考始终开启，由模型自主决策），严禁使用旧版 budget_tokens
	// 2. 采样参数彻底置空：Anthropic 官方与智算网关严格禁止传递外部采样参数 (temperature / top_p / top_k)，置为 nil 避免引发网关 503 重试超时
	// 3. 思考强度控制：若客户端传递了 OpenAI 标准的 reasoning_effort（如 low/medium/high/xhigh/max），标准映射为 output_config.effort
	// 4. 默认 max_tokens 兜底：若上游调用方未显式传递 max_tokens，且未命中全局选项时，设置 4096 作为安全默认值，避免转换抛错
	isClaudeFable := strings.HasPrefix(strings.ToLower(strings.TrimSpace(claudeRequest.Model)), "claude-fable-")
	if isClaudeFable {
		if claudeRequest.Thinking == nil {
			claudeRequest.Thinking = &dto.Thinking{
				Type: "adaptive",
			}
		} else {
			claudeRequest.Thinking.Type = "adaptive"
			claudeRequest.Thinking.BudgetTokens = nil
		}
		// 严禁传递采样参数，赋值为 nil，JSON 序列化时由于 omitempty 会被自动省略
		claudeRequest.Temperature = nil
		claudeRequest.TopP = nil
		claudeRequest.TopK = nil

		// 若客户端传递了 OpenAI 的 reasoning_effort，转译为网宿与 Claude 标准的 output_config.effort
		if textRequest.ReasoningEffort != "" {
			claudeRequest.OutputConfig = json.RawMessage(fmt.Sprintf(`{"effort":"%s"}`, textRequest.ReasoningEffort))
		}

		// 安全兜底：当客户端未指定 max_tokens 时赋予默认值 4096，防止触发 ErrMissingMaxTokens 拦截
		if claudeRequest.MaxTokens == nil || *claudeRequest.MaxTokens == 0 {
			claudeRequest.MaxTokens = kitutil.GetPointer[uint](4096)
		}
	}

	// 仅对非自适应思考模型应用旧版的基于 budget_tokens 的 reasoning_effort 转换
	if textRequest.ReasoningEffort != "" && !isClaudeFable {
		switch textRequest.ReasoningEffort {
		case "low":
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: kitutil.GetPointer[int](1280),
			}
		case "medium":
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: kitutil.GetPointer[int](2048),
			}
		case "high":
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: kitutil.GetPointer[int](4096),
			}
		}
	}

	// 仅对非自适应思考模型解析 OpenRouter 格式的 reasoning 配置，防止意外将自适应思考模型覆写为 enabled+budget_tokens 模式
	if textRequest.Reasoning != nil && !isClaudeFable {
		var reasoningConfig openRouterRequestReasoning
		if err := kitutil.Unmarshal(textRequest.Reasoning, &reasoningConfig); err != nil {
			return nil, err
		}

		budgetTokens := reasoningConfig.MaxTokens
		if budgetTokens > 0 {
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: &budgetTokens,
			}
		}
	}

	if textRequest.Stop != nil {
		switch stop := textRequest.Stop.(type) {
		case string:
			claudeRequest.StopSequences = []string{stop}
		case []interface{}:
			stopSequences := make([]string, 0)
			for _, item := range stop {
				stopSequences = append(stopSequences, item.(string))
			}
			claudeRequest.StopSequences = stopSequences
		}
	}

	formatMessages := make([]dto.Message, 0)
	lastMessage := dto.Message{
		Role: "tool",
	}
	for i, message := range textRequest.Messages {
		if message.Role == "" {
			textRequest.Messages[i].Role = "user"
		}
		fmtMessage := dto.Message{
			Role:    message.Role,
			Content: message.Content,
		}
		if message.Role == "tool" {
			fmtMessage.ToolCallId = message.ToolCallId
		}
		if message.Role == "assistant" && message.ToolCalls != nil {
			fmtMessage.ToolCalls = message.ToolCalls
		}
		if lastMessage.Role == message.Role && lastMessage.Role != "tool" {
			if lastMessage.IsStringContent() && message.IsStringContent() {
				fmtMessage.SetStringContent(strings.Trim(fmt.Sprintf("%s %s", lastMessage.StringContent(), message.StringContent()), "\""))
				formatMessages = formatMessages[:len(formatMessages)-1]
			}
		}
		if fmtMessage.Content == nil || (fmtMessage.IsStringContent() && fmtMessage.StringContent() == "") {
			fmtMessage.SetStringContent("...")
		}
		formatMessages = append(formatMessages, fmtMessage)
		lastMessage = fmtMessage
	}

	claudeMessages := make([]dto.ClaudeMessage, 0)
	isFirstMessage := true
	var systemMessages []dto.ClaudeMediaMessage

	for _, message := range formatMessages {
		if message.Role == "system" {
			if message.IsStringContent() {
				if text := message.StringContent(); text != "" {
					systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
						Type: "text",
						Text: kitutil.GetPointer[string](text),
					})
				}
			} else {
				for _, ctx := range message.ParseContent() {
					if ctx.Type == "text" && ctx.Text != "" {
						systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
							Type: "text",
							Text: kitutil.GetPointer[string](ctx.Text),
						})
					}
				}
			}
			continue
		}

		if isFirstMessage {
			isFirstMessage = false
			if message.Role != "user" {
				claudeMessage := dto.ClaudeMessage{
					Role: "user",
					Content: []dto.ClaudeMediaMessage{
						{
							Type: "text",
							Text: kitutil.GetPointer[string]("..."),
						},
					},
				}
				claudeMessages = append(claudeMessages, claudeMessage)
			}
		}

		claudeMessage := dto.ClaudeMessage{
			Role: message.Role,
		}
		if message.Role == "tool" {
			if len(claudeMessages) > 0 && claudeMessages[len(claudeMessages)-1].Role == "user" {
				lastClaudeMessage := claudeMessages[len(claudeMessages)-1]
				if content, ok := lastClaudeMessage.Content.(string); ok {
					lastClaudeMessage.Content = []dto.ClaudeMediaMessage{
						{
							Type: "text",
							Text: kitutil.GetPointer[string](content),
						},
					}
				}
				lastClaudeMessage.Content = append(lastClaudeMessage.Content.([]dto.ClaudeMediaMessage), dto.ClaudeMediaMessage{
					Type:      "tool_result",
					ToolUseId: message.ToolCallId,
					Content:   message.Content,
				})
				claudeMessages[len(claudeMessages)-1] = lastClaudeMessage
				continue
			}

			claudeMessage.Role = "user"
			claudeMessage.Content = []dto.ClaudeMediaMessage{
				{
					Type:      "tool_result",
					ToolUseId: message.ToolCallId,
					Content:   message.Content,
				},
			}
		} else if message.IsStringContent() && message.ToolCalls == nil {
			text := message.StringContent()
			if text == "" {
				text = "..."
			}
			claudeMessage.Content = text
		} else {
			claudeMediaMessages := make([]dto.ClaudeMediaMessage, 0)
			for _, mediaMessage := range message.ParseContent() {
				switch mediaMessage.Type {
				case "text":
					if mediaMessage.Text != "" {
						claudeMediaMessages = append(claudeMediaMessages, dto.ClaudeMediaMessage{
							Type: "text",
							Text: kitutil.GetPointer[string](mediaMessage.Text),
						})
					}
				default:
					source := mediaMessage.ToFileSource()
					if source == nil {
						continue
					}
					base64Data, mimeType, err := relaymedia.ResolveBase64Data(c, source, "formatting image for Claude")
					if err != nil {
						return nil, fmt.Errorf("get file data failed: %s", err.Error())
					}
					claudeMediaMessage := dto.ClaudeMediaMessage{
						Source: &dto.ClaudeMessageSource{
							Type: "base64",
						},
					}
					if strings.HasPrefix(mimeType, "application/pdf") {
						claudeMediaMessage.Type = "document"
					} else {
						claudeMediaMessage.Type = "image"
					}

					claudeMediaMessage.Source.MediaType = mimeType
					claudeMediaMessage.Source.Data = base64Data
					claudeMediaMessages = append(claudeMediaMessages, claudeMediaMessage)
					continue
				}
			}

			if message.ToolCalls != nil {
				for _, toolCall := range message.ParseToolCalls() {
					inputObj := make(map[string]any)
					if args := toolCall.Function.Arguments; args != "" {
						if err := kitutil.Unmarshal([]byte(args), &inputObj); err != nil {
							kitutil.LogInfo("tool call function arguments is not a map[string]any: " + fmt.Sprintf("%v", toolCall.Function.Arguments))
						}
					}
					claudeMediaMessages = append(claudeMediaMessages, dto.ClaudeMediaMessage{
						Type:  "tool_use",
						Id:    toolCall.ID,
						Name:  toolCall.Function.Name,
						Input: inputObj,
					})
				}
			}
			claudeMessage.Content = claudeMediaMessages
		}
		claudeMessages = append(claudeMessages, claudeMessage)
	}

	if len(systemMessages) > 0 {
		claudeRequest.System = systemMessages
	}

	claudeRequest.Prompt = ""
	claudeRequest.Messages = claudeMessages

	// 映射 OpenAI 请求中的 user 外部用户标识到 Claude metadata.user_id 字段（符合网宿智算网关及 Anthropic 官方规范）
	if len(textRequest.User) > 0 && string(textRequest.User) != "null" && len(claudeRequest.Metadata) == 0 {
		var userStr string
		if err := json.Unmarshal(textRequest.User, &userStr); err == nil && userStr != "" {
			claudeRequest.Metadata = json.RawMessage(fmt.Sprintf(`{"user_id":%q}`, userStr))
		} else {
			userRaw := strings.Trim(string(textRequest.User), "\"")
			if userRaw != "" && userRaw != "null" {
				claudeRequest.Metadata = json.RawMessage(fmt.Sprintf(`{"user_id":%q}`, userRaw))
			}
		}
	}

	// Checked last so every injection path (default hook, thinking adapter
	// floor) has had its chance to satisfy the required field.
	if claudeRequest.MaxTokens == nil {
		return nil, sharedclaude.ErrMissingMaxTokens
	}
	return &claudeRequest, nil
}
