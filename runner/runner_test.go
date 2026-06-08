package runner

import (
	stdctx "context"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/flow"
	"github.com/likun666661/rive-adk-go/llmagent"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/tool"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestRunner(t *testing.T, name string, agentResponses ...*model.LLMResponse) *Runner {
	t.Helper()
	fm := model.NewFakeModel("fake", agentResponses...)
	f := &flow.Flow{Model: fm}
	ag, err := llmagent.New(name, "test agent", f)
	if err != nil {
		t.Fatal(err)
	}
	var a agent.Agent = ag
	ea, ok := a.(ExecutableAgent)
	if !ok {
		t.Fatal("agent does not implement ExecutableAgent")
	}
	r, err := New(Config{
		AppName:        "testapp",
		Agent:          ea,
		SessionService: NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func newTestRunnerWithTools(t *testing.T, name string, tools map[string]tool.FunctionTool, responses ...*model.LLMResponse) *Runner {
	t.Helper()
	fm := model.NewFakeModel("fake", responses...)
	f := &flow.Flow{
		Model: fm,
		Tools: tools,
	}
	ag, err := llmagent.New(name, "test agent", f)
	if err != nil {
		t.Fatal(err)
	}
	var a agent.Agent = ag
	ea, ok := a.(ExecutableAgent)
	if !ok {
		t.Fatal("agent does not implement ExecutableAgent")
	}
	r, err := New(Config{
		AppName:        "testapp",
		Agent:          ea,
		SessionService: NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// ---------------------------------------------------------------------------
// Test 1: runner validation
// ---------------------------------------------------------------------------

func TestRunnerValidation(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Error("expected error for empty config")
	}

	r := newTestRunner(t, "test", model.TextResponse("hi"))
	if r.appName != "testapp" {
		t.Errorf("appName = %q, want 'testapp'", r.appName)
	}
}

// ---------------------------------------------------------------------------
// Test 2: simple text-only run (no tools)
// ---------------------------------------------------------------------------

func TestRunnerSimpleTextRun(t *testing.T) {
	r := newTestRunner(t, "echo_bot", model.TextResponse("Hello, how can I help?"))

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-1", "Hi!")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Agent should produce 1 model event.
	if len(events) != 1 {
		t.Fatalf("expected 1 agent event, got %d", len(events))
	}
	if events[0].Role != event.RoleModel {
		t.Errorf("event role = %q, want 'model'", events[0].Role)
	}
	if events[0].Content.Parts[0].Text != "Hello, how can I help?" {
		t.Errorf("text = %q", events[0].Content.Parts[0].Text)
	}

	// Session should have 2 events: user message + model response.
	if sess.EventCount() != 2 {
		t.Fatalf("expected 2 session events, got %d", sess.EventCount())
	}
	allEvs := sess.Events()
	if allEvs[0].Role != event.RoleUser {
		t.Errorf("session event 0 role = %q, want 'user'", allEvs[0].Role)
	}
	if allEvs[1].Role != event.RoleModel {
		t.Errorf("session event 1 role = %q, want 'model'", allEvs[1].Role)
	}
}

// ---------------------------------------------------------------------------
// Test 3: tool call + final response
// ---------------------------------------------------------------------------

func TestRunnerToolCallAndFinalResponse(t *testing.T) {
	weatherTool := tool.NewFunctionTool("get_weather", "Get weather",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"temp": 22, "condition": "sunny"}, nil
		},
	)

	r := newTestRunnerWithTools(t, "weather_bot",
		map[string]tool.FunctionTool{"get_weather": weatherTool},
		model.FunctionCallResponse("Let me check.",
			event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
		),
		model.TextResponse("Tokyo is 22°C and sunny."),
	)

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-2", "Weather in Tokyo?")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Events: model(fc) → tool(result) → model(final) = 3
	if len(events) != 3 {
		t.Fatalf("expected 3 agent events, got %d", len(events))
	}

	// Verify roles in order.
	wantRoles := []event.Role{event.RoleModel, event.RoleTool, event.RoleModel}
	for i, want := range wantRoles {
		if events[i].Role != want {
			t.Errorf("event[%d] role = %q, want %q", i, events[i].Role, want)
		}
	}

	// Tool event should have function response.
	evTool := events[1]
	if evTool.Content == nil || len(evTool.Content.Parts) == 0 {
		t.Fatal("tool event should have content")
	}
	fr := evTool.Content.Parts[0].FunctionResponse
	if fr == nil || fr.Name != "get_weather" {
		t.Errorf("function response name = %v", fr)
	}

	// Session should have 4 events: user + 3 agent events.
	if sess.EventCount() != 4 {
		t.Fatalf("expected 4 session events, got %d", sess.EventCount())
	}

	// Verify session event roles.
	sessionEvents := sess.Events()
	wantSessionRoles := []event.Role{event.RoleUser, event.RoleModel, event.RoleTool, event.RoleModel}
	for i, want := range wantSessionRoles {
		if sessionEvents[i].Role != want {
			t.Errorf("session event[%d] role = %q, want %q", i, sessionEvents[i].Role, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 4: session is auto-created when not found
// ---------------------------------------------------------------------------

func TestRunnerAutoCreateSession(t *testing.T) {
	r := newTestRunner(t, "echo", model.TextResponse("OK"))

	sess, _, err := r.Run(stdctx.Background(), "user-2", "nonexistent", "Hello")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if sess == nil {
		t.Fatal("expected non-nil session after auto-create")
	}
	if sess.UserID() != "user-2" {
		t.Errorf("UserID = %q, want 'user-2'", sess.UserID())
	}
	if sess.EventCount() < 1 {
		t.Error("session should have at least the user message")
	}
}

// ---------------------------------------------------------------------------
// Test 5: existing session is reused
// ---------------------------------------------------------------------------

func TestRunnerSessionReuse(t *testing.T) {
	r := newTestRunner(t, "echo",
		model.TextResponse("First response."),
		model.TextResponse("Second response."),
	)

	// First run creates the session.
	sess1, _, err := r.Run(stdctx.Background(), "user-3", "my-sess", "First msg")
	if err != nil {
		t.Fatalf("Run 1 error: %v", err)
	}

	// Second run should reuse the same session.
	sess2, _, err := r.Run(stdctx.Background(), "user-3", "my-sess", "Second msg")
	if err != nil {
		t.Fatalf("Run 2 error: %v", err)
	}

	if sess1.ID() != sess2.ID() {
		t.Errorf("session IDs differ: %q vs %q", sess1.ID(), sess2.ID())
	}

	// Session should accumulate events across runs.
	// Run 1: user1 + model1 = 2 events.
	// Run 2: user2 + model2 = 2 events → total 4.
	if sess2.EventCount() != 4 {
		t.Fatalf("expected 4 session events after 2 runs, got %d", sess2.EventCount())
	}

	seenIDs := make(map[string]bool)
	for _, ev := range sess2.Events() {
		if seenIDs[ev.ID] {
			t.Fatalf("duplicate persisted event ID %q across session reuse", ev.ID)
		}
		seenIDs[ev.ID] = true
	}
}

// ---------------------------------------------------------------------------
// Test 6: partial events are NOT persisted
// ---------------------------------------------------------------------------

func TestRunnerPartialEventsNotPersisted(t *testing.T) {
	// A partial event should be yielded but not persisted.
	// We test this by having the agent directly produce a partial event
	// followed by a final event, and verifying only the final is persisted.
	ag, err := agent.New(agent.Config{
		Name: "stream_bot",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return []*event.Event{
				{ID: "p1", Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "streaming..."}}}, Partial: true},
				{ID: "f1", Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "final"}}}},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var a agent.Agent = ag
	ea, ok := a.(ExecutableAgent)
	if !ok {
		t.Fatal("agent does not implement ExecutableAgent")
	}

	r, err := New(Config{
		AppName:        "stream_app",
		Agent:          ea,
		SessionService: NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, events, err := r.Run(stdctx.Background(), "user-5", "sess-stream", "Hello")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Both events should be returned to caller.
	if len(events) != 2 {
		t.Fatalf("expected 2 events yielded, got %d", len(events))
	}

	// But only non-partial events should be persisted.
	if sess.EventCount() != 2 { // user message + final event
		t.Fatalf("expected 2 persisted events (user + final), got %d", sess.EventCount())
	}

	persisted := sess.Events()
	if persisted[1].ID != "f1" {
		t.Errorf("persisted event 1 ID = %q, want 'f1'", persisted[1].ID)
	}
}

// ---------------------------------------------------------------------------
// Test 7: session creation respects unique user/name pairs
// ---------------------------------------------------------------------------

func TestRunnerSessionIsolation(t *testing.T) {
	r := newTestRunner(t, "echo", model.TextResponse("OK"), model.TextResponse("OK"))

	// Run for user-A.
	sessA, _, err := r.Run(stdctx.Background(), "user-A", "sess-a", "Hi")
	if err != nil {
		t.Fatal(err)
	}

	// Run for user-B.
	sessB, _, err := r.Run(stdctx.Background(), "user-B", "sess-b", "Hi")
	if err != nil {
		t.Fatal(err)
	}

	if sessA.ID() == sessB.ID() {
		t.Error("different users should have different sessions")
	}
}

// ---------------------------------------------------------------------------
// Test 8: state delta from tool persists in session
// ---------------------------------------------------------------------------

func TestRunnerStateDeltaPersistence(t *testing.T) {
	stateTool := tool.NewFunctionTool("set_state", "Set state",
		func(args map[string]any) (map[string]any, error) {
			key, _ := args["key"].(string)
			val, _ := args["value"].(string)
			return map[string]any{
				"status":      "ok",
				"state_delta": map[string]any{key: val},
			}, nil
		},
	)

	r := newTestRunnerWithTools(t, "state_bot",
		map[string]tool.FunctionTool{"set_state": stateTool},
		model.FunctionCallResponse("Setting state.",
			event.FunctionCall{ID: "fc1", Name: "set_state", Args: map[string]any{"key": "weather.last", "value": "sunny"}},
		),
		model.TextResponse("State set."),
	)

	sess, _, err := r.Run(stdctx.Background(), "user-6", "sess-state", "Set weather to sunny")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	v, ok := sess.State().Get("weather.last")
	if !ok {
		t.Error("expected 'weather.last' in session state")
	}
	if v != "sunny" {
		t.Errorf("weather.last = %v, want 'sunny'", v)
	}
}

// ---------------------------------------------------------------------------
// Test 9: after-agent callback with EndInvocation
// ---------------------------------------------------------------------------

func TestRunnerAfterAgentEndInvocation(t *testing.T) {
	ag, err := agent.New(agent.Config{
		Name: "end_bot",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return []*event.Event{
				{ID: "e1", Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "processing"}}}},
			}, nil
		},
		AfterAgentCallbacks: []agent.AfterAgentCallback{
			func(ctx agent.InvocationContext, events []*event.Event) (*event.Event, error) {
				return &event.Event{
					ID:      "end_ev",
					Actions: event.EventActions{EndInvocation: true},
					Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "bye"}}},
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var a agent.Agent = ag
	ea, ok := a.(ExecutableAgent)
	if !ok {
		t.Fatal("agent does not implement ExecutableAgent")
	}

	r, err := New(Config{
		AppName:        "end_app",
		Agent:          ea,
		SessionService: NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, events, err := r.Run(stdctx.Background(), "user-7", "sess-end", "Trigger end")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events (run + after), got %d", len(events))
	}

	lastEv := events[1]
	if lastEv.ID != "end_ev" {
		t.Errorf("last event ID = %q, want 'end_ev'", lastEv.ID)
	}

	// Session should have user + e1 + end_ev = 3.
	if sess.EventCount() != 3 {
		t.Fatalf("expected 3 session events, got %d", sess.EventCount())
	}
}
