# Tool System Integration Report

## implemented

1. **Flow integration with Toolsets**: Added `Toolsets []tool.Toolset` field to `flow.Flow`. The Flow auto-resolves tools from toolsets (cached once per invocation) and injects `Declaration` entries into `model.LLMRequest.ToolDeclarations` before each model call. Tool execution in `handleFunctionCalls` looks up tools from both `Flow.Tools` and resolved toolsets.

2. **ConfirmationControl interface**: Exported `tool.ConfirmationControl` interface (`SetConfirmed(bool)`) on the previously unexported `confirmationTool`. This allows callers to programmatically approve or reject confirmation-gated tool calls.

3. **Model helper**: Added `model.FunctionResponseResponse` for constructing model responses containing function responses (useful for confirmation reply simulation).

4. **Demo — filtered tools**: `demoFilteredTools()` demonstrates `FilterToolset` + `AllowedToolsPredicate`. A flow with two tools (`get_weather`, `delete_data`) and a `FilterToolset` only exposes `get_weather` declarations to the model.

5. **Demo — confirmed tool call**: `demoConfirmedToolCall()` shows a `WithConfirmation` tool requiring approval. The first call returns `{"confirmation_required": true}`; after `SetConfirmed(true)`, the second call executes the handler.

6. **Demo — rejected confirmation**: `demoRejectedConfirmation()` shows the rejection path — `SetConfirmed(false)` followed by `Run` returns `{"confirmation_rejected": true}` and `ErrConfirmationRejected`.

7. **Demo — streaming tool non-live**: `demoStreamingToolNonLive()` creates a `StreamingFunctionTool`, calls `ExecuteStream`, and displays collected chunks as a single concatenated result.

8. **Demo — long-running metadata**: `demoLongRunningTool()` creates a `NewLongRunningFunctionTool`, verifies `IsLongRunning() == true`, checks the "Do not call again" annotation in the declaration, and shows `{"job_id": "...", "status": "pending"}` result.

9. **README.md Chapter 03 section**: Documents the tool system problem, why it's hard, what this replica implements, and intentional omissions.

## files_changed

| File | Change |
|------|--------|
| `flow/flow.go` | Added `Toolsets` field, `injectToolDeclarations()`, `resolveToolsets()`, `lookupTool()` |
| `flow/flow_test.go` | Added 6 tests: toolset resolution, declaration injection, filtered toolset, streaming, long-running, resolution cache |
| `model/model.go` | Added `FunctionResponseResponse()` helper |
| `tool/tool.go` | Exported `ConfirmationControl` interface |
| `runner/runner_test.go` | Added 6 integration tests: confirmation confirmed, streaming non-live, long-running, filtered toolset, confirmation rejection, toolset declarations |
| `cmd/demo/main.go` | Added `runChapter03()` with 5 demos: filtering, confirmed, rejected, streaming, long-running |
| `README.md` | Added Chapter 03 section; updated Quick Start and Demo sections |

## tests

- `flow/flow_test.go`: `TestFlowToolsetResolution`, `TestFlowToolsetDeclarationInjection`, `TestFlowFilteredToolset`, `TestFlowStreamingToolNonLiveMode`, `TestFlowLongRunningTool`, `TestFlowToolsetResolutionCache`
- `runner/runner_test.go`: `TestRunnerFullChainConfirmationConfirmed`, `TestRunnerFullChainStreamingTool`, `TestRunnerFullChainLongRunningTool`, `TestRunnerFullChainFilteredToolset`, `TestRunnerConfirmationRejectionChain`, `TestRunnerFullChainToolsetDeclarations`
- `tool/tool_confirmation_streaming_long_running_test.go` (existing, unchanged): 18 tests for confirmation, streaming, long-running at the tool level

`go test ./...` — all packages pass.
`go vet ./...` — clean.

## notes

- Toolset resolution is cached (once per `Flow.Run` invocation) to avoid re-querying dynamic toolsets on every step. This matches the upstream `toolProcessor` pattern (`f.Tools != nil` skip).
- Confirmation flow in this replica differs from upstream: the tool returns `ErrConfirmationRequired` as a result error, and the flow loop continues to the next model response (which produces final text). Upstream ADK Go has a dedicated `generateRequestConfirmationEvent` → `RequestConfirmationRequestProcessor` pump. The core HITL contract (`WithConfirmation` + `SetConfirmed`) is preserved.
- Streaming in non-live mode collects all `StreamChunk` entries into a single `map[string]any{"result": concatenated}`. Live streaming (goroutine + yield per chunk) is intentionally omitted.
- The `demo` binary was run and produces the expected output for all 5 Chapter 03 demos.
