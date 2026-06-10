# Node 4: Codex Final Review, Fixes, Verification, Commit

You are the final Codex steward inside a Rive workflow.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Review the OpenCode implementation against Chapter 07, fix quality issues, run
verification, and create the final git commit.

## Required review criteria

1. The implementation demonstrates Chapter 07's core architecture pressure:
   - ReAct loop as model/tool/event/session iteration;
   - transfer-to-agent as a structured action, not text;
   - active-agent routing from session history;
   - multi-agent tree constraints;
   - policy layer for exit loop / reflection / hidden tool args;
   - configurable construction as a small teaching surface.
2. The code extends the existing Chapter 01-06 runtime instead of replacing it.
3. Tests cover transfer success/failure, active-agent routing, exit/reflection
   policy, config validation, and demo behavior.
4. README and docs clearly state what is implemented and what is deliberately
   simplified or omitted.
5. `go test ./...`, `go vet ./...`, `git diff --check`, and `go run ./cmd/demo`
   pass.

## Actions

- Inspect repo state and reports under `.rive-artifacts/28..30`.
- Fix bugs, missing tests, naming drift, package organization, and docs gaps.
- Remove stale/untracked scratch files that are clearly produced by this
  workflow and should not be committed; do not remove unrelated user files.
- Run all verification commands.
- Create one git commit for the final Chapter 07 result if verification passes.
- Write `{{repo_path}}/.rive-artifacts/31-codex-agent-flow-merge.md`.
- Capture a snapshot and report:

```sh
SNAPSHOT_ID=$(rive snapshot capture --path "$RIVE_WORKSPACE" --label "chapter 07 agent flow codex final merge" --dispatch "$RIVE_DISPATCH_ID" | python3 -c 'import json,sys; print(json.load(sys.stdin)["protocol"]["snapshot_id"])')
team report --dispatch "$RIVE_DISPATCH_ID" --status done --snapshot "$SNAPSHOT_ID" --command-id "codex-agent-flow-merge-report-$(date +%s)" --artifact-ref file:{{repo_path}}/.rive-artifacts/31-codex-agent-flow-merge.md --stdin < {{repo_path}}/.rive-artifacts/31-codex-agent-flow-merge.md
```

Report sections: `review_findings`, `fixes`, `verification`, `commit`.
