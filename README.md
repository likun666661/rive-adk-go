# rive-adk-go

A small Go replica of the Google ADK Go runtime flow.

The target is not API compatibility with `google/adk-go`. It is an educational
runtime skeleton that preserves the architecture lines described in the
deep-read guides:

- **Chapter 01**: `Runner -> Agent -> LLM Flow -> Model/Tool -> Event -> Session`
- **Chapter 02**: State lifecycle — session scoping, memory, and artifacts
- **Chapter 03**: Tool system — declarations, streaming, confirmation, long-running
- **Chapter 04**: Callbacks / plugins / instruction injection
- **Chapter 05**: Multi-agent composition — workflows, AgentTool, remote A2A
- **Chapter 06**: Entrypoints, deploy, and telemetry — console/web routing, REST/SSE, dry-run deploy plans, instrumentation model
- **Chapter 07**: Configurable agent flow — ReAct loop, agent transfer, ExitLoop/retry-reflect/hidden-arg policies, JSON config loader

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
| `workflow` | Sequential, parallel, and loop agent orchestration |
| `agent/agentconfig` | JSON config loader: builds agent trees from declarative config with validation |
| `tool/agenttool` | Agent-as-tool: wraps an agent as a FunctionTool with isolated child session |
| `agent/remoteagent` | Remote A2A bridge: streaming, conversion, partial aggregation, cleanup |
| `tool/exitloop` | ExitLoop tool: LLM-callable early loop termination signal |
| `plugin/retryreflect` | RetryAndReflect plugin: tool error recovery with reflection guidance |
| `plugin/functionmodifier` | FunctionCallModifier plugin: inject/strip hidden args from tool declarations |
| `cmd/launcher` | Entrypoint abstractions: Config, Launcher, SubLauncher, AgentLoader |
| `cmd/launcher/universal` | Keyword-based router; first sublauncher is default |
| `cmd/launcher/console` | Interactive console driving `runner.Run` from stdin |
| `cmd/launcher/web` | HTTP server wrapper mounting adkrest endpoints |
| `server/adkrest` | REST API server: JSON, SSE, strict request validation |
| `deploy` | Dry-run deploy plans: CloudRunPlan, AgentEnginePlan (zero cloud calls) |
| `telemetry` | In-memory span/log recorder with OTel-inspired instrumentation helpers |

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
- Chapter 04 — callbacks, plugins, and instruction injection: logging
  plugins, before-model cache early-exit, state-driven instruction
  interpolation, and plugin/callback ordering.
- Chapter 05 — multi-agent composition: sequential/parallel/loop workflows,
  AgentTool delegation with isolated child sessions, and remote A2A
  streaming aggregation with partial-to-full event merging.
- Chapter 06 — entrypoint, deploy, and telemetry: launcher config abstraction,
  console/web routing, REST JSON/SSE protocols, deterministic dry-run deploy
  plans (Cloud Run + Agent Engine), and in-memory telemetry recorder with
  span/log instrumentation around runner invocations.
- Chapter 07 — configurable agent flow: ReAct function-call loop, agent
  transfer via transfer_to_agent (host → specialist), policy extensions
  (ExitLoop, retry/reflect, hidden args), and configurable agent tree
  construction from a JSON config file with duplicate/type/tool validation.

## Real LLM Smoke

The default demo uses `FakeModel` for deterministic tests. To verify the same
runner/flow/tool loop against a real OpenAI-compatible backend, run:

```bash
DEEPSEEK_API_KEY=... go run ./cmd/realllm
```

Optional environment:

- `DEEPSEEK_MODEL` (default: `deepseek-chat`)
- `DEEPSEEK_BASE_URL` (default: `https://api.deepseek.com/v1`)
- `ADKGO_REAL_LLM_PROMPT` (default asks the model to call `get_weather`)

The smoke succeeds only if the real model calls `get_weather`, the tool result
is fed into the next model turn, and a final answer is produced. This exercises
the same `Runner -> Agent -> Flow -> Model/Tool -> Event -> Session` path as the
fake demo, but uses the `model.OpenAICompatibleModel` adapter.

---

## Chapter 07: Configurable Agent Flow

Chapter 07 extends the Chapter 01-06 runtime rather than replacing it. The
same `runner.Run -> agent.Execute -> flow.Flow -> model/tool -> event/session`
path now also demonstrates:

- ReAct iteration as repeated model calls, function calls, tool result events,
  and final model responses.
