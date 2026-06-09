package runner

import (
	stdctx "context"
	"fmt"
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/artifact"
	"github.com/likun666661/rive-adk-go/callbackctx"
	invctx "github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/flow"
	"github.com/likun666661/rive-adk-go/instruction"
	"github.com/likun666661/rive-adk-go/llmagent"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/plugin"
	"github.com/likun666661/rive-adk-go/tool"
)

// =============================================================================
// Chapter 04 — Instruction / Plugin / Callback integration tests
// =============================================================================

// ---------------------------------------------------------------------------
// Test 24: instruction processor injects SystemInstruction into request
// ---------------------------------------------------------------------------

func TestRunnerInstructionProcessor(t *testing.T) {
	var capturedInstruction string

	f := &flow.Flow{
		Model: model.NewFakeModel("fake", model.TextResponse("ok")),
		RequestProcessors: []flow.RequestProcessor{
			instruction.ToRequestProcessor(instruction.NewRequestProcessor(instruction.Config{
				Instruction: "You are a weather bot. Be concise.",
			})),
		},
		BeforeModelCallbacks: []flow.BeforeModelCallback{
			func(_ invctx.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				capturedInstruction = req.SystemInstruction
				return nil, nil
			},
		},
	}

	ag, err := llmagent.New("instr_bot", "test", f)
	if err != nil {
		t.Fatal(err)
	}

	r, err := New(Config{
		AppName:        "instr_app",
		Agent:          ag.(ExecutableAgent),
		SessionService: NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = r.Run(stdctx.Background(), "user-1", "sess-i", "hi")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if capturedInstruction != "You are a weather bot. Be concise." {
		t.Errorf("instruction = %q, want %q", capturedInstruction, "You are a weather bot. Be concise.")
	}
}

// ---------------------------------------------------------------------------
// Test 25: dynamic instruction provider sees UserContent
// ---------------------------------------------------------------------------

func TestRunnerDynamicInstructionProvider(t *testing.T) {
	var capturedInstruction string

	dynamicProvider := func(ctx instruction.ReadonlyContext) (string, error) {
		if strings.Contains(ctx.UserContent(), "weather") {
			return "The user asked about weather. Provide weather info.", nil
		}
		return "", nil
	}

	f := &flow.Flow{
		Model: model.NewFakeModel("fake", model.TextResponse("It is sunny.")),
		RequestProcessors: []flow.RequestProcessor{
			instruction.ToRequestProcessor(instruction.NewRequestProcessor(instruction.Config{
				InstructionProvider: dynamicProvider,
			})),
		},
		BeforeModelCallbacks: []flow.BeforeModelCallback{
			func(_ invctx.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				capturedInstruction = req.SystemInstruction
				return nil, nil
			},
		},
	}

	ag, _ := llmagent.New("dynamic_bot", "test", f)
	r, _ := New(Config{
		AppName:        "dyn_app",
		Agent:          ag.(ExecutableAgent),
		SessionService: NewInMemorySessionService(),
	})

	_, _, err := r.Run(stdctx.Background(), "user-1", "sess-dyn", "What's the weather?")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	expected := "The user asked about weather. Provide weather info."
	if capturedInstruction != expected {
		t.Errorf("instruction = %q, want %q", capturedInstruction, expected)
	}
}

// ---------------------------------------------------------------------------
// Test 26: global instruction applied only for root agent
// ---------------------------------------------------------------------------

func TestRunnerGlobalInstruction(t *testing.T) {
	var capturedInstruction string

	f := &flow.Flow{
		Model: model.NewFakeModel("fake", model.TextResponse("ok")),
		RequestProcessors: []flow.RequestProcessor{
			instruction.ToRequestProcessor(instruction.NewRequestProcessor(instruction.Config{
				Instruction:       "Agent-level instruction.",
				GlobalInstruction: "Global safety rules: be polite.",
				IsRootAgent:       func() bool { return true },
			})),
		},
		BeforeModelCallbacks: []flow.BeforeModelCallback{
			func(_ invctx.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				capturedInstruction = req.SystemInstruction
				return nil, nil
			},
		},
	}

	ag, _ := llmagent.New("global_bot", "test", f)
	r, _ := New(Config{
		AppName:        "global_app",
		Agent:          ag.(ExecutableAgent),
		SessionService: NewInMemorySessionService(),
	})

	_, _, err := r.Run(stdctx.Background(), "user-1", "sess-global", "hi")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if !strings.Contains(capturedInstruction, "Global safety rules: be polite.") {
		t.Errorf("expected global instruction in %q", capturedInstruction)
	}
	if !strings.Contains(capturedInstruction, "Agent-level instruction.") {
		t.Errorf("expected agent instruction in %q", capturedInstruction)
	}
}

// ---------------------------------------------------------------------------
// Test 27: template injection from session state
// ---------------------------------------------------------------------------

func TestRunnerInstructionTemplateInjection(t *testing.T) {
	var capturedInstruction string

	sessionSvc := NewInMemorySessionService()

	f := &flow.Flow{
		Model: model.NewFakeModel("fake", model.TextResponse("ok")),
		RequestProcessors: []flow.RequestProcessor{
			func(ic invctx.InvocationContext, req *model.LLMRequest) (*event.Event, error) {
				st := ic.Session().State()
				merged := instruction.MergeStateView(
					map[string]any{"version": "2.0"},
					map[string]any{"lang": "en"},
					st.All(),
				)
				req.SystemInstruction = "App v{app:version}, lang={user:lang}, task={topic}."
				injected, err := instruction.InjectSessionState(req.SystemInstruction, merged)
				if err != nil {
					return nil, err
				}
				req.SystemInstruction = injected
				capturedInstruction = injected
				return nil, nil
			},
		},
	}

	ag, _ := llmagent.New("tmpl_bot", "test", f)

	sess, _ := sessionSvc.Create(stdctx.Background(), "tmpl_app", "user-1", "sess-tmpl")
	sess.State().Set("topic", "report-generation")

	nextOrd := sess.EventCount() + 1
	invID := fmt.Sprintf("%s-inv-%d", sess.ID(), nextOrd)
	userEv := event.NewEvent(fmt.Sprintf("%s-user-%d", sess.ID(), nextOrd), "user", event.RoleUser)
	userEv.Branch = ag.Name()
	userEv.Content = &event.Content{Role: event.RoleUser, Parts: []event.Part{{Text: "Generate a report"}}}
	sess.AppendEvent(userEv)

	ic := invctx.NewInvocationContext(invctx.Params{
		Ctx:          stdctx.Background(),
		Agent:        ag,
		Session:      sess,
		InvocationID: invID,
		Branch:       ag.Name(),
		UserContent:  "Generate a report",
	})

	_, err := ag.(ExecutableAgent).Execute(ic)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	expected := "App v2.0, lang=en, task=report-generation."
	if capturedInstruction != expected {
		t.Errorf("instruction = %q, want %q", capturedInstruction, expected)
	}
}

// Need callbackctx import
// (imported at top)

// ---------------------------------------------------------------------------
// Test 28: Plugin before-model early exit (cache / mock)
// ---------------------------------------------------------------------------

func TestRunnerPluginBeforeModelEarlyExit(t *testing.T) {
	mgr := plugin.NewManager()
	mgr.Register(plugin.New(plugin.Config{
		Name: "early",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			if ctx.UserContent() == "cached" {
				return model.TextResponse("CACHED RESPONSE"), nil
			}
			return nil, nil
		},
	}))

	f := &flow.Flow{
		Model: model.NewFakeModel("fake",
			model.TextResponse("REAL RESPONSE — should not appear"),
		),
		PluginManager: mgr,
	}

	ag, _ := llmagent.New("cache_bot", "test", f)
	r, _ := New(Config{
		AppName:        "cache_app",
		Agent:          ag.(ExecutableAgent),
		SessionService: NewInMemorySessionService(),
	})

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-cache", "cached")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event (early exit), got %d", len(events))
	}
	text := events[0].Content.Parts[0].Text
	if text != "CACHED RESPONSE" {
		t.Errorf("text = %q, want %q", text, "CACHED RESPONSE")
	}
}

