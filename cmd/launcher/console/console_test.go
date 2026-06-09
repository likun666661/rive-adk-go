package console

import (
	stdctx "context"
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/artifact"
	"github.com/likun666661/rive-adk-go/cmd/launcher"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/flow"
	"github.com/likun666661/rive-adk-go/llmagent"
	"github.com/likun666661/rive-adk-go/memory"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/plugin"
	"github.com/likun666661/rive-adk-go/runner"
	"github.com/likun666661/rive-adk-go/tool"
)

type simpleLoader struct {
	agent agent.Agent
}

func (l *simpleLoader) RootAgent() agent.Agent { return l.agent }

func newTestAgent(t *testing.T, name string, responses ...*model.LLMResponse) agent.Agent {
	t.Helper()
	f := &flow.Flow{Model: model.NewFakeModel("fake-"+name, responses...)}
	ag, err := llmagent.New(name, "test agent for console", f)
	if err != nil {
		t.Fatal(err)
	}
	return ag
}

func newToolAgent(t *testing.T, name string, tools map[string]tool.FunctionTool, responses ...*model.LLMResponse) agent.Agent {
	t.Helper()
	f := &flow.Flow{
		Model: model.NewFakeModel("fake-"+name, responses...),
		Tools: tools,
	}
	ag, err := llmagent.New(name, "test tool agent", f)
	if err != nil {
		t.Fatal(err)
	}
	return ag
}

func TestConsoleKeyword(t *testing.T) {
	c := NewConsole()
	if c.Keyword() != "console" {
		t.Errorf("keyword = %q, want 'console'", c.Keyword())
	}
}

func TestConsoleParse(t *testing.T) {
	c := NewConsole()
	remaining, err := c.Parse([]string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 {
		t.Errorf("remaining = %v, want 2 args", remaining)
	}
}

func TestConsoleSimpleDescription(t *testing.T) {
	c := NewConsole()
	if c.SimpleDescription() == "" {
		t.Error("description should not be empty")
	}
}

func TestConsoleCommandLineSyntax(t *testing.T) {
	c := NewConsole()
	if c.CommandLineSyntax() == "" {
		t.Error("syntax should not be empty")
	}
}

func TestConsoleRequiresAgentLoader(t *testing.T) {
	c := NewConsole()
	err := c.Run(stdctx.Background(), &launcher.Config{})
	if err == nil {
		t.Fatal("expected error for missing AgentLoader")
	}
	if !strings.Contains(err.Error(), "AgentLoader") {
		t.Errorf("error should mention AgentLoader: %v", err)
	}
}

func TestConsoleRunSimpleText(t *testing.T) {
	var buf strings.Builder

	ag := newTestAgent(t, "echo_bot", model.TextResponse("Hello from test!"))

	in := strings.NewReader("Hello world\n")
	c := NewConsole(WithInput(in), WithOutput(&buf))

	config := &launcher.Config{
		AgentLoader:    &simpleLoader{agent: ag},
		SessionService: runner.NewInMemorySessionService(),
	}

	err := c.Run(stdctx.Background(), config)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Hello from test!") {
		t.Errorf("output should contain model response: %s", output)
	}
	if !strings.Contains(output, "events persisted") {
		t.Errorf("output should mention persisted events: %s", output)
	}
}

func TestConsoleRunToolChain(t *testing.T) {
	var buf strings.Builder

	weatherTool := tool.NewFunctionTool("get_weather", "Get weather",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"temp": 22, "condition": "sunny"}, nil
		},
	)

	ag := newToolAgent(t, "weather_bot",
		map[string]tool.FunctionTool{"get_weather": weatherTool},
		model.FunctionCallResponse("Let me check.",
			event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
		),
		model.TextResponse("Tokyo is 22°C and sunny."),
	)

	in := strings.NewReader("Weather in Tokyo?\n")
	c := NewConsole(WithInput(in), WithOutput(&buf))

	sessionSvc := runner.NewInMemorySessionService()
	config := &launcher.Config{
		AgentLoader:    &simpleLoader{agent: ag},
		SessionService: sessionSvc,
	}

	err := c.Run(stdctx.Background(), config)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "get_weather") {
		t.Errorf("output should mention tool call: %s", output)
	}
	if !strings.Contains(output, "Tokyo is 22") {
		t.Errorf("output should contain final text: %s", output)
	}

	// Verify session persisted events.
	sess, err := sessionSvc.Get(stdctx.Background(), defaultAppName, defaultUserID, "console-session-1")
	if err != nil {
		t.Fatalf("session lookup: %v", err)
	}
	events := sess.Events()
	// user + model(fc) + tool + model(final) = 4 events
	if len(events) != 4 {
		t.Fatalf("expected 4 session events, got %d", len(events))
	}
	wantRoles := []event.Role{event.RoleUser, event.RoleModel, event.RoleTool, event.RoleModel}
	for i, want := range wantRoles {
		if events[i].Role != want {
			t.Errorf("session event[%d] role = %q, want %q", i, events[i].Role, want)
		}
	}
}

