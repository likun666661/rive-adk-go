// Package model defines the LLM interface and a deterministic fake model for tests.
//
// The LLM interface allows the flow to call a model and receive a response.
// The FakeModel is a configurable implementation that returns responses from
// a predefined queue, enabling deterministic integration tests for the flow loop.
package model

import (
	"fmt"
	"sync"

	"github.com/likun666661/rive-adk-go/event"
)

// LLMRequest is the request sent to a model.
type LLMRequest struct {
	Model    string
	Contents []LLMContent
}

// LLMContent represents a single message in the conversation history.
type LLMContent struct {
	Role  string
	Parts []LLMPart
}

// LLMPart is a single piece of content (text or function call/response).
type LLMPart struct {
	Text             string
	FunctionCall     *event.FunctionCall
	FunctionResponse *event.FunctionResponse
}

// LLMResponse is the response from a model.
type LLMResponse struct {
	Content      *LLMContent
	Partial      bool
	TurnComplete bool
	ErrorCode    string
	ErrorMessage string
	Interrupted  bool
}

// LLM is the interface for calling a language model.
type LLM interface {
	Name() string
	GenerateContent(req *LLMRequest) (*LLMResponse, error)
}

// FakeModel is a deterministic model that returns responses from an ordered queue.
// This enables writing integration tests that exercise the full flow loop without
// a real LLM backend.
type FakeModel struct {
	mu        sync.Mutex
	name      string
	responses []*LLMResponse
	nextIdx   int
}

// NewFakeModel creates a FakeModel with the given name and response queue.
func NewFakeModel(name string, responses ...*LLMResponse) *FakeModel {
	return &FakeModel{
		name:      name,
		responses: responses,
	}
}

func (m *FakeModel) Name() string { return m.name }

func (m *FakeModel) GenerateContent(req *LLMRequest) (*LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nextIdx >= len(m.responses) {
		return nil, fmt.Errorf("model %q: no more queued responses (called %d times)", m.name, m.nextIdx)
	}
	resp := m.responses[m.nextIdx]
	m.nextIdx++
	return resp, nil
}

// TextResponse creates a simple LLMResponse with a single text part.
func TextResponse(text string) *LLMResponse {
	return &LLMResponse{
		Content: &LLMContent{
			Role: "model",
			Parts: []LLMPart{
				{Text: text},
			},
		},
		TurnComplete: true,
	}
}

// FunctionCallResponse creates an LLMResponse containing function calls.
func FunctionCallResponse(text string, calls ...event.FunctionCall) *LLMResponse {
	content := &LLMContent{Role: "model"}
	if text != "" {
		content.Parts = append(content.Parts, LLMPart{Text: text})
	}
	for _, fc := range calls {
		fcCopy := fc
		content.Parts = append(content.Parts, LLMPart{FunctionCall: &fcCopy})
	}
	return &LLMResponse{
		Content:      content,
		TurnComplete: true,
	}
}

// ErrorResponse creates an LLMResponse representing a model error.
func ErrorResponse(code, message string) *LLMResponse {
	return &LLMResponse{
		ErrorCode:    code,
		ErrorMessage: message,
	}
}
