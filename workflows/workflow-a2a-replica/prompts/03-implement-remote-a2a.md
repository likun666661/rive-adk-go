# Node 3: Implement Lightweight Remote A2A

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Implement a small educational Remote A2A bridge that shows how local agent
events can be converted to a remote protocol stream and then back into local
session events. This is a teaching model, not a network-compatible ADK A2A
implementation.

## Required scope

1. Read the Chapter 05 guide and ADK Go remote A2A v2 reference.
2. Add a compact package or set of types for:
   - remote agent card / identity metadata;
   - a client interface with streaming event calls;
   - conversion between local `session.Event` and a small remote event model;
   - partial artifact/text aggregation with a terminal flush;
   - cleanup/cancel callback semantics for failed or abandoned remote tasks.
3. Use in-memory fake clients for tests. Do not introduce real network
   dependencies unless already present in this repo.
4. Add tests for:
   - remote message/event conversion;
   - streaming partial aggregation and final flush;
   - error propagation;
   - cleanup callback invocation.

## Constraints

- Build on the workflow-agent changes from node 1 where useful.
- Edit only under `{{repo_path}}`.
- Preserve Chapter 01-04 tests.
- Run `go test ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/20-remote-a2a.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/20-remote-a2a.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
