# Node 2: Implement Flow, Model, Tool Loop

## implemented

- **`model` package** (`model/model.go`): LLM interface with `Name()` and `GenerateContent(req)`. `FakeModel` is a
  deterministic, queue-backed implementation that returns pre-configured responses in order.
  Helpers: `TextResponse`, `FunctionCallResponse`, `ErrorResponse`.

- **`tool` package** (`tool/tool.go`): `FunctionTool` interface (Name, Description, Run). `FuncTool` convenience
  wrapper. `Execute` captures tool errors as structured `CallResult` entries (error string + `result["error"]`)
  so the flow never silently swallows failures. `MergeResults` combines parallel results by call ID.

- **`flow` package** (`flow/flow.go`): `Flow` struct with:
  - Request/response processor hooks (pipeline pattern)
  - Before/after model callback hooks (short-circuit on non-nil response)
  - Before/after tool callback hooks (pre-execute override, post-execute transform)
  - `Run(ctx)` → multi-step loop: calls `runOneStep` until the model event is final
  - `runOneStep` → preprocess → callModel (with empty-response skip/retry) → postprocess → finalizeEvent → handleFunctionCalls
  - `handleFunctionCalls` → parallel execution via `sync.WaitGroup` → results merged into a single tool event
  - State delta extraction from tool results (key `state_delta`) and merge via `session.MergeStateDelta`
  - Loop termination when the model response event `IsFinalResponse()`

## files_changed

| File | Action |
|---|---|
| `tool/tool.go` | New — FunctionTool interface, Execute, MergeResults |
| `tool/tool_test.go` | New — 7 tests |
| `model/model.go` | New — LLM interface, FakeModel, response helpers |
| `model/model_test.go` | New — 5 tests |
| `flow/flow.go` | New — Flow.Run, runOneStep, processor/callback hooks, parallel tools |
| `flow/flow_test.go` | New — 14 tests |

## tests

```
ok  	github.com/likun666661/rive-adk-go/agent     (cached)
ok  	github.com/likun666661/rive-adk-go/event     (cached)
ok  	github.com/likun666661/rive-adk-go/flow      0.619s
ok  	github.com/likun666661/rive-adk-go/model     0.654s
ok  	github.com/likun666661/rive-adk-go/session    (cached)
ok  	github.com/likun666661/rive-adk-go/tool      0.893s
```

26 new tests total (14 flow + 5 model + 7 tool).

### Coverage summary

- **Final model response**: text-only response terminates the loop in 1 step.
- **One tool call + final**: model returns function call → tool executes → next step returns final text (3 events).
- **Multiple parallel tool calls**: 2 FunctionCalls in one response → both execute concurrently → result event has 2 FunctionResponse parts.
- **Deterministic merge**: `MergeResults` indexes by call ID for deterministic lookup.
- **State delta**: tool result containing `state_delta` key is deep-merged into session state.
- **Processor/callback ordering**: verified reqProcessor1 → reqProcessor2 → beforeModel → afterModel → respProcessor.
- **Request processor short-circuit**: non-nil event from processor skips model call.
- **Before model callback short-circuit**: non-nil response skips model call.
- **Before tool callback override**: cached result returned, actual tool not called.
- **After tool callback transform**: result enriched with computed value.
- **Tool error → event error**: failing tool populates `CallResult.Error` and `FunctionResponse.Error`; `ErrorMessage` set on event.
- **Tool not found**: missing tool produces error result, not panic.
- **Multi-step tool loop**: model → tool → model → tool → model (5 events, 3 model + 2 tool).
- **Empty response skip**: response with nil content and no error code is skipped, next model call proceeds.

## notes

- The fake model and tool errors are designed to surface failures as event fields (not Go errors), matching the ADK Go
  pattern where errors propagate through the event stream.
- The tool execution uses `sync.WaitGroup` for true parallelism; results are collected in a pre-allocated slice indexed
  by call position, ensuring deterministic ordering regardless of goroutine completion order.
- All existing Node 1 contracts (`agent`, `event`, `session`, `context`) remain unchanged.
- Empty model responses (no content, no error) are handled with a retry loop inside `runOneStep`, allowing the fake model
  queue to advance to the next meaningful response.
