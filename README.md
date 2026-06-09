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

// 1a. Or create a tool with an explicit LLM declaration
decl := tool.NewDeclaration("get_weather", "Get weather for a city",
    map[string]any{
        "type":       "object",
        "properties": map[string]any{"city": map[string]any{"type": "string"}},
    },
    map[string]any{
        "type":       "object",
        "properties": map[string]any{"temperature": map[string]any{"type": "number"}},
    },
)
weatherToolWithDecl := tool.NewFunctionToolWithDeclaration("get_weather", "Get weather", decl, handler)

// 1b. Or create tools from a Toolset (dynamic collection)
ts := tool.NewStaticToolset("weather_tools", []tool.Tool{
    weatherToolWithDecl.(tool.Tool),
})
filteredTs := tool.NewFilterToolset("safe_tools", ts,
    tool.AllowedToolsPredicate("get_weather"),
)

// 2. Create a fake model with queued responses
fakeModel := model.NewFakeModel("my-model",
    model.FunctionCallResponse("Let me check...",
        event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
    ),
    model.TextResponse("Tokyo is 22°C."),
)

// 3. Wire up the Flow with tools and toolsets
f := &flow.Flow{
    Model:    fakeModel,
    Tools:    map[string]tool.FunctionTool{"get_weather": weatherTool},
    Toolsets: []tool.Toolset{filteredTs},  // toolset declarations auto-injected
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
- Chapter 03 — tool system integration: declaration injection, toolset
  filtering, confirmation (approve/reject), streaming tool collection,
  and long-running tool metadata.

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

---

## Chapter 03: Tool System Integration

### The Problem — Many Tool Sources Must Look Uniform to the LLM Flow

The agent at runtime interacts with diverse tool sources: Go functions, streaming
functions, MCP servers, Gemini native tools, child-agent proxies, and
file-system skills. Each source has:

- A different **schema source** (Go generics → JSON Schema, MCP `ListTools`,
  Gemini `genai.Tool`, `SKILL.md` files)
- A different **invocation model** (local function call, RPC, API-roundtrip)
- A different **lifecycle** (stateless, connection-managed, file-system-backed)
- A different **confirmation path** (static flag, dynamic provider function)

The core challenge is a unified `Tool` / `Toolset` abstraction that lets the
LLM agent treat all tools interchangeably while giving each source a
minimum-effort adaptor.

### Why Hard — Schema, Streaming, Confirmation, and Long-Running

| Concern | Complexity |
|---------|------------|
| **Schema generation** | Go generics infer JSON Schema via reflection; MCP tools carry `nil` schema traps; custom schemas must merge safely |
| **Args / Result encoding** | LLM returns `map[string]any` → convert to typed `TArgs`; tool result `TResults` → back to `map[string]any`; primitive types auto-wrapped |
| **Streaming** | `StreamingFunctionTool` returns `[]StreamChunk`; live (bidi streaming) mode pushes chunks in real time; non-live mode collects all chunks into one result |
| **Long-running** | `IsLongRunning()` flag injects a "NOTE: Do not call again" annotation into the LLM-facing declaration; tool returns `{"job_id": "...", "status": "pending"}` and completes later |
| **Confirmation (HITL)** | Three-layer confirmation: static `RequireConfirmation` flag, dynamic `ConfirmationProvider` function, and `WithConfirmation` toolset wrapper. On first call the tool returns `{"confirmation_required": true}`; a second call with `SetConfirmed(true)` executes the actual handler |
| **External toolsets (MCP)** | Connection lifecycle: lazy connect, ping-probe, auto-reconnect, retry-on-refreshable-errors; cursor-based pagination with reconnect-safe resets |

### What This Replica Implements

| Feature | Implementation |
|---------|---------------|
| `Tool` interface | Minimal: `Name()`, `Description()`, `IsLongRunning()` |
| `FunctionTool` | Extends `Tool` with `Run(args map[string]any)` — local execution |
| `StreamingFunctionTool` | `RunStream(args) ([]StreamChunk, error)` — collected in non-live mode |
| `Declaration` | Name + description + `InputSchema`/`OutputSchema` (both `map[string]any`) |
| `DeclarationProvider` | Tools can provide stable `Declaration()` for LLM injection |
| `Toolset` | Dynamic collection: `Name()`, `Tools() ([]Tool, error)` |
| `StaticToolset` | Fixed tool list |
| `FilterToolset` | Decorator: applies `Predicate func(Tool) bool` to inner toolset |
| `AllowedToolsPredicate` | Name-based whitelist filter |
| `RequestProcessor` | `ProcessRequest(*model.LLMRequest) error` — injects tool declarations |
| `InjectDeclarations` | Auto-collects `DeclarationProvider` tools, sorts, sets on `LLMRequest.ToolDeclarations` |
| `Flow.Toolsets` | Flow auto-resolves toolsets and injects declarations before each model call |
| `WithConfirmation` | Wraps a `FunctionTool` with HITL confirmation logic |
| `ConfirmationControl` | `SetConfirmed(bool)` interface for approval/rejection |
| `LongRunningFunctionTool` | `IsLongRunning() == true`, declaration annotated with "Do not repeat" note |
| `CollectStreamChunks` | Merges `[]StreamChunk` into `map[string]any{"result": concatenated}` |
| `ContextExecute` | Tool execution with `ToolContext` — preferred path for context-aware tools |
| `StreamChunk` | `{Text, Error, Final}` — streaming result atom |
| `ErrConfirmationRequired` / `ErrConfirmationRejected` | Sentinel errors for HITL flow |
| `ConfirmationProvider` | Dynamic `func(toolName, args) bool` predicate |

### Intentional Omissions

This chapter guide (`03-tool-system-deep-dive.md`) describes the full ADK Go
tool architecture, including MCP connection management, Gemini native tools,
agent-as-tool proxying, skill file-system toolsets, and callback-chain
confirmation orchestration. This runtime intentionally omits:

| Omission | Reason |
|----------|--------|
| MCP toolset (`mcptoolset`) | Requires external process lifecycle and JSON-RPC; educational scope focuses on local tools |
| Gemini native tools (`geminitool`) | GoogleSearch/Retrieval are Gemini API features; the model interface is generic |
| Agent-as-tool (`agenttool`) | Sub-agent proxying requires runner nesting; deferred for now |
| Skill toolset (`skilltoolset`) | File-system-backed `SKILL.md` parsing is a separate concern |
| Built-in infrastructure tools (`load_artifacts`, `load_memory`, `exit_loop`) | Each requires service integration; can be added incrementally |
| Live streaming (bidi) mode | Non-live chunk collection demonstrates the streaming contract without goroutine lifecycle |
| `generateRequestConfirmationEvent` | Full HITL confirm/reject pump with session event scanning; `WithConfirmation` + `SetConfirmed` demonstrates the core pattern |
| MCP connection lifecycle | `connectionRefresher`, ping/lazy-connect/retry logic; out of scope for local tools |
| `typeutil.ConvertToWithJSONSchema` | Go generics → JSON Schema inference is a separate infrastructure concern |
| `toolutils.PackTool` | Merging declarations into `genai.Tool` structure; our `InjectDeclarations` fills the same role for `LLMRequest.ToolDeclarations` |

```bash
go test ./...
go vet ./...
git diff --check
```
