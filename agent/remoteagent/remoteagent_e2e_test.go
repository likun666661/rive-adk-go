package remoteagent

import (
	stdctx "context"
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/runner"
)

// =============================================================================
// Chapter 05 — End-to-end: Remote A2A streaming aggregation through runner
//
// Demonstrates the full pipeline: RemoteAgent → Runner.Run → aggregated events.
// =============================================================================

// ---------------------------------------------------------------------------
// E2E: Remote A2A streaming aggregation through runner.Run()
// ---------------------------------------------------------------------------

func TestRemoteAgent_StreamingAggregation_ThroughRunner(t *testing.T) {
	cfg := FakeA2AClientConfig{
		Card: AgentCard{Name: "remote-kb", Description: "Knowledge base agent", StreamingSupported: true},
		Events: []StreamEvent{
			// Streaming partial chunks simulating A2A artifact updates.
			{Event: &RemoteEvent{
				Type:      RemoteEventTaskArtifactUpdate,
				TaskID:    "task-1",
				Parts:     []RemotePart{{Text: "The capital "}},
				Append:    true,
				LastChunk: false,
			}},
			{Event: &RemoteEvent{
				Type:      RemoteEventTaskArtifactUpdate,
				TaskID:    "task-1",
				Parts:     []RemotePart{{Text: "of France "}},
				Append:    true,
				LastChunk: false,
			}},
			{Event: &RemoteEvent{
				Type:      RemoteEventTaskArtifactUpdate,
				TaskID:    "task-1",
				Parts:     []RemotePart{{Text: "is Paris."}},
				Append:    true,
				LastChunk: true,
			}},
			// Terminal status.
			{Event: &RemoteEvent{
				Type:   RemoteEventTaskStatusUpdate,
				TaskID: "task-1",
				State:  TaskStateCompleted,
			}},
		},
	}

	remoteAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "kb-agent",
		Description: "Remote knowledge base",
		AgentCard:   cfg.Card,
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return NewFakeA2AClient(cfg), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Ensure RemoteAgent implements agent.Agent.
	var _ agent.Agent = remoteAgent
	// Ensure RemoteAgent implements runner.ExecutableAgent.
	var _ runner.ExecutableAgent = remoteAgent

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "e2e_remote",
		Agent:          remoteAgent,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-e2e-a2a", "What is the capital of France?")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	_ = sess

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events (aggregated text + terminal status), got %d", len(events))
	}

	// Find the aggregated text event (non-partial, with content, not a status event).
	var textEvent *event.Event
	for _, ev := range events {
		if ev.Content != nil && len(ev.Content.Parts) > 0 && ev.Content.Parts[0].Text != "" && !ev.Partial {
			isStatus := ev.Actions.StateDelta != nil && ev.Actions.StateDelta["remote_task_state"] != nil
			if !isStatus {
				textEvent = ev
				break
			}
		}
	}
	if textEvent == nil {
		// Fallback: any content event.
		for _, ev := range events {
			if ev.Content != nil && len(ev.Content.Parts) > 0 && ev.Content.Parts[0].Text != "" {
				textEvent = ev
				break
			}
		}
	}

	if textEvent == nil {
		t.Fatal("expected an aggregated text event with content")
	}
	if textEvent.Partial {
		t.Error("aggregated text event should not be partial")
	}

	combined := textEvent.Content.Parts[0].Text
	if !strings.Contains(combined, "capital") {
		t.Errorf("aggregated text should contain 'capital', got %q", combined)
	}
	if !strings.Contains(combined, "Paris") {
		t.Errorf("aggregated text should contain 'Paris', got %q", combined)
	}

	// Verify terminal status event is present.
	foundTerminal := false
	for _, ev := range events {
		if ev.Actions.StateDelta != nil {
			if state, ok := ev.Actions.StateDelta["remote_task_state"]; ok && state == string(TaskStateCompleted) {
				foundTerminal = true
				break
			}
		}
	}
	if !foundTerminal {
		t.Error("expected terminal completed status event")
	}
}

