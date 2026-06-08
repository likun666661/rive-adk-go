// Package runner implements the top-level execution orchestrator.
//
// The Runner ties together sessions, agents, and invocations:
//
//	Runner.Run:
//	  1. Get or create session
//	  2. Create user event and append to session
//	  3. Create InvocationContext
//	  4. Execute agent
//	  5. Persist non-partial events to session
//	  6. Yield all events to caller
//
// The Runner is the entry point for user interactions. It manages
// session lifecycle and ensures events are properly persisted.
package runner

import (
	stdctx "context"
	stderrors "errors"
	"fmt"

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
//  2. Create a user event and append it to the session
//  3. Build an InvocationContext
//  4. Execute the agent (agent.Execute → Run → Flow.Run → ...)
//  5. Persist non-partial events to the session
//  6. Return the session and all events
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

	ic := invctx.NewInvocationContext(invctx.Params{
		Ctx:          ctx,
		Agent:        r.agent,
		Session:      sess,
		Memory:       r.memoryService,
		Artifact:     r.artifactService,
		InvocationID: invocationID,
		Branch:       r.agent.Name(),
		UserContent:  message,
	})

	sessEvents, err := r.agent.Execute(ic)
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
