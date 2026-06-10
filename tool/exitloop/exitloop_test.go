package exitloop

import (
	"testing"

	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/tool"
)

func TestExitLoopName(t *testing.T) {
	el := NewExitLoopTool()
	if el.Name() != "exit_loop" {
		t.Errorf("name = %q, want 'exit_loop'", el.Name())
	}
}

func TestExitLoopDescription(t *testing.T) {
	el := NewExitLoopTool()
	if el.Description() == "" {
		t.Error("description should not be empty")
	}
}

func TestExitLoopImplementsToolInterfaces(t *testing.T) {
	var _ tool.Tool = (*ExitLoopTool)(nil)
	var _ tool.DeclarationProvider = (*ExitLoopTool)(nil)
	var _ tool.FunctionTool = (*ExitLoopTool)(nil)
	var _ tool.ContextFunctionTool = (*ExitLoopTool)(nil)
}

func TestExitLoopDeclaration(t *testing.T) {
	el := NewExitLoopTool()
	dp, ok := any(el).(tool.DeclarationProvider)
	if !ok {
		t.Fatal("ExitLoopTool should implement DeclarationProvider")
	}
	decl := dp.Declaration()
	if decl.Name != "exit_loop" {
		t.Errorf("declaration name = %q, want 'exit_loop'", decl.Name)
	}
	if decl.Description == "" {
		t.Error("declaration description should not be empty")
	}
}

func TestExitLoopRunReturnsEnded(t *testing.T) {
	el := NewExitLoopTool()
	result, err := el.Run(map[string]any{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	ended, ok := result["ended"].(bool)
	if !ok || !ended {
		t.Errorf("result['ended'] = %v, want true", result["ended"])
	}
}

func TestExitLoopRunWithContextSetsEndInvocation(t *testing.T) {
	el := NewExitLoopTool()

	actions := &event.EventActions{}
	tc := tool.NewToolContext(nil, "fc-test", actions, nil)

	result, err := el.RunWithContext(tc, map[string]any{})
	if err != nil {
		t.Fatalf("RunWithContext() error = %v", err)
	}
	ended, ok := result["ended"].(bool)
	if !ok || !ended {
		t.Errorf("result['ended'] = %v, want true", result["ended"])
	}
	if !actions.EndInvocation {
		t.Error("actions.EndInvocation should be true after exit_loop call")
	}
}
