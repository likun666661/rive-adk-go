package plugin

import (
	stdctx "context"
	"errors"
	"fmt"
	"testing"

	"github.com/likun666661/rive-adk-go/artifact"
	"github.com/likun666661/rive-adk-go/callbackctx"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/memory"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/session"
)

// stubReadonlyState satisfies session.ReadonlyState.
type stubReadonlyState struct{}

func (s *stubReadonlyState) Get(key string) (any, bool) { return nil, false }
func (s *stubReadonlyState) All() map[string]any         { return nil }

// dummyState satisfies session.State.
type dummyState struct{}

func (d *dummyState) Get(key string) (any, bool)  { return nil, false }
func (d *dummyState) Set(key string, val any)       {}
func (d *dummyState) Delete(key string)             {}
func (d *dummyState) All() map[string]any           { return nil }

// stubArtifactSvc satisfies artifact.Service.
type stubArtifactSvc struct{}

func (s *stubArtifactSvc) Save(ctx stdctx.Context, req *artifact.SaveRequest) (*artifact.SaveResponse, error) {
	return nil, nil
}
func (s *stubArtifactSvc) Load(ctx stdctx.Context, req *artifact.LoadRequest) (*artifact.LoadResponse, error) {
	return nil, nil
}
func (s *stubArtifactSvc) Delete(ctx stdctx.Context, req *artifact.DeleteRequest) error { return nil }
func (s *stubArtifactSvc) List(ctx stdctx.Context, req *artifact.ListRequest) (*artifact.ListResponse, error) {
	return nil, nil
}
func (s *stubArtifactSvc) Versions(ctx stdctx.Context, req *artifact.VersionsRequest) (*artifact.VersionsResponse, error) {
	return nil, nil
}
func (s *stubArtifactSvc) GetArtifactVersion(ctx stdctx.Context, req *artifact.GetArtifactVersionRequest) (*artifact.GetArtifactVersionResponse, error) {
	return nil, nil
}

// stubMemorySvc satisfies memory.Service.
type stubMemorySvc struct{}

func (s *stubMemorySvc) AddSessionToMemory(ctx stdctx.Context, sess session.Session) error { return nil }
func (s *stubMemorySvc) SearchMemory(ctx stdctx.Context, req *memory.SearchRequest) (*memory.SearchResponse, error) {
	return nil, nil
}

// minimalCallbackContext satisfies callbackctx.CallbackContext.
type minimalCallbackContext struct{}

func (m *minimalCallbackContext) UserContent() string                  { return "" }
func (m *minimalCallbackContext) InvocationID() string                 { return "" }
func (m *minimalCallbackContext) AgentName() string                    { return "test-agent" }
func (m *minimalCallbackContext) ReadonlyState() session.ReadonlyState { return &stubReadonlyState{} }
func (m *minimalCallbackContext) UserID() string                       { return "" }
func (m *minimalCallbackContext) AppName() string                      { return "" }
func (m *minimalCallbackContext) SessionID() string                    { return "" }
func (m *minimalCallbackContext) Branch() string                       { return "" }
func (m *minimalCallbackContext) ArtifactService() artifact.Service    { return &stubArtifactSvc{} }
func (m *minimalCallbackContext) MemoryService() memory.Service        { return &stubMemorySvc{} }
func (m *minimalCallbackContext) State() session.State                 { return &dummyState{} }

var _ callbackctx.CallbackContext = (*minimalCallbackContext)(nil)

// minimalToolContext satisfies callbackctx.ToolContext.
type minimalToolContext struct {
    fcID string
}

