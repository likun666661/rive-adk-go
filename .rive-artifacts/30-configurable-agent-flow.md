## implemented

Chapter 07 configurable agent flow integrated across three areas:

1. **Configurable agent tree construction** (`agent/agentconfig/`):
   - JSON config loader (`FromJSON`, `Build`) supporting `llm_agent`, `sequential`,
     `parallel`, and `loop` agent types.
   - `ToolRegistry` for tool name → implementation resolution.
   - Validation: duplicate agent names (deterministic error with source path),
     missing tool references (lists available tools), unknown type errors.
   - Parent chain wiring via exported `agent.SetParent`/`agent.SetSubAgents` helpers.
   - Transfer constraint support (`DisallowTransferToParent`, `DisallowTransferToPeers`).

2. **Chapter 07 demo** (`cmd/demo/main.go`):
   - Demo 7.1 — ReAct function-call loop: user → model (FC) → tool → model (final).
   - Demo 7.2 — Agent transfer: host agent delegates to specialist via `transfer_to_agent`.
   - Demo 7.3 — Policy extensions: ExitLoop (EndInvocation), retry/reflect
     (tool error recovery with reflection), hidden args (FunctionCallModifier).
   - Demo 7.4 — Configurable construction: JSON config → agent tree with validation errors.

3. **Agent package extension** (`agent/agent.go`):
   - Exported `SetSubAgents`, `SetParent`, `SetDisallowTransferToParent`,
     `SetDisallowTransferToPeers` functions for post-construction agent tree wiring.
   - Lenient for non-`baseAgent` types (workflow agents silently skipped).

## files_changed

- `agent/agentconfig/config.go` — JSON config loader (new package, ~270 lines)
- `agent/agentconfig/config_test.go` — config loader tests (new, ~260 lines)
- `agent/agent.go` — added SetSubAgents, SetParent, SetDisallowTransferTo(Parent|Peers)
- `cmd/demo/main.go` — added Chapter 07 demo (ReAct, transfer, policy, configurable)
- `README.md` — Chapter 07 in chapter list, package table, and demo section

## tests

| Test | Location | Criteria |
|------|----------|----------|
| TestFromJSONBasic | `agent/agentconfig/config_test.go` | JSON parse produces correct fields |
| TestFromJSONInvalidJSON | `agent/agentconfig/config_test.go` | Invalid JSON yields error |
| TestFromJSONFullConfig | `agent/agentconfig/config_test.go` | Nested config parses correctly |
| TestBuildValidTree | `agent/agentconfig/config_test.go` | Agent tree with sub-agents |
| TestBuildDuplicateNames | `agent/agentconfig/config_test.go` | Duplicate names rejected |
| TestBuildMissingName | `agent/agentconfig/config_test.go` | Missing name rejected |
| TestBuildMissingType | `agent/agentconfig/config_test.go` | Missing type rejected |
| TestBuildUnknownTool | `agent/agentconfig/config_test.go` | Unknown tool reference rejected |
| TestBuildUnknownType | `agent/agentconfig/config_test.go` | Unknown type rejected |
| TestBuildSequentialAgent | `agent/agentconfig/config_test.go` | Sequential agent construction |
| TestBuildParallelAgent | `agent/agentconfig/config_test.go` | Parallel agent construction |
| TestBuildLoopAgent | `agent/agentconfig/config_test.go` | Loop agent construction |
| TestBuildTransferConstraints | `agent/agentconfig/config_test.go` | DisallowTransferTo(Parent|Peers) set correctly |
| TestBuildNestedTreeWithParentLinks | `agent/agentconfig/config_test.go` | Parent chain wiring + FindAgent |

All 29 test packages pass (`go test ./...`), `go vet ./...` reports no warnings,
`git diff --check` reports no whitespace issues, and `go run ./cmd/demo` completes
all 7 chapters successfully including the new Chapter 07 output.

## notes

The implementation follows the Chapter 07 deep-dive design with these educational
simplifications:

- **Config format is JSON, not YAML**: Avoids an external dependency. The JSON
  schema is minimal and documented inline in the package doc.
- **FakeModel only**: Config-built agents use `model.NewFakeModel`. A real model
  name can be specified in config but is informational only — no real LLM is
  called. An extension point exists (`cfg.Model`) for real model integration.
- **No plugin/instruction config in JSON**: Plugin configuration and instruction
  strings are left as extension slots. The config format focuses on topology,
  tools, and transfer constraints.
- **Workflow agents skip parent wiring**: `SetParent` is a no-op for workflow
  agent types (Sequential/Parallel/Loop). The parent chain is fully wired for
  all `llm_agent` nodes, which is sufficient for the transfer system.
- **Streaming partial aggregation is still simplified**: The config builder
  creates standard `flow.Flow` instances; streaming is unchanged from the
  existing implementation.
- **NL planning/code execution are extension slots**: No changes to the
  request processor pipeline. The Chapter 07 deep-dive mentions these as
  future work; the config format has no dedicated fields for them.
- **This is an educational replica, not API-compatible ADK Go**: The config
  format is purpose-built for teaching, not for production deployment.
