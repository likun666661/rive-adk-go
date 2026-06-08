# Node 2: Memory and Artifact Services

## implemented

### memory package (`memory/`)

Three files implementing an in-memory memory service that demonstrates cross-session long-term knowledge retrieval:

- `service.go` — Defines `Service` interface with `AddSessionToMemory` and `SearchMemory`, plus `SearchRequest`, `SearchResponse`, and `Entry` types. Entry carries event content, author, timestamp, and custom metadata.
- `inmemory.go` — Thread-safe in-memory implementation using `map[appUserKey]map[sessionID][]entryValue`. `AddSessionToMemory` ingests non-partial events from a session, extracts text parts, tokenises into lowercase words for keyword matching. `SearchMemory` tokenises the query and uses word-set intersection (`wordsIntersect`) to find matching entries. Store is keyed by (appName, userID) so memory survives across sessions but is scoped to app+user.
- `inmemory_test.go` — 10 test functions covering: basic add+search, cross-session memory survival, app-scoped isolation, user-scoped isolation, no-match behaviour, empty-store safety, partial-event skipping, session-entry overwrite on re-add, content copy (mutation resistance), author/timestamp propagation, and nil-content skipping.

### artifact package (`artifact/`)

Three files implementing an in-memory versioned artifact store with app/user/session scoping and user-cross-session namespaces:

- `service.go` — Defines `Service` interface with six methods (`Save`, `Load`, `Delete`, `List`, `Versions`, `GetArtifactVersion`). Includes request/response types with `Validate()` methods, `ArtifactPart` (text or inline binary data), `ArtifactVersion` metadata type, and filename validation (no path separators).
- `inmemory.go` — Thread-safe implementation using `map[artifactIdentity][]versionedPart`. Key operations:
  - **Save**: resolves identity (user:` prefix → sessionID replaced with constant `"user"`), finds max version in existing slice, appends new `versionedPart` with `version+1`.
  - **Load**: returns latest version (last in slice) or exact version if `Version > 0` is specified.
  - **Delete**: deletes a specific version or all versions (`Version=0` removes entire identity entry).
  - **List**: collects filenames across session-scoped and user-scoped (`user:` prefix) artifacts, deduplicates, returns sorted.
  - **Versions**: returns all version numbers for a given artifact identity.
  - **GetArtifactVersion**: returns metadata (version, mime type) without exposing blob content.
- `inmemory_test.go` — Comprehensive test suite covering:
  - Basic Save/Load (latest and explicit version)
  - Delete specific version
  - Delete all versions
  - Delete non-existing (no-op)
  - Load non-existing (fs.ErrNotExist)
  - Versions listing
  - Versions non-existing
  - List with session scoping
  - List empty
  - GetArtifactVersion (text and binary mime detection)
  - GetArtifactVersion explicit version

### Lifecycle boundary tests

Four focused test categories:

1. **Versions independent from events**: `TestInMemoryService_VersionsIndependentFromEvents` — saving the same file multiple times produces versions 1, 2, 3 without any reference to session events. Different files start at version 1 independently.

2. **Version increments deterministic**: `TestInMemoryService_VersionIncrementDeterministic` — five saves to the same file produce exactly versions [1,2,3,4,5], confirming the version counter is sequential.

3. **user: namespace visible across sessions**: `TestInMemoryService_UserScopedArtifactCrossSession` — a `user:preferences.json` file saved from `sess1` is loadable from `sess2` (same user). `TestInMemoryService_UserScopedArtifactIsolatedByUser` — user-scoped artifact does not leak to another user. `TestInMemoryService_UserScopedArtifactIsolatedByApp` — user-scoped artifact does not leak across apps. `TestInMemoryService_UserScopedArtifactVersions` — version counter for user-scoped artifacts is shared across sessions (saving from sess1 and sess2 produces [1,2]). `TestInMemoryService_UserScopedArtifactListedInBothSessions` — listing correctly includes user-scoped files in all the user's sessions.

4. **Memory survives across sessions but scoped**: `TestInMemoryService_MemorySurvivesAcrossSessions` — Python knowledge from sess1 is still findable after sess2 adds Go knowledge; Go is also findable. `TestInMemoryService_AppScopedIsolation` — memory from app_a does not leak to app_b. `TestInMemoryService_UserScopedIsolation` — memory from user_a does not leak to user_b.

## files_changed

| File | Description |
|------|-------------|
| `memory/service.go` | Memory Service interface, SearchRequest/Response, Entry types |
| `memory/inmemory.go` | In-memory implementation with keyword search |
| `memory/inmemory_test.go` | 10 tests covering lifecycle boundaries |
| `artifact/service.go` | Artifact Service interface, request/response types, validation |
| `artifact/inmemory.go` | In-memory versioned store implementation |
| `artifact/inmemory_test.go` | 20+ tests covering lifecycle boundaries |

## tests

```
$ go test ./...
ok  github.com/likun666661/rive-adk-go/memory    0.846s
ok  github.com/likun666661/rive-adk-go/artifact   0.601s
ok  github.com/likun666661/rive-adk-go/agent      (cached)
ok  github.com/likun666661/rive-adk-go/event      (cached)
ok  github.com/likun666661/rive-adk-go/flow       (cached)
ok  github.com/likun666661/rive-adk-go/llmagent   (cached)
ok  github.com/likun666661/rive-adk-go/model      (cached)
ok  github.com/likun666661/rive-adk-go/runner     (cached)
ok  github.com/likun666661/rive-adk-go/session    (cached)
ok  github.com/likun666661/rive-adk-go/tool       (cached)
```

All existing tests continue to pass. No regressions.

## notes

- **Design alignment with Chapter 02**: Memory uses (app, user) scoping and survives across sessions; artifacts use (app, user, session) scoping with independent versioning; user:` prefix provides cross-session visibility. This separation prevents session bloat, memory pollution from conversational state, and version coupling to event timestamps.

- **Compact implementation**: Both services use only the standard library and existing project types (`session.Session`, `event.Event`, `event.Content`). No external dependencies were added (the ADK Go reference uses `rsc.io/ordered`/`rsc.io/omap` and `google.golang.org/genai`, but the in-memory maps approach suffices for this compact demo).

- **Thread safety**: Both services use `sync.RWMutex`. Memory service holds a read lock during search (since it only iterates maps). Artifact service holds a read lock during Load/List/Versions/GetArtifactVersion and a write lock during Save/Delete.

- **Partial event exclusion**: The memory service skips events with `Partial=true`, matching the session service's persistence semantics and preventing in-flight streaming chunks from polluting long-term memory.

- **Built on Node 1**: The memory service uses `session.Session.AppName()`, `UserID()`, `ID()`, and `Events()` from the existing session package. No session package changes were needed.