func (m *minimalToolContext) UserContent() string                  { return "" }
func (m *minimalToolContext) InvocationID() string                 { return "" }
func (m *minimalToolContext) AgentName() string                    { return "test-tool-agent" }
func (m *minimalToolContext) ReadonlyState() session.ReadonlyState { return &stubReadonlyState{} }
func (m *minimalToolContext) UserID() string                       { return "" }
func (m *minimalToolContext) AppName() string                      { return "" }
func (m *minimalToolContext) SessionID() string                    { return "" }
func (m *minimalToolContext) Branch() string                       { return "" }
func (m *minimalToolContext) ArtifactService() artifact.Service    { return &stubArtifactSvc{} }
func (m *minimalToolContext) MemoryService() memory.Service        { return &stubMemorySvc{} }
func (m *minimalToolContext) State() session.State                 { return &dummyState{} }
func (m *minimalToolContext) FunctionCallID() string               { return m.fcID }
func (m *minimalToolContext) Actions() *event.EventActions         { return nil }
func (m *minimalToolContext) SearchMemory(ctx stdctx.Context, query string) (*memory.SearchResponse, error) {
	return nil, nil
}

var _ callbackctx.ToolContext = (*minimalToolContext)(nil)

// ==========================================================================
// Test 1: registration order is preserved
// ==========================================================================

func TestManagerRegistrationOrder(t *testing.T) {
	m := NewManager()

	var order []string

	p1 := New(Config{
		Name:        "first",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			order = append(order, "first")
			return nil, nil
		},
	})
	p2 := New(Config{
		Name:        "second",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			order = append(order, "second")
			return nil, nil
		},
	})
	p3 := New(Config{
		Name:        "third",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			order = append(order, "third")
			return nil, nil
		},
	})

	m.Register(p1)
	m.Register(p2)
	m.Register(p3)

	if m.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", m.Len())
	}

	cctx := &minimalCallbackContext{}
	_, err := m.RunBeforeModelCallback(cctx, &model.LLMRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if len(order) != 3 {
		t.Fatalf("expected 3 hook invocations, got %d", len(order))
	}
	expected := []string{"first", "second", "third"}
	for i, want := range expected {
		if order[i] != want {
			t.Errorf("order[%d] = %q, want %q", i, order[i], want)
		}
	}
}

// ==========================================================================
// Test 2: nil hooks are skipped
// ==========================================================================

func TestManagerSkipsNilHooks(t *testing.T) {
	m := NewManager()

	var called []string

	m.Register(New(Config{
		Name:        "has_before_model",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			called = append(called, "has_before_model")
			return nil, nil
		},
	}))
	m.Register(New(Config{
		Name: "no_before_model",
	}))
	m.Register(New(Config{
		Name:        "also_has_before_model",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			called = append(called, "also_has_before_model")
			return nil, nil
		},
	}))

	cctx := &minimalCallbackContext{}
	_, err := m.RunBeforeModelCallback(cctx, &model.LLMRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if len(called) != 2 {
		t.Fatalf("expected 2 calls (nil skipped), got %d: %v", len(called), called)
	}
	if called[0] != "has_before_model" || called[1] != "also_has_before_model" {
		t.Errorf("order = %v, want [has_before_model also_has_before_model]", called)
	}
}

// ==========================================================================
// Test 3: early exit — first non-nil result stops the chain
// ==========================================================================

func TestManagerEarlyExitBeforeAgent(t *testing.T) {
	m := NewManager()

	var called []string

	m.Register(New(Config{
		Name:       "blocker",
		BeforeAgent: func(ctx callbackctx.CallbackContext) (*event.Event, error) {
			called = append(called, "blocker")
			return &event.Event{ID: "early_exit"}, nil
		},
	}))
	m.Register(New(Config{
		Name:       "should_not_run",
		BeforeAgent: func(ctx callbackctx.CallbackContext) (*event.Event, error) {
			called = append(called, "should_not_run")
			return nil, nil
		},
	}))

	cctx := &minimalCallbackContext{}
	ev, err := m.RunBeforeAgentCallback(cctx)
	if err != nil {
		t.Fatal(err)
	}
	if ev == nil {
		t.Fatal("expected non-nil event from early exit")
	}
	if ev.ID != "early_exit" {
		t.Errorf("event ID = %q, want 'early_exit'", ev.ID)
	}
	if len(called) != 1 {
		t.Fatalf("expected 1 call (early exit), got %d: %v", len(called), called)
	}
	if called[0] != "blocker" {
		t.Errorf("called[0] = %q, want 'blocker'", called[0])
	}
}

