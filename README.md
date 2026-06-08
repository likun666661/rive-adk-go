# rive-adk-go

A small Go replica of the Google ADK Go runtime flow.

The target is not API compatibility with `google/adk-go`. It is an educational
runtime skeleton that preserves the architecture lines described in the
deep-read guides:

- **Chapter 01**: `Runner -> Agent -> LLM Flow -> Model/Tool -> Event -> Session`
- **Chapter 02**: State lifecycle — session scoping, memory, and artifacts

The implementation is produced through a Rive workflow:

- OpenCode workers implement the runtime in staged nodes.
- A final Codex steward reviews, fixes, verifies, and commits the result.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                       Runner.Run                              │
│  session.Get/Create → append user event → create ctx          │
│  → agent.Execute → persist non-partial events → yield         │
└──────────────────────────┬───────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────┐
│                   agent.Execute (baseAgent)                    │
│  beforeAgentCallbacks → a.run(ctx) → afterAgentCallbacks      │
└──────────────────────────┬───────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────┐
│                     Flow.Run                                   │
│  for { runOneStep → if IsFinalResponse() → return }           │
│  runOneStep: preprocess → callModel → postprocess             │
│              → yield model event → handleFunctionCalls         │
└──────────────────────────┬───────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────┐
│                   Session Persistence                          │
│  Events: user → model(fc) → tool(result) → model(final)       │
│  Only non-partial events are appended to session               │
└──────────────────────────────────────────────────────────────┘
```

## Packages

| Package | Role |
|---------|------|
| `runner` | Top-level orchestrator: session management, invocation, event persistence |
| `agent` | Agent interface, lifecycle (before/after callbacks), execution |
| `llmagent` | Minimal LLM agent that wraps `flow.Flow` |
| `flow` | Multi-step model/tool execution loop with processor/callback pipeline |
| `model` | LLM interface and deterministic `FakeModel` for tests |
| `tool` | Function tool interface, parallel execution helpers |
| `event` | Core event types (Content, Part, FunctionCall/Response, Actions) |
| `session` | Session state with app/user/session/temp scope routing |
| `context` | Invocation context carrying agent, session, memory, artifact services |
| `memory` | Cross-session long-term memory with keyword search |
| `artifact` | Versioned file store scoped by app/user/session |

## Quick Start

```go
// 1. Create a tool
weatherTool := tool.NewFunctionTool("get_weather", "Get weather",
    func(args map[string]any) (map[string]any, error) {
        city, _ := args["city"].(string)
        return map[string]any{"city": city, "temp": 22}, nil
    },
)

// 2. Create a fake model with queued responses
fakeModel := model.NewFakeModel("my-model",
    model.FunctionCallResponse("Let me check...",
        event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
    ),
    model.TextResponse("Tokyo is 22°C."),
)

// 3. Wire up the Flow
f := &flow.Flow{
    Model: fakeModel,
    Tools: map[string]tool.FunctionTool{"get_weather": weatherTool},
}

// 4. Create the LLM agent
ag, _ := llmagent.New("weather_bot", "Answers weather questions.", f)

// 5. Create runner with in-memory services
r, _ := runner.New(runner.Config{
    AppName:         "my_app",
    Agent:           ag.(runner.ExecutableAgent),
    SessionService:  runner.NewInMemorySessionService(),
    MemoryService:   memory.InMemoryService(),
    ArtifactService: artifact.InMemoryService(),
})

