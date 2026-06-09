package remoteagent

import (
	stdctx "context"
	"fmt"

	"github.com/likun666661/rive-adk-go/agent"
	invctx "github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
)

// A2AClientProvider creates an A2AClient from an AgentCard.
//
// For real implementations this would use REST/gRPC factories.
// For tests this returns a FakeA2AClient.
type A2AClientProvider func(card AgentCard) (A2AClient, error)

// RemoteAgentConfig configures a RemoteAgent.
type RemoteAgentConfig struct {
	// Name is the agent's display name.
	Name string

	// Description is a human-readable description.
	Description string

	// AgentCard is the remote agent's capability card.
	AgentCard AgentCard

	// ClientProvider creates an A2AClient from the AgentCard.
	// If nil, NewRemoteAgent returns an error.
	ClientProvider A2AClientProvider

	// Converter converts RemoteEvents to session.Events.
	// If nil, DefaultConvertToSessionEvent is used.
	Converter Converter

	// CleanupCallbacks are invoked in order when a remote task needs cleanup.
	// Typical use: call CancelTask on the remote service.
	// Callbacks are invoked even if the remote task completed normally
	// (they can inspect lastState to decide whether cleanup is needed).
	CleanupCallbacks []CleanupCallback

	// RequestMetadata is optional metadata sent with every request.
	RequestMetadata map[string]string
}

// NewRemoteAgent creates an agent that communicates with a remote agent via A2A.
//
// The returned agent implements agent.Agent and can be used with workflows
// (workflow.SubAgent) and other orchestration.
func NewRemoteAgent(cfg RemoteAgentConfig) (*RemoteAgent, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("remoteagent: name is required")
	}
	if cfg.ClientProvider == nil {
		return nil, fmt.Errorf("remoteagent: ClientProvider is required")
	}

	return &RemoteAgent{
		name:             cfg.Name,
		description:      cfg.Description,
		agentCard:        cfg.AgentCard,
		clientProvider:   cfg.ClientProvider,
		converter:        cfg.Converter,
		cleanupCallbacks: cfg.CleanupCallbacks,
		requestMetadata:  cfg.RequestMetadata,
	}, nil
}

// RemoteAgent is an agent that communicates with a remote agent via A2A.
//
// It implements agent.Agent and workflow.SubAgent, allowing it to be used
// transparently in sequential/parallel/loop workflows.
type RemoteAgent struct {
	name             string
	description      string
	agentCard        AgentCard
	clientProvider   A2AClientProvider
	converter        Converter
	cleanupCallbacks []CleanupCallback
	requestMetadata  map[string]string
}

func (a *RemoteAgent) Name() string        { return a.name }
func (a *RemoteAgent) Description() string { return a.description }

// Execute sends a request to the remote agent and collects the response
// events. It implements the full lifecycle:
//
//  1. Create an A2AClient via ClientProvider.
//  2. Construct a SendMessageRequest from invocation context.
//  3. Stream remote events via SendStreamingMessage.
//  4. Convert each remote event to session.Event(s).
//  5. Aggregate partial events through the aggregator.
//  6. On error or context cancellation, invoke CleanupCallbacks.
func (a *RemoteAgent) Execute(ctx agent.InvocationContext) ([]*event.Event, error) {
	return a.execute(ctx)
}

