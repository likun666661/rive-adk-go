package flow

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/session"
	"github.com/likun666661/rive-adk-go/tool"
)

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

type testAgent struct{ name, desc string }

func (a *testAgent) Name() string                        { return a.name }
func (a *testAgent) Description() string                 { return a.desc }
func (a *testAgent) SubAgents() []agent.Agent            { return nil }
func (a *testAgent) FindAgent(name string) agent.Agent {
	if name == a.name {
		return a
	}
	return nil
}
func (a *testAgent) Parent() agent.Agent               { return nil }
func (a *testAgent) DisallowTransferToParent() bool     { return false }
func (a *testAgent) DisallowTransferToPeers() bool      { return false }

func newTestCtx(name string) context.InvocationContext {
	a := &testAgent{name: name, desc: "test agent"}
	s := session.NewInMemorySession("sid-1", "app", "user1")
	return context.NewInvocationContext(context.Params{
		Agent:        a,
		Session:      s,
		InvocationID: "inv-1",
		Branch:       "test_agent",
		UserContent:  "hello",
	})
}

// ---------------------------------------------------------------------------
// Test 1: final model response (text‑only, single step)
// ---------------------------------------------------------------------------

func TestFlowFinalModelResponse(t *testing.T) {
	ctx := newTestCtx("test_agent")

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.TextResponse("The weather is sunny with 22°C."),
		),
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if !ev.IsFinalResponse() {
		t.Error("text-only event should be final")
	}
	if ev.Content == nil || len(ev.Content.Parts) == 0 {
		t.Fatal("expected content with parts")
	}
	if ev.Content.Parts[0].Text != "The weather is sunny with 22°C." {
		t.Errorf("text = %q", ev.Content.Parts[0].Text)
	}
	if ev.Author != "test_agent" {
		t.Errorf("author = %q, want 'test_agent'", ev.Author)
	}
}

// ---------------------------------------------------------------------------
// Test 2: one tool call followed by final response
// ---------------------------------------------------------------------------

func TestFlowOneToolCallThenFinalResponse(t *testing.T) {
	ctx := newTestCtx("weather_agent")

	weatherTool := tool.NewFunctionTool("get_weather", "Get weather for a city",
		func(args map[string]any) (map[string]any, error) {
			city, _ := args["city"].(string)
			return map[string]any{
				"city":        city,
				"temperature": 22,
				"condition":   "sunny",
			}, nil
		},
	)

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			// Step 1: model returns a function call
			model.FunctionCallResponse("Let me check the weather.",
				event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
			),
			// Step 2: model returns final text response
			model.TextResponse("The weather in Tokyo is 22°C and sunny."),
		),
		Tools: map[string]tool.FunctionTool{
			"get_weather": weatherTool,
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// Expect: [model event with fc, tool result event, final model event]
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Event 1: model response with function call
	ev1 := events[0]
	if !ev1.HasFunctionCalls() {
		t.Error("event 1 should have function calls")
	}
	if ev1.IsFinalResponse() {
		t.Error("event 1 (with function call) should NOT be final")
	}

	// Event 2: tool result
	ev2 := events[1]
	if ev2.Role != event.RoleTool {
		t.Errorf("event 2 role = %q, want 'tool'", ev2.Role)
	}
	if ev2.Content == nil || len(ev2.Content.Parts) == 0 {
		t.Fatal("tool event should have content")
	}
	fr := ev2.Content.Parts[0].FunctionResponse
	if fr == nil {
		t.Fatal("tool event should have function response")
	}
	if fr.Name != "get_weather" {
		t.Errorf("tool name = %q, want 'get_weather'", fr.Name)
	}
	if fr.Error != "" {
		t.Errorf("tool error = %q, want empty", fr.Error)
	}
	temp, _ := fr.Result["temperature"].(int)
	if temp != 22 {
		t.Errorf("temperature = %d, want 22", temp)
	}

	// Event 3: final model response
	ev3 := events[2]
	if !ev3.IsFinalResponse() {
		t.Error("event 3 should be final")
	}
	if ev3.Content.Parts[0].Text != "The weather in Tokyo is 22°C and sunny." {
		t.Errorf("final text = %q", ev3.Content.Parts[0].Text)
	}
}

// ---------------------------------------------------------------------------
// Test 3: multiple tool calls executing in parallel with deterministic merge
// ---------------------------------------------------------------------------

