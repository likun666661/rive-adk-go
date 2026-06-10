// Package transfer implements the transfer_to_agent function tool.
//
// When a parent agent has sub-agents or allows transfer to parent/peers,
// the transfer_to_agent tool is dynamically injected into every LLM request.
// The LLM can call transfer_to_agent(agent_name) to hand control to another
// agent in the tree.
//
// The tool's Run method sets the TransferToAgent action on the event, which
// the flow detects and executes inline.
package transfer

import (
	"fmt"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/tool"
)

const transferAgentName = "transfer_to_agent"

// TransferToAgentTool is the function tool that the LLM calls to transfer
// control to another agent in the tree.
type TransferToAgentTool struct {
	supportedAgents []agent.Agent
}

// NewTransferToAgentTool creates a TransferToAgentTool with the given
// list of allowed transfer targets.
func NewTransferToAgentTool(targets []agent.Agent) *TransferToAgentTool {
	return &TransferToAgentTool{supportedAgents: targets}
}

func (t *TransferToAgentTool) Name() string { return transferAgentName }
func (t *TransferToAgentTool) Description() string {
	return "Transfer the question to another agent when it is more suitable to answer."
}
func (t *TransferToAgentTool) IsLongRunning() bool { return false }

// Declaration returns the LLM-facing function declaration with an
// enumerated list of allowed agent names.
func (t *TransferToAgentTool) Declaration() tool.Declaration {
	agentNames := make([]string, len(t.supportedAgents))
	for i, a := range t.supportedAgents {
		agentNames[i] = a.Name()
	}

	enumValues := make([]any, len(agentNames))
	for i, n := range agentNames {
		enumValues[i] = n
	}

	return tool.Declaration{
		Name:        transferAgentName,
		Description: t.Description(),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agent_name": map[string]any{
					"type":        "string",
					"description": "The agent name to transfer to.",
					"enum":        enumValues,
				},
			},
			"required": []any{"agent_name"},
		},
	}
}

// Run validates the target agent name and returns a result.
// The ContextFunctionTool variant (RunWithContext) is used during
// actual execution so TransferToAgent can be set on tool actions.
func (t *TransferToAgentTool) Run(args map[string]any) (map[string]any, error) {
	agentName, ok := args["agent_name"].(string)
	if !ok || agentName == "" {
		return nil, fmt.Errorf("transfer_to_agent: agent_name is required and must be a string")
	}

	for _, a := range t.supportedAgents {
		if a.Name() == agentName {
			return map[string]any{
				"transferred_to": agentName,
			}, nil
		}
	}

	return nil, fmt.Errorf("transfer_to_agent: invalid agent name %q", agentName)
}

// RunWithContext sets TransferToAgent on the tool context's actions so
// the flow picks it up after execution.
func (t *TransferToAgentTool) RunWithContext(tc tool.ToolContext, args map[string]any) (map[string]any, error) {
	agentName, ok := args["agent_name"].(string)
	if !ok || agentName == "" {
		err := fmt.Errorf("transfer_to_agent: agent_name is required and must be a string")
		return nil, err
	}

	for _, a := range t.supportedAgents {
		if a.Name() == agentName {
			actions := tc.Actions()
			if actions != nil {
				actions.TransferToAgent = agentName
			}
			return map[string]any{
				"transferred_to": agentName,
			}, nil
		}
	}

	return nil, fmt.Errorf("transfer_to_agent: invalid agent name %q", agentName)
}

// TargetNames returns the list of allowed agent name targets.
func (t *TransferToAgentTool) TargetNames() []string {
	names := make([]string, len(t.supportedAgents))
	for i, a := range t.supportedAgents {
		names[i] = a.Name()
	}
	return names
}

var _ tool.Tool = (*TransferToAgentTool)(nil)
var _ tool.DeclarationProvider = (*TransferToAgentTool)(nil)
var _ tool.FunctionTool = (*TransferToAgentTool)(nil)
var _ tool.ContextFunctionTool = (*TransferToAgentTool)(nil)

// ComputeTransferTargets calculates the allowed transfer targets for an agent
// within the agent tree. It follows the same rules as ADK Go:
//
//  1. All sub-agents are always included.
//  2. The parent agent is included unless DisallowTransferToParent is true.
//  3. Peer agents are included unless DisallowTransferToPeers is true,
//     and only if the parent is an auto-flow agent (has sub-agents or
//     allows transfer).
func ComputeTransferTargets(a agent.Agent) []agent.Agent {
	targets := append([]agent.Agent(nil), a.SubAgents()...)

	parent := a.Parent()
	if parent == nil {
		return targets
	}

	if !a.DisallowTransferToParent() {
		targets = append(targets, parent)
	}

	if !a.DisallowTransferToPeers() && shouldAllowPeers(parent) {
		for _, peer := range parent.SubAgents() {
			if peer.Name() != a.Name() {
				targets = append(targets, peer)
			}
		}
	}

	return targets
}

func shouldAllowPeers(parent agent.Agent) bool {
	if len(parent.SubAgents()) > 0 {
		return true
	}
	if !parent.DisallowTransferToParent() {
		return true
	}
	if !parent.DisallowTransferToPeers() {
		return true
	}
	return false
}

// TransferInstructions generates instructions injected into the LLM's
// system prompt to inform it about available agents for transfer.
func TransferInstructions(curName string, targets []agent.Agent) string {
	if len(targets) == 0 {
		return ""
	}

	s := "You have a list of other agents to transfer to:\n\n"
	for _, t := range targets {
		s += fmt.Sprintf("Agent name: %s\nAgent description: %s\n\n", t.Name(), t.Description())
	}
	s += fmt.Sprintf(
		"If another agent is better for answering the question, call `%s` "+
			"function to transfer the question to that agent.\n"+
			"When transferring, do not generate any text other than the function call.\n",
		transferAgentName,
	)
	return s
}

// InjectTransferTool checks whether an agent should have the transfer tool,
// and if so, injects its declaration and instructions into the LLM request.
//
// It returns the TransferToAgentTool if one was created, or nil if transfer
// is not applicable (no agents to transfer to).
func InjectTransferTool(a agent.Agent, req *model.LLMRequest) *TransferToAgentTool {
	targets := ComputeTransferTargets(a)
	if len(targets) == 0 {
		return nil
	}

	tt := NewTransferToAgentTool(targets)

	if req.ToolDeclarations == nil {
		req.ToolDeclarations = make([]any, 0, 1)
	}
	req.ToolDeclarations = append(req.ToolDeclarations, tt.Declaration())

	instructions := TransferInstructions(a.Name(), targets)
	if instructions != "" {
		if req.SystemInstruction != "" {
			req.SystemInstruction += "\n\n" + instructions
		} else {
			req.SystemInstruction = instructions
		}
	}

	return tt
}
