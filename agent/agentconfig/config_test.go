package agentconfig

import (
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/tool"
)

func TestFromJSONBasic(t *testing.T) {
	data := []byte(`{
		"type": "llm_agent",
		"name": "root",
		"description": "Root agent",
		"tools": ["get_weather"]
	}`)

	cfg, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	if cfg.Type != "llm_agent" {
		t.Errorf("Type = %q, want llm_agent", cfg.Type)
	}
	if cfg.Name != "root" {
		t.Errorf("Name = %q, want root", cfg.Name)
	}
	if cfg.Description != "Root agent" {
		t.Errorf("Description = %q, want Root agent", cfg.Description)
	}
	if len(cfg.Tools) != 1 || cfg.Tools[0] != "get_weather" {
		t.Errorf("Tools = %v, want [get_weather]", cfg.Tools)
	}
}

func TestFromJSONInvalidJSON(t *testing.T) {
	_, err := FromJSON([]byte(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestBuildValidTree(t *testing.T) {
	cfg := AgentConfig{
		Type:        "llm_agent",
		Name:        "root",
		Description: "Root agent",
		Tools:       []string{"greet"},
		SubAgents: []AgentConfig{
			{
				Type:        "llm_agent",
				Name:        "child",
				Description: "Child agent",
			},
		},
	}

	registry := ToolRegistry{
		"greet": tool.NewFunctionTool("greet", "Greets the user",
			func(args map[string]any) (map[string]any, error) {
				return map[string]any{"greeting": "hello"}, nil
			},
		),
	}

	a, err := Build(cfg, registry)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "root" {
		t.Errorf("Name = %q, want root", a.Name())
	}

	subs := a.SubAgents()
	if len(subs) != 1 {
		t.Fatalf("SubAgents = %d, want 1", len(subs))
	}
	if subs[0].Name() != "child" {
		t.Errorf("SubAgent[0].Name = %q, want child", subs[0].Name())
	}
}

func TestBuildDuplicateNames(t *testing.T) {
	cfg := AgentConfig{
		Type: "llm_agent",
		Name: "root",
		SubAgents: []AgentConfig{
			{Type: "llm_agent", Name: "duplicate"},
			{Type: "llm_agent", Name: "duplicate"},
		},
	}

	_, err := Build(cfg, nil)
	if err == nil {
		t.Fatal("expected error for duplicate agent names")
	}
}

func TestBuildMissingName(t *testing.T) {
	cfg := AgentConfig{
		Type: "llm_agent",
		Name: "",
	}

	_, err := Build(cfg, nil)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestBuildMissingType(t *testing.T) {
	cfg := AgentConfig{
		Name: "test",
	}

	_, err := Build(cfg, nil)
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestBuildUnknownTool(t *testing.T) {
	cfg := AgentConfig{
		Type:  "llm_agent",
		Name:  "root",
		Tools: []string{"nonexistent"},
	}

	_, err := Build(cfg, ToolRegistry{})
	if err == nil {
		t.Fatal("expected error for unknown tool reference")
	}
}

func TestBuildUnknownToolListsAvailableToolsDeterministically(t *testing.T) {
	cfg := AgentConfig{
		Type:  "llm_agent",
		Name:  "root",
		Tools: []string{"missing"},
	}
	registry := ToolRegistry{
		"zeta":  tool.NewFunctionTool("zeta", "Zeta", func(args map[string]any) (map[string]any, error) { return nil, nil }),
		"alpha": tool.NewFunctionTool("alpha", "Alpha", func(args map[string]any) (map[string]any, error) { return nil, nil }),
	}

	_, err := Build(cfg, registry)
	if err == nil {
		t.Fatal("expected error for unknown tool reference")
	}
	if !strings.Contains(err.Error(), "available tools: [alpha zeta]") {
		t.Fatalf("available tool list should be sorted, got: %v", err)
	}
}

func TestBuildSequentialAgent(t *testing.T) {
	cfg := AgentConfig{
		Type: "sequential",
		Name: "pipeline",
		SubAgents: []AgentConfig{
			{
				Type:        "llm_agent",
				Name:        "step1",
				Description: "First step",
			},
			{
				Type:        "llm_agent",
				Name:        "step2",
				Description: "Second step",
			},
		},
	}

	a, err := Build(cfg, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "pipeline" {
		t.Errorf("Name = %q, want pipeline", a.Name())
	}
	subs := a.SubAgents()
	if len(subs) != 2 {
		t.Fatalf("SubAgents = %d, want 2", len(subs))
	}
	if subs[0].Name() != "step1" {
		t.Errorf("SubAgent[0].Name = %q, want step1", subs[0].Name())
	}
	if subs[1].Name() != "step2" {
		t.Errorf("SubAgent[1].Name = %q, want step2", subs[1].Name())
	}
}

func TestBuildLoopAgent(t *testing.T) {
	cfg := AgentConfig{
		Type:          "loop",
		Name:          "fixer",
		MaxIterations: 5,
		SubAgents: []AgentConfig{
			{
				Type:        "llm_agent",
				Name:        "step",
				Description: "Fix step",
			},
		},
	}

	a, err := Build(cfg, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "fixer" {
		t.Errorf("Name = %q, want fixer", a.Name())
	}
	subs := a.SubAgents()
	if len(subs) != 1 {
		t.Fatalf("SubAgents = %d, want 1", len(subs))
	}
	if subs[0].Name() != "step" {
		t.Errorf("SubAgent[0].Name = %q, want step", subs[0].Name())
	}
}

func TestBuildParallelAgent(t *testing.T) {
	cfg := AgentConfig{
		Type: "parallel",
		Name: "reviewers",
		SubAgents: []AgentConfig{
			{Type: "llm_agent", Name: "reviewer1", Description: "First reviewer"},
			{Type: "llm_agent", Name: "reviewer2", Description: "Second reviewer"},
		},
	}

	a, err := Build(cfg, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "reviewers" {
		t.Errorf("Name = %q, want reviewers", a.Name())
	}
	subs := a.SubAgents()
	if len(subs) != 2 {
		t.Fatalf("SubAgents = %d, want 2", len(subs))
	}
}

func TestBuildTransferConstraints(t *testing.T) {
	cfg := AgentConfig{
		Type:                     "llm_agent",
		Name:                     "leaf",
		Description:              "Leaf agent",
		DisallowTransferToParent: true,
		DisallowTransferToPeers:  true,
	}

	a, err := Build(cfg, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !a.DisallowTransferToParent() {
		t.Error("DisallowTransferToParent should be true")
	}
	if !a.DisallowTransferToPeers() {
		t.Error("DisallowTransferToPeers should be true")
	}
}

func TestBuildUnknownType(t *testing.T) {
	cfg := AgentConfig{
		Type: "nonexistent",
		Name: "test",
	}

	_, err := Build(cfg, nil)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestFromJSONFullConfig(t *testing.T) {
	data := []byte(`{
		"type": "sequential",
		"name": "pipeline",
		"description": "Full pipeline",
		"sub_agents": [
			{
				"type": "llm_agent",
				"name": "generator",
				"description": "Code generator",
				"tools": ["write_file"],
				"disallow_transfer_to_parent": true
			},
			{
				"type": "loop",
				"name": "fix_loop",
				"description": "Fix loop",
				"max_iterations": 3,
				"sub_agents": [
					{
						"type": "llm_agent",
						"name": "fixer",
						"description": "Fixes issues"
					}
				]
			}
		]
	}`)

	cfg, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	if cfg.Type != "sequential" {
		t.Errorf("Type = %q", cfg.Type)
	}
	if len(cfg.SubAgents) != 2 {
		t.Fatalf("SubAgents = %d, want 2", len(cfg.SubAgents))
	}

	gen := cfg.SubAgents[0]
	if gen.Type != "llm_agent" || gen.Name != "generator" {
		t.Errorf("SubAgent[0] = %+v", gen)
	}
	if !gen.DisallowTransferToParent {
		t.Error("SubAgent[0].DisallowTransferToParent should be true")
	}

	loop := cfg.SubAgents[1]
	if loop.Type != "loop" || loop.MaxIterations != 3 {
		t.Errorf("SubAgent[1] = %+v", loop)
	}
	if len(loop.SubAgents) != 1 || loop.SubAgents[0].Name != "fixer" {
		t.Errorf("Loop sub-agent = %+v", loop.SubAgents)
	}
}

func TestBuildNestedTreeWithParentLinks(t *testing.T) {
	cfg := AgentConfig{
		Type: "llm_agent",
		Name: "root",
		SubAgents: []AgentConfig{
			{
				Type: "llm_agent",
				Name: "middle",
				SubAgents: []AgentConfig{
					{
						Type: "llm_agent",
						Name: "leaf",
					},
				},
			},
		},
	}

	a, err := Build(cfg, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Verify parent chain
	subs := a.SubAgents()
	if len(subs) != 1 {
		t.Fatalf("root SubAgents = %d", len(subs))
	}
	middle := subs[0]
	if middle.Parent() != a {
		t.Error("middle.Parent() should be root")
	}
	leafSubs := middle.SubAgents()
	if len(leafSubs) != 1 {
		t.Fatalf("middle SubAgents = %d", len(leafSubs))
	}
	leaf := leafSubs[0]
	if leaf.Parent() != middle {
		t.Error("leaf.Parent() should be middle")
	}

	// Verify FindAgent works
	if found := a.FindAgent("leaf"); found == nil {
		t.Error("FindAgent(leaf) should find leaf")
	} else if found.Name() != "leaf" {
		t.Errorf("FindAgent(leaf).Name = %q", found.Name())
	}
	if found := a.FindAgent("middle"); found == nil {
		t.Error("FindAgent(middle) should find middle")
	}
	if found := a.FindAgent("nonexistent"); found != nil {
		t.Error("FindAgent(nonexistent) should return nil")
	}
}

func TestBuildWorkflowAgentParentLinksAndTransferConstraints(t *testing.T) {
	cfg := AgentConfig{
		Type: "llm_agent",
		Name: "root",
		SubAgents: []AgentConfig{
			{
				Type:                     "sequential",
				Name:                     "pipeline",
				DisallowTransferToParent: true,
				DisallowTransferToPeers:  true,
				SubAgents: []AgentConfig{
					{Type: "llm_agent", Name: "step"},
				},
			},
		},
	}

	a, err := Build(cfg, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	pipeline := a.FindAgent("pipeline")
	if pipeline == nil {
		t.Fatal("FindAgent(pipeline) should find workflow agent")
	}
	if pipeline.Parent() != a {
		t.Error("pipeline.Parent() should be root")
	}
	if !pipeline.DisallowTransferToParent() {
		t.Error("workflow transfer-to-parent constraint should be set")
	}
	if !pipeline.DisallowTransferToPeers() {
		t.Error("workflow transfer-to-peers constraint should be set")
	}

	step := a.FindAgent("step")
	if step == nil {
		t.Fatal("FindAgent(step) should find nested llm agent")
	}
	if step.Parent() != pipeline {
		t.Error("step.Parent() should be pipeline")
	}
}
