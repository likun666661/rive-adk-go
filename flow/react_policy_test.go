package flow

import (
	"errors"
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/plugin"
	"github.com/likun666661/rive-adk-go/plugin/functionmodifier"
	"github.com/likun666661/rive-adk-go/plugin/retryreflect"
	"github.com/likun666661/rive-adk-go/tool"
	"github.com/likun666661/rive-adk-go/tool/exitloop"

	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/model"
)

// =============================================================================
// Chapter 07 — ReAct policy extension tests
// =============================================================================

// exit_loop stops a multi-step ReAct run.
func TestFlowExitLoopStopsMultiStep(t *testing.T) {
	ctx := newTestCtx("exit_agent")

	exitTool := exitloop.NewExitLoopTool()
	f := &Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Ending now.",
				event.FunctionCall{ID: "fc1", Name: "exit_loop", Args: map[string]any{}},
			),
			model.TextResponse("This should not be reached."),
		),
		Tools: map[string]tool.FunctionTool{
			"exit_loop": exitTool,
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !ctx.Ended() {
		t.Error("context should be ended after exit_loop")
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events (model fc + tool EndInvocation), got %d", len(events))
	}

	ev2 := events[1]
	if ev2.Role != event.RoleTool {
		t.Errorf("event 2 role = %q, want 'tool'", ev2.Role)
	}
	if !ev2.Actions.EndInvocation {
		t.Error("tool event should have EndInvocation = true")
	}
}

// exit_loop in the middle of a multi-step chain stops immediately.
func TestFlowExitLoopAfterToolCall(t *testing.T) {
	ctx := newTestCtx("exit_multi_agent")

	weatherTool := tool.NewFunctionTool("get_weather", "Get weather",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"temperature": 22}, nil
		},
	)
	exitTool := exitloop.NewExitLoopTool()

	f := &Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Checking weather.",
				event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
			),
			model.FunctionCallResponse("Now ending.",
				event.FunctionCall{ID: "fc2", Name: "exit_loop", Args: map[string]any{}},
			),
			model.TextResponse("Should not appear."),
		),
		Tools: map[string]tool.FunctionTool{
			"get_weather": weatherTool,
			"exit_loop":   exitTool,
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !ctx.Ended() {
		t.Error("context should be ended after exit_loop")
	}

	if len(events) != 4 {
		t.Fatalf("expected 4 events (model fc + tool + model fc + exit), got %d", len(events))
	}

	lastEv := events[len(events)-1]
	if !lastEv.Actions.EndInvocation {
		t.Error("last event should have EndInvocation")
	}
}

// exit_loop stops even when there are more model responses queued (in a
// multi-step flow where exit_loop appears mid-sequence).
func TestFlowExitLoopSkipsRemainingQueue(t *testing.T) {
	ctx := newTestCtx("exit_queue_agent")

	// Use a simple tool that succeeds.
	simpleTool := tool.NewFunctionTool("do_work", "Do some work",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"done": true}, nil
		},
	)
	exitTool := exitloop.NewExitLoopTool()

	f := &Flow{
		Model: model.NewFakeModel("fake",
			// Step 1: do_work
			model.FunctionCallResponse("Working...",
				event.FunctionCall{ID: "fc1", Name: "do_work", Args: map[string]any{}},
			),
			// Step 2: exit_loop (should stop here)
			model.FunctionCallResponse("Exit now.",
				event.FunctionCall{ID: "fc2", Name: "exit_loop", Args: map[string]any{}},
			),
			// Step 3 should never execute
			model.TextResponse("Unreachable."),
		),
		Tools: map[string]tool.FunctionTool{
			"do_work":   simpleTool,
			"exit_loop": exitTool,
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !ctx.Ended() {
		t.Error("context should be ended")
	}

	// The third model response should never be reached
	for _, ev := range events {
		if ev.Content != nil {
			for _, p := range ev.Content.Parts {
				if p.Text == "Unreachable." {
					t.Error("'Unreachable' model response was consumed after exit_loop")
				}
			}
		}
	}
}

// =============================================================================
// Tool failure reflection tests
// =============================================================================

// tool failure produces reflection content visible to the model.
func TestFlowRetryReflectPlugin(t *testing.T) {
	ctx := newTestCtx("reflect_agent")

	failingTool := tool.NewFunctionTool("unreliable", "Always fails",
		func(args map[string]any) (map[string]any, error) {
			return nil, errors.New("internal error")
		},
	)

	rrp := retryreflect.New(retryreflect.Config{
		Name:       "retry_reflect",
		MaxRetries: 2,
	})

	pm := plugin.NewManager()
	pm.Register(rrp.Plugin)

	f := &Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Calling tool.",
				event.FunctionCall{ID: "fc1", Name: "unreliable", Args: map[string]any{}},
			),
			model.TextResponse("Let me try again."),
		),
		Tools:         map[string]tool.FunctionTool{"unreliable": failingTool},
		PluginManager: pm,
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	ev2 := events[1]
	if ev2.Role != event.RoleTool {
		t.Errorf("event 2 role = %q, want 'tool'", ev2.Role)
	}

	fr := ev2.Content.Parts[0].FunctionResponse
	if fr == nil {
		t.Fatal("expected function response")
	}
	// The original error is preserved in Result["error"] and not hidden.
	if _, ok := fr.Result["error"]; !ok {
		t.Error("expected error field preserved in tool result")
	} else {
		errMsg, _ := fr.Result["error"].(string)
		if errMsg != "internal error" {
			t.Errorf("result[error] = %q, want 'internal error'", errMsg)
		}
	}
	if _, ok := fr.Result["reflection"]; !ok {
		t.Error("expected reflection field in tool result")
	}
	if rrp.FailureCount("unreliable") != 1 {
		t.Errorf("failure count = %d, want 1", rrp.FailureCount("unreliable"))
	}
}

