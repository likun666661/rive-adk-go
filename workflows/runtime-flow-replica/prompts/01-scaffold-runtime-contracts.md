# Node 1: Scaffold Runtime Contracts

You are an OpenCode worker inside a Rive workflow.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Create the foundation of a small Go runtime-flow replica inspired by Chapter 01.
Do not copy the ADK Go source. Implement a compact educational architecture that
makes the same design problems visible.

## Required scope

1. Initialize a Go module named `github.com/likun666661/rive-adk-go` if it is not
   already initialized.
2. Add core packages/types for:
   - event content, events, event actions, partial/final markers;
   - session state and append-only event history;
   - invocation context;
   - agent interface and base agent callback lifecycle.
3. Add focused unit tests for session append, state delta merge, callback early
   exit, and final response detection.
4. Keep the API small and idiomatic Go.

## Expected architecture

The final repo should make this chain explicit:

```text
Runner -> Agent -> Flow -> Model/Tool -> Event -> Session
```

Your node owns the leftmost contracts and must leave clear TODO comments only
when they define a boundary for later nodes.

## Constraints

- Edit only under `{{repo_path}}`.
- Do not modify the source ADK Go repo.
- Run `go test ./...`.
- Write a short report to `{{repo_path}}/.rive-artifacts/01-scaffold-runtime-contracts.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/01-scaffold-runtime-contracts.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.

