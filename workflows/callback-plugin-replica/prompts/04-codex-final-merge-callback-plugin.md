# Node 4: Codex Final Review, Fixes, Verification, Commit

You are the final Codex steward inside a Rive workflow.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Review the OpenCode implementation against Chapter 04, fix quality issues, run
verification, and create the final git commit.

## Required review criteria

1. The code demonstrates Chapter 04's callback/plugin/instruction design
   pressure:
   - cross-cutting callbacks;
   - plugin ordering and early-exit;
   - callback context state/action tracking;
   - instruction providers and state placeholder injection;
   - model/tool interception and optional error recovery if implemented.
2. It extends the existing Chapter 01-03 runtime rather than replacing it.
3. Tests cover:
   - callback state/action tracking;
   - artifact delta tracking or equivalent;
   - plugin ordering and early-exit;
   - plugin before direct callbacks;
   - instruction interpolation from scoped state;
   - full runner/flow integration.
4. README clearly states what is implemented and what is intentionally omitted.
5. `go test ./...`, `go vet ./...`, `git diff --check`, and `go run ./cmd/demo`
   pass.

## Actions

- Inspect repo state and reports under `.rive-artifacts/14..16`.
- Fix bugs, naming drift, missing tests, or documentation gaps.
- Run all verification commands.
- Create a git commit for the final result if verification passes.
- Write `{{repo_path}}/.rive-artifacts/17-codex-callback-plugin-merge.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/17-codex-callback-plugin-merge.md`.

Report sections: `review_findings`, `fixes`, `verification`, `commit`.
