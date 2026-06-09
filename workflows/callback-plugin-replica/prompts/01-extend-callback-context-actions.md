# Node 1: Extend Callback Context and Event Actions

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Extend the Chapter 01-03 replica with the callback-context ideas from Chapter
04: callback-friendly readonly context, write-through state action tracking, and
artifact delta tracking.

## Required scope

1. Read the Chapter 04 guide and relevant ADK Go callback context files. Use the
   source as design reference only.
2. Add a compact callback context surface that exposes:
   - readonly identity/session/user/app/branch information;
   - state reads/writes that record `EventActions.StateDelta`;
   - artifact save tracking that records an artifact delta or similar metadata;
   - access to memory/artifact services already introduced in Chapter 02.
3. Keep existing `agent.BeforeAgentCallback`, `flow.BeforeModelCallback`,
   `flow.BeforeToolCallback`, and after-callback APIs working.
4. Add tests for:
   - callback state write-through and delta recording;
   - callback reads seeing local delta before durable state;
   - artifact save tracking records artifact metadata on the event action;
   - early-exit callback behavior remains deterministic.

## Constraints

- Edit only under `{{repo_path}}`.
- Do not copy ADK Go source; keep this an educational replica.
- Preserve Chapter 01-03 tests.
- Run `go test ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/14-callback-context-actions.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/14-callback-context-actions.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
