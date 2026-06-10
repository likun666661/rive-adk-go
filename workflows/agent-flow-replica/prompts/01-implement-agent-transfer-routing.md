# Node 1: Implement Agent Transfer Routing

You are an OpenCode implementation worker inside a Rive workflow.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Implement the Chapter 07 agent tree / transfer-to-agent / active-agent routing
slice in the `rive-adk-go` educational replica.

This is not API compatibility with Google ADK Go. Keep the implementation small,
typed, and aligned with the existing Chapter 01-06 style.

If a previous failed attempt already left partial transfer-routing code in the
workspace, do not restart from scratch. Inspect it, fix gaps, verify the
acceptance criteria, then report through the Rive ledger.

## Read first

Read these files before editing:

- `{{chapter_path}}`
- `{{repo_path}}/README.md`
- `{{repo_path}}/agent/agent.go`
- `{{repo_path}}/runner/runner.go`
- `{{repo_path}}/flow/flow.go`
- `{{repo_path}}/event/event.go`
- `{{repo_path}}/tool/tool.go`
- `{{repo_path}}/workflow/workflow.go`
- `{{repo_path}}/tool/agenttool/agent_tool.go`
- The relevant ADK Go source files under `{{source_repo}}`, especially
  `runner/runner.go`, `agent/loader.go`, and `internal/llminternal/agent_transfer.go`
  if present.

## Required implementation shape

Implement a minimal but real teaching-model transfer stack:

1. Agent tree metadata:
   - support named sub-agents without breaking existing agents;
   - expose a way to resolve agents by name;
   - preserve current `agent.Agent` usage in existing tests.
2. Transfer tool:
   - expose a `transfer_to_agent` function tool/declaration;
   - validate target agent names against the current tree;
   - set `event.EventActions.TransferToAgent` rather than faking success in text.
3. Flow transfer execution:
   - after a model/tool event requests transfer, execute the target agent in the
     same invocation with an appropriate branch/author boundary;
   - avoid infinite transfer loops with a clear max-depth/visited guard.
4. Runner active-agent routing:
   - later user messages in the same session should route to the active agent
     implied by prior `TransferToAgent` actions when allowed by the tree;
   - this should be deterministic and testable from session history.
5. Tests:
   - transfer tool declaration has the allowed agent names;
   - model-triggered transfer delegates execution to the target agent;
   - invalid transfer target yields a structured tool error;
   - a second `runner.Run` routes to the active agent from previous history;
   - existing tests continue to pass.

Prefer small new packages/files over large rewrites. Do not replace the existing
Runner/Flow architecture.

## Verification

Run:

```sh
go test ./...
go vet ./...
git diff --check
```

If a command fails, fix it before reporting.

## Report

Write `{{repo_path}}/.rive-artifacts/28-agent-transfer-routing.md` with:

- `implemented`
- `files_changed`
- `tests`
- `notes`

Then capture a snapshot and report:

```sh
SNAPSHOT_ID=$(rive snapshot capture --path "$RIVE_WORKSPACE" --label "agent transfer routing implemented" --dispatch "$RIVE_DISPATCH_ID" | python3 -c 'import json,sys; print(json.load(sys.stdin)["protocol"]["snapshot_id"])')
team report --dispatch "$RIVE_DISPATCH_ID" --status done --snapshot "$SNAPSHOT_ID" --command-id "agent-transfer-routing-report-$(date +%s)" --artifact-ref file:{{repo_path}}/.rive-artifacts/28-agent-transfer-routing.md --stdin < {{repo_path}}/.rive-artifacts/28-agent-transfer-routing.md
```
