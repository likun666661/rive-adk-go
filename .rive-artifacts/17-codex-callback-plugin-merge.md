## review_findings

- Reviewed Chapter 04 against the prior node reports in `.rive-artifacts/14..16`.
- Found that `Flow` declared context-aware callback fields but did not execute them.
- Found that model/tool plugin callbacks used fresh `EventActions`, so callback state writes and artifact saves did not surface on emitted events.
- Found README drift: it still described callback `ArtifactDelta` tracking as intentionally omitted.

## fixes

- Wired `BeforeModelCallbacksCtx`, `AfterModelCallbacksCtx`, `BeforeToolCallbacksCtx`, and `AfterToolCallbacksCtx` into the flow.
- Reused shared `EventActions` for model plugin/direct callbacks and attached those actions to model events.
- Added per-tool `EventActions` to `tool.CallResult` and merged tool callback/plugin state, artifact, and control actions into the tool-result event.
- Enabled artifact tracking for tool callback contexts.
- Added runner integration tests covering model callback state/artifact deltas, tool callback state/artifact deltas, and plugin-before-direct tool ordering.
- Updated README to document implemented callback context action tracking and remaining intentional omissions.

## verification

```bash
go test ./...
go vet ./...
git diff --check
go run ./cmd/demo
```

All verification commands passed.

## commit

- Commit: `ff69a0d` (`Implement callback plugin instruction runtime`)
- Included the Chapter 04 callback context, plugin manager, instruction processor, demo, tests, workflow prompt files, and prior node artifacts.
