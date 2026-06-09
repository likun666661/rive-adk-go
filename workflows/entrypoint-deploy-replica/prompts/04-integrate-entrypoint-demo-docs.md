# Node 4: Integrate Entrypoint Demo, Docs, and Tests

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Integrate Chapter 06 launcher/server/deploy/telemetry into the project demo and
README so the replica teaches how one runtime can be exposed through local CLI,
HTTP server, dry-run deploy plans, and telemetry.

## Required scope

1. Read reports `.rive-artifacts/23..25` and the Chapter 06 guide.
2. Add or update end-to-end tests demonstrating:
   - launcher console path;
   - web/server JSON path;
   - SSE encoding path;
   - deploy dry-run plan generation;
   - telemetry capture around a runner invocation.
3. Update `cmd/demo` with a Chapter 06 section.
4. Update `README.md` with:
   - what Chapter 06 adds;
   - how launcher config keeps entrypoints stable;
   - what deploy/telemetry pieces are intentionally dry-run/simplified;
   - relation to earlier runtime/workflow layers.
5. Run full verification:
   - `go test ./...`
   - `go vet ./...`
   - `git diff --check`
   - `go run ./cmd/demo`

## Constraints

- Edit only under `{{repo_path}}`.
- Keep demo deterministic and offline.
- Write a report to `{{repo_path}}/.rive-artifacts/26-entrypoint-demo-docs.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/26-entrypoint-demo-docs.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
