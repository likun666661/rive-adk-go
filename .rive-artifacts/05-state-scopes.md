# Node 1: Extend Session State Scope Semantics

## implemented

Extended the Chapter 01 runtime replica with session state lifecycle ideas from
Chapter 02. The implementation covers four state scopes (`session:`, `app:`,
`user:`, `temp:`), partial-event guards, deletion/tombstone semantics, and a
`Service` layer that manages multi-session state sharing.

### Scope prefixes (`session/session.go:27–36`)

| Constant | Prefix | Semantics |
|---|---|---|
| `KeyPrefixApp` | `app:` | Shared by all users and sessions within an app. |
| `KeyPrefixUser` | `user:` | Shared across all sessions for the same user within an app. |
| `KeyPrefixTemp` | `temp:` | Visible during the current invocation; trimmed from persisted events. |
| (no prefix) | — | Scoped to the individual session. |

### Utility functions (`session/session.go`)

- **`ExtractStateDeltas(delta) → (appDelta, userDelta, sessionDelta)`** — Splits
  a delta map by prefix. `app:` keys go to appDelta (prefix stripped), `user:`
  keys go to userDelta, `temp:` keys are silently dropped, and un-prefixed keys
  go to sessionDelta.

- **`MergeStates(app, user, session) → merged`** — Overlays three state layers:
  session (highest priority) → user → app (lowest). App keys carry `app:`
  prefix in output, user keys carry `user:` prefix. Tombstone values in session
  state (`TombstoneValue = "__STATE_TOMBSTONE__"`) hide the corresponding
  `app:` / `user:` keys from lower layers.

- **`trimTempDeltaState(delta) → delta`** — Strips `temp:` keys from a delta
  map. Used on the event's `StateDelta` before persisting the event.

- **`TombstoneValue`** — Sentinel `"__STATE_TOMBSTONE__"` that marks a key as
  deleted. In `MergeStates`, a tombstoned session key masks the same key from
  the app and user layers.

### Service (`session/session.go:324–525`)

A `Service` struct manages multiple sessions with cross-cutting state:

- **`appState`**: `map[appName]map[string]any` — app-scoped key/value store.
- **`userState`**: `map[appName]map[userID]map[string]any` — user-scoped store.
- **`sessions`**: `map[key]*inMemorySession` — session lookup.

Key methods:
- `Create(appName, userID, sessionID, initialState)` — Creates a session and
  routes initial scoped state.
- `Get(appName, userID, sessionID)` — Returns session; `State()` reflects local
  session state only.
- `GetOrCreate(...)` — Convenience combination.
- `AppendEvent(sess, ev)` — Full scope pipeline:
  1. Partial events silently dropped (no state mutation, no persistence).
  2. Full StateDelta applied to session state (including `temp:`) for invocation visibility.
  3. `temp:` keys stripped from the event's `StateDelta` before appending.
  4. `app:` / `user:` keys routed to shared app/user stores.
- `GetMergedState(app, user, sid)` — Returns the fully merged
  app+user+session view with overlay order, tombstone masking, and scope-prefix
  filtering.
- `DeleteSession(...)` — Removes a session.

### Partial event guard

Partial events (`event.Partial == true`) are never persisted and cannot mutate
durable state:

- Standalone session (`inMemorySession.AppendEvent`): rejects with an error.
- Service-backed (`Service.AppendEvent`): silently drops (`return nil`).
- Both paths: session state is unchanged, event count is unchanged, and
  app/user store state is unchanged.

### Temp state lifecyle

1. During invocation: `temp:` keys are written into session state, visible via
   `State().Get()` or `State().All()`.
2. Before persistence: `trimTempDeltaState` strips `temp:` keys from the
   event's `StateDelta`, so the persisted event carries no temp data.
3. In the merged view (`GetMergedState`): temp keys are excluded because
   `mergeStatesForSession` filters them out.

### Session-level `AppendEvent` integration

`inMemorySession.AppendEvent` delegates to `Service.AppendEvent` when the
session has a service back-reference. This ensures proper lock ordering:
the service-level lock (`svc.mu`) is held during scope routing to shared state,
avoiding races between concurrent sessions.

## files_changed

