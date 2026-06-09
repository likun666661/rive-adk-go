package tool

import (
	"fmt"

	"github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
)

// ToolContext is the context passed to a tool when it is called.
// It provides access to invocation/session identity, state mutation
// helpers, and confirmation status/request records.
type ToolContext interface {
	// InvocationContext returns the parent invocation context, giving
	// access to the agent, session, invocation ID, etc.
	InvocationContext() context.InvocationContext

	// FunctionCallID returns the unique identifier of the function
	// call that triggered this tool execution.
	FunctionCallID() string

	// ToolConfirmation returns the Human-in-the-Loop confirmation
	// handle for the current tool execution, or nil if no confirmation
	// is associated with the call.
	ToolConfirmation() *event.ToolConfirmation

	// RequestConfirmation initiates the HITL approval flow for the
	// current tool call. It records a pending confirmation in the
	// underlying EventActions.
	RequestConfirmation(hint string, payload any) error

	// Actions returns the EventActions for the current event. Tools
	// can mutate the returned value to influence the agent loop.
	Actions() *event.EventActions
}

// NewToolContext constructs a ToolContext for a tool execution.
func NewToolContext(ic context.InvocationContext, functionCallID string, actions *event.EventActions, confirmation *event.ToolConfirmation) ToolContext {
	if actions == nil {
		actions = &event.EventActions{}
	}
	return &toolContextImpl{
		ic:            ic,
		functionCallID: functionCallID,
		actions:        actions,
		confirmation:   confirmation,
	}
}

type toolContextImpl struct {
	ic             context.InvocationContext
	functionCallID string
	actions        *event.EventActions
	confirmation   *event.ToolConfirmation
}

func (t *toolContextImpl) InvocationContext() context.InvocationContext { return t.ic }
func (t *toolContextImpl) FunctionCallID() string                      { return t.functionCallID }
func (t *toolContextImpl) ToolConfirmation() *event.ToolConfirmation    { return t.confirmation }
func (t *toolContextImpl) Actions() *event.EventActions                 { return t.actions }

func (t *toolContextImpl) RequestConfirmation(hint string, payload any) error {
	if t.functionCallID == "" {
		return fmt.Errorf("tool: function call ID not set when requesting confirmation")
	}
	if t.actions.RequestedToolConfirmations == nil {
		t.actions.RequestedToolConfirmations = make(map[string]event.ToolConfirmation)
	}
	t.actions.RequestedToolConfirmations[t.functionCallID] = event.ToolConfirmation{
		Hint:      hint,
		Confirmed: false,
		Payload:   payload,
	}
	t.actions.SkipSummarization = true
	return nil
}
