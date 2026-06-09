# Node 3: Deploy Plan and Telemetry Instrumentation Model

## implemented

### deploy package (`deploy/`)

- **`deploy/deploy.go`** — Core types and validators:
  - `Protocol` type with constants (`api`, `a2a`, `webui`, `agentengine`, `pubsub`, `eventarc`)
  - `Plan` interface with `String()` and `Lines()` methods
  - `ValidationError` for collecting multiple validation failures
  - `ValidateEntryPoint`, `ValidateProjectName`, `ValidateRegion`, `ValidateServiceName`, `ValidateServerPort`, `ValidateProtocols` — individual validators
  - `ValidateAll` — runs multiple validators and aggregates errors

- **`deploy/cloudrun.go`** — Cloud Run dry-run plan:
  - `CloudRunConfig` / `CloudRunPlan` — input/output types
  - `PlanCloudRun()` — validates inputs, produces deterministic plan with:
    - Linux build target (CGO_ENABLED=0 GOOS=linux GOARCH=amd64)
    - Distroless Dockerfile with CMD containing `web` launcher and enabled protocol flags
    - gcloud run deploy command, secrets (GOOGLE_API_KEY), proxy command
    - Access URLs (REST API, Web UI)
  - Zero external command execution — all output is deterministic strings

- **`deploy/agentengine.go`** — Agent Engine dry-run plan:
  - `AgentEngineConfig` / `AgentEnginePlan` — input/output types
  - `ClassMethod` type for Reasoning Engine class method metadata
  - `PlanAgentEngine()` — validates inputs, produces deterministic plan with:
    - Multi-stage Dockerfile (golang:1.24 builder + distroless)
    - Source archive command (tar with exclusions)
    - Class methods (custom or default 5-method list)
    - Environment variables and secrets
    - Stream query endpoint URL
    - Optional memory bank configuration

### telemetry package (`telemetry/`)

- **`telemetry/telemetry.go`** — In-memory recorder and provider model:
  - `SpanRecord` / `LogRecord` — in-memory record types for inspection
  - `Recorder` — thread-safe span/log accumulator with `Spans()`, `Logs()`, `SpanCount()`, `LogCount()`, `Reset()`
  - `Option` / `WithCaptureMessageContent` — configuration model (mirrors ADK Go's option pattern)
  - `Providers` — lifecycle wrapper with `Init()` / `Shutdown()` semantics
  - `DefaultRecorder` / `SetDefaultRecorder` — package-level shared instance

- **`telemetry/instrumentation.go`** — Instrumentation helpers (mirroring `google.golang.org/adk/internal/telemetry`):
  - `StartInvokeAgentSpan` — agent invocation span with name, description, session ID, invocation ID
  - `StartGenerateContentSpan` — model generation span with model name, invocation ID
  - `StartExecuteToolSpan` — tool execution span with tool name, serialized args
  - `StartServerEventSpan` — server event span with operation and path
  - `StartedSpan` — in-progress span with `End()`, `EndWithError()`, `SetAttribute()`
  - `SetTokenUsage` — records prompt/candidate/cached/thoughts token counts
  - `SetEventID` — records event ID on a span
  - `LogRequest` — system + user message logs with content capture toggle
  - `LogResponse` — model response log with finish reason, tool calls
  - `LogServerEvent` — HTTP request/reply log with method, path, status, duration
  - `Drain` / `FlushSpanCount` / `FlushLogCount` — explicit flush/clear semantics

## files_changed

- `deploy/deploy.go` (new)
- `deploy/cloudrun.go` (new)
- `deploy/agentengine.go` (new)
- `deploy/deploy_test.go` (new)
- `telemetry/telemetry.go` (new)
- `telemetry/instrumentation.go` (new)
- `telemetry/telemetry_test.go` (new)

## tests

### deploy tests (all passing)

- `TestCloudRunPlanDeterministic` — verifies every field, Dockerfile content, build/proxy commands, lines output
- `TestCloudRunPlanAllProtocols` — all 5 protocols reflected in Dockerfile
- `TestCloudRunPlanDefaults` — default port values applied
- `TestAgentEnginePlanDeterministic` — verifies fields, Dockerfile, build command, stream URL, lines
- `TestAgentEnginePlanDefaultClassMethods` — default 5 class methods listed when none provided
- `TestAgentEnginePlanWithMemoryBank` — memory bank section rendered
- `TestValidateEntryPoint` — 6 cases (empty, whitespace, valid, no-go-ext, dir)
- `TestValidateProjectName` — empty/whitespace/valid
- `TestValidateRegion` — empty/valid
- `TestValidateServiceName` — empty/valid
- `TestValidateProtocols` — nil/empty/valid/unknown
- `TestValidateServerPort` — 7 cases (0, -1, overflow, 1, 80, 8080, 65535)
- `TestValidateAll` — aggregate error contains both failures
- `TestValidateAllPasses` — aggregate succeeds when all pass
- `TestStripExtension` — 4 cases
- `TestCloudRunPlanValidationErrors` — multiple validation errors from empty config
- `TestAgentEnginePlanValidationErrors` — multiple validation errors from empty config
- `TestValidationErrorSingle` / `TestValidationErrorMultiple` — error formatting

### telemetry tests (all passing)

- `TestRecorderSpanRecording` — 3 span types with attribute verification (agent name, session ID, invocation ID, model, tokens, tool args)
- `TestSpanErrorRecording` — ERROR status and error message recorded
- `TestServerEventSpan` — server operation and path attributes
- `TestLogRequest` — system + user messages with content capture
- `TestLogRequestElided` — message content elided when capture off
- `TestLogResponse` — finish reason, content, tool calls in response log
- `TestLogResponseElided` — content elided in response log
- `TestLogServerEvent` — HTTP method, path, status code, duration
- `TestProvidersInitShutdown` — full lifecycle flow
- `TestProvidersShutdownKeepsData` — data inspectable after shutdown
- `TestDefaultRecorder` — package-level default recorder works
- `TestRecorderConcurrency` — 10 goroutines x 100 iterations, no data loss
- `TestDrain` — Reset clears all data
- `TestSpanTimestamps` — chronological ordering verified

## notes

- Zero external dependencies — no gcloud, Docker, OpenTelemetry, or GCP SDK imports
- Builds on existing launcher/server patterns (Chapter 01-05 tests preserved — full `go test ./...` passes all 23 packages)
- Deploy plans are purely educational/deterministic — every run produces identical output for the same inputs
- Telemetry recorder is in-memory and thread-safe, designed for test inspection
- Content capture toggle (`WithCaptureMessageContent`) mirrors the ADK Go `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT` pattern
- Class methods list mirrors the `server/agentengine.ListClassMethods()` pattern described in Chapter 06
