package context

import (
	stdctx "context"
	"fmt"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/artifact"
	"github.com/likun666661/rive-adk-go/callbackctx"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/memory"
	"github.com/likun666661/rive-adk-go/session"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type testAgent struct{ name, desc string }

func (a *testAgent) Name() string                        { return a.name }
func (a *testAgent) Description() string                 { return a.desc }
func (a *testAgent) SubAgents() []agent.Agent            { return nil }
func (a *testAgent) FindAgent(name string) agent.Agent {
	if a.name == name {
		return a
	}
	return nil
}
func (a *testAgent) Parent() agent.Agent               { return nil }
func (a *testAgent) DisallowTransferToParent() bool     { return false }
func (a *testAgent) DisallowTransferToPeers() bool      { return false }

func newTestIC(name string) InvocationContext {
	a := &testAgent{name: name, desc: "test agent"}
	s := session.NewInMemorySession("sid-1", "myapp", "user1")
	return NewInvocationContext(Params{
		Agent:        a,
		Session:      s,
		Memory:       memory.InMemoryService(),
		Artifact:     artifact.InMemoryService(),
		InvocationID: "inv-1",
		Branch:       name,
		UserContent:  "hello",
	})
}

func newActions() *event.EventActions {
	return &event.EventActions{}
}

// ---------------------------------------------------------------------------
// Test 1: ReadonlyContext exposes identity fields correctly
// ---------------------------------------------------------------------------

func TestReadonlyContextIdentityFields(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()
	cctx := NewCallbackContext(ic, actions)

	if cctx.AgentName() != "test_agent" {
		t.Errorf("AgentName = %q, want 'test_agent'", cctx.AgentName())
	}
	if cctx.UserID() != "user1" {
		t.Errorf("UserID = %q, want 'user1'", cctx.UserID())
	}
	if cctx.AppName() != "myapp" {
		t.Errorf("AppName = %q, want 'myapp'", cctx.AppName())
	}
	if cctx.SessionID() != "sid-1" {
		t.Errorf("SessionID = %q, want 'sid-1'", cctx.SessionID())
	}
	if cctx.InvocationID() != "inv-1" {
		t.Errorf("InvocationID = %q, want 'inv-1'", cctx.InvocationID())
	}
	if cctx.Branch() != "test_agent" {
		t.Errorf("Branch = %q, want 'test_agent'", cctx.Branch())
	}
	if cctx.UserContent() != "hello" {
		t.Errorf("UserContent = %q, want 'hello'", cctx.UserContent())
	}
}

// ---------------------------------------------------------------------------
// Test 2: ReadonlyState is truly read‑only (writes are not allowed)
// ---------------------------------------------------------------------------

func TestReadonlyStateIsReadOnly(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()
	cctx := NewCallbackContext(ic, actions)

	rs := cctx.ReadonlyState()
	_, ok := rs.Get("nonexistent")
	if ok {
		t.Error("expected nonexistent key to not be found")
	}

	all := rs.All()
	if len(all) != 0 {
		t.Errorf("expected empty All(), got %d entries", len(all))
	}
}

// ---------------------------------------------------------------------------
// Test 3: state write‑through — Set records in both delta and durable state
// ---------------------------------------------------------------------------

func TestCallbackStateWriteThrough(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()
	cctx := NewCallbackContext(ic, actions)

	st := cctx.State()
	st.Set("my_key", "my_value")

	// Verify delta recording.
	if v, ok := actions.StateDelta["my_key"]; !ok {
		t.Error("my_key should be in StateDelta")
	} else if v != "my_value" {
		t.Errorf("StateDelta[my_key] = %v, want 'my_value'", v)
	}

	// Verify durable state write‑through.
	if v, ok := ic.Session().State().Get("my_key"); !ok {
		t.Error("my_key should be in durable session state")
	} else if v != "my_value" {
		t.Errorf("durable state my_key = %v, want 'my_value'", v)
	}
}

// ---------------------------------------------------------------------------
// Test 4: Get reads delta before durable state
// ---------------------------------------------------------------------------

func TestCallbackStateGetPriorityDeltaFirst(t *testing.T) {
	ic := newTestIC("test_agent")

	// Set a value in durable state.
	ic.Session().State().Set("shadow", "durable_value")

	actions := newActions()
	cctx := NewCallbackContext(ic, actions)

	// Write to delta (simulating a previous callback in same step).
	actions.StateDelta["shadow"] = "delta_value"

	// Read via callback state — should see delta value.
	st := cctx.State()
	v, ok := st.Get("shadow")
	if !ok {
		t.Error("shadow should be found")
	}
	if v != "delta_value" {
		t.Errorf("Get(shadow) = %v, want 'delta_value' (delta priority)", v)
	}

	// A key only in durable state should still be readable.
	v2, ok2 := st.Get("shadow")
	if !ok2 {
		t.Error("reading from delta pathway failed")
	}
	if v2 != "delta_value" {
		t.Errorf("v2 = %v, want 'delta_value'", v2)
	}
}

// ---------------------------------------------------------------------------
// Test 5: callback sees its own write (intra‑step delta read)
// ---------------------------------------------------------------------------

func TestCallbackStateIntraStepDeltaVisible(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()
	cctx := NewCallbackContext(ic, actions)

	st := cctx.State()

	// Write via state.
	st.Set("temp_var", 42)

	// Read back — should see the delta value.
	v, ok := st.Get("temp_var")
	if !ok {
		t.Error("temp_var should be visible after Set (intra‑step delta)")
	}
	if v != 42 {
		t.Errorf("temp_var = %v, want 42", v)
	}

	// All() should include the delta key.
	all := st.All()
	if val, exists := all["temp_var"]; !exists {
		t.Error("All() should include temp_var from delta")
	} else if val != 42 {
		t.Errorf("All()[temp_var] = %v, want 42", val)
	}
}

// ---------------------------------------------------------------------------
// Test 6: multiple callbacks sharing delta — second sees first's writes
// ---------------------------------------------------------------------------

func TestCallbackStateDeltaAcrossCallbacks(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()

	cctx := NewCallbackContext(ic, actions)

	// Simulate callback 1 writing state.
	st1 := cctx.State()
	st1.Set("stage", "from_callback_1")

	// In the same step, another callback gets the same cctx.
	// It should see callback 1's write via the delta.
	st2 := cctx.State()
	v, ok := st2.Get("stage")
	if !ok {
		t.Error("callback 2 should see callback 1's write via delta")
	}
	if v != "from_callback_1" {
		t.Errorf("stage = %v, want 'from_callback_1'", v)
	}
}

// ---------------------------------------------------------------------------
// Test 7: state Delete writes tombstone and removes from durable state
// ---------------------------------------------------------------------------

func TestCallbackStateDelete(t *testing.T) {
	ic := newTestIC("test_agent")
	ic.Session().State().Set("to_delete", "exists")

	actions := newActions()
	cctx := NewCallbackContext(ic, actions)

	st := cctx.State()
	st.Delete("to_delete")

	// Delta should contain tombstone.
	if v, ok := actions.StateDelta["to_delete"]; !ok {
		t.Error("StateDelta should contain tombstone value")
	} else if v != session.TombstoneValue {
		t.Errorf("StateDelta[to_delete] = %v, want TombstoneValue", v)
	}

	// Durable state should have the key removed.
	if _, ok := ic.Session().State().Get("to_delete"); ok {
		t.Error("durable state should not have 'to_delete' after Delete")
	}
}

// ---------------------------------------------------------------------------
// Test 8: artifact save tracking records version in ArtifactDelta
// ---------------------------------------------------------------------------

func TestArtifactSaveTracking(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()
	cctx := NewCallbackContextWithArtifactTracking(ic, actions)

	artSvc := cctx.ArtifactService()

	resp, err := artSvc.Save(stdctx.Background(), &artifact.SaveRequest{
		AppName:   ic.AppName(),
		UserID:    ic.UserID(),
		SessionID: ic.SessionID(),
		FileName:  "report.txt",
		Part:      &artifact.ArtifactPart{Text: "hello world"},
	})
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if resp.Version != 1 {
		t.Errorf("Version = %d, want 1", resp.Version)
	}

	// ArtifactDelta should be recorded.
	if v, ok := actions.ArtifactDelta["report.txt"]; !ok {
		t.Error("ArtifactDelta should contain report.txt")
	} else if v != 1 {
		t.Errorf("ArtifactDelta[report.txt] = %d, want 1", v)
	}
}

// ---------------------------------------------------------------------------
// Test 9: artifact save tracking records multiple files
// ---------------------------------------------------------------------------

func TestArtifactSaveTrackingMultiple(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()
	cctx := NewCallbackContextWithArtifactTracking(ic, actions)

	artSvc := cctx.ArtifactService()

	artSvc.Save(stdctx.Background(), &artifact.SaveRequest{
		AppName:   ic.AppName(),
		UserID:    ic.UserID(),
		SessionID: ic.SessionID(),
		FileName:  "a.txt",
		Part:      &artifact.ArtifactPart{Text: "A"},
	})
	artSvc.Save(stdctx.Background(), &artifact.SaveRequest{
		AppName:   ic.AppName(),
		UserID:    ic.UserID(),
		SessionID: ic.SessionID(),
		FileName:  "b.txt",
		Part:      &artifact.ArtifactPart{Text: "B"},
	})
	artSvc.Save(stdctx.Background(), &artifact.SaveRequest{
		AppName:   ic.AppName(),
		UserID:    ic.UserID(),
		SessionID: ic.SessionID(),
		FileName:  "a.txt",
		Part:      &artifact.ArtifactPart{Text: "A v2"},
	})

	if actions.ArtifactDelta["a.txt"] != 2 {
		t.Errorf("ArtifactDelta[a.txt] = %d, want 2", actions.ArtifactDelta["a.txt"])
	}
	if actions.ArtifactDelta["b.txt"] != 1 {
		t.Errorf("ArtifactDelta[b.txt] = %d, want 1", actions.ArtifactDelta["b.txt"])
	}
	if len(actions.ArtifactDelta) != 2 {
		t.Errorf("len(ArtifactDelta) = %d, want 2", len(actions.ArtifactDelta))
	}
}

// ---------------------------------------------------------------------------
// Test 10: non‑tracked artifact service does not record deltas
// ---------------------------------------------------------------------------

func TestArtifactNoTrackingWithoutWrapper(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()

	// Use NewCallbackContext (without artifact tracking).
	cctx := NewCallbackContext(ic, actions)

	artSvc := cctx.ArtifactService()
	artSvc.Save(stdctx.Background(), &artifact.SaveRequest{
		AppName:   ic.AppName(),
		UserID:    ic.UserID(),
		SessionID: ic.SessionID(),
		FileName:  "secret.txt",
		Part:      &artifact.ArtifactPart{Text: "secret"},
	})

	if len(actions.ArtifactDelta) != 0 {
		t.Errorf("ArtifactDelta should be empty without tracking, got %v", actions.ArtifactDelta)
	}
}

// ---------------------------------------------------------------------------
// Test 11: ToolContext provides FunctionCallID and Actions access
// ---------------------------------------------------------------------------

func TestToolContextFields(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()
	tctx := NewToolContext(ic, "fc_12345", actions)

	if tctx.FunctionCallID() != "fc_12345" {
		t.Errorf("FunctionCallID = %q, want 'fc_12345'", tctx.FunctionCallID())
	}
	if tctx.Actions() != actions {
		t.Error("Actions() should return the same actions pointer")
	}
}

// ---------------------------------------------------------------------------
// Test 12: SearchMemory on ToolContext works
// ---------------------------------------------------------------------------

func TestToolContextSearchMemory(t *testing.T) {
	ic := newTestIC("test_agent")

	// Add an event to memory by ingesting session.
	ic.Session().AppendEvent(&event.Event{
		ID:      "ev1",
		Author:  "test_agent",
		Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "Paris weather is sunny"}}},
	})
	ic.MemoryService().AddSessionToMemory(stdctx.Background(), ic.Session())

	actions := newActions()
	tctx := NewToolContext(ic, "fc1", actions)

	resp, err := tctx.SearchMemory(stdctx.Background(), "Paris weather")
	if err != nil {
		t.Fatalf("SearchMemory error: %v", err)
	}
	if len(resp.Memories) != 1 {
		t.Errorf("expected 1 memory, got %d", len(resp.Memories))
	}
	if resp.Memories[0].Author != "test_agent" {
		t.Errorf("memory author = %q, want 'test_agent'", resp.Memories[0].Author)
	}

	// Empty search should return nothing.
	resp2, _ := tctx.SearchMemory(stdctx.Background(), "nothingmatches")
	if len(resp2.Memories) != 0 {
		t.Errorf("expected 0 memories, got %d", len(resp2.Memories))
	}
}