- `transfer_to_agent` as a structured tool action recorded in
  `EventActions.TransferToAgent`, not as plain text.
- Active-agent routing from session history, so a later user turn can continue
  with the last transferable specialist agent.
- Agent tree metadata (`SubAgents`, `Parent`, `FindAgent`) and transfer
  constraints for parent/peer routing.
- Policy hooks for `ExitLoop`, retry/reflection on tool errors, and hidden
  function-call args.
- A small JSON config loader for building teaching examples with `llm_agent`,
  `sequential`, `parallel`, and `loop` nodes.

Deliberate simplifications:

- Config is JSON only; YAML and production deployment config are omitted.
- Config-built LLM agents use deterministic `FakeModel` instances. Real model
  provider selection is left as an extension point.
- Plugin and instruction configuration are not represented in JSON.
- Streaming and partial aggregation remain the simplified behavior introduced
  in earlier chapters.
- This replica teaches the architecture shape; it is not API-compatible with
  `google/adk-go`.

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
- **ArtifactDelta tracking**: callback contexts record artifact saves in
  `EventActions.ArtifactDelta`. `AppendEvent` persists the event metadata but
  does not replay artifact writes; artifact content is saved explicitly through
  the artifact service.
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
| Automatic artifact replay from events | Artifact content is saved through the artifact service; events only record `ArtifactDelta` metadata |
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

---

## Chapter 04: Callback / Plugin / Instruction Integration

### Why Three Extension Layers Exist

The agent runtime needs cross-cutting extension points for observability, flow
control, request rewriting, error recovery, and state management. Without
unified hooks, each feature would require invasive changes to core logic.

Three layers serve distinct purposes:

| Layer | Role | Key Types |
|-------|------|-----------|
| **Instruction (Processor)** | Inject system instructions before each LLM call using static templates, dynamic providers, and `{placeholder}` injection from session state | `instruction.Config`, `instruction.Provider`, `InjectSessionState` |
| **Callback** | Direct hooks on flow events (before/after model, before/after tool) registered on individual agents, with optional `CallbackContext` state/action tracking | `flow.BeforeModelCallbackCtx`, `flow.AfterModelCallbackCtx`, `flow.BeforeToolCallbackCtx`, `flow.AfterToolCallbackCtx` |
| **Plugin** | Composable, named, ordered hook bundles registered on a `plugin.Manager` that runs before direct callbacks | `plugin.Plugin`, `plugin.Manager`, `Plugin.BeforeModelCallback()`, etc. |

### Instruction Processors

**Static instruction** — a literal string in agent configuration:

```go
cfg := instruction.Config{
    Instruction: "You are a helpful assistant for {topic}.",
}
```

**Dynamic instruction provider** — a function called at each LLM request:

```go
cfg.InstructionProvider = func(ctx instruction.ReadonlyContext) (string, error) {
    return "Current user: " + ctx.UserID(), nil
}
```

**Global instruction + provider** — applied only for root agents:

```go
cfg.GlobalInstruction = "Safety rules apply to all agents."
cfg.GlobalInstructionProvider = globalRulesProvider
cfg.IsRootAgent = func() bool { return true }
```

**`{placeholder}` injection** — resolve `{key}`, `{app:key}`, `{user:key}`,
`{temp:key}`, and optional `{key?}` patterns from the merged session state:

```go
injected, err := instruction.InjectSessionState(
    "User {user:name} is working on {task}.", mergedState)
```

Wire into the flow as a `RequestProcessor`:

```go
f.RequestProcessors = []flow.RequestProcessor{
    instruction.ToRequestProcessor(instruction.NewRequestProcessor(cfg)),
}
```

### Plugin Layer

Plugins compose multiple hook types into a named, orderable unit. The
`plugin.Manager` runs plugin hooks before direct callbacks (Chapter 04
teaching model).

**Hook execution semantics:**
- **Registration order** — first registered runs first.
- **Nil skip** — hooks a plugin doesn't implement are silently skipped.
- **Early exit** — first non-nil result short-circuits the remaining chain.
- **Immediate error** — any hook error aborts the entire chain.

**Plugin vs Callback:**

| Dimension | Callback | Plugin |
|-----------|----------|--------|
| Installation | Directly in `Flow` struct fields | Through `PluginManager.Register()` |
| Lifecycle | Bound to agent/flow instance | Global; shareable across agents |
| Hook coverage | Model/Tool level | Model/Tool/Agent level |
| Composability | Static lists | Ordered dynamic list |
| Use case | Single-agent customization | Cross-agent concerns (logging, caching, retry) |

