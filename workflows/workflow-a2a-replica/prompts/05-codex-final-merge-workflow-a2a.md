# Node 5: Codex Final Review, Fixes, Verification, Commit

You are the final Codex steward inside a Rive workflow.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Review the OpenCode implementation against Chapter 05, fix quality issues, run
verification, and create the final git commit.

## Required review criteria

1. The code demonstrates Chapter 05's orchestration design pressure:
   - sequential, parallel, and loop workflow agents;
   - event stream aggregation and branch metadata;
   - agent-as-tool sandbox delegation;
   - lightweight remote A2A conversion/stream aggregation/cleanup;
   - clear difference between local workflow, tool delegation, and remote
     delegation.
2. It extends the existing Chapter 01-04 runtime rather than replacing it.
3. Tests cover:
   - sequential ordering;
   - parallel aggregation/branching;
   - loop stop/max iteration;
   - AgentTool state isolation;
   - remote A2A partial aggregation and cleanup/error path;
   - full demo/runner integration.
4. README clearly states what is implemented and what is intentionally omitted.
5. `go test ./...`, `go vet ./...`, `git diff --check`, and `go run ./cmd/demo`
   pass.

## Actions

- Inspect repo state and reports under `.rive-artifacts/18..21`.
- Fix bugs, naming drift, missing tests, or documentation gaps.
- Run all verification commands.
- Create a git commit for the final result if verification passes.
- Write `{{repo_path}}/.rive-artifacts/22-codex-workflow-a2a-merge.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/22-codex-workflow-a2a-merge.md`.

Report sections: `review_findings`, `fixes`, `verification`, `commit`.
