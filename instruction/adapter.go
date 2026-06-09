package instruction

import (
	invctx "github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/session"
)

// ToRequestProcessor wraps NewRequestProcessor output into a flow.RequestProcessor.
// It extracts the ReadonlyContext from the InvocationContext and delegates
// to the instruction processor.
func ToRequestProcessor(proc func(ctx ReadonlyContext, req *model.LLMRequest) (*event.Event, error)) func(ctx invctx.InvocationContext, req *model.LLMRequest) (*event.Event, error) {
	return func(ctx invctx.InvocationContext, req *model.LLMRequest) (*event.Event, error) {
		ro := &readonlyAdapter{ic: ctx}
		return proc(ro, req)
	}
}

// ReadonlyContext is a minimal subset of callbackctx.ReadonlyContext
// needed by instruction processors. It avoids import cycles between
// instruction and callbackctx.
type ReadonlyContext interface {
	UserContent() string
	InvocationID() string
	AgentName() string
	ReadonlyState() session.ReadonlyState
	UserID() string
	AppName() string
	SessionID() string
	Branch() string
}

// readonlyAdapter adapts context.InvocationContext to instruction.ReadonlyContext.
type readonlyAdapter struct {
	ic invctx.InvocationContext
}

func (a *readonlyAdapter) UserContent() string             { return a.ic.UserContent() }
func (a *readonlyAdapter) InvocationID() string            { return a.ic.InvocationID() }
func (a *readonlyAdapter) AgentName() string               { return a.ic.AgentName() }
func (a *readonlyAdapter) ReadonlyState() session.ReadonlyState { return a.ic.ReadonlyState() }
func (a *readonlyAdapter) UserID() string                  { return a.ic.UserID() }
func (a *readonlyAdapter) AppName() string                 { return a.ic.AppName() }
func (a *readonlyAdapter) SessionID() string               { return a.ic.SessionID() }
func (a *readonlyAdapter) Branch() string                  { return a.ic.Branch() }