### What This Replica Implements

| Feature | Implementation |
|---------|---------------|
| Static instruction | `instruction.Config.Instruction` |
| Dynamic instruction provider | `instruction.Provider` with `ReadonlyContext` |
| Global instruction (root-only) | `instruction.Config.GlobalInstruction` + `IsRootAgent` |
| Global instruction provider (root-only) | `instruction.Config.GlobalInstructionProvider` |
| `{placeholder}` injection | `instruction.InjectSessionState` — regex-based, supports `{key}`, `{key?}`, `{app:key}`, `{user:key}`, `{temp:key}` |
| Flow integration | `instruction.ToRequestProcessor` adapts to `flow.RequestProcessor` |
| `LLMRequest.SystemInstruction` | New field on `model.LLMRequest` |
| Plugin Manager | `plugin.Manager` with ordered execution + early exit |
| Plugin hooks | Before/After agent, model, tool + OnError hooks |
| Callback context | `callbackctx.CallbackContext` / `ToolContext` expose readonly identity, write-through state, artifact tracking, and action access |
| Event action deltas | Callback state writes surface on emitted model/tool events through `StateDelta`; callback artifact saves surface through `ArtifactDelta` |
| Plugin before direct callbacks | Flow runs plugin hooks before context-aware direct callbacks and legacy direct callbacks |
| Plugin logging demo | `demoPluginLogging` — pure observer, all hooks |
| Before-model cache/mock | `demoBeforeModelCache` — early exit from cache |
| Instruction interpolation | `demoInstructionInterpolation` — state → instruction |
| Plugin/callback ordering | `demoPluginOrdering` — plugins before callbacks |

### Why Hook Ordering and Early-Exit Matter

1. **Observability plugins must run first** — a logging plugin registered first
   should see the request before other plugins modify it.
2. **Cache plugins must run before the LLM call** — returning a cached
   `LLMResponse` from `BeforeModelCallback` short-circuits the expensive API
   call (early exit).
3. **State mutation in callbacks** — a `BeforeAgentCallback` writing to session
   state is visible to subsequent `RequestProcessor` running in `preprocess`,
   enabling instruction interpolation from dynamically set state.
4. **Error recovery chain** — `OnModelError` hooks can replace errors with
   recovery responses, but only if they run in the correct order.

### Intentional Omissions

| Omission | Reason |
|----------|--------|
| Real telemetry exporters (OTLP, Prometheus) | Educational scope; the logging plugin demonstrates the pure observer pattern |
| Auth plugins (OAuth, API key injection) | Requires external auth services |
| Full ADK plugin API compatibility | This replica defines its own minimal Plugin/Manager types |
| `Plugin.Close()` with timeout | Close is synchronous with no timeout; adequate for in-process plugins |
| Plugin priority/dependency declarations | Priority is registration order only; no explicit dependency graph |
| Agent-level plugin hooks in the standard `runner.Run` path | `BeforeAgent`/`AfterAgent` are available through `context.RunWithCallbackContext`; the existing Chapter 01-03 `agent.Execute` path remains unchanged |
| `RetryAndReflect` plugin | The retry loop requires multi-turn LLM coordination; the before-model cache demo covers the early-exit pattern |
| Configurable layer (YAML → plugin wiring) | Plugins are registered programmatically; YAML mapping is deferred |
| `FunctionCallModifier` (tool schema rewriting) | The `InjectDeclarations` mechanism in flow serves a similar purpose for tool declaration injection |

```bash
go test ./...
go vet ./...
git diff --check
```

---

## Chapter 05: Workflow Agents / AgentTool / Remote A2A

### What Chapter 05 Adds

On top of the single-agent LLM loop (Chapters 01–04), Chapter 05 adds a
multi-agent **composition layer** with three distinct delegation mechanisms:

| Mechanism | What it does | Use case |
|-----------|-------------|----------|
| **SequentialAgent** | Runs sub-agents one at a time in declaration order. Events concatenate sequentially. | Pipeline: code-gen → review → fix |
| **ParallelAgent** | Runs sub-agents concurrently. Branch labels (`parent.child`) on each event. Results emitted in declaration order. | Multi-perspective analysis, ensemble review |
| **LoopAgent** | Runs sub-agents repeatedly until max iterations or `Actions.Escalate`. | Iterative fix-then-test, optimization loops |
| **AgentTool** | Wraps an agent as a `FunctionTool`. Child runs in an isolated session with parent state copied (minus `_adk` internals). | On-demand delegation: "I need a math agent" |
| **RemoteAgent** | Bridges local invocation to a remote A2A stream. Supports partial-to-full event aggregation, cleanup callbacks, and custom converters. | Cloud agent proxies, service meshes |

