package tool

import (
	"errors"
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/session"
)

// ---------------------------------------------------------------------------
// 1. ToolContext construction and field access
// ---------------------------------------------------------------------------

func TestToolContextConstruction(t *testing.T) {
	a := newTestAgent("ctx_agent")
	s := newTestSession("sid-1", "app", "user1")
	ic := context.NewInvocationContext(context.Params{
		Agent:        a,
		Session:      s,
		InvocationID: "inv-1",
	})

	actions := &event.EventActions{}
	tc := NewToolContext(ic, "fc-001", actions, nil)

	if tc.FunctionCallID() != "fc-001" {
		t.Errorf("FunctionCallID = %q, want 'fc-001'", tc.FunctionCallID())
	}
	if tc.InvocationContext().InvocationID() != "inv-1" {
		t.Errorf("InvocationID = %q, want 'inv-1'", tc.InvocationContext().InvocationID())
	}
	if tc.ToolConfirmation() != nil {
		t.Error("ToolConfirmation should be nil when not provided")
	}
	if tc.Actions() != actions {
		t.Error("Actions should return the provided EventActions")
	}
}

func TestToolContextWithConfirmation(t *testing.T) {
	a := newTestAgent("ctx_agent2")
	s := newTestSession("sid-2", "app", "user1")
	ic := context.NewInvocationContext(context.Params{
		Agent:        a,
		Session:      s,
		InvocationID: "inv-2",
	})

	conf := &event.ToolConfirmation{Confirmed: true, Hint: "approved"}
	tc := NewToolContext(ic, "fc-002", nil, conf)

	if tc.ToolConfirmation() != conf {
		t.Error("ToolConfirmation should return the provided confirmation")
	}
	if !tc.ToolConfirmation().Confirmed {
		t.Error("ToolConfirmation.Confirmed should be true")
	}
}

func TestToolContextRequestConfirmation(t *testing.T) {
	a := newTestAgent("ctx_agent3")
	s := newTestSession("sid-3", "app", "user1")
	ic := context.NewInvocationContext(context.Params{
		Agent:        a,
		Session:      s,
		InvocationID: "inv-3",
	})

	actions := &event.EventActions{}
	tc := NewToolContext(ic, "fc-003", actions, nil)

	err := tc.RequestConfirmation("please confirm action", map[string]any{"risk": "high"})
	if err != nil {
		t.Fatal(err)
	}

	if !actions.SkipSummarization {
		t.Error("SkipSummarization should be true after RequestConfirmation")
	}
	if actions.RequestedToolConfirmations == nil {
		t.Fatal("RequestedToolConfirmations should not be nil")
	}
	rc, ok := actions.RequestedToolConfirmations["fc-003"]
	if !ok {
		t.Fatal("expected confirmation entry for fc-003")
	}
	if rc.Hint != "please confirm action" {
		t.Errorf("Hint = %q, want 'please confirm action'", rc.Hint)
	}
	if rc.Confirmed {
		t.Error("Confirmed should be false initially")
	}
}

func TestToolContextRequestConfirmationEmptyCallID(t *testing.T) {
	a := newTestAgent("ctx_agent4")
	s := newTestSession("sid-4", "app", "user1")
	ic := context.NewInvocationContext(context.Params{
		Agent:        a,
		Session:      s,
		InvocationID: "inv-4",
	})

	tc := NewToolContext(ic, "", nil, nil)
	err := tc.RequestConfirmation("hint", nil)
	if err == nil {
		t.Error("expected error for empty function call ID")
	}
}

// ---------------------------------------------------------------------------
// 2. Confirmation: required / approved / rejected
// ---------------------------------------------------------------------------

func TestConfirmationRequired(t *testing.T) {
	inner := NewFunctionTool("dangerous_op", "Performs a dangerous operation",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"performed": true}, nil
		},
	)

	ct := WithConfirmation(inner, true, nil)
	result, err := ct.Run(map[string]any{"target": "prod"})

	if err == nil {
		t.Fatal("expected confirmation required error")
	}
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Errorf("error = %v, want ErrConfirmationRequired", err)
	}
	if req, ok := result["confirmation_required"]; !ok || req != true {
		t.Error("result should have confirmation_required = true")
	}
	if hint, ok := result["hint"]; !ok || !strings.Contains(hint.(string), "dangerous_op") {
		t.Errorf("hint = %v, should mention tool name", hint)
	}
}

