package workflow

import (
	stdctx "context"
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
	"github.com/likun666661/rive-adk-go/tool/agenttool"
)

// getFirstText returns the text from the first content part of an event.
func getFirstText(ev *event.Event) string {
	if ev == nil || ev.Content == nil || len(ev.Content.Parts) == 0 {
		return ""
	}
	return ev.Content.Parts[0].Text
}

// =============================================================================
// Chapter 05 — End-to-end runner integration tests
//
// These tests exercise the full Runner → Agent → Event → Session chain
// for each workflow/A2A pattern described in the Chapter 05 guide.
// =============================================================================

// ---------------------------------------------------------------------------
// E2E: Sequential workflow through runner
// ---------------------------------------------------------------------------

func TestWorkflowE2E_Sequential_ThroughRunner(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	// Two sub-agents in sequence: code generator → code reviewer.
	coder := newSubAgent(t, "coder",
		model.TextResponse("func main() { println(\"hello\") }"),
	)
	reviewer := newSubAgent(t, "reviewer",
		model.TextResponse("Code review passed"),
	)

	seq := NewSequentialAgent("pipeline", "code-gen → review pipeline", []SubAgent{coder, reviewer})

	r, err := runner.New(runner.Config{
		AppName:        "e2e_seq",
		Agent:          seq,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-e2e-seq", "Write a function")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Verify order: coder first, reviewer second.
	if len(events) != 2 {
		t.Fatalf("expected 2 events (coder + reviewer), got %d", len(events))
	}
	if events[0].Author != "coder" {
		t.Errorf("event[0].Author = %q, want 'coder'", events[0].Author)
	}
	if events[1].Author != "reviewer" {
		t.Errorf("event[1].Author = %q, want 'reviewer'", events[1].Author)
	}

	// Session should have user + 2 agent events = 3.
	if sess.EventCount() != 3 {
		t.Errorf("expected 3 session events, got %d", sess.EventCount())
	}

	// Verify content.
	if coderText := getFirstText(events[0]); !strings.Contains(coderText, "println") {
		t.Errorf("coder text should contain 'println', got %q", coderText)
	}
}

// ---------------------------------------------------------------------------
// E2E: Parallel workflow with branch/event labels through runner
// ---------------------------------------------------------------------------

func TestWorkflowE2E_Parallel_WithBranchLabels(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	analyst := newSubAgent(t, "analyst", model.TextResponse("Market analysis complete"))
	critic := newSubAgent(t, "critic", model.TextResponse("Critique: over-optimistic"))
	planner := newSubAgent(t, "planner", model.TextResponse("Plan: three phases"))

	par := NewParallelAgent("review-team", "parallel review", []SubAgent{analyst, critic, planner})

	r, err := runner.New(runner.Config{
		AppName:        "e2e_par",
		Agent:          par,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-e2e-par", "Analyze the market")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Verify branch labels are set in parent.child format.
	for i, want := range []string{"review-team.analyst", "review-team.critic", "review-team.planner"} {
		if events[i].Branch != want {
			t.Errorf("event[%d].Branch = %q, want %q", i, events[i].Branch, want)
		}
	}

	// Verify declaration order is preserved.
	expectedAuthors := []string{"analyst", "critic", "planner"}
	for i, want := range expectedAuthors {
		if events[i].Author != want {
			t.Errorf("event[%d].Author = %q, want %q", i, events[i].Author, want)
		}
	}
}

// ---------------------------------------------------------------------------
// E2E: Loop workflow early stop through runner (Escalate pattern)
// ---------------------------------------------------------------------------

func TestWorkflowE2E_Loop_EarlyStop_ThroughRunner(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	// Sub-agent that emits Escalate after 2 rounds, simulating fix-complete signal.
	callCount := 0
	fixer := newRawSubAgent(t, "fixer", func(ctx agent.InvocationContext) ([]*event.Event, error) {
		callCount++
		if callCount >= 3 {
			return []*event.Event{{
				ID:      "fix-done",
				Author:  "fixer",
				Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "all tests pass"}}},
				Actions: event.EventActions{Escalate: true},
			}}, nil
		}
		return []*event.Event{{
			ID:      fmt.Sprintf("fix-%d", callCount),
			Author:  "fixer",
			Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: fmt.Sprintf("fix round %d", callCount)}}},
		}}, nil
	})

	loop := NewLoopAgent("fix-loop", "code-fix loop", []SubAgent{fixer}, 10)

	r, err := runner.New(runner.Config{
		AppName:        "e2e_loop",
		Agent:          loop,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-e2e-loop", "Fix the code")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// 2 fix events + 1 escalate event = 3 events.
	if len(events) != 3 {
		t.Fatalf("expected 3 events (2 fixes + 1 escalate), got %d", len(events))
	}
	if callCount != 3 {
		t.Errorf("expected fixer called 3 times, got %d", callCount)
	}
	if countEscalateEvents(events) != 1 {
		t.Error("expected exactly 1 escalate event")
	}
}

// ---------------------------------------------------------------------------
// E2E: Loop workflow max iterations through runner
// ---------------------------------------------------------------------------

func TestWorkflowE2E_Loop_MaxIterations_ThroughRunner(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	worker := newSubAgent(t, "worker",
		model.TextResponse("iteration 1"),
		model.TextResponse("iteration 2"),
	)

	loop := NewLoopAgent("bounded-loop", "max 2 iterations", []SubAgent{worker}, 2)

	r, err := runner.New(runner.Config{
		AppName:        "e2e_loop_max",
		Agent:          loop,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-e2e-loop-max", "Process")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events from max iterations, got %d", len(events))
	}
	for _, ev := range events {
		if ev.Author != "worker" {
			t.Errorf("author = %q, want 'worker'", ev.Author)
		}
	}
}

// ---------------------------------------------------------------------------
// E2E: AgentTool delegation through runner (agent as tool inside parent flow)
// ---------------------------------------------------------------------------

func TestWorkflowE2E_AgentTool_Delegation_ThroughRunner(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	// Child agent: a sub-agent that will be wrapped as a tool.
	childAgent, err := agent.New(agent.Config{
		Name:        "math_agent",
		Description: "Solves math problems",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			ev := event.NewEvent("math-result", "math_agent", event.RoleModel)
			ev.Content = &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: "42"}},
			}
			return []*event.Event{ev}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wrap the child agent as a tool.
	at := agenttool.New(childAgent, nil)
	ft, ok := at.(tool.FunctionTool)
	if !ok {
		t.Fatal("agenttool does not implement FunctionTool")
	}

	// Parent agent: an LLM agent that delegates to math_agent via function call.
	fakeModel := model.NewFakeModel("parent-model",
		model.FunctionCallResponse("Let me delegate to math_agent.",
			event.FunctionCall{ID: "fc-math", Name: "math_agent", Args: map[string]any{"request": "what is 6*7"}},
		),
		model.TextResponse("The answer is 42 according to the math agent."),
	)

	parentFlow := &flow.Flow{
		Model: fakeModel,
		Tools: map[string]tool.FunctionTool{
			"math_agent": ft,
		},
	}

	parentAgent, err := llmagent.New("orchestrator", "Parent agent that delegates to child agents.", parentFlow)
	if err != nil {
		t.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:        "e2e_agenttool",
		Agent:          parentAgent.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-e2e-at", "What is 6*7?")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	_ = sess

	// Expect: user → function call → tool response → final text → persisted events.
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	// Verify tool response contains "42" from the child agent.
	foundMathResult := false
	for _, ev := range events {
		if ev.Content != nil {
			for _, p := range ev.Content.Parts {
				if p.FunctionResponse != nil && p.FunctionResponse.Name == "math_agent" {
					if result, ok := p.FunctionResponse.Result["result"]; ok {
						foundMathResult = true
						if result != "42" {
							t.Errorf("math_agent result = %v, want '42'", result)
						}
					}
				}
			}
		}
	}
	if !foundMathResult {
		t.Error("expected function response from math_agent with result=42")
	}
}

// ---------------------------------------------------------------------------
// E2E: AgentTool with SkipSummarization through runner
// ---------------------------------------------------------------------------

func TestWorkflowE2E_AgentTool_SkipSummarization_ThroughRunner(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	childAgent, err := agent.New(agent.Config{
		Name:        "summarizer",
		Description: "Summarizes content",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			ev := event.NewEvent("sum-result", "summarizer", event.RoleModel)
			ev.Content = &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: "TL;DR: everything is fine"}},
			}
			return []*event.Event{ev}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	at := agenttool.New(childAgent, &agenttool.Config{SkipSummarization: true})
	ft, ok := at.(tool.FunctionTool)
	if !ok {
		t.Fatal("agenttool does not implement FunctionTool")
	}

	fakeModel := model.NewFakeModel("parent-model",
		model.FunctionCallResponse("Let me summarize.",
			event.FunctionCall{ID: "fc-sum", Name: "summarizer", Args: map[string]any{"request": "summarize this"}},
		),
		model.TextResponse("Summary complete."),
	)

	parentFlow := &flow.Flow{
		Model: fakeModel,
		Tools: map[string]tool.FunctionTool{
			"summarizer": ft,
		},
	}

	parentAgent, err := llmagent.New("skip-orb", "Parent with skip-summarization agent tool.", parentFlow)
	if err != nil {
		t.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:        "e2e_agenttool_skip",
		Agent:          parentAgent.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-e2e-skip", "Summarize this text")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Verify SkipSummarization was activated on tool response event.
	foundSkipSummarization := false
	for _, ev := range events {
		if ev.Actions.SkipSummarization {
			foundSkipSummarization = true
			break
		}
	}
	if !foundSkipSummarization {
		t.Error("expected SkipSummarization=true on tool result event")
	}
}

// ---------------------------------------------------------------------------
// E2E: Sequential workflow with shared state through runner
// ---------------------------------------------------------------------------

func TestWorkflowE2E_Sequential_StateSharing_ThroughRunner(t *testing.T) {
	sessionSvc := runner.NewInMemorySessionService()

	// Writer: uses a tool to set state.
	writerTool := tool.NewFunctionTool("set_config", "Set configuration",
		func(args map[string]any) (map[string]any, error) {
			key, _ := args["key"].(string)
			val, _ := args["val"].(string)
			return map[string]any{
				"state_delta": map[string]any{key: val},
			}, nil
		},
	)
	writer := newSubAgentWithTool(t, "writer",
		map[string]tool.FunctionTool{"set_config": writerTool},
		model.FunctionCallResponse("Setting config.",
			event.FunctionCall{ID: "fc1", Name: "set_config", Args: map[string]any{"key": "branch", "val": "main"}},
		),
		model.TextResponse("Config set"),
	)

	// Reader: reads the state set by the writer via the session.
	// The subCtx embeds context.InvocationContext which has Session() session.Session.
	var readVal any
	reader := newRawSubAgent(t, "reader", func(ctx agent.InvocationContext) ([]*event.Event, error) {
		// subCtx embeds context.InvocationContext; access Session() through type assertion.
		if sc, ok := ctx.(interface{ Session() session.Session }); ok {
			v, _ := sc.Session().State().Get("branch")
			readVal = v
		}
		return []*event.Event{{
			ID:      "read-result",
			Author:  "reader",
			Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: fmt.Sprintf("read: %v", readVal)}}},
		}}, nil
	})

	seq := NewSequentialAgent("write-read-seq", "write then read", []SubAgent{writer, reader})

	r, err := runner.New(runner.Config{
		AppName:        "e2e_seq_state",
		Agent:          seq,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-e2e-sstate", "Set branch to main")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	_ = events
	if readVal != "main" {
		t.Errorf("reader saw branch = %v, want 'main'", readVal)
	}
}
