# Codex State Lifecycle Review

## review_findings

Reviewed the implementation against Chapter 02 and the prior node reports in
`.rive-artifacts/05-state-scopes.md`,
`.rive-artifacts/06-memory-artifact-services.md`, and
`.rive-artifacts/07-state-lifecycle-integration.md`.

The Session / Memory / Artifact split is explicit and understandable. The
README explains why the stores are separate, including lifetime, data model,
query model, scope, and concurrency differences (`README.md:119-151`). The
demo has Chapter 02 scenarios for scoped state, artifact versions, and memory
search (`cmd/demo/main.go:167-360`). The runner and invocation context expose
memory and artifact services without making them mandatory (`runner/runner.go:76-102`,
`context/context.go:26-40`).

State scoping is mostly deterministic in the merged service view:

- `app:` and `user:` deltas are split with prefixes stripped by
  `ExtractStateDeltas` (`session/session.go:228-253`).
- app and user stores are maintained separately (`session/session.go:324-346`,
  `session/session.go:475-496`).
- `GetMergedState` overlays app, user, and session layers while filtering
  `app:`, `user:`, and `temp:` raw session keys from the session layer
  (`session/session.go:498-530`).
- Partial events do not persist or mutate durable state through the service
  path (`session/session.go:415-424`), and tests cover that behavior
  (`session/session_test.go:466-514`).

However, `temp:` state currently leaks into the durable in-memory session
object. `Service.AppendEvent` applies the full pre-trim delta to `is.state`
(`session/session.go:439-442`), and `applyStateDelta` writes every key,
including `temp:`, into `st.data` (`session/session.go:463-468`). The runner
path also merges tool `state_delta` directly into `ctx.Session().State()` before
the event is appended (`flow/flow.go:270-273`). Tests and the demo assert that
`temp:` remains visible in raw session state after `Run`/`AppendEvent`
(`runner/runner_test.go:693-696`, `session/session_test.go:427-430`,
`cmd/demo/main.go:344-347`). That satisfies "not in persisted event delta" and
"not in merged view", but not the stricter acceptance criterion that `temp:`
keys do not leak into durable session state. Since the same session object is
reused for later invocations, direct `Session.State().Get("temp:*")` can observe
stale invocation-local data after the invocation has ended.

Memory is compact and scoped by app/user. The store key is `(appName, userID)`
with per-session entries (`memory/inmemory.go:20-38`, `memory/inmemory.go:75-90`),
and search uses the same app/user key (`memory/inmemory.go:93-127`). Tests cover
cross-session retrieval plus app and user isolation (`memory/inmemory_test.go:71-169`).

Artifacts are versioned and scoped. The artifact identity includes app, user,
session, and filename, with `user:` filenames normalized to the shared
`user` session id (`artifact/inmemory.go:12-49`). Saves append monotonically
increasing versions (`artifact/inmemory.go:51-69`), loads can target latest or a
specific version (`artifact/inmemory.go:72-98`), and list includes session-local
plus user-scoped files only for the requested app/user (`artifact/inmemory.go:129-154`).
Tests cover independent version increments, `user:` cross-session visibility,
and app/user/session isolation (`artifact/inmemory_test.go:337-581`).

One non-blocking test-quality note: memory and artifact saved content are only
shallow-copied. `memory.AddSessionToMemory` stores `ev.Content` by pointer
(`memory/inmemory.go:65-72`), and artifact `Save` copies `ArtifactPart` but not
the nested inline data (`artifact/inmemory.go:67-68`). This is outside the
explicit node acceptance criteria, but the final merge node should consider
deep-copy tests if snapshot immutability is desired.

## required_fixes

Before final acceptance, fix the `temp:` lifecycle so temp keys are invocation
local and are absent from durable session state after the invocation is
persisted.

Recommended shape:

- Keep `temp:` visible during the active invocation through an invocation-local
  overlay or explicit cleanup phase.
- Do not leave `temp:` keys in `Session.State().All()` or `Session.State().Get`
  after `Runner.Run` returns or after a service append has completed.
- Update tests that currently assert post-run raw visibility
  (`runner/runner_test.go:693-696`, `session/session_test.go:427-430`,
  `session/session_test.go:731-734`) so they assert cleanup instead.
- Keep existing assertions that persisted event `StateDelta` is trimmed and
  `GetMergedState` excludes `temp:`.

No source fixes were made in this review node. This node only writes the review
artifact; the final OpenCode merge node owns implementation changes and the
final commit.

## verification

Commands run from `/Users/likun/Desktop/workspace-for-google-adk-go/rive-adk-go`:

```bash
go test ./...
go vet ./...
git diff --check
go run ./cmd/demo
```

Results:

- `go test ./...` passed for all packages.
- `go vet ./...` passed with no output.
- `git diff --check` passed with no whitespace errors.
- `go run ./cmd/demo` passed and printed both Chapter 01 and Chapter 02
  scenarios.

## merge_guidance

Do not merge as final without addressing the `temp:` durable-state leak. The
rest of the implementation is suitable for a compact educational Chapter 02
replica: the lifecycle split is documented, app/user/session scoping works in
the merged service view, partial events are guarded, memory is app/user scoped,
artifacts are versioned, and the required verification commands pass.

After fixing `temp:`, rerun:

```bash
go test ./...
go vet ./...
git diff --check
go run ./cmd/demo
```