func TestFlowMultipleToolCallsDeterministic(t *testing.T) {
	ctx := newTestCtx("multi_agent")

	// Use a mutex to track execution order — tools should execute.
	var mu sync.Mutex
	var callOrder []string

	weatherTool := tool.NewFunctionTool("get_weather", "Get weather",
		func(args map[string]any) (map[string]any, error) {
			mu.Lock()
			callOrder = append(callOrder, "get_weather")
			mu.Unlock()
			return map[string]any{"condition": "sunny"}, nil
		},
	)
	searchTool := tool.NewFunctionTool("search", "Search web",
		func(args map[string]any) (map[string]any, error) {
			mu.Lock()
			callOrder = append(callOrder, "search")
			mu.Unlock()
			return map[string]any{"results": []string{"link1", "link2"}}, nil
		},
	)

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.FunctionCallResponse("Let me look into that.",
				event.FunctionCall{ID: "fc-w", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
				event.FunctionCall{ID: "fc-s", Name: "search", Args: map[string]any{"q": "weather Tokyo"}},
			),
			model.TextResponse("Both tools completed successfully."),
		),
		Tools: map[string]tool.FunctionTool{
			"get_weather": weatherTool,
			"search":      searchTool,
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(callOrder) != 2 {
		t.Errorf("expected 2 tool calls, got %d: %v", len(callOrder), callOrder)
	}

	// Expect: [model event with 2 fcs, tool result event, final model event]
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// The tool result event should have 2 function responses.
	ev2 := events[1]
	if ev2.Role != event.RoleTool {
		t.Errorf("event 2 role = %q, want 'tool'", ev2.Role)
	}
	if ev2.Content == nil {
		t.Fatal("tool event should have content")
	}
	if len(ev2.Content.Parts) != 2 {
		t.Fatalf("expected 2 function response parts, got %d", len(ev2.Content.Parts))
	}

	// Collect names from responses
	names := make(map[string]bool)
	for _, p := range ev2.Content.Parts {
		if p.FunctionResponse != nil {
			names[p.FunctionResponse.Name] = true
		}
	}
	if !names["get_weather"] || !names["search"] {
		t.Errorf("missing expected tool names in responses: %v", names)
	}
}

// ---------------------------------------------------------------------------
// Test 4: state delta merge from tool results
// ---------------------------------------------------------------------------

func TestFlowStateDeltaMerge(t *testing.T) {
	ctx := newTestCtx("state_agent")

	stateTool := tool.NewFunctionTool("update_state", "Update session state",
		func(args map[string]any) (map[string]any, error) {
			key, _ := args["key"].(string)
			return map[string]any{
				"status":      "ok",
				"state_delta": map[string]any{key: args["value"]},
			}, nil
		},
	)

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.FunctionCallResponse("Updating state...",
				event.FunctionCall{ID: "fc1", Name: "update_state", Args: map[string]any{"key": "weather.last", "value": "sunny"}},
			),
			model.TextResponse("State updated."),
		),
		Tools: map[string]tool.FunctionTool{
			"update_state": stateTool,
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Verify session state was updated.
	v, ok := ctx.Session().State().Get("weather.last")
	if !ok {
		t.Error("expected 'weather.last' in session state")
	}
	if v != "sunny" {
		t.Errorf("weather.last = %v, want 'sunny'", v)
	}
}

// ---------------------------------------------------------------------------
// Test 5: processor / callback ordering
// ---------------------------------------------------------------------------

func TestFlowProcessorCallbackOrdering(t *testing.T) {
	ctx := newTestCtx("order_agent")

	var steps []string

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.TextResponse("All done."),
		),
		RequestProcessors: []RequestProcessor{
			func(ctx context.InvocationContext, req *model.LLMRequest) (*event.Event, error) {
				steps = append(steps, "reqProcessor1")
				return nil, nil
			},
			func(ctx context.InvocationContext, req *model.LLMRequest) (*event.Event, error) {
				steps = append(steps, "reqProcessor2")
				return nil, nil
			},
		},
		BeforeModelCallbacks: []BeforeModelCallback{
			func(ctx context.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				steps = append(steps, "beforeModel")
				return nil, nil
			},
		},
		AfterModelCallbacks: []AfterModelCallback{
			func(ctx context.InvocationContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
				steps = append(steps, "afterModel")
				return nil, nil
			},
		},
		ResponseProcessors: []ResponseProcessor{
			func(ctx context.InvocationContext, req *model.LLMRequest, resp *model.LLMResponse) error {
				steps = append(steps, "respProcessor")
				return nil
			},
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Expected ordering: reqProcessor1 → reqProcessor2 → beforeModel → afterModel → respProcessor
	expected := []string{
		"reqProcessor1", "reqProcessor2",
		"beforeModel", "afterModel",
		"respProcessor",
	}
	if len(steps) != len(expected) {
		t.Fatalf("expected %d steps, got %d: %v", len(expected), len(steps), steps)
	}
	for i, want := range expected {
		if steps[i] != want {
			t.Errorf("step[%d] = %q, want %q", i, steps[i], want)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 6: tool error becomes an event/error result (not silent success)
// ---------------------------------------------------------------------------

func TestFlowToolErrorBecomesEvent(t *testing.T) {
	ctx := newTestCtx("error_agent")

	failingTool := tool.NewFunctionTool("unreliable", "Always fails",
		func(args map[string]any) (map[string]any, error) {
			return nil, errors.New("database connection refused")
		},
	)

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.FunctionCallResponse("I will try to query.",
				event.FunctionCall{ID: "fc1", Name: "unreliable", Args: map[string]any{"query": "SELECT 1"}},
			),
			model.TextResponse("Sorry, the operation failed."),
		),
		Tools: map[string]tool.FunctionTool{
			"unreliable": failingTool,
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Event 2: tool result with error
	ev2 := events[1]
	if ev2.Role != event.RoleTool {
		t.Errorf("event 2 role = %q, want 'tool'", ev2.Role)
	}
	if ev2.ErrorMessage == "" {
		t.Error("tool error event should have ErrorMessage set")
	}
	fr := ev2.Content.Parts[0].FunctionResponse
	if fr == nil {
		t.Fatal("expected function response")
	}
	if fr.Error == "" {
		t.Error("function response should have non-empty Error field")
	}
	if fr.Result == nil {
		t.Fatal("function response should have Result map")
	}
	if errStr, ok := fr.Result["error"].(string); !ok || errStr != "database connection refused" {
		t.Errorf("result[error] = %v", fr.Result["error"])
	}
}

// ---------------------------------------------------------------------------
// Test 7: request processor short-circuit
// ---------------------------------------------------------------------------

func TestFlowRequestProcessorShortCircuit(t *testing.T) {
	ctx := newTestCtx("short_agent")

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.TextResponse("This should never be called."),
		),
		RequestProcessors: []RequestProcessor{
			func(ctx context.InvocationContext, req *model.LLMRequest) (*event.Event, error) {
				return &event.Event{
					ID:      "early",
					Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "early exit"}}},
				}, nil
			},
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (early exit), got %d", len(events))
	}
	if events[0].ID != "early" {
		t.Errorf("event ID = %q, want 'early'", events[0].ID)
	}
}