// ==========================================================================
// Test 4: early exit with BeforeModel callback
// ==========================================================================

func TestManagerEarlyExitBeforeModel(t *testing.T) {
	m := NewManager()

	overrideResp := model.TextResponse("From plugin.")

	m.Register(New(Config{
		Name:         "shortcut",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			return overrideResp, nil
		},
	}))
	m.Register(New(Config{
		Name:         "never_reached",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			return nil, errors.New("should not be called")
		},
	}))

	cctx := &minimalCallbackContext{}
	resp, err := m.RunBeforeModelCallback(cctx, &model.LLMRequest{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response from early exit")
	}
	if resp.Content == nil || len(resp.Content.Parts) == 0 {
		t.Fatal("expected content in response")
	}
	if resp.Content.Parts[0].Text != "From plugin." {
		t.Errorf("text = %q, want 'From plugin.'", resp.Content.Parts[0].Text)
	}
}

// ==========================================================================
// Test 5: error propagation — first error stops the chain
// ==========================================================================

func TestManagerErrorPropagation(t *testing.T) {
	m := NewManager()

	m.Register(New(Config{
		Name:       "failer",
		BeforeAgent: func(ctx callbackctx.CallbackContext) (*event.Event, error) {
			return nil, fmt.Errorf("boom")
		},
	}))
	m.Register(New(Config{
		Name:       "never_reached",
		BeforeAgent: func(ctx callbackctx.CallbackContext) (*event.Event, error) {
			return &event.Event{ID: "should_not_see_this"}, nil
		},
	}))

	cctx := &minimalCallbackContext{}
	ev, err := m.RunBeforeAgentCallback(cctx)
	if err == nil {
		t.Error("expected error from failing hook")
	}
	if ev != nil {
		t.Error("expected nil event with error")
	}
}

// ==========================================================================
// Test 6: BeforeTool early exit — first non-nil result stops chain
// ==========================================================================

func TestManagerEarlyExitBeforeTool(t *testing.T) {
	m := NewManager()

	m.Register(New(Config{
		Name:       "tool_blocker",
		BeforeTool: func(ctx callbackctx.ToolContext, toolName string, args map[string]any) (map[string]any, error) {
			return map[string]any{"cached": true}, nil
		},
	}))
	m.Register(New(Config{
		Name:       "tool_never_reached",
		BeforeTool: func(ctx callbackctx.ToolContext, toolName string, args map[string]any) (map[string]any, error) {
			return nil, errors.New("should not be called")
		},
	}))

	tctx := &minimalToolContext{fcID: "fc1"}
	result, err := m.RunBeforeToolCallback(tctx, "test_tool", map[string]any{"key": "val"})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from early exit")
	}
	cached, _ := result["cached"].(bool)
	if !cached {
		t.Errorf("expected cached=true, got %v", result)
	}
}

// ==========================================================================
// Test 7: plugin-before-direct ordering (model lifecycle)
// ==========================================================================

func TestManagerPluginBeforeDirectModelOrdering(t *testing.T) {
	m := NewManager()
	var order []string

	m.Register(New(Config{
		Name: "plugin_a",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			order = append(order, "plugin_before")
			return nil, nil
		},
		AfterModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
			order = append(order, "plugin_after")
			return nil, nil
		},
	}))

	cctx := &minimalCallbackContext{}

	_, _ = m.RunBeforeModelCallback(cctx, &model.LLMRequest{})
	order = append(order, "direct_before")
	order = append(order, "direct_after")
	_, _ = m.RunAfterModelCallback(cctx, &model.LLMRequest{}, nil, nil)

	expected := []string{"plugin_before", "direct_before", "direct_after", "plugin_after"}
	for i, want := range expected {
		if order[i] != want {
			t.Errorf("order[%d] = %q, want %q", i, order[i], want)
		}
	}
}