// ==========================================================================
// early‑exit behaviour with CallbackContext‑aware callbacks
// ==========================================================================

// ---------------------------------------------------------------------------
// Test 13: before callback early‑exit skips agent run
// ---------------------------------------------------------------------------

func TestRunWithCallbackContextBeforeEarlyExit(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()

	runCalled := false

	events, err := RunWithCallbackContext(ic, actions, nil,
		[]agent.BeforeAgentCallbackCtx{
			func(ctx callbackctx.CallbackContext) (*event.Event, error) {
				return &event.Event{
					ID: "early_exit",
					Content: &event.Content{
						Role:  event.RoleModel,
						Parts: []event.Part{{Text: "early"}},
					},
				}, nil
			},
		},
		func(ctx agent.InvocationContext) ([]*event.Event, error) {
			runCalled = true
			return []*event.Event{{ID: "should_not_appear"}}, nil
		},
		nil,
	)

	if err != nil {
		t.Fatalf("RunWithCallbackContext error: %v", err)
	}
	if runCalled {
		t.Error("agent run should NOT have been called (early exit)")
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != "early_exit" {
		t.Errorf("event ID = %q, want 'early_exit'", events[0].ID)
	}
}

// ---------------------------------------------------------------------------
// Test 14: after callback is invoked after agent run
// ---------------------------------------------------------------------------

func TestRunWithCallbackContextAfterCallback(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()

	events, err := RunWithCallbackContext(ic, actions, nil,
		nil,
		func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return []*event.Event{{ID: "run_event",
				Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "result"}}}}}, nil
		},
		[]agent.AfterAgentCallbackCtx{
			func(ctx callbackctx.CallbackContext, events []*event.Event) (*event.Event, error) {
				return &event.Event{
					ID:      "after_event",
					Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "after"}}},
				}, nil
			},
		},
	)

	if err != nil {
		t.Fatalf("RunWithCallbackContext error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].ID != "run_event" {
		t.Errorf("first event = %q, want 'run_event'", events[0].ID)
	}
	if events[1].ID != "after_event" {
		t.Errorf("second event = %q, want 'after_event'", events[1].ID)
	}
}

