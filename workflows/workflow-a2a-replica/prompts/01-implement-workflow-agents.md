# Node 1: Implement Workflow Agents

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Extend the Chapter 01-04 educational replica with workflow-agent orchestration:
sequential, parallel, and loop agents that compose existing `agent.Agent`
instances through the same runner/event/session abstractions.

## Required scope

1. Read the Chapter 05 guide and relevant ADK Go workflow agent code. Use the
   source as design reference only; do not copy ADK Go source.
2. Add a compact workflow package or equivalent surfaces for:
   - sequential agent: runs sub-agents in order and forwards event streams;
   - parallel agent: runs independent sub-agents concurrently, forwards events
     deterministically enough for tests, and records branch metadata;
   - loop agent: repeats child agents until max iterations or an escalate/stop
     signal represented in this replica's event/action model.
3. Preserve existing Chapter 01-04 APIs and tests.
4. Keep the design small but explain the hard parts in comments/tests:
   state sharing vs branch isolation, event stream aggregation, error
   propagation, and backpressure simplification.
5. Add focused tests for:
   - sequential order;
   - parallel branch/event aggregation;
   - loop max iteration and early stop;
   - error propagation.

## Constraints

- Edit only under `{{repo_path}}`.
- Keep this an educational Go replica, not an API-compatible ADK fork.
- Run `go test ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/18-workflow-agents.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/18-workflow-agents.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
