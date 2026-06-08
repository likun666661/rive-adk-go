# Node 5: OpenCode Final Merge for State Lifecycle

You are the final OpenCode merge node inside a Rive workflow. Do not use
OpenCode's internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Turn the implementation plus Codex review into a clean committed second cut.
This node owns final fixes, verification, and the git commit.

## Required scope

1. Read the prior node reports:
   - `.rive-artifacts/05-state-scopes.md`
   - `.rive-artifacts/06-memory-artifact-services.md`
   - `.rive-artifacts/07-state-lifecycle-integration.md`
   - `.rive-artifacts/08-codex-state-lifecycle-review.md`
2. Address any real Codex review findings. If a finding is intentionally
   deferred, document the reason in the final report.
3. Ensure the repo is coherent:
   - no accidental generated/debug files are committed;
   - workflow package files are included if they are useful for replaying this
     dogfood run;
   - `.rive-artifacts/` reports may be committed as run evidence.
4. Run:
   - `go test ./...`
   - `go vet ./...`
   - `git diff --check`
   - `go run ./cmd/demo`
5. Create a git commit with a concise message, for example:
   `Add state lifecycle replica`.

## Constraints

- Edit only under `{{repo_path}}`.
- Do not modify the source ADK Go repo.
- Do not print or expose any Rive agent tokens.
- Write `{{repo_path}}/.rive-artifacts/09-opencode-state-lifecycle-merge.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/09-opencode-state-lifecycle-merge.md`.

Report sections: `review_response`, `fixes`, `verification`, `commit`.
