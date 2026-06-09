# Node 3: Integrate Tool System With Flow, Docs, and Demos

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Make Chapter 03 visible from the runtime surface: model request declaration
injection, flow tool execution, confirmation, streaming, long-running metadata,
and documentation.

## Required scope

1. Integrate tool declarations/toolsets into `flow` and/or `model.LLMRequest`
   so tests can inspect what the model would see.
2. Extend or add demos showing:
   - allowed tool filtering;
   - a confirmed tool call;
   - a rejected confirmation path;
   - a streaming tool collected in non-live mode;
   - long-running tool metadata.
3. Update `README.md` with a Chapter 03 section:
   - problem: many tool sources must look uniform to the LLM flow;
   - why hard: schema, args/results, streaming, confirmation, long-running,
     external toolsets;
   - what this replica implements vs intentionally omits.
4. Add integration tests that run through the full
   `Runner -> Agent -> Flow -> Tool -> Event -> Session` path for at least one
   confirmation case and one streaming/long-running case.

## Constraints

- Keep package boundaries understandable; do not introduce a second flow engine.
- Keep tests deterministic and stdlib-only.
- Edit only under `{{repo_path}}`.
- Run `go test ./...` and `go vet ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/12-tool-system-integration.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/12-tool-system-integration.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