// ---------------------------------------------------------------------------
// Test 8: before model callback short-circuit
// ---------------------------------------------------------------------------

func TestFlowBeforeModelCallbackShortCircuit(t *testing.T) {
	ctx := newTestCtx("short_model_agent")

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.TextResponse("This should never be called."),
		),
		BeforeModelCallbacks: []BeforeModelCallback{
			func(ctx context.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				return model.TextResponse("From before callback."), nil
			},
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Content.Parts[0].Text != "From before callback." {
		t.Errorf("text = %q", events[0].Content.Parts[0].Text)
	}
}

// ---------------------------------------------------------------------------
// Test 9: before tool callback overrides tool result
// ---------------------------------------------------------------------------

func TestFlowBeforeToolCallbackOverride(t *testing.T) {
	ctx := newTestCtx("override_agent")

	weatherTool := tool.NewFunctionTool("get_weather", "Get weather",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"real": true}, nil
		},
	)

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.FunctionCallResponse("Checking weather...",
				event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Paris"}},
			),
			model.TextResponse("Weather checked."),
		),
		Tools: map[string]tool.FunctionTool{
			"get_weather": weatherTool,
		},
		BeforeToolCallbacks: []BeforeToolCallback{
			func(ctx context.InvocationContext, toolName string, args map[string]any) (map[string]any, error) {
				return map[string]any{"cached": true, "temperature": 18}, nil
			},
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// Tool event should have cached result.
	ev2 := events[1]
	fr := ev2.Content.Parts[0].FunctionResponse
	cached, _ := fr.Result["cached"].(bool)
	if !cached {
		t.Error("expected cached result from before tool callback override")
	}
}

// ---------------------------------------------------------------------------
// Test 10: after tool callback transforms result
// ---------------------------------------------------------------------------

func TestFlowAfterToolCallbackTransform(t *testing.T) {
	ctx := newTestCtx("transform_agent")

	weatherTool := tool.NewFunctionTool("get_weather", "Get weather",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"temp_c": 22}, nil
		},
	)

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.FunctionCallResponse("Checking weather...",
				event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Berlin"}},
			),
			model.TextResponse("Done."),
		),
		Tools: map[string]tool.FunctionTool{
			"get_weather": weatherTool,
		},
		AfterToolCallbacks: []AfterToolCallback{
			func(ctx context.InvocationContext, toolName string, args, result map[string]any, runErr error) (map[string]any, error) {
				tempC, ok := result["temp_c"].(int)
				if !ok {
					return result, nil
				}
				return map[string]any{
					"temp_c": tempC,
					"temp_f": tempC*9/5 + 32,
				}, nil
			},
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	ev2 := events[1]
	fr := ev2.Content.Parts[0].FunctionResponse
	tempF, ok := fr.Result["temp_f"].(int)
	if !ok || tempF != 71 {
		t.Errorf("temp_f = %v, want 71", fr.Result["temp_f"])
	}
}

