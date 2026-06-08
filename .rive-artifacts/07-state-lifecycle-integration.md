# 07-state-lifecycle-integration.md

## implemented

1. **Exposed memory and artifact services through runner configuration and invocation context**:
   - Added `MemoryService` (memory.Service) and `ArtifactService` (artifact.Service) fields to `runner.Config` and `Runner` struct.
   - Both services are optional — runner works without them for backward compatibility.
   - Added `MemoryService()` and `ArtifactService()` accessors to `context.InvocationContext` interface.
   - Added `Memory` and `Artifact` fields to `context.Params` so the runner wires them into each invocation.
   - Added `GetMergedState` method to `runner.InMemorySessionService` for inspecting the full app+user+session state overlay from tests and demos.

2. **Extended cmd/demo with Chapter 02 state lifecycle scenarios**:
   - **Scenario 1**: Session sets `app:`, `user:`, and session-scoped state via tool `state_delta`. Merged state shows all layers.
   - **Scenario 2**: Same user, different session — sees shared `app:` and `user:` state but NOT session-local state. Temp state visible during invocation, absent from merged view.
   - **Scenario 3**: Artifact save (version auto-increment), load (latest and specific version), list.
   - **Scenario 4**: Add completed sessions to memory, search across sessions, verify user/app isolation.

3. **Updated README.md with Chapter 02 section**:
   - Problem each lifecycle solves (session, memory, artifact).
   - Table comparing scopes, lifetimes, data models, and query patterns.
   - Why merging into one store causes session bloat, memory pollution, and version coupling.
   - Simplified semantics in this implementation (keyword-only search, no cloud backends, in-memory only).
   - Intentional omissions table (database backend, GCS, Vertex AI, stale-session detection, etc.).

4. **Added 8 integration tests in runner_test.go**:
   - `TestRunnerConfigWithMemoryAndArtifact` — validates optional services.
   - `TestRunnerScopedStateMutation` — app/user/session/temp state through tool events via runner.
   - `TestRunnerStateMergeAcrossSessions` — app/user state shared, session state isolated across two sessions.
   - `TestRunnerTempStateLifecycle` — temp visible during invocation, trimmed on persist.
   - `TestRunnerArtifactSaveLoad` — artifact save/load alongside runner, version increment, user-scoped cross-session visibility.
   - `TestRunnerMemoryAddSearch` — add sessions to memory, search, user/app isolation.
   - `TestRunnerFullChain` — end-to-end: runner → state scoping → artifact save/load → memory add/search → second session verifying user state carries over.
   - `TestRunnerArtifactVersionIndependence` — artifact versions evolve independently from session event count.

## files_changed

| File | Change |
|------|--------|
| `context/context.go` | Added `Memory`/`Artifact` fields to `Params`, `MemoryService()`/`ArtifactService()` to interface and struct |
| `runner/runner.go` | Added `MemoryService`/`ArtifactService` to `Config`/`Runner`, wired into `InvocationContext`, added `GetMergedState` to `InMemorySessionService` |
| `cmd/demo/main.go` | Extended with `runChapter02()` demonstrating state lifecycle (scopes, artifacts, memory) |
| `runner/runner_test.go` | Added 8 integration tests (Tests 10-17) testing the full state lifecycle chain |
| `README.md` | Added Chapter 02 section explaining state lifecycle, scope semantics, and intentional omissions |

## tests

```
go test ./...      — all 12 packages pass (0 failures)
go vet ./...       — clean, no issues
go run ./cmd/demo  — runs both Chapter 01 and Chapter 02 scenarios successfully
```

### New test coverage:
- Runner + session state scoping (app/user/session/temp) — 4 tests
- Runner + artifact save/load/version/scoping — 2 tests
- Runner + memory add/search/isolation — 2 tests full-chain
- Config validation with optional services — 1 test

All existing Chapter 01 tests pass without modification (9 tests preserved).

## notes

- `memory.InMemoryService` uses `strings.Fields` for tokenization, which does not strip punctuation. This is consistent with the upstream ADK Go in-memory implementation. Tests use queries without punctuation.
- The `runner.InMemorySessionService` wraps `session.Service` internally. The added `GetMergedState` method delegates to the underlying service for full app+user+session state overlay.
- Memory and artifact services are optional in `runner.Config` — nil is accepted for backward compatibility with Chapter 01 usage.
- The `EventActions.ArtifactDelta` field exists in the event type but is not consumed during `AppendEvent` (consistent with upstream behavior — artifact saves are explicit service calls, not automatic event side-effects).