// 6. Run
sess, events, _ := r.Run(context.Background(), "user-1", "sess-1", "Weather in Tokyo?")
```

## Demo

```bash
go run ./cmd/demo
```

Output shows:
- Chapter 01 — the complete chain: user message, model function call, tool
  execution, final model response, and session persistence.
- Chapter 02 — state lifecycle: scoped state mutation, artifact versioning,
  and cross-session memory search.

---

## Chapter 02: State Lifecycle

### The Problem Each Lifecycle Solves

**Session state** is a short-term container for a single conversation thread. It
holds ordered event history and mutable key-value state. Session state is scoped
to one `(app, user, session)` triple. It perishes when the session ends or is
explicitly deleted.

**Memory** provides long-term knowledge that survives across sessions. When a
user tells an agent "I prefer Python" in one session, subsequent sessions
should recall that preference. Memory ingests session events and supports
keyword-based search.

**Artifacts** are versioned files produced or referenced during a conversation
(charts, reports, generated code). Artifacts evolve independently of event
history — each save increments a version counter. Artifact versions are
loadable by version number or as "latest."

### Why Session / Memory / Artifact Should Not Be One Store

| Dimension | Session | Memory | Artifact |
|-----------|---------|--------|----------|
| **Lifetime** | Created → events appended → ended/deleted | Long-lived, cross-session | Per-file, independently versioned |
| **Data model** | Ordered event list + KV map | Content fragments + keyword index | File blob + version number |
| **Query** | Key lookup, time range, last N | Keyword/intersection search | File name + optional version |
| **Scope** | app + user + session | app + user (cross-session) | app + user + session (or user:) |
| **Concurrency** | Append with state merge | Batch overwrite per session | Version auto-increment |

Merging them into one store would:
- Bloat session events with large file content (artifact blobs in event stream).
- Pollute long-term memory with transient conversational state.
- Couple file version lifetime to session deletion.

### Scoped State (app / user / session / temp)

State keys use optional prefixes to route mutations to the correct scope:

| Prefix | Scope | Lifetime |
|--------|-------|----------|
| `app:` | Shared by all users and sessions in the app | App lifetime |
| `user:` | Shared across all sessions for the same user | User lifetime |
| *(none)* | Private to the individual session | Session lifetime |
| `temp:` | Visible only during the current invocation | Invocation lifetime |

When reading, layers are overlaid: `session > user > app`. Session-level keys
take priority. A `__STATE_TOMBSTONE__` value in session state hides the
corresponding `app:` and `user:` keys from the merged view.

**Example — two sessions for the same user:**

```
sess-1 sets: app:env=prod, user:theme=dark, topic=lifecycle
sess-2 sees: app:env=prod, user:theme=dark (but NOT topic — session-scoped)
```

### Artifacts

- Each `Save` auto-increments the version and returns it.
- `Load` without a version returns the latest; with a version returns that
  specific version.
- File names prefixed with `user:` are shared across all sessions for a user.
- `List` returns file names (not blob content) for a session.

### Memory

- `AddSessionToMemory` ingests all non-partial events from a session, tokenizes
  text into lowercase words, and stores them by `(app, user)`.
- `SearchMemory` performs word-intersection match against the query.
- Memory is scoped to `(app, user)` — sessions from other users or apps are
  never returned.

### Simplified Semantics in This Implementation

- **No vector/embedding search**: keyword intersection only (matches upstream
  in-memory implementation).
- **No stale-session detection**: unlike the database backend described in the
  chapter guide, this runtime has no multi-writer coordination.
- **No external backends**: only in-memory implementations are provided. The
  service interfaces are public, enabling swap-in of GCS, SQL, or Vertex AI
  backends.
- **ArtifactDelta tracking**: `EventActions.ArtifactDelta` is defined but not
  consumed during `AppendEvent`. Artifact saves are explicit calls through the
  service, not automatic side-effects of event processing.
- **Memory session overwrite**: calling `AddSessionToMemory` twice for the same
  session replaces all entries (not incremental append). This matches the
  upstream in-memory behavior.

### Intentional Omissions

This chapter guide (`02-state-lifecycle-deep-dive.md`) describes the full ADK
Go state architecture, including GORM-backed SQL sessions, Vertex AI
MemoryBank, GCS artifacts, and multi-backend session implementations. This
runtime intentionally omits:

| Omission | Reason |
|----------|--------|
| Database session backend (GORM/SQLite) | Educational scope; the in-memory service demonstrates the full state routing logic |
| Vertex AI session and memory backends | Requires cloud credentials and external services |
| GCS artifact backend | Requires cloud credentials; in-memory covers all API semantics |
| `ArtifactDelta` auto-tracking in callbacks | Callback context injection is an internal ADK Go detail not needed for the simplified model |
| Stale session detection (microsecond timestamps) | No multi-writer scenario in this runtime |
| `SaveRequest.Version` for optimistic concurrency | The in-memory implementation ignores this field (consistent with upstream in-memory) |
| Request-history reconstruction from session events | A separate concern not needed for this demo |
| Event filtering by `After` timestamp | Not required for most educational scenarios |
| Long-running tool support | Not related to state lifecycle |

---

## Verification

```bash
go test ./...
go vet ./...
git diff --check
```
