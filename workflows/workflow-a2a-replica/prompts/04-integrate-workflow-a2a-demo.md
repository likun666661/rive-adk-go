# Node 4: Integrate Workflow/A2A Demo, Docs, and Tests

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Integrate the Chapter 05 workflow-agent, AgentTool, and remote A2A surfaces into
the README and demo so the project now teaches the composition layer on top of
Chapters 01-04.

## Required scope

1. Read reports `.rive-artifacts/18..20` and the Chapter 05 guide.
2. Add or update end-to-end runner tests that demonstrate:
   - sequential workflow;
   - parallel workflow with branch/event labels;
   - loop workflow early stop;
   - AgentTool delegation inside a parent agent/tool flow;
   - remote A2A streaming aggregation.
3. Update `cmd/demo` with a Chapter 05 section.
4. Update `README.md` with:
   - what Chapter 05 adds;
   - what is intentionally simplified;
   - how workflow agents differ from tool delegation and remote delegation.
5. Run full verification:
   - `go test ./...`
   - `go vet ./...`
   - `git diff --check`
   - `go run ./cmd/demo`

## Constraints

- Edit only under `{{repo_path}}`.
- Keep the demo compact enough to run quickly but explicit enough to teach the
  pattern.
- Write a report to `{{repo_path}}/.rive-artifacts/21-workflow-a2a-integration.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/21-workflow-a2a-integration.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
