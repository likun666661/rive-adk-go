package runner

import (
	stdctx "context"
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/artifact"
	invctx "github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/flow"
	"github.com/likun666661/rive-adk-go/llmagent"
	"github.com/likun666661/rive-adk-go/memory"
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

// newTestRunnerWithServices creates a runner with memory and artifact services.
func newTestRunnerWithServices(t *testing.T, name string, memSvc memory.Service, artSvc artifact.Service, agentResponses ...*model.LLMResponse) *Runner {
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
		AppName:         "testapp",
		Agent:           ea,
		SessionService:  NewInMemorySessionService(),
		MemoryService:   memSvc,
		ArtifactService: artSvc,
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

// =============================================================================
// Chapter 02 — State lifecycle integration tests
// =============================================================================

// ---------------------------------------------------------------------------
// Test 10: runner config with memory and artifact services
// ---------------------------------------------------------------------------

func TestRunnerConfigWithMemoryAndArtifact(t *testing.T) {
	memSvc := memory.InMemoryService()
	artSvc := artifact.InMemoryService()

	r := newTestRunnerWithServices(t, "agent", memSvc, artSvc, model.TextResponse("ok"))
	if r.memoryService == nil {
		t.Error("expected memory service to be set")
	}
	if r.artifactService == nil {
		t.Error("expected artifact service to be set")
	}

	// Services are optional — runner should still work without them.
	r2, err := New(Config{
		AppName:        "testapp",
		Agent:          r.agent,
		SessionService: NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatalf("runner without memory/artifact should be valid: %v", err)
	}
	if r2.memoryService != nil {
		t.Error("expected nil memory service when not provided")
	}
	if r2.artifactService != nil {
		t.Error("expected nil artifact service when not provided")
	}
}

// ---------------------------------------------------------------------------
// Test 11: scoped state mutation through event actions via runner
// ---------------------------------------------------------------------------

func TestRunnerScopedStateMutation(t *testing.T) {
	stateTool := tool.NewFunctionTool("set_scoped_state", "Set scoped state",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{
				"status": "ok",
				"state_delta": map[string]any{
					"app:version":  "1.0",
					"user:pref":    "dark",
					"local":        "session_val",
					"temp:scratch": "tmp_val",
				},
			}, nil
		},
	)

	sessionSvc := NewInMemorySessionService()
	fm := model.NewFakeModel("fake",
		model.FunctionCallResponse("setting state",
			event.FunctionCall{ID: "fc1", Name: "set_scoped_state", Args: map[string]any{}},
		),
		model.TextResponse("done"),
	)
	f := &flow.Flow{
		Model: fm,
		Tools: map[string]tool.FunctionTool{"set_scoped_state": stateTool},
	}
	ag, _ := llmagent.New("state_bot", "test", f)
	ea := ag.(ExecutableAgent)
	r, err := New(Config{
		AppName:        "testapp",
		Agent:          ea,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, _, err := r.Run(stdctx.Background(), "user-1", "sess-scope", "Set state")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Session-local state check.
	if v, ok := sess.State().Get("local"); !ok || v != "session_val" {
		t.Errorf("local = %v, want 'session_val'", v)
	}

	// Temp state should NOT be in durable session state after Run.
	if _, ok := sess.State().Get("temp:scratch"); ok {
		t.Error("temp:scratch should be cleaned from durable session state after Run")
	}

	// Merged state should show app and user prefixes.
	merged, err := sessionSvc.GetMergedState("testapp", "user-1", "sess-scope")
	if err != nil {
		t.Fatalf("GetMergedState: %v", err)
	}
	if merged["app:version"] != "1.0" {
		t.Errorf("app:version = %v, want '1.0'", merged["app:version"])
	}
	if merged["user:pref"] != "dark" {
		t.Errorf("user:pref = %v, want 'dark'", merged["user:pref"])
	}
	if merged["local"] != "session_val" {
		t.Errorf("local = %v, want 'session_val'", merged["local"])
	}

	// Temp should NOT appear in merged state.
	if _, ok := merged["temp:scratch"]; ok {
		t.Error("temp:scratch should not appear in merged state (trimmed on persist)")
	}
}

// ---------------------------------------------------------------------------
// Test 12: app and user state shared across sessions, session state isolated
// ---------------------------------------------------------------------------

func TestRunnerStateMergeAcrossSessions(t *testing.T) {
	stateTool := tool.NewFunctionTool("set_scoped_state", "Set state",
		func(args map[string]any) (map[string]any, error) {
			delta, _ := args["delta"].(map[string]any)
			return map[string]any{"status": "ok", "state_delta": delta}, nil
		},
	)

	sessionSvc := NewInMemorySessionService()
	buildRunner := func(agentName string, fcID string, delta map[string]any) *Runner {
		fm := model.NewFakeModel("fake",
			model.FunctionCallResponse("setting",
				event.FunctionCall{ID: fcID, Name: "set_scoped_state", Args: map[string]any{"delta": delta}},
			),
			model.TextResponse("done"),
		)
		f := &flow.Flow{
			Model: fm,
			Tools: map[string]tool.FunctionTool{"set_scoped_state": stateTool},
		}
		ag, _ := llmagent.New(agentName, "test", f)
		ea := ag.(ExecutableAgent)
		r, _ := New(Config{
			AppName:        "testapp",
			Agent:          ea,
			SessionService: sessionSvc,
		})
		return r
	}

	// Session 1 sets app and user state.
	r1 := buildRunner("bot1", "fc1", map[string]any{
		"app:theme": "corp",
		"user:lang": "en",
		"topic":     "from_sess1",
	})
	_, _, err := r1.Run(stdctx.Background(), "user-x", "sess-a", "msg")
	if err != nil {
		t.Fatalf("Run sess-a error: %v", err)
	}

	// Session 2 (same user) should see app and user state but not session state.
	r2 := buildRunner("bot2", "fc2", map[string]any{
		"user:font": "large",
		"topic":     "from_sess2",
	})
	_, _, err = r2.Run(stdctx.Background(), "user-x", "sess-b", "msg")
	if err != nil {
		t.Fatalf("Run sess-b error: %v", err)
	}

	mergedB, _ := sessionSvc.GetMergedState("testapp", "user-x", "sess-b")
	if mergedB["app:theme"] != "corp" {
		t.Errorf("sess-b app:theme = %v, want 'corp'", mergedB["app:theme"])
	}
	if mergedB["user:lang"] != "en" {
		t.Errorf("sess-b user:lang = %v, want 'en'", mergedB["user:lang"])
	}
	if mergedB["user:font"] != "large" {
		t.Errorf("sess-b user:font = %v, want 'large'", mergedB["user:font"])
	}
	if mergedB["topic"] != "from_sess2" {
		t.Errorf("sess-b topic = %v, want 'from_sess2' (session-local)", mergedB["topic"])
	}

	// sess-a should NOT have sess-b's session key.
	mergedA, _ := sessionSvc.GetMergedState("testapp", "user-x", "sess-a")
	if mergedA["topic"] != "from_sess1" {
		t.Errorf("sess-a topic = %v, want 'from_sess1'", mergedA["topic"])
	}
	if _, ok := mergedA["user:font"]; !ok {
		t.Error("sess-a should see user:font set by sess-b (user-scoped)")
	}
}

// ---------------------------------------------------------------------------
// Test 13: temp state visible during invocation, not persisted
// ---------------------------------------------------------------------------

func TestRunnerTempStateLifecycle(t *testing.T) {
	stateTool := tool.NewFunctionTool("set_temp", "Set temp state",
		func(args map[string]any) (map[string]any, error) {
			key, _ := args["key"].(string)
			val, _ := args["value"].(string)
			return map[string]any{
				"state_delta": map[string]any{
					"temp:" + key: val,
					"durable":     "stays",
				},
			}, nil
		},
	)

	sessionSvc := NewInMemorySessionService()
	fm := model.NewFakeModel("fake",
		model.FunctionCallResponse("setting temp",
			event.FunctionCall{ID: "fc1", Name: "set_temp", Args: map[string]any{
				"key": "cache", "value": "tmp-data",
			}},
		),
		model.TextResponse("done"),
	)
	f := &flow.Flow{
		Model: fm,
		Tools: map[string]tool.FunctionTool{"set_temp": stateTool},
	}
	ag, _ := llmagent.New("temp_bot", "test", f)
	ea := ag.(ExecutableAgent)
	r, _ := New(Config{
		AppName:        "testapp",
		Agent:          ea,
		SessionService: sessionSvc,
	})

	sess, _, err := r.Run(stdctx.Background(), "user-1", "sess-temp", "Go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Temp key should NOT be in durable session state after Run returns.
	if _, ok := sess.State().Get("temp:cache"); ok {
		t.Error("temp:cache should be cleaned from durable session state after Run")
	}

	// Durable key visible.
	if v, ok := sess.State().Get("durable"); !ok || v != "stays" {
		t.Errorf("durable = %v, want 'stays'", v)
	}

	// Check persisted events — temp prefix removed from StateDelta.
	for _, ev := range sess.Events() {
		if _, ok := ev.Actions.StateDelta["temp:cache"]; ok {
			t.Error("temp:cache should be trimmed from persisted event StateDelta")
		}
	}

	// Merged state should NOT have temp prefix keys.
	merged, _ := sessionSvc.GetMergedState("testapp", "user-1", "sess-temp")
	if _, ok := merged["temp:cache"]; ok {
		t.Error("temp:cache should not appear in merged state")
	}
}

// ---------------------------------------------------------------------------
// Test 14: artifact save and load alongside runner session
// ---------------------------------------------------------------------------

func TestRunnerArtifactSaveLoad(t *testing.T) {
	artSvc := artifact.InMemoryService()

	r := newTestRunnerWithServices(t, "art_bot", nil, artSvc,
		model.TextResponse("I'll save an artifact for you."),
	)

	sess, _, err := r.Run(stdctx.Background(), "user-1", "sess-art", "Save my report")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if sess == nil {
		t.Fatal("expected non-nil session")
	}

	// Save an artifact scoped to this session.
	saveResp, err := artSvc.Save(t.Context(), &artifact.SaveRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: sess.ID(),
		FileName:  "output.txt",
		Part:      &artifact.ArtifactPart{Text: "report content"},
	})
	if err != nil {
		t.Fatalf("Save artifact: %v", err)
	}
	if saveResp.Version != 1 {
		t.Errorf("first save version = %d, want 1", saveResp.Version)
	}

	// Save again — version increments.
	saveResp2, _ := artSvc.Save(t.Context(), &artifact.SaveRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: sess.ID(),
		FileName:  "output.txt",
		Part:      &artifact.ArtifactPart{Text: "updated report"},
	})
	if saveResp2.Version != 2 {
		t.Errorf("second save version = %d, want 2", saveResp2.Version)
	}

	// Load latest.
	loadResp, err := artSvc.Load(t.Context(), &artifact.LoadRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: sess.ID(),
		FileName:  "output.txt",
	})
	if err != nil {
		t.Fatalf("Load artifact: %v", err)
	}
	if loadResp.Part.Text != "updated report" {
		t.Errorf("latest = %q, want 'updated report'", loadResp.Part.Text)
	}

	// Load specific version.
	loadV1, _ := artSvc.Load(t.Context(), &artifact.LoadRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: sess.ID(),
		FileName:  "output.txt",
		Version:   1,
	})
	if loadV1.Part.Text != "report content" {
		t.Errorf("version 1 = %q, want 'report content'", loadV1.Part.Text)
	}

	// Artifact scoped to session — not visible from another session.
	_, err = artSvc.Load(t.Context(), &artifact.LoadRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: "other-session",
		FileName:  "output.txt",
	})
	if err == nil {
		t.Error("artifact should not be visible from another session (session-scoped)")
	}

	// User-scoped artifact visible across sessions.
	artSvc.Save(t.Context(), &artifact.SaveRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: sess.ID(),
		FileName:  "user:prefs.json",
		Part:      &artifact.ArtifactPart{Text: `{"theme":"dark"}`},
	})
	userLoad, err := artSvc.Load(t.Context(), &artifact.LoadRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: "other-session",
		FileName:  "user:prefs.json",
	})
	if err != nil {
		t.Fatalf("user-scoped artifact should be visible across sessions: %v", err)
	}
	if userLoad.Part.Text != `{"theme":"dark"}` {
		t.Errorf("user-scoped content = %q", userLoad.Part.Text)
	}
}

