// Package runner implements the top-level execution orchestrator.
//
// The Runner ties together sessions, agents, and invocations:
//
//	Runner.Run:
//	  1. Get or create session
//	  2. Find the active agent from session history (or fall back to root)
//	  3. Create user event and append to session
//	  4. Create InvocationContext
//	  5. Execute agent
//	  6. Persist non-partial events to session
//	  7. Yield all events to caller
//
// The Runner is the entry point for user interactions. It manages
// session lifecycle and ensures events are properly persisted.
//
// Agent tree routing:
//
// After a TransferToAgent action, subsequent user messages in the same
// session are routed to the active agent implied by session history.
// The runner scans events backwards, skipping user-author events, and
// finds the first agent whose full ancestor chain allows transfer.
package runner

import (
	stdctx "context"
	stderrors "errors"
	"fmt"
	"log"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/artifact"
	invctx "github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/memory"
	"github.com/likun666661/rive-adk-go/session"
)

// ExecutableAgent is an agent that can be executed within an invocation.
type ExecutableAgent interface {
	agent.Agent
	Execute(ctx agent.InvocationContext) ([]*event.Event, error)
}

// SessionService abstracts session retrieval and creation.
type SessionService interface {
	Get(ctx stdctx.Context, appName, userID, sessionID string) (session.Session, error)
	Create(ctx stdctx.Context, appName, userID, sessionID string) (session.Session, error)
}

// InMemorySessionService is a scope-aware session store backed by session.Service.
type InMemorySessionService struct {
	svc *session.Service
}

// NewInMemorySessionService creates a new scope-aware InMemorySessionService.
func NewInMemorySessionService() *InMemorySessionService {
	return &InMemorySessionService{
		svc: session.NewService(),
	}
}

func (s *InMemorySessionService) Get(ctx stdctx.Context, appName, userID, sessionID string) (session.Session, error) {
	return s.svc.Get(appName, userID, sessionID)
}

func (s *InMemorySessionService) Create(ctx stdctx.Context, appName, userID, sessionID string) (session.Session, error) {
	return s.svc.GetOrCreate(appName, userID, sessionID)
}

// GetMergedState returns the fully merged app+user+session state for a session.
func (s *InMemorySessionService) GetMergedState(appName, userID, sessionID string) (map[string]any, error) {
	return s.svc.GetMergedState(appName, userID, sessionID)
}

// Runner orchestrates the Agent → Flow → Event → Session chain.
type Runner struct {
	appName         string
	agent           ExecutableAgent
	sessionService  SessionService
	memoryService   memory.Service
	artifactService artifact.Service
}

// Config holds configuration for creating a new Runner.
type Config struct {
	AppName         string
	Agent           ExecutableAgent
	SessionService  SessionService
	MemoryService   memory.Service
	ArtifactService artifact.Service
}

// New creates a new Runner.
func New(cfg Config) (*Runner, error) {
	if cfg.AppName == "" {
		return nil, fmt.Errorf("runner: AppName is required")
	}
	if cfg.Agent == nil {
		return nil, fmt.Errorf("runner: Agent is required")
	}
	if cfg.SessionService == nil {
		return nil, fmt.Errorf("runner: SessionService is required")
	}
	return &Runner{
		appName:         cfg.AppName,
		agent:           cfg.Agent,
		sessionService:  cfg.SessionService,
		memoryService:   cfg.MemoryService,
		artifactService: cfg.ArtifactService,
	}, nil
}

// Run executes the full chain for a single user message.
//
// Flow:
//  1. Get or create the session
//  2. Find the active agent from session history
//  3. Create a user event and append it to the session
//  4. Build an InvocationContext
//  5. Execute the active agent (agent.Execute → Run → Flow.Run → ...)
//  6. Persist non-partial events to the session
//  7. Return the session and all events
func (r *Runner) Run(ctx stdctx.Context, userID, sessionID, message string) (session.Session, []*event.Event, error) {
	sess, err := r.sessionService.Get(ctx, r.appName, userID, sessionID)
	if err != nil {
		sess, err = r.sessionService.Create(ctx, r.appName, userID, sessionID)
		if err != nil {
			return nil, nil, fmt.Errorf("runner: failed to create session: %w", err)
		}
	}

	nextEventOrdinal := sess.EventCount() + 1
	invocationID := fmt.Sprintf("%s-inv-%d", sess.ID(), nextEventOrdinal)

	userEvent := event.NewEvent(
		fmt.Sprintf("%s-user-%d", sess.ID(), nextEventOrdinal),
		"user",
		event.RoleUser,
	)
	userEvent.Branch = r.agent.Name()
	userEvent.Content = &event.Content{
		Role: event.RoleUser,
		Parts: []event.Part{
			{Text: message},
		},
	}

	if err := sess.AppendEvent(userEvent); err != nil {
		return nil, nil, fmt.Errorf("runner: failed to append user event: %w", err)
	}

	agentToRun := r.findAgentToRun(sess)
	ea, ok := agentToRun.(ExecutableAgent)
	if !ok {
		agentToRun = r.agent
		ea = r.agent
	}

	ic := invctx.NewInvocationContext(invctx.Params{
		Ctx:          ctx,
		Agent:        agentToRun,
		RootAgent:    r.agent,
		Session:      sess,
		Memory:       r.memoryService,
		Artifact:     r.artifactService,
		InvocationID: invocationID,
		Branch:       agentToRun.Name(),
		UserContent:  message,
	})

	sessEvents, err := ea.Execute(ic)
	if err != nil {
		return sess, sessEvents, fmt.Errorf("runner: agent execution error: %w", err)
	}

	for _, ev := range sessEvents {
		if ev.Partial {
			continue
		}
		if appendErr := sess.AppendEvent(ev); appendErr != nil {
			err = stderrors.Join(err, fmt.Errorf("runner: failed to append event %q: %w", ev.ID, appendErr))
		}
	}

	return sess, sessEvents, err
}

// findAgentToRun determines which agent should handle the next user message
// by scanning session history for the last non-user event authored by a
// transferable agent.
func (r *Runner) findAgentToRun(sess session.Session) agent.Agent {
	events := sess.Events()
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev == nil {
			continue
		}
		if ev.Author == "user" {
			continue
		}

		candidate := r.agent.FindAgent(ev.Author)
		if candidate == nil {
			log.Printf("Event from unknown agent: %s, event id: %s", ev.Author, ev.ID)
			continue
		}

		if r.isTransferableAcrossAgentTree(candidate) {
			return candidate
		}
	}

	return r.agent
}

// isTransferableAcrossAgentTree checks if the given agent and all its ancestors
// allow transfer up the tree. An agent is transferable only if every ancestor
// in the chain (including itself) has DisallowTransferToParent == false.
func (r *Runner) isTransferableAcrossAgentTree(a agent.Agent) bool {
	for cur := a; cur != nil; cur = cur.Parent() {
		if cur.DisallowTransferToParent() {
			return false
		}
	}
	return true
}