// ---------------------------------------------------------------------------
// Test 29: Plugin ordering — plugins before direct callbacks
// ---------------------------------------------------------------------------

func TestRunnerPluginOrdering(t *testing.T) {
	var order []string
	record := func(s string) { order = append(order, s) }

	mgr := plugin.NewManager()
	mgr.Register(plugin.New(plugin.Config{
		Name: "p1",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			record("plugin:beforeModel")
			return nil, nil
		},
		AfterModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
			record("plugin:afterModel")
			return nil, nil
		},
	}))

	f := &flow.Flow{
		Model: model.NewFakeModel("fake", model.TextResponse("ok")),
		BeforeModelCallbacks: []flow.BeforeModelCallback{
			func(_ invctx.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				record("direct:beforeModel")
				return nil, nil
			},
		},
		AfterModelCallbacks: []flow.AfterModelCallback{
			func(_ invctx.InvocationContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
				record("direct:afterModel")
				return nil, nil
			},
		},
		PluginManager: mgr,
	}

	ag, _ := llmagent.New("order_bot", "test", f)
	r, _ := New(Config{
		AppName:        "order_app",
		Agent:          ag.(ExecutableAgent),
		SessionService: NewInMemorySessionService(),
	})

	_, _, err := r.Run(stdctx.Background(), "user-1", "sess-order", "hi")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	want := []string{
		"plugin:beforeModel",
		"direct:beforeModel",
		"plugin:afterModel",
		"direct:afterModel",
	}
	for i, w := range want {
		if i >= len(order) {
			t.Fatalf("step[%d] missing, want %q", i, w)
		}
		if order[i] != w {
			t.Errorf("step[%d] = %q, want %q", i, order[i], w)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 30: Plugin after-model transforms response
// ---------------------------------------------------------------------------

func TestRunnerPluginAfterModelTransform(t *testing.T) {
	mgr := plugin.NewManager()
	mgr.Register(plugin.New(plugin.Config{
		Name: "transformer",
		AfterModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
			if resp != nil && resp.Content != nil && len(resp.Content.Parts) > 0 {
				resp.Content.Parts[0].Text = "[TRANSFORMED] " + resp.Content.Parts[0].Text
			}
			return resp, nil
		},
	}))

	f := &flow.Flow{
		Model:         model.NewFakeModel("fake", model.TextResponse("original text")),
		PluginManager: mgr,
	}

	ag, _ := llmagent.New("transform_bot", "test", f)
	r, _ := New(Config{
		AppName:        "trans_app",
		Agent:          ag.(ExecutableAgent),
		SessionService: NewInMemorySessionService(),
	})

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-trans", "hi")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	text := events[0].Content.Parts[0].Text
	if text != "[TRANSFORMED] original text" {
		t.Errorf("text = %q, want %q", text, "[TRANSFORMED] original text")
	}
}

// ---------------------------------------------------------------------------
// Test 31: Full chain — instruction processor + plugin + callback + state
// ---------------------------------------------------------------------------

func TestRunnerFullChainInstructionPluginCallback(t *testing.T) {
	var log []string
	record := func(s string) { log = append(log, s) }

	mgr := plugin.NewManager()
	mgr.Register(plugin.New(plugin.Config{
		Name: "logging-plugin",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			record("plugin:beforeModel")
			return nil, nil
		},
		AfterTool: func(ctx callbackctx.ToolContext, toolName string, args, result map[string]any, runErr error) (map[string]any, error) {
			record("plugin:afterTool")
			return nil, nil
		},
	}))

	sessionSvc := NewInMemorySessionService()

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

	var capturedInstruction string

	f := &flow.Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Setting state...",
				event.FunctionCall{ID: "fc1", Name: "set_state", Args: map[string]any{"key": "topic", "value": "integration-test"}},
			),
			model.TextResponse("Hello! I'll help with integration-test."),
		),
		Tools: map[string]tool.FunctionTool{"set_state": stateTool},
		RequestProcessors: []flow.RequestProcessor{
			func(ic invctx.InvocationContext, req *model.LLMRequest) (*event.Event, error) {
				state := ic.Session().State()
				merged := instruction.MergeStateView(nil, nil, state.All())
				tmpl := "Help the user. Current topic: {topic}."
				injected, _ := instruction.InjectSessionState(tmpl, merged)
				req.SystemInstruction = injected
				capturedInstruction = injected
				record("requestProcessor")
				return nil, nil
			},
		},
		BeforeModelCallbacks: []flow.BeforeModelCallback{
			func(_ invctx.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				record("direct:beforeModel")
				return nil, nil
			},
		},
		PluginManager: mgr,
	}

	ag, _ := llmagent.New("full_bot", "test", f)
	r, _ := New(Config{
		AppName:        "full_app",
		Agent:          ag.(ExecutableAgent),
		SessionService: sessionSvc,
	})

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-full", "Help me")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events (model fc + tool + model final), got %d", len(events))
	}

	if !strings.Contains(capturedInstruction, "integration-test") {
		t.Errorf("instruction missing topic injection: %q", capturedInstruction)
	}

	foundRP := false
	foundPlugin := false
	foundDirect := false
	for _, s := range log {
		if s == "requestProcessor" {
			foundRP = true
		}
		if s == "plugin:beforeModel" {
			foundPlugin = true
		}
		if s == "direct:beforeModel" {
			foundDirect = true
		}
	}
	if !foundRP {
		t.Error("requestProcessor not executed")
	}
	if !foundPlugin {
		t.Error("plugin:beforeModel not executed")
	}
	if !foundDirect {
		t.Error("direct:beforeModel not executed")
	}

	toolEv := events[1]
	if toolEv.Role != event.RoleTool {
		t.Errorf("expected tool event, got role=%q", toolEv.Role)
	}
}

