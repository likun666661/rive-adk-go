# AgentTool Sandbox — Implementation Report

## Implemented

The `tool/agenttool` package implements the Chapter 05 "agent as tool" pattern: wrapping an
existing agent as a tool callable from a parent flow while running in an isolated child session.

Key design decisions:

1. **Package `tool/agenttool`** — A new sub-package under `tool/` mirroring the ADK Go reference
   structure (`adk-go/tool/agenttool`).

2. **`agentTool` struct** — Implements `tool.Tool`, `tool.DeclarationProvider`, `tool.FunctionTool`,
   and `tool.ContextFunctionTool`, providing maximal compatibility with the tool execution path.

3. **Isolated child session** — Each delegated agent invocation creates a new `runner.InMemorySessionService`,
   `artifact.InMemoryService`, and `memory.InMemoryService`. The child runner is wholly independent
   of the parent, preventing session pollution.

4. **State copying** — Non-internal parent state (all keys that do NOT start with `_adk`) is copied
   from the parent session into the child session before execution. Internal/private state is excluded.

5. **Result extraction** — After the child agent completes, the last event with text content is
   returned as `{"result": "<text>"}`. Empty outputs return an empty map.

6. **SkipSummarization** — When configured, sets `SkipSummarization` on the parent event actions
   to stop the parent agent loop after the tool call.

7. **Flow integration** — Added `tool.ContextFunctionTool` case in `flow.Flow.executeToolCall`
   (before `tool.FunctionTool`) so context-aware tools receive the `ToolContext` with access to
   the parent invocation state.

## Files Changed

- **`tool/agenttool/agent_tool.go`** (new, 145 lines) — Core AgentTool implementation
- **`tool/agenttool/agent_tool_test.go`** (new, 390 lines) — Test suite (9 tests)
- **`flow/flow.go`** (modified, +2 lines) — Added `ContextFunctionTool` case in `executeToolCall`

## Tests

All 9 agenttool tests pass, plus all existing Chapter 01–04 tests continue to pass:

| Test | Coverage |
|------|----------|
| `TestAgentTool_Declaration` | Verifies tool metadata and Declaration generation |
| `TestAgentTool_Run_ChildOutput` | Child agent text output returned in result |
| `TestAgentTool_Run_EmptyOutput` | Empty model response returns empty result map |
| `TestAgentTool_Run_SessionIsolation` | Child session state does NOT leak into parent session |
| `TestAgentTool_Run_ParentStateCopied` | Non-internal parent state (`shared_key`) visible in child |
| `TestAgentTool_Run_InternalStateNotCopied` | `_adk` prefixed keys excluded from child session |
| `TestAgentTool_Run_MissingRequest` | Error on missing `request` argument |
| `TestAgentTool_Run_SkipSummarization` | `SkipSummarization` flag set/not-set per config |
| `TestAgentTool_Run_ChildSessionHasOwnRunner` | Child agent runs in isolated runner environment |

Full project test run: `go test ./...` — all 17 packages pass (including existing agent, artifact,
context, event, flow, llmagent, memory, model, plugin, runner, session, tool, workflow).

## Notes

- The implementation follows the rive-adk-go conventions: `tool.Declaration` with `map[string]any`
  schemas instead of Google `genai` types, `runner.Runner.Run` returning `([]*event.Event, error)`
  instead of `iter.Seq2`, and `agent.InvocationContext` as the minimal agent-facing interface.

- `ContextFunctionTool` support was added to `flow.Flow.executeToolCall` so that agent tools
  receive the parent `ToolContext` with access to `InvocationContext().Session().State()` for
  state copying. This is backward-compatible since `FunctionTool` tools still work unchanged.

- Internal state is identified by `_adk` key prefix (matching the ADK Go convention). In the
  rive-adk-go state model, `temp:` keys are invocation-local and naturally don't need explicit
  filtering since they're scoped to individual sessions.
