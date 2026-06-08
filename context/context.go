// Package context defines the invocation context used throughout agent execution.
//
// The invocation context carries the current agent, session, user content,
// and lifecycle control (EndInvocation). It is the shared context object
// passed through all layers:
//
//	Runner -> Agent -> Flow -> Model/Tool -> Event -> Session
//
// An invocation starts with a user message and ends with a final response.
// It can contain multiple agent calls (via agent transfer) and multiple
// steps (LLM calls + tool calls) within each agent call.
package context

import (
	stdctx "context"
	"sync"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/session"
)

// InvocationContext is the central context passed through agent execution.
// It wraps a standard Go context and provides access to runtime resources.
type InvocationContext interface {
	stdctx.Context

	Agent() agent.Agent
	Session() session.Session
	InvocationID() string
	Branch() string
	UserContent() string

	EndInvocation()
	Ended() bool
}

// Params holds the constructor parameters for an InvocationContext.
type Params struct {
	Ctx          stdctx.Context
	Agent        agent.Agent
	Session      session.Session
	InvocationID string
	Branch       string
	UserContent  string
}

// NewInvocationContext creates a new invocation context from parameters.
func NewInvocationContext(p Params) InvocationContext {
	if p.Ctx == nil {
		p.Ctx = stdctx.Background()
	}
	return &invocationContext{
		Context:      p.Ctx,
		ag:           p.Agent,
		session:      p.Session,
		invocationID: p.InvocationID,
		branch:       p.Branch,
		userContent:  p.UserContent,
	}
}

type invocationContext struct {
	stdctx.Context

	ag           agent.Agent
	session      session.Session
	invocationID string
	branch       string
	userContent  string

	mu    sync.RWMutex
	ended bool
}

func (c *invocationContext) Agent() agent.Agent     { return c.ag }
func (c *invocationContext) Session() session.Session { return c.session }
func (c *invocationContext) InvocationID() string     { return c.invocationID }
func (c *invocationContext) Branch() string           { return c.branch }
func (c *invocationContext) UserContent() string      { return c.userContent }

func (c *invocationContext) EndInvocation() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ended = true
}

func (c *invocationContext) Ended() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ended
}