// ---------------------------------------------------------------------------
// Test 15: state delta from callbacks persists after early exit
// ---------------------------------------------------------------------------

func TestStateDeltaPersistsAfterEarlyExit(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()

	_, err := RunWithCallbackContext(ic, actions, nil,
		[]agent.BeforeAgentCallbackCtx{
			func(ctx callbackctx.CallbackContext) (*event.Event, error) {
				ctx.State().Set("was_here", true)
				// Return event to trigger early exit.
				return &event.Event{
					ID:      "exit",
					Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "shortcut"}}},
				}, nil
			},
		},
		nil, nil,
	)
	if err != nil {
		t.Fatalf("RunWithCallbackContext error: %v", err)
	}

	// State should be persisted despite early exit (write‑through).
	if v, ok := ic.Session().State().Get("was_here"); !ok {
		t.Error("state 'was_here' should persist after early‑exit callback")
	} else if v != true {
		t.Errorf("was_here = %v, want true", v)
	}

	// Delta should still be recorded.
	if v, ok := actions.StateDelta["was_here"]; !ok {
		t.Error("StateDelta should contain 'was_here'")
	} else if v != true {
		t.Errorf("StateDelta[was_here] = %v, want true", v)
	}
}

// ---------------------------------------------------------------------------
// Test 16: before callback error aborts with error
// ---------------------------------------------------------------------------

