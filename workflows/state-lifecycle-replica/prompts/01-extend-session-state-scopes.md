# Node 1: Extend Session State Scope Semantics

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Extend the existing Chapter 01 runtime replica with the state lifecycle ideas
from Chapter 02. Focus on session state scoping, event state deltas, partial
event behavior, and invocation-local temp state.

## Required scope

1. Read the chapter guide and the relevant source ADK Go files for session
   state behavior. Use the source as design reference, not copied code.
2. Replace the TODO-level state behavior with a compact implementation for:
   - session-scoped keys;
   - `app:` state shared by app;
   - `user:` state shared by app + user;
   - `temp:` state visible during the current invocation but not persisted;
   - deletion/tombstone semantics where useful for the small replica.
3. Keep `Partial=true` events non-persistent, and make tests prove partial
   events cannot mutate durable state.
4. Add or update tests covering create/get/append state merge, scoped overlay
   order, temp trimming, delete semantics, and partial-event guard behavior.

## Constraints

- Edit only under `{{repo_path}}`.
- Keep APIs small and idiomatic Go; this is an educational replica, not an ADK
  Go fork.
- Do not modify `.rive/`, `.opencode/`, or the source ADK Go repo.
- Run `go test ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/05-state-scopes.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/05-state-scopes.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