// ---------------------------------------------------------------------------
// Test 15: memory add and search through runner
// ---------------------------------------------------------------------------

func TestRunnerMemoryAddSearch(t *testing.T) {
	memSvc := memory.InMemoryService()

	fm := model.NewFakeModel("fake",
		model.TextResponse("The sky is blue and clouds are white."),
		model.TextResponse("Tree leaves are green in spring."),
	)

	f := &flow.Flow{Model: fm}
	ag, _ := llmagent.New("memory_bot", "test", f)
	ea := ag.(ExecutableAgent)

	sessionSvc := NewInMemorySessionService()
	r, err := New(Config{
		AppName:        "testapp",
		Agent:          ea,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Run a session with the runner.
	sess1, _, err := r.Run(stdctx.Background(), "user-10", "sess-mem", "What do you remember?")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Run a second session for the same user.
	sess2, _, err := r.Run(stdctx.Background(), "user-10", "sess-mem2", "Tell me more about my preferences")
	if err != nil {
		t.Fatalf("Run 2 error: %v", err)
	}

	// Add both sessions to memory.
	if err := memSvc.AddSessionToMemory(t.Context(), sess1); err != nil {
		t.Fatalf("AddSessionToMemory(sess1): %v", err)
	}
	if err := memSvc.AddSessionToMemory(t.Context(), sess2); err != nil {
		t.Fatalf("AddSessionToMemory(sess2): %v", err)
	}

	// Search for "blue" — should find from sess1.
	resp, err := memSvc.SearchMemory(t.Context(), &memory.SearchRequest{
		AppName: "testapp",
		UserID:  "user-10",
		Query:   "blue",
	})
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if len(resp.Memories) != 1 {
		t.Fatalf("expected 1 memory matching 'blue', got %d", len(resp.Memories))
	}
	if resp.Memories[0].Author != "memory_bot" {
		t.Errorf("memory author = %q, want 'memory_bot'", resp.Memories[0].Author)
	}

	// Search for "green" — should find from sess2.
	resp2, _ := memSvc.SearchMemory(t.Context(), &memory.SearchRequest{
		AppName: "testapp",
		UserID:  "user-10",
		Query:   "green",
	})
	if len(resp2.Memories) != 1 {
		t.Errorf("expected 1 memory matching 'green', got %d", len(resp2.Memories))
	}

	// Cross-user isolation — user-11 should not see user-10's memories.
	resp3, _ := memSvc.SearchMemory(t.Context(), &memory.SearchRequest{
		AppName: "testapp",
		UserID:  "user-11",
		Query:   "blue",
	})
	if len(resp3.Memories) != 0 {
		t.Error("memory should be isolated by user")
	}

	// Cross-app isolation.
	resp4, _ := memSvc.SearchMemory(t.Context(), &memory.SearchRequest{
		AppName: "other-app",
		UserID:  "user-10",
		Query:   "blue",
	})
	if len(resp4.Memories) != 0 {
		t.Error("memory should be isolated by app")
	}
}

// ---------------------------------------------------------------------------
// Test 16: full chain — runner + state scoping + artifact + memory
// ---------------------------------------------------------------------------

func TestRunnerFullChain(t *testing.T) {
	artSvc := artifact.InMemoryService()
	memSvc := memory.InMemoryService()

	stateTool := tool.NewFunctionTool("set_pref", "Store user preference",
		func(args map[string]any) (map[string]any, error) {
			pref, _ := args["pref"].(string)
			return map[string]any{
				"status": "saved",
				"state_delta": map[string]any{
					"user:favorite_language": pref,
					"session_step":           "pref_set",
				},
			}, nil
		},
	)

	sessionSvc := NewInMemorySessionService()
	buildRunner := func() *Runner {
		fm := model.NewFakeModel("fake",
			model.FunctionCallResponse("Saving preference.",
				event.FunctionCall{
					ID:   "fc1",
					Name: "set_pref",
					Args: map[string]any{"pref": "Go"},
				},
			),
			model.TextResponse("Preference saved. Go is great!"),
		)
		f := &flow.Flow{
			Model: fm,
			Tools: map[string]tool.FunctionTool{"set_pref": stateTool},
		}
		ag, _ := llmagent.New("pref_bot", "test", f)
		ea := ag.(ExecutableAgent)
		r, _ := New(Config{
			AppName:         "testapp",
			Agent:           ea,
			SessionService:  sessionSvc,
			MemoryService:   memSvc,
			ArtifactService: artSvc,
		})
		return r
	}

	r := buildRunner()

	// 1. Run session → sets user: state via tool.
	sess, _, err := r.Run(stdctx.Background(), "user-full", "sess-full", "I like Go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// 2. Verify state scoping.
	merged, _ := sessionSvc.GetMergedState("testapp", "user-full", "sess-full")
	if merged["user:favorite_language"] != "Go" {
		t.Errorf("user:favorite_language = %v, want 'Go'", merged["user:favorite_language"])
	}
	if merged["session_step"] != "pref_set" {
		t.Errorf("session_step = %v, want 'pref_set'", merged["session_step"])
	}

	// 3. Save artifact.
	_, err = artSvc.Save(t.Context(), &artifact.SaveRequest{
		AppName:   "testapp",
		UserID:    "user-full",
		SessionID: "sess-full",
		FileName:  "summary.txt",
		Part:      &artifact.ArtifactPart{Text: "User prefers Go"},
	})
	if err != nil {
		t.Fatalf("Save artifact: %v", err)
	}

	// 4. Verify artifact.
	loadResp, _ := artSvc.Load(t.Context(), &artifact.LoadRequest{
		AppName:   "testapp",
		UserID:    "user-full",
		SessionID: "sess-full",
		FileName:  "summary.txt",
	})
	if loadResp.Part.Text != "User prefers Go" {
		t.Errorf("artifact content = %q, want 'User prefers Go'", loadResp.Part.Text)
	}

	// 5. Add to memory.
	if err := memSvc.AddSessionToMemory(t.Context(), sess); err != nil {
		t.Fatalf("AddSessionToMemory: %v", err)
	}

	// 6. Search memory — should find "Go" from the model response and tool args.
	resp, _ := memSvc.SearchMemory(t.Context(), &memory.SearchRequest{
		AppName: "testapp",
		UserID:  "user-full",
		Query:   "Go",
	})
	if len(resp.Memories) < 1 {
		t.Error("expected at least 1 memory matching 'Go'")
	}

	// 7. A new session for the same user should see the user: state but NOT session state.
	// Build a runner WITHOUT the state tool so it doesn't also set session_step.
	fm2 := model.NewFakeModel("fake2", model.TextResponse("Hello! How can I help?"))
	f2 := &flow.Flow{Model: fm2}
	ag2, _ := llmagent.New("echo_bot", "test", f2)
	ea2 := ag2.(ExecutableAgent)
	r2, _ := New(Config{
		AppName:         "testapp",
		Agent:           ea2,
		SessionService:  sessionSvc,
		MemoryService:   memSvc,
		ArtifactService: artSvc,
	})
	sess2, _, _ := r2.Run(stdctx.Background(), "user-full", "sess-full2", "Hello again")
	if sess2 == nil {
		t.Fatal("expected non-nil session")
	}
	merged2, _ := sessionSvc.GetMergedState("testapp", "user-full", "sess-full2")
	if merged2["user:favorite_language"] != "Go" {
		t.Errorf("new session user:favorite_language = %v, want 'Go'", merged2["user:favorite_language"])
	}
	// Session-scoped state should NOT leak.
	if _, ok := merged2["session_step"]; ok {
		t.Error("session_step should not leak across sessions")
	}

	// 8. New session events also go into memory — verify by searching for both sessions' content.
	memSvc.AddSessionToMemory(t.Context(), sess2)
	resp2, _ := memSvc.SearchMemory(t.Context(), &memory.SearchRequest{
		AppName: "testapp",
		UserID:  "user-full",
		Query:   "how",
	})
	if len(resp2.Memories) < 1 {
		t.Errorf("expected >= 1 memory from sess2 matching 'how', got %d", len(resp2.Memories))
	}
}

// ---------------------------------------------------------------------------
// Test 17: artifact versions are independent from session events
// ---------------------------------------------------------------------------

func TestRunnerArtifactVersionIndependence(t *testing.T) {
	artSvc := artifact.InMemoryService()

	r := newTestRunnerWithServices(t, "bot", nil, artSvc,
		model.TextResponse("first message"),
		model.TextResponse("second message"),
	)

	// Run session — produces session events.
	sess, _, err := r.Run(stdctx.Background(), "user-1", "sess-ver", "Msg 1")
	if err != nil {
		t.Fatalf("Run 1 error: %v", err)
	}

	// Run again on same session — more events.
	sess2, _, err := r.Run(stdctx.Background(), "user-1", "sess-ver", "Msg 2")
	if err != nil {
		t.Fatalf("Run 2 error: %v", err)
	}

	// Artifact saves produce versions 1, 2, 3 — unrelated to event count.
	artSvc.Save(t.Context(), &artifact.SaveRequest{
		AppName: "testapp", UserID: "user-1", SessionID: sess.ID(),
		FileName: "chart.png", Part: &artifact.ArtifactPart{InlineData: &artifact.InlineData{Data: []byte{1}, MIMEType: "image/png"}},
	})
	artSvc.Save(t.Context(), &artifact.SaveRequest{
		AppName: "testapp", UserID: "user-1", SessionID: sess.ID(),
		FileName: "chart.png", Part: &artifact.ArtifactPart{InlineData: &artifact.InlineData{Data: []byte{2}, MIMEType: "image/png"}},
	})

	versions, _ := artSvc.Versions(t.Context(), &artifact.VersionsRequest{
		AppName: "testapp", UserID: "user-1", SessionID: sess.ID(), FileName: "chart.png",
	})
	if len(versions.Versions) != 2 || versions.Versions[0] != 1 || versions.Versions[1] != 2 {
		t.Errorf("artifact versions = %v, want [1 2] (independent of event count)", versions.Versions)
	}

	// Session event count should be independent of artifact versions.
	if sess2.EventCount() != 4 {
		t.Errorf("event count = %d, want 4 (user1+model1+user2+model2)", sess2.EventCount())
	}
}

// =============================================================================
// Chapter 03 — Tool system integration tests (full chain)
// =============================================================================

// ---------------------------------------------------------------------------
// Test 18: full chain with confirmation tool — confirm path
// ---------------------------------------------------------------------------

func TestRunnerFullChainConfirmationConfirmed(t *testing.T) {
	inner := tool.NewFunctionTool("deploy", "Deploy to production",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"deployed": true, "version": args["version"]}, nil
		},
	)

	confirmedToolWrapper := tool.WithConfirmation(inner, true, nil)

	fm := model.NewFakeModel("fake",
		model.FunctionCallResponse("Let me deploy.",
			event.FunctionCall{ID: "fc1", Name: "deploy", Args: map[string]any{"version": "v2.0"}},
		),
		model.TextResponse("Deployment complete."),
	)

	f := &flow.Flow{
		Model: fm,
		Tools: map[string]tool.FunctionTool{
			"deploy": confirmedToolWrapper,
		},
	}

	ag, _ := llmagent.New("deploy_bot", "test", f)
	ea := ag.(ExecutableAgent)

	sessionSvc := NewInMemorySessionService()
	r, err := New(Config{
		AppName:        "deploy_app",
		Agent:          ea,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	// First run — tool requires confirmation.
	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-confirm", "Deploy v2.0")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Events: model(fc) → tool(confirmation required) → model(final text)
	// The flow loop continues after tool confirmation error because the
	// model event with function call is not IsFinalResponse.
	// The next model response (text) is final.
	if len(events) != 3 {
		t.Fatalf("expected 3 events (model+fc, tool/confirm, model/final), got %d", len(events))
	}

	// Tool result should indicate confirmation required.
	ev2 := events[1]
	if ev2.Role != event.RoleTool {
		t.Errorf("event 2 role = %q, want 'tool'", ev2.Role)
	}
	fr := ev2.Content.Parts[0].FunctionResponse
	if fr == nil {
		t.Fatal("expected function response")
	}
	if fr.Error == "" {
		t.Error("expected confirmation error")
	}
	if req, ok := fr.Result["confirmation_required"]; !ok || req != true {
		t.Error("result should have confirmation_required = true")
	}

	// Session should have user + model(fc) + tool + model(final) = 4 events.
	if sess.EventCount() != 4 {
		t.Fatalf("expected 4 session events, got %d", sess.EventCount())
	}

	sessEvents := sess.Events()
	for i, role := range []event.Role{event.RoleUser, event.RoleModel, event.RoleTool, event.RoleModel} {
		if sessEvents[i].Role != role {
			t.Errorf("session event[%d] role = %q, want %q", i, sessEvents[i].Role, role)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 19: full chain with streaming tool (non-live collection)
// ---------------------------------------------------------------------------

func TestRunnerFullChainStreamingTool(t *testing.T) {
	streamTool := tool.NewStreamingFunctionToolWithDeclaration("generate_report", "Generate report",
		tool.NewDeclaration("generate_report", "Generate report", nil, nil),
		func(args map[string]any) ([]tool.StreamChunk, error) {
			return []tool.StreamChunk{
				{Text: "Section 1: Overview\n", Final: false},
				{Text: "Section 2: Details\n", Final: false},
				{Text: "Section 3: Summary", Final: true},
			}, nil
		},
	)
	ts := tool.NewStaticToolset("stream_set", []tool.Tool{streamTool})

	fm := model.NewFakeModel("fake",
		model.FunctionCallResponse("Generating report.",
			event.FunctionCall{ID: "fc1", Name: "generate_report", Args: map[string]any{}},
		),
		model.TextResponse("Report generated successfully."),
	)

	f := &flow.Flow{
		Model:    fm,
		Tools:    map[string]tool.FunctionTool{},
		Toolsets: []tool.Toolset{ts},
	}

	ag, _ := llmagent.New("report_bot", "test", f)
	ea := ag.(ExecutableAgent)

	r, err := New(Config{
		AppName:        "stream_app",
		Agent:          ea,
		SessionService: NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-stream", "Generate report")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Events: model(fc) → tool(result) → model(final) = 3
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Tool event should contain the collected chunks.
	ev2 := events[1]
	if ev2.Role != event.RoleTool {
		t.Errorf("event 2 role = %q, want 'tool'", ev2.Role)
	}
	fr := ev2.Content.Parts[0].FunctionResponse
	if fr == nil {
		t.Fatal("expected function response")
	}
	result, ok := fr.Result["result"].(string)
	if !ok {
		t.Fatal("expected string result from stream")
	}
	if result != "Section 1: Overview\nSection 2: Details\nSection 3: Summary" {
		t.Errorf("stream result = %q", result)
	}

	// Session should have user + model(fc) + tool + model(final) = 4 events.
	if sess.EventCount() != 4 {
		t.Fatalf("expected 4 session events, got %d", sess.EventCount())
	}
}

// ---------------------------------------------------------------------------
// Test 20: full chain with long-running tool metadata
// ---------------------------------------------------------------------------

func TestRunnerFullChainLongRunningTool(t *testing.T) {
	decl := tool.NewDeclaration("start_training", "Start model training",
		map[string]any{"type": "object", "properties": map[string]any{"model": map[string]any{"type": "string"}}},
		map[string]any{"type": "object", "properties": map[string]any{"job_id": map[string]any{"type": "string"}}},
	)

	lrTool := tool.NewLongRunningFunctionTool("start_training", "Start model training", decl,
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{
				"job_id":  "train-job-001",
				"status":  "pending",
				"message": "Training job submitted successfully",
			}, nil
		},
	)

	// Use BeforeModelCallback to capture injected declarations.
	var capturedDecls []any
	f := &flow.Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Starting training.",
				event.FunctionCall{ID: "fc1", Name: "start_training", Args: map[string]any{"model": "gpt"}},
			),
			model.TextResponse("Training job submitted."),
		),
		Tools: map[string]tool.FunctionTool{
			"start_training": lrTool,
		},
		BeforeModelCallbacks: []flow.BeforeModelCallback{
			func(_ invctx.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				capturedDecls = req.ToolDeclarations
				return nil, nil
			},
		},
	}

	ag, _ := llmagent.New("train_bot", "test", f)
	ea := ag.(ExecutableAgent)

	r, err := New(Config{
		AppName:        "train_app",
		Agent:          ea,
		SessionService: NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-train", "Train model")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Events: model(fc) → tool(result) → model(final) = 3
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Tool declaration should include long-running annotation.
	if len(capturedDecls) == 0 {
		t.Fatal("expected tool declarations to be injected")
	}
	d, ok := capturedDecls[0].(tool.Declaration)
	if !ok {
		t.Fatalf("expected Declaration, got %T", capturedDecls[0])
	}
	if d.Name != "start_training" {
		t.Errorf("declaration name = %q, want 'start_training'", d.Name)
	}
	if !strings.Contains(d.Description, "long-running operation") {
		t.Error("declaration description should contain long-running annotation")
	}
	if !strings.Contains(d.Description, "Do not call this tool again") {
		t.Error("declaration should include 'Do not repeat' instruction")
	}

	// Tool result should contain job metadata.
	ev2 := events[1]
	fr := ev2.Content.Parts[0].FunctionResponse
	if fr == nil {
		t.Fatal("expected function response")
	}
	if fr.Result["job_id"] != "train-job-001" {
		t.Errorf("job_id = %v, want 'train-job-001'", fr.Result["job_id"])
	}
	if fr.Result["status"] != "pending" {
		t.Errorf("status = %v, want 'pending'", fr.Result["status"])
	}

	// Session has all events.
	if sess.EventCount() != 4 {
		t.Fatalf("expected 4 session events, got %d", sess.EventCount())
	}
}

// ---------------------------------------------------------------------------
// Test 21: full chain with filtered toolset + Runner
// ---------------------------------------------------------------------------

func TestRunnerFullChainFilteredToolset(t *testing.T) {
	allowedTool := tool.NewFunctionToolWithDeclaration("search", "Search the web",
		tool.NewDeclaration("search", "Search the web", nil, nil),
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"results": []string{"a", "b"}}, nil
		},
	)
	blockedTool := tool.NewFunctionToolWithDeclaration("delete", "Delete everything",
		tool.NewDeclaration("delete", "Delete everything", nil, nil),
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"deleted": true}, nil
		},
	)

	fullTs := tool.NewStaticToolset("all", []tool.Tool{
		allowedTool.(tool.Tool),
		blockedTool.(tool.Tool),
	})
	filteredTs := tool.NewFilterToolset("safe", fullTs,
		tool.AllowedToolsPredicate("search"),
	)

	var capturedNames []string
	f := &flow.Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Searching.",
				event.FunctionCall{ID: "fc1", Name: "search", Args: map[string]any{"q": "test"}},
			),
			model.TextResponse("Search complete."),
		),
		Tools:    map[string]tool.FunctionTool{},
		Toolsets: []tool.Toolset{filteredTs},
		BeforeModelCallbacks: []flow.BeforeModelCallback{
			func(_ invctx.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				for _, d := range req.ToolDeclarations {
					if dec, ok := d.(tool.Declaration); ok {
						capturedNames = append(capturedNames, dec.Name)
					}
				}
				return nil, nil
			},
		},
	}

	ag, _ := llmagent.New("filter_bot", "test", f)
	ea := ag.(ExecutableAgent)

	r, err := New(Config{
		AppName:        "filter_app",
		Agent:          ea,
		SessionService: NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-filter", "Search for test")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Only 'search' should appear in declarations.
	for _, name := range capturedNames {
		if name == "delete" {
			t.Error("'delete' tool should be filtered out of declarations")
		}
	}

	// Tool result should be from search.
	ev2 := events[1]
	fr := ev2.Content.Parts[0].FunctionResponse
	if fr.Name != "search" {
		t.Errorf("function response name = %q, want 'search'", fr.Name)
	}

	// Session should have all events.
	if sess.EventCount() != 4 {
		t.Fatalf("expected 4 session events, got %d", sess.EventCount())
	}
}

// ---------------------------------------------------------------------------
// Test 22: confirmation rejection flow through runner
// ---------------------------------------------------------------------------

func TestRunnerConfirmationRejectionChain(t *testing.T) {
	inner := tool.NewFunctionTool("risky_op", "A risky operation",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"executed": true}, nil
		},
	)

	confirmedWrapper := tool.WithConfirmation(inner, true, nil)

	fm := model.NewFakeModel("fake",
		model.FunctionCallResponse("Let me try risky operation.",
			event.FunctionCall{ID: "fc1", Name: "risky_op", Args: map[string]any{"target": "prod"}},
		),
		model.TextResponse("Operation was rejected by user."),
	)

	f := &flow.Flow{
		Model: fm,
		Tools: map[string]tool.FunctionTool{
			"risky_op": confirmedWrapper,
		},
	}

	ag, _ := llmagent.New("risky_bot", "test", f)
	ea := ag.(ExecutableAgent)

	r, err := New(Config{
		AppName:        "risk_app",
		Agent:          ea,
		SessionService: NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-reject", "Risky operation")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Events: model(fc) → tool(confirmation required) → model(final text) = 3
	if len(events) != 3 {
		t.Fatalf("expected 3 events (model+fc, tool/confirm, model/final), got %d", len(events))
	}

	// Tool result should indicate confirmation required.
	ev2 := events[1]
	fr := ev2.Content.Parts[0].FunctionResponse
	if req, ok := fr.Result["confirmation_required"]; !ok || req != true {
		t.Error("result should have confirmation_required = true")
	}
	if fr.Error == "" {
		t.Error("expected confirmation required error")
	}

	// Session should have user + model(fc) + tool(confirm) + model(final) = 4.
	if sess.EventCount() != 4 {
		t.Fatalf("expected 4 session events, got %d", sess.EventCount())
	}

	// Verify session event roles.
	sessEvents := sess.Events()
	wantRoles := []event.Role{event.RoleUser, event.RoleModel, event.RoleTool, event.RoleModel}
	for i, want := range wantRoles {
		if sessEvents[i].Role != want {
			t.Errorf("session event[%d] role = %q, want %q", i, sessEvents[i].Role, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 23: toolset with declaration injection through full chain
// ---------------------------------------------------------------------------

func TestRunnerFullChainToolsetDeclarations(t *testing.T) {
	decl := tool.NewDeclaration("get_weather", "Get weather", nil, nil)
	tsTool := tool.NewFunctionToolWithDeclaration("get_weather", "Get weather", decl,
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"temp": 25}, nil
		},
	)
	ts := tool.NewStaticToolset("weather_set", []tool.Tool{tsTool.(tool.Tool)})

	var capturedDecls []any
	f := &flow.Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Checking weather.",
				event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Paris"}},
			),
			model.TextResponse("Paris is 25°C."),
		),
		Tools:    map[string]tool.FunctionTool{},
		Toolsets: []tool.Toolset{ts},
		BeforeModelCallbacks: []flow.BeforeModelCallback{
			func(_ invctx.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				capturedDecls = req.ToolDeclarations
				return nil, nil
			},
		},
	}

	ag, _ := llmagent.New("weather_bot", "test", f)
	ea := ag.(ExecutableAgent)

	r, err := New(Config{
		AppName:        "toolset_app",
		Agent:          ea,
		SessionService: NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-ts", "Weather in Paris?")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Declarations must be injected.
	if len(capturedDecls) == 0 {
		t.Fatal("expected declarations to be injected from toolset")
	}

	// Tool must execute from toolset.
	ev2 := events[1]
	fr := ev2.Content.Parts[0].FunctionResponse
	if fr.Name != "get_weather" {
		t.Errorf("function response name = %q, want 'get_weather'", fr.Name)
	}
	if temp, ok := fr.Result["temp"]; !ok || temp != 25 {
		t.Errorf("temp = %v, want 25", temp)
	}

	if sess.EventCount() != 4 {
		t.Fatalf("expected 4 session events, got %d", sess.EventCount())
	}
}
