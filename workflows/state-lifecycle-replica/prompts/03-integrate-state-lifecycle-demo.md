# Node 3: Integrate State Lifecycle Into Runner and Docs

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Make the session/memory/artifact lifecycle visible at the runtime boundary:
`Runner -> InvocationContext -> Agent/Flow -> Event Actions -> Session`.

## Required scope

1. Expose the new memory and artifact services through runner configuration
   and/or invocation context when that improves the example.
2. Add one executable example or extend `cmd/demo` so it demonstrates:
   - durable session state mutation through event actions;
   - app/user/session/temp scoping in a small scenario;
   - saving/loading an artifact;
   - adding/searching memory from a completed session.
3. Update `README.md` with a Chapter 02 section explaining:
   - the problem each lifecycle solves;
   - why session/memory/artifact should not be one store;
   - the simplified semantics implemented here;
   - intentional omissions.
4. Add integration tests tying runner/session state, memory, and artifacts
   together without relying on external services.

## Constraints

- Keep the public surface minimal.
- Preserve all Chapter 01 tests and demo behavior unless the README explains a
  deliberate extension.
- Edit only under `{{repo_path}}`.
- Run `go test ./...`.
- Run `go vet ./...` if tests pass.
- Write a report to `{{repo_path}}/.rive-artifacts/07-state-lifecycle-integration.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/07-state-lifecycle-integration.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