// ---------------------------------------------------------------------------
// Test 11: no model configured returns error
// ---------------------------------------------------------------------------

func TestFlowNoModelReturnsError(t *testing.T) {
	ctx := newTestCtx("no_model_agent")
	f := &Flow{}
	_, err := f.Run(ctx)
	if err == nil {
		t.Error("expected error for missing model")
	}
}

// ---------------------------------------------------------------------------
// Test 12: multiple steps with tool loop then final
// ---------------------------------------------------------------------------

func TestFlowMultiToolMultiStep(t *testing.T) {
	ctx := newTestCtx("multi_step_agent")

	queryTool := tool.NewFunctionTool("query_db", "Query a database",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"rows": 42}, nil
		},
	)
	formatTool := tool.NewFunctionTool("format_result", "Format a result",
		func(args map[string]any) (map[string]any, error) {
			val, _ := args["value"]
			return map[string]any{"formatted": fmt.Sprintf("Result: %v", val)}, nil
		},
	)

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			// Step 1: call query_db
			model.FunctionCallResponse("Let me query.",
				event.FunctionCall{ID: "fc1", Name: "query_db", Args: map[string]any{"sql": "SELECT count(*)"}},
			),
			// Step 2: after getting query result, call format_result
			model.FunctionCallResponse("Now let me format.",
				event.FunctionCall{ID: "fc2", Name: "format_result", Args: map[string]any{"value": 42}},
			),
			// Step 3: final text
			model.TextResponse("The query returned: Result: 42"),
		),
		Tools: map[string]tool.FunctionTool{
			"query_db":      queryTool,
			"format_result": formatTool,
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// Expect: model(fc query) → tool(query result) → model(fc format) → tool(format result) → model(final)
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Verify final event is final
	lastEv := events[len(events)-1]
	if !lastEv.IsFinalResponse() {
		t.Error("last event should be final")
	}

	// Count tool events
	toolCount := 0
	for _, ev := range events {
		if ev.Role == event.RoleTool {
			toolCount++
		}
	}
	if toolCount != 2 {
		t.Errorf("expected 2 tool events, got %d", toolCount)
	}

	seenIDs := make(map[string]bool)
	for _, ev := range events {
		if seenIDs[ev.ID] {
			t.Fatalf("duplicate event ID %q in multi-step flow", ev.ID)
		}
		seenIDs[ev.ID] = true
	}
}

// ---------------------------------------------------------------------------
// Test 13: tool not found creates error result
// ---------------------------------------------------------------------------

func TestFlowToolNotFound(t *testing.T) {
	ctx := newTestCtx("missing_tool_agent")

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.FunctionCallResponse("Calling unknown tool.",
				event.FunctionCall{ID: "fc1", Name: "nonexistent", Args: map[string]any{}},
			),
			model.TextResponse("Fallback response."),
		),
		Tools: map[string]tool.FunctionTool{},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Tool result event should carry error
	ev2 := events[1]
	if ev2.Role != event.RoleTool {
		t.Errorf("event 2 role = %q, want 'tool'", ev2.Role)
	}
	if ev2.ErrorMessage == "" {
		t.Error("tool-not-found should populate ErrorMessage")
	}
	fr := ev2.Content.Parts[0].FunctionResponse
	if fr.Error == "" {
		t.Error("function response should have error for missing tool")
	}
}

// ---------------------------------------------------------------------------
// Test 14: empty response with no content and no error is skipped
// ---------------------------------------------------------------------------

func TestFlowEmptyResponseSkipped(t *testing.T) {
	ctx := newTestCtx("empty_agent")

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			// First response: empty (no content, no error code) — should skip
			&model.LLMResponse{},
			// Second response: final text
			model.TextResponse("After empty skip."),
		),
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (empty skipped), got %d", len(events))
	}
	if events[0].Content.Parts[0].Text != "After empty skip." {
		t.Errorf("text = %q", events[0].Content.Parts[0].Text)
	}
}

// ---------------------------------------------------------------------------
// Test 15: nil model response fails cleanly
// ---------------------------------------------------------------------------

type nilResponseModel struct{}

func (nilResponseModel) Name() string { return "nil-model" }

func (nilResponseModel) GenerateContent(req *model.LLMRequest) (*model.LLMResponse, error) {
	return nil, nil
}

func TestFlowNilModelResponseReturnsError(t *testing.T) {
	ctx := newTestCtx("nil_agent")
	f := &Flow{Model: nilResponseModel{}}

	_, err := f.Run(ctx)
	if err == nil {
		t.Fatal("expected error for nil model response")
	}
	if got := err.Error(); got != `flow: model "nil-model" returned nil response` {
		t.Fatalf("error = %q", got)
	}
}