// ---------------------------------------------------------------------------
// Test 32: global instruction NOT applied for non-root (IsRootAgent=false)
// ---------------------------------------------------------------------------

func TestRunnerGlobalInstructionNotRoot(t *testing.T) {
	var capturedInstruction string

	f := &flow.Flow{
		Model: model.NewFakeModel("fake", model.TextResponse("ok")),
		RequestProcessors: []flow.RequestProcessor{
			instruction.ToRequestProcessor(instruction.NewRequestProcessor(instruction.Config{
				Instruction:       "Agent instruction.",
				GlobalInstruction: "SHOULD NOT APPEAR",
				IsRootAgent:       func() bool { return false },
			})),
		},
		BeforeModelCallbacks: []flow.BeforeModelCallback{
			func(_ invctx.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				capturedInstruction = req.SystemInstruction
				return nil, nil
			},
		},
	}

	ag, _ := llmagent.New("nonroot_bot", "test", f)
	r, _ := New(Config{
		AppName:        "nonroot_app",
		Agent:          ag.(ExecutableAgent),
		SessionService: NewInMemorySessionService(),
	})

	_, _, err := r.Run(stdctx.Background(), "user-1", "sess-nr", "hi")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if strings.Contains(capturedInstruction, "SHOULD NOT APPEAR") {
		t.Error("global instruction leaked to non-root agent")
	}
	if capturedInstruction != "Agent instruction." {
		t.Errorf("instruction = %q, want %q", capturedInstruction, "Agent instruction.")
	}
}

