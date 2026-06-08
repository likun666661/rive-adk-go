# Node 2: Add Memory and Artifact Services

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Add compact `memory` and `artifact` service packages that demonstrate why
Chapter 02 separates session, memory, and artifact lifecycles.

## Required scope

1. Read the chapter guide and ADK Go reference files for memory and artifact
   behavior. Use them as design reference only.
2. Add a `memory` package with an in-memory service that can:
   - ingest a session's non-partial events as long-term entries;
   - search entries by simple keyword/content matching;
   - preserve app/user boundaries.
3. Add an `artifact` package with an in-memory versioned store that can:
   - save named artifacts by app/user/session;
   - support `user:` names that are user-scoped across sessions;
   - load latest and explicit versions;
   - list artifacts without exposing blobs.
4. Add focused tests for lifecycle boundaries:
   - memory survives across sessions but remains app/user scoped;
   - artifacts version independently from events;
   - `user:` artifact names are visible across sessions for the same user;
   - artifact versions increment deterministically.

## Constraints

- Build on Node 1's state/session changes; do not rewrite unrelated runtime
  flow or tool-loop code.
- Edit only under `{{repo_path}}`.
- Run `go test ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/06-memory-artifact-services.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/06-memory-artifact-services.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