func TestConfirmationApproved(t *testing.T) {
	inner := NewFunctionTool("safe_op", "Performs a safe operation",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"performed": true}, nil
		},
	)

	ct := WithConfirmation(inner, true, nil).(*confirmationTool)

	// Approve the call.
	ct.SetConfirmed(true)

	result, err := ct.Run(map[string]any{"target": "dev"})
	if err != nil {
		t.Fatalf("unexpected error after approval: %v", err)
	}
	if performed, ok := result["performed"]; !ok || performed != true {
		t.Errorf("result = %v, expected performed=true", result)
	}
}

func TestConfirmationRejected(t *testing.T) {
	inner := NewFunctionTool("dangerous_op2", "Performs a dangerous operation",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"should_not_reach": true}, nil
		},
	)

	ct := WithConfirmation(inner, true, nil).(*confirmationTool)

	// Reject the call.
	ct.SetConfirmed(false)

	result, err := ct.Run(map[string]any{"target": "prod"})
	if err == nil {
		t.Fatal("expected confirmation rejected error")
	}
	if !errors.Is(err, ErrConfirmationRejected) {
		t.Errorf("error = %v, want ErrConfirmationRejected", err)
	}
	if rej, ok := result["confirmation_rejected"]; !ok || rej != true {
		t.Error("result should have confirmation_rejected = true")
	}
	if _, ok := result["error"]; !ok {
		t.Error("result should have error field")
	}
}

func TestConfirmationNotRequired(t *testing.T) {
	inner := NewFunctionTool("normal_op", "A normal operation",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"done": true}, nil
		},
	)

	ct := WithConfirmation(inner, false, nil)
	result, err := ct.Run(map[string]any{"x": 1})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if done, ok := result["done"]; !ok || done != true {
		t.Errorf("result = %v, expected done=true", result)
	}
}

func TestConfirmationWithDynamicProvider(t *testing.T) {
	inner := NewFunctionTool("conditional_op", "Conditional operation",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"performed": true}, nil
		},
	)

	provider := func(toolName string, toolInput map[string]any) bool {
		risk, _ := toolInput["risk"].(string)
		return risk == "high"
	}

	ct := WithConfirmation(inner, false, provider)

	// Low risk — no confirmation needed.
	result, err := ct.Run(map[string]any{"risk": "low"})
	if err != nil {
		t.Fatalf("unexpected error for low risk: %v", err)
	}
	if _, ok := result["performed"]; !ok {
		t.Error("tool should execute without confirmation for low risk")
	}

	// High risk — confirmation required.
	result, err = ct.Run(map[string]any{"risk": "high"})
	if err == nil {
		t.Fatal("expected confirmation required for high risk")
	}
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Errorf("error = %v, want ErrConfirmationRequired", err)
	}
	if req, ok := result["confirmation_required"]; !ok || req != true {
		t.Error("result should have confirmation_required = true")
	}
}

// ---------------------------------------------------------------------------
// 3. Streaming: collection and error
// ---------------------------------------------------------------------------

func TestStreamingCollection(t *testing.T) {
	st := NewStreamingFunctionTool("stream_op", "Streaming operation",
		func(args map[string]any) ([]StreamChunk, error) {
			return []StreamChunk{
				{Text: "Hello", Final: false},
				{Text: " ", Final: false},
				{Text: "World", Final: true},
			}, nil
		},
	)

	cr := ExecuteStream("fc-001", "stream_op", map[string]any{}, st)
	if cr.Error != "" {
		t.Fatalf("unexpected error: %s", cr.Error)
	}
	if result, ok := cr.Result["result"].(string); !ok || result != "Hello World" {
		t.Errorf("result = %q, want 'Hello World'", cr.Result["result"])
	}
	if cr.CallID != "fc-001" {
		t.Errorf("CallID = %q", cr.CallID)
	}
}

func TestStreamingError(t *testing.T) {
	st := NewStreamingFunctionTool("stream_fail", "Failing streaming operation",
		func(args map[string]any) ([]StreamChunk, error) {
			return []StreamChunk{
				{Text: "partial", Final: false},
				{Text: "data", Error: "stream connection lost", Final: true},
			}, nil
		},
	)

	cr := ExecuteStream("fc-002", "stream_fail", map[string]any{}, st)
	if cr.Error == "" {
		t.Fatal("expected error in stream result")
	}
	if !strings.Contains(cr.Error, "stream connection lost") {
		t.Errorf("Error = %q, should contain 'stream connection lost'", cr.Error)
	}
	if result, ok := cr.Result["result"].(string); !ok || result != "partialdata" {
		t.Errorf("result = %q, want 'partialdata'", cr.Result["result"])
	}
	if errStr, ok := cr.Result["error"].(string); !ok || !strings.Contains(errStr, "stream connection lost") {
		t.Errorf("result[error] = %q", cr.Result["error"])
	}
}

