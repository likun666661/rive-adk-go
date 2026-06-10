# Node 2: Implement ReAct Policy Extensions

You are an OpenCode implementation worker inside a Rive workflow.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Build on the transfer/routing implementation and add the Chapter 07 ReAct policy
surface: exit-loop semantics, reflection/retry behavior, and request/tool
shaping hooks that demonstrate why ADK Go keeps policy outside the bare model
loop.

Keep this educational and deterministic. Do not introduce real LLM/network
dependencies.

If previous failed attempts already left partial policy-extension code in the
workspace, do not restart from scratch. Inspect it, fix gaps, verify the
acceptance criteria, then report through the Rive ledger.

## Read first

- `{{chapter_path}}`
- `{{repo_path}}/.rive-artifacts/28-agent-transfer-routing.md`
- `{{repo_path}}/flow/flow.go`
- `{{repo_path}}/plugin/manager.go`
- `{{repo_path}}/plugin/plugin.go`
- `{{repo_path}}/tool/tool.go`
- `{{repo_path}}/instruction/`
- ADK Go reference: retry/reflect plugin, skill/toolset code, exit loop tool,
  and any request processors discussed in the chapter guide.

## Required implementation shape

Implement a small policy layer with concrete tests:

1. Exit loop:
   - add an `exit_loop` style tool or helper that sets `EndInvocation`;
   - ensure Flow/Runner stop naturally and persist the event correctly.
2. Retry/reflection:
   - add a deterministic plugin or flow option that turns a tool error into a
     reflection prompt/content for the next model call;
   - do not hide the original tool error from events.
3. Tool/request shaping:
   - demonstrate one policy that injects hidden args into a tool before
     execution, or strips internal args from model-visible declarations;
   - document why this protects the LLM-facing cognitive space.
4. Tests:
   - `exit_loop` stops a multi-step ReAct run;
   - tool failure can be reflected back to the model and then resolved;
   - hidden arg injection works without appearing in the model request;
   - transfer behavior from Node 1 still passes.

Do not add external dependencies unless there is no reasonable standard-library
alternative.

## Verification

Run:

```sh
go test ./...
go vet ./...
git diff --check
```

## Report

Write `{{repo_path}}/.rive-artifacts/29-react-policy-extensions.md` with:

- `implemented`
- `files_changed`
- `tests`
- `notes`

Then capture a snapshot and report:

```sh
SNAPSHOT_ID=$(rive snapshot capture --path "$RIVE_WORKSPACE" --label "react policy extensions implemented" --dispatch "$RIVE_DISPATCH_ID" | python3 -c 'import json,sys; print(json.load(sys.stdin)["protocol"]["snapshot_id"])')
team report --dispatch "$RIVE_DISPATCH_ID" --status done --snapshot "$SNAPSHOT_ID" --command-id "react-policy-extensions-report-$(date +%s)" --artifact-ref file:{{repo_path}}/.rive-artifacts/29-react-policy-extensions.md --stdin < {{repo_path}}/.rive-artifacts/29-react-policy-extensions.md
```
