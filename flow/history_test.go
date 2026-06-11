package flow_test

import (
	"context"
	"testing"

	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/flow"
	"github.com/likun666661/rive-adk-go/llmagent"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/runner"
	"github.com/likun666661/rive-adk-go/tool"
)

func TestFlowFeedsPriorStepEventsIntoNextModelRequest(t *testing.T) {
	rec := &recordingHistoryModel{}
	weather := tool.NewFunctionTool("get_weather", "Get weather", func(args map[string]any) (map[string]any, error) {
		return map[string]any{"city": args["city"], "temperature": 22}, nil
	})
	f := &flow.Flow{
		Model: rec,
		Tools: map[string]tool.FunctionTool{"get_weather": weather},
	}
	ag, err := llmagent.New("weather_bot", "test", f)
	if err != nil {
		t.Fatal(err)
	}
	r, err := runner.New(runner.Config{
		AppName:        "history_test",
		Agent:          ag.(runner.ExecutableAgent),
		SessionService: runner.NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Run(context.Background(), "u", "s", "weather in Tokyo?"); err != nil {
		t.Fatal(err)
	}
	if len(rec.requests) != 2 {
		t.Fatalf("model calls = %d, want 2", len(rec.requests))
	}
	second := rec.requests[1]
	if len(second.Contents) != 3 {
		t.Fatalf("second request contents = %d, want user+model+tool", len(second.Contents))
	}
	if !hasFunctionCallPart(second.Contents[1]) {
		t.Fatalf("second request missing prior model function call: %#v", second.Contents[1])
	}
	if !hasFunctionResponsePart(second.Contents[2]) {
		t.Fatalf("second request missing prior tool response: %#v", second.Contents[2])
	}
}

type recordingHistoryModel struct {
	requests []*model.LLMRequest
}

func (m *recordingHistoryModel) Name() string { return "recording-history" }

func (m *recordingHistoryModel) GenerateContent(req *model.LLMRequest) (*model.LLMResponse, error) {
	m.requests = append(m.requests, cloneRequestForTest(req))
	if len(m.requests) == 1 {
		return model.FunctionCallResponse("checking",
			event.FunctionCall{ID: "call_1", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
		), nil
	}
	return model.TextResponse("Tokyo is 22C."), nil
}

func cloneRequestForTest(req *model.LLMRequest) *model.LLMRequest {
	cp := *req
	cp.Contents = make([]model.LLMContent, len(req.Contents))
	for i, content := range req.Contents {
		cp.Contents[i] = model.LLMContent{
			Role:  content.Role,
			Parts: append([]model.LLMPart{}, content.Parts...),
		}
	}
	return &cp
}

func hasFunctionCallPart(content model.LLMContent) bool {
	for _, part := range content.Parts {
		if part.FunctionCall != nil {
			return true
		}
	}
	return false
}

func hasFunctionResponsePart(content model.LLMContent) bool {
	for _, part := range content.Parts {
		if part.FunctionResponse != nil {
			return true
		}
	}
	return false
}