// ---------------------------------------------------------------------------
// Test 33: Plugin before-tool early exit bypasses tool execution
// ---------------------------------------------------------------------------

func TestRunnerPluginBeforeToolEarlyExit(t *testing.T) {
	mgr := plugin.NewManager()
	mgr.Register(plugin.New(plugin.Config{
		Name: "mock-tool",
		BeforeTool: func(ctx callbackctx.ToolContext, toolName string, args map[string]any) (map[string]any, error) {
			return map[string]any{"mocked": true, "original_tool": toolName}, nil
		},
	}))

	realTool := tool.NewFunctionTool("real_tool", "Should not execute",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"executed": "REAL — should not happen"}, nil
		},
	)

	f := &flow.Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Calling tool...",
				event.FunctionCall{ID: "fc1", Name: "real_tool", Args: map[string]any{"input": "test"}},
			),
			model.TextResponse("Done."),
		),
		Tools:         map[string]tool.FunctionTool{"real_tool": realTool},
		PluginManager: mgr,
	}

	ag, _ := llmagent.New("mock_bot", "test", f)
	r, _ := New(Config{
		AppName:        "mock_app",
		Agent:          ag.(ExecutableAgent),
		SessionService: NewInMemorySessionService(),
	})

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-mock", "Test")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	toolEv := events[1]
	fr := toolEv.Content.Parts[0].FunctionResponse
	if fr == nil {
		t.Fatal("expected function response")
	}
	mocked, ok := fr.Result["mocked"].(bool)
	if !ok || !mocked {
		t.Errorf("expected mocked=true, got result=%v", fr.Result)
	}
	if fr.Result["original_tool"] != "real_tool" {
		t.Errorf("original_tool = %v, want 'real_tool'", fr.Result["original_tool"])
	}
}

