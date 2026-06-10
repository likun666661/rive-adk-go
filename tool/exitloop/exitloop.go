package exitloop

import "github.com/likun666661/rive-adk-go/tool"

type ExitLoopTool struct{}

func NewExitLoopTool() *ExitLoopTool {
	return &ExitLoopTool{}
}

func (e *ExitLoopTool) Name() string             { return "exit_loop" }
func (e *ExitLoopTool) Description() string      { return "Signal that the current invocation should end immediately." }
func (e *ExitLoopTool) IsLongRunning() bool       { return false }

func (e *ExitLoopTool) Declaration() tool.Declaration {
	return tool.Declaration{
		Name:        "exit_loop",
		Description: e.Description(),
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func (e *ExitLoopTool) Run(args map[string]any) (map[string]any, error) {
	return map[string]any{"ended": true}, nil
}

func (e *ExitLoopTool) RunWithContext(tc tool.ToolContext, args map[string]any) (map[string]any, error) {
	if actions := tc.Actions(); actions != nil {
		actions.EndInvocation = true
	}
	return map[string]any{"ended": true}, nil
}

var _ tool.Tool = (*ExitLoopTool)(nil)
var _ tool.DeclarationProvider = (*ExitLoopTool)(nil)
var _ tool.FunctionTool = (*ExitLoopTool)(nil)
var _ tool.ContextFunctionTool = (*ExitLoopTool)(nil)
