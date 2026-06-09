package context

import (
	"fmt"

	stdctx "context"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/artifact"
	"github.com/likun666661/rive-adk-go/callbackctx"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/memory"
	"github.com/likun666661/rive-adk-go/plugin"
	"github.com/likun666661/rive-adk-go/session"
)

// NewCallbackContext creates a CallbackContext from an InvocationContext and
// an EventActions reference. State writes go through to both the delta and
// durable session state (write‑through).
//
// The returned context satisfies CallbackContext but not ToolContext.
func NewCallbackContext(ic InvocationContext, actions *event.EventActions) callbackctx.CallbackContext {
	prepareEventActions(actions)
	return &callbackContext{
		invocationContext: ic,
		actions:           actions,
		functionCallID:    "",
	}
}

// NewCallbackContextWithArtifactTracking creates a CallbackContext whose
// ArtifactService() is decorated to automatically record saved versions
// into actions.ArtifactDelta.
func NewCallbackContextWithArtifactTracking(ic InvocationContext, actions *event.EventActions) callbackctx.CallbackContext {
	prepareEventActions(actions)
	return &callbackContext{
		invocationContext: ic,
		actions:           actions,
		functionCallID:    "",
		trackArtifacts:    true,
	}
}

// NewToolContext creates a ToolContext for a specific tool invocation.
// This provides access to FunctionCallID, Actions, and SearchMemory.
func NewToolContext(ic InvocationContext, functionCallID string, actions *event.EventActions) callbackctx.ToolContext {
	prepareEventActions(actions)
	return &callbackContext{
		invocationContext: ic,
		actions:           actions,
		functionCallID:    functionCallID,
		trackArtifacts:    true,
	}
}

// prepareEventActions ensures that StateDelta and ArtifactDelta maps are
// initialized before any callback writes to them.
func prepareEventActions(actions *event.EventActions) {
	if actions.StateDelta == nil {
		actions.StateDelta = make(map[string]any)
	}
	if actions.ArtifactDelta == nil {
		actions.ArtifactDelta = make(map[string]int64)
	}
}

// callbackContext is the single concrete type that satisfies both
// CallbackContext and ToolContext.
type callbackContext struct {
	invocationContext InvocationContext
	actions           *event.EventActions
	functionCallID    string
	trackArtifacts    bool
}

// ==========================================================================
// ReadonlyContext methods — delegated to InvocationContext
// ==========================================================================

func (c *callbackContext) UserContent() string  { return c.invocationContext.UserContent() }
func (c *callbackContext) InvocationID() string { return c.invocationContext.InvocationID() }
func (c *callbackContext) AgentName() string    { return c.invocationContext.AgentName() }
func (c *callbackContext) ReadonlyState() session.ReadonlyState {
	return c.invocationContext.ReadonlyState()
}
func (c *callbackContext) UserID() string    { return c.invocationContext.UserID() }
func (c *callbackContext) AppName() string   { return c.invocationContext.AppName() }
func (c *callbackContext) SessionID() string { return c.invocationContext.SessionID() }
func (c *callbackContext) Branch() string    { return c.invocationContext.Branch() }

// ==========================================================================
// CallbackContext methods
// ==========================================================================

func (c *callbackContext) ArtifactService() artifact.Service {
	if c.trackArtifacts && c.actions != nil {
		return &trackedArtifacts{
			inner:   c.invocationContext.ArtifactService(),
			actions: c.actions,
		}
	}
	return c.invocationContext.ArtifactService()
}

func (c *callbackContext) MemoryService() memory.Service {
	return c.invocationContext.MemoryService()
}

// State returns a write‑through state that records writes into
// actions.StateDelta and also persists them to the durable session state.
//
// Reads first check the current step's StateDelta (so callbacks can see
// writes from earlier callbacks in the same step) and fall back to the
// durable session state.
func (c *callbackContext) State() session.State {
	return &callbackContextState{
		ctx: c,
	}
}

// ==========================================================================
// ToolContext methods
// ==========================================================================

func (c *callbackContext) FunctionCallID() string {
	return c.functionCallID
}

func (c *callbackContext) Actions() *event.EventActions {
	return c.actions
}

func (c *callbackContext) SearchMemory(ctx stdctx.Context, query string) (*memory.SearchResponse, error) {
	return c.invocationContext.MemoryService().SearchMemory(ctx, &memory.SearchRequest{
		Query:   query,
		UserID:  c.invocationContext.UserID(),
		AppName: c.invocationContext.AppName(),
	})
}

// ==========================================================================
// callbackContextState — write‑through state with delta‑prioritized reads
// ==========================================================================

// callbackContextState is a session.State decorator that:
//   - Reads: checks actions.StateDelta first, then falls back to durable state.
//   - Writes: records into actions.StateDelta and immediately persists to
//     durable session state (write‑through strategy).
//
// This means callbacks in the same step can see each other's writes via the
// delta, and the writes are durable even if a later callback errors out.
type callbackContextState struct {
	ctx *callbackContext
}

func (c *callbackContextState) Get(key string) (any, bool) {
	if c.ctx.actions != nil && c.ctx.actions.StateDelta != nil {
		if v, ok := c.ctx.actions.StateDelta[key]; ok {
			return v, true
		}
	}
	return c.ctx.invocationContext.Session().State().Get(key)
}

func (c *callbackContextState) Set(key string, val any) {
	if c.ctx.actions != nil && c.ctx.actions.StateDelta != nil {
		c.ctx.actions.StateDelta[key] = val
	}
	c.ctx.invocationContext.Session().State().Set(key, val)
}

func (c *callbackContextState) Delete(key string) {
	if c.ctx.actions != nil && c.ctx.actions.StateDelta != nil {
		c.ctx.actions.StateDelta[key] = session.TombstoneValue
	}
	c.ctx.invocationContext.Session().State().Delete(key)
}

