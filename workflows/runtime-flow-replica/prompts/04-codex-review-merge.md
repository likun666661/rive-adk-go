# Node 4: Codex Review, Fix, Verify, Commit

You are the final Codex steward inside a Rive workflow.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Review the OpenCode implementation against Chapter 01 and turn the repo into a
coherent first cut. You own final quality, verification, and the final git
commit.

## Required review criteria

1. The code demonstrates the Chapter 01 architecture:
   `Runner -> Agent -> Flow -> Model/Tool -> Event -> Session`.
2. The implementation is not a blind copy of ADK Go; it is a compact replica
   with clear names and tests.
3. Tests cover:
   - session append/persistence;
   - callback/processor ordering;
   - model final response;
   - model tool-call loop;
   - multiple tool calls;
   - runner end-to-end chain.
4. README explains what is implemented, what is intentionally omitted, and how
   to run tests/demo.
5. `go test ./...`, `go vet ./...`, and `git diff --check` pass.

## Actions

- Inspect the repo state and previous node reports under `.rive-artifacts/`.
- Fix bugs, naming drift, missing tests, or documentation gaps.
- Run verification commands.
- Create a git commit for the final result if verification passes.
- Write `{{repo_path}}/.rive-artifacts/04-codex-review-merge.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/04-codex-review-merge.md`.

Report sections: `review_findings`, `fixes`, `verification`, `commit`.

