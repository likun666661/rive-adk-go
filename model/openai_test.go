package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/likun666661/rive-adk-go/event"
)

func TestOpenAICompatibleModelToolCallResponse(t *testing.T) {
	var captured openAIChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"tool_calls": [{
						"id": "call_1",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{\"city\":\"Tokyo\"}"
						}
					}]
				},
				"finish_reason": "tool_calls"
			}]
		}`))
	}))
	defer server.Close()

	m, err := NewOpenAICompatibleModel(OpenAICompatibleConfig{
		Name:    "test-model",
		BaseURL: server.URL,
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleModel: %v", err)
	}
	resp, err := m.GenerateContent(&LLMRequest{
		SystemInstruction: "Use tools.",
		Contents: []LLMContent{
			{Role: "user", Parts: []LLMPart{{Text: "weather?"}}},
		},
		ToolDeclarations: []any{
			map[string]any{
				"name":        "get_weather",
				"description": "Get weather.",
				"inputSchema": map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	if captured.Model != "test-model" {
		t.Fatalf("model = %q", captured.Model)
	}
	if len(captured.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(captured.Messages))
	}
	if captured.Messages[0].Role != "system" || captured.Messages[1].Role != "user" {
		t.Fatalf("message roles = %#v", captured.Messages)
	}
	if len(captured.Tools) != 1 || captured.Tools[0].Function.Name != "get_weather" {
		t.Fatalf("tools = %#v", captured.Tools)
	}
	if resp.Content == nil || len(resp.Content.Parts) != 1 {
		t.Fatalf("response content = %#v", resp.Content)
	}
	fc := resp.Content.Parts[0].FunctionCall
	if fc == nil || fc.ID != "call_1" || fc.Name != "get_weather" || fc.Args["city"] != "Tokyo" {
		t.Fatalf("function call = %#v", fc)
	}
}

func TestContentsFromEventsPreservesToolLoop(t *testing.T) {
	events := []*event.Event{
		{
			Content: &event.Content{Role: event.RoleUser, Parts: []event.Part{{Text: "weather?"}}},
		},
		{
			Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{FunctionCall: &event.FunctionCall{
				ID:   "call_1",
				Name: "get_weather",
				Args: map[string]any{"city": "Tokyo"},
			}}}},
		},
		{
			Content: &event.Content{Role: event.RoleTool, Parts: []event.Part{{FunctionResponse: &event.FunctionResponse{
				ID:     "call_1",
				Name:   "get_weather",
				Result: map[string]any{"temperature": 22},
			}}}},
		},
	}
	contents := ContentsFromEvents(events)
	if len(contents) != 3 {
		t.Fatalf("contents = %d, want 3", len(contents))
	}
	if contents[1].Parts[0].FunctionCall.Args["city"] != "Tokyo" {
		t.Fatalf("function call not preserved: %#v", contents[1].Parts[0].FunctionCall)
	}
	events[1].Content.Parts[0].FunctionCall.Args["city"] = "Paris"
	if contents[1].Parts[0].FunctionCall.Args["city"] != "Tokyo" {
		t.Fatalf("ContentsFromEvents should clone function call args")
	}
}
