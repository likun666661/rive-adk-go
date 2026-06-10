// Package agentconfig provides a simple JSON-based config loader for
// constructing agent trees for educational examples.
//
// The JSON config format supports three agent types:
//   - "llm_agent" — an LLM-driven ReAct agent with tools and optional sub-agents
//   - "sequential" — runs sub-agents in declaration order
//   - "parallel"   — runs sub-agents concurrently
//   - "loop"       — runs sub-agents in a loop up to max_iterations times
//
// A minimal example:
//
//	{
//	  "type": "llm_agent",
//	  "name": "root",
//	  "description": "A helpful agent",
//	  "tools": ["get_weather"],
//	  "sub_agents": [
//	    {
//	      "type": "llm_agent",
//	      "name": "specialist",
//	      "description": "Handles specialized queries"
//	    }
//	  ]
//	}
//
// Tools are resolved from a registry passed at build time. The registry is a
// map of tool name -> tool.FunctionTool. Duplicate agent names and missing tool
// references produce deterministic errors.
//
// This package is intentionally minimal. It does not support YAML, plugin
// configuration, instruction strings, callback registration, or deployment
// config. Those are left as extension slots for educational exploration.
package agentconfig

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/flow"
	"github.com/likun666661/rive-adk-go/llmagent"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/tool"
	"github.com/likun666661/rive-adk-go/workflow"
)

