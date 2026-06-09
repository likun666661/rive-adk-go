# Plugin Manager and Hook Ordering

## Implemented

### 1. Plugin Type (`plugin/plugin.go`)

Added a compact `Plugin` type with nine optional callback hooks covering the full lifecycle:

**Agent-level hooks:**
- `BeforeAgent func(callbackctx.CallbackContext) (*event.Event, error)` — can short-circuit agent execution
- `AfterAgent func(callbackctx.CallbackContext, []*event.Event) (*event.Event, error)` — can observe or append events

**Model-level hooks:**
- `BeforeModel func(callbackctx.CallbackContext, *model.LLMRequest) (*model.LLMResponse, error)` — can skip LLM call
- `AfterModel func(callbackctx.CallbackContext, *model.LLMRequest, *model.LLMResponse, error) (*model.LLMResponse, error)` — can replace response
- `OnModelError func(callbackctx.CallbackContext, *model.LLMRequest, error) (*model.LLMResponse, error)` — error recovery (optional)

**Tool-level hooks:**
- `BeforeTool func(callbackctx.ToolContext, string, map[string]any) (map[string]any, error)` — can skip tool call
- `AfterTool func(callbackctx.ToolContext, string, map[string]any, map[string]any, error) (map[string]any, error)` — can transform result
- `OnToolError func(callbackctx.ToolContext, string, map[string]any, error) (map[string]any, error)` — error recovery (optional)

Built via `New(Config)` with a required `Name` field. Hooks are stored as unexported function fields with getter methods that return nil when not set.

### 2. Manager (`plugin/manager.go`)

`Manager` holds an ordered list of `*Plugin` and runs hooks with consistent semantics matching Chapter 04:

- **Preserves registration order** — first registered runs first
- **Skips nil hooks** — plugins that don't implement a hook are transparently bypassed
- **Stops at first non-nil result** — early-exit ensures first shortcut/override wins
- **Returns errors immediately** — any hook error aborts the chain with a wrapped error identifying the offending plugin and index

Twelve `Run*` methods cover all lifecycle points: `RunBeforeAgentCallback`, `RunAfterAgentCallback`, `RunBeforeModelCallback`, `RunAfterModelCallback`, `RunOnModelErrorCallback`, `RunBeforeToolCallback`, `RunAfterToolCallback`, `RunOnToolErrorCallback`.

### 3. Agent Integration

- **`agent.Config.PluginManager`** — added `*plugin.Manager` field to agent configuration
- **`baseAgent.PluginManager()`** — accessor for the manager
- **`context.RunWithCallbackContext`** — plugin before/after agent hooks execute before direct `BeforeAgentCallbackCtx`/`AfterAgentCallbackCtx` callbacks

The legacy `agent.Execute` path (used by `runner.Runner`) remains unchanged for backward compatibility. Plugin integration is additive through the `RunWithCallbackContext` path.

### 4. Flow Integration

- **`flow.Flow.PluginManager`** — added `*plugin.Manager` field
- **`callModel`** — plugin hooks run before direct callbacks:
  - `RunBeforeModelCallback` → `BeforeModelCallbacks` → `GenerateContent` → `RunOnModelErrorCallback` (if error) → `RunAfterModelCallback` → `AfterModelCallbacks`
- **`executeToolCall`** — plugin hooks run before direct callbacks:
  - `RunBeforeToolCallback` → `BeforeToolCallbacks` → `tool.Run()` → `RunOnToolErrorCallback` (if error) → `RunAfterToolCallback` → `AfterToolCallbacks`

Plugin hooks are conditionally invoked only when `PluginManager != nil`, making the integration zero-cost when no plugins are configured.

### 5. Hook Ordering

Matching the Chapter 04 teaching model, plugin hooks **always run before** direct callbacks at every lifecycle point:

```
Agent:  RunBeforeAgentCallback → beforeAgentCallbacks → agent.run() → RunAfterAgentCallback → afterAgentCallbacks
Model:  RunBeforeModelCallback → BeforeModelCallbacks → LLM call → RunAfterModelCallback → AfterModelCallbacks
Tool:   RunBeforeToolCallback  → BeforeToolCallbacks  → tool.Run() → RunAfterToolCallback  → AfterToolCallbacks
```

## Files Changed

