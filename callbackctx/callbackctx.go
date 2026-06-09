// Package callbackctx defines the callback context interfaces shared between
// agent, flow, and context packages. Keeping these interfaces in a separate
// lightweight package avoids import cycles.
//
// ReadonlyContext provides immutable identity/session/user/app/branch info.
// CallbackContext adds write-through state and artifact/memory service access.
// ToolContext adds function-call-specific metadata and search.
package callbackctx

import (
	stdctx "context"

	"github.com/likun666661/rive-adk-go/artifact"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/memory"
	"github.com/likun666661/rive-adk-go/session"
)

// ReadonlyContext exposes readonly identity, session, user, app, and branch
// information to callbacks. It is a minimal read-only surface designed to
// prevent callbacks from calling EndInvocation() or accessing RunConfig().
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

// CallbackContext is the unified callback context providing readonly information
// plus write-through state and artifact/memory service access.
// It embeds ReadonlyContext and adds writable state, artifact tracking, and
// memory/artifact service access.
type CallbackContext interface {
	ReadonlyContext

	ArtifactService() artifact.Service
	MemoryService() memory.Service
	State() session.State
}

// ToolContext extends CallbackContext with tool-specific metadata:
// the current function call ID, access to the event's EventActions,
// and a SearchMemory method scoped to the invocation.
type ToolContext interface {
	CallbackContext

	FunctionCallID() string
	Actions() *event.EventActions
	SearchMemory(ctx stdctx.Context, query string) (*memory.SearchResponse, error)
}
