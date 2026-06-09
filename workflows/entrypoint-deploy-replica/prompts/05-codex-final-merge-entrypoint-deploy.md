# Node 5: Codex Final Review, Fixes, Verification, Commit

You are the final Codex steward inside a Rive workflow.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Review the OpenCode implementation against Chapter 06, fix quality issues, run
verification, and create the final git commit.

## Required review criteria

1. The code demonstrates Chapter 06's productization design pressure:
   - launcher config and sublauncher routing;
   - console/local entrypoint;
   - HTTP JSON and SSE runtime server;
   - deploy dry-run plans for Cloud Run and Agent Engine;
   - telemetry span/log capture and shutdown;
   - examples/docs explaining why the same runtime can serve many entrypoints.
2. It extends the existing Chapter 01-05 runtime rather than replacing it.
3. Tests cover:
   - launcher routing/default semantics;
   - console invocation;
   - JSON and SSE server paths;
   - deploy plan validation and deterministic output;
   - telemetry recording/flush;
   - demo/README integration.
4. README clearly states what is implemented and what is intentionally omitted.
5. `go test ./...`, `go vet ./...`, `git diff --check`, and `go run ./cmd/demo`
   pass.

## Actions

- Inspect repo state and reports under `.rive-artifacts/23..26`.
- Fix bugs, naming drift, missing tests, or documentation gaps.
- Run all verification commands.
- Create a git commit for the final result if verification passes.
- Write `{{repo_path}}/.rive-artifacts/27-codex-entrypoint-deploy-merge.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/27-codex-entrypoint-deploy-merge.md`.

Report sections: `review_findings`, `fixes`, `verification`, `commit`.