func TestRunWithCallbackContextBeforeError(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()

	_, err := RunWithCallbackContext(ic, actions, nil,
		[]agent.BeforeAgentCallbackCtx{
			func(ctx callbackctx.CallbackContext) (*event.Event, error) {
				return nil, fmt.Errorf("deadly error")
			},
		},
		nil, nil,
	)

	if err == nil {
		t.Error("expected error from failing before callback")
	}
}

// ---------------------------------------------------------------------------
// Test 17: after callback error aborts with error
// ---------------------------------------------------------------------------

func TestRunWithCallbackContextAfterError(t *testing.T) {
	ic := newTestIC("test_agent")
	actions := newActions()

	_, err := RunWithCallbackContext(ic, actions, nil,
		nil,
		func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return nil, nil
		},
		[]agent.AfterAgentCallbackCtx{
			func(ctx callbackctx.CallbackContext, events []*event.Event) (*event.Event, error) {
				return nil, fmt.Errorf("after error")
			},
		},
	)

	if err == nil {
		t.Error("expected error from failing after callback")
	}
}

// ---------------------------------------------------------------------------
// Test 18: legacy callbacks still work (unchanged)
// ---------------------------------------------------------------------------

func TestLegacyCallbacksStillWork(t *testing.T) {
	a, err := agent.New(agent.Config{
		Name: "legacy_agent",
		BeforeAgentCallbacks: []agent.BeforeAgentCallback{
			func(ctx agent.InvocationContext) (*event.Event, error) {
				return nil, nil
			},
		},
		AfterAgentCallbacks: []agent.AfterAgentCallback{
			func(ctx agent.InvocationContext, events []*event.Event) (*event.Event, error) {
				return nil, nil
			},
		},
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return []*event.Event{{ID: "e1",
				Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "ok"}}}}}, nil
		},
	})
	if err != nil {
		t.Fatalf("agent.New error: %v", err)
	}

	// Use mock context from agent_test pattern.
	ctx := &mockInvocationContext{agent: a}
	events, err := a.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

