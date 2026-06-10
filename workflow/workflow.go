// Package workflow provides agent orchestration primitives for composing
// multiple agents into sequential, parallel, and looped pipelines.
//
// All workflow agents implement agent.Agent and runner.ExecutableAgent,
// allowing them to be used transparently with the existing runner, event,
// and session abstractions.
//
// # State sharing vs branch isolation
//
// Sub-agents share the same underlying session, so state deltas written by
// one sub-agent are visible to subsequent sub-agents in sequential/loop
// workflows. For parallel workflows, concurrent writes to session state are
// protected by the session's internal mutex, but the order of concurrent
// state merges is non‑deterministic.
//
// Branch isolation is achieved by tagging each sub-agent's events with a
// branch identifier of the form "parent.child". The session service uses
// branches for event grouping but does not isolate state by branch.
//
// # Event stream aggregation
//
// Since this replica uses []*event.Event (not iter.Seq2), each sub-agent
// produces all its events at once. Sequential and loop agents simply
// concatenate event slices in order. Parallel agents collect results from
// concurrent goroutines and emit them in sub-agent declaration order for
// deterministic test output.
//
// # Error propagation
//
// Sequential: the first sub-agent error stops the chain immediately.
// Parallel: all sub-agents run to completion; the first error is returned
// alongside successfully produced events.
// Loop: an error from any sub-agent terminates the loop immediately.
//
// # Backpressure simplification
//
// Full ADK Go parallel agents implement per‑event backpressure via an
// ackChan mechanism. This replica simplifies by collecting all events from
// each sub-agent into a slice, so backpressure is not needed.
package workflow

import (
	"fmt"
	"sync"

	"github.com/likun666661/rive-adk-go/agent"
	invctx "github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
)

// SubAgent is the interface for agents used as sub‑agents in workflows.
// It mirrors runner.ExecutableAgent but avoids import cycles between the
// workflow and runner packages.
type SubAgent interface {
	agent.Agent
	Execute(ctx agent.InvocationContext) ([]*event.Event, error)
}

// subCtx wraps a full InvocationContext and overrides Agent() so each
// sub‑agent sees itself as the current agent. EndInvocation/Ended are
// isolated per wrapper: calling EndInvocation() on the wrapper does not
// affect the parent context. This prevents sub‑agent lifecycle calls from
// accidentally terminating the workflow itself.
type subCtx struct {
	invctx.InvocationContext
	a     agent.Agent
	mu    sync.RWMutex
	ended bool
}

func (c *subCtx) Agent() agent.Agent { return c.a }

func (c *subCtx) EndInvocation() {
	c.mu.Lock()
	c.ended = true
	c.mu.Unlock()
}

func (c *subCtx) Ended() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ended
}

// SequentialAgent runs sub‑agents one at a time in declaration order.
//
// Events from each sub‑agent are forwarded in sequence. If a sub‑agent
// returns an error or signals EndInvocation, the chain stops immediately
// and no further sub‑agents are executed. Events produced before the
// failure are still returned.
type SequentialAgent struct {
	name                     string
	description              string
	subAgents                []SubAgent
	parent                   agent.Agent
	disallowTransferToParent bool
	disallowTransferToPeers  bool
}

// NewSequentialAgent creates a SequentialAgent.
func NewSequentialAgent(name, description string, subAgents []SubAgent) *SequentialAgent {
	return &SequentialAgent{
		name:        name,
		description: description,
		subAgents:   subAgents,
	}
}

func (a *SequentialAgent) Name() string                   { return a.name }
func (a *SequentialAgent) Description() string            { return a.description }
func (a *SequentialAgent) Parent() agent.Agent            { return a.parent }
func (a *SequentialAgent) DisallowTransferToParent() bool { return a.disallowTransferToParent }
func (a *SequentialAgent) DisallowTransferToPeers() bool  { return a.disallowTransferToPeers }

func (a *SequentialAgent) SetParentAgent(parent agent.Agent) { a.parent = parent }
func (a *SequentialAgent) SetDisallowTransferToParentFlag(val bool) {
	a.disallowTransferToParent = val
}
func (a *SequentialAgent) SetDisallowTransferToPeersFlag(val bool) {
	a.disallowTransferToPeers = val
}

