package agent

import (
	"errors"
	"testing"

	"github.com/likun666661/rive-adk-go/event"
)

type mockInvocationContext struct {
	agent Agent
	ended bool
}

func (m *mockInvocationContext) Agent() Agent    { return m.agent }
func (m *mockInvocationContext) EndInvocation()  { m.ended = true }
func (m *mockInvocationContext) Ended() bool      { return m.ended }

func TestNewAgentValidation(t *testing.T) {
	_, err := New(Config{Name: ""})
	if err == nil {
		t.Error("expected error for empty name")
	}

	_, err = New(Config{Name: "a", Run: nil})
	if err == nil {
		t.Error("expected error for nil run")
	}
}

func TestAgentExecuteBasic(t *testing.T) {
	a, err := New(Config{
		Name: "test_agent",
		Run: func(ctx InvocationContext) ([]*event.Event, error) {
			return []*event.Event{
				{ID: "ev1", Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "response"}}}},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Name() != "test_agent" {
		t.Errorf("Name = %q, want 'test_agent'", a.Name())
	}

	ctx := &mockInvocationContext{agent: a}
	events, err := a.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Content.Parts[0].Text != "response" {
		t.Errorf("text = %q, want 'response'", events[0].Content.Parts[0].Text)
	}
}

func TestBeforeAgentCallbackEarlyExit(t *testing.T) {
	a, err := New(Config{
		Name: "test_agent",
		BeforeAgentCallbacks: []BeforeAgentCallback{
			func(ctx InvocationContext) (*event.Event, error) {
				return &event.Event{
					ID:      "early",
					Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "early exit"}}},
				}, nil
			},
		},
		Run: func(ctx InvocationContext) ([]*event.Event, error) {
			return []*event.Event{
				{ID: "should_not_appear", Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "should not appear"}}}},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := &mockInvocationContext{agent: a}
	events, err := a.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event from early exit, got %d", len(events))
	}
	if events[0].ID != "early" {
		t.Errorf("event ID = %q, want 'early'", events[0].ID)
	}
}

func TestAfterAgentCallback(t *testing.T) {
	a, err := New(Config{
		Name: "test_agent",
		Run: func(ctx InvocationContext) ([]*event.Event, error) {
			return []*event.Event{
				{ID: "run_ev", Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "from run"}}}},
			}, nil
		},
		AfterAgentCallbacks: []AfterAgentCallback{
			func(ctx InvocationContext, events []*event.Event) (*event.Event, error) {
				return &event.Event{
					ID:      "after_ev",
					Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "from after"}}},
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := &mockInvocationContext{agent: a}
	events, err := a.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events (run + after), got %d", len(events))
	}
	if events[0].ID != "run_ev" {
		t.Errorf("first event ID = %q, want 'run_ev'", events[0].ID)
	}
	if events[1].ID != "after_ev" {
		t.Errorf("second event ID = %q, want 'after_ev'", events[1].ID)
	}
}

func TestAfterAgentCallbackEndInvocation(t *testing.T) {
	a, err := New(Config{
		Name: "test_agent",
		Run: func(ctx InvocationContext) ([]*event.Event, error) {
			return []*event.Event{
				{ID: "run_ev", Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "done"}}}},
			}, nil
		},
		AfterAgentCallbacks: []AfterAgentCallback{
			func(ctx InvocationContext, events []*event.Event) (*event.Event, error) {
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

	ctx := &mockInvocationContext{agent: a}
	events, err := a.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ctx.Ended() {
		t.Error("expected invocation to be ended after afterCallback with EndInvocation")
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestFinalResponseDetection(t *testing.T) {
	partialEv := &event.Event{
		ID:      "p1",
		Partial: true,
		Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "streaming..."}}},
	}
	if partialEv.IsFinalResponse() {
		t.Error("partial event should not be final")
	}

	finalEv := &event.Event{
		ID:      "f1",
		Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "complete response"}}},
	}
	if !finalEv.IsFinalResponse() {
		t.Error("text-only non-partial event should be final")
	}

	funcCallEv := &event.Event{
		ID:      "fc1",
		Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{
			{Text: "let me check"},
			{FunctionCall: &event.FunctionCall{Name: "get_weather"}},
		}},
	}
	if funcCallEv.IsFinalResponse() {
		t.Error("event with function call should not be final")
	}
}

func TestBeforeAgentCallbackError(t *testing.T) {
	a, err := New(Config{
		Name: "test_agent",
		BeforeAgentCallbacks: []BeforeAgentCallback{
			func(ctx InvocationContext) (*event.Event, error) {
				return nil, errors.New("callback failed")
			},
		},
		Run: func(ctx InvocationContext) ([]*event.Event, error) {
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := &mockInvocationContext{agent: a}
	_, err = a.Execute(ctx)
	if err == nil {
		t.Error("expected error from failing before callback")
	}
}

func TestRunError(t *testing.T) {
	a, err := New(Config{
		Name: "test_agent",
		Run: func(ctx InvocationContext) ([]*event.Event, error) {
			return nil, errors.New("run failed")
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := &mockInvocationContext{agent: a}
	_, err = a.Execute(ctx)
	if err == nil {
		t.Error("expected error from failing run")
	}
}

func TestAfterAgentCallbackError(t *testing.T) {
	a, err := New(Config{
		Name: "test_agent",
		Run: func(ctx InvocationContext) ([]*event.Event, error) {
			return []*event.Event{{ID: "e1"}}, nil
		},
		AfterAgentCallbacks: []AfterAgentCallback{
			func(ctx InvocationContext, events []*event.Event) (*event.Event, error) {
				return nil, errors.New("after callback failed")
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := &mockInvocationContext{agent: a}
	_, err = a.Execute(ctx)
	if err == nil {
		t.Error("expected error from failing after callback")
	}
}
