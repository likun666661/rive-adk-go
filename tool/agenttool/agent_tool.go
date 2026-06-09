// Package agenttool provides a tool that wraps an agent and exposes it
// as a callable FunctionTool. The wrapped agent runs in an isolated child
// session (sandbox) with non-internal parent state copied in.
package agenttool

import (
	"fmt"
	"strings"
	"time"

	stdctx "context"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/artifact"
	"github.com/likun666661/rive-adk-go/memory"
	"github.com/likun666661/rive-adk-go/runner"
	"github.com/likun666661/rive-adk-go/tool"
)

const internalStatePrefix = "_adk"

// agentTool wraps an agent so a parent flow can invoke it as a tool.
type agentTool struct {
	agent             agent.Agent
	skipSummarization bool
}

// Config holds the configuration for an agent tool.
type Config struct {
	SkipSummarization bool
}

// New creates a new agent tool that wraps the given agent.
func New(a agent.Agent, cfg *Config) tool.Tool {
	at := &agentTool{agent: a}
	if cfg != nil {
		at.skipSummarization = cfg.SkipSummarization
	}
	return at
}

func (t *agentTool) Name() string        { return t.agent.Name() }
func (t *agentTool) Description() string { return t.agent.Description() }
func (t *agentTool) IsLongRunning() bool { return false }

func (t *agentTool) Declaration() tool.Declaration {
	return tool.Declaration{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"request": map[string]any{"type": "string"},
			},
			"required": []any{"request"},
		},
	}
}

func (t *agentTool) Run(args map[string]any) (map[string]any, error) {
	return t.runWithContext(nil, args)
}

func (t *agentTool) RunWithContext(tc tool.ToolContext, args map[string]any) (map[string]any, error) {
	if t.skipSummarization {
		if actions := tc.Actions(); actions != nil {
			actions.SkipSummarization = true
		}
	}
	return t.runWithContext(tc, args)
}

func (t *agentTool) runWithContext(tc tool.ToolContext, args map[string]any) (map[string]any, error) {
	input, ok := args["request"]
	if !ok {
		return nil, fmt.Errorf("agenttool: missing required argument 'request' for agent %q", t.agent.Name())
	}
	inputText, ok := input.(string)
	if !ok {
		inputText = fmt.Sprint(input)
	}

	execAgent, ok := t.agent.(runner.ExecutableAgent)
	if !ok {
		return nil, fmt.Errorf("agenttool: agent %q does not implement runner.ExecutableAgent", t.agent.Name())
	}

	sessionService := runner.NewInMemorySessionService()
	artifactService := artifact.InMemoryService()
	memoryService := memory.InMemoryService()

	r, err := runner.New(runner.Config{
		AppName:         t.agent.Name(),
		Agent:           execAgent,
		SessionService:  sessionService,
		ArtifactService: artifactService,
		MemoryService:   memoryService,
	})
	if err != nil {
		return nil, fmt.Errorf("agenttool: failed to create runner: %w", err)
	}

	childUserID := "agenttool-user"
	var ctx stdctx.Context = stdctx.Background()

	if tc != nil {
		ic := tc.InvocationContext()
		if ic != nil {
			childUserID = ic.UserID()
			ctx = ic
		}
	}

	childSessionID := fmt.Sprintf("%s-agenttool-%d", t.agent.Name(), time.Now().UnixNano())

	sess, err := sessionService.Create(ctx, t.agent.Name(), childUserID, childSessionID)
	if err != nil {
		return nil, fmt.Errorf("agenttool: failed to create child session: %w", err)
	}

	if tc != nil {
		ic := tc.InvocationContext()
		if ic != nil {
			parentSession := ic.Session()
			if parentSession != nil {
				for k, v := range parentSession.State().All() {
					if !strings.HasPrefix(k, internalStatePrefix) {
						sess.State().Set(k, v)
					}
				}
			}
		}
	}

	sess, events, err := r.Run(ctx, childUserID, sess.ID(), inputText)
	if err != nil {
		return nil, fmt.Errorf("agenttool: sub-agent %q execution error: %w", t.agent.Name(), err)
	}

	_ = sess

	var lastText string
	for _, ev := range events {
		if ev.ErrorCode != "" || ev.ErrorMessage != "" {
			return nil, fmt.Errorf("agenttool: error from sub-agent %q (code: %q, message: %q)", t.agent.Name(), ev.ErrorCode, ev.ErrorMessage)
		}
		if ev.Content != nil {
			var texts []string
			for _, part := range ev.Content.Parts {
				if part.Text != "" {
					texts = append(texts, part.Text)
				}
			}
			if len(texts) > 0 {
				lastText = strings.Join(texts, "\n")
			}
		}
	}

	if lastText == "" {
		return map[string]any{}, nil
	}

	return map[string]any{"result": lastText}, nil
}

var _ tool.Tool = (*agentTool)(nil)
var _ tool.DeclarationProvider = (*agentTool)(nil)
var _ tool.FunctionTool = (*agentTool)(nil)
var _ tool.ContextFunctionTool = (*agentTool)(nil)
