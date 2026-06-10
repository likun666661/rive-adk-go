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
	"github.com/likun666661/rive-adk-go/artifact"
	"github.com/likun666661/rive-adk-go/callbackctx"
	"github.com/likun666661/rive-adk-go/memory"
	"github.com/likun666661/rive-adk-go/session"
)

// ReadonlyContext exposes readonly identity, session, user, app, and branch
// information to callbacks. It is a minimal read-only surface designed to
// prevent callbacks from calling EndInvocation() or accessing RunConfig().
//
// The canonical definition lives in callbackctx.ReadonlyContext.
type ReadonlyContext = callbackctx.ReadonlyContext

// CallbackContext is the unified callback context providing readonly information
// plus write-through state and artifact/memory service access.
//
// The canonical definition lives in callbackctx.CallbackContext.
type CallbackContext = callbackctx.CallbackContext

// ToolContext extends CallbackContext with tool-specific metadata.
//
// The canonical definition lives in callbackctx.ToolContext.
type ToolContext = callbackctx.ToolContext

// InvocationContext is the central context passed through agent execution.
// It wraps a standard Go context and provides access to runtime resources.
type InvocationContext interface {
	stdctx.Context

	Agent() agent.Agent
	RootAgent() agent.Agent
	Session() session.Session
	InvocationID() string
	Branch() string
	UserContent() string

	AgentName() string
	UserID() string
	AppName() string
	SessionID() string
	ReadonlyState() session.ReadonlyState

	MemoryService() memory.Service
	ArtifactService() artifact.Service

	EndInvocation()
	Ended() bool
}

// Params holds the constructor parameters for an InvocationContext.
type Params struct {
	Ctx          stdctx.Context
	Agent        agent.Agent
	RootAgent    agent.Agent
	Session      session.Session
	Memory       memory.Service
	Artifact     artifact.Service
	InvocationID string
	Branch       string
	UserContent  string
}

// NewInvocationContext creates a new invocation context from parameters.
// If RootAgent is nil, Agent is used as the root agent.
func NewInvocationContext(p Params) InvocationContext {
	if p.Ctx == nil {
		p.Ctx = stdctx.Background()
	}
	rootAgent := p.RootAgent
	if rootAgent == nil {
		rootAgent = p.Agent
	}
	return &invocationContext{
		Context:         p.Ctx,
		ag:              p.Agent,
		rootAgent:       rootAgent,
		session:         p.Session,
		memoryService:   p.Memory,
		artifactService: p.Artifact,
		invocationID:    p.InvocationID,
		branch:          p.Branch,
		userContent:     p.UserContent,
	}
}

type invocationContext struct {
	stdctx.Context

	ag              agent.Agent
	rootAgent       agent.Agent
	session         session.Session
	memoryService   memory.Service
	artifactService artifact.Service
	invocationID    string
	branch          string
	userContent     string

	mu    sync.RWMutex
	ended bool
}

func (c *invocationContext) Agent() agent.Agent              { return c.ag }
func (c *invocationContext) RootAgent() agent.Agent            { return c.rootAgent }
func (c *invocationContext) AgentName() string               { return c.ag.Name() }
func (c *invocationContext) Session() session.Session          { return c.session }
func (c *invocationContext) UserID() string                    { return c.session.UserID() }
func (c *invocationContext) AppName() string                   { return c.session.AppName() }
func (c *invocationContext) SessionID() string                 { return c.session.ID() }
func (c *invocationContext) MemoryService() memory.Service     { return c.memoryService }
func (c *invocationContext) ArtifactService() artifact.Service { return c.artifactService }
func (c *invocationContext) InvocationID() string              { return c.invocationID }
func (c *invocationContext) Branch() string                    { return c.branch }
func (c *invocationContext) UserContent() string               { return c.userContent }

func (c *invocationContext) ReadonlyState() session.ReadonlyState {
	return session.NewReadonlyState(c.session.State())
}

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