// ==========================================================================
// Test 8: all hook types exercise
// ==========================================================================

func TestManagerAllHookTypesNoErrorRecovery(t *testing.T) {
	m := NewManager()
	var called []string

	m.Register(New(Config{
		Name:        "full_plugin",
		BeforeAgent: func(ctx callbackctx.CallbackContext) (*event.Event, error) {
			called = append(called, "before_agent")
			return nil, nil
		},
		AfterAgent: func(ctx callbackctx.CallbackContext, events []*event.Event) (*event.Event, error) {
			called = append(called, "after_agent")
			return nil, nil
		},
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			called = append(called, "before_model")
			return nil, nil
		},
		AfterModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
			called = append(called, "after_model")
			return nil, nil
		},
		BeforeTool: func(ctx callbackctx.ToolContext, toolName string, args map[string]any) (map[string]any, error) {
			called = append(called, "before_tool")
			return nil, nil
		},
		AfterTool: func(ctx callbackctx.ToolContext, toolName string, args map[string]any, result map[string]any, runErr error) (map[string]any, error) {
			called = append(called, "after_tool")
			return nil, nil
		},
	}))

	cctx := &minimalCallbackContext{}
	tctx := &minimalToolContext{fcID: "fc1"}

	m.RunBeforeAgentCallback(cctx)
	m.RunBeforeModelCallback(cctx, &model.LLMRequest{})
	m.RunBeforeToolCallback(tctx, "tool1", nil)
	m.RunAfterToolCallback(tctx, "tool1", nil, nil, nil)
	m.RunAfterModelCallback(cctx, &model.LLMRequest{}, nil, nil)
	m.RunAfterAgentCallback(cctx, nil)

	expected := []string{
		"before_agent",
		"before_model",
		"before_tool",
		"after_tool",
		"after_model",
		"after_agent",
	}
	if len(called) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(called), called)
	}
	for i, want := range expected {
		if called[i] != want {
			t.Errorf("called[%d] = %q, want %q", i, called[i], want)
		}
	}
}

// ==========================================================================
// Test 9: empty manager returns nil,nil for all hooks
// ==========================================================================

func TestManagerEmptyReturnsNilNil(t *testing.T) {
	m := NewManager()
	cctx := &minimalCallbackContext{}
	tctx := &minimalToolContext{fcID: "fc1"}

	ev, err := m.RunBeforeAgentCallback(cctx)
	if ev != nil || err != nil {
		t.Errorf("BeforeAgent: ev=%v, err=%v", ev, err)
	}

	resp, err := m.RunBeforeModelCallback(cctx, &model.LLMRequest{})
	if resp != nil || err != nil {
		t.Errorf("BeforeModel: resp=%v, err=%v", resp, err)
	}

	result, err := m.RunBeforeToolCallback(tctx, "t", nil)
	if result != nil || err != nil {
		t.Errorf("BeforeTool: result=%v, err=%v", result, err)
	}
}

// ==========================================================================
// Test 10: nil Register is no-op
// ==========================================================================

func TestManagerRegisterNil(t *testing.T) {
	m := NewManager()
	m.Register(nil)
	if m.Len() != 0 {
		t.Errorf("Len() = %d after nil register, want 0", m.Len())
	}
}

// ==========================================================================
// Test 11: OnModelError — error recovery
// ==========================================================================

func TestManagerOnModelErrorRecovery(t *testing.T) {
	m := NewManager()

	recoveryResp := model.TextResponse("Recovered from model error.")

	m.Register(New(Config{
		Name: "error_recoverer",
		OnModelError: func(ctx callbackctx.CallbackContext, req *model.LLMRequest, originalErr error) (*model.LLMResponse, error) {
			if originalErr != nil {
				return recoveryResp, nil
			}
			return nil, nil
		},
	}))

	cctx := &minimalCallbackContext{}
	resp, err := m.RunOnModelErrorCallback(cctx, &model.LLMRequest{}, errors.New("original model error"))
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected recovery response")
	}
	if resp.Content.Parts[0].Text != "Recovered from model error." {
		t.Errorf("text = %q", resp.Content.Parts[0].Text)
	}
}

