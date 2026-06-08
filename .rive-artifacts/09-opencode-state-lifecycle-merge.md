# 09-opencode-state-lifecycle-merge.md

## review_response

All findings from `.rive-artifacts/08-codex-state-lifecycle-review.md` have been
addressed.

The primary required fix — `temp:` keys leaking into durable session state after
`AppendEvent` — has been resolved by adding a `removeTempKeysFromState` cleanup
step in both `Service.AppendEvent` and standalone `inMemorySession.AppendEvent`.
Temp keys are now cleaned from `stateImpl.data` immediately after the trimmed
event is persisted. This ensures `Session.State().Get("temp:*")` returns
`(nil, false)` after `Runner.Run` returns or after a service append completes.

The non-blocking test-quality note about shallow copies in memory and artifact
services (pointer storage / `ArtifactPart` reuse) was reviewed. This is
consistent with the upstream ADK Go in-memory implementation and is acceptable
for this compact educational replica. Snapshot immutability would require
deep-copy tests, which are intentionally deferred as out-of-scope for Chapter 02.

## fixes

**`session/session.go`**:
- Added `removeTempKeysFromState(*stateImpl)` helper that iterates
  `st.data` under lock and deletes any key with `"temp:"` prefix
  (`session/session.go:297-306`).
- `Service.AppendEvent`: calls `removeTempKeysFromState(is.state)` after
  applying the full delta and trimming the persisted `StateDelta`
  (`session/session.go:455`).
- `inMemorySession.AppendEvent` (standalone path): calls
  `removeTempKeysFromState(s.state)` after applying delta and trimming
  (`session/session.go:174`).

**`session/session_test.go`**:
- Renamed `TestServiceTempStateVisibleDuringInvocation` →
  `TestServiceTempStateCleanupAfterAppend`. Now asserts temp keys are
  NOT present in session state after AppendEvent, while durable keys
  survive and temp is absent from the persisted event's StateDelta.
- `TestStandaloneSessionTempStateHandling`: changed assertion from
  "temp should be visible" to "temp should be cleaned".

**`runner/runner_test.go`**:
- `TestRunnerScopedStateMutation` (line 544): changed assertion from
  "temp:scratch = tmp_val" to "temp should be cleaned from durable
  session state after Run".
- `TestRunnerTempStateLifecycle` (line 693): same change — temp key
  must be absent from session state after Run returns.

**`cmd/demo/main.go`**:
- Updated Scenario 2 output: after checking that temp is absent from
  merged state, now also checks session raw state and reports
  "temp:scratch correctly absent from durable session state (cleaned
  after persist)".

**`.gitignore`**:
- Added `/demo` to prevent the compiled demo binary from being
  committed.

## verification

All four required commands pass from the workspace root:

```
go test ./...
ok  	github.com/likun666661/rive-adk-go/agent	(cached)
ok  	github.com/likun666661/rive-adk-go/artifact	(cached)
ok  	github.com/likun666661/rive-adk-go/event	(cached)
ok  	github.com/likun666661/rive-adk-go/flow	(cached)
ok  	github.com/likun666661/rive-adk-go/llmagent	(cached)
ok  	github.com/likun666661/rive-adk-go/memory	(cached)
ok  	github.com/likun666661/rive-adk-go/model	(cached)
ok  	github.com/likun666661/rive-adk-go/runner	(cached)
ok  	github.com/likun666661/rive-adk-go/session	(cached)
ok  	github.com/likun666661/rive-adk-go/tool	(cached)
```

```
go vet ./...
(no output)
```

```
git diff --check
(no output — no whitespace errors)
```

```
go run ./cmd/demo
All 4 Chapter 02 scenarios pass.
temp:scratch correctly absent from both merged state and durable session state.
```

## commit

- **SHA**: final merge commit in `git log --oneline -1`
- **Message**: `Add state lifecycle replica`
- **Includes**: session state scopes, memory service, artifact service,
  runner/context integration, demo scenarios, workflow prompts, Codex review
  artifacts, temp lifecycle fix, .gitignore update.
- 24 files changed, 4689 insertions(+), 100 deletions(-).