### How Workflow Agents Differ from Tool Delegation and Remote Delegation

| Dimension | Workflow Agent (Sequential/Parallel/Loop) | AgentTool (Agent as Tool) | Remote A2A |
|-----------|------------------------------------------|--------------------------|------------|
| **Session** | Shared session; sub-agents see each other's state | Isolated child session; non-`_adk` parent state copied | Remote session (opaque to local) |
| **Invocation** | Sub-agents execute as part of the same `Execute()` call | Child invoked synchronously via `Tool.Run()` — blocks parent LLM | Network RPC via `A2AClient.SendStreamingMessage()` |
| **Lifecycle** | Parent agent owns the full sub-agent sequence/loop | Child runner created per tool call; disposed after result | Remote task lifecycle managed with `cleanupRemoteTask` / `CancelTask` |
| **Event model** | Sub-agent events flow through parent's event iterator | Child events collected internally; last text result returned | A2A events converted via `Converter` then aggregated via `aggregator` |
| **Error semantics** | First sub-agent error stops chain (sequential) or propagates with partial results (parallel) | Error returned as tool result | Stream/convert/cleanup errors combined and returned |
| **State sharing** | Direct — writes to same session state | Explicit copy — `_adk` filtered, no write-back | N/A — remote session is opaque |

### Architecture: Composition Layer

```
┌──────────────────────────────────────────────────────────────┐
│                       Runner.Run                              │
│  session.Get/Create → append user event → create ctx          │
│  → agent.Execute → persist non-partial events → yield         │
└──────────────────────────┬───────────────────────────────────┘
                            │
                            ▼
┌──────────────────────────────────────────────────────────────┐
│                  Workflow Agent (composed)                     │
│                                                               │
│  Sequential: subAgent[0] → subAgent[1] → ... → subAgent[N]   │
│  Parallel:   subAgent[0] ∥ subAgent[1] ∥ ... ∥ subAgent[N]  │
│  Loop:       for { subAgent[0] → ... → subAgent[N] }         │
│                                                               │
│  Each sub-agent can be:                                       │
│    • LLM agent (local)                                        │
│    • Another workflow agent (nesting)                         │
│    • Remote agent (A2A bridge)                                │
│    • AgentTool (via parent LLM function call)                 │
└──────────────────────────────────────────────────────────────┘
```

### Sequential Workflow

```go
coder := newDemoAgent("coder", "Generates Go code.",
    model.TextResponse("func Add(a, b int) int { return a + b }"),
)
reviewer := newDemoAgent("reviewer", "Reviews Go code.",
    model.TextResponse("Review passed: function is correct."),
)

seq := workflow.NewSequentialAgent("pipeline", "code-gen → review",
    []workflow.SubAgent{coder, reviewer},
)

r, _ := runner.New(runner.Config{AppName: "demo", Agent: seq, SessionService: ...})
_, events, _ := r.Run(ctx, "user-1", "sess-1", "Write an Add function")
// events[0].Author == "coder"
// events[1].Author == "reviewer"
```

**Key semantics:**
- Strict order — each sub-agent's events are fully consumed before the next starts.
- Shared session — state written by `coder` is visible to `reviewer`.
- `EndInvocation()` from any sub-agent stops the chain.

### Parallel Workflow

```go
par := workflow.NewParallelAgent("review-team", "parallel review",
    []workflow.SubAgent{analyst, critic, evaluator},
)

_, events, _ := r.Run(ctx, "user-1", "sess-1", "Analyze")
// Each event carries branch label "review-team.analyst", etc.
```

**Key semantics:**
- All sub-agents run concurrently via goroutines.
- Results collected via buffered channel, emitted in declaration order (deterministic).
- Branch labels on events enable event grouping by sub-agent identity.
- First error propagates; all successful events preserved.

### Loop Workflow

```go
loop := workflow.NewLoopAgent("fix-loop", "fix-then-test",
    []workflow.SubAgent{fixer}, 10,  // max 10 iterations
)

// Sub-agent sets event.Actions.Escalate = true to stop early.
```

