## implemented

- `server/adkrest/models.go` — Request/response structs (`RunRequest`, `EventResponse`, `PartResponse`, `ContentResponse`, `ActionsResponse`, etc.) with `EventToResponse` converter that maps `event.Event` to JSON-safe types including function calls, function responses, state deltas, artifact deltas, and tool confirmations.

- `server/adkrest/server.go` — `Server` struct implementing `http.Handler`:
  - `NewServer(cfg *launcher.Config)` constructor validates and wires services from the launcher config; no global singletons.
  - `POST /run` — JSON handler: decodes `RunRequest` (strict `DisallowUnknownFields`), validates required fields, loads agent via `AgentLoader`, creates `runner.Runner` from config services, executes `runner.Run`, returns collected events as JSON array.
  - `POST /run_sse` — SSE handler: same pipeline but streams events with `text/event-stream` content type, each event as `data: <json>\n\n`, errors as `event: error\ndata: <json>\n\n`.
  - `runAgent` shared method encapsulates agent loading, runner creation, and execution in one place.
  - `decodeRunRequest` uses `json.Decoder.DisallowUnknownFields()` for strict parsing.
  - `validateRunRequest` checks all required fields (`appName`, `userId`, `sessionId`, `newMessage`).

- `server/adkrest/serverError` — structured error with HTTP status code and message, implementing `error` interface.

- `cmd/launcher/web/web.go` — `Web` sublauncher implementing `launcher.SubLauncher`:
  - Keyword `"web"`, parses `-port` flag.
  - `Run` creates an `adkrest.Server` from launcher config, starts `http.Server`, handles graceful shutdown on context cancellation.

## files_changed

| file | action |
|---|---|
| `server/adkrest/models.go` | created |
| `server/adkrest/server.go` | created |
| `server/adkrest/server_test.go` | created |
| `cmd/launcher/web/web.go` | created |

No existing files were modified. Chapter 01-05 tests are preserved.

## tests

| test | coverage |
|---|---|
| `TestRunHandlerHappyPath` | JSON POST /run returns 200 with correct EventResponse |
| `TestRunSSEHandlerHappyPath` | SSE POST /run_sse returns 200 with `text/event-stream`, `data:` framing, correct event content |
| `TestRunSSEMultipleEvents` | SSE streams multiple events (partial + final) in correct frame count |
| `TestRunHandlerMissingAgent` | Server without AgentLoader returns 404 |
| `TestMalformedRequestDoesNotCorruptLaterRequests` | Malformed JSON returns 400; subsequent valid request returns 200 (no state corruption) |
| `TestRunHandlerMissingFields` | Missing `newMessage`, `userId`, `sessionId` each return 400; unknown field returns 400 |
| `TestRunHandlerMethodNotAllowed` | GET /run returns 405 |
| `TestSSEErrorEvent` | Agent that returns error produces `event: error` SSE frame with error message |
| `TestSessionReuseAcrossRequests` | Three successive POSTs to same session accumulate state correctly |
| `TestEventConversionFunctionCalls` | `EventToResponse` correctly maps function calls, function responses, state deltas, and escalate actions |

All 10 tests pass. Full project `go test ./...` passes for all 22 packages.

## notes

- The server uses the launcher `Config` directly — no global runtime singletons. Services (session, memory, artifact, agent loader) are injected through the config.
- SSE handler uses `http.Flusher` interface (standard library), avoiding any external SSE library dependency.
- `json.Decoder.DisallowUnknownFields()` is used to reject unknown JSON keys, preventing silent field corruption.
- The `serverError` type carries both HTTP status code and message, avoiding error-mapping logic scattered across handlers.
- The web sublauncher is a minimal `launcher.SubLauncher` implementation; it mounts routes at `/` (all of adkrest server) and can be combined with other sublaunchers via the universal router.
- Zero external dependencies added — all implementation uses `net/http`, `encoding/json`, and existing project packages.
