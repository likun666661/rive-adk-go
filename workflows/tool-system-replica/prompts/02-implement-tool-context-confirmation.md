# Node 2: Implement ToolContext, Confirmation, Streaming, Long-Running Semantics

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Add the Chapter 03 execution semantics that make tools hard in practice:
ToolContext, confirmation requests, streaming tools, and long-running markers.

## Required scope

1. Read the Chapter 03 guide's sections on HITL confirmation, streaming
   function tools, and long-running tools.
2. Add a compact ToolContext model that gives tools access to:
   - invocation/session identity;
   - state mutation helpers where appropriate;
   - confirmation status/request records.
3. Add confirmation support:
   - a tool can require confirmation statically or dynamically;
   - first call should produce a structured confirmation-required result/event;
   - confirmed calls should execute;
   - rejected calls should produce a structured rejected result.
4. Add streaming support in the small replica:
   - a streaming tool returns deterministic chunks;
   - non-live mode may collect chunks into a normal function response;
   - errors during streaming become structured tool errors.
5. Add long-running support:
   - tool declarations expose a long-running marker/description;
   - function call results carry enough metadata to demonstrate "do not repeat"
     semantics.
6. Add focused tests for confirmation required/approved/rejected, streaming
   collection, streaming error, and long-running declaration/result metadata.

## Constraints

- Build on Node 1's public surface; do not rewrite the whole tool package.
- Preserve existing flow behavior and tests.
- Edit only under `{{repo_path}}`.
- Run `go test ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/11-tool-context-confirmation.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/11-tool-context-confirmation.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