type mockInvocationContext struct {
	agent agent.Agent
	ended bool
}

func (m *mockInvocationContext) Agent() agent.Agent { return m.agent }
func (m *mockInvocationContext) EndInvocation()     { m.ended = true }
func (m *mockInvocationContext) Ended() bool        { return m.ended }

// ---------------------------------------------------------------------------
// Test 19: prepareEventActions initializes nil maps
// ---------------------------------------------------------------------------

func TestPrepareEventActionsInitializesNilMaps(t *testing.T) {
	actions := &event.EventActions{}
	prepareEventActions(actions)

	if actions.StateDelta == nil {
		t.Error("StateDelta should be non-nil after prepare")
	}
	if actions.ArtifactDelta == nil {
		t.Error("ArtifactDelta should be non-nil after prepare")
	}
}

// ---------------------------------------------------------------------------
// Test 20: all identity fields in ToolContext
// ---------------------------------------------------------------------------

func TestToolContextInheritsIdentity(t *testing.T) {
	ic := newTestIC("tool_agent")
	actions := newActions()
	tctx := NewToolContext(ic, "fc1", actions)

	if tctx.AgentName() != "tool_agent" {
		t.Errorf("AgentName = %q", tctx.AgentName())
	}
	if tctx.UserID() != "user1" {
		t.Errorf("UserID = %q", tctx.UserID())
	}
	if tctx.AppName() != "myapp" {
		t.Errorf("AppName = %q", tctx.AppName())
	}
	if tctx.SessionID() != "sid-1" {
		t.Errorf("SessionID = %q", tctx.SessionID())
	}
}