**Key semantics:**
- `maxIterations=0` means infinite loop (stopped only by escalate or error).
- Each full pass through all sub-agents counts as one iteration.
- `Actions.Escalate` is the cooperative stop signal.

### AgentTool Delegation

```go
childAgent, _ := agent.New(agent.Config{
    Name: "math_agent", Description: "Solves math problems.",
    Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
        return []*event.Event{eventWithText("42")}, nil
    },
})

at := agenttool.New(childAgent, nil)
ft := at.(tool.FunctionTool)

parentFlow := &flow.Flow{
    Model: fakeModel,
    Tools: map[string]tool.FunctionTool{"math_agent": ft},
}
```

**Key semantics:**
- Child runs in a new `InMemorySessionService` — fully isolated.
- Non-internal parent state (`_adk`-prefixed keys excluded) copied into child session.
- Child output returned as `{"result": "<last text>"}`.
- `SkipSummarization` config flag stops parent agent loop after delegation.

### Remote A2A Streaming

```go
cfg := remoteagent.FakeA2AClientConfig{
    Card:  remoteagent.AgentCard{Name: "remote-kb", StreamingSupported: true},
    Events: []remoteagent.StreamEvent{
        {Event: &remoteagent.RemoteEvent{Type: ..., Parts: []remoteagent.RemotePart{{Text: "chunk1"}}, Append: true}},
        {Event: &remoteagent.RemoteEvent{Type: ..., Parts: []remoteagent.RemotePart{{Text: "chunk2"}}, Append: true, LastChunk: true}},
        {Event: &remoteagent.RemoteEvent{Type: ..., State: remoteagent.TaskStateCompleted}},
    },
}

remoteAgent, _ := remoteagent.NewRemoteAgent(remoteagent.RemoteAgentConfig{
    Name: "kb-bridge", AgentCard: cfg.Card,
    ClientProvider: func(card remoteagent.AgentCard) (remoteagent.A2AClient, error) {
        return remoteagent.NewFakeA2AClient(cfg), nil
    },
    CleanupCallbacks: []remoteagent.CleanupCallback{...},
})
```

**Key semantics:**
- `A2AClient` interface: `SendStreamingMessage`, `CancelTask`, `Destroy`.
- `Converter` maps `RemoteEvent` → `[]*session.Event`. Default handles all 3 event types.
- `aggregator` accumulates `Append` chunks; flushes on `LastChunk` or terminal status.
- `CleanupCallback`s invoked in order on stream error or context cancellation.

### What Is Intentionally Simplified

This replica is an educational teaching model. The following real ADK Go features
are simplified or omitted:

| Omission | Reason |
|----------|--------|
| `RunLive` (bidi streaming for sequential agent) | Requires `task_completed` tool injection and multi-session routing; the sync `Execute` model suffices for teaching |
| Per-event backpressure (`ackChan`) in parallel agent | Simplified to collection-then-order; backpressure is irrelevant for `[]*event.Event` (not `iter.Seq2`) |
| Real network A2A (REST/gRPC) | `FakeA2AClient` uses Go channels for in-memory streaming; the interface is extensible to real transports |
| A2A v0/v1 protocol version negotiation | This replica defines its own simplified `RemoteEvent` model; no JSON-RPC or protocol negotiation |
| `OutputArtifactPerEvent` vs `OutputArtifactPerRun` artifact modes | Aggregation is text-part-only; no artifact service integration |
| MCP toolset in AgentTool context | AgentTool uses the local tool/runner sandbox only; no external process lifecycle |
| `IsLongRunning` for AgentTool | Always returns `false` — agent-as-tool is synchronous in this model |
| Gemini native tools via remote A2A | The `RemotePart` model supports `FunctionCall`/`FunctionResponse` but does not parse Gemini-specific metadata |

### Verification

```bash
go test ./...
go vet ./...
git diff --check
```

---

## Chapter 06: Entrypoint, Deploy, Telemetry

### What Chapter 06 Adds

On top of the single-agent LLM loop (Chapters 01–04) and multi-agent composition
(Chapter 05), Chapter 06 adds the **productization boundary**: how one agent
runtime exposes itself through different entrypoints and what observability
looks like.

| Layer | Role |
|-------|------|
| **Launcher** | Stable `Config` abstraction carrying all services; `SubLauncher` interface for composable entrypoints; keyword-based router |
| **REST Server** | HTTP JSON (`/run`) and SSE (`/run_sse`) endpoints wrapping `runner.Run` |
| **Deploy Plans** | Deterministic dry-run snapshots — Cloud Run Dockerfile + gcloud commands, Agent Engine multi-stage Dockerfile + class methods + archive |
| **Telemetry** | In-memory span/log recorder; instrumentation helpers for invoke agent, generate content, execute tool, server events |

