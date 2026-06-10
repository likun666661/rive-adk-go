// Package agent defines the Agent interface and base agent callback lifecycle.
//
// The Agent is the central abstraction in the runtime. Every agent
// has a Name, a Run function, and optional before/after callbacks.
//
// Execution lifecycle for a single agent run:
//
//	beforeAgentCallbacks()  →  if non-nil content → yield event; return
//	a.run(ctx)              →  iterate events from agent logic
//	afterAgentCallbacks()   →  yield final callback events
//
// Callbacks can return content to shortcut execution (early exit),
// produce side effects (state delta only), or return errors.
//
// Two callback flavours are supported:
//
//   - Legacy: BeforeAgentCallback / AfterAgentCallback accepting
//     InvocationContext (preserved for Chapter 01‑03 compatibility).
//   - Context‑aware: BeforeAgentCallbackCtx / AfterAgentCallbackCtx
//     accepting callbackctx.CallbackContext, providing write‑through state
//     with delta recording, artifact tracking, and readonly identity
//     information.
//
// Use context.RunWithCallbackContext to execute agents with the
// CallbackContext‑aware lifecycle.
package agent

import (
	"fmt"

	"github.com/likun666661/rive-adk-go/callbackctx"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/plugin"
)

// Agent is the base interface that all agents must implement.
//
// SubAgents returns the direct child agents in the tree.
// FindAgent performs a depth-first search for an agent by name,
// returning nil if not found.
type Agent interface {
	Name() string
	Description() string
	SubAgents() []Agent
	FindAgent(name string) Agent
	Parent() Agent
	DisallowTransferToParent() bool
	DisallowTransferToPeers() bool
}

// InvocationContext is the context interface consumed by agent callbacks.
// It mirrors the full context.InvocationContext but only requires methods
// relevant to callback execution, avoiding import cycles.
type InvocationContext interface {
	Agent() Agent
	EndInvocation()
	Ended() bool
}

// BeforeAgentCallback is invoked before the agent's Run function.
// Returning non-nil *event.Event causes early exit: the event is yielded
// and Run is skipped. A nil event with a non-nil error also aborts.
type BeforeAgentCallback func(ctx InvocationContext) (*event.Event, error)

// AfterAgentCallback is invoked after the agent's Run completes.
// The events parameter contains all events produced by Run.
type AfterAgentCallback func(ctx InvocationContext, events []*event.Event) (*event.Event, error)

// BeforeAgentCallbackCtx is a context‑aware before‑agent callback.
// It receives a full CallbackContext, enabling:
//   - State reads/writes with delta recording (State()).
//   - Artifact save tracking (ArtifactService() saves are auto‑recorded).
//   - Readonly identity/session/user/app/branch information.
type BeforeAgentCallbackCtx func(ctx callbackctx.CallbackContext) (*event.Event, error)

// AfterAgentCallbackCtx is a context‑aware after‑agent callback.
type AfterAgentCallbackCtx func(ctx callbackctx.CallbackContext, events []*event.Event) (*event.Event, error)

// Config configures a new Agent.
type Config struct {
	Name                 string
	Description          string
	PluginManager        *plugin.Manager
	BeforeAgentCallbacks []BeforeAgentCallback
	AfterAgentCallbacks  []AfterAgentCallback
	Run                  func(ctx InvocationContext) ([]*event.Event, error)

	SubAgents                []Agent
	Parent                   Agent
	DisallowTransferToParent bool
	DisallowTransferToPeers  bool
}

// baseAgent is the concrete implementation.
type baseAgent struct {
	name                 string
	description          string
	pluginManager        *plugin.Manager
	beforeAgentCallbacks []BeforeAgentCallback
	afterAgentCallbacks  []AfterAgentCallback
	run                  func(ctx InvocationContext) ([]*event.Event, error)

	subAgents                []Agent
	parent                   Agent
	disallowTransferToParent bool
	disallowTransferToPeers  bool
}

func (a *baseAgent) Name() string                   { return a.name }
func (a *baseAgent) Description() string            { return a.description }
func (a *baseAgent) Parent() Agent                  { return a.parent }
func (a *baseAgent) DisallowTransferToParent() bool { return a.disallowTransferToParent }
func (a *baseAgent) DisallowTransferToPeers() bool  { return a.disallowTransferToPeers }

// SubAgents returns the direct child agents.
func (a *baseAgent) SubAgents() []Agent {
	return append([]Agent(nil), a.subAgents...)
}

// FindAgent performs a depth-first search for an agent by name.
func (a *baseAgent) FindAgent(name string) Agent {
	if a.name == name {
		return a
	}
	for _, sub := range a.subAgents {
		if found := sub.FindAgent(name); found != nil {
			return found
		}
	}
	return nil
}

