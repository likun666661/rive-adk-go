package functionmodifier

import (
	"testing"

	"github.com/likun666661/rive-adk-go/artifact"
	"github.com/likun666661/rive-adk-go/callbackctx"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/memory"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/session"
	"github.com/likun666661/rive-adk-go/tool"
)

type testCallbackContext struct {
	st session.State
}

func (t *testCallbackContext) UserContent() string                 { return "" }
func (t *testCallbackContext) InvocationID() string                { return "" }
func (t *testCallbackContext) AgentName() string                   { return "test" }
func (t *testCallbackContext) ReadonlyState() session.ReadonlyState { return nil }
func (t *testCallbackContext) UserID() string                      { return "" }
func (t *testCallbackContext) AppName() string                     { return "" }
func (t *testCallbackContext) SessionID() string                   { return "" }
func (t *testCallbackContext) Branch() string                      { return "" }
func (t *testCallbackContext) ArtifactService() artifact.Service   { return nil }
func (t *testCallbackContext) MemoryService() memory.Service       { return nil }
func (t *testCallbackContext) State() session.State                { return t.st }

var _ callbackctx.CallbackContext = (*testCallbackContext)(nil)

type dummyState struct {
	data map[string]any
}

func (d *dummyState) Get(key string) (any, bool) {
	if d.data == nil {
		return nil, false
	}
	v, ok := d.data[key]
	return v, ok
}
func (d *dummyState) Set(key string, val any) {
	if d.data == nil {
		d.data = make(map[string]any)
	}
	d.data[key] = val
}
func (d *dummyState) Delete(key string) {
	if d.data != nil {
		delete(d.data, key)
	}
}
func (d *dummyState) All() map[string]any {
	if d.data == nil {
		return make(map[string]any)
	}
	out := make(map[string]any, len(d.data))
	for k, v := range d.data {
		out[k] = v
	}
	return out
}

var _ session.State = (*dummyState)(nil)

func TestPluginName(t *testing.T) {
	p := New(Config{Name: "my_modifier"})
	if p.Name() != "my_modifier" {
		t.Errorf("name = %q, want 'my_modifier'", p.Name())
	}
}

func TestPluginDefaultName(t *testing.T) {
	p := New(Config{})
	if p.Name() != "function_call_modifier" {
		t.Errorf("default name = %q, want 'function_call_modifier'", p.Name())
	}
}

func TestPluginBeforeModelInjectsHiddenArgs(t *testing.T) {
	p := New(Config{
		Name: "test_modifier",
		Predicate: func(name string) bool {
			return name == "search"
		},
		HiddenArgs: map[string]any{
			"user_id": map[string]any{"type": "string"},
		},
	})

	req := &model.LLMRequest{
		ToolDeclarations: []any{
			tool.Declaration{
				Name:        "search",
				Description: "Search the web",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
					},
					"required": []any{"query"},
				},
			},
			tool.Declaration{
				Name:        "other",
				Description: "Other tool",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{},
				},
			},
		},
	}

	_, err := p.BeforeModelCallback()(nil, req)
	if err != nil {
		t.Fatalf("BeforeModel error = %v", err)
	}

	searchDecl := req.ToolDeclarations[0].(tool.Declaration)
	props := searchDecl.InputSchema["properties"].(map[string]any)
	if _, ok := props["user_id"]; !ok {
		t.Error("expected user_id in search tool properties")
	}

	otherDecl := req.ToolDeclarations[1].(tool.Declaration)
	otherProps := otherDecl.InputSchema["properties"].(map[string]any)
	if _, ok := otherProps["user_id"]; ok {
		t.Error("user_id should NOT be injected into non-matching tool")
	}
}

func TestPluginAfterModelStripsHiddenArgs(t *testing.T) {
	p := New(Config{
		Name: "test_modifier",
		Predicate: func(name string) bool {
			return name == "search"
		},
		HiddenArgs: map[string]any{
			"user_id": "usr-123",
			"session": "sess-456",
		},
	})

	resp := &model.LLMResponse{
		Content: &model.LLMContent{
			Role: "model",
			Parts: []model.LLMPart{
				{
					FunctionCall: &event.FunctionCall{
						ID:   "fc1",
						Name: "search",
						Args: map[string]any{
							"query":   "hello",
							"user_id": "usr-123",
							"session": "sess-456",
						},
					},
				},
			},
		},
	}

	cctx := &testCallbackContext{st: &dummyState{}}
	_, err := p.AfterModelCallback()(cctx, nil, resp, nil)
	if err != nil {
		t.Fatalf("AfterModel error = %v", err)
	}

	fc := resp.Content.Parts[0].FunctionCall
	if _, ok := fc.Args["user_id"]; ok {
		t.Error("user_id should be stripped from function call")
	}
	if _, ok := fc.Args["session"]; ok {
		t.Error("session should be stripped from function call")
	}
	if fc.Args["query"] != "hello" {
		t.Errorf("query = %q, want 'hello'", fc.Args["query"])
	}

	ds := cctx.st.(*dummyState)
	if v, ok := ds.Get("hidden/fc1/user_id"); !ok || v != "usr-123" {
		t.Errorf("expected hidden/fc1/user_id = 'usr-123' in state, got %v, %v", v, ok)
	}
	if v, ok := ds.Get("hidden/fc1/session"); !ok || v != "sess-456" {
		t.Errorf("expected hidden/fc1/session = 'sess-456' in state, got %v, %v", v, ok)
	}
}

func TestPluginAfterModelDoesNotStripNonMatchingTools(t *testing.T) {
	p := New(Config{
		Name: "test_modifier",
		Predicate: func(name string) bool {
			return name == "sensitive_tool"
		},
		HiddenArgs: map[string]any{"internal_key": "secret"},
	})

	resp := &model.LLMResponse{
		Content: &model.LLMContent{
			Role: "model",
			Parts: []model.LLMPart{
				{
					FunctionCall: &event.FunctionCall{
						ID:   "fc2",
						Name: "public_tool",
						Args: map[string]any{
							"query":       "hello",
							"internal_key": "secret",
						},
					},
				},
			},
		},
	}

	cctx := &testCallbackContext{st: &dummyState{}}
	_, err := p.AfterModelCallback()(cctx, nil, resp, nil)
	if err != nil {
		t.Fatalf("AfterModel error = %v", err)
	}

	fc := resp.Content.Parts[0].FunctionCall
	if _, ok := fc.Args["internal_key"]; !ok {
		t.Error("internal_key should NOT be stripped from non-matching tool")
	}
}

func TestPluginHiddenArgsAccessor(t *testing.T) {
	p := New(Config{
		HiddenArgs: map[string]any{"key": "val"},
	})
	cloned := p.HiddenArgs()
	if cloned["key"] != "val" {
		t.Errorf("HiddenArgs = %v", cloned)
	}
}