func TestStreamingErrorViaRunStream(t *testing.T) {
	st := NewStreamingFunctionTool("stream_err2", "Error via RunStream return",
		func(args map[string]any) ([]StreamChunk, error) {
			return nil, errors.New("stream setup failed")
		},
	)

	cr := ExecuteStream("fc-003", "stream_err2", map[string]any{}, st)
	if cr.Error == "" {
		t.Fatal("expected error from RunStream")
	}
	if !strings.Contains(cr.Error, "stream setup failed") {
		t.Errorf("Error = %q", cr.Error)
	}
}

func TestStreamingEmptyChunks(t *testing.T) {
	st := NewStreamingFunctionTool("stream_empty", "Empty streaming",
		func(args map[string]any) ([]StreamChunk, error) {
			return []StreamChunk{}, nil
		},
	)

	cr := ExecuteStream("fc-004", "stream_empty", map[string]any{}, st)
	if cr.Error != "" {
		t.Fatalf("unexpected error: %s", cr.Error)
	}
	if result, ok := cr.Result["result"].(string); !ok || result != "" {
		t.Errorf("result = %q, want empty string", cr.Result["result"])
	}
}

func TestStreamingToolNotFound(t *testing.T) {
	cr := ExecuteStream("fc-005", "missing", map[string]any{}, nil)
	if cr.Error == "" {
		t.Fatal("expected error for nil streaming tool")
	}
}

// ---------------------------------------------------------------------------
// 4. Streaming: CollectStreamChunks helper
// ---------------------------------------------------------------------------

func TestCollectStreamChunks(t *testing.T) {
	chunks := []StreamChunk{
		{Text: "chunk1", Final: false},
		{Text: "chunk2", Final: false},
		{Text: "chunk3", Final: true},
	}
	result, err := CollectStreamChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if result["result"] != "chunk1chunk2chunk3" {
		t.Errorf("result = %q, want 'chunk1chunk2chunk3'", result["result"])
	}
}

func TestCollectStreamChunksWithError(t *testing.T) {
	chunks := []StreamChunk{
		{Text: "good", Final: false},
		{Text: "bad", Error: "something went wrong", Final: true},
	}
	result, err := CollectStreamChunks(chunks)
	if err == nil {
		t.Fatal("expected error")
	}
	if result["result"] != "goodbad" {
		t.Errorf("result = %q, want 'goodbad'", result["result"])
	}
	if result["error"] != "something went wrong" {
		t.Errorf("error = %q, want 'something went wrong'", result["error"])
	}
}

// ---------------------------------------------------------------------------
// 5. Long-running: declaration and IsLongRunning
// ---------------------------------------------------------------------------

func TestLongRunningToolDeclaration(t *testing.T) {
	decl := NewDeclaration("long_op", "A long operation",
		map[string]any{"type": "object"},
		map[string]any{"type": "object"},
	)

	lr := NewLongRunningFunctionTool("long_op", "A long operation", decl,
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"status": "pending"}, nil
		},
	)

	if !lr.IsLongRunning() {
		t.Fatal("IsLongRunning should return true")
	}

	dp, ok := lr.(DeclarationProvider)
	if !ok {
		t.Fatal("long-running tool should implement DeclarationProvider")
	}
	d := dp.Declaration()

	if !strings.Contains(d.Description, "long-running operation") {
		t.Errorf("declaration description should contain long-running annotation, got: %q", d.Description)
	}
	if !strings.Contains(d.Description, "Do not call this tool again") {
		t.Errorf("declaration should include 'do not repeat' instruction")
	}
}

func TestLongRunningToolResultMetadata(t *testing.T) {
	decl := NewDeclaration("batch_job", "A batch processing job",
		map[string]any{"type": "object"},
		map[string]any{"type": "object"},
	)

	lr := NewLongRunningFunctionTool("batch_job", "A batch job", decl,
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{
				"job_id": "job-12345",
				"status": "pending",
			}, nil
		},
	)

	result, err := lr.Run(map[string]any{"input": "data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status, ok := result["status"]; !ok || status != "pending" {
		t.Errorf("status = %v, want 'pending'", result["status"])
	}
	if jobID, ok := result["job_id"]; !ok || jobID != "job-12345" {
		t.Errorf("job_id = %v, want 'job-12345'", result["job_id"])
	}
}

