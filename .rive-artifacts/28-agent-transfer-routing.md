## implemented

Chapter 07 agent tree / transfer-to-agent / active-agent routing is fully
implemented across four packages:

1. **Agent tree metadata** (`agent/agent.go`)
   - `Agent` interface extended with `SubAgents()`, `FindAgent(name)`, `Parent()`,
     `DisallowTransferToParent()`, `DisallowTransferToPeers()`.
   - `Config` struct accepts `SubAgents`, `Parent`, `DisallowTransferToParent`,
     `DisallowTransferToPeers` fields.
   - `baseAgent.FindAgent()` performs depth-first search by name.
   - Preserves backward compatibility with all existing `agent.Agent` usage.

2. **Transfer tool** (`tool/transfer/tool.go`)
   - `TransferToAgentTool` implements `Tool`, `FunctionTool`, `ContextFunctionTool`,
     `DeclarationProvider`.
   - `Declaration()` exposes `agent_name` parameter with an enum of allowed targets.
   - `Run()` validates target name; `RunWithContext()` sets `EventActions.TransferToAgent`.
   - `ComputeTransferTargets(a agent.Agent)` computes allowed targets:
     * All sub-agents always included.
     * Parent included unless `DisallowTransferToParent` is true.
     * Peers included unless `DisallowTransferToPeers` is true and parent is an auto-flow agent.
   - `InjectTransferTool()` injects the transfer tool declaration and transfer
     instructions into `model.LLMRequest` when applicable.

3. **Flow transfer execution** (`flow/flow.go`)
   - `injectTransferTool()` is called each step in `runOneStep` to populate
     `activeTransferTool`.
   - `handleFunctionCalls()` detects `TransferToAgent` on the merged tool event
     and calls `executeTransfer()`.
   - `executeTransfer()` resolves the target via `RootAgent().FindAgent()`,
     creates a `transferContext` wrapping the invocation context with the
     target agent as `Agent()`, validates `maxTransferDepth` (10) to prevent
     infinite loops, executes the target inline, and chains further transfers.
   - `transferContext` overrides `Agent()`, `AgentName()`, and `Branch()`.

4. **Runner active-agent routing** (`runner/runner.go`)
   - `findAgentToRun()` scans session events backward, skips `user`-authored
     events, resolves the event author via `FindAgent()`, and checks
     `isTransferableAcrossAgentTree()`.
   - `isTransferableAcrossAgentTree()` walks the parent chain and returns false
     if any ancestor has `DisallowTransferToParent == true`.
   - Falls back to the root agent if no transferable target is found.

## files_changed

- `agent/agent.go` — Agent interface with tree metadata; baseAgent implementation
- `flow/flow.go` — injectTransferTool, executeTransfer, transferContext,
  transferDepth guard, activeTransferTool lookup
- `runner/runner.go` — findAgentToRun, isTransferableAcrossAgentTree
- `event/event.go` — EventActions.TransferToAgent (already present)
- `tool/transfer/tool.go` — TransferToAgentTool, ComputeTransferTargets,
  InjectTransferTool, TransferInstructions (new package)
- `tool/tool.go` — ContextFunctionTool interface, ContextExecute (already present)

## tests

All transfer acceptance criteria covered:

| Test | Location | Criteria |
|------|----------|----------|
| `TestTransferToolDeclarationHasAllowedNames` | `tool/transfer/tool_test.go` | Declaration has agent name enum |
| `TestTransferToolValidTarget` | `tool/transfer/tool_test.go` | Valid transfer returns success |
| `TestTransferToolInvalidTargetYieldsError` | `tool/transfer/tool_test.go` | Invalid target yields structured error |
| `TestTransferToolMissingAgentName` | `tool/transfer/tool_test.go` | Missing agent_name yields error |
| `TestComputeTransferTargets*` (5 tests) | `tool/transfer/tool_test.go` | Target computation covers parent/peer/sub rules |
| `TestTransferInstructions` | `tool/transfer/tool_test.go` | Instructions include agent names and descriptions |
| `TestFlowTransferToSubAgent` | `flow/flow_test.go` | Model-triggered transfer delegates to target |
| `TestFlowTransferInvalidTarget` | `flow/flow_test.go` | Invalid transfer target yields structured tool error |
| `TestFlowTransferLoopDetection` | `flow/flow_test.go` | Max depth guard detects infinite transfer loop |
| `TestFlowTransferToParent` | `flow/flow_test.go` | Child-to-parent transfer works |
| `TestFlowTransferWithoutSubAgentsHasEmptyTargets` | `flow/flow_test.go` | Agents without targets don't inject transfer tool |
| `TestRunnerFindAgentToRun*` (3 tests) | `runner/runner_test.go` | Active agent routing from history |
| `TestRunnerIsTransferable*` (2 tests) | `runner/runner_test.go` | isTransferableAcrossAgentTree with parent chain checks |
| `TestRunnerTransferFullChain` | `runner/runner_test.go` | End-to-end transfer in runner |
| `TestRunnerSecondRunRoutesToActiveAgent` | `runner/runner_test.go` | Second run routes to active agent from history |

All 28 test packages pass (`go test ./...`), `go vet ./...` reports no warnings,
and `git diff --check` reports no whitespace issues.

## notes

The implementation follows the Chapter 07 deep-dive design with these educational
simplifications compared to full ADK Go:

- **No SingleFlow/AutoFlow distinction**: The replica uses a simpler model where
  transfer is available whenever `ComputeTransferTargets` returns non-empty
  targets, rather than the formal `shouldUseAutoFlow` check.
- **No `llminternal.Agent` interface**: The agent tree uses only the public
  `agent.Agent` interface; the internal `DisallowTransferToParent` check is
  available directly on the interface.
- **No partial-to-full event merging in transfer**: Unlike ADK Go's
  `mergeParallelFunctionResponseEvents`, this implementation merges actions
  from parallel tool results inline in `mergeResultsToEvent`.
- **No plugin manager integration for transfer tool**: The transfer tool is
  injected directly into the LLM request rather than through a plugin or
  request processor pipeline hook.
- **Workflow agent SubAgents list sub-agents**: `SequentialAgent`, `ParallelAgent`,
  and `LoopAgent` expose their sub-agents via the `Agent` interface, enabling
  `ComputeTransferTargets` to include them as transfer targets.