func TestConsoleRunMultipleMessages(t *testing.T) {
	var buf strings.Builder

	ag := newTestAgent(t, "multi_bot",
		model.TextResponse("first reply"),
		model.TextResponse("second reply"),
	)

	in := strings.NewReader("msg1\nmsg2\n")
	c := NewConsole(WithInput(in), WithOutput(&buf))

	sessionSvc := runner.NewInMemorySessionService()
	config := &launcher.Config{
		AgentLoader:    &simpleLoader{agent: ag},
		SessionService: sessionSvc,
	}

	err := c.Run(stdctx.Background(), config)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "first reply") {
		t.Error("output should contain first reply")
	}
	if !strings.Contains(output, "second reply") {
		t.Error("output should contain second reply")
	}

	// Session should accumulate events across both runs.
	sess, _ := sessionSvc.Get(stdctx.Background(), defaultAppName, defaultUserID, "console-session-1")
	// Run 1: user + model = 2. Run 2: user + model = 2. Total = 4.
	if sess.EventCount() != 4 {
		t.Errorf("session event count = %d, want 4", sess.EventCount())
	}
}

func TestConsoleRunWithMemoryAndArtifactServices(t *testing.T) {
	var buf strings.Builder

	memSvc := memory.InMemoryService()
	artSvc := artifact.InMemoryService()

	ag := newTestAgent(t, "svc_bot", model.TextResponse("service test"))

	in := strings.NewReader("test\n")
	c := NewConsole(WithInput(in), WithOutput(&buf))

	config := &launcher.Config{
		AgentLoader:     &simpleLoader{agent: ag},
		SessionService:  runner.NewInMemorySessionService(),
		MemoryService:   memSvc,
		ArtifactService: artSvc,
	}

	err := c.Run(stdctx.Background(), config)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "service test") {
		t.Errorf("output should contain response: %s", output)
	}
}

func TestConsoleRunEmptyLinesSkipped(t *testing.T) {
	var buf strings.Builder

	ag := newTestAgent(t, "skip_bot", model.TextResponse("reply"))

	in := strings.NewReader("\n\nhello\n\n\n")
	c := NewConsole(WithInput(in), WithOutput(&buf))

	config := &launcher.Config{
		AgentLoader:    &simpleLoader{agent: ag},
		SessionService: runner.NewInMemorySessionService(),
	}

	err := c.Run(stdctx.Background(), config)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "reply") {
		t.Errorf("output should contain reply after empty lines: %s", output)
	}
}

func TestConsoleRunWithPluginManager(t *testing.T) {
	var buf strings.Builder

	mgr := plugin.NewManager()
	mgr.Register(plugin.New(plugin.Config{
		Name: "observer",
	}))

	ag := newTestAgent(t, "plugin_bot", model.TextResponse("plugin test"))
	in := strings.NewReader("test\n")
	c := NewConsole(WithInput(in), WithOutput(&buf))

	config := &launcher.Config{
		AgentLoader:    &simpleLoader{agent: ag},
		SessionService: runner.NewInMemorySessionService(),
		PluginManager:  mgr,
	}

	err := c.Run(stdctx.Background(), config)
	if err != nil {
		t.Fatalf("Run with plugin failed: %v", err)
	}

	if !strings.Contains(buf.String(), "plugin test") {
		t.Errorf("output should contain response: %s", buf.String())
	}
}

func TestConsoleDefaultSessionService(t *testing.T) {
	var buf strings.Builder

	ag := newTestAgent(t, "default_bot", model.TextResponse("default session svc"))

	in := strings.NewReader("test\n")
	c := NewConsole(WithInput(in), WithOutput(&buf))

	config := &launcher.Config{
		AgentLoader: &simpleLoader{agent: ag},
	}

	err := c.Run(stdctx.Background(), config)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if !strings.Contains(buf.String(), "default session svc") {
		t.Errorf("output should contain response: %s", buf.String())
	}
}

func TestConsoleSubLauncherInterface(t *testing.T) {
	var s launcher.SubLauncher = NewConsole()
	if s == nil {
		t.Fatal("Console should satisfy SubLauncher")
	}
}
