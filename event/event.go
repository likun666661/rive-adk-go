// Package event defines the core event types for the runtime flow.
//
// Events flow through the chain:
//
//	Runner -> Agent -> Flow -> Model/Tool -> Event -> Session
//
// Each event carries content (LLM response, tool call, tool result),
// represents whether it is partial (streaming) or final, and may
// carry actions that affect execution (state delta, agent transfer, etc.).
package event

import "time"

// Role is the role of the entity that authored this event.
type Role string

const (
	RoleUser  Role = "user"
	RoleModel Role = "model"
	RoleTool  Role = "tool"
)

// Part is a single typed piece of content within an event.
// Only one of the pointer fields should be non-nil.
type Part struct {
	Text          string  `json:"text,omitempty"`
	FunctionCall  *FunctionCall  `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
	Thought       bool    `json:"thought,omitempty"`
}

// FunctionCall represents a model's request to invoke a tool.
type FunctionCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// FunctionResponse represents the result of a tool invocation.
type FunctionResponse struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Result map[string]any `json:"result"`
	Error  string         `json:"error,omitempty"`
}

// Content is the content payload of an event.
type Content struct {
	Role  Role   `json:"role"`
	Parts []Part `json:"parts,omitempty"`
}

// ToolConfirmation represents the state and details of a user confirmation
// request for a tool execution.
type ToolConfirmation struct {
	// Hint is the message provided to the user to explain why confirmation is needed.
	Hint string `json:"hint"`

	// Confirmed indicates the user's decision: true if approved, false if denied.
	Confirmed bool `json:"confirmed"`

	// Payload contains any additional data or context related to the confirmation request.
	Payload any `json:"payload,omitempty"`
}

// EventActions carry side-effect instructions attached to an event.
// They are consumed by the runner or flow after the event is processed.
type EventActions struct {
	// StateDelta is a map of key->value pairs to merge into session state.
	StateDelta map[string]any `json:"stateDelta,omitempty"`

	// ArtifactDelta records filename→version changes produced during
	// callback artifact saves. Each entry records the version returned
	// by the artifact service after a successful Save.
	ArtifactDelta map[string]int64 `json:"artifactDelta,omitempty"`

	// TransferToAgent signals that execution should hand over to a different agent.
	// TODO: later node — implement agent transfer resolution in runner/flow.
	TransferToAgent string `json:"transferToAgent,omitempty"`

	// EndInvocation signals that the current invocation should terminate.
	EndInvocation bool `json:"endInvocation,omitempty"`

	// Escalate signals a request to escalate to a parent agent or human.
	Escalate bool `json:"escalate,omitempty"`

	// SkipSummarization signals that the agent loop should stop after this
	// tool call (used for confirmation flows).
	SkipSummarization bool `json:"skipSummarization,omitempty"`

	// RequestedToolConfirmations maps function call IDs to pending
	// ToolConfirmation entries that require user approval.
	RequestedToolConfirmations map[string]ToolConfirmation `json:"requestedToolConfirmations,omitempty"`
}

// ConfirmationFunctionCallName defines the function call name emitted
// when a Human-in-the-Loop confirmation is required.
const ConfirmationFunctionCallName = "adk_request_confirmation"

// Event is the central unit of communication in the runtime.
//
// Events are produced by agents/models/tools and consumed by the runner
// and session service. Partial events represent streaming chunks; final
// events represent complete units that can be persisted.
type Event struct {
	// ID is a unique identifier for this event.
	ID string `json:"id"`

	// Author is the agent name that produced this event.
	Author string `json:"author"`

	// Role is the role of the author (user, model, tool).
	Role Role `json:"role"`

	// Content is the content payload. May be nil for pure-action events.
	Content *Content `json:"content,omitempty"`

	// Actions are side-effects attached to this event.
	Actions EventActions `json:"actions"`

	// Partial indicates this event is a streaming chunk, not a complete unit.
	// Partial events should NOT be persisted to session history.
	Partial bool `json:"partial"`

	// Timestamp is when the event was created.
	Timestamp time.Time `json:"timestamp"`

	// Branch identifies the agent branch that produced this event.
	// The format is "agent_1.agent_2.agent_3" representing the agent tree path.
	Branch string `json:"branch,omitempty"`

	// Error is an optional error associated with this event.
	Error error `json:"-"`

	// ErrorCode is a machine-readable error code.
	ErrorCode string `json:"errorCode,omitempty"`

	// ErrorMessage is a human-readable error description.
	ErrorMessage string `json:"errorMessage,omitempty"`

	// Interrupted indicates the event was produced due to an interrupt.
	Interrupted bool `json:"interrupted,omitempty"`

	// TurnComplete indicates the model has finished its turn.
	TurnComplete bool `json:"turnComplete,omitempty"`
}

// NewEvent creates a new event with the given ID, author, and role.
func NewEvent(id, author string, role Role) *Event {
	return &Event{
		ID:        id,
		Author:    author,
		Role:      role,
		Timestamp: time.Now(),
	}
}

// IsFinalResponse returns true if this event terminates the current step loop.
//
// An event is final when:
//   - It is not partial
//   - It has no function calls (no tool to execute)
//   - It does not request an agent transfer
//   - It does not have an error that could be retried
//   - It is not interrupted
func (e *Event) IsFinalResponse() bool {
	if e == nil || e.Partial {
		return false
	}
	if e.Interrupted {
		return false
	}
	if e.Error != nil || e.ErrorCode != "" {
		return false
	}
	if e.Actions.TransferToAgent != "" {
		return false
	}
	if c := e.Content; c != nil {
		for _, p := range c.Parts {
			if p.FunctionCall != nil {
				return false
			}
		}
	}
	return true
}

// HasFunctionCalls returns true if the event content contains function calls.
func (e *Event) HasFunctionCalls() bool {
	if e == nil || e.Content == nil {
		return false
	}
	for _, p := range e.Content.Parts {
		if p.FunctionCall != nil {
			return true
		}
	}
	return false
}

// FunctionCalls returns the list of function calls in this event.
func (e *Event) FunctionCalls() []*FunctionCall {
	if e == nil || e.Content == nil {
		return nil
	}
	var calls []*FunctionCall
	for _, p := range e.Content.Parts {
		if p.FunctionCall != nil {
			calls = append(calls, p.FunctionCall)
		}
	}
	return calls
}