| File | Change |
|---|---|
| `session/session.go` | Replaced TODO-level state with full scope semantics, utility functions, `Service` struct, tombstone support, and scoped `AppendEvent`. |
| `session/session_test.go` | Added 22 test functions covering all scope behaviors. |
| `runner/runner.go` | `InMemorySessionService` now delegates to `session.Service` for scope-aware state routing. Removed unused `sync` import. |

## tests

### Existing tests (all pass)

7 existing tests continue to pass unchanged.

### New session tests (22 functions, all pass)

| Test | Coverage |
|---|---|
| `TestExtractStateDeltas` | App/user/session/temp prefix routing |
| `TestExtractStateDeltasNil` / `Empty` | Nil and empty delta edge cases |
| `TestMergeStatesBasicOverlay` | Three-layer overlay with prefix addition |
| `TestMergeStatesTombstoneHidesLowerLayers` | Tombstone masks app: and user: keys |
| `TestMergeStatesEmptyMaps` | Nil inputs produce empty map |
| `TestMergeStatesSessionPriority` | Session keys take priority |
| `TestTrimTempDeltaState` | Temp keys stripped, others retained |
| `TestTrimTempDeltaStateNoTemp` | Pass-through when no temp keys |
| `TestTrimTempDeltaStateEmpty` | Empty delta unchanged |
| `TestServiceAppStateShared` | app: state visible across users |
| `TestServiceUserStateScoped` | user: state isolated per user |
| `TestServiceUserStateSharedAcrossSessions` | user: state shared across same user's sessions |
| `TestServiceSessionStateLocal` | Session keys not leaked across sessions |
| `TestServiceTempStateVisibleDuringInvocation` | temp: visible in session state, but NOT in persisted event delta |
| `TestServiceTempStateNotInMergedView` | temp: keys excluded from GetMergedState |
| `TestServicePartialEventDoesNotPersist` | Partial event not appended, state not mutated |
| `TestServicePartialEventCannotMutateState` | Partial event's app: delta does not overwrite existing |
| `TestServiceTombstoneDeleteHidesSharedState` | Session tombstone hides app: and user: keys |
| `TestServiceTombstoneDeleteSessionOnly` | Tombstone on session-only key works |
| `TestServiceCreateWithInitialState` | Initial scoped state routing on create |
| `TestServiceCreateDuplicateFails` | Duplicate session ID rejected |
| `TestServiceGetOrCreate` | Existing session returned, new created on miss |
| `TestServiceAppStateIsolation` | Different apps' app: state isolated |
| `TestServiceDeleteSession` | Deletion makes Get fail |
| `TestServiceStateMergeOverlayOrder` | Full app < user < session priority chain |
| `TestStandaloneSessionTempStateHandling` | Non-service session temp visibility + trim |
| `TestStandaloneSessionPartialEventStateGuard` | Non-service session partial state guard |

### Runner tests (all pass)

Existing runner tests (9 functions) continue to pass because `InMemorySessionService`
now uses `session.Service` with the same interface contract.

### Verification

```bash
$ go test ./...
ok  	github.com/likun666661/rive-adk-go/session	0.255s
ok  	github.com/likun666661/rive-adk-go/runner	0.499s
... (all 8 packages pass)

$ go vet ./...
(no output)

$ go test -race ./session/
ok  	github.com/likun666661/rive-adk-go/session	1.265s
```

## notes

- The implementation follows the state lifecycle from Chapter 02 of the deep-read
  guide: scope-based prefix routing, temp-state trim before persist, partial-event
  gating, and merge overlay order.
- `Service` provides the same three-layer model as the source ADK Go
  (`app:` → `user:` → session-scoped) but keeps the implementation compact (one
  file, no external dependencies beyond `maps`/`strings`/`sync`).
- `inMemorySession` carries an optional `svc *Service` back-reference. Sessions
  created via `NewInMemorySession` (standalone) have `svc == nil` and skip scope
  routing. Sessions created via `Service.Create` are service-backed and fully
  participate in app/user state sharing.
- The runner's `InMemorySessionService` now wraps `session.Service`, so all
  runner-created sessions have full scope routing. This is transparent to
  existing callers.
- Tombstone semantics are implemented at the `MergeStates` level: a session key
  set to `TombstoneValue` hides the same key in the app and user layers.
- The `EventActions` `StateDelta` from Chapter 01 was already present; this node
  extends the delta processing pipeline with scope awareness.