// =============================================================================
// Chapter 03 — Tool system integration tests
// =============================================================================

// ---------------------------------------------------------------------------
// Test 16: Flow with Toolsets — tool resolution
// ---------------------------------------------------------------------------

func TestFlowToolsetResolution(t *testing.T) {
	ctx := newTestCtx("toolset_agent")

	tsTool := tool.NewFunctionTool("ts_search", "Search from toolset",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"source": "toolset", "q": args["q"]}, nil
		},
	)
	ts := tool.NewStaticToolset("search_set", []tool.Tool{tsTool.(tool.Tool)})

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.FunctionCallResponse("Looking up from toolset.",
				event.FunctionCall{ID: "fc1", Name: "ts_search", Args: map[string]any{"q": "test"}},
			),
			model.TextResponse("Search complete."),
		),
		Tools:    map[string]tool.FunctionTool{},
		Toolsets: []tool.Toolset{ts},
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
	if fr == nil || fr.Name != "ts_search" {
		t.Errorf("function response = %v", fr)
	}
	if src, _ := fr.Result["source"].(string); src != "toolset" {
		t.Errorf("source = %q, want 'toolset'", src)
	}
}

// ---------------------------------------------------------------------------
// Test 17: Flow with Toolsets — declaration injection
// ---------------------------------------------------------------------------

func TestFlowToolsetDeclarationInjection(t *testing.T) {
	ctx := newTestCtx("decl_inject_agent")

	decl := tool.NewDeclaration("api_call", "Make an API call",
		map[string]any{"type": "object", "properties": map[string]any{"endpoint": map[string]any{"type": "string"}}},
		nil,
	)
	tsTool := tool.NewFunctionToolWithDeclaration("api_call", "Make an API call", decl,
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"status": "ok"}, nil
		},
	)
	ts := tool.NewStaticToolset("api_set", []tool.Tool{tool.FunctionToolAsTool(tsTool)})

	// Use a BeforeModelCallback to inspect the request's tool declarations.
	var capturedDecls []any
	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.FunctionCallResponse("Calling API.",
				event.FunctionCall{ID: "fc1", Name: "api_call", Args: map[string]any{"endpoint": "/test"}},
			),
			model.TextResponse("API call complete."),
		),
		Tools:    map[string]tool.FunctionTool{},
		Toolsets: []tool.Toolset{ts},
		BeforeModelCallbacks: []BeforeModelCallback{
			func(ctx context.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				capturedDecls = req.ToolDeclarations
				return nil, nil
			},
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Verify declarations were injected.
	if len(capturedDecls) != 1 {
		t.Fatalf("expected 1 tool declaration in request, got %d", len(capturedDecls))
	}
	d, ok := capturedDecls[0].(tool.Declaration)
	if !ok {
		t.Fatalf("expected Declaration type, got %T", capturedDecls[0])
	}
	if d.Name != "api_call" {
		t.Errorf("declaration name = %q, want 'api_call'", d.Name)
	}
}

// ---------------------------------------------------------------------------
// Test 18: Flow with FilterToolset
// ---------------------------------------------------------------------------

func TestFlowFilteredToolset(t *testing.T) {
	ctx := newTestCtx("filter_agent")

	declA := tool.NewDeclaration("allowed_tool", "Allowed tool", nil, nil)
	declB := tool.NewDeclaration("blocked_tool", "Blocked tool", nil, nil)

	allowedTool := tool.NewFunctionToolWithDeclaration("allowed_tool", "Allowed", declA,
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"result": "allowed"}, nil
		},
	)
	blockedTool := tool.NewFunctionToolWithDeclaration("blocked_tool", "Blocked", declB,
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"result": "should_not_reach"}, nil
		},
	)

	fullTs := tool.NewStaticToolset("full", []tool.Tool{
		tool.FunctionToolAsTool(allowedTool),
		tool.FunctionToolAsTool(blockedTool),
	})
	filteredTs := tool.NewFilterToolset("filtered", fullTs, tool.AllowedToolsPredicate("allowed_tool"))

	var capturedDecls []any
	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.FunctionCallResponse("Calling allowed tool.",
				event.FunctionCall{ID: "fc1", Name: "allowed_tool", Args: map[string]any{}},
			),
			model.TextResponse("Done."),
		),
		Tools:    map[string]tool.FunctionTool{},
		Toolsets: []tool.Toolset{filteredTs},
		BeforeModelCallbacks: []BeforeModelCallback{
			func(ctx context.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				capturedDecls = req.ToolDeclarations
				return nil, nil
			},
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Only allowed_tool should be in declarations.
	if len(capturedDecls) != 1 {
		t.Fatalf("expected 1 filtered declaration, got %d", len(capturedDecls))
	}
	d := capturedDecls[0].(tool.Declaration)
	if d.Name != "allowed_tool" {
		t.Errorf("declaration name = %q, want 'allowed_tool'", d.Name)
	}

	// Tool should execute.
	ev2 := events[1]
	fr := ev2.Content.Parts[0].FunctionResponse
	if fr.Result["result"] != "allowed" {
		t.Errorf("result = %v, want 'allowed'", fr.Result["result"])
	}
}

