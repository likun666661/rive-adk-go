package transfer

import (
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/event"
)

type testAgent struct{ name, desc string }

func (a *testAgent) Name() string               { return a.name }
func (a *testAgent) Description() string        { return a.desc }
func (a *testAgent) Parent() agent.Agent        { return nil }
func (a *testAgent) DisallowTransferToParent() bool { return false }
func (a *testAgent) DisallowTransferToPeers() bool  { return false }
func (a *testAgent) SubAgents() []agent.Agent    { return nil }
func (a *testAgent) FindAgent(name string) agent.Agent {
	if a.name == name {
		return a
	}
	return nil
}

func TestTransferToolDeclarationHasAllowedNames(t *testing.T) {
	targets := []agent.Agent{
		&testAgent{name: "math_bot", desc: "Solves math"},
		&testAgent{name: "weather_bot", desc: "Gets weather"},
	}
	tt := NewTransferToAgentTool(targets)

	decl := tt.Declaration()
	if decl.Name != "transfer_to_agent" {
		t.Errorf("declaration name = %q, want 'transfer_to_agent'", decl.Name)
	}

	enum, ok := decl.InputSchema["properties"].(map[string]any)["agent_name"].(map[string]any)["enum"].([]any)
	if !ok {
		t.Fatalf("expected enum in agent_name schema")
	}
	if len(enum) != 2 {
		t.Fatalf("expected 2 enum values, got %d", len(enum))
	}
	found := make(map[string]bool)
	for _, v := range enum {
		found[v.(string)] = true
	}
	if !found["math_bot"] || !found["weather_bot"] {
		t.Errorf("enum = %v, want math_bot and weather_bot", found)
	}

	names := tt.TargetNames()
	if len(names) != 2 {
		t.Fatalf("TargetNames = %v, want 2 names", names)
	}
}

func TestTransferToolValidTarget(t *testing.T) {
	targets := []agent.Agent{
		&testAgent{name: "math_bot", desc: "Solves math"},
	}
	tt := NewTransferToAgentTool(targets)

	result, err := tt.Run(map[string]any{"agent_name": "math_bot"})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if result["transferred_to"] != "math_bot" {
		t.Errorf("transferred_to = %v, want 'math_bot'", result["transferred_to"])
	}
}

func TestTransferToolInvalidTargetYieldsError(t *testing.T) {
	targets := []agent.Agent{
		&testAgent{name: "math_bot", desc: "Solves math"},
	}
	tt := NewTransferToAgentTool(targets)

	_, err := tt.Run(map[string]any{"agent_name": "nonexistent"})
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
	if !strings.Contains(err.Error(), "invalid agent name") {
		t.Errorf("error = %q, want 'invalid agent name'", err.Error())
	}
}

func TestTransferToolMissingAgentName(t *testing.T) {
	targets := []agent.Agent{
		&testAgent{name: "math_bot", desc: "Solves math"},
	}
	tt := NewTransferToAgentTool(targets)

	_, err := tt.Run(map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing agent_name")
	}
	if !strings.Contains(err.Error(), "agent_name is required") {
		t.Errorf("error = %q", err.Error())
	}

	_, err = tt.Run(map[string]any{"agent_name": ""})
	if err == nil {
		t.Fatal("expected error for empty agent_name")
	}
}

func TestTransferToolEmptyTargets(t *testing.T) {
	tt := NewTransferToAgentTool(nil)

	decl := tt.Declaration()
	enum, ok := decl.InputSchema["properties"].(map[string]any)["agent_name"].(map[string]any)["enum"].([]any)
	if !ok {
		t.Fatalf("expected enum")
	}
	if len(enum) != 0 {
		t.Errorf("expected empty enum, got %v", enum)
	}
}

func TestComputeTransferTargetsNoParent(t *testing.T) {
	sub := &testAgent{name: "sub1", desc: "sub agent"}
	parentAgent, _ := agent.New(agent.Config{
		Name:       "parent",
		SubAgents:  []agent.Agent{sub},
		Run:        func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
	})

	targets := ComputeTransferTargets(parentAgent)
	if len(targets) != 1 {
		t.Fatalf("expected 1 target (sub1), got %d: %v", len(targets), targetNames(targets))
	}
	if targets[0].Name() != "sub1" {
		t.Errorf("target = %q, want 'sub1'", targets[0].Name())
	}
}

func TestComputeTransferTargetsWithParent(t *testing.T) {
	parentAgent, _ := agent.New(agent.Config{
		Name:    "parent",
		Run:     func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
	})

	child, _ := agent.New(agent.Config{
		Name:    "child",
		Parent:  parentAgent,
		SubAgents: []agent.Agent{
			mustNewAgent(t, "grandchild", parentAgent),
		},
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
	})

	targets := ComputeTransferTargets(child)
	if len(targets) < 2 {
		t.Fatalf("expected at least 2 targets, got %d: %v", len(targets), targetNames(targets))
	}
}

func TestComputeTransferTargetsDisallowParent(t *testing.T) {
	parentAgent, _ := agent.New(agent.Config{
		Name:    "parent",
		Run:     func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
	})

	child, _ := agent.New(agent.Config{
		Name:                     "child",
		Parent:                   parentAgent,
		DisallowTransferToParent: true,
		Run:                      func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
	})

	targets := ComputeTransferTargets(child)
	for _, tgt := range targets {
		if tgt.Name() == "parent" {
			t.Error("parent should NOT be in targets when DisallowTransferToParent=true")
		}
	}
}

func TestComputeTransferTargetsDisallowPeers(t *testing.T) {
	peer, _ := agent.New(agent.Config{
		Name:    "peer",
		Run:     func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
	})

	parentAgent, _ := agent.New(agent.Config{
		Name:      "parent",
		SubAgents: []agent.Agent{peer},
		Run:       func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
	})

	child, _ := agent.New(agent.Config{
		Name:                    "child",
		Parent:                  parentAgent,
		DisallowTransferToPeers: true,
		Run:                     func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
	})

	targets := ComputeTransferTargets(child)
	for _, tgt := range targets {
		if tgt.Name() == "peer" {
			t.Error("peer should NOT be in targets when DisallowTransferToPeers=true")
		}
	}
}

func TestTransferInstructions(t *testing.T) {
	targets := []agent.Agent{
		&testAgent{name: "math_bot", desc: "Solves math problems"},
		&testAgent{name: "weather_bot", desc: "Gets weather forecasts"},
	}

	instructions := TransferInstructions("root", targets)
	if instructions == "" {
		t.Fatal("expected non-empty instructions")
	}
	if !strings.Contains(instructions, "math_bot") {
		t.Error("instructions should contain math_bot")
	}
	if !strings.Contains(instructions, "weather_bot") {
		t.Error("instructions should contain weather_bot")
	}
	if !strings.Contains(instructions, "transfer_to_agent") {
		t.Error("instructions should contain transfer_to_agent function name")
	}
	if !strings.Contains(instructions, "Solves math problems") {
		t.Error("instructions should contain math bot description")
	}
}

func TestTransferInstructionsEmptyTargets(t *testing.T) {
	if TransferInstructions("root", nil) != "" {
		t.Error("expected empty instructions for empty targets")
	}
}

func mustNewAgent(t *testing.T, name string, parent agent.Agent) agent.Agent {
	t.Helper()
	a, err := agent.New(agent.Config{
		Name:   name,
		Parent: parent,
		Run:    func(ctx agent.InvocationContext) ([]*event.Event, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func targetNames(targets []agent.Agent) []string {
	names := make([]string, len(targets))
	for i, t := range targets {
		names[i] = t.Name()
	}
	return names
}
