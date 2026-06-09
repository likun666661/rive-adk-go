# Node 4: Integrate Entrypoint Demo, Docs, and Tests

## implemented

1. **End-to-end integration tests** (`cmd/demo/main_test.go`) — 10 tests covering:
   - Launcher console path (`TestEndToEndLauncherConsolePath`): scripted stdin → runner.Run → session persistence
   - Web/server JSON path (`TestEndToEndWebJSONPath`): HTTP POST /run → JSON response → event validation
   - SSE encoding path (`TestEndToEndSSEEncodingPath`): HTTP POST /run_sse → text/event-stream → data: framing → JSON unmarshal per frame
   - Deploy dry-run plan generation (`TestEndToEndDeployDryRunPlan`): CloudRunPlan and AgentEnginePlan with field, Dockerfile, build/proxy command, and line verification
   - Telemetry capture around runner invocation (`TestEndToEndTelemetryCaptureRunnerInvocation`): spans (invoke_agent, generate_content, server event) + logs (system, user, choice, server) with token usage and content capture verification
   - Universal launcher routing (`TestEndToEndUniversalLauncherRouting`): no-args → default (console), keyword → console, unknown keyword → error
   - Telemetry content capture toggle (`TestEndToEndTelemetryContentCaptureToggle`): capture on → visible, capture off → `<elided>`
   - Providers lifecycle with runner invocation (`TestEndToEndProvidersLifecycle`): Init → record → Shutdown → data preserved
   - Multi-protocol deploy plan (`TestEndToEndMultiProtocolDeployPlan`): all 6 protocols in Dockerfile
   - Web session persistence (`TestEndToEndWebSessionPersistence`): 3 successive POSTs reuse session state

2. **Chapter 06 demo section** (`cmd/demo/main.go`):
   - `runChapter06()` entry point with three sub-demos
   - Demo 6.1 — Launcher Config: documents launcher.Config fields, SubLauncher interface, universal routing table
   - Demo 6.2 — Deploy Plans: generates CloudRunPlan (distroless Dockerfile, build command, proxy command) and AgentEnginePlan (multi-stage Dockerfile, source archive, class methods, stream URL)
   - Demo 6.3 — Telemetry: records invoke_agent, generate_content, server spans + system/user/choice/server logs around a runner invocation

3. **README.md updates**:
   - Added Chapter 06 to architecture bullet list
   - Extended packages table with `cmd/launcher`, `cmd/launcher/universal`, `cmd/launcher/console`, `cmd/launcher/web`, `server/adkrest`, `deploy`, `telemetry`
   - Updated demo output section to include Chapter 06
   - Added full Chapter 06 section covering: what it adds, how launcher config keeps entrypoints stable, deploy/telemetry dry-run simplifications, architecture diagram, relation to earlier layers, and intentional omissions

## files_changed

| File | Status | Description |
|------|--------|-------------|
| `cmd/demo/main_test.go` | created | 10 e2e integration tests for Chapter 06 features |
| `cmd/demo/main.go` | modified | Added runChapter06() with 3 demos + imports (deploy, telemetry) |
| `README.md` | modified | Chapter 06 section, extended packages table, updated demo output |

## tests

All 24 packages pass (`go test ./...`):

- **Launcher console path**: Full MDN(model→tool→model) chain through console stdin → session persistence (4 events persisted)
- **Web JSON path**: POST /run → 200 → Content-Type application/json → correct event response
- **SSE encoding path**: POST /run_sse → text/event-stream → 3 data frames → valid JSON per frame
- **Deploy Cloud Run plan**: Dockerfile contains all 3 protocols (api, a2a, webui) + build command + proxy command
- **Deploy Agent Engine plan**: Multi-stage Dockerfile + class methods + stream URL + env/secrets
- **Telemetry capture**: 3 spans (invoke_agent, generate_content, server) + 4 logs (server.request, gen_ai.system.message, gen_ai.user.message, gen_ai.choice) with token usage verification
- **Content capture toggle**: WithCaptureMessageContent(true) shows content; default shows `<elided>`
- **Providers lifecycle**: Init → record → Shutdown preserves data
- **All 5 protocols**: Dockerfile validates api, a2a, webui, pubsub, eventarc
- **Session persistence**: 3 consecutive POSTs reuse and accumulate session state

`go vet ./...` — zero warnings  
`git diff --check` — no whitespace issues  
`go run ./cmd/demo` — all 6 chapters run to completion

## notes

- The implementation builds on work from nodes 23 (launcher CLI), 24 (REST server), and 25 (deploy/telemetry) — the e2e tests wire these pieces together into integration scenarios.
- All tests are deterministic and offline — no gcloud, Docker, network, or external dependencies.
- The `agentLoader` helper reuses the same pattern from `cmd/launcher/console/console_test.go`.
- Demo 6.1 is purely educational (documents the architecture); Demos 6.2 and 6.3 produce runnable output.
- The e2e `TestEndToEndTelemetryCaptureRunnerInvocation` exercises the full telemetry lifecycle around `runner.Run`, verifying spans and logs are recorded correctly.
- Content capture toggle (`WithCaptureMessageContent`) mirrors the real ADK Go `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT` pattern as described in the Chapter 06 guide.
- No existing tests were modified — Chapters 01-05 continue to pass unchanged.
