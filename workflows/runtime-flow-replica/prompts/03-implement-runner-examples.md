# Node 3: Implement Runner, Examples, Integration Tests

You are an OpenCode worker inside a Rive workflow.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Implement the `Runner -> Agent -> Flow -> Event -> Session` chain and add a
small executable example that proves the runtime flow works end to end.

## Required scope

1. Add a `runner` package that:
   - creates or retrieves a session;
   - appends user input as a session event;
   - invokes the selected agent;
   - persists non-partial events;
   - yields events to callers.
2. Add a minimal LLM agent implementation that wraps `flow.Flow`.
3. Add a small example or `cmd/demo` showing:
   - user message;
   - fake model emits a tool call;
   - tool returns a result;
   - fake model emits final answer;
   - session contains user/model/tool/final events.
4. Add integration tests for the complete chain and for session persistence.
5. Update `README.md` with usage and architecture notes.

## Constraints

- Build on Nodes 1 and 2; do not rewrite their design wholesale.
- Edit only under `{{repo_path}}`.
- Run `go test ./...`.
- If there is a demo command, run it once.
- Write a short report to `{{repo_path}}/.rive-artifacts/03-implement-runner-examples.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/03-implement-runner-examples.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.

