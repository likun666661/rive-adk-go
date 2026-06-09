# Node 2: Implement AgentTool Sandbox

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Implement the Chapter 05 "agent as tool" idea: wrap an existing agent as a tool
that can be called from a parent flow while running in an isolated child
session.

## Required scope

1. Read the Chapter 05 guide and ADK Go `tool/agenttool` reference.
2. Add a compact `AgentTool` or equivalent package that:
   - exposes an agent as a normal tool declaration/call;
   - creates an isolated child session for the delegated agent;
   - copies non-internal parent state into the child session;
   - runs the child agent through the existing runner/flow abstractions;
   - returns a concise tool result that can either summarize child output or
     skip summarization and pass through the final text.
3. Add tests for:
   - child session isolation from parent session;
   - parent non-internal state copied into child;
   - internal/private state not copied;
   - returned result contains child output;
   - integration with existing tool execution path.

## Constraints

- Build on the workflow-agent changes from node 1.
- Edit only under `{{repo_path}}`.
- Preserve Chapter 01-04 tests.
- Run `go test ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/19-agent-tool-sandbox.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/19-agent-tool-sandbox.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
