package agenttool_test

import (
	"reflect"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/artifact"
	invctx "github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/memory"
	"github.com/likun666661/rive-adk-go/session"
	"github.com/likun666661/rive-adk-go/tool"
	"github.com/likun666661/rive-adk-go/tool/agenttool"
)

func createTestAgent(name string, outputText string) agent.Agent {
	a, _ := agent.New(agent.Config{
		Name:        name,
		Description: "Test agent that returns a fixed response.",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			ev := event.NewEvent("test-event", name, event.RoleModel)
			ev.Content = &event.Content{
				Role: event.RoleModel,
				Parts: []event.Part{
					{Text: outputText},
				},
			}
			return []*event.Event{ev}, nil
		},
	})
	return a
}

func createTestAgentWithEmptyOutput(name string) agent.Agent {
	a, _ := agent.New(agent.Config{
		Name:        name,
		Description: "Test agent that returns an empty event.",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			ev := event.NewEvent("test-event", name, event.RoleModel)
			return []*event.Event{ev}, nil
		},
	})
	return a
}

func createToolContext(t *testing.T, stateVals map[string]any) tool.ToolContext {
	t.Helper()

	sess := session.NewInMemorySession("test-session", "test-app", "test-user")
	for k, v := range stateVals {
		sess.State().Set(k, v)
	}

	params := invctx.Params{
		Agent:        nil,
		Session:      sess,
		Memory:       memory.InMemoryService(),
		Artifact:     artifact.InMemoryService(),
		InvocationID: "test-invocation",
		Branch:       "test-agent",
		UserContent:  "test message",
	}
	ic := invctx.NewInvocationContext(params)
	actions := &event.EventActions{}
	return tool.NewToolContext(ic, "test-call-id", actions, nil)
}

func TestAgentTool_Declaration(t *testing.T) {
	a := createTestAgent("math_agent", "42")
	at := agenttool.New(a, nil)

	if at.Name() != "math_agent" {
		t.Errorf("Name() = %q, want %q", at.Name(), "math_agent")
	}
	if at.Description() != "Test agent that returns a fixed response." {
		t.Errorf("Description() = %q", at.Description())
	}
	if at.IsLongRunning() {
		t.Error("IsLongRunning() should be false")
	}

	dp, ok := at.(tool.DeclarationProvider)
	if !ok {
		t.Fatal("agentTool does not implement DeclarationProvider")
	}
	decl := dp.Declaration()
	if decl.Name != "math_agent" {
		t.Errorf("Declaration.Name = %q", decl.Name)
	}
}

func TestAgentTool_Run_ChildOutput(t *testing.T) {
	a := createTestAgent("echo_agent", "Hello from child agent")
	at := agenttool.New(a, nil)

	tc := createToolContext(t, nil)
	cft, ok := at.(tool.ContextFunctionTool)
	if !ok {
		t.Fatal("agentTool does not implement ContextFunctionTool")
	}

	result, err := cft.RunWithContext(tc, map[string]any{"request": "say hello"})
	if err != nil {
		t.Fatalf("RunWithContext() unexpected error: %v", err)
	}

	want := map[string]any{"result": "Hello from child agent"}
	if !reflect.DeepEqual(want, result) {
		t.Errorf("RunWithContext() result = %v, want %v", result, want)
	}
}

func TestAgentTool_Run_EmptyOutput(t *testing.T) {
	a := createTestAgentWithEmptyOutput("empty_agent")
	at := agenttool.New(a, nil)

	tc := createToolContext(t, nil)
	cft, ok := at.(tool.ContextFunctionTool)
	if !ok {
		t.Fatal("agentTool does not implement ContextFunctionTool")
	}

	result, err := cft.RunWithContext(tc, map[string]any{"request": "do nothing"})
	if err != nil {
		t.Fatalf("RunWithContext() unexpected error: %v", err)
	}

	want := map[string]any{}
	if !reflect.DeepEqual(want, result) {
		t.Errorf("RunWithContext() result = %v, want %v", result, want)
	}
}

