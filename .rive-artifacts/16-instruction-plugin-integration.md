# Node 3: Integrate Instruction Processors, Docs, and Demo — Report

## implemented

### 1. Instruction Processor Layer (`instruction/` package)

- **`instruction/instruction.go`**: Core instruction processing module with:
  - `Config` struct holding all instruction parameters: static `Instruction`, dynamic `InstructionProvider`, `GlobalInstruction`, `GlobalInstructionProvider`, `IsRootAgent` discriminator, and `ReadonlyState` factory for template injection.
  - `Provider` type: `func(ReadonlyContext) (string, error)` — dynamic instruction provider receiving readonly execution context.
  - `InjectSessionState(template, state)` — regex-based `{placeholder}` resolver supporting `{key}`, `{key?}`, `{app:key}`, `{user:key}`, `{temp:key}` patterns against merged session state.
  - `NewRequestProcessor(cfg)` — creates an instruction processor that injects `SystemInstruction` into `model.LLMRequest`. Execution order: GlobalInstruction → GlobalInstructionProvider → Instruction → InstructionProvider → template injection.
  - `MergeStateView(app, user, session)` — builds a merged `ReadonlyState` for template injection.

- **`instruction/adapter.go`**: `ToRequestProcessor` adapter bridging instruction processors to `flow.RequestProcessor` signature. Defines local `ReadonlyContext` interface to avoid import cycles with `callbackctx`.

### 2. Integration into Flow Pipeline

- **`model/model.go`**: Added `SystemInstruction string` field to `LLMRequest`.
- Instruction processors are wired as `flow.RequestProcessor` entries via `instruction.ToRequestProcessor()`.
- The processor runs during `flow.preprocess()` before tool declaration injection and model callbacks.

### 3. Demo Scenarios (`cmd/demo/main.go`)

- **Demo 4.1 — Plugin Logging/Observability**: Registers a `plugin.Plugin` with `BeforeModel`, `AfterModel`, `BeforeTool`, and `AfterTool` hooks. All hooks return `nil, nil` (pure observer pattern). Logs model, instructions, tool names, args, and results.
- **Demo 4.2 — Before-Model Cache/Mock Response Early-Exit**: A cache plugin checks `ctx.UserContent()` in `BeforeModel` and returns a cached `LLMResponse`, bypassing the LLM call. Demonstrates early-exit semantics: first non-nil result stops the chain.
- **Demo 4.3 — Callback State Mutation & Instruction Interpolation**: Pre-populates session state via `SessionService.Create`, then a `RequestProcessor` reads state and builds a `SystemInstruction` string. Demonstrates the instruction interpolation pipeline from state to prompt.
- **Demo 4.4 — Plugin Ordering Relative to Direct Callbacks**: Registers two plugins (plugin-a, plugin-b) and direct callbacks. Execution order output shows: plugin-a:beforeModel → plugin-b:beforeModel → direct:beforeModel-1 → plugin-a:afterModel → plugin-b:afterModel → direct:afterModel-1. Confirms plugins always run before direct callbacks.

### 4. README.md Update

Added comprehensive Chapter 04 section covering:
- Why three extension layers exist (Instruction, Callback, Plugin)
- Instruction processor API (static, dynamic, global, placeholder injection)
- Plugin layer semantics (ordered execution, early exit, nil skip, immediate error)
- Plugin vs Callback comparison table
- Why hook ordering and early-exit matter (four scenarios)
- What's implemented and intentional omissions table
- Quick-start code examples

### 5. Integration Tests (`runner/chapter04_test.go`)

10 new tests covering the full runner/flow path:
- Test 24: Instruction processor injects SystemInstruction
- Test 25: Dynamic instruction provider sees UserContent
- Test 26: Global instruction applied only for root agent
- Test 27: Template injection from session state (app:, user:, session scopes)
- Test 28: Plugin before-model early exit (cache/mock)
- Test 29: Plugin ordering — plugins before direct callbacks
- Test 30: Plugin after-model transforms response
- Test 31: Full chain — instruction + plugin + callback + state + tool
- Test 32: Global instruction NOT applied for non-root
- Test 33: Plugin before-tool early exit bypasses tool execution

## files_changed

| File | Action | Description |
|------|--------|-------------|
| `model/model.go` | Modified | Added `SystemInstruction string` field to `LLMRequest` |
| `instruction/instruction.go` | Created | Core instruction processing: Config, Provider, InjectSessionState, NewRequestProcessor, MergeStateView |
| `instruction/adapter.go` | Created | ToRequestProcessor adapter, ReadonlyContext interface |
| `instruction/instruction_test.go` | Created | 8 unit tests for InjectSessionState and NewRequestProcessor |
| `cmd/demo/main.go` | Modified | Added `runChapter04()` with 4 demo scenarios (logging, cache, interpolation, ordering) |
| `runner/chapter04_test.go` | Created | 10 integration tests for full runner/flow path |
| `README.md` | Modified | Added Chapter 04 section with architecture, API, and omissions |

## tests

```
$ go test ./...
ok  agent, artifact, context, event, flow, instruction, llmagent, memory, model, plugin, runner, session, tool

$ go vet ./...
(no output)
```

All 33 pre-existing tests continue to pass. 18 new tests added (8 instruction unit + 10 runner integration).

## notes

- **Deterministic and stdlib-only**: All instruction processing uses Go stdlib `regexp`. No external dependencies added.
- **Preserves existing demos and tests**: Chapters 01-03 demos and all existing tests pass unchanged.
- **Plugin BeforeAgent in demo 4.3**: The demo uses manual state pre-population via `SessionService.Create` rather than plugin `BeforeAgent` hooks because the standard `llmagent.New` → `agent.Config.Run` path does not invoke the context-aware `RunWithCallbackContext` lifecycle. Plugin BeforeAgent hooks work correctly when using `context.RunWithCallbackContext`.
- **Local ReadonlyContext interface**: Defined in `instruction/adapter.go` to avoid import cycles between `instruction` and `callbackctx` packages. Identical in structure to `callbackctx.ReadonlyContext`.
- **Template injection scope**: `InjectSessionState` requires a merged (app+user+session) state view. `MergeStateView` helper builds this. For runtime use, `instruction.Config.ReadonlyState` provides the factory.
- **Global instruction semantics**: Only applied when `IsRootAgent() == true`, matching the upstream ADK Go behavior where global instructions are root-agent-only.
