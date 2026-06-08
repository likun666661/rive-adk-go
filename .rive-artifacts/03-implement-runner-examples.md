# Node 3: Implement Runner, Examples, Integration Tests

## implemented

1. **`runner` package** (`runner/runner.go`): Top-level orchestrator implementing the full chain:
   - `Runner.Run`: session Get/Create → append user event → build InvocationContext → agent.Execute → persist non-partial events → return session + events
   - `ExecutableAgent` interface: extends `agent.Agent` with `Execute` method
   - `SessionService` interface: abstracts session retrieval and creation
   - `InMemorySessionService`: thread-safe in-memory implementation

2. **`llmagent` package** (`llmagent/llmagent.go`): Minimal LLM agent wrapping `flow.Flow`:
   - `New(name, description, flow)`: creates an agent whose Run function type-asserts to `context.InvocationContext` and delegates to `flow.Flow.Run`

3. **Demo** (`cmd/demo/main.go`): End-to-end example showing:
   - User message: "What's the weather in Tokyo?"
   - Fake model emits function call for `get_weather` tool
   - Tool returns weather data (22°C, sunny, 45% humidity)
   - Fake model emits final text response
   - Session persists 4 events: user → model(fc) → tool(result) → model(final)

4. **Integration tests** (`runner/runner_test.go`): 9 tests covering:
   - Runner validation (empty config)
   - Simple text-only run with session persistence verification
   - Tool call + final response chain (3 agent events, 4 session events)
   - Auto-creation of sessions when not found
   - Session reuse across multiple Run calls
   - Partial events are yielded but NOT persisted
   - Session isolation across different users
   - State delta from tool persists in session state
   - After-agent callback with EndInvocation

5. **README.md**: Updated with architecture diagram, package listing, quick start code, demo instructions, and test command.

## files_changed

| File | Action |
|------|--------|
| `runner/runner.go` | Created - Runner, SessionService, InMemorySessionService |
| `runner/runner_test.go` | Created - 9 integration tests |
| `llmagent/llmagent.go` | Created - Minimal LLM agent wrapper |
| `cmd/demo/main.go` | Created - End-to-end demo |
| `README.md` | Updated - Architecture, usage, package docs |

## tests

```
$ go test ./...
ok  	github.com/likun666661/rive-adk-go/agent	(cached)
ok  	github.com/likun666661/rive-adk-go/event	(cached)
ok  	github.com/likun666661/rive-adk-go/flow	(cached)
ok  	github.com/likun666661/rive-adk-go/model	(cached)
ok  	github.com/likun666661/rive-adk-go/runner	... 9 tests pass
ok  	github.com/likun666661/rive-adk-go/session	(cached)
ok  	github.com/likun666661/rive-adk-go/tool	(cached)
```

All 9 new integration tests pass. No existing tests were broken.

## notes

- The `ExecutableAgent` interface in the runner package uses structural typing — `*agent.baseAgent` satisfies it without modifications to the `agent` package, preserving the design from Nodes 1 and 2.
- `llmagent.New` performs a type assertion from `agent.InvocationContext` to `context.InvocationContext` inside the Run function, matching the pattern from the flow tests.
- The demo produces exactly the expected flow: user message → fake model emits tool call → tool returns result → fake model emits final answer → session contains user/model/tool/final events.
