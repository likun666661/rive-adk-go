package model

import (
	"testing"

	"github.com/likun666661/rive-adk-go/event"
)

func TestFakeModelTextResponse(t *testing.T) {
	m := NewFakeModel("test", TextResponse("hello"))
	resp, err := m.GenerateContent(&LLMRequest{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content == nil || len(resp.Content.Parts) == 0 {
		t.Fatal("expected content with parts")
	}
	if resp.Content.Parts[0].Text != "hello" {
		t.Errorf("text = %q", resp.Content.Parts[0].Text)
	}
	if !resp.TurnComplete {
		t.Error("expected TurnComplete=true")
	}
}

func TestFakeModelMultipleResponses(t *testing.T) {
	m := NewFakeModel("test",
		TextResponse("first"),
		TextResponse("second"),
	)

	resp1, _ := m.GenerateContent(nil)
	if resp1.Content.Parts[0].Text != "first" {
		t.Errorf("first = %q", resp1.Content.Parts[0].Text)
	}

	resp2, _ := m.GenerateContent(nil)
	if resp2.Content.Parts[0].Text != "second" {
		t.Errorf("second = %q", resp2.Content.Parts[0].Text)
	}

	_, err := m.GenerateContent(nil)
	if err == nil {
		t.Error("expected error when queue exhausted")
	}
}

func TestFakeModelFunctionCallResponse(t *testing.T) {
	m := NewFakeModel("test",
		FunctionCallResponse("Let me check.",
			event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
			event.FunctionCall{ID: "fc2", Name: "search", Args: map[string]any{"q": "weather"}},
		),
	)

	resp, err := m.GenerateContent(nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content == nil {
		t.Fatal("expected content")
	}
	if len(resp.Content.Parts) != 3 {
		t.Fatalf("expected 3 parts (text + 2 function calls), got %d", len(resp.Content.Parts))
	}
	if resp.Content.Parts[0].Text != "Let me check." {
		t.Errorf("text = %q", resp.Content.Parts[0].Text)
	}
	if fc := resp.Content.Parts[1].FunctionCall; fc == nil || fc.Name != "get_weather" {
		t.Errorf("function call 1 = %v", fc)
	}
	if fc := resp.Content.Parts[2].FunctionCall; fc == nil || fc.Name != "search" {
		t.Errorf("function call 2 = %v", fc)
	}
}

func TestFakeModelErrorResponse(t *testing.T) {
	m := NewFakeModel("test",
		ErrorResponse("RATE_LIMITED", "too many requests"),
	)

	resp, err := m.GenerateContent(nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ErrorCode != "RATE_LIMITED" {
		t.Errorf("ErrorCode = %q", resp.ErrorCode)
	}
	if resp.ErrorMessage != "too many requests" {
		t.Errorf("ErrorMessage = %q", resp.ErrorMessage)
	}
	if resp.Content != nil {
		t.Error("error response should have nil content")
	}
}

func TestFakeModelName(t *testing.T) {
	m := NewFakeModel("gemini-pro")
	if m.Name() != "gemini-pro" {
		t.Errorf("Name = %q", m.Name())
	}
}