### How Launcher Config Keeps Entrypoints Stable

The core design is that `launcher.Config` (at `cmd/launcher/launcher.go:36`)
carries all service dependencies — session, memory, artifact, agent loader, and
plugin manager. No entrypoint creates its own global singleton.

```
launcher.Config
  → AgentLoader.RootAgent()
  → SessionService / MemoryService / ArtifactService
  → PluginManager (optional)
     ↓
  launcher.Launcher
     ↓
  universal.New(console, web)
     ↓
  no args → console.Run()
  "web"   → web.Run()
  other   → unknown-command error
```

Console and web don't know about each other. They only know `launcher.Config`.
Adding a new entrypoint (e.g. gRPC) means implementing `SubLauncher` and
registering it — no changes to runner, flow, or agent code.

### Deploy and Telemetry Pieces: Dry-Run / Simplified

**Deploy plans** (`deploy/cloudrun.go`, `deploy/agentengine.go`) are
intentionally dry-run and deterministic:

- No `gcloud`, `docker`, `tar`, or network calls.
- Every run produces identical output for the same inputs.
- Cloud Run plan generates a distroless Dockerfile with `CMD` that invokes
  the `web` launcher with protocol flags (api, a2a, webui, etc.).
- Agent Engine plan generates a multi-stage Dockerfile (golang builder +
  distroless) and documents the source archive, class methods, environment
  variables, and stream query endpoint.
- Real deployments follow this exact shape; the dry-run plan teaches the
  blueprint without requiring GCP credentials.

**Telemetry** (`telemetry/telemetry.go`, `telemetry/instrumentation.go`) is
in-memory and test-only:

- No OpenTelemetry SDK, GCP exporter, or OTLP dependencies.
- `Recorder` stores spans and logs in memory (thread-safe).
- `StartInvokeAgentSpan`, `StartGenerateContentSpan`, `StartExecuteToolSpan`,
  `StartServerEventSpan` mirror ADK Go's `internal/telemetry` patterns.
- `WithCaptureMessageContent(false)` (default) elides message bodies as
  `<elided>` — mirrors the `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT`
  toggle from ADK Go.
- `Providers` wraps the recorder with `Init`/`Shutdown` lifecycle semantics.

### Relation to Earlier Runtime/Workflow Layers

```
                          ┌──────────────────┐
                          │   launcher.Config │
                          │   (entrypoints)   │
                          └────────┬─────────┘
                                   │
          ┌────────────────────────┼────────────────────────┐
          │                        │                        │
   console (stdin)           web (HTTP)              deploy (plans)
          │                        │                        │
          ▼                        ▼                        ▼
     runner.Run              adkrest.Server         CloudRunPlan
          │                 /run /run_sse        AgentEnginePlan
          ▼
   ┌──────────────┐
   │  runner.Run   │  ←─ telemetry spans/logs wrap invocation
   └──────┬───────┘
          │
          ▼
   Chapters 01-05 layers (agent, flow, tool, workflow, plugin)
```

The runner (`runner.Run`) is the shared core. All entrypoints delegate to it.
Telemetry instruments the runner invocation boundary (agent invoke, model
generate, tool execute) and the server boundary (HTTP requests). Deploy plans
explain how to package and ship this exact runtime on Cloud Run or Agent Engine.

### Intentional Omissions

| Omission | Reason |
|----------|--------|
| Real OpenTelemetry SDK (OTLP, GCP exporters) | Educational scope; the in-memory recorder demonstrates the span/log pattern |
| `gcloud` / `docker` execution | Deploy plans are deterministic dry-run snapshots |
| Web UI (`webui` launcher, `go:embed` bundle) | Requires upstream `google/adk-web` build; not needed for teaching |
| A2A launcher and `adka2a` server | Chapter 05 Remote A2A covers the streaming bridge pattern |
| Agent Engine launcher (class method handler) | Not needed without a real Vertex AI Reasoning Engine instance |
| PubSub / Eventarc triggers | Cloud-specific and orthogonal to the agent runtime pattern |
| `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT` env-based toggling | The `WithCaptureMessageContent` option demonstrates the same pattern |

```bash
go test ./...
go vet ./...
git diff --check
```
