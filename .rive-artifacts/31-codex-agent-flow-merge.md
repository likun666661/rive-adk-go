## review_findings

Reviewed the Chapter 07 implementation against the deep-read acceptance points
and prior reports in `.rive-artifacts/28-agent-transfer-routing.md`,
`.rive-artifacts/29-react-policy-extensions.md`, and
`.rive-artifacts/30-configurable-agent-flow.md`.

The implementation extends the Chapter 01-06 runtime rather than replacing it:
`runner.Run` still drives sessions and invocation context, `agent.Execute`
still owns lifecycle callbacks, and `flow.Flow` still owns the ReAct
model/tool/event loop.

Review found and corrected three quality issues:

- Active-agent routing fell back to the root executable when a history-selected
  agent was not executable, but it kept the non-executable agent in the
  invocation context.
- Workflow agents exposed sub-agents but did not accept parent/transfer
  constraint wiring from `agentconfig`, leaving configurable workflow trees
  weaker than LLM-agent trees.
- Config validation claimed deterministic unknown-tool output, but available
  tool names came from unsorted map keys.

## fixes

- Added safe cloning for base agent sub-agent slices and transfer target
  computation.
- Added mutator hooks used by `agent.SetParent`,
  `agent.SetDisallowTransferToParent`, and
  `agent.SetDisallowTransferToPeers`, then implemented those hooks on
  sequential, parallel, and loop workflow agents.
- Updated `agentconfig` to sort tool registry keys and apply transfer
  constraints to workflow agents.
- Fixed runner fallback routing so the invocation context agent and branch
  are reset to the root when the selected active agent is not executable.
- Added regression coverage for non-executable active-agent fallback,
  workflow config parent links, workflow transfer constraints, and sorted
  config validation output.
- Updated `README.md` with a Chapter 07 scope section describing implemented
  behavior and deliberate simplifications.

## verification

All required verification passed after final fixes:

- `go test ./...`
- `go vet ./...`
- `git diff --check`
- `go run ./cmd/demo`

The demo completes Chapters 01-07 and shows Chapter 07 ReAct looping,
structured `transfer_to_agent`, policy extensions, and JSON config validation.

## commit

Created one final Chapter 07 git commit after verification, containing the
runtime changes, tests, workflow prompt artifacts, prior node artifacts, and
this final review artifact.
