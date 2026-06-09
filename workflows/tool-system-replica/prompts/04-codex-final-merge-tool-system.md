# Node 4: Codex Final Review, Fixes, Verification, Commit

You are the final Codex steward inside a Rive workflow.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Review the OpenCode implementation against Chapter 03, fix quality issues, run
verification, and create the final git commit.

## Required review criteria

1. The code demonstrates Chapter 03's tool-system design pressure:
   unified declarations, toolsets/filtering, confirmation, streaming,
   long-running metadata, and structured errors.
2. It extends the existing Chapter 01/02 runtime rather than replacing it.
3. Tests cover:
   - stable tool declarations;
   - filtered toolsets;
   - declaration injection into model request;
   - confirmation required/approved/rejected;
   - streaming collection and streaming error;
   - long-running marker;
   - full runner/flow integration.
4. README clearly states what is implemented and what is intentionally omitted
   (MCP, real Skill toolsets, Gemini native tools, live session streaming).
5. `go test ./...`, `go vet ./...`, `git diff --check`, and `go run ./cmd/demo`
   pass.

## Actions

- Inspect the repo state and reports under `.rive-artifacts/10..12`.
- Fix bugs, naming drift, missing tests, or docs gaps.
- Run all verification commands.
- Create a git commit for the final result if verification passes.
- Write `{{repo_path}}/.rive-artifacts/13-codex-tool-system-merge.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/13-codex-tool-system-merge.md`.

Report sections: `review_findings`, `fixes`, `verification`, `commit`.
