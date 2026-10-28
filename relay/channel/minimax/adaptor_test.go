package minimax

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
)

func TestGetRequestURLForImageGeneration(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://api.minimax.chat",
		},
	}

	got, err := GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}

	want := "https://api.minimax.chat/v1/image_generation"
	if got != want {
		t.Fatalf("GetRequestURL() = %q, want %q", got, want)
	}
}

func TestConvertImageRequest(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "image-01",
	}
	request := dto.ImageRequest{
		Model:          "image-01",
		Prompt:         "a red fox in snowfall",
		Size:           "1536x1024",
		ResponseFormat: "url",
		N:              uintPtr(2),
	}

	got, err := adaptor.ConvertImageRequest(gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()), info, request)
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if payload["model"] != "image-01" {
		t.Fatalf("model = %#v, want %q", payload["model"], "image-01")
	}
	if payload["prompt"] != request.Prompt {
		t.Fatalf("prompt = %#v, want %q", payload["prompt"], request.Prompt)
	}
	if payload["n"] != float64(2) {
		t.Fatalf("n = %#v, want 2", payload["n"])
	}
	if payload["aspect_ratio"] != "3:2" {
		t.Fatalf("aspect_ratio = %#v, want %q", payload["aspect_ratio"], "3:2")
	}
	if payload["response_format"] != "url" {
		t.Fatalf("response_format = %#v, want %q", payload["response_format"], "url")
	}
}

func init() {
	gin.SetMode(gin.TestMode)
}

func TestDoResponseForImageGeneration(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		StartTime: time.Unix(1700000000, 0),
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       httptest.NewRecorder().Result().Body,
	}
	resp.Body = ioNopCloser(`{"data":{"image_urls":["https://example.com/minimax.png"]}}`)

	adaptor := &Adaptor{}
	usage, err := adaptor.DoResponse(c, resp, info)
	if err != nil {
		t.Fatalf("DoResponse returned error: %v", err)
	}
	if usage == nil {
		t.Fatalf("DoResponse returned nil usage")
	}

	body := recorder.Body.String()
	if !strings.Contains(body, `"url":"https://example.com/minimax.png"`) {
		t.Fatalf("response body = %s, want OpenAI image response with image URL", body)
	}
	if strings.Contains(body, `"image_urls"`) {
		t.Fatalf("response body = %s, should not expose raw MiniMax image_urls payload", body)
	}
}

type nopReadCloser struct {
	*strings.Reader
}

func (n nopReadCloser) Close() error {
	return nil
}

func ioNopCloser(body string) nopReadCloser {
	return nopReadCloser{Reader: strings.NewReader(body)}
}

func uintPtr(v uint) *uint {
	return &v
}

func TestGetRequestURLForChatCompletions(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://api.minimax.cn",
		},
	}

	got, err := GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}

	want := "https://api.minimax.cn/v1/chat/completions"
	if got != want {
		t.Fatalf("GetRequestURL() = %q, want %q", got, want)
	}
}

func TestGetModelList_IncludesM3Models(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	models := adaptor.GetModelList()

	expectedModels := []string{
		"MiniMax-M3.1-Flash-Preview",
		"MiniMax-M3",
	}

	for _, expected := range expectedModels {
		if !slices.Contains(models, expected) {
			t.Errorf("GetModelList() missing model: %s", expected)
		}
	}
}

func TestConvertOpenAIRequest_MaxTokensUpgrade(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeChatCompletions,
		OriginModelName: "MiniMax-M3.1-Flash-Preview",
	}

	maxTokensVal := uint(4096)
	request := &dto.GeneralOpenAIRequest{
		Model:     "MiniMax-M3.1-Flash-Preview",
		MaxTokens: &maxTokensVal,
	}

	converted, err := adaptor.ConvertOpenAIRequest(gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()), info, request)
	if err != nil {
		t.Fatalf("ConvertOpenAIRequest failed: %v", err)
	}

	convertedReq, ok := converted.(*dto.GeneralOpenAIRequest)
	if !ok {
		t.Fatalf("expected *dto.GeneralOpenAIRequest, got %T", converted)
	}

	if convertedReq.MaxCompletionTokens == nil || *convertedReq.MaxCompletionTokens != 4096 {
		t.Fatalf("expected MaxCompletionTokens = 4096, got %#v", convertedReq.MaxCompletionTokens)
	}
}

func TestConvertOpenAIRequest_M31ThinkingGuard(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeChatCompletions,
		OriginModelName: "MiniMax-M3.1-Flash-Preview",
	}

	// 场景 1: reasoning_effort 传入 "none" 应被自动清除，防止 MiniMax 400 拒绝
	reqWithNone := &dto.GeneralOpenAIRequest{
		Model:           "MiniMax-M3.1-Flash-Preview",
		ReasoningEffort: "none",
	}
	converted1, err := adaptor.ConvertOpenAIRequest(gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()), info, reqWithNone)
	if err != nil {
		t.Fatalf("ConvertOpenAIRequest failed: %v", err)
	}
	res1 := converted1.(*dto.GeneralOpenAIRequest)
	if res1.ReasoningEffort != "" {
		t.Fatalf("expected ReasoningEffort to be empty, got %q", res1.ReasoningEffort)
	}

	// 场景 2: thinking.type 为 disabled 应被安全移除，防止 MiniMax 400 拒绝
	reqWithDisabled := &dto.GeneralOpenAIRequest{
		Model:    "MiniMax-M3.1-Flash-Preview",
		THINKING: json.RawMessage(`{"type":"disabled"}`),
	}
	converted2, err := adaptor.ConvertOpenAIRequest(gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()), info, reqWithDisabled)
	if err != nil {
		t.Fatalf("ConvertOpenAIRequest failed: %v", err)
	}
	res2 := converted2.(*dto.GeneralOpenAIRequest)
	if len(res2.THINKING) > 0 {
		t.Fatalf("expected THINKING to be cleared for M3.1 when disabled, got %s", string(res2.THINKING))
	}

	// 场景 3: 正常的 reasoning_effort 档位（如 high/max）需无损保留
	reqWithHigh := &dto.GeneralOpenAIRequest{
		Model:           "MiniMax-M3.1-Flash-Preview",
		ReasoningEffort: "high",
	}
	converted3, err := adaptor.ConvertOpenAIRequest(gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()), info, reqWithHigh)
	if err != nil {
		t.Fatalf("ConvertOpenAIRequest failed: %v", err)
	}
	res3 := converted3.(*dto.GeneralOpenAIRequest)
	if res3.ReasoningEffort != "high" {
		t.Fatalf("expected ReasoningEffort to be 'high', got %q", res3.ReasoningEffort)
	}
}