func TestRemoteAgent_RequestConversionAndClientDestroy_ThroughRunner(t *testing.T) {
	client := &recordingA2AClient{
		events: []StreamEvent{
			{Event: &RemoteEvent{
				Type:   RemoteEventMessage,
				TaskID: "task-request",
				Parts:  []RemotePart{{Text: "converted"}},
			}},
			{Event: &RemoteEvent{
				Type:   RemoteEventTaskStatusUpdate,
				TaskID: "task-request",
				State:  TaskStateCompleted,
			}},
		},
	}

	remoteAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:      "request-agent",
		AgentCard: AgentCard{Name: "remote-request", StreamingSupported: true},
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return client, nil
		},
		RequestMetadata: map[string]string{"trace": "yes"},
	})
	if err != nil {
		t.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:        "e2e_request",
		Agent:          remoteAgent,
		SessionService: runner.NewInMemorySessionService(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = r.Run(stdctx.Background(), "user-1", "sess-e2e-request", "Please answer remotely")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(client.request.Parts) != 1 {
		t.Fatalf("expected 1 converted request part, got %d", len(client.request.Parts))
	}
	if client.request.Parts[0].Text != "Please answer remotely" {
		t.Errorf("request text = %q, want %q", client.request.Parts[0].Text, "Please answer remotely")
	}
	if !client.request.Streaming {
		t.Error("expected streaming request when agent card supports streaming")
	}
	if client.request.Metadata["trace"] != "yes" {
		t.Errorf("metadata trace = %q, want yes", client.request.Metadata["trace"])
	}
	if !client.destroyed {
		t.Error("expected RemoteAgent to destroy client after execution")
	}
}

// ---------------------------------------------------------------------------
// E2E: Remote A2A cleanup callback invoked on stream error
// ---------------------------------------------------------------------------

func TestRemoteAgent_Cleanup_ThroughRunner(t *testing.T) {
	var cleanupCalled bool
	cleanup := func(taskID string, lastState TaskState, reason error) error {
		cleanupCalled = true
		return nil
	}

	cfg := FakeA2AClientConfig{
		Card: AgentCard{Name: "unstable", StreamingSupported: true},
		Events: []StreamEvent{
			{Event: &RemoteEvent{
				Type:   RemoteEventMessage,
				TaskID: "task-err",
				Parts:  []RemotePart{{Text: "processing..."}},
			}},
			{Err: &fakeStreamError{msg: "remote disconnected"}},
		},
	}

	remoteAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "unstable-agent",
		Description: "Unstable remote agent",
		AgentCard:   cfg.Card,
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return NewFakeA2AClient(cfg), nil
		},
		CleanupCallbacks: []CleanupCallback{cleanup},
	})
	if err != nil {
		t.Fatal(err)
	}

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "e2e_cleanup",
		Agent:          remoteAgent,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = r.Run(stdctx.Background(), "user-1", "sess-e2e-cleanup", "Do something")
	if err == nil {
		t.Error("expected error from stream disconnection")
	}
	if !cleanupCalled {
		t.Error("expected cleanup callback to be invoked on stream error")
	}
}

// fakeStreamError is a simple error type for testing stream errors.
type fakeStreamError struct{ msg string }

func (e *fakeStreamError) Error() string { return e.msg }

type recordingA2AClient struct {
	request   SendMessageRequest
	events    []StreamEvent
	destroyed bool
}

func (c *recordingA2AClient) SendStreamingMessage(req SendMessageRequest) <-chan StreamEvent {
	c.request = req
	ch := make(chan StreamEvent, len(c.events))
	go func() {
		for _, ev := range c.events {
			ch <- ev
		}
		close(ch)
	}()
	return ch
}

func (c *recordingA2AClient) CancelTask(taskID string) error {
	return nil
}

func (c *recordingA2AClient) Destroy() error {
	c.destroyed = true
	return nil
}
