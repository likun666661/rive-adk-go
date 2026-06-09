# 10-tool-declaration-toolset

## implemented

Extended the `tool` package toward the Chapter 03 tool-system shape with the following educational types:

### Tool/Toolset hierarchy
- **`Tool` interface** — minimal public interface (`Name()`, `Description()`) as the base for all tool types.
- **`FunctionTool` interface** — extends `Tool` with `Run(args)`, keeping backward compatibility with all existing code.
- **`Declaration` struct** — tool metadata for LLM requests: `Name`, `Description`, `InputSchema`, `OutputSchema` (all lightweight `map[string]any` schema maps).
- **`DeclarationProvider` interface** — optional interface exposing `Declaration()` for tools that declare their shape to the LLM.
- **`FuncTool`** — now carries a `Declaration` field and implements `DeclarationProvider`. `NewFunctionToolWithDeclaration` constructor added alongside existing `NewFunctionTool`.
- **`FunctionToolAsTool`** — convenience adaptor to use `FunctionTool` values where `Tool` is expected.

### Toolset collections
- **`Toolset` interface** — `Name()` + `Tools() ([]Tool, error)` for dynamic tool collections.
- **`StaticToolset`** — concrete implementation backed by a static slice, satisfying `Toolset`.

### Filtering
- **`Predicate`** — `func(tool Tool) bool` for filter functions.
- **`AllowedToolsPredicate`** — returns a `Predicate` that allows only tools whose names appear in a given allow-list.
- **`FilterToolset`** — decorator wrapping a `Toolset` with a `Predicate`. Only tools matching the predicate are returned.

### Request processing
- **`RequestProcessor` interface** — `ProcessRequest(req *model.LLMRequest) error` for injecting tool declarations into LLM requests.
- **`InjectDeclarations(req, tools)`** — collects declarations from tools implementing `DeclarationProvider` and sets them on `req.ToolDeclarations` sorted by name.
- **`CollectDeclarations(tools)`** — gathers and sorts declarations from tools; used internally by `InjectDeclarations`.

### model.LLMRequest extension
- Added `ToolDeclarations []any` field to `model.LLMRequest` to carry tool metadata for model consumption.

## files_changed

| File | Change |
|------|--------|
| `tool/tool.go` | Extended with `Tool`, `Declaration`, `DeclarationProvider`, `Toolset`, `StaticToolset`, `Predicate`, `AllowedToolsPredicate`, `FilterToolset`, `RequestProcessor`, `InjectDeclarations`, `CollectDeclarations`, `FunctionToolAsTool`. `FuncTool` now has `decl Declaration` field and `Declaration()` method. |
| `tool/tool_declaration_test.go` | 32 new tests covering all new types and backward compatibility. |
| `model/model.go` | Added `ToolDeclarations []any` field to `LLMRequest`. |

## tests

All tests pass (`go test ./...` — 110+ tests across all packages). New test coverage:

1. **Tool interface + backward compat** — `FunctionTool` satisfies `Tool`; existing `Run()`, `Execute()`, `MergeResults()` continue to work.
2. **Stable declarations** — `Declaration()` returns consistent values; empty declarations are not collected.
3. **Toolset** — `StaticToolset` returns correct tools; handles empty sets.
4. **Filtering** — `AllowedToolsPredicate` correctly allows/rejects; `FilterToolset` applies predicates; edge cases (all blocked, allow-all, empty predicate).
5. **Declaration injection** — `InjectDeclarations` produces deterministic, name-sorted output across repeated calls; handles nil request; skips tools without declarations; does not mutate input.
6. **RequestProcessor** — injects sorted declarations; tracks call count.
7. **Full pipeline** — Toolset → filter → inject produces deterministic ordered output.
8. **Backward compatibility** — existing `Execute`, `MergeResults`, and flow tests pass without modification.

## notes

- `Declaration.InputSchema` and `Declaration.OutputSchema` use `map[string]any` (JSON-Schema–like maps) to avoid importing heavy schema libraries, keeping the code educational and dependency-light.
- Declarations in `LLMRequest.ToolDeclarations` are stored as `[]any` to avoid a `model → tool` import cycle (since `tool` already imports `model`). Consumers type-assert back to `Declaration`.
- `CollectDeclarations` sorts by name using `sort.Slice` to guarantee deterministic ordering regardless of tool registration order.
- The `Predicate` type uses `Tool` (not `FunctionTool`) so filtering works on any tool type.
- `FunctionToolAsTool` is a trivial identity adaptor that avoids verbose interface assertions at call sites.