func (a *RemoteAgent) execute(ctx agent.InvocationContext) ([]*event.Event, error) {
	// 1. Create client.
	client, err := a.clientProvider(a.agentCard)
	if err != nil {
		return nil, fmt.Errorf("remoteagent %q: create client: %w", a.name, err)
	}

	// 2. Construct request.
	req := SendMessageRequest{
		Streaming: a.agentCard.StreamingSupported,
		Metadata:  a.requestMetadata,
	}
	if parts, err := requestPartsFromContext(ctx); err != nil {
		return nil, fmt.Errorf("remoteagent %q: convert request: %w", a.name, err)
	} else {
		req.Parts = parts
	}

	// 3. Stream remote events.
	stream := client.SendStreamingMessage(req)

	// 4. Select converter.
	conv := a.converter
	if conv == nil {
		conv = DefaultConvertToSessionEvent
	}

	// 5. Process stream.
	aggregator := newAggregator()
	var allEvents []*event.Event
	var lastRemote *RemoteEvent
	var lastErr error

	done := doneChannel(ctx)
	for {
		select {
		case <-done:
			lastErr = contextError(ctx)
			goto streamDone
		case se, ok := <-stream:
			if !ok {
				goto streamDone
			}
			if se.Err != nil {
				lastErr = se.Err
				goto streamDone
			}
			if se.Event != nil {
				lastRemote = se.Event
			}
			converted, convErr := conv(se.Event)
			if convErr != nil {
				lastErr = fmt.Errorf("remoteagent %q: convert: %w", a.name, convErr)
				goto streamDone
			}

			// Aggregate partial events.
			toEmit := aggregator.process(se.Event, converted)
			allEvents = append(allEvents, toEmit...)
		}
	}

streamDone:
	// 6. Invoke cleanup callbacks.
	cleanupErr := a.invokeCleanupCallbacks(lastRemote, lastErr)
	destroyErr := client.Destroy()

	// 7. Combine errors: stream/convert error takes priority over cleanup error,
	//    which takes priority over client destroy errors.
	if lastErr != nil {
		return allEvents, lastErr
	}
	if cleanupErr != nil {
		return allEvents, cleanupErr
	}
	if destroyErr != nil {
		return allEvents, fmt.Errorf("remoteagent %q: destroy client: %w", a.name, destroyErr)
	}

	// 8. If the stream ended without a terminal status, ensure the aggregator
	//    is flushed and emit a final event.
	if lastRemote == nil || (lastRemote.Type == RemoteEventTaskStatusUpdate && !lastRemote.State.IsTerminal()) {
		allEvents = append(allEvents, aggregator.flush()...)
	}

	return allEvents, nil
}

func requestPartsFromContext(ctx agent.InvocationContext) ([]RemotePart, error) {
	ic, ok := ctx.(invctx.InvocationContext)
	if !ok || ic.UserContent() == "" {
		return nil, nil
	}

	ev := event.NewEvent(
		fmt.Sprintf("%s-user-request", ic.InvocationID()),
		"user",
		event.RoleUser,
	)
	ev.Content = &event.Content{
		Role: event.RoleUser,
		Parts: []event.Part{
			{Text: ic.UserContent()},
		},
	}
	return ConvertSessionEventToRemote(ev)
}

func doneChannel(ctx agent.InvocationContext) <-chan struct{} {
	if c, ok := ctx.(stdctx.Context); ok {
		return c.Done()
	}
	return nil
}

func contextError(ctx agent.InvocationContext) error {
	if c, ok := ctx.(stdctx.Context); ok {
		return c.Err()
	}
	return nil
}

// invokeCleanupCallbacks calls each CleanupCallback in order.
//
// Parameters:
//   - lastRemote: the last remote event received (nil if none).
//   - reason: the error that triggered cleanup (nil if clean shutdown).
//
// If a CleanupCallback returns an error, subsequent callbacks are still invoked
// but the first error is returned.
func (a *RemoteAgent) invokeCleanupCallbacks(lastRemote *RemoteEvent, reason error) error {
	if len(a.cleanupCallbacks) == 0 {
		return nil
	}

	var taskID string
	var lastState TaskState
	if lastRemote != nil {
		taskID = lastRemote.TaskID
		lastState = lastRemote.State
	}

	var firstErr error
	for i, cb := range a.cleanupCallbacks {
		if cb == nil {
			continue
		}
		if err := cb(taskID, lastState, reason); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("remoteagent %q: cleanup callback[%d]: %w", a.name, i, err)
			}
		}
	}

	return firstErr
}
