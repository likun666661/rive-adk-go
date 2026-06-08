package event

import (
	"testing"
	"time"
)

func TestIsFinalResponse(t *testing.T) {
	tests := []struct {
		name  string
		event *Event
		want  bool
	}{
		{
			name: "nil event",
			event: nil,
			want:  false,
		},
		{
			name:  "partial event is not final",
			event: &Event{Partial: true, Content: &Content{Parts: []Part{{Text: "hello"}}}},
			want:  false,
		},
		{
			name:  "interrupted event is not final",
			event: &Event{Interrupted: true, Content: &Content{Parts: []Part{{Text: "hello"}}}},
			want:  false,
		},
		{
			name:  "event with error is not final",
			event: &Event{ErrorCode: "RATE_LIMITED", Content: &Content{Parts: []Part{{Text: "hello"}}}},
			want:  false,
		},
		{
			name:  "event with function call is not final",
			event: &Event{Content: &Content{Parts: []Part{{FunctionCall: &FunctionCall{Name: "get_weather"}}}}},
			want:  false,
		},
		{
			name:  "event with transfer request is not final",
			event: &Event{Actions: EventActions{TransferToAgent: "agent_2"}, Content: &Content{Parts: []Part{{Text: "done"}}}},
			want:  false,
		},
		{
			name:  "text-only non-partial event is final",
			event: &Event{Content: &Content{Parts: []Part{{Text: "The weather is sunny"}}}},
			want:  true,
		},
		{
			name:  "event with no content is final",
			event: &Event{Timestamp: time.Now()},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.event.IsFinalResponse()
			if got != tt.want {
				t.Errorf("IsFinalResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasFunctionCalls(t *testing.T) {
	tests := []struct {
		name  string
		event *Event
		want  bool
	}{
		{"nil", nil, false},
		{"nil content", &Event{}, false},
		{"text only", &Event{Content: &Content{Parts: []Part{{Text: "hi"}}}}, false},
		{"single func call", &Event{Content: &Content{Parts: []Part{
			{Text: "ok"},
			{FunctionCall: &FunctionCall{Name: "search"}},
		}}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.event.HasFunctionCalls(); got != tt.want {
				t.Errorf("HasFunctionCalls() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFunctionCalls(t *testing.T) {
	fc := &FunctionCall{ID: "fc1", Name: "search", Args: map[string]any{"q": "test"}}
	ev := &Event{Content: &Content{Parts: []Part{
		{FunctionCall: fc},
	}}}

	calls := ev.FunctionCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "search" {
		t.Errorf("expected 'search', got %q", calls[0].Name)
	}
}
