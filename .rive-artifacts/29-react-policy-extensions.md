## implemented

ReAct policy extensions implemented across three new packages and one modified
file, following the Chapter 07 deep-dive design:

1. **Exit loop** (`tool/exitloop/`):
   - `ExitLoopTool` implements `ContextFunctionTool`, setting
     `EventActions.EndInvocation = true` via `RunWithContext`.
   - `Flow.Run` detects `EndInvocation` on any step event, calls
     `ctx.EndInvocation()`, and stops the loop immediately — no further
     model responses are consumed from the queue.

2. **Retry/reflection** (`plugin/retryreflect/`):
   - `RetryAndReflect` plugin hooks `OnToolErrorCallback` to intercept tool
     failures and produce reflection guidance content in the result.
   - Per-tool-name failure counter with configurable `MaxRetries`.
   - Counter is reset on successful tool execution (when the result has no
     `"error"` key).
   - The original error message is preserved in `Result["error"]` and
     `Result["reflection"]` / `Result["reflection_exceeded"]` is added for
     the model to see on the next iteration.

3. **Tool/request shaping** (`plugin/functionmodifier/`):
   - `FunctionCallModifier` plugin uses `BeforeModelCallback` to inject
     hidden args into tool declarations for matching tools (via `Predicate`).
   - `AfterModelCallback` strips those args from the model's function calls
     before they reach the actual tool execution, storing them in session
     state under `hidden/{callID}/{argName}`.
   - Demonstrates LLM-facing cognitive space protection — internal
     parameters like `user_id` never appear in the model's visible schema
     or call arguments.

4. **Flow integration** (`flow/flow.go`):
   - `Run` loop now scans all step events for `EndInvocation` and exits
     immediately when detected.

## files_changed

- `tool/exitloop/exitloop.go` — ExitLoopTool (new package)
- `tool/exitloop/exitloop_test.go` — unit tests (new)
- `plugin/retryreflect/plugin.go` — RetryAndReflect plugin (new package)
- `plugin/retryreflect/plugin_test.go` — unit tests (new)
- `plugin/functionmodifier/plugin.go` — FunctionCallModifier plugin (new package)
- `plugin/functionmodifier/plugin_test.go` — unit tests (new)
- `flow/flow.go` — EndInvocation detection added to `Flow.Run`
- `flow/react_policy_test.go` — integration tests (new)

## tests

All test groups pass (`go test ./...`, `go vet ./...`, `git diff --check`):

| Test group | Location | Criteria |
|---|---|---|
| ExitLoop name/declaration | `tool/exitloop/exitloop_test.go` | Tool name, description, Declaration |
| ExitLoop Run/RunWithContext | `tool/exitloop/exitloop_test.go` | Result contains `ended:true`, actions set `EndInvocation` |
| ExitLoop stops multi-step | `flow/react_policy_test.go` | Exit in first step stops immediately |
| ExitLoop after tool call | `flow/react_policy_test.go` | Multi-step chain terminates mid-sequence |
| ExitLoop skips remaining queue | `flow/react_policy_test.go` | Model responses after exit not consumed |
| RetryReflect plugin name/cfg | `plugin/retryreflect/plugin_test.go` | Name, MaxRetries, default name |
| RetryReflect adds reflection | `plugin/retryreflect/plugin_test.go` | `result["reflection"]` present |
| RetryReflect exceeds max | `plugin/retryreflect/plugin_test.go` | `result["reflection_exceeded"]` after retries |
| RetryReflect resets on success | `plugin/retryreflect/plugin_test.go` | Counter cleared after successful result |
| RetryReflect preserves error | `plugin/retryreflect/plugin_test.go` | Error stays when result has `"error"` key |
| Flow retry reflect adds reflection | `flow/react_policy_test.go` | `Result["reflection"]` present in tool event |
| Flow retry reflect resolves | `flow/react_policy_test.go` | Failed then successful tool resets counter |
| Flow retry reflect preserves original error | `flow/react_policy_test.go` | `Result["error"]` contains original message |
| FunctionModifier name/cfg | `plugin/functionmodifier/plugin_test.go` | Name, HiddenArgs accessor |
| FunctionModifier injection | `plugin/functionmodifier/plugin_test.go` | Hidden args in matching tool's InputSchema |
| FunctionModifier stripping | `plugin/functionmodifier/plugin_test.go` | Hidden args removed from tool call Args |
| FunctionModifier predicate | `plugin/functionmodifier/plugin_test.go` | Non-matching tools not modified |
| Flow hidden arg injection | `flow/react_policy_test.go` | Tool receives stripped args, no hidden params |
| Flow hidden arg predicate | `flow/react_policy_test.go` | Non-matching tool unaffected |
| Flow transfer still passes | `flow/react_policy_test.go` | Agent transfer works with both plugins active |

## notes

- The RetryAndReflect plugin returns `(result, nil)` from `OnToolError` so the
  plugin manager treats it as a recovery. The original error is preserved in
  `result["error"]` rather than the return value, satisfying "do not hide the
  original tool error from events".
- The FunctionCallModifier stores stripped args in session state under
  `hidden/{callID}/{key}` so they are accessible to policies that need them
  (e.g. injecting `user_id` into the downstream request) while staying
  invisible to the model.
- The exit loop detection in `Flow.Run` is checked before
  `IsFinalResponse`/`TransferToAgent`, so it takes priority as the loop
  stop signal.
- Transfer behaviour from Node 1 is verified to still pass when both plugins
  (retry/reflect and function modifier) are registered simultaneously.
- No external dependencies added beyond the standard library and existing
  `rive-adk-go` packages.
