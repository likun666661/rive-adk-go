# Node 3: Implement Deploy Plan and Telemetry Model

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Add Chapter 06 educational deploy and telemetry surfaces without invoking real
cloud CLIs: produce dry-run deploy plans and in-memory telemetry spans/logs that
explain how entrypoints become product/server deployments.

## Required scope

1. Read the Chapter 06 guide and ADK Go deploy/telemetry code references.
2. Add a compact deploy package or equivalent with:
   - Cloud Run-style dry-run plan: linux binary build target, Dockerfile command,
     enabled web protocols, env/secrets, proxy URL hints;
   - Agent Engine-style dry-run plan: source archive, class methods,
     stream query endpoint, runtime command;
   - validation for entrypoint path, project/region/service name, and protocol
     options.
3. Add a compact telemetry package or equivalent with:
   - provider/options model;
   - span/log recorder abstraction;
   - instrumentation helpers for runner/model/tool/server events;
   - explicit shutdown/flush semantics.
4. Add tests for:
   - deterministic Cloud Run plan;
   - deterministic Agent Engine plan;
   - validation errors;
   - telemetry span/log recording and flush.

## Constraints

- Do not call real `gcloud`, Docker, or network APIs.
- Build on launcher/server packages from nodes 1 and 2 where useful.
- Edit only under `{{repo_path}}`.
- Preserve Chapter 01-05 tests.
- Run `go test ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/25-deploy-telemetry.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/25-deploy-telemetry.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
