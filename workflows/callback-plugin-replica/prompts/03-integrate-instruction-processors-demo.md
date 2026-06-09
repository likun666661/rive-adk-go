# Node 3: Integrate Instruction Processors, Docs, and Demo

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Make Chapter 04 visible from the runtime surface: instruction providers,
session-state template injection, plugin observability/caching examples, and
documentation.

## Required scope

1. Add a compact instruction processor layer:
   - static agent instruction;
   - dynamic instruction provider;
   - global instruction provider;
   - `{placeholder}` injection from session/user/app state.
2. Integrate instruction processing into `model.LLMRequest` or the existing flow
   request processor pipeline so tests can inspect the final request.
3. Add demo scenarios for:
   - plugin logging/observability;
   - before-model cache/mock response early-exit;
   - callback state mutation and instruction interpolation;
   - plugin ordering relative to direct callbacks.
4. Update `README.md` with a Chapter 04 section explaining:
   - why callback/plugin/instruction hooks exist;
   - why hook ordering and early-exit matter;
   - what is intentionally omitted (real telemetry exporters, auth plugins,
     full ADK plugin API compatibility).
5. Add integration tests covering the full runner/flow path.

## Constraints

- Preserve existing demos and tests.
- Keep everything deterministic and stdlib-only.
- Edit only under `{{repo_path}}`.
- Run `go test ./...` and `go vet ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/16-instruction-plugin-integration.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/16-instruction-plugin-integration.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
