// Package llmagent provides a minimal LLM-based agent that wraps flow.Flow.
//
// The LLMAgent is created via New and its Run function delegates to
// flow.Flow.Run, which orchestrates the model-call/tool-execution loop.
//
// Usage:
//
//	flow := &flow.Flow{
//	    Model: fakeModel,
//	    Tools: tools,
//	}
//	a := llmagent.New("my_agent", flow)
//	// a satisfies agent.Agent; use with runner.New to execute.
package llmagent

import (
	"fmt"

	"github.com/likun666661/rive-adk-go/agent"
	invctx "github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/flow"
)

// New creates an agent whose Run delegates to flow.Flow.Run.
//
// The returned agent can be used with runner.Runner to execute
// the full Agent → Flow → Event → Session chain.
func New(name, description string, f *flow.Flow) (agent.Agent, error) {
	if f == nil {
		return nil, fmt.Errorf("llmagent: flow is required")
	}
	a, err := agent.New(agent.Config{
		Name:        name,
		Description: description,
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			ic, ok := ctx.(invctx.InvocationContext)
			if !ok {
				return nil, fmt.Errorf("llmagent: expected context.InvocationContext, got %T", ctx)
			}
			return f.Run(ic)
		},
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}