func (c *callbackContextState) All() map[string]any {
	merged := c.ctx.invocationContext.Session().State().All()
	if c.ctx.actions != nil && c.ctx.actions.StateDelta != nil {
		for k, v := range c.ctx.actions.StateDelta {
			merged[k] = v
		}
	}
	return merged
}

// ==========================================================================
// trackedArtifacts — artifact save tracking
// ==========================================================================

// trackedArtifacts decorates artifact.Service so that every successful
// Save() records the returned version number into actions.ArtifactDelta.
type trackedArtifacts struct {
	inner   artifact.Service
	actions *event.EventActions
}

func (t *trackedArtifacts) Save(ctx stdctx.Context, req *artifact.SaveRequest) (*artifact.SaveResponse, error) {
	resp, err := t.inner.Save(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("artifact save failed: %w", err)
	}
	if t.actions != nil && t.actions.ArtifactDelta != nil {
		t.actions.ArtifactDelta[req.FileName] = resp.Version
	}
	return resp, nil
}

func (t *trackedArtifacts) Load(ctx stdctx.Context, req *artifact.LoadRequest) (*artifact.LoadResponse, error) {
	return t.inner.Load(ctx, req)
}

func (t *trackedArtifacts) Delete(ctx stdctx.Context, req *artifact.DeleteRequest) error {
	return t.inner.Delete(ctx, req)
}

func (t *trackedArtifacts) List(ctx stdctx.Context, req *artifact.ListRequest) (*artifact.ListResponse, error) {
	return t.inner.List(ctx, req)
}

func (t *trackedArtifacts) Versions(ctx stdctx.Context, req *artifact.VersionsRequest) (*artifact.VersionsResponse, error) {
	return t.inner.Versions(ctx, req)
}

func (t *trackedArtifacts) GetArtifactVersion(ctx stdctx.Context, req *artifact.GetArtifactVersionRequest) (*artifact.GetArtifactVersionResponse, error) {
	return t.inner.GetArtifactVersion(ctx, req)
}

// Compile‑time interface check.
var _ callbackctx.CallbackContext = (*callbackContext)(nil)
var _ callbackctx.ToolContext = (*callbackContext)(nil)
var _ session.State = (*callbackContextState)(nil)
var _ artifact.Service = (*trackedArtifacts)(nil)

// ==========================================================================
// RunWithCallbackContext — agent lifecycle with CallbackContext‑aware callbacks
// ==========================================================================

// RunWithCallbackContext executes the agent lifecycle using
// CallbackContext‑aware callbacks. It accepts a full InvocationContext so
// that state deltas and artifact tracking can be routed through
// EventActions.
//
// The lifecycle:
//  1. Run plugin before-agent hooks (if PluginManager is set).
//  2. Run ctx‑aware before callbacks with a shared EventActions.
//  3. Run the agent with the InvocationContext.
//  4. Run plugin after-agent hooks.
//  5. Run ctx‑aware after callbacks with a shared EventActions.
//
// Plugin hooks always run before direct callbacks, matching the Chapter 04
// teaching model.
func RunWithCallbackContext(
	ic InvocationContext,
	actions *event.EventActions,
	pm *plugin.Manager,
	beforeCallbacks []agent.BeforeAgentCallbackCtx,
	run func(agent.InvocationContext) ([]*event.Event, error),
	afterCallbacks []agent.AfterAgentCallbackCtx,
) ([]*event.Event, error) {
	var allEvents []*event.Event

	cctx := NewCallbackContext(ic, actions)

	if pm != nil {
		ev, err := pm.RunBeforeAgentCallback(cctx)
		if err != nil {
			return nil, err
		}
		if ev != nil {
			return []*event.Event{ev}, nil
		}
	}

	ev, err := runBeforeCallbacksCtx(cctx, beforeCallbacks)
	if err != nil {
		return nil, err
	}
	if ev != nil {
		return []*event.Event{ev}, nil
	}

	runEvents, err := run(ic)
	if err != nil {
		return nil, err
	}
	allEvents = append(allEvents, runEvents...)

	if pm != nil {
		ev, err = pm.RunAfterAgentCallback(cctx, runEvents)
		if err != nil {
			return nil, err
		}
		if ev != nil {
			allEvents = append(allEvents, ev)
			if ev.Actions.EndInvocation {
				ic.EndInvocation()
			}
			return allEvents, nil
		}
	}

	ev, err = runAfterCallbacksCtx(cctx, runEvents, afterCallbacks)
	if err != nil {
		return nil, err
	}
	if ev != nil {
		allEvents = append(allEvents, ev)
		if ev.Actions.EndInvocation {
			ic.EndInvocation()
		}
	}

	return allEvents, nil
}

func runBeforeCallbacksCtx(cctx callbackctx.CallbackContext, cbs []agent.BeforeAgentCallbackCtx) (*event.Event, error) {
	for i, cb := range cbs {
		if cb == nil {
			continue
		}
		ev, err := cb(cctx)
		if err != nil {
			return nil, fmt.Errorf("beforeAgentCallbackCtx[%d]: %w", i, err)
		}
		if ev != nil {
			return ev, nil
		}
	}
	return nil, nil
}

func runAfterCallbacksCtx(cctx callbackctx.CallbackContext, events []*event.Event, cbs []agent.AfterAgentCallbackCtx) (*event.Event, error) {
	for i, cb := range cbs {
		if cb == nil {
			continue
		}
		ev, err := cb(cctx, events)
		if err != nil {
			return nil, fmt.Errorf("afterAgentCallbackCtx[%d]: %w", i, err)
		}
		if ev != nil {
			return ev, nil
		}
	}
	return nil, nil
}
