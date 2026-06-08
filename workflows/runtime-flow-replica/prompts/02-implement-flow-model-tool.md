# Node 2: Implement Flow, Model, Tool Loop

You are an OpenCode worker inside a Rive workflow.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Implement the core `Flow.Run` / `runOneStep` replica around the contracts from
Node 1. This should demonstrate how a model response can produce either a final
assistant event or tool calls that continue the loop.

## Required scope

1. Add a `model` package with a deterministic fake model interface suitable for
   tests.
2. Add a `tool` package with function tools, tool call/result structs, and error
   handling.
3. Add a `flow` package that supports:
   - request processor hooks;
   - model callback hooks;
   - response processor hooks;
   - parallel execution of multiple tool calls;
   - state delta merge from tool results;
   - loop termination when an event is final.
4. Add tests covering:
   - final model response;
   - model response with one tool call followed by final response;
   - multiple tool calls executing and merging results deterministically;
   - processor/callback ordering;
   - tool error becomes an event/error result, not a silent success.

## Constraints

- Build on the current repo state; do not replace Node 1's public contracts
  unless tests prove the change is necessary.
- Edit only under `{{repo_path}}`.
- Run `go test ./...`.
- Write a short report to `{{repo_path}}/.rive-artifacts/02-implement-flow-model-tool.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/02-implement-flow-model-tool.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.