// PluginManager returns the agent's plugin manager, or nil.
func (a *baseAgent) PluginManager() *plugin.Manager { return a.pluginManager }

// Execute runs the full lifecycle: before callbacks → run → after callbacks.
// It returns all events produced and the last error encountered.
//
// The lifecycle is:
//  1. Run beforeAgentCallbacks in order. The first non-nil event short-circuits.
//  2. If no early exit, run a.run(ctx).
//  3. Run afterAgentCallbacks in order. The first non-nil event is appended.
//  4. If after-callback returns an event with EndInvocation, mark ctx ended.
func (a *baseAgent) Execute(ctx InvocationContext) ([]*event.Event, error) {
	var allEvents []*event.Event

	ev, err := runBeforeCallbacks(ctx, a.beforeAgentCallbacks)
	if err != nil {
		return nil, err
	}
	if ev != nil {
		return []*event.Event{ev}, nil
	}

	runEvents, err := a.run(ctx)
	if err != nil {
		return nil, err
	}
	allEvents = append(allEvents, runEvents...)

	ev, err = runAfterCallbacks(ctx, runEvents, a.afterAgentCallbacks)
	if err != nil {
		return nil, err
	}
	if ev != nil {
		allEvents = append(allEvents, ev)
		if ev.Actions.EndInvocation {
			ctx.EndInvocation()
		}
	}

	return allEvents, nil
}

// New creates an Agent from a Config.
func New(cfg Config) (*baseAgent, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("agent: name is required")
	}
	if cfg.Run == nil {
		return nil, fmt.Errorf("agent: run function is required")
	}
	return &baseAgent{
		name:                     cfg.Name,
		description:              cfg.Description,
		pluginManager:            cfg.PluginManager,
		beforeAgentCallbacks:     cfg.BeforeAgentCallbacks,
		afterAgentCallbacks:      cfg.AfterAgentCallbacks,
		run:                      cfg.Run,
		subAgents:                cfg.SubAgents,
		parent:                   cfg.Parent,
		disallowTransferToParent: cfg.DisallowTransferToParent,
		disallowTransferToPeers:  cfg.DisallowTransferToPeers,
	}, nil
}

// SetSubAgents replaces the sub-agents list on agents that support mutation.
// Agent types without a mutator are silently skipped.
func SetSubAgents(a Agent, subs []Agent) error {
	ba, ok := a.(*baseAgent)
	if !ok {
		if setter, ok := a.(interface{ SetSubAgentsForAgent([]Agent) }); ok {
			setter.SetSubAgentsForAgent(subs)
		}
		return nil
	}
	ba.subAgents = append([]Agent(nil), subs...)
	return nil
}

// SetParent sets the parent reference on agents that support mutation.
// Agent types without a mutator are silently skipped.
func SetParent(a Agent, parent Agent) error {
	ba, ok := a.(*baseAgent)
	if !ok {
		if setter, ok := a.(interface{ SetParentAgent(Agent) }); ok {
			setter.SetParentAgent(parent)
		}
		return nil
	}
	ba.parent = parent
	return nil
}

// SetDisallowTransferToParent sets the transfer-to-parent constraint on a baseAgent.
func SetDisallowTransferToParent(a Agent, val bool) error {
	ba, ok := a.(*baseAgent)
	if !ok {
		if setter, ok := a.(interface{ SetDisallowTransferToParentFlag(bool) }); ok {
			setter.SetDisallowTransferToParentFlag(val)
		}
		return nil
	}
	ba.disallowTransferToParent = val
	return nil
}

// SetDisallowTransferToPeers sets the transfer-to-peers constraint on a baseAgent.
func SetDisallowTransferToPeers(a Agent, val bool) error {
	ba, ok := a.(*baseAgent)
	if !ok {
		if setter, ok := a.(interface{ SetDisallowTransferToPeersFlag(bool) }); ok {
			setter.SetDisallowTransferToPeersFlag(val)
		}
		return nil
	}
	ba.disallowTransferToPeers = val
	return nil
}

func runBeforeCallbacks(ctx InvocationContext, cbs []BeforeAgentCallback) (*event.Event, error) {
	for i, cb := range cbs {
		if cb == nil {
			continue
		}
		ev, err := cb(ctx)
		if err != nil {
			return nil, fmt.Errorf("beforeAgentCallback[%d]: %w", i, err)
		}
		if ev != nil {
			return ev, nil
		}
	}
	return nil, nil
}

func runAfterCallbacks(ctx InvocationContext, events []*event.Event, cbs []AfterAgentCallback) (*event.Event, error) {
	for i, cb := range cbs {
		if cb == nil {
			continue
		}
		ev, err := cb(ctx, events)
		if err != nil {
			return nil, fmt.Errorf("afterAgentCallback[%d]: %w", i, err)
		}
		if ev != nil {
			return ev, nil
		}
	}
	return nil, nil
}