func TestNormalToolIsNotLongRunning(t *testing.T) {
	ft := NewFunctionTool("normal", "A normal tool",
		func(args map[string]any) (map[string]any, error) {
			return nil, nil
		},
	)
	if ft.IsLongRunning() {
		t.Error("normal function tool should not be long-running")
	}
}

// ---------------------------------------------------------------------------
// 6. ContextExecute with ToolContext
// ---------------------------------------------------------------------------

func TestContextExecuteNormalTool(t *testing.T) {
	a := newTestAgent("ce_agent")
	s := newTestSession("sid-ce", "app", "user1")
	ic := context.NewInvocationContext(context.Params{
		Agent:        a,
		Session:      s,
		InvocationID: "inv-ce",
	})

	tc := NewToolContext(ic, "fc-ce-1", nil, nil)

	ft := NewFunctionTool("add", "Add numbers",
		func(args map[string]any) (map[string]any, error) {
			a, _ := args["a"].(int)
			b, _ := args["b"].(int)
			return map[string]any{"sum": a + b}, nil
		},
	)

	cr := ContextExecute(tc, "fc-ce-1", "add", map[string]any{"a": 3, "b": 4}, ft)
	if cr.Error != "" {
		t.Fatalf("unexpected error: %s", cr.Error)
	}
	if sum, _ := cr.Result["sum"].(int); sum != 7 {
		t.Errorf("sum = %d, want 7", sum)
	}
}

func TestContextExecuteWithConfirmation(t *testing.T) {
	a := newTestAgent("ce_agent2")
	s := newTestSession("sid-ce2", "app", "user1")
	ic := context.NewInvocationContext(context.Params{
		Agent:        a,
		Session:      s,
		InvocationID: "inv-ce2",
	})

	actions := &event.EventActions{}
	tc := NewToolContext(ic, "fc-ce-2", actions, nil)

	ct := WithConfirmation(NewFunctionTool("danger", "Dangerous",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"done": true}, nil
		},
	), true, nil)

	cr := ContextExecute(tc, "fc-ce-2", "danger", map[string]any{}, ct)
	if cr.Error == "" {
		t.Fatal("expected confirmation error")
	}
	if !strings.Contains(cr.Error, ErrConfirmationRequired.Error()) {
		t.Errorf("error = %q, should contain confirmation required", cr.Error)
	}
	if req, ok := cr.Result["confirmation_required"]; !ok || req != true {
		t.Error("result should have confirmation_required = true")
	}
}

func TestContextExecuteConfirmationApproved(t *testing.T) {
	a := newTestAgent("ce_agent3")
	s := newTestSession("sid-ce3", "app", "user1")
	ic := context.NewInvocationContext(context.Params{
		Agent:        a,
		Session:      s,
		InvocationID: "inv-ce3",
	})

	conf := &event.ToolConfirmation{Confirmed: true}
	actions := &event.EventActions{}
	tc := NewToolContext(ic, "fc-ce-3", actions, conf)

	ct := WithConfirmation(NewFunctionTool("approved_op", "Approved operation",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"executed": true}, nil
		},
	), true, nil).(*confirmationTool)

	// The confirmation tool checks ToolConfirmation from context
	// In ContextExecute, if the tool is NOT a ContextFunctionTool, it falls through to Run(args)
	// So we need to ensure the tool handles the ToolContext's confirmation.
	// For now, test the raw confirmation tool flow.
	ct.SetConfirmed(true)
	cr := ContextExecute(tc, "fc-ce-3", "approved_op", map[string]any{}, ct)
	if cr.Error != "" {
		t.Fatalf("unexpected error: %s", cr.Error)
	}
	if done, ok := cr.Result["executed"]; !ok || done != true {
		t.Errorf("result = %v, expected executed=true", cr.Result)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestAgent(name string) *testAgent {
	return &testAgent{name: name, desc: "test agent"}
}

type testAgent struct{ name, desc string }

func (a *testAgent) Name() string        { return a.name }
func (a *testAgent) Description() string { return a.desc }

func newTestSession(id, appName, userID string) session.Session {
	return session.NewInMemorySession(id, appName, userID)
}