// ---------------------------------------------------------------------------
// Test 19: Flow with streaming tool in non-live mode
// ---------------------------------------------------------------------------

func TestFlowStreamingToolNonLiveMode(t *testing.T) {
	ctx := newTestCtx("stream_agent")

	streamTool := tool.NewStreamingFunctionToolWithDeclaration("stream_data", "Stream data",
		tool.NewDeclaration("stream_data", "Stream data", nil, nil),
		func(args map[string]any) ([]tool.StreamChunk, error) {
			return []tool.StreamChunk{
				{Text: "Hello ", Final: false},
				{Text: "World", Final: true},
			}, nil
		},
	)
	ts := tool.NewStaticToolset("stream_set", []tool.Tool{streamTool})

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.FunctionCallResponse("Streaming data...",
				event.FunctionCall{ID: "fc1", Name: "stream_data", Args: map[string]any{}},
			),
			model.TextResponse("Stream complete."),
		),
		Tools:    map[string]tool.FunctionTool{},
		Toolsets: []tool.Toolset{ts},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	ev2 := events[1]
	fr := ev2.Content.Parts[0].FunctionResponse
	if fr.Result["result"] != "Hello World" {
		t.Errorf("result = %v, want Hello World", fr.Result["result"])
	}
}

// ---------------------------------------------------------------------------
// Test 20: Flow with long-running tool
// ---------------------------------------------------------------------------

func TestFlowLongRunningTool(t *testing.T) {
	ctx := newTestCtx("longrun_agent")

	decl := tool.NewDeclaration("batch_job", "A long batch job",
		map[string]any{"type": "object"},
		map[string]any{"type": "object"},
	)
	lrTool := tool.NewLongRunningFunctionTool("batch_job", "A long batch job", decl,
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"job_id": "job-123", "status": "pending"}, nil
		},
	)

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.FunctionCallResponse("Starting batch job.",
				event.FunctionCall{ID: "fc1", Name: "batch_job", Args: map[string]any{"input": "data"}},
			),
			model.TextResponse("Job started with ID job-123."),
		),
		Tools: map[string]tool.FunctionTool{
			"batch_job": lrTool,
		},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Verify tool is marked as long-running.
	if !lrTool.IsLongRunning() {
		t.Error("tool should be long-running")
	}

	// Verify the long-running annotation is in the declaration.
	dp := lrTool.(tool.DeclarationProvider)
	d := dp.Declaration()
	if d.Description == "" || len(d.Description) == 0 {
		t.Error("long-running declaration description should be non-empty")
	}

	// Verify result contains job metadata.
	ev2 := events[1]
	fr := ev2.Content.Parts[0].FunctionResponse
	if fr.Result["job_id"] != "job-123" {
		t.Errorf("job_id = %v, want 'job-123'", fr.Result["job_id"])
	}
	if fr.Result["status"] != "pending" {
		t.Errorf("status = %v, want 'pending'", fr.Result["status"])
	}
}

// ---------------------------------------------------------------------------
// Test 21: Flow resolution caches toolsets (resolved once)
// ---------------------------------------------------------------------------

func TestFlowToolsetResolutionCache(t *testing.T) {
	ctx := newTestCtx("cache_agent")

	callCount := 0
	tsTool := tool.NewFunctionTool("cached_tool", "Cached tool",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		},
	)

	dynamicTs := &countingToolset{
		name:  "dynamic",
		tools: []tool.Tool{tool.FunctionToolAsTool(tsTool)},
		calls: &callCount,
	}

	f := &Flow{
		Model: model.NewFakeModel("fake-model",
			model.FunctionCallResponse("Calling cached.",
				event.FunctionCall{ID: "fc1", Name: "cached_tool", Args: map[string]any{}},
			),
			model.FunctionCallResponse("Calling cached again.",
				event.FunctionCall{ID: "fc2", Name: "cached_tool", Args: map[string]any{}},
			),
			model.TextResponse("Done."),
		),
		Tools:    map[string]tool.FunctionTool{},
		Toolsets: []tool.Toolset{dynamicTs},
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Tools() should have been called only once (cached).
	if callCount != 1 {
		t.Errorf("Tools() call count = %d, want 1 (cached)", callCount)
	}
}

type countingToolset struct {
	name  string
	tools []tool.Tool
	calls *int
}