// tool failure with reflection can be retried and then resolved.
func TestFlowRetryReflectThenResolve(t *testing.T) {
	ctx := newTestCtx("resolve_agent")

	var callCount int
	failingThenWorking := tool.NewFunctionTool("flaky", "Fails then works",
		func(args map[string]any) (map[string]any, error) {
			callCount++
			if callCount <= 1 {
				return nil, errors.New("transient error")
			}
			return map[string]any{"status": "ok"}, nil
		},
	)

	rrp := retryreflect.New(retryreflect.Config{
		Name:       "retry_reflect",
		MaxRetries: 3,
	})

	pm := plugin.NewManager()
	pm.Register(rrp.Plugin)

	f := &Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("First attempt.",
				event.FunctionCall{ID: "fc1", Name: "flaky", Args: map[string]any{}},
			),
			model.FunctionCallResponse("Second attempt.",
				event.FunctionCall{ID: "fc2", Name: "flaky", Args: map[string]any{}},
			),
			model.TextResponse("It worked!"),
		),
		Tools:         map[string]tool.FunctionTool{"flaky": failingThenWorking},
		PluginManager: pm,
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(events) != 5 {
		t.Fatalf("expected 5 events (2 model fc + 2 tool + final), got %d", len(events))
	}

	ev2 := events[1]
	fr2 := ev2.Content.Parts[0].FunctionResponse
	// Original error preserved in Result["error"]
	if _, ok := fr2.Result["error"]; !ok {
		t.Error("first call should have error in result")
	}
	if _, ok := fr2.Result["reflection"]; !ok {
		t.Error("first call should have reflection")
	}
	ev4 := events[3]
	fr4 := ev4.Content.Parts[0].FunctionResponse
	if fr4.Result["status"] != "ok" {
		t.Errorf("second call result = %v", fr4.Result)
	}
	// After success, counter should reset
	if rrp.FailureCount("flaky") != 0 {
		t.Errorf("failure count after success = %d, want 0", rrp.FailureCount("flaky"))
	}

	lastEv := events[len(events)-1]
	if !lastEv.IsFinalResponse() {
		t.Error("last event should be final")
	}
}

// original tool error is NOT hidden from events.
func TestFlowRetryReflectPreservesOriginalError(t *testing.T) {
	ctx := newTestCtx("error_preserve_agent")

	failingTool := tool.NewFunctionTool("crash", "Always fails",
		func(args map[string]any) (map[string]any, error) {
			return nil, errors.New("original database error")
		},
	)

	rrp := retryreflect.New(retryreflect.Config{
		Name:       "retry_reflect",
		MaxRetries: 1,
	})

	pm := plugin.NewManager()
	pm.Register(rrp.Plugin)

	f := &Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Calling crash.",
				event.FunctionCall{ID: "fc1", Name: "crash", Args: map[string]any{}},
			),
			model.TextResponse("Fallback."),
		),
		Tools:         map[string]tool.FunctionTool{"crash": failingTool},
		PluginManager: pm,
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	ev2 := events[1]
	fr := ev2.Content.Parts[0].FunctionResponse

	// The original error is preserved in Result["error"]
	frErr, ok := fr.Result["error"].(string)
	if !ok || !strings.Contains(frErr, "original database error") {
		t.Errorf("result[error] = %v, should contain 'original database error'", fr.Result["error"])
	}
	if _, ok := fr.Result["reflection"]; !ok {
		t.Error("result should have reflection field")
	}
	if rrp.FailureCount("crash") != 1 {
		t.Errorf("failure count = %d, want 1", rrp.FailureCount("crash"))
	}
}

