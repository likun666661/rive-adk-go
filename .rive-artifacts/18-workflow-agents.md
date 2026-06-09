# Workflow Agents — Implementation Report

## implemented

Added a `workflow` package (`workflow/workflow.go`) providing three orchestration agents:

1. **SequentialAgent** — Runs sub-agents in declaration order. Events are concatenated in
   sequence. The first sub-agent error or `EndInvocation` stops the chain immediately.

2. **ParallelAgent** — Runs sub-agents concurrently via goroutines. Each sub-agent
   receives a branch-tagged wrapper context (`subCtx`) that isolates `EndInvocation` state
   and presents the sub-agent as `ctx.Agent()`. Results are collected via buffered channels
   and emitted in declaration order for deterministic test output. Branch metadata
   (`parent.child`) is recorded on every event.

3. **LoopAgent** — Repeatedly runs sub-agents in sequence. Termination conditions:
   `maxIterations` (0 = infinite), `Actions.Escalate` on any event, or sub-agent error.

All three implement `agent.Agent` and `runner.ExecutableAgent`, so they can be used
transparently with the existing runner/session/event abstractions.

### Key design decisions

| Concern | Approach |
|---------|----------|
| Context isolation | `subCtx` embeds `context.InvocationContext` and overrides `Agent()`, `EndInvocation()`, `Ended()` |
| State sharing | Sub-agents share the same session; writes from one are visible to subsequent agents |
| Branch isolation | Branch `"parent.child"` tag on events provides identity grouping; state is NOT isolated by branch |
| Error propagation | Sequential/loop: stop on first error. Parallel: run all, report first error with all events |
| Backpressure | Simplified out — this replica uses `[]*event.Event` (not `iter.Seq2`), so backpressure is irrelevant |
| Event aggregation | Sequential/loop: simple slice concatenation. Parallel: index-ordered deterministic merging |

## files_changed

- `workflow/workflow.go` — new file: SequentialAgent, ParallelAgent, LoopAgent, subCtx, SubAgent interface
- `workflow/workflow_test.go` — new file: 18 tests covering all scenarios

## tests

18 tests, all passing (`go test ./...` — all packages pass):

| Test | What it verifies |
|------|-----------------|
| `TestSequentialAgentOrder` | Events appear in sub-agent declaration order |
| `TestSequentialAgentErrorStopsChain` | First error stops the chain, prior events preserved |
| `TestSequentialAgentEndInvocationStopsChain` | `EndInvocation()` halts the sequence |
| `TestParallelAgentBranchAndEventAggregation` | Branch tags set; events in declaration order |
| `TestParallelAgentErrorPropagation` | All sub-agents run; first error reported; ok events preserved |
| `TestLoopAgentMaxIterations` | Exact iteration count; all events present |
| `TestLoopAgentEarlyStopOnEscalate` | `Escalate` stops loop before max iterations |
| `TestLoopAgentErrorStopsLoop` | Sub-agent error terminates loop; prior events preserved |
| `TestLoopAgentZeroMaxIterations` | `maxIterations=0` runs until `Escalate` (5 iterations) |
| `TestNestedSequentialInParallel` | Workflow agent as sub-agent of another workflow |
| `TestSequentialAgentStateSharing` | State set by first sub-agent visible to second (shared session) |
| `TestLoopAgentMultipleSubAgents` | Multi-sub-agent loop: correct order across iterations |
| `TestSequentialAgentEmptySubAgents` | Zero sub-agents → zero events, no error |
| `TestParallelAgentEmptySubAgents` | Zero sub-agents → zero events, no error |
| `TestLoopAgentEmptySubAgents` | Zero sub-agents → zero events, no error |
| `TestSequentialAgentImplementsAgent` | Interface satisfaction check |
| `TestParallelAgentImplementsAgent` | Interface satisfaction check |
| `TestLoopAgentImplementsAgent` | Interface satisfaction check |

## notes

- The `subCtx` wrapper is the critical glue: it embeds `context.InvocationContext` so all
  downstream methods (Session, Branch, UserContent, etc.) delegate to the parent
  unmodified. Only `Agent()` is shadowed to present the sub-agent identity.
- `EndInvocation()` and `Ended()` are isolated per `subCtx` instance, preventing
  sub-agent lifecycle calls from accidentally terminating the parent workflow.
- Parallel agent uses `sync.WaitGroup` + buffered channel + index-based ordering.
  This is simpler than the real ADK Go's `errgroup` + `ackChan` backpressure model
  but still demonstrates the core concepts.
- The `SubAgent` interface mirrors `runner.ExecutableAgent` to avoid import cycles
  between the `workflow` and `runner` packages.
- All existing Chapter 01–04 APIs and tests are preserved; no modifications required.