func (c *countingToolset) Name() string { return c.name }
func (c *countingToolset) Tools() ([]tool.Tool, error) {
	*c.calls++
	return c.tools, nil
}

// =============================================================================
// Chapter 07 — Agent transfer tests
// =============================================================================

// newTransferTestCtx creates a context with a parent agent that has sub-agents
// and a properly configured root agent.
func newTransferTestCtx(parent agent.Agent) context.InvocationContext {
	s := session.NewInMemorySession("sid-tf", "app", "user1")
	return context.NewInvocationContext(context.Params{
		Agent:        parent,
		RootAgent:    parent,
		Session:      s,
		InvocationID: "inv-tf",
		Branch:       parent.Name(),
		UserContent:  "hello",
	})
}

func newTransferTargetAgent(name, description, responseText string) agent.Agent {
	a, err := agent.New(agent.Config{
		Name:        name,
		Description: description,
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return []*event.Event{
				{
					ID:      fmt.Sprintf("%s-ev", name),
					Author:  name,
					Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: responseText}}},
					Branch:  name,
				},
			}, nil
		},
	})
	if err != nil {
		panic(err)
	}
	return a
}

// ---------------------------------------------------------------------------
// Test 22: model-triggered transfer delegates execution to target agent
// ---------------------------------------------------------------------------

func TestFlowTransferToSubAgent(t *testing.T) {
	targetAgent := newTransferTargetAgent("math_bot", "Solves math problems", "The answer is 42.")
	parentAgent, err := agent.New(agent.Config{
		Name:        "root",
		Description: "Root agent",
		SubAgents:   []agent.Agent{targetAgent},
		Run:         func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
		Parent:      nil,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := newTransferTestCtx(parentAgent)

	f := &Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Transferring to math_bot.",
				event.FunctionCall{ID: "fc1", Name: "transfer_to_agent", Args: map[string]any{"agent_name": "math_bot"}},
			),
		),
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should have: model event (transfer fc), tool result, target agent events
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events (model + tool + target), got %d", len(events))
	}

	// Event 1: model response with transfer_to_agent function call
	ev1 := events[0]
	if ev1.Role != event.RoleModel {
		t.Errorf("event 1 role = %q, want 'model'", ev1.Role)
	}
	if !ev1.HasFunctionCalls() {
		t.Error("event 1 should have function calls (transfer_to_agent)")
	}

	// Event 2: tool result with TransferToAgent action
	ev2 := events[1]
	if ev2.Role != event.RoleTool {
		t.Errorf("event 2 role = %q, want 'tool'", ev2.Role)
	}
	if ev2.Actions.TransferToAgent != "math_bot" {
		t.Errorf("TransferToAgent = %q, want 'math_bot'", ev2.Actions.TransferToAgent)
	}

	// Last event should be from the target agent
	lastEv := events[len(events)-1]
	if lastEv.Author != "math_bot" {
		t.Errorf("last event Author = %q, want 'math_bot'", lastEv.Author)
	}
	if lastEv.Content == nil || len(lastEv.Content.Parts) == 0 {
		t.Fatal("target event should have content")
	}
	if lastEv.Content.Parts[0].Text != "The answer is 42." {
		t.Errorf("target text = %q, want 'The answer is 42.'", lastEv.Content.Parts[0].Text)
	}
}

// ---------------------------------------------------------------------------
// Test 23: invalid transfer target yields structured tool error
// ---------------------------------------------------------------------------