// ==========================================================================
// Test 12: OnToolError — error recovery
// ==========================================================================

func TestManagerOnToolErrorRecovery(t *testing.T) {
	m := NewManager()

	m.Register(New(Config{
		Name: "tool_error_recoverer",
		OnToolError: func(ctx callbackctx.ToolContext, toolName string, args map[string]any, originalErr error) (map[string]any, error) {
			if originalErr != nil {
				return map[string]any{"recovered": true, "tool": toolName}, nil
			}
			return nil, nil
		},
	}))

	tctx := &minimalToolContext{fcID: "fc1"}
	result, err := m.RunOnToolErrorCallback(tctx, "failing_tool", map[string]any{"x": 1}, errors.New("tool crashed"))
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected recovery result")
	}
	if v, ok := result["recovered"]; !ok || v != true {
		t.Errorf("result = %v", result)
	}
	if result["tool"] != "failing_tool" {
		t.Errorf("tool = %v", result["tool"])
	}
}

// ==========================================================================
// Test 13: OnToolError — no error means no recovery needed
// ==========================================================================

func TestManagerOnToolErrorNoError(t *testing.T) {
	m := NewManager()

	m.Register(New(Config{
		Name: "tool_error_handler",
		OnToolError: func(ctx callbackctx.ToolContext, toolName string, args map[string]any, originalErr error) (map[string]any, error) {
			if originalErr != nil {
				return map[string]any{"recovered": true}, nil
			}
			return nil, nil
		},
	}))

	tctx := &minimalToolContext{fcID: "fc1"}
	result, err := m.RunOnToolErrorCallback(tctx, "working_tool", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Errorf("expected nil result when no error, got %v", result)
	}
}

// ==========================================================================
// Test 14: OnModelError — no error, plugin does nothing
// ==========================================================================

func TestManagerOnModelErrorNoError(t *testing.T) {
	m := NewManager()

	m.Register(New(Config{
		Name: "model_error_handler",
		OnModelError: func(ctx callbackctx.CallbackContext, req *model.LLMRequest, originalErr error) (*model.LLMResponse, error) {
			if originalErr != nil {
				return model.TextResponse("recovered"), nil
			}
			return nil, nil
		},
	}))

	cctx := &minimalCallbackContext{}
	resp, err := m.RunOnModelErrorCallback(cctx, &model.LLMRequest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp != nil {
		t.Errorf("expected nil response when no error, got %v", resp)
	}
}

// ==========================================================================
// Test 15: AfterModel — plugin can replace the response
// ==========================================================================

func TestManagerAfterModelReplaceResponse(t *testing.T) {
	m := NewManager()

	replacement := model.TextResponse("Replaced by plugin.")

	m.Register(New(Config{
		Name: "response_replacer",
		AfterModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
			return replacement, nil
		},
	}))

	cctx := &minimalCallbackContext{}
	original := model.TextResponse("Original response.")
	resp, err := m.RunAfterModelCallback(cctx, &model.LLMRequest{}, original, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected non-nil replacement response")
	}
	if resp.Content.Parts[0].Text != "Replaced by plugin." {
		t.Errorf("text = %q", resp.Content.Parts[0].Text)
	}
}

// ==========================================================================
// Test 16: AfterTool — plugin can replace the result
// ==========================================================================

func TestManagerAfterToolReplaceResult(t *testing.T) {
	m := NewManager()

	m.Register(New(Config{
		Name: "result_transformer",
		AfterTool: func(ctx callbackctx.ToolContext, toolName string, args map[string]any, result map[string]any, runErr error) (map[string]any, error) {
			tempC, ok := result["temp_c"].(int)
			if !ok {
				return nil, nil
			}
			return map[string]any{
				"temp_c": tempC,
				"temp_f": tempC*9/5 + 32,
			}, nil
		},
	}))

	tctx := &minimalToolContext{fcID: "fc1"}
	originalResult := map[string]any{"temp_c": 25}
	result, err := m.RunAfterToolCallback(tctx, "get_weather", nil, originalResult, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected transformed result")
	}
	tempF, ok := result["temp_f"].(int)
	if !ok {
		t.Fatalf("temp_f missing or wrong type: %v", result)
	}
	if tempF != 77 {
		t.Errorf("temp_f = %d, want 77", tempF)
	}
}