func TestRunnerContextAwareModelCallbackActionsSurfaceOnEvent(t *testing.T) {
	artSvc := artifact.InMemoryService()

	f := &flow.Flow{
		Model: model.NewFakeModel("fake", model.TextResponse("ok")),
		BeforeModelCallbacksCtx: []flow.BeforeModelCallbackCtx{
			func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				ctx.State().Set("model_callback", "ran")
				_, err := ctx.ArtifactService().Save(stdctx.Background(), &artifact.SaveRequest{
					AppName:   ctx.AppName(),
					UserID:    ctx.UserID(),
					SessionID: ctx.SessionID(),
					FileName:  "model-note.txt",
					Part:      &artifact.ArtifactPart{Text: "model callback"},
				})
				return nil, err
			},
		},
	}

	ag, _ := llmagent.New("model_ctx_bot", "test", f)
	r, _ := New(Config{
		AppName:         "model_ctx_app",
		Agent:           ag.(ExecutableAgent),
		SessionService:  NewInMemorySessionService(),
		ArtifactService: artSvc,
	})

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-model-ctx", "hi")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 model event, got %d", len(events))
	}
	if events[0].Actions.StateDelta["model_callback"] != "ran" {
		t.Fatalf("model event missing callback state delta: %#v", events[0].Actions.StateDelta)
	}
	if events[0].Actions.ArtifactDelta["model-note.txt"] != 1 {
		t.Fatalf("model event missing artifact delta: %#v", events[0].Actions.ArtifactDelta)
	}
	if v, ok := sess.State().Get("model_callback"); !ok || v != "ran" {
		t.Fatalf("session state model_callback = %v, %v; want ran, true", v, ok)
	}
}

