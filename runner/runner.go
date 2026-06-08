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
	"sync"

	"github.com/likun666661/rive-adk-go/agent"
	invctx "github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
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

// InMemorySessionService is a simple in-memory session store.
type InMemorySessionService struct {
	mu       sync.RWMutex
	sessions map[string]session.Session
}

// NewInMemorySessionService creates a new InMemorySessionService.
func NewInMemorySessionService() *InMemorySessionService {
	return &InMemorySessionService{
		sessions: make(map[string]session.Session),
	}
}

func (s *InMemorySessionService) key(appName, userID, sessionID string) string {
	return appName + "/" + userID + "/" + sessionID
}

func (s *InMemorySessionService) Get(ctx stdctx.Context, appName, userID, sessionID string) (session.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[s.key(appName, userID, sessionID)]
	if !ok {
		return nil, fmt.Errorf("runner: session %q not found", sessionID)
	}
	return sess, nil
}

func (s *InMemorySessionService) Create(ctx stdctx.Context, appName, userID, sessionID string) (session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.key(appName, userID, sessionID)
	if existing, ok := s.sessions[key]; ok {
		return existing, nil
	}
	sess := session.NewInMemorySession(sessionID, appName, userID)
	s.sessions[key] = sess
	return sess, nil
}

// Runner orchestrates the Agent → Flow → Event → Session chain.
type Runner struct {
	appName        string
	agent          ExecutableAgent
	sessionService SessionService
}

// Config holds configuration for creating a new Runner.
type Config struct {
	AppName        string
	Agent          ExecutableAgent
	SessionService SessionService
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
		appName:        cfg.AppName,
		agent:          cfg.Agent,
		sessionService: cfg.SessionService,
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