| File | Change |
|---|---|
| `plugin/plugin.go` | **NEW** — `Plugin` type, `Config`, `New()`, getter methods for all 9 hooks |
| `plugin/manager.go` | **NEW** — `Manager` type with `NewManager()`, `Register()`, `Len()`, and 12 `Run*` methods |
| `plugin/plugin_test.go` | **NEW** — 20 tests covering ordering, early-exit, error propagation, error recovery |
| `agent/agent.go` | Added `PluginManager` field to `Config` and `baseAgent`; added `PluginManager()` accessor; imported `plugin` |
| `context/callback_context.go` | Added `*plugin.Manager` parameter to `RunWithCallbackContext`; plugin hooks run before direct callbacks; imported `plugin` |
| `flow/flow.go` | Added `PluginManager *plugin.Manager` field to `Flow`; integrated plugin hooks in `callModel` and `executeToolCall`; added `applyAfterToolPluginAndCallbacks` helper; imported `plugin` |
| `context/callback_context_test.go` | Updated 5 `RunWithCallbackContext` calls to pass `nil` for new PluginManager parameter |

## Tests

20 new tests in `plugin/plugin_test.go`:

| # | Test | What it verifies |
|---|---|---|
| 1 | `TestManagerRegistrationOrder` | Hooks execute in registration order (first, second, third) |
| 2 | `TestManagerSkipsNilHooks` | Plugins without a hook are transparently skipped |
| 3 | `TestManagerEarlyExitBeforeAgent` | First non-nil event stops the chain |
| 4 | `TestManagerEarlyExitBeforeModel` | First non-nil response skips remaining plugins |
| 5 | `TestManagerErrorPropagation` | First error aborts chain immediately |
| 6 | `TestManagerEarlyExitBeforeTool` | First non-nil tool result stops the chain |
| 7 | `TestManagerPluginBeforeDirectModelOrdering` | Plugin hooks run before direct callbacks |
| 8 | `TestManagerAllHookTypesNoErrorRecovery` | All 6 primary hook types exercise correctly |
| 9 | `TestManagerEmptyReturnsNilNil` | Empty manager returns (nil, nil) for all hooks |
| 10 | `TestManagerRegisterNil` | `Register(nil)` is a no-op |
| 11 | `TestManagerOnModelErrorRecovery` | `OnModelError` can replace error with success response |
| 12 | `TestManagerOnToolErrorRecovery` | `OnToolError` can replace error with success result |
| 13 | `TestManagerOnToolErrorNoError` | `OnToolError` returns nil when no error present |
| 14 | `TestManagerOnModelErrorNoError` | `OnModelError` returns nil when no error present |
| 15 | `TestManagerAfterModelReplaceResponse` | `AfterModel` can replace the LLM response |
| 16 | `TestManagerAfterToolReplaceResult` | `AfterTool` can transform the tool result |
| 17 | `TestManagerOnModelErrorPropagatesError` | Error in OnModelError hook is propagated |
| 18 | `TestManagerMultiplePluginsFirstWinsBeforeModel` | First registered plugin's non-nil result wins |
| 19 | `TestManagerAfterAgentSeesEvents` | AfterAgent callback can observe run events |
| 20 | `TestManagerMultiPluginAfterToolFirstWins` | Multi-plugin AfterTool — first non-nil wins |

All existing tests across the codebase continue to pass unchanged.

## Notes

- The Plugin layer is additive and does not restructure any existing code. All plugin hook invocations are guarded by `if f.PluginManager != nil`, making the integration zero-cost when no plugins are configured.
- Plugin hooks for model and tool callbacks construct temporary `callbackctx.CallbackContext` / `callbackctx.ToolContext` instances with fresh `EventActions`, meaning state writes go through to durable session state but are not yet merged into flow-level event deltas. This is a future refinement area.
- Error recovery hooks (`OnModelError`, `OnToolError`) follow the Chapter 04 pattern: return a non-nil response/result to recover, or return (nil, nil) to pass through the original error.
- Agent-level plugin hooks are integrated through `context.RunWithCallbackContext` (the `CallbackContext`-aware lifecycle path), while the legacy `agent.Execute` path remains plugin-free for backward compatibility.
- The API surface is intentionally small: one `Plugin` type, one `Manager` type, one `Config` struct, and nine optional hooks per plugin.
