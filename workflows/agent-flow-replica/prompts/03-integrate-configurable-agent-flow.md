# Node 3: Integrate Configurable Agent Flow

You are an OpenCode implementation worker inside a Rive workflow.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Integrate the Chapter 07 feature into demos/docs and add a small configurable
agent-flow surface, so the replica can teach Agent Flow/ReAct/Multi-Agent as a
coherent chapter.

If previous failed attempts already left partial config/demo code in the
workspace, do not restart from scratch. Inspect it, fix gaps, verify the
acceptance criteria, then report through the Rive ledger.

## Read first

- `{{chapter_path}}`
- `{{repo_path}}/.rive-artifacts/28-agent-transfer-routing.md`
- `{{repo_path}}/.rive-artifacts/29-react-policy-extensions.md`
- `{{repo_path}}/README.md`
- `{{repo_path}}/cmd/demo/main.go`
- `{{repo_path}}/cmd/demo/main_test.go`
- `{{repo_path}}/docs/source.md`
- ADK Go configurable code discussed in the chapter guide.

## Required implementation shape

1. Configurable surface:
   - add a tiny standard-library config loader for teaching examples;
   - support at least `llm_agent`, sub-agent names/descriptions, and simple
     tool references from a registry;
   - validate duplicates and missing references with deterministic errors.
2. Chapter 07 demo:
   - update `cmd/demo` to show:
     - ReAct function-call loop;
     - transfer from a host agent to a specialist;
     - exit loop / reflection or hidden arg policy;
     - configurable construction if practical.
3. README/docs:
   - add Chapter 07 to the chapter list and package table if new packages were
     introduced;
   - explain what is implemented and intentionally omitted:
     - streaming partial aggregation is still simplified;
     - NL planning/code execution are extension slots, not full products;
     - this is an educational replica, not API-compatible ADK Go.
4. Tests:
   - config loader constructs a valid tree and rejects invalid references;
   - demo still runs;
   - full suite passes.

Keep the config format minimal; no YAML dependency is required. A simple JSON
or line-oriented format is fine if documented.

## Verification

Run:

```sh
go test ./...
go vet ./...
git diff --check
go run ./cmd/demo
```

## Report

Write `{{repo_path}}/.rive-artifacts/30-configurable-agent-flow.md` with:

- `implemented`
- `files_changed`
- `tests`
- `notes`

Then capture a snapshot and report:

```sh
SNAPSHOT_ID=$(rive snapshot capture --path "$RIVE_WORKSPACE" --label "configurable agent flow integrated" --dispatch "$RIVE_DISPATCH_ID" | python3 -c 'import json,sys; print(json.load(sys.stdin)["protocol"]["snapshot_id"])')
team report --dispatch "$RIVE_DISPATCH_ID" --status done --snapshot "$SNAPSHOT_ID" --command-id "configurable-agent-flow-report-$(date +%s)" --artifact-ref file:{{repo_path}}/.rive-artifacts/30-configurable-agent-flow.md --stdin < {{repo_path}}/.rive-artifacts/30-configurable-agent-flow.md
```