// =============================================================================
// Hidden arg injection (function call modifier)
// =============================================================================

// hidden arg injection works without appearing in the model request.
func TestFlowFunctionCallModifierHiddenArgs(t *testing.T) {
	ctx := newTestCtx("hidden_agent")

	var capturedArgs map[string]any
	searchTool := tool.NewFunctionTool("search", "Search tool",
		func(args map[string]any) (map[string]any, error) {
			capturedArgs = args
			return map[string]any{"results": "found"}, nil
		},
	)

	fcp := functionmodifier.New(functionmodifier.Config{
		Name: "hidden_modifier",
		Predicate: func(name string) bool {
			return name == "search"
		},
		HiddenArgs: map[string]any{
			"user_id":  "usr-789",
			"internal": true,
		},
	})

	pm := plugin.NewManager()
	pm.Register(fcp.Plugin)

	f := &Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Searching.",
				event.FunctionCall{ID: "fc1", Name: "search", Args: map[string]any{"query": "hello"}},
			),
			model.TextResponse("Done."),
		),
		Tools:         map[string]tool.FunctionTool{"search": searchTool},
		PluginManager: pm,
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	if capturedArgs == nil {
		t.Fatal("tool was never called")
	}

	// The tool should NOT see the hidden args (they're stripped before execution)
	if _, ok := capturedArgs["user_id"]; ok {
		t.Error("user_id should be stripped before tool execution")
	}
	if _, ok := capturedArgs["internal"]; ok {
		t.Error("internal should be stripped before tool execution")
	}
	if capturedArgs["query"] != "hello" {
		t.Errorf("query = %q, want 'hello'", capturedArgs["query"])
	}
}

// hidden args are not visible in the tool declaration for non-matching tools.
func TestFlowFunctionCallModifierOnlyMatchesPredicate(t *testing.T) {
	ctx := newTestCtx("predicate_agent")

	var capturedArgs map[string]any
	publicTool := tool.NewFunctionTool("public", "Public tool",
		func(args map[string]any) (map[string]any, error) {
			capturedArgs = args
			return map[string]any{"ok": true}, nil
		},
	)

	fcp := functionmodifier.New(functionmodifier.Config{
		Name: "predicate_modifier",
		Predicate: func(name string) bool {
			return name == "sensitive"
		},
		HiddenArgs: map[string]any{
			"api_key": "sk-xxx",
		},
	})

	pm := plugin.NewManager()
	pm.Register(fcp.Plugin)

	f := &Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Calling public.",
				event.FunctionCall{ID: "fc1", Name: "public", Args: map[string]any{"data": "value"}},
			),
			model.TextResponse("Done."),
		),
		Tools:         map[string]tool.FunctionTool{"public": publicTool},
		PluginManager: pm,
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	if capturedArgs == nil {
		t.Fatal("tool was never called")
	}
	if _, ok := capturedArgs["api_key"]; ok {
		t.Error("api_key should not be injected into non-matching tool")
	}
}

// =============================================================================
// Transfer behaviour from Node 1 still passes
// =============================================================================

func TestFlowTransferStillWorksWithPolicyPlugins(t *testing.T) {
	targetAgent := newTransferTargetAgent("math_bot", "Solves math problems", "42")
	parentAgent, err := agent.New(agent.Config{
		Name:        "root",
		Description: "Root agent",
		SubAgents:   []agent.Agent{targetAgent},
		Run:         func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := newTransferTestCtx(parentAgent)

	rrp := retryreflect.New(retryreflect.Config{Name: "rr", MaxRetries: 1})
	fcp := functionmodifier.New(functionmodifier.Config{
		Name: "fc",
		Predicate: func(name string) bool {
			return name == "transfer_to_agent"
		},
		HiddenArgs: map[string]any{"internal": "true"},
	})

	pm := plugin.NewManager()
	pm.Register(rrp.Plugin)
	pm.Register(fcp.Plugin)

	f := &Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Transferring.",
				event.FunctionCall{ID: "fc1", Name: "transfer_to_agent", Args: map[string]any{"agent_name": "math_bot"}},
			),
		),
		PluginManager: pm,
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	ev2 := events[1]
	if ev2.Actions.TransferToAgent != "math_bot" {
		t.Errorf("TransferToAgent = %q, want 'math_bot'", ev2.Actions.TransferToAgent)
	}

	lastEv := events[len(events)-1]
	if lastEv.Author != "math_bot" {
		t.Errorf("last event Author = %q, want 'math_bot'", lastEv.Author)
	}
}