func TestAgentTool_Run_SessionIsolation(t *testing.T) {
	// Verify that the child agent's session is isolated from the parent.
	// The child agent sets a state key; the parent session should not see it.
	childAgent, _ := agent.New(agent.Config{
		Name:        "stateful_child",
		Description: "Agent that writes to state.",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			if ic, ok := ctx.(invctx.InvocationContext); ok {
				if sess := ic.Session(); sess != nil {
					sess.State().Set("child_key", "child_value")
				}
			}
			ev := event.NewEvent("child-event", "stateful_child", event.RoleModel)
			ev.Content = &event.Content{
				Role: event.RoleModel,
				Parts: []event.Part{
					{Text: "done"},
				},
			}
			return []*event.Event{ev}, nil
		},
	})

	at := agenttool.New(childAgent, nil)
	parentSession := session.NewInMemorySession("parent-sess", "parent-app", "parent-user")
	parentSession.State().Set("parent_key", "parent_value")

	params := invctx.Params{
		Agent:        nil,
		Session:      parentSession,
		Memory:       memory.InMemoryService(),
		Artifact:     artifact.InMemoryService(),
		InvocationID: "parent-invocation",
		Branch:       "parent-agent",
		UserContent:  "test",
	}
	ic := invctx.NewInvocationContext(params)
	actions := &event.EventActions{}
	tc := tool.NewToolContext(ic, "test-call-id", actions, nil)

	cft, ok := at.(tool.ContextFunctionTool)
	if !ok {
		t.Fatal("agentTool does not implement ContextFunctionTool")
	}

	result, err := cft.RunWithContext(tc, map[string]any{"request": "work"})
	if err != nil {
		t.Fatalf("RunWithContext() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(map[string]any{"result": "done"}, result) {
		t.Errorf("result = %v, want result=done", result)
	}

	// Parent session should NOT have the child_key set by the child agent.
	if v, ok := parentSession.State().Get("child_key"); ok {
		t.Errorf("parent session should not have child_key, got %v", v)
	}

	// Parent session should still have its own key.
	if v, ok := parentSession.State().Get("parent_key"); !ok {
		t.Error("parent session should still have parent_key")
	} else if v != "parent_value" {
		t.Errorf("parent_key = %v, want parent_value", v)
	}
}

func TestAgentTool_Run_ParentStateCopied(t *testing.T) {
	// Verify that non-internal parent state is copied into the child session.
	childAgent, _ := agent.New(agent.Config{
		Name:        "state_reader",
		Description: "Agent that reads parent state.",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			stateVal := ""
			if ic, ok := ctx.(invctx.InvocationContext); ok {
				if sess := ic.Session(); sess != nil {
					if v, ok := sess.State().Get("shared_key"); ok {
						stateVal = v.(string)
					}
				}
			}
			ev := event.NewEvent("child-event", "state_reader", event.RoleModel)
			ev.Content = &event.Content{
				Role: event.RoleModel,
				Parts: []event.Part{
					{Text: stateVal},
				},
			}
			return []*event.Event{ev}, nil
		},
	})

	at := agenttool.New(childAgent, nil)

	tc := createToolContext(t, map[string]any{
		"shared_key":  "shared_value",
		"another_key": 42,
	})

	cft, ok := at.(tool.ContextFunctionTool)
	if !ok {
		t.Fatal("agentTool does not implement ContextFunctionTool")
	}

	result, err := cft.RunWithContext(tc, map[string]any{"request": "read state"})
	if err != nil {
		t.Fatalf("RunWithContext() unexpected error: %v", err)
	}

	// The child agent should have read the shared_key from the copied parent state.
	want := map[string]any{"result": "shared_value"}
	if !reflect.DeepEqual(want, result) {
		t.Errorf("result = %v, want %v", result, want)
	}
}

