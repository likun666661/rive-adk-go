# Node 1: Implement Launcher CLI

You are an OpenCode worker inside a Rive workflow. Do not use OpenCode's
internal task/subagent feature; Rive is the only orchestration layer.

## Inputs

- Target repo: `{{repo_path}}`
- Chapter guide: `{{chapter_path}}`
- Source ADK Go repo for reference only: `{{source_repo}}`

## Goal

Add a compact Chapter 06 launcher layer that exposes the existing runtime
through stable entrypoint abstractions: launcher config, sublauncher routing,
console execution, and CLI-friendly command parsing.

## Required scope

1. Read the Chapter 06 guide and relevant ADK Go launcher/CLI code. Use the
   source as design reference only; do not copy ADK Go source.
2. Add a small `launcher` package or equivalent with:
   - `Config` carrying session/artifact/memory/agent loader/plugin/telemetry
     dependencies;
   - `Launcher` and `SubLauncher` interfaces;
   - universal routing by first argv token with a documented default
     sublauncher;
   - console sublauncher that drives the existing runner from scripted input.
3. Add tests for:
   - default sublauncher selection;
   - keyword-based routing;
   - unknown command errors;
   - console execution persisting events through the existing runner/session.
4. Keep the API educational and compact. Do not implement full cobra or real
   terminal streaming unless already present in this repo.

## Constraints

- Edit only under `{{repo_path}}`.
- Preserve Chapter 01-05 behavior and tests.
- Run `go test ./...`.
- Write a report to `{{repo_path}}/.rive-artifacts/23-launcher-cli.md`.
- Capture a snapshot and `team report --status done --artifact-ref file:{{repo_path}}/.rive-artifacts/23-launcher-cli.md`.

Report sections: `implemented`, `files_changed`, `tests`, `notes`.
