# Node 2: Implement ToolContext, Confirmation, Streaming, Long-Running Semantics

## implemented

### 1. ToolContext (`tool/context.go`)
Compact ToolContext model that gives tools access to:
- **Invocation/session identity** via `InvocationContext()` returning the full `context.InvocationContext`
- **State mutation helpers** via `Actions()` returning `*event.EventActions`
- **Confirmation status** via `ToolConfirmation()` returning `*event.ToolConfirmation`
- **Confirmation requests** via `RequestConfirmation(hint, payload)` that populates `EventActions.RequestedToolConfirmations` and sets `SkipSummarization`

### 2. Confirmation support (`tool/tool.go` — `WithConfirmation`, `confirmationTool`)
- **Sentinel errors**: `ErrConfirmationRequired` and `ErrConfirmationRejected`
- **`ConfirmationProvider`** type: `func(toolName string, toolInput map[string]any) bool` for dynamic confirmation logic
- **`WithConfirmation(funcTool, requireConfirmation, provider)`** wrapper that:
  - First call (no prior confirmation): produces structured `confirmation_required` result with hint
  - Call after `SetConfirmed(true)`: executes the inner tool normally
  - Call after `SetConfirmed(false)`: produces structured `confirmation_rejected` result with error
- Static `requireConfirmation` flag and dynamic `ConfirmationProvider` both supported

### 3. Streaming support (`tool/tool.go` — `StreamingFunctionTool`, `StreamChunk`; `tool/streaming_tool.go`)
- **`StreamingFunctionTool`** interface with `RunStream(args) ([]StreamChunk, error)`
- **`StreamChunk`** struct with `Text`, `Error`, `Final` fields
- **`CollectStreamChunks(chunks)`** collects all chunks into a normal `map[string]any{"result": text}` response (non-live mode)
- **`ExecuteStream(callID, name, args, t)`** runs a streaming tool and returns a `CallResult`
- Errors during streaming become structured tool errors in the result map

### 4. Long-running support (`tool/tool.go`)
- **`IsLongRunning() bool`** added to the `Tool` interface — all tools now declare whether they are long-running
- **`NewLongRunningFunctionTool(name, desc, decl, run)`** creates a tool whose `Declaration().Description` is automatically annotated with: _"NOTE: This is a long-running operation. Do not call this tool again if it has already returned some intermediate or pending status."_
- Function call results carry `job_id` + `status: pending` metadata demonstrating "do not repeat" semantics

### 5. Context-aware execution (`tool/tool.go` — `ContextFunctionTool`, `ContextExecute`)
- **`ContextFunctionTool`** interface extends `FunctionTool` with `RunWithContext(ctx ToolContext, args) (map[string]any, error)`
- **`ContextExecute(ctx, callID, name, args, t)`** prefers `ContextFunctionTool.RunWithContext` over `FunctionTool.Run`, and handles `ErrConfirmationRequired`/`ErrConfirmationRejected` errors with structured results

### 6. EventActions extensions (`event/event.go`)
- Added `ToolConfirmation` struct to the event package
- Added `SkipSummarization bool` and `RequestedToolConfirmations map[string]ToolConfirmation` to `EventActions`
- Added `ConfirmationFunctionCallName = "adk_request_confirmation"` constant

## files_changed

| File | Change |
|------|--------|
| `event/event.go` | Added `ToolConfirmation` struct, `SkipSummarization`, `RequestedToolConfirmations` fields to `EventActions`, `ConfirmationFunctionCallName` constant |
| `tool/context.go` | **New** — `ToolContext` interface + `toolContextImpl` with `NewToolContext()` constructor |
| `tool/tool.go` | Added `IsLongRunning()` to `Tool`, sentinel errors, `ConfirmationProvider`, `ContextFunctionTool`, `StreamingFunctionTool`, `StreamChunk`, `CollectStreamChunks`, `WithConfirmation`, `confirmationTool`, `ContextExecute`, `NewLongRunningFunctionTool` |
| `tool/streaming_tool.go` | **New** — `StreamFuncTool` implementation, `NewStreamingFunctionTool`, `NewStreamingFunctionToolWithDeclaration`, `ExecuteStream` |
| `tool/tool_confirmation_streaming_long_running_test.go` | **New** — 22 focused tests |

No changes were made to `flow/flow.go` — all new functionality builds on the existing `FunctionTool` and `Tool` interfaces, preserving backward compatibility.

## tests

All **22 new tests** pass, plus all **36 existing tests** remain green (58 total in tool package):

**ToolContext tests (4):**
- `TestToolContextConstruction` — context creation and field access
- `TestToolContextWithConfirmation` — context with confirmation handle
- `TestToolContextRequestConfirmation` — confirmation request populates actions
- `TestToolContextRequestConfirmationEmptyCallID` — error on empty call ID

**Confirmation tests (5):**
- `TestConfirmationRequired` — static confirmation required produces structured result + `ErrConfirmationRequired`
- `TestConfirmationApproved` — `SetConfirmed(true)` executes the tool normally
- `TestConfirmationRejected` — `SetConfirmed(false)` produces rejected result + `ErrConfirmationRejected`
- `TestConfirmationNotRequired` — no confirmation flag allows normal execution
- `TestConfirmationWithDynamicProvider` — provider-based confirmation (low risk passes, high risk requires)

**Streaming tests (6):**
- `TestStreamingCollection` — chunks concatenated into `{"result": "Hello World"}`
- `TestStreamingError` — error in chunk produces structured error in result
- `TestStreamingErrorViaRunStream` — `RunStream` returning error is captured
- `TestStreamingEmptyChunks` — empty stream returns empty result
- `TestStreamingToolNotFound` — nil tool produces error
- `TestCollectStreamChunks` — helper concatenates text, propagates errors

**Long-running tests (2):**
- `TestLongRunningToolDeclaration` — `IsLongRunning()=true`, declaration annotated with "NOTE" and "Do not call this tool again"
- `TestLongRunningToolResultMetadata` — result carries `job_id` + `status: pending`
- `TestNormalToolIsNotLongRunning` — regular tool returns `IsLongRunning()=false`

**ContextExecute tests (3):**
- `TestContextExecuteNormalTool` — `ContextExecute` works with normal `FunctionTool`
- `TestContextExecuteWithConfirmation` — context-aware execution handles `ErrConfirmationRequired`
- `TestContextExecuteConfirmationApproved` — context-aware execution with approved confirmation

## notes

1. The implementation builds on Node 1's public surface without rewriting the tool package. New interfaces (`ContextFunctionTool`, `StreamingFunctionTool`) extend rather than replace `FunctionTool`.
2. The `confirmationTool` uses an explicit `SetConfirmed(bool)` method for confirmation approval/rejection, which mirrors the model where the flow sets confirmation state before calling `Run`.
3. Streaming in non-live mode collects all chunks into a `{"result": concatenated_string}` map, matching the ADK Go reference behavior.
4. The `ContextExecute` function provides a bridge for flows that want to pass `ToolContext` to tools while maintaining backward compatibility with plain `FunctionTool`.
5. All 58 tests pass with `go test ./...` across the entire repository.
