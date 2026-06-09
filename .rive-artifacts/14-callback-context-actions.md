# Callback Context and Event Actions Extension

## Implemented

### 1. Callback Context Surface (3-layer permission model)

Added three context interfaces inspired by the ADK Go callback design:

- **`callbackctx.ReadonlyContext`** — immutable identity surface exposing `AgentName()`, `InvocationID()`, `SessionID()`, `UserID()`, `AppName()`, `Branch()`, `UserContent()`, and `ReadonlyState()`. Callbacks cannot call `EndInvocation()` or access `RunConfig()` through this interface.

- **`callbackctx.CallbackContext`** — embeds `ReadonlyContext` and adds writable `State()` (write-through to delta), `ArtifactService()`, and `MemoryService()`.

- **`callbackctx.ToolContext`** — embeds `CallbackContext` and adds `FunctionCallID()`, `Actions()` (returns `*EventActions`), and `SearchMemory()`.

The interfaces are defined in a standalone `callbackctx` package to avoid import cycles between `agent` and `context`.

### 2. State Write-Through with StateDelta Recording

Implemented `callbackContextState` (`context/callback_context.go:69-94`) — a `session.State` decorator:

- **`Get(key)`**: checks `actions.StateDelta` first (intra-step visibility), falls back to durable session state.
- **`Set(key, val)`**: writes to `actions.StateDelta` AND durable session state immediately (write-through strategy).
- **`Delete(key)`**: writes `TombstoneValue` to delta AND removes from durable state.
- **`All()`**: merges delta over durable state.

This means callbacks in the same step can see each other's writes via the delta, and the writes are durable even if a later callback errors out.

### 3. Artifact Save Tracking

Implemented `trackedArtifacts` (`context/callback_context.go:99-140`) — an `artifact.Service` decorator that automatically records each successful `Save()` result:

- `Save()` returns the version from the real service and records `fileName → version` in `actions.ArtifactDelta`.
- All other methods (`Load`, `Delete`, `List`, `Versions`, `GetArtifactVersion`) delegate transparently.
- Use `NewCallbackContextWithArtifactTracking()` to enable tracking.
- Use plain `NewCallbackContext()` for untracked artifact access.

### 4. EventActions.ArtifactDelta

Added `ArtifactDelta map[string]int64` field to `event.EventActions` (`event/event.go:74`) that records filename→version mappings from callback artifact saves.

### 5. New Callback Types

Added context-aware callback types alongside existing legacy callbacks:

**`agent` package:**
- `BeforeAgentCallbackCtx func(callbackctx.CallbackContext) (*event.Event, error)`
- `AfterAgentCallbackCtx func(callbackctx.CallbackContext, events []*event.Event) (*event.Event, error)`

**`flow` package:**
- `BeforeModelCallbackCtx func(callbackctx.CallbackContext, *LLMRequest) (*LLMResponse, error)`
- `AfterModelCallbackCtx func(callbackctx.CallbackContext, *LLMRequest, *LLMResponse, error) (*LLMResponse, error)`
- `BeforeToolCallbackCtx func(callbackctx.ToolContext, string, map[string]any) (map[string]any, error)`
- `AfterToolCallbackCtx func(callbackctx.ToolContext, string, args, result map[string]any, error) (map[string]any, error)`

### 6. RunWithCallbackContext

Added `context.RunWithCallbackContext()` — drives the agent lifecycle with `CallbackContext`-aware before/after callbacks, supporting:
- Early exit (before callback returns non-nil event → skip agent run)
- State delta persistence after early exit (write-through)
- Error propagation from callbacks
- `EndInvocation` handling in after-callbacks

## Files Changed

| File | Change |
|---|---|
| `callbackctx/callbackctx.go` | **NEW** — `ReadonlyContext`, `CallbackContext`, `ToolContext` interfaces |
| `context/callback_context.go` | **NEW** — `callbackContext`, `callbackContextState`, `trackedArtifacts`, `RunWithCallbackContext`, constructors |
| `context/callback_context_test.go` | **NEW** — 20 tests covering all callback context features |
| `context/context.go` | Added type aliases for `callbackctx` interfaces; added `AgentName()`, `UserID()`, `AppName()`, `SessionID()`, `ReadonlyState()` to `InvocationContext` and concrete impl |
| `event/event.go` | Added `ArtifactDelta map[string]int64` to `EventActions` |
| `agent/agent.go` | Added `BeforeAgentCallbackCtx`, `AfterAgentCallbackCtx` types; updated doc |
| `session/session.go` | Added `NewReadonlyState()` factory and `readonlyStateWrapper` |
| `flow/flow.go` | Added `BeforeModelCallbackCtx`, `AfterModelCallbackCtx`, `BeforeToolCallbackCtx`, `AfterToolCallbackCtx` types and corresponding Flow fields |

## Tests

20 new tests in `context/callback_context_test.go`:

| # | Test | What it verifies |
|---|---|---|
| 1 | `TestReadonlyContextIdentityFields` | All identity fields exposed correctly |
| 2 | `TestReadonlyStateIsReadOnly` | `ReadonlyState()` is truly read-only |
| 3 | `TestCallbackStateWriteThrough` | `Set()` writes to both delta and durable state |
| 4 | `TestCallbackStateGetPriorityDeltaFirst` | `Get()` checks delta before durable state |
| 5 | `TestCallbackStateIntraStepDeltaVisible` | Callback sees its own write immediately |
| 6 | `TestCallbackStateDeltaAcrossCallbacks` | Second callback sees first's delta write |
| 7 | `TestCallbackStateDelete` | `Delete()` writes tombstone and removes durable key |
| 8 | `TestArtifactSaveTracking` | Single save records version in `ArtifactDelta` |
| 9 | `TestArtifactSaveTrackingMultiple` | Multiple saves track per-file versions |
| 10 | `TestArtifactNoTrackingWithoutWrapper` | Untracked context does not pollute delta |
| 11 | `TestToolContextFields` | `FunctionCallID()` and `Actions()` work |
| 12 | `TestToolContextSearchMemory` | `SearchMemory()` returns matching entries |
| 13 | `TestRunWithCallbackContextBeforeEarlyExit` | Before callback early exit skips agent run |
| 14 | `TestRunWithCallbackContextAfterCallback` | After callback runs after agent run |
| 15 | `TestStateDeltaPersistsAfterEarlyExit` | State persists despite early exit (write-through) |
| 16 | `TestRunWithCallbackContextBeforeError` | Error in before callback aborts |
| 17 | `TestRunWithCallbackContextAfterError` | Error in after callback aborts |
| 18 | `TestLegacyCallbacksStillWork` | Chapter 01-03 callbacks unchanged |
| 19 | `TestPrepareEventActionsInitializesNilMaps` | Nil map initialization |
| 20 | `TestToolContextInheritsIdentity` | ToolContext inherits ReadonlyContext fields |

Existing Chapter 01-03 tests continue to pass unchanged.

## Notes

- The `callbackctx` package was introduced as a standalone package to break the import cycle between `agent` and `context`. The types are re-exported as type aliases in `context` (`context.ReadonlyContext`, `context.CallbackContext`, `context.ToolContext`).
- `RunWithCallbackContext` lives in `context` package (not `agent`) because it bridges both `context.InvocationContext` and `callbackctx.CallbackContext` and imports both `agent` and `callbackctx`.
- The write-through strategy mirrors the ADK Go implementation: `State().Set()` simultaneously writes to `StateDelta` and durable state, making callback writes immediately durable even if a subsequent callback errors.
- The delta-prioritized read (`Get()` checks delta first) ensures intra-step visibility across callbacks sharing the same `EventActions`.