// AgentConfig is the JSON-serializable configuration for a single agent node.
type AgentConfig struct {
	Type        string        `json:"type"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Model       string        `json:"model,omitempty"`
	Tools       []string      `json:"tools,omitempty"`
	SubAgents   []AgentConfig `json:"sub_agents,omitempty"`

	MaxIterations int `json:"max_iterations,omitempty"`

	DisallowTransferToParent bool `json:"disallow_transfer_to_parent,omitempty"`
	DisallowTransferToPeers  bool `json:"disallow_transfer_to_peers,omitempty"`
}

// ToolRegistry maps tool names to their FunctionTool implementations.
type ToolRegistry map[string]tool.FunctionTool

// Build constructs an agent tree from the config.
func Build(cfg AgentConfig, registry ToolRegistry) (agent.Agent, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("agentconfig: agent name is required")
	}
	if err := validateNoDuplicateNames(cfg, nil); err != nil {
		return nil, err
	}
	if err := validateToolRefs(cfg, registry); err != nil {
		return nil, err
	}

	a, err := buildNode(cfg, registry)
	if err != nil {
		return nil, err
	}

	wireParents(a, nil)
	return a, nil
}

func validateNoDuplicateNames(cfg AgentConfig, seen map[string]string) error {
	if seen == nil {
		seen = make(map[string]string)
	}
	path := cfg.Type + "/" + cfg.Name
	if prevPath, exists := seen[cfg.Name]; exists {
		return fmt.Errorf("agentconfig: duplicate agent name %q (first defined at %q, second at %q)", cfg.Name, prevPath, path)
	}
	seen[cfg.Name] = path
	for _, sub := range cfg.SubAgents {
		if err := validateNoDuplicateNames(sub, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateToolRefs(cfg AgentConfig, registry ToolRegistry) error {
	for _, toolName := range cfg.Tools {
		if _, exists := registry[toolName]; !exists {
			return fmt.Errorf("agentconfig: agent %q references unknown tool %q; available tools: %v",
				cfg.Name, toolName, registryKeys(registry))
		}
	}
	for _, sub := range cfg.SubAgents {
		if err := validateToolRefs(sub, registry); err != nil {
			return err
		}
	}
	return nil
}

func registryKeys(r ToolRegistry) []string {
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func buildNode(cfg AgentConfig, registry ToolRegistry) (agent.Agent, error) {
	switch cfg.Type {
	case "llm_agent":
		return buildLLMAgent(cfg, registry)
	case "sequential":
		return buildSequentialAgent(cfg, registry)
	case "parallel":
		return buildParallelAgent(cfg, registry)
	case "loop":
		return buildLoopAgent(cfg, registry)
	case "":
		return nil, fmt.Errorf("agentconfig: agent %q: type is required (one of: llm_agent, sequential, parallel, loop)", cfg.Name)
	default:
		return nil, fmt.Errorf("agentconfig: agent %q: unknown type %q (expected llm_agent, sequential, parallel, or loop)", cfg.Name, cfg.Type)
	}
}

func buildLLMAgent(cfg AgentConfig, registry ToolRegistry) (agent.Agent, error) {
	toolsMap := make(map[string]tool.FunctionTool, len(cfg.Tools))
	for _, name := range cfg.Tools {
		toolsMap[name] = registry[name]
	}

	var subs []agent.Agent
	for _, sub := range cfg.SubAgents {
		a, err := buildNode(sub, registry)
		if err != nil {
			return nil, err
		}
		subs = append(subs, a)
	}

	modelName := cfg.Model
	if modelName == "" {
		modelName = "config-agent"
	}

	f := &flow.Flow{
		Model: model.NewFakeModel(modelName),
		Tools: toolsMap,
	}

	a, err := llmagent.New(cfg.Name, cfg.Description, f)
	if err != nil {
		return nil, err
	}

	for _, s := range subs {
		if err := agent.SetParent(s, a); err != nil {
			return nil, fmt.Errorf("agentconfig: setting parent for sub-agent %q of %q: %w", s.Name(), cfg.Name, err)
		}
	}
	if err := agent.SetSubAgents(a, subs); err != nil {
		return nil, fmt.Errorf("agentconfig: setting sub-agents for %q: %w", cfg.Name, err)
	}
	if cfg.DisallowTransferToParent {
		_ = agent.SetDisallowTransferToParent(a, true)
	}
	if cfg.DisallowTransferToPeers {
		_ = agent.SetDisallowTransferToPeers(a, true)
	}

	return a, nil
}

func buildSequentialAgent(cfg AgentConfig, registry ToolRegistry) (agent.Agent, error) {
	var subs []agent.Agent
	for _, sub := range cfg.SubAgents {
		a, err := buildNode(sub, registry)
		if err != nil {
			return nil, err
		}
		subs = append(subs, a)
	}

	subList := make([]workflow.SubAgent, len(subs))
	for i, sa := range subs {
		var ok bool
		subList[i], ok = sa.(workflow.SubAgent)
		if !ok {
			return nil, fmt.Errorf("agentconfig: sequential agent %q: sub-agent %q does not implement SubAgent", cfg.Name, sa.Name())
		}
	}

	a := workflow.NewSequentialAgent(cfg.Name, cfg.Description, subList)
	applyTransferConstraints(a, cfg)
	return a, nil
}

func buildParallelAgent(cfg AgentConfig, registry ToolRegistry) (agent.Agent, error) {
	var subs []agent.Agent
	for _, sub := range cfg.SubAgents {
		a, err := buildNode(sub, registry)
		if err != nil {
			return nil, err
		}
		subs = append(subs, a)
	}

	subList := make([]workflow.SubAgent, len(subs))
	for i, sa := range subs {
		var ok bool
		subList[i], ok = sa.(workflow.SubAgent)
		if !ok {
			return nil, fmt.Errorf("agentconfig: parallel agent %q: sub-agent %q does not implement SubAgent", cfg.Name, sa.Name())
		}
	}

	a := workflow.NewParallelAgent(cfg.Name, cfg.Description, subList)
	applyTransferConstraints(a, cfg)
	return a, nil
}

func buildLoopAgent(cfg AgentConfig, registry ToolRegistry) (agent.Agent, error) {
	var subs []agent.Agent
	for _, sub := range cfg.SubAgents {
		a, err := buildNode(sub, registry)
		if err != nil {
			return nil, err
		}
		subs = append(subs, a)
	}

	subList := make([]workflow.SubAgent, len(subs))
	for i, sa := range subs {
		var ok bool
		subList[i], ok = sa.(workflow.SubAgent)
		if !ok {
			return nil, fmt.Errorf("agentconfig: loop agent %q: sub-agent %q does not implement SubAgent", cfg.Name, sa.Name())
		}
	}

	maxIter := cfg.MaxIterations
	if maxIter == 0 {
		maxIter = 10
	}

	a := workflow.NewLoopAgent(cfg.Name, cfg.Description, subList, maxIter)
	applyTransferConstraints(a, cfg)
	return a, nil
}

func applyTransferConstraints(a agent.Agent, cfg AgentConfig) {
	if cfg.DisallowTransferToParent {
		_ = agent.SetDisallowTransferToParent(a, true)
	}
	if cfg.DisallowTransferToPeers {
		_ = agent.SetDisallowTransferToPeers(a, true)
	}
}

// wireParents walks the agent tree and sets parent references for sub-agents
// of workflow agents (which don't do this automatically).
func wireParents(a agent.Agent, parent agent.Agent) {
	if parent != nil {
		_ = agent.SetParent(a, parent)
	}
	for _, sub := range a.SubAgents() {
		wireParents(sub, a)
	}
}

// FromJSON parses a JSON byte slice into an AgentConfig.
func FromJSON(data []byte) (AgentConfig, error) {
	var cfg AgentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return AgentConfig{}, fmt.Errorf("agentconfig: invalid JSON: %w", err)
	}
	return cfg, nil
}
