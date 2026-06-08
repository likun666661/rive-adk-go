# rive-adk-go

A small Go replica of the Google ADK Go runtime flow described by
`01-runtime-flow-deep-dive.md`.

The target is not API compatibility with `google/adk-go`. It is an educational
runtime skeleton that preserves the Chapter 01 architecture line:

```text
Runner -> Agent -> LLM Flow -> Model/Tool -> Event -> Session
```

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
| `session` | Session state and append-only event history |
| `context` | Invocation context carrying agent, session, and lifecycle control |

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
    // Step 1: function call
    model.FunctionCallResponse("Let me check...",
        event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
    ),
    // Step 2: final text
    model.TextResponse("Tokyo is 22°C."),
)

// 3. Wire up the Flow
f := &flow.Flow{
    Model: fakeModel,
    Tools: map[string]tool.FunctionTool{"get_weather": weatherTool},
}

// 4. Create the LLM agent
ag, _ := llmagent.New("weather_bot", "Answers weather questions.", f)

// 5. Create runner with in-memory session service
r, _ := runner.New(runner.Config{
    AppName:        "my_app",
    Agent:          ag.(runner.ExecutableAgent),
    SessionService: runner.NewInMemorySessionService(),
})

// 6. Run
sess, events, _ := r.Run(context.Background(), "user-1", "sess-1", "Weather in Tokyo?")
// sess contains 4 events: user → model(fc) → tool(result) → model(final)
```

## Demo

```bash
go run ./cmd/demo
```

Output shows the complete chain: user message, model function call, tool
execution, final model response, and session persistence.

## Intentionally Omitted

This is a compact replica of the Chapter 01 runtime loop, not an ADK Go fork.
The first cut intentionally omits:

- streaming and live bidirectional sessions;
- multi-agent trees and transfer resolution;
- plugin managers, telemetry, artifacts, memory, and auth;
- request-history reconstruction from session events;
- long-running tools, tool confirmation, and toolsets;
- external model clients or persistent storage backends.

The omitted pieces are represented as small interfaces, callbacks, or event
fields where they help explain the architecture without copying ADK Go.

## Verification

```bash
go test ./...
go vet ./...
git diff --check
```
