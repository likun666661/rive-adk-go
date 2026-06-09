# Node 1: Extend Tool Declarations and Toolsets

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Extend the existing compact `tool` package toward the Chapter 03 tool-system
shape: a minimal `Tool` abstraction, tool declarations for model requests, and
dynamic toolsets with filtering.

## Required scope

1. Read the Chapter 03 guide and relevant ADK Go reference files for tool/toolset
   declarations. Use them as design reference; do not copy ADK Go code.
2. Keep the existing function-tool behavior compatible, but introduce small
   educational types for:
   - tool declaration metadata;
   - input/output schema as lightweight maps or structs;
   - toolset collections;
   - filtering/allowed-tools predicates;
   - request processing that exposes declarations to `model.LLMRequest`.
3. Add tests that prove:
   - function tools expose stable declarations;
   - toolsets can be filtered by context/tool name;
   - declaration injection is deterministic and ordered;
   - existing `tool.Execute` and flow tests continue to work.

## Constraints

- Edit only under `{{repo_path}}`.
- Keep this educational and idiomatic Go; do not import heavy schema libraries
  unless absolutely necessary.
- Do not modify `.rive/`, `.opencode/`, or the source ADK Go repo.
- Run `go test ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/10-tool-declaration-toolset.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/10-tool-declaration-toolset.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
