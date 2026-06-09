package instruction

import (
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/session"
)

func TestInjectSessionState(t *testing.T) {
	state := MergeStateView(
		map[string]any{"env": "prod"},
		map[string]any{"theme": "dark"},
		map[string]any{"topic": "weather", "city": "Tokyo"},
	)

	tests := []struct {
		name     string
		template string
		want     string
		wantErr  bool
	}{
		{
			name:     "empty template",
			template: "",
			want:     "",
		},
		{
			name:     "no placeholders",
			template: "You are a helpful assistant.",
			want:     "You are a helpful assistant.",
		},
		{
			name:     "session state variable",
			template: "Talk about {topic}.",
			want:     "Talk about weather.",
		},
		{
			name:     "app scoped variable",
			template: "Environment is {app:env}.",
			want:     "Environment is prod.",
		},
		{
			name:     "user scoped variable",
			template: "Theme is {user:theme}.",
			want:     "Theme is dark.",
		},
		{
			name:     "optional variable exists",
			template: "Topic is {topic?}.",
			want:     "Topic is weather.",
		},
		{
			name:     "optional variable absent",
			template: "Missing is {missing?}.",
			want:     "Missing is .",
		},
		{
			name:     "multiple placeholders",
			template: "You are for {topic}. City: {city}. Theme: {user:theme}.",
			want:     "You are for weather. City: Tokyo. Theme: dark.",
		},
		{
			name:     "required variable absent",
			template: "{missing}",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InjectSessionState(tt.template, state)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewRequestProcessorStaticInstruction(t *testing.T) {
	cfg := Config{
		Instruction: "You are a helpful bot.",
	}
	proc := NewRequestProcessor(cfg)

	req := &model.LLMRequest{}
	ro := &fakeReadonlyContext{}
	_, err := proc(ro, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.SystemInstruction != "You are a helpful bot." {
		t.Errorf("got %q, want %q", req.SystemInstruction, "You are a helpful bot.")
	}
}

func TestNewRequestProcessorDynamicProvider(t *testing.T) {
	var capturedContent string
	cfg := Config{
		InstructionProvider: func(ctx ReadonlyContext) (string, error) {
			capturedContent = ctx.UserContent()
			if ctx.UserContent() == "hello" {
				return "User said hello, be friendly.", nil
			}
			return "", nil
		},
	}
	proc := NewRequestProcessor(cfg)

	req := &model.LLMRequest{}
	ro := &fakeReadonlyContext{userContent: "hello"}
	_, err := proc(ro, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedContent != "hello" {
		t.Errorf("expected UserContent 'hello', got %q", capturedContent)
	}
	if req.SystemInstruction != "User said hello, be friendly." {
		t.Errorf("got %q", req.SystemInstruction)
	}
}

func TestNewRequestProcessorDynamicProviderEmpty(t *testing.T) {
	cfg := Config{
		InstructionProvider: func(ctx ReadonlyContext) (string, error) {
			return "", nil
		},
	}
	proc := NewRequestProcessor(cfg)

	req := &model.LLMRequest{}
	ro := &fakeReadonlyContext{}
	_, err := proc(ro, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.SystemInstruction != "" {
		t.Errorf("expected empty instruction, got %q", req.SystemInstruction)
	}
}

func TestNewRequestProcessorGlobalInstruction(t *testing.T) {
	cfg := Config{
		Instruction:       "Agent instruction.",
		GlobalInstruction: "Global safety rules.",
		IsRootAgent:       func() bool { return true },
	}
	proc := NewRequestProcessor(cfg)

	req := &model.LLMRequest{}
	ro := &fakeReadonlyContext{}
	_, err := proc(ro, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(req.SystemInstruction, "Global safety rules.") {
		t.Errorf("expected global instruction, got %q", req.SystemInstruction)
	}
	if !strings.Contains(req.SystemInstruction, "Agent instruction.") {
		t.Errorf("expected agent instruction, got %q", req.SystemInstruction)
	}
}

func TestNewRequestProcessorGlobalInstructionNotRoot(t *testing.T) {
	cfg := Config{
		Instruction:       "Agent instruction.",
		GlobalInstruction: "Global safety rules.",
		IsRootAgent:       func() bool { return false },
	}
	proc := NewRequestProcessor(cfg)

	req := &model.LLMRequest{}
	ro := &fakeReadonlyContext{}
	_, err := proc(ro, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(req.SystemInstruction, "Global safety rules.") {
		t.Errorf("global instruction should not appear for non-root agent, got %q", req.SystemInstruction)
	}
	if req.SystemInstruction != "Agent instruction." {
		t.Errorf("got %q, want %q", req.SystemInstruction, "Agent instruction.")
	}
}

func TestNewRequestProcessorTemplateInjection(t *testing.T) {
	cfg := Config{
		Instruction: "Talk about {topic} in {city}.",
		ReadonlyState: func() session.ReadonlyState {
			return MergeStateView(nil, nil, map[string]any{
				"topic": "AI",
				"city":  "Paris",
			})
		},
	}
	proc := NewRequestProcessor(cfg)

	req := &model.LLMRequest{}
	ro := &fakeReadonlyContext{}
	_, err := proc(ro, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.SystemInstruction != "Talk about AI in Paris." {
		t.Errorf("got %q, want %q", req.SystemInstruction, "Talk about AI in Paris.")
	}
}

func TestNewRequestProcessorNoConfig(t *testing.T) {
	proc := NewRequestProcessor(Config{})

	req := &model.LLMRequest{}
	ro := &fakeReadonlyContext{}
	_, err := proc(ro, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.SystemInstruction != "" {
		t.Errorf("expected empty instruction, got %q", req.SystemInstruction)
	}
}

// fakeReadonlyContext satisfies instruction.ReadonlyContext.
type fakeReadonlyContext struct {
	userContent string
	state       session.ReadonlyState
}

func (f *fakeReadonlyContext) UserContent() string                   { return f.userContent }
func (f *fakeReadonlyContext) InvocationID() string                  { return "inv-1" }
func (f *fakeReadonlyContext) AgentName() string                     { return "test-agent" }
func (f *fakeReadonlyContext) ReadonlyState() session.ReadonlyState  { return f.state }
func (f *fakeReadonlyContext) UserID() string                        { return "user-1" }
func (f *fakeReadonlyContext) AppName() string                       { return "app" }
func (f *fakeReadonlyContext) SessionID() string                     { return "sess-1" }
func (f *fakeReadonlyContext) Branch() string                        { return "test-agent" }