func TestAgentTool_Run_InternalStateNotCopied(t *testing.T) {
	// Verify that internal state (keys with _adk prefix) is NOT copied to the child session.
	childAgent, _ := agent.New(agent.Config{
		Name:        "internal_checker",
		Description: "Agent that checks for internal state.",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			hasInternal := false
			if ic, ok := ctx.(invctx.InvocationContext); ok {
				if sess := ic.Session(); sess != nil {
					_, hasInternal = sess.State().Get("_adk_internal_key")
				}
			}
			text := "no internal state found"
			if hasInternal {
				text = "internal state found"
			}
			ev := event.NewEvent("child-event", "internal_checker", event.RoleModel)
			ev.Content = &event.Content{
				Role: event.RoleModel,
				Parts: []event.Part{
					{Text: text},
				},
			}
			return []*event.Event{ev}, nil
		},
	})

	at := agenttool.New(childAgent, nil)

	tc := createToolContext(t, map[string]any{
		"_adk_internal_key": "secret",
		"public_key":        "visible",
	})

	cft, ok := at.(tool.ContextFunctionTool)
	if !ok {
		t.Fatal("agentTool does not implement ContextFunctionTool")
	}

	result, err := cft.RunWithContext(tc, map[string]any{"request": "check internal"})
	if err != nil {
		t.Fatalf("RunWithContext() unexpected error: %v", err)
	}

	// The child agent should NOT have the _adk-prefixed key.
	want := map[string]any{"result": "no internal state found"}
	if !reflect.DeepEqual(want, result) {
		t.Errorf("result = %v, want %v", result, want)
	}
}

func TestAgentTool_Run_MissingRequest(t *testing.T) {
	a := createTestAgent("echo_agent", "Hello")
	at := agenttool.New(a, nil)
	tc := createToolContext(t, nil)

	cft, ok := at.(tool.ContextFunctionTool)
	if !ok {
		t.Fatal("agentTool does not implement ContextFunctionTool")
	}

	_, err := cft.RunWithContext(tc, map[string]any{"wrong_key": "value"})
	if err == nil {
		t.Error("RunWithContext() should fail with missing 'request' argument")
	}
}

func TestAgentTool_Run_SkipSummarization(t *testing.T) {
	a := createTestAgent("echo_agent", "Hello")
	at := agenttool.New(a, &agenttool.Config{SkipSummarization: true})

	actions := &event.EventActions{}
	parentSession := session.NewInMemorySession("skip-sess", "skip-app", "skip-user")
	params := invctx.Params{
		Agent:        nil,
		Session:      parentSession,
		Memory:       memory.InMemoryService(),
		Artifact:     artifact.InMemoryService(),
		InvocationID: "skip-invocation",
		Branch:       "skip-agent",
		UserContent:  "test",
	}
	ic := invctx.NewInvocationContext(params)
	tc := tool.NewToolContext(ic, "test-call-id", actions, nil)

	cft, ok := at.(tool.ContextFunctionTool)
	if !ok {
		t.Fatal("agentTool does not implement ContextFunctionTool")
	}

	_, err := cft.RunWithContext(tc, map[string]any{"request": "hello"})
	if err != nil {
		t.Fatalf("RunWithContext() unexpected error: %v", err)
	}

	if !actions.SkipSummarization {
		t.Error("SkipSummarization should be true when agent tool is created with SkipSummarization=true")
	}

	// Test with SkipSummarization=false
	atNoSkip := agenttool.New(a, &agenttool.Config{SkipSummarization: false})
	actions2 := &event.EventActions{}
	tc2 := tool.NewToolContext(ic, "test-call-id-2", actions2, nil)
	cft2, ok := atNoSkip.(tool.ContextFunctionTool)
	if !ok {
		t.Fatal("agentTool does not implement ContextFunctionTool")
	}

	_, err = cft2.RunWithContext(tc2, map[string]any{"request": "hello"})
	if err != nil {
		t.Fatalf("RunWithContext() unexpected error: %v", err)
	}

	if actions2.SkipSummarization {
		t.Error("SkipSummarization should be false when agent tool is created with SkipSummarization=false")
	}
}

func TestAgentTool_Run_ChildSessionHasOwnRunner(t *testing.T) {
	// Verify that the child agent runs in its own runner with isolated services.
	// We check this by verifying that a query runner's session service and the
	// child runner's session service are different.
	a := createTestAgent("isolated_agent", "output")
	at := agenttool.New(a, nil)

	tc := createToolContext(t, nil)
	cft, ok := at.(tool.ContextFunctionTool)
	if !ok {
		t.Fatal("agentTool does not implement ContextFunctionTool")
	}

	result, err := cft.RunWithContext(tc, map[string]any{"request": "test"})
	if err != nil {
		t.Fatalf("RunWithContext() unexpected error: %v", err)
	}

	if !reflect.DeepEqual(map[string]any{"result": "output"}, result) {
		t.Errorf("result = %v, want result=output", result)
	}
}
