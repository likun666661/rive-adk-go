# Node 2: Implement REST and SSE Server

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Add a compact Chapter 06 server layer that exposes the runtime via HTTP JSON and
SSE-style event streams while reusing the launcher config and existing runner.

## Required scope

1. Read the Chapter 06 guide and ADK Go `server/adkrest` / web launcher
   reference.
2. Add a small `server` or `adkrest` package with:
   - run request/response structs;
   - JSON handler that loads an agent and returns collected events;
   - SSE handler or encoder that streams events as `data: <json>\\n\\n`;
   - explicit error mapping for missing agent/session/decode failures;
   - no global runtime singletons: use launcher config/services.
3. Integrate a web sublauncher or helper enough for tests/demo to mount routes.
4. Add tests for:
   - JSON run happy path;
   - SSE framing;
   - missing agent/session error;
   - malformed request error does not corrupt later requests.

## Constraints

- Build on node 1 launcher config.
- Edit only under `{{repo_path}}`.
- Preserve Chapter 01-05 tests.
- Run `go test ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/24-rest-server.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/24-rest-server.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