// ==========================================================================
// Test 17: error in onModelError hook is propagated
// ==========================================================================

func TestManagerOnModelErrorPropagatesError(t *testing.T) {
	m := NewManager()

	m.Register(New(Config{
		Name: "error_recovery_failer",
		OnModelError: func(ctx callbackctx.CallbackContext, req *model.LLMRequest, originalErr error) (*model.LLMResponse, error) {
			return nil, fmt.Errorf("recovery also failed")
		},
	}))

	cctx := &minimalCallbackContext{}
	resp, err := m.RunOnModelErrorCallback(cctx, &model.LLMRequest{}, errors.New("original"))
	if err == nil {
		t.Error("expected error from failing recovery hook")
	}
	if resp != nil {
		t.Error("expected nil response with error")
	}
}

// ==========================================================================
// Test 18: multiple plugins, same hook — first non-nil wins
// ==========================================================================

func TestManagerMultiplePluginsFirstWinsBeforeModel(t *testing.T) {
	m := NewManager()

	m.Register(New(Config{
		Name: "fast",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			return model.TextResponse("fast_response"), nil
		},
	}))
	m.Register(New(Config{
		Name: "slow",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			return model.TextResponse("slow_response"), nil
		},
	}))

	cctx := &minimalCallbackContext{}
	resp, _ := m.RunBeforeModelCallback(cctx, &model.LLMRequest{})
	if resp.Content.Parts[0].Text != "fast_response" {
		t.Errorf("text = %q, want 'fast_response'", resp.Content.Parts[0].Text)
	}
}

// ==========================================================================
// Test 19: AfterAgent callback can observe run events
// ==========================================================================

func TestManagerAfterAgentSeesEvents(t *testing.T) {
	m := NewManager()
	var observedLen int

	m.Register(New(Config{
		Name: "event_observer",
		AfterAgent: func(ctx callbackctx.CallbackContext, events []*event.Event) (*event.Event, error) {
			observedLen = len(events)
			return nil, nil
		},
	}))

	cctx := &minimalCallbackContext{}
	runEvents := []*event.Event{
		{ID: "ev1"},
		{ID: "ev2"},
		{ID: "ev3"},
	}

	ev, err := m.RunAfterAgentCallback(cctx, runEvents)
	if err != nil {
		t.Fatal(err)
	}
	if ev != nil {
		t.Errorf("expected nil event, got %v", ev)
	}
	if observedLen != 3 {
		t.Errorf("observedLen = %d, want 3", observedLen)
	}
}

// ==========================================================================
// Test 20: multi-plugin AfterTool — first non-nil wins
// ==========================================================================

func TestManagerMultiPluginAfterToolFirstWins(t *testing.T) {
	m := NewManager()

	m.Register(New(Config{
		Name: "first_transformer",
		AfterTool: func(ctx callbackctx.ToolContext, toolName string, args map[string]any, result map[string]any, runErr error) (map[string]any, error) {
			return map[string]any{"transformed_by": "first"}, nil
		},
	}))
	m.Register(New(Config{
		Name: "second_transformer",
		AfterTool: func(ctx callbackctx.ToolContext, toolName string, args map[string]any, result map[string]any, runErr error) (map[string]any, error) {
			return map[string]any{"transformed_by": "second"}, nil
		},
	}))

	tctx := &minimalToolContext{fcID: "fc1"}
	result, err := m.RunAfterToolCallback(tctx, "tool1", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := result["transformed_by"].(string); v != "first" {
		t.Errorf("transformed_by = %q, want 'first' (first wins)", v)
	}
}
