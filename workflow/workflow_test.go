package workflow

import (
	stdctx "context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/flow"
	"github.com/likun666661/rive-adk-go/llmagent"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/runner"
	"github.com/likun666661/rive-adk-go/session"
	"github.com/likun666661/rive-adk-go/tool"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newSubAgent creates an LLM‑backed sub‑agent that is guaranteed to satisfy
// SubAgent (agent.Agent + Execute).
func newSubAgent(t *testing.T, name string, responses ...*model.LLMResponse) SubAgent {
	t.Helper()
	f := &flow.Flow{Model: model.NewFakeModel("fake-"+name, responses...)}
	a, err := llmagent.New(name, name+" description", f)
	if err != nil {
		t.Fatal(err)
	}
	sa, ok := a.(SubAgent)
	if !ok {
		t.Fatalf("%s does not implement SubAgent", name)
	}
	return sa
}

// newSubAgentWithTool creates an LLM‑backed sub‑agent with a tool map.
func newSubAgentWithTool(t *testing.T, name string, tools map[string]tool.FunctionTool, responses ...*model.LLMResponse) SubAgent {
	t.Helper()
	f := &flow.Flow{
		Model: model.NewFakeModel("fake-"+name, responses...),
		Tools: tools,
	}
	a, err := llmagent.New(name, name+" description", f)
	if err != nil {
		t.Fatal(err)
	}
	sa, ok := a.(SubAgent)
	if !ok {
		t.Fatalf("%s does not implement SubAgent", name)
	}
	return sa
}

// newRawSubAgent creates a sub‑agent from agent.Config directly (no flow).
// agent.New returns *baseAgent which implements SubAgent via its Execute method.
func newRawSubAgent(t *testing.T, name string, run func(ctx agent.InvocationContext) ([]*event.Event, error)) SubAgent {
	t.Helper()
	a, err := agent.New(agent.Config{
		Name:        name,
		Description: name + " description",
		Run:         run,
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// countEscalateEvents counts events with Escalate=true in a slice.
func countEscalateEvents(events []*event.Event) int {
	n := 0
	for _, ev := range events {
		if ev != nil && ev.Actions.Escalate {
			n++
		}
	}
	return n
}

// =============================================================================
// Sequential agent tests
// =============================================================================

func TestSequentialAgentOrder(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	sa1 := newSubAgent(t, "first", model.TextResponse("from first"))
	sa2 := newSubAgent(t, "second", model.TextResponse("from second"))
	sa3 := newSubAgent(t, "third", model.TextResponse("from third"))

	seq := NewSequentialAgent("seq", "sequential test", []SubAgent{sa1, sa2, sa3})

	r, err := runner.New(runner.Config{
		AppName:        "test_seq",
		Agent:          seq,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-seq", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Each sub‑agent produces 1 text event → 3 total agent events.
	if len(events) != 3 {
		t.Fatalf("expected 3 agent events, got %d", len(events))
	}

	// Verify order by Author.
	for i, want := range []string{"first", "second", "third"} {
		if events[i].Author != want {
			t.Errorf("event[%d].Author = %q, want %q", i, events[i].Author, want)
		}
	}

	// Session should have user + 3 agent events = 4.
	if sess.EventCount() != 4 {
		t.Errorf("expected 4 session events, got %d", sess.EventCount())
	}
}

func TestSequentialAgentErrorStopsChain(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	sa1 := newSubAgent(t, "ok", model.TextResponse("ok"))
	saFail := newRawSubAgent(t, "failer", func(ctx agent.InvocationContext) ([]*event.Event, error) {
		return nil, errors.New("boom")
	})
	sa3 := newSubAgent(t, "never", model.TextResponse("should not appear"))

	seq := NewSequentialAgent("seq", "error test", []SubAgent{sa1, saFail, sa3})

	r, err := runner.New(runner.Config{
		AppName:        "test_seq_err",
		Agent:          seq,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-err", "go")
	if err == nil {
		t.Error("expected error from failing sub‑agent")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should mention 'boom', got: %v", err)
	}

	// The first sub‑agent's events should still be present.
	if len(events) == 0 {
		t.Error("expected events from first sub‑agent before error")
	}
	if len(events) > 0 && events[0].Author != "ok" {
		t.Errorf("first event author = %q, want 'ok'", events[0].Author)
	}
}

func TestSequentialAgentEndInvocationStopsChain(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	saEnd := newRawSubAgent(t, "ender", func(ctx agent.InvocationContext) ([]*event.Event, error) {
		ctx.EndInvocation()
		return []*event.Event{
			{ID: "e1", Author: "ender", Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "ending"}}}},
		}, nil
	})
	saNever := newSubAgent(t, "never", model.TextResponse("should not appear"))

	seq := NewSequentialAgent("seq", "end test", []SubAgent{saEnd, saNever})

	r, err := runner.New(runner.Config{
		AppName:        "test_seq_end",
		Agent:          seq,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-end", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Only the first sub‑agent should have produced events.
	if len(events) != 1 {
		t.Fatalf("expected 1 event from first sub‑agent, got %d", len(events))
	}
	if events[0].Author != "ender" {
		t.Errorf("author = %q, want 'ender'", events[0].Author)
	}
}

// =============================================================================
// Parallel agent tests
// =============================================================================

func TestParallelAgentBranchAndEventAggregation(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	sa1 := newSubAgent(t, "analyst", model.TextResponse("analysis result"))
	sa2 := newSubAgent(t, "critic", model.TextResponse("critique here"))
	sa3 := newSubAgent(t, "evaluator", model.TextResponse("evaluation done"))

	par := NewParallelAgent("par", "parallel test", []SubAgent{sa1, sa2, sa3})

	r, err := runner.New(runner.Config{
		AppName:        "test_par",
		Agent:          par,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-par", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Each sub‑agent produces 1 event → 3 total.
	if len(events) != 3 {
		t.Fatalf("expected 3 agent events, got %d", len(events))
	}

	// Events should appear in sub‑agent declaration order (deterministic).
	for i, want := range []string{"analyst", "critic", "evaluator"} {
		if events[i].Author != want {
			t.Errorf("event[%d].Author = %q, want %q", i, events[i].Author, want)
		}
	}

	// Branch metadata should use parent.child labels.
	for i, want := range []string{"par.analyst", "par.critic", "par.evaluator"} {
		if events[i].Branch != want {
			t.Errorf("event[%d].Branch = %q, want %q", i, events[i].Branch, want)
		}
	}
}

func TestParallelAgentErrorPropagation(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	// Create separate instances — sharing a single sub‑agent instance across
	// parallel goroutines causes concurrent access to the same fake model.
	saOK1 := newSubAgent(t, "ok-1", model.TextResponse("ok"))
	saOK2 := newSubAgent(t, "ok-2", model.TextResponse("ok"))
	saErr := newRawSubAgent(t, "err", func(ctx agent.InvocationContext) ([]*event.Event, error) {
		return nil, fmt.Errorf("sub‑agent failure")
	})

	// Place the failing agent between two successful ones.
	par := NewParallelAgent("par", "error test", []SubAgent{saOK1, saErr, saOK2})

	r, err := runner.New(runner.Config{
		AppName:        "test_par_err",
		Agent:          par,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-par-err", "go")
	if err == nil {
		t.Error("expected error from failing sub‑agent")
	}
	if !strings.Contains(err.Error(), "sub‑agent failure") {
		t.Errorf("error should mention 'sub‑agent failure', got: %v", err)
	}

	// Successful sub‑agents' events should still be present (2 ok sub‑agents * 1 event = 2).
	if len(events) != 2 {
		t.Fatalf("expected 2 events from ok sub‑agents, got %d", len(events))
	}
	// Verify the ok‑1 event is first (declaration order dictates event ordering).
	if events[0].Author != "ok-1" {
		t.Errorf("event[0] author = %q, want 'ok-1'", events[0].Author)
	}
}

// =============================================================================
// Loop agent tests
// =============================================================================

func TestLoopAgentMaxIterations(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	// Each of 3 iterations consumes one response from the fake model queue.
	sa := newSubAgent(t, "worker",
		model.TextResponse("loop response"),
		model.TextResponse("loop response"),
		model.TextResponse("loop response"),
	)

	loop := NewLoopAgent("loop", "max iterations test", []SubAgent{sa}, 3)

	r, err := runner.New(runner.Config{
		AppName:        "test_loop",
		Agent:          loop,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-loop", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// 3 iterations * 1 sub‑agent * 1 event each = 3 events.
	if len(events) != 3 {
		t.Fatalf("expected 3 agent events (3 iterations), got %d", len(events))
	}
	for _, ev := range events {
		if ev.Author != "worker" {
			t.Errorf("author = %q, want 'worker'", ev.Author)
		}
	}
}

func TestLoopAgentEarlyStopOnEscalate(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	// This sub‑agent produces an Escalate event on its first call.
	sa := newRawSubAgent(t, "escalater", func(ctx agent.InvocationContext) ([]*event.Event, error) {
		return []*event.Event{{
			ID:      "esc",
			Author:  "escalater",
			Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "escalating!"}}},
			Actions: event.EventActions{Escalate: true},
		}}, nil
	})

	// maxIterations = 10, but should stop after 1 due to escalate.
	loop := NewLoopAgent("loop", "escalate test", []SubAgent{sa}, 10)

	r, err := runner.New(runner.Config{
		AppName:        "test_loop_esc",
		Agent:          loop,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-esc", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Should stop after 1 iteration (1 event from the escalater).
	if len(events) != 1 {
		t.Fatalf("expected 1 event (escalate stops loop), got %d", len(events))
	}
	if !events[0].Actions.Escalate {
		t.Error("expected Escalate=true on the event")
	}
}

func TestLoopAgentErrorStopsLoop(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	callCount := 0
	sa := newRawSubAgent(t, "counter", func(ctx agent.InvocationContext) ([]*event.Event, error) {
		callCount++
		if callCount >= 3 {
			return nil, errors.New("too many iterations")
		}
		return []*event.Event{
			{ID: fmt.Sprintf("ev%d", callCount), Author: "counter", Content: &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: fmt.Sprintf("iteration %d", callCount)}},
			}},
		}, nil
	})

	// maxIterations = 100, error at iteration 3.
	loop := NewLoopAgent("loop", "error test", []SubAgent{sa}, 100)

	r, err := runner.New(runner.Config{
		AppName:        "test_loop_err",
		Agent:          loop,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-lerr", "go")
	if err == nil {
		t.Error("expected error from loop sub‑agent")
	}
	if !strings.Contains(err.Error(), "too many iterations") {
		t.Errorf("error should mention 'too many iterations', got: %v", err)
	}

	// Events from iterations 1 and 2 should be present (2 events).
	if len(events) != 2 {
		t.Fatalf("expected 2 events from first 2 iterations, got %d", len(events))
	}
}

func TestLoopAgentZeroMaxIterations(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	callCount := 0
	sa := newRawSubAgent(t, "limited", func(ctx agent.InvocationContext) ([]*event.Event, error) {
		callCount++
		// Escalate after 5 iterations to prevent infinite loop in test.
		if callCount >= 5 {
			return []*event.Event{{
				ID:      "esc",
				Author:  "limited",
				Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "done"}}},
				Actions: event.EventActions{Escalate: true},
			}}, nil
		}
		return []*event.Event{
			{ID: fmt.Sprintf("ev%d", callCount), Author: "limited", Content: &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: fmt.Sprintf("iteration %d", callCount)}},
			}},
		}, nil
	})

	// maxIterations = 0 (infinite), stops on escalate.
	loop := NewLoopAgent("loop", "infinite test", []SubAgent{sa}, 0)

	r, err := runner.New(runner.Config{
		AppName:        "test_loop_inf",
		Agent:          loop,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-inf", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// 4 iterations + 1 escalate event = 5 events.
	if len(events) != 5 {
		t.Fatalf("expected 5 events (4 iterations + escalate), got %d", len(events))
	}
	if countEscalateEvents(events) != 1 {
		t.Error("expected exactly 1 escalate event")
	}
}

// =============================================================================
// Integration: workflow agent as sub‑agent of another workflow
// =============================================================================

func TestNestedSequentialInParallel(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	innerSeq := NewSequentialAgent("inner", "inner seq", []SubAgent{
		newSubAgent(t, "step1", model.TextResponse("step1")),
		newSubAgent(t, "step2", model.TextResponse("step2")),
	})

	par := NewParallelAgent("outer", "outer par", []SubAgent{
		innerSeq,
		newSubAgent(t, "side", model.TextResponse("side")),
	})

	r, err := runner.New(runner.Config{
		AppName:        "test_nest",
		Agent:          par,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-nest", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Events: step1, step2 (from inner) + side (from parallel agent) = 3.
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	authors := make(map[string]int)
	for _, ev := range events {
		authors[ev.Author]++
	}
	if authors["step1"] != 1 || authors["step2"] != 1 || authors["side"] != 1 {
		t.Errorf("unexpected author distribution: %v", authors)
	}
}

// =============================================================================
// State sharing across sequential sub‑agents
// =============================================================================

func TestSequentialAgentStateSharing(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	// Sub‑agent that writes state via tool.
	writerTool := tool.NewFunctionTool("write_state", "Write state",
		func(args map[string]any) (map[string]any, error) {
			key, _ := args["key"].(string)
			val, _ := args["value"].(string)
			return map[string]any{
				"state_delta": map[string]any{key: val},
			}, nil
		},
	)

	writer := newSubAgentWithTool(t, "writer",
		map[string]tool.FunctionTool{"write_state": writerTool},
		model.FunctionCallResponse("Writing state.",
			event.FunctionCall{ID: "fc1", Name: "write_state", Args: map[string]any{"key": "shared", "value": "hello"}},
		),
		model.TextResponse("done"),
	)

	// Sub‑agent that reads state via its Run function.
	// subCtx (the wrapper) embeds context.InvocationContext so all session
	// methods are available through type assertion to a named interface.
	var readValue any
	type sessionViewer interface {
		Session() session.Session
	}
	reader := newRawSubAgent(t, "reader", func(ctx agent.InvocationContext) ([]*event.Event, error) {
		sc, ok := ctx.(sessionViewer)
		if !ok {
			return nil, fmt.Errorf("cannot access session state: context is %T", ctx)
		}
		v, _ := sc.Session().State().Get("shared")
		readValue = v
		return []*event.Event{
			{ID: "rd", Author: "reader", Content: &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: fmt.Sprintf("read: %v", v)}},
			}},
		}, nil
	})

	seq := NewSequentialAgent("seq", "state share", []SubAgent{writer, reader})

	r, err := runner.New(runner.Config{
		AppName:        "test_state",
		Agent:          seq,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-state", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	_ = events

	if readValue != "hello" {
		t.Errorf("reader saw shared value = %v, want 'hello'", readValue)
	}
}

// =============================================================================
// Loop agent with multi sub‑agent iteration
// =============================================================================

func TestLoopAgentMultipleSubAgents(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	// 2 iterations * 2 sub‑agents = 4 calls to fake model.
	sa1 := newSubAgent(t, "coder",
		model.TextResponse("code generated"),
		model.TextResponse("code generated v2"),
	)
	sa2 := newSubAgent(t, "reviewer",
		model.TextResponse("review done"),
		model.TextResponse("review done v2"),
	)

	loop := NewLoopAgent("loop", "multi sub", []SubAgent{sa1, sa2}, 2)

	r, err := runner.New(runner.Config{
		AppName:        "test_loop_multi",
		Agent:          loop,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-multi", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// 2 iterations * 2 sub‑agents * 1 event = 4 events.
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	// Pattern should be: coder, reviewer, coder, reviewer.
	wantOrder := []string{"coder", "reviewer", "coder", "reviewer"}
	for i, want := range wantOrder {
		if events[i].Author != want {
			t.Errorf("event[%d].Author = %q, want %q", i, events[i].Author, want)
		}
	}
}

// =============================================================================
// Empty sub‑agents edge cases
// =============================================================================

func TestSequentialAgentEmptySubAgents(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()
	seq := NewSequentialAgent("seq", "empty", nil)

	r, err := runner.New(runner.Config{
		AppName:        "test_empty",
		Agent:          seq,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-empty", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events from empty sequential agent, got %d", len(events))
	}
}

func TestParallelAgentEmptySubAgents(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()
	par := NewParallelAgent("par", "empty", nil)

	r, err := runner.New(runner.Config{
		AppName:        "test_empty",
		Agent:          par,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-empty", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events from empty parallel agent, got %d", len(events))
	}
}

func TestLoopAgentEmptySubAgents(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()
	loop := NewLoopAgent("loop", "empty", nil, 100)

	r, err := runner.New(runner.Config{
		AppName:        "test_empty",
		Agent:          loop,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-empty", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events from empty loop agent, got %d", len(events))
	}
}

// =============================================================================
// Verify SequentialAgent, ParallelAgent, LoopAgent implement agent.Agent
// =============================================================================

func TestSequentialAgentImplementsAgent(t *testing.T) {
	sa1 := newSubAgent(t, "a", model.TextResponse("hi"))
	seq := NewSequentialAgent("s", "desc", []SubAgent{sa1})
	var a agent.Agent = seq
	if a.Name() != "s" {
		t.Errorf("Name = %q, want 's'", a.Name())
	}
	if a.Description() != "desc" {
		t.Errorf("Description = %q, want 'desc'", a.Description())
	}
}

func TestParallelAgentImplementsAgent(t *testing.T) {
	sa1 := newSubAgent(t, "a", model.TextResponse("hi"))
	par := NewParallelAgent("p", "desc", []SubAgent{sa1})
	var a agent.Agent = par
	if a.Name() != "p" || a.Description() != "desc" {
		t.Error("ParallelAgent should implement agent.Agent")
	}
}

func TestLoopAgentImplementsAgent(t *testing.T) {
	sa1 := newSubAgent(t, "a", model.TextResponse("hi"))
	loop := NewLoopAgent("l", "desc", []SubAgent{sa1}, 1)
	var a agent.Agent = loop
	if a.Name() != "l" || a.Description() != "desc" {
		t.Error("LoopAgent should implement agent.Agent")
	}
}
