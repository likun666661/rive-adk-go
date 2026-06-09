# Chapter 06: Launcher CLI Implementation Report

## implemented

1. **`cmd/launcher` package** — Defines stable entrypoint abstractions:
   - `Config` struct carrying `SessionService`, `ArtifactService`, `MemoryService`, `AgentLoader`, and `PluginManager`.
   - `Launcher` interface (`Execute`, `CommandLineSyntax`).
   - `SubLauncher` interface (`Keyword`, `Parse`, `Run`, `CommandLineSyntax`, `SimpleDescription`).
   - `AgentLoader` interface with `RootAgent()` method.

2. **`cmd/launcher/universal` package** — Keyword-based router:
   - Maps first argv token to a `SubLauncher` keyword.
   - First registered sublauncher is the default when no keyword matches.
   - Unknown commands produce an error listing available keywords.
   - Duplicate keyword registration is detected and rejected.
   - `CommandLineSyntax()` generates a formatted help string.

3. **`cmd/launcher/console` package** — Console sublauncher:
   - Configurable `io.Reader`/`io.Writer` for testability (defaults to `os.Stdin`/`os.Stdout`).
   - Auto-creates `InMemorySessionService` if none provided.
   - Loads root agent via `AgentLoader`, ensures it implements `runner.ExecutableAgent`.
   - Creates a `runner.Runner` and drives `runner.Run` for each input line.
   - Prints events with role, text, function calls, and responses.
   - Persists non-partial events through the existing runner/session chain.

## files_changed

| File | Status | Description |
|------|--------|-------------|
| `cmd/launcher/launcher.go` | created | Config, Launcher, SubLauncher, AgentLoader |
| `cmd/launcher/universal/universal.go` | created | Keyword router (universal launcher) |
| `cmd/launcher/console/console.go` | created | Console sublauncher driving runner.Run |
| `cmd/launcher/launcher_test.go` | created | Interface compliance, config, sublauncher tests |
| `cmd/launcher/universal/universal_test.go` | created | Routing, default, unknown command, error propagation |
| `cmd/launcher/console/console_test.go` | created | Console execution, session persistence, tool chain |

## tests

All existing and new tests pass (`go test -count=1 ./...` — 20 packages, all OK):

- **Default sublauncher selection**: No-arg invocation routes to first registered sublauncher.
- **Keyword-based routing**: `"web"` keyword selects web sublauncher; console is not called.
- **Keyword routing with args**: Remaining args after keyword are passed to the chosen sublauncher's `Parse`.
- **Unknown command errors**: `"unknown"` produces error referencing the bad keyword and available sublaunchers.
- **No sublaunchers error**: Empty sublauncher list produces error.
- **Duplicate keywords error**: Two sublaunchers with the same keyword are rejected.
- **Console execution**: Scripted input drives `runner.Run` and produces formatted output.
- **Console tool chain**: Full model(fc) -> tool -> model(final) cycle persists to session.
- **Console session persistence**: Events (user + model + tool + model) are persisted in session.
- **Console empty line skipping**: Empty input lines are silently skipped.
- **Console multiple messages**: Session accumulates events across consecutive runs.
- **Console with services**: Memory and artifact services are plumbed through.
- **Console default session service**: Falls back to `InMemorySessionService` when none provided.
- **Interface compliance**: Both `Launcher` and `SubLauncher` are compile-time checked.

## notes

- The implementation follows the ADK Go design pattern of `Config -> Launcher -> SubLauncher` routing without copying ADK Go source.
- The universal router's "first sublauncher as default" behavior matches the Chapter 06 guide's description that console is the default entrypoint.
- Unknown commands produce actionable errors (listing available keywords) rather than silently falling through to the default — this is intentional for CLI usability.
- The console sublauncher uses `io.Reader`/`io.Writer` injection for testability without requiring real stdin/stdout.
- Chapters 01-05 tests remain unchanged and continue to pass.
- No cobra, flag sets, or real terminal streaming are added — the implementation stays compact and educational.