func (a *SequentialAgent) SubAgents() []agent.Agent {
	result := make([]agent.Agent, len(a.subAgents))
	for i, s := range a.subAgents {
		result[i] = s
	}
	return result
}

func (a *SequentialAgent) FindAgent(name string) agent.Agent {
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

// Execute runs sub‑agents sequentially.
func (a *SequentialAgent) Execute(ctx agent.InvocationContext) ([]*event.Event, error) {
	ic, ok := ctx.(invctx.InvocationContext)
	if !ok {
		return nil, fmt.Errorf("sequential agent %q: expected context.InvocationContext, got %T", a.name, ctx)
	}

	var allEvents []*event.Event
	for _, sub := range a.subAgents {
		sc := &subCtx{InvocationContext: ic, a: sub}
		events, err := sub.Execute(sc)
		if err != nil {
			return allEvents, fmt.Errorf("sequential agent %q: sub‑agent %q: %w", a.name, sub.Name(), err)
		}
		allEvents = append(allEvents, events...)

		if sc.Ended() {
			// Sub-agent requested end of invocation. Propagate to parent
			// and stop the chain.
			ctx.EndInvocation()
			break
		}
	}
	return allEvents, nil
}

// ---------------------------------------------------------------------------
// ParallelAgent
// ---------------------------------------------------------------------------

// ParallelAgent runs sub‑agents concurrently in isolated goroutines.
//
// Each sub‑agent produces its full []*event.Event slice independently.
// Results are collected and emitted in sub‑agent declaration order to
// guarantee deterministic output for tests.
//
// Branch metadata is recorded: each event's Branch field is set to
// "parent.child" (or extended if the sub‑agent already has its own branch).
//
// If multiple sub‑agents return errors, only the first is reported.
// Successfully produced events are always returned alongside the error.
type ParallelAgent struct {
	name                     string
	description              string
	subAgents                []SubAgent
	parent                   agent.Agent
	disallowTransferToParent bool
	disallowTransferToPeers  bool
}

// NewParallelAgent creates a ParallelAgent.
func NewParallelAgent(name, description string, subAgents []SubAgent) *ParallelAgent {
	return &ParallelAgent{
		name:        name,
		description: description,
		subAgents:   subAgents,
	}
}

func (a *ParallelAgent) Name() string                   { return a.name }
func (a *ParallelAgent) Description() string            { return a.description }
func (a *ParallelAgent) Parent() agent.Agent            { return a.parent }
func (a *ParallelAgent) DisallowTransferToParent() bool { return a.disallowTransferToParent }
func (a *ParallelAgent) DisallowTransferToPeers() bool  { return a.disallowTransferToPeers }

func (a *ParallelAgent) SetParentAgent(parent agent.Agent) { a.parent = parent }
func (a *ParallelAgent) SetDisallowTransferToParentFlag(val bool) {
	a.disallowTransferToParent = val
}
func (a *ParallelAgent) SetDisallowTransferToPeersFlag(val bool) {
	a.disallowTransferToPeers = val
}

func (a *ParallelAgent) SubAgents() []agent.Agent {
	result := make([]agent.Agent, len(a.subAgents))
	for i, s := range a.subAgents {
		result[i] = s
	}
	return result
}

func (a *ParallelAgent) FindAgent(name string) agent.Agent {
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

// pResult holds the output of a single sub‑agent in a parallel run.
type pResult struct {
	index  int
	events []*event.Event
	err    error
}

// Execute runs sub‑agents concurrently and aggregates results.
func (a *ParallelAgent) Execute(ctx agent.InvocationContext) ([]*event.Event, error) {
	ic, ok := ctx.(invctx.InvocationContext)
	if !ok {
		return nil, fmt.Errorf("parallel agent %q: expected context.InvocationContext, got %T", a.name, ctx)
	}

	n := len(a.subAgents)
	results := make(chan pResult, n)

	var wg sync.WaitGroup
	for i, sub := range a.subAgents {
		wg.Add(1)
		go func(idx int, sa SubAgent) {
			defer wg.Done()

			branch := fmt.Sprintf("%s.%s", a.name, sa.Name())
			sc := &subCtx{InvocationContext: ic, a: sa}

			events, err := sa.Execute(sc)

			// Tag each event with branch metadata.
			for _, ev := range events {
				if ev != nil {
					switch {
					case ev.Branch == "" || ev.Branch == a.name:
						ev.Branch = branch
					default:
						ev.Branch = branch + "." + ev.Branch
					}
				}
			}

			results <- pResult{index: idx, events: events, err: err}
		}(i, sub)
	}

	wg.Wait()
	close(results)

	// Collect into index-ordered slice for deterministic output.
	ordered := make([]pResult, n)
	for r := range results {
		ordered[r.index] = r
	}

	var allEvents []*event.Event
	var firstErr error
	for _, r := range ordered {
		if r.err != nil && firstErr == nil {
			firstErr = fmt.Errorf("parallel agent %q: sub‑agent %q: %w",
				a.name, a.subAgents[r.index].Name(), r.err)
		}
		allEvents = append(allEvents, r.events...)
	}

	return allEvents, firstErr
}

// ---------------------------------------------------------------------------
// LoopAgent
// ---------------------------------------------------------------------------

// LoopAgent repeatedly runs its sub‑agents in declaration order.
//
// The loop terminates when one of these conditions is met:
//   - maxIterations is reached (each full pass through all sub‑agents counts
//     as one iteration).
//   - Any sub‑agent produces an event with Actions.Escalate set to true.
//   - Any sub‑agent returns an error.
//
// When maxIterations is 0 the loop runs indefinitely (stopping only on
// escalate or error). The Escalate action provides a cooperative stop
// signal: sub‑agents can set event.Actions.Escalate = true to request
// early termination from within their Run logic.
type LoopAgent struct {
	name                     string
	description              string
	subAgents                []SubAgent
	maxIterations            int
	parent                   agent.Agent
	disallowTransferToParent bool
	disallowTransferToPeers  bool
}

// NewLoopAgent creates a LoopAgent.
//
// maxIterations controls how many times the full sub‑agent list is run.
// A value of 0 means "run indefinitely".
func NewLoopAgent(name, description string, subAgents []SubAgent, maxIterations int) *LoopAgent {
	return &LoopAgent{
		name:          name,
		description:   description,
		subAgents:     subAgents,
		maxIterations: maxIterations,
	}
}

func (a *LoopAgent) Name() string                   { return a.name }
func (a *LoopAgent) Description() string            { return a.description }
func (a *LoopAgent) Parent() agent.Agent            { return a.parent }
func (a *LoopAgent) DisallowTransferToParent() bool { return a.disallowTransferToParent }
func (a *LoopAgent) DisallowTransferToPeers() bool  { return a.disallowTransferToPeers }

func (a *LoopAgent) SetParentAgent(parent agent.Agent) { a.parent = parent }
func (a *LoopAgent) SetDisallowTransferToParentFlag(val bool) {
	a.disallowTransferToParent = val
}
func (a *LoopAgent) SetDisallowTransferToPeersFlag(val bool) {
	a.disallowTransferToPeers = val
}

func (a *LoopAgent) SubAgents() []agent.Agent {
	result := make([]agent.Agent, len(a.subAgents))
	for i, s := range a.subAgents {
		result[i] = s
	}
	return result
}

func (a *LoopAgent) FindAgent(name string) agent.Agent {
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

// Execute runs the loop.
func (a *LoopAgent) Execute(ctx agent.InvocationContext) ([]*event.Event, error) {
	ic, ok := ctx.(invctx.InvocationContext)
	if !ok {
		return nil, fmt.Errorf("loop agent %q: expected context.InvocationContext, got %T", a.name, ctx)
	}

	var allEvents []*event.Event
	count := a.maxIterations

	for {
		shouldExit := false
		for _, sub := range a.subAgents {
			sc := &subCtx{InvocationContext: ic, a: sub}
			events, err := sub.Execute(sc)
			if err != nil {
				return allEvents, fmt.Errorf("loop agent %q: iteration %d, sub‑agent %q: %w",
					a.name, a.maxIterations-count, sub.Name(), err)
			}
			allEvents = append(allEvents, events...)

			// Check for Escalate signal in any event produced.
			for _, ev := range events {
				if ev != nil && ev.Actions.Escalate {
					shouldExit = true
					break
				}
			}
			if shouldExit {
				return allEvents, nil
			}
		}

		if a.maxIterations > 0 {
			count--
			if count == 0 {
				return allEvents, nil
			}
		}
	}
}
