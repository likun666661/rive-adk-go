# Node 2: Implement Plugin Manager and Hook Ordering

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Add a compact plugin layer that composes existing callback hook points with
ordered execution and early-exit semantics.

## Required scope

1. Read the Chapter 04 guide's plugin manager and hook-ordering sections.
2. Add a small `plugin` package or equivalent surface with:
   - a `Plugin` type/interface that can provide before/after agent callbacks;
   - before/after model callbacks;
   - before/after tool callbacks;
   - optional model/tool error recovery hooks if this can be done cleanly.
3. Add a `Manager` that:
   - preserves registration order;
   - skips nil hooks;
   - stops at first non-nil result;
   - returns errors immediately;
   - can be plugged into `agent` and `flow` without rewriting their core loop.
4. Integrate plugin hooks so they run before direct callbacks, matching the
   Chapter 04 teaching model.
5. Add tests for hook ordering, early-exit, error propagation, plugin-before
   direct callback ordering, and optional error recovery if implemented.

## Constraints

- Build on Node 1's callback context/action surface.
- Keep the API small and deterministic.
- Edit only under `{{repo_path}}`.
- Run `go test ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/15-plugin-manager-hooks.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/15-plugin-manager-hooks.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
