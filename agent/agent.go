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
package agent

import (
	"fmt"

	"github.com/likun666661/rive-adk-go/event"
)

// Agent is the base interface that all agents must implement.
//
// TODO: later node — add SubAgents() and FindAgent(name) for agent tree.
// TODO: later node — switch Run to Go 1.23 iter.Seq2 for streaming.
type Agent interface {
	Name() string
	Description() string
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

// Config configures a new Agent.
type Config struct {
	Name                 string
	Description          string
	BeforeAgentCallbacks []BeforeAgentCallback
	AfterAgentCallbacks  []AfterAgentCallback
	Run                  func(ctx InvocationContext) ([]*event.Event, error)
}

// baseAgent is the concrete implementation.
type baseAgent struct {
	name                 string
	description          string
	beforeAgentCallbacks []BeforeAgentCallback
	afterAgentCallbacks  []AfterAgentCallback
	run                  func(ctx InvocationContext) ([]*event.Event, error)
}

func (a *baseAgent) Name() string        { return a.name }
func (a *baseAgent) Description() string { return a.description }

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

// New creates an Agent from a Config.
func New(cfg Config) (*baseAgent, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("agent: name is required")
	}
	if cfg.Run == nil {
		return nil, fmt.Errorf("agent: run function is required")
	}
	return &baseAgent{
		name:                 cfg.Name,
		description:          cfg.Description,
		beforeAgentCallbacks: cfg.BeforeAgentCallbacks,
		afterAgentCallbacks:  cfg.AfterAgentCallbacks,
		run:                  cfg.Run,
	}, nil
}