func TestFlowTransferInvalidTarget(t *testing.T) {
	targetAgent := newTransferTargetAgent("math_bot", "Solves math", "42")
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

	f := &Flow{
		Model: model.NewFakeModel("fake",
			// The transfer tool IS injected (because there are sub-agents),
			// but the target name is invalid.
			model.FunctionCallResponse("Transferring to nonexistent.",
				event.FunctionCall{ID: "fc1", Name: "transfer_to_agent", Args: map[string]any{"agent_name": "nonexistent"}},
			),
			model.TextResponse("Fallback: I could not transfer."),
		),
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should get: model event with fc, tool error, model final
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	// The tool event should have error about invalid target
	toolEv := events[1]
	if toolEv.ErrorMessage == "" {
		t.Error("tool error event should have ErrorMessage")
	}
	if !strings.Contains(toolEv.ErrorMessage, "nonexistent") {
		t.Errorf("error message = %q, should mention 'nonexistent'", toolEv.ErrorMessage)
	}
	if toolEv.Content != nil && len(toolEv.Content.Parts) > 0 {
		fr := toolEv.Content.Parts[0].FunctionResponse
		if fr == nil || fr.Name != "transfer_to_agent" {
			t.Errorf("expected transfer_to_agent function response, got %v", fr)
		}
		if fr.Error == "" || !strings.Contains(fr.Error, "nonexistent") {
			t.Errorf("function response error = %q, should mention 'nonexistent'", fr.Error)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 24: transfer loop detection (max depth guard)
// ---------------------------------------------------------------------------

func TestFlowTransferLoopDetection(t *testing.T) {
	// Create two agents that recursively transfer to each other
	// root <-> child
	transferEv := func(targetName string) *event.Event {
		return &event.Event{
			ID:      fmt.Sprintf("fc-%s", targetName),
			Author:  targetName,
			Actions: event.EventActions{TransferToAgent: targetName},
		}
	}

	child, err := agent.New(agent.Config{
		Name:        "child",
		Description: "Child agent",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return []*event.Event{transferEv("root")}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	rootAgent, err := agent.New(agent.Config{
		Name:        "root",
		Description: "Root agent",
		SubAgents:   []agent.Agent{child},
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return []*event.Event{transferEv("child")}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := newTransferTestCtx(rootAgent)

	f := &Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Transferring to child.",
				event.FunctionCall{ID: "fc1", Name: "transfer_to_agent", Args: map[string]any{"agent_name": "child"}},
			),
		),
	}

	_, err = f.Run(ctx)
	if err == nil {
		t.Fatal("expected transfer loop error, got nil")
	}
	if !strings.Contains(err.Error(), "transfer loop detected") {
		t.Errorf("error = %q, should contain 'transfer loop detected'", err.Error())
	}
	if !strings.Contains(err.Error(), "max depth") {
		t.Errorf("error = %q, should contain 'max depth'", err.Error())
	}
}

// TestFlowTransferToParent tests transfer from child to parent.
func TestFlowTransferToParent(t *testing.T) {
	parentAgent, err := agent.New(agent.Config{
		Name:        "parent",
		Description: "Parent agent",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return []*event.Event{
				{
					ID:      "parent-ev",
					Author:  "parent",
					Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "Parent handled it."}}},
					Branch:  "parent",
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	childAgent, err := agent.New(agent.Config{
		Name:        "child",
		Description: "Child agent",
		Parent:      parentAgent,
		Run:         func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	// Set up parent with child as sub-agent so FindAgent can find it.
	// Recreate parent with child as SubAgent
	parentWithChild, err := agent.New(agent.Config{
		Name:        "parent",
		Description: "Parent agent",
		SubAgents:   []agent.Agent{childAgent},
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return []*event.Event{
				{
					ID:      "parent-ev",
					Author:  "parent",
					Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "Parent handled it."}}},
					Branch:  "parent",
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Use parent as root and child as current agent
	ctx := newTransferTestCtx(parentWithChild)
	// Override context to have child as current agent but parent as root
	s := session.NewInMemorySession("sid-tf2", "app", "user1")
	ctx = context.NewInvocationContext(context.Params{
		Agent:        childAgent,
		RootAgent:    parentWithChild,
		Session:      s,
		InvocationID: "inv-tf2",
		Branch:       "child",
		UserContent:  "hello",
	})

	f := &Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Transferring to parent.",
				event.FunctionCall{ID: "fc1", Name: "transfer_to_agent", Args: map[string]any{"agent_name": "parent"}},
			),
		),
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	// Transfer should set the action
	ev2 := events[1]
	if ev2.Actions.TransferToAgent != "parent" {
		t.Errorf("TransferToAgent = %q, want 'parent'", ev2.Actions.TransferToAgent)
	}

	// Last event should be from parent
	lastEv := events[len(events)-1]
	if lastEv.Author != "parent" {
		t.Errorf("last event Author = %q, want 'parent'", lastEv.Author)
	}
}

func TestFlowTransferWithoutSubAgentsHasEmptyTargets(t *testing.T) {
	agentWithoutSubs, err := agent.New(agent.Config{
		Name:                     "loner",
		Description:              "Loner agent",
		DisallowTransferToParent: true,
		DisallowTransferToPeers:  true,
		Run:                      func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := newTransferTestCtx(agentWithoutSubs)

	// Model calls transfer_to_agent but agent has no transfer targets
	f := &Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Trying to transfer.",
				event.FunctionCall{ID: "fc1", Name: "transfer_to_agent", Args: map[string]any{"agent_name": "anyone"}},
			),
			model.TextResponse("Final response."),
		),
	}

	events, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Since there are no transfer targets, the transfer tool is NOT injected.
	// So transfer_to_agent should be treated as a regular tool not found error.
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events (model fc, tool error, model final), got %d", len(events))
	}

	// Event 2 should be a tool-not-found error
	ev2 := events[1]
	if ev2.Role != event.RoleTool {
		t.Errorf("event 2 role = %q, want 'tool'", ev2.Role)
	}
	if ev2.ErrorMessage == "" {
		t.Error("expected error message for tool not found")
	}
}