func TestRunnerContextAwareToolCallbacksMergeActionsAndOrdering(t *testing.T) {
	var order []string
	record := func(step string) { order = append(order, step) }
	directSawPluginState := false

	artSvc := artifact.InMemoryService()
	mgr := plugin.NewManager()
	mgr.Register(plugin.New(plugin.Config{
		Name: "tool-plugin",
		BeforeTool: func(ctx callbackctx.ToolContext, toolName string, args map[string]any) (map[string]any, error) {
			record("plugin:beforeTool")
			ctx.State().Set("tool_plugin", true)
			return nil, nil
		},
	}))

	realTool := tool.NewFunctionTool("real_tool", "Runs once",
		func(args map[string]any) (map[string]any, error) {
			record("tool:run")
			return map[string]any{"status": "ok"}, nil
		},
	)

	f := &flow.Flow{
		Model: model.NewFakeModel("fake",
			model.FunctionCallResponse("Calling tool...",
				event.FunctionCall{ID: "fc1", Name: "real_tool", Args: map[string]any{}},
			),
			model.TextResponse("Done."),
		),
		Tools:         map[string]tool.FunctionTool{"real_tool": realTool},
		PluginManager: mgr,
		BeforeToolCallbacksCtx: []flow.BeforeToolCallbackCtx{
			func(ctx callbackctx.ToolContext, toolName string, args map[string]any) (map[string]any, error) {
				record("direct:beforeToolCtx")
				if v, ok := ctx.State().Get("tool_plugin"); !ok || v != true {
					directSawPluginState = false
				} else {
					directSawPluginState = true
				}
				ctx.State().Set("tool_direct_before", true)
				return nil, nil
			},
		},
		AfterToolCallbacksCtx: []flow.AfterToolCallbackCtx{
			func(ctx callbackctx.ToolContext, toolName string, args, result map[string]any, runErr error) (map[string]any, error) {
				record("direct:afterToolCtx")
				ctx.State().Set("tool_after", "ctx")
				_, err := ctx.ArtifactService().Save(stdctx.Background(), &artifact.SaveRequest{
					AppName:   ctx.AppName(),
					UserID:    ctx.UserID(),
					SessionID: ctx.SessionID(),
					FileName:  "tool-note.txt",
					Part:      &artifact.ArtifactPart{Text: "tool callback"},
				})
				return nil, err
			},
		},
	}

	ag, _ := llmagent.New("tool_ctx_bot", "test", f)
	r, _ := New(Config{
		AppName:         "tool_ctx_app",
		Agent:           ag.(ExecutableAgent),
		SessionService:  NewInMemorySessionService(),
		ArtifactService: artSvc,
	})

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-tool-ctx", "go")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	wantOrder := []string{"plugin:beforeTool", "direct:beforeToolCtx", "tool:run", "direct:afterToolCtx"}
	for i, want := range wantOrder {
		if i >= len(order) {
			t.Fatalf("order[%d] missing, want %q; full order=%v", i, want, order)
		}
		if order[i] != want {
			t.Fatalf("order[%d] = %q, want %q; full order=%v", i, order[i], want, order)
		}
	}
	if !directSawPluginState {
		t.Fatal("direct before-tool callback did not see plugin state write")
	}

	toolEv := events[1]
	if toolEv.Actions.StateDelta["tool_plugin"] != true {
		t.Fatalf("tool event missing plugin state delta: %#v", toolEv.Actions.StateDelta)
	}
	if toolEv.Actions.StateDelta["tool_direct_before"] != true {
		t.Fatalf("tool event missing direct before state delta: %#v", toolEv.Actions.StateDelta)
	}
	if toolEv.Actions.StateDelta["tool_after"] != "ctx" {
		t.Fatalf("tool event missing after state delta: %#v", toolEv.Actions.StateDelta)
	}
	if toolEv.Actions.ArtifactDelta["tool-note.txt"] != 1 {
		t.Fatalf("tool event missing artifact delta: %#v", toolEv.Actions.ArtifactDelta)
	}
	if v, ok := sess.State().Get("tool_after"); !ok || v != "ctx" {
		t.Fatalf("session state tool_after = %v, %v; want ctx, true", v, ok)
	}
}
