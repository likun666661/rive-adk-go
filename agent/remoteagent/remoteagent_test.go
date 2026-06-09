package remoteagent

import (
	"errors"
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/event"
)

// ---------------------------------------------------------------------------
// Test: FakeA2AClient
// ---------------------------------------------------------------------------

func TestFakeA2AClient_SendStreamingMessage(t *testing.T) {
	cfg := FakeA2AClientConfig{
		Card: AgentCard{Name: "test-remote", StreamingSupported: true},
		Events: []StreamEvent{
			{Event: &RemoteEvent{Type: RemoteEventMessage, TaskID: "t1", Parts: []RemotePart{{Text: "hello"}}}},
			{Event: &RemoteEvent{Type: RemoteEventTaskStatusUpdate, TaskID: "t1", State: TaskStateCompleted}},
		},
	}
	client := NewFakeA2AClient(cfg)
	if client == nil {
		t.Fatal("NewFakeA2AClient returned nil")
	}
	if client.AgentCard().Name != "test-remote" {
		t.Errorf("AgentCard name = %q, want 'test-remote'", client.AgentCard().Name)
	}

	stream := client.SendStreamingMessage(SendMessageRequest{})
	events := collectStream(t, stream)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Parts[0].Text != "hello" {
		t.Errorf("first event text = %q, want 'hello'", events[0].Parts[0].Text)
	}
	if events[1].State != TaskStateCompleted {
		t.Errorf("second event state = %q, want 'completed'", events[1].State)
	}
}

func TestFakeA2AClient_CancelTask(t *testing.T) {
	var recordCancels []string
	cfg := FakeA2AClientConfig{
		RecordCancels: &recordCancels,
	}
	client := NewFakeA2AClient(cfg)

	if err := client.CancelTask("task-1"); err != nil {
		t.Fatalf("CancelTask failed: %v", err)
	}
	if !client.IsCanceled("task-1") {
		t.Error("expected task-1 to be canceled")
	}
	if len(recordCancels) != 1 || recordCancels[0] != "task-1" {
		t.Errorf("recordCancels = %v, want ['task-1']", recordCancels)
	}

	if err := client.CancelTask("task-2"); err != nil {
		t.Fatalf("CancelTask failed: %v", err)
	}
	if client.CancelCount() != 2 {
		t.Errorf("CancelCount = %d, want 2", client.CancelCount())
	}
}

func TestFakeA2AClient_CancelTaskError(t *testing.T) {
	cfg := FakeA2AClientConfig{
		CancelError: errors.New("cancel failed"),
	}
	client := NewFakeA2AClient(cfg)

	if err := client.CancelTask("task-1"); err == nil {
		t.Error("expected error from CancelTask")
	}
}

func TestFakeA2AClient_CancelTaskMaxCancels(t *testing.T) {
	cfg := FakeA2AClientConfig{
		MaxCancels: 1,
	}
	client := NewFakeA2AClient(cfg)

	if err := client.CancelTask("task-1"); err != nil {
		t.Fatalf("first CancelTask failed: %v", err)
	}
	if err := client.CancelTask("task-2"); err == nil {
		t.Error("expected error on second CancelTask")
	}
}

func TestFakeA2AClient_Destroy(t *testing.T) {
	cfg := FakeA2AClientConfig{}
	client := NewFakeA2AClient(cfg)

	if err := client.Destroy(); err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}
	if !client.IsDestroyed() {
		t.Error("expected client to be destroyed")
	}

	// Subsequent calls should fail.
	if err := client.CancelTask("task-1"); err == nil {
		t.Error("expected error from CancelTask on destroyed client")
	}

	stream := client.SendStreamingMessage(SendMessageRequest{})
	ev := <-stream
	if ev.Err == nil {
		t.Error("expected error from SendStreamingMessage on destroyed client")
	}
}

// ---------------------------------------------------------------------------
// Test: ConvertToSessionEvent - content events
// ---------------------------------------------------------------------------

func TestConvertToSessionEvent_Message(t *testing.T) {
	remote := &RemoteEvent{
		Type:   RemoteEventMessage,
		TaskID: "t1",
		Parts: []RemotePart{
			{Text: "hi there"},
		},
	}
	events, err := DefaultConvertToSessionEvent(remote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Author != "remoteagent" {
		t.Errorf("author = %q, want 'remoteagent'", ev.Author)
	}
	if ev.Content == nil {
		t.Fatal("event content is nil")
	}
	if ev.Content.Parts[0].Text != "hi there" {
		t.Errorf("text = %q, want 'hi there'", ev.Content.Parts[0].Text)
	}
	if ev.Partial {
		t.Error("non-append message should not be partial")
	}
}

func TestConvertToSessionEvent_MessageWithFunctionCall(t *testing.T) {
	remote := &RemoteEvent{
		Type:   RemoteEventMessage,
		TaskID: "t1",
		Parts: []RemotePart{
			{Text: "let me search"},
			{
				FunctionCall: &RemoteFunctionCall{
					ID:   "fc-1",
					Name: "search",
					Args: map[string]any{"query": "golang"},
				},
			},
		},
	}
	events, err := DefaultConvertToSessionEvent(remote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if len(ev.Content.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(ev.Content.Parts))
	}
	if ev.Content.Parts[1].FunctionCall == nil {
		t.Fatal("expected function call part")
	}
	if ev.Content.Parts[1].FunctionCall.Name != "search" {
		t.Errorf("function name = %q, want 'search'", ev.Content.Parts[1].FunctionCall.Name)
	}
}

func TestConvertToSessionEvent_MessageAppendPartial(t *testing.T) {
	remote := &RemoteEvent{
		Type:      RemoteEventMessage,
		TaskID:    "t1",
		Parts:     []RemotePart{{Text: "chunk "}},
		Append:    true,
		LastChunk: false,
	}
	ev, err := DefaultConvertToSessionEvent(remote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ev[0].Partial {
		t.Error("append-only message should be partial")
	}
}

func TestConvertToSessionEvent_MessageAppendLastChunk(t *testing.T) {
	remote := &RemoteEvent{
		Type:      RemoteEventMessage,
		TaskID:    "t1",
		Parts:     []RemotePart{{Text: "final chunk"}},
		Append:    true,
		LastChunk: true,
	}
	ev, err := DefaultConvertToSessionEvent(remote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev[0].Partial {
		t.Error("append+last-chunk message should not be partial")
	}
}

// ---------------------------------------------------------------------------
// Test: ConvertToSessionEvent - status events
// ---------------------------------------------------------------------------

func TestConvertToSessionEvent_TaskStatusSubmitted(t *testing.T) {
	remote := &RemoteEvent{
		Type:   RemoteEventTaskStatusUpdate,
		TaskID: "t1",
		State:  TaskStateSubmitted,
	}
	ev, err := DefaultConvertToSessionEvent(remote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ev[0].Partial {
		t.Error("non-terminal status should be partial")
	}
	if ev[0].Actions.StateDelta == nil {
		t.Fatal("expected state delta")
	}
	if ev[0].Actions.StateDelta["remote_task_state"] != string(TaskStateSubmitted) {
		t.Errorf("remote_task_state = %q, want 'submitted'", ev[0].Actions.StateDelta["remote_task_state"])
	}
}

func TestConvertToSessionEvent_TaskStatusCompleted(t *testing.T) {
	remote := &RemoteEvent{
		Type:   RemoteEventTaskStatusUpdate,
		TaskID: "t1",
		State:  TaskStateCompleted,
	}
	ev, err := DefaultConvertToSessionEvent(remote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev[0].Partial {
		t.Error("terminal status should not be partial")
	}
	if !ev[0].TurnComplete {
		t.Error("terminal status should mark TurnComplete")
	}
}

func TestConvertToSessionEvent_Error(t *testing.T) {
	sentinel := errors.New("remote failure")
	remote := &RemoteEvent{
		Type:         RemoteEventTaskStatusUpdate,
		TaskID:       "t1",
		State:        TaskStateFailed,
		Error:        sentinel,
		ErrorCode:    "REMOTE_ERROR",
		ErrorMessage: "something broke",
	}
	ev, err := DefaultConvertToSessionEvent(remote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev[0].Error != sentinel {
		t.Error("expected propagated error")
	}
	if ev[0].ErrorCode != "REMOTE_ERROR" {
		t.Errorf("ErrorCode = %q, want 'REMOTE_ERROR'", ev[0].ErrorCode)
	}
}

func TestConvertToSessionEvent_Nil(t *testing.T) {
	_, err := DefaultConvertToSessionEvent(nil)
	if err == nil {
		t.Error("expected error for nil event")
	}
}

// ---------------------------------------------------------------------------
// Test: ConvertSessionEventToRemote
// ---------------------------------------------------------------------------

func TestConvertSessionEventToRemote_Text(t *testing.T) {
	ev := event.NewEvent("e1", "user", event.RoleUser)
	ev.Content = &event.Content{
		Role:  event.RoleUser,
		Parts: []event.Part{{Text: "hello remote"}},
	}
	parts, err := ConvertSessionEventToRemote(ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0].Text != "hello remote" {
		t.Errorf("text = %q, want 'hello remote'", parts[0].Text)
	}
}

func TestConvertSessionEventToRemote_FunctionCall(t *testing.T) {
	ev := event.NewEvent("e1", "model", event.RoleModel)
	ev.Content = &event.Content{
		Role: event.RoleModel,
		Parts: []event.Part{
			{
				FunctionCall: &event.FunctionCall{
					ID:   "fc-1",
					Name: "get_weather",
					Args: map[string]any{"city": "paris"},
				},
			},
		},
	}
	parts, err := ConvertSessionEventToRemote(ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0].FunctionCall == nil {
		t.Fatal("expected function call in remote part")
	}
	if parts[0].FunctionCall.Name != "get_weather" {
		t.Errorf("function name = %q, want 'get_weather'", parts[0].FunctionCall.Name)
	}
}

func TestConvertSessionEventToRemote_Nil(t *testing.T) {
	_, err := ConvertSessionEventToRemote(nil)
	if err == nil {
		t.Error("expected error for nil event")
	}
}

func TestConvertSessionEventToRemote_NilContent(t *testing.T) {
	ev := event.NewEvent("e1", "user", event.RoleUser)
	ev.Content = nil
	parts, err := ConvertSessionEventToRemote(ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parts) != 0 {
		t.Errorf("expected 0 parts for nil content, got %d", len(parts))
	}
}

// ---------------------------------------------------------------------------
// Test: Partial aggregation
// ---------------------------------------------------------------------------

func TestAggregator_EmptyStream(t *testing.T) {
	ag := newAggregator()
	flushed := ag.flush()
	if len(flushed) != 0 {
		t.Errorf("expected empty flush, got %d events", len(flushed))
	}
}

func TestAggregator_SingleNonAppendMessage(t *testing.T) {
	ag := newAggregator()
	remote := &RemoteEvent{
		Type:   RemoteEventMessage,
		TaskID: "t1",
		Parts:  []RemotePart{{Text: "standalone"}},
	}
	converted, _ := DefaultConvertToSessionEvent(remote)
	events := ag.process(remote, converted)
	if len(events) != 1 {
		t.Fatalf("expected 1 emitted event, got %d", len(events))
	}
	if events[0].Content.Parts[0].Text != "standalone" {
		t.Errorf("text = %q, want 'standalone'", events[0].Content.Parts[0].Text)
	}
}

func TestAggregator_AppendChunksThenLastChunk(t *testing.T) {
	ag := newAggregator()

	// Chunk 1: Append only, not LastChunk → suppress
	remote1 := &RemoteEvent{
		Type:      RemoteEventMessage,
		TaskID:    "t1",
		Parts:     []RemotePart{{Text: "Hello "}},
		Append:    true,
		LastChunk: false,
	}
	converted1, _ := DefaultConvertToSessionEvent(remote1)
	if events := ag.process(remote1, converted1); events != nil {
		t.Errorf("append chunk should be suppressed, got %d events", len(events))
	}

	// Chunk 2: Append only, not LastChunk → suppress
	remote2 := &RemoteEvent{
		Type:      RemoteEventMessage,
		TaskID:    "t1",
		Parts:     []RemotePart{{Text: "World"}},
		Append:    true,
		LastChunk: false,
	}
	converted2, _ := DefaultConvertToSessionEvent(remote2)
	if events := ag.process(remote2, converted2); events != nil {
		t.Errorf("append chunk should be suppressed, got %d events", len(events))
	}

	// Chunk 3: Append + LastChunk → flush
	remote3 := &RemoteEvent{
		Type:      RemoteEventMessage,
		TaskID:    "t1",
		Parts:     []RemotePart{{Text: "!"}},
		Append:    true,
		LastChunk: true,
	}
	converted3, _ := DefaultConvertToSessionEvent(remote3)
	events := ag.process(remote3, converted3)
	if len(events) != 1 {
		t.Fatalf("expected 1 flushed event, got %d", len(events))
	}
	// Aggregated text should be "Hello World!" (merged from three chunks, but
	// the first two are merged in order and the third is appended).
	if !strings.Contains(events[0].Content.Parts[0].Text, "Hello") {
		t.Errorf("flushed text should contain 'Hello', got %q", events[0].Content.Parts[0].Text)
	}
	if !strings.Contains(events[0].Content.Parts[0].Text, "World") {
		t.Errorf("flushed text should contain 'World', got %q", events[0].Content.Parts[0].Text)
	}
	if events[0].Partial {
		t.Error("flushed aggregated event should not be partial")
	}
}

func TestAggregator_TerminalFlush(t *testing.T) {
	ag := newAggregator()

	// Append some text without last chunk → suppressed
	remote1 := &RemoteEvent{
		Type:      RemoteEventMessage,
		TaskID:    "t1",
		Parts:     []RemotePart{{Text: "partial text"}},
		Append:    true,
		LastChunk: false,
	}
	converted1, _ := DefaultConvertToSessionEvent(remote1)
	ag.process(remote1, converted1)

	// Terminal status update → should flush accumulated text first, then emit status
	remote2 := &RemoteEvent{
		Type:   RemoteEventTaskStatusUpdate,
		TaskID: "t1",
		State:  TaskStateCompleted,
	}
	converted2, _ := DefaultConvertToSessionEvent(remote2)
	events := ag.process(remote2, converted2)

	if len(events) < 2 {
		t.Fatalf("expected >= 2 events (flushed text + status), got %d", len(events))
	}
	// First event should be the flushed aggregated text.
	if events[0].Content.Parts[0].Text != "partial text" {
		t.Errorf("flushed text = %q, want 'partial text'", events[0].Content.Parts[0].Text)
	}
	if events[0].Partial {
		t.Error("flushed text event should not be partial")
	}
	// Last event should be the terminal status.
	last := events[len(events)-1]
	if last.Actions.StateDelta["remote_task_state"] != string(TaskStateCompleted) {
		t.Errorf("terminal state = %q, want 'completed'", last.Actions.StateDelta["remote_task_state"])
	}
	if last.Partial {
		t.Error("terminal status event should not be partial")
	}
}

func TestAggregator_NonAppendResets(t *testing.T) {
	ag := newAggregator()

	// Append some text
	remote1 := &RemoteEvent{
		Type:      RemoteEventMessage,
		TaskID:    "t1",
		Parts:     []RemotePart{{Text: "old"}},
		Append:    true,
		LastChunk: false,
	}
	converted1, _ := DefaultConvertToSessionEvent(remote1)
	ag.process(remote1, converted1)

	// Non-append message should flush old text and emit new message
	remote2 := &RemoteEvent{
		Type:   RemoteEventMessage,
		TaskID: "t1",
		Parts:  []RemotePart{{Text: "new standalone"}},
	}
	converted2, _ := DefaultConvertToSessionEvent(remote2)
	events := ag.process(remote2, converted2)

	if len(events) != 2 {
		t.Fatalf("expected 2 events (flushed old + new), got %d", len(events))
	}
	if events[0].Content.Parts[0].Text != "old" {
		t.Errorf("first event text = %q, want 'old'", events[0].Content.Parts[0].Text)
	}
	if events[1].Content.Parts[0].Text != "new standalone" {
		t.Errorf("second event text = %q, want 'new standalone'", events[1].Content.Parts[0].Text)
	}
}

func TestAggregator_TaskStatusWorking(t *testing.T) {
	ag := newAggregator()

	// Non-terminal status update should pass through without flushing.
	remote := &RemoteEvent{
		Type:   RemoteEventTaskStatusUpdate,
		TaskID: "t1",
		State:  TaskStateWorking,
	}
	converted, _ := DefaultConvertToSessionEvent(remote)
	events := ag.process(remote, converted)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if !events[0].Partial {
		t.Error("working status should be partial")
	}
}

// ---------------------------------------------------------------------------
// Test: Error propagation
// ---------------------------------------------------------------------------

func TestConvertToSessionEvent_ErrorPropagation(t *testing.T) {
	sentinel := errors.New("remote exploded")
	remote := &RemoteEvent{
		Type:         RemoteEventMessage,
		TaskID:       "t1",
		Parts:        []RemotePart{{Text: "before crash"}},
		Error:        sentinel,
		ErrorMessage: "remote exploded",
	}
	ev, err := DefaultConvertToSessionEvent(remote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev[0].Error != sentinel {
		t.Errorf("expected error %v, got %v", sentinel, ev[0].Error)
	}
	if ev[0].ErrorMessage != "remote exploded" {
		t.Errorf("ErrorMessage = %q, want 'remote exploded'", ev[0].ErrorMessage)
	}
}

// ---------------------------------------------------------------------------
// Test: Cleanup callbacks
// ---------------------------------------------------------------------------

func TestCleanupCallbacks_InvokedOnStreamError(t *testing.T) {
	var cleanedTaskID string
	var cleanedState TaskState

	cleanup := func(taskID string, lastState TaskState, reason error) error {
		cleanedTaskID = taskID
		cleanedState = lastState
		return nil
	}

	var recordCancels []string
	cfg := FakeA2AClientConfig{
		Card: AgentCard{Name: "remote", StreamingSupported: true},
		Events: []StreamEvent{
			{Err: errors.New("stream broke")},
		},
		RecordCancels: &recordCancels,
	}

	remoteAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "remote-agent",
		Description: "test remote agent",
		AgentCard:   cfg.Card,
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return NewFakeA2AClient(cfg), nil
		},
		CleanupCallbacks: []CleanupCallback{cleanup},
	})
	if err != nil {
		t.Fatalf("NewRemoteAgent failed: %v", err)
	}

	ctx := &mockInvocationContext{agent: remoteAgent}
	_, err = remoteAgent.Execute(ctx)
	if err == nil {
		t.Error("expected error from stream")
	}

	if cleanedTaskID != "" {
		t.Logf("cleanup called with taskID=%q, state=%q", cleanedTaskID, cleanedState)
	}
}

func TestCleanupCallbacks_InvokedOnConvertError(t *testing.T) {
	var cleanedTaskID string

	cleanup := func(taskID string, lastState TaskState, reason error) error {
		cleanedTaskID = taskID
		return nil
	}

	// Emit a valid event first, then a status update that will have a convert error.
	cfg := FakeA2AClientConfig{
		Card: AgentCard{Name: "remote", StreamingSupported: true},
		Events: []StreamEvent{
			{Event: &RemoteEvent{Type: RemoteEventMessage, TaskID: "t1", Parts: []RemotePart{{Text: "hello"}}}},
		},
	}

	// Custom converter that fails.
	failConverter := func(remote *RemoteEvent) ([]*event.Event, error) {
		return nil, errors.New("convert failed")
	}

	remoteAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "remote-agent",
		Description: "test remote agent",
		AgentCard:   cfg.Card,
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return NewFakeA2AClient(cfg), nil
		},
		Converter:        failConverter,
		CleanupCallbacks: []CleanupCallback{cleanup},
	})
	if err != nil {
		t.Fatalf("NewRemoteAgent failed: %v", err)
	}

	ctx := &mockInvocationContext{agent: remoteAgent}
	_, err = remoteAgent.Execute(ctx)
	if err == nil {
		t.Error("expected error from converter")
	}
	if cleanedTaskID != "" {
		t.Logf("cleanup called with taskID=%q", cleanedTaskID)
	}
}

func TestCleanupCallbacks_MultipleOrdered(t *testing.T) {
	var callOrder []int

	cb1 := func(taskID string, lastState TaskState, reason error) error {
		callOrder = append(callOrder, 1)
		return nil
	}
	cb2 := func(taskID string, lastState TaskState, reason error) error {
		callOrder = append(callOrder, 2)
		return nil
	}

	cfg := FakeA2AClientConfig{
		Card:   AgentCard{Name: "remote"},
		Events: []StreamEvent{{Err: errors.New("fail")}},
	}

	remoteAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "remote-agent",
		Description: "test remote agent",
		AgentCard:   cfg.Card,
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return NewFakeA2AClient(cfg), nil
		},
		CleanupCallbacks: []CleanupCallback{cb1, cb2},
	})
	if err != nil {
		t.Fatalf("NewRemoteAgent failed: %v", err)
	}

	ctx := &mockInvocationContext{agent: remoteAgent}
	remoteAgent.Execute(ctx)

	if len(callOrder) != 2 {
		t.Fatalf("expected 2 cleanup calls, got %d", len(callOrder))
	}
	if callOrder[0] != 1 || callOrder[1] != 2 {
		t.Errorf("callOrder = %v, want [1, 2]", callOrder)
	}
}

func TestCleanupCallbacks_ErrorReturned(t *testing.T) {
	cleanupErr := errors.New("cleanup failed")

	cleanup := func(taskID string, lastState TaskState, reason error) error {
		return cleanupErr
	}

	cfg := FakeA2AClientConfig{
		Card:   AgentCard{Name: "remote"},
		Events: []StreamEvent{{Err: errors.New("stream fail")}},
	}

	remoteAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "remote-agent",
		Description: "test remote agent",
		AgentCard:   cfg.Card,
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return NewFakeA2AClient(cfg), nil
		},
		CleanupCallbacks: []CleanupCallback{cleanup},
	})
	if err != nil {
		t.Fatalf("NewRemoteAgent failed: %v", err)
	}

	ctx := &mockInvocationContext{agent: remoteAgent}
	_, err = remoteAgent.Execute(ctx)
	if err == nil {
		t.Error("expected error from cleanup callback")
	}
}

func TestCleanupCallbacks_NotInvokedWhenEmpty(t *testing.T) {
	cfg := FakeA2AClientConfig{
		Card: AgentCard{Name: "remote"},
		Events: []StreamEvent{
			{Event: &RemoteEvent{Type: RemoteEventMessage, TaskID: "t1", Parts: []RemotePart{{Text: "ok"}}}},
		},
	}

	remoteAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "remote-agent",
		Description: "test remote agent",
		AgentCard:   cfg.Card,
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return NewFakeA2AClient(cfg), nil
		},
	})
	if err != nil {
		t.Fatalf("NewRemoteAgent failed: %v", err)
	}

	ctx := &mockInvocationContext{agent: remoteAgent}
	events, err := remoteAgent.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) == 0 {
		t.Error("expected events from successful execution")
	}
}

// ---------------------------------------------------------------------------
// Test: RemoteAgent integration with fake client
// ---------------------------------------------------------------------------

func TestRemoteAgent_BasicStreaming(t *testing.T) {
	cfg := FakeA2AClientConfig{
		Card: AgentCard{Name: "echo-bot", Description: "echoes messages", StreamingSupported: true},
		Events: []StreamEvent{
			{Event: &RemoteEvent{Type: RemoteEventMessage, TaskID: "t1", Parts: []RemotePart{{Text: "echo: hello"}}}},
			{Event: &RemoteEvent{Type: RemoteEventTaskStatusUpdate, TaskID: "t1", State: TaskStateCompleted}},
		},
	}

	remoteAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "echo-agent",
		Description: "echo agent",
		AgentCard:   cfg.Card,
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return NewFakeA2AClient(cfg), nil
		},
	})
	if err != nil {
		t.Fatalf("NewRemoteAgent failed: %v", err)
	}

	if remoteAgent.Name() != "echo-agent" {
		t.Errorf("Name = %q, want 'echo-agent'", remoteAgent.Name())
	}
	if remoteAgent.Description() != "echo agent" {
		t.Errorf("Description = %q, want 'echo agent'", remoteAgent.Description())
	}

	ctx := &mockInvocationContext{agent: remoteAgent}
	events, err := remoteAgent.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected at least 1 event")
	}
	// Check the text content event.
	found := false
	for _, ev := range events {
		if ev.Content != nil && len(ev.Content.Parts) > 0 && ev.Content.Parts[0].Text == "echo: hello" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected event with text 'echo: hello', got events: %+v", events)
	}
}

func TestRemoteAgent_StreamError(t *testing.T) {
	cfg := FakeA2AClientConfig{
		Card: AgentCard{Name: "remote", StreamingSupported: true},
		Events: []StreamEvent{
			{Err: errors.New("connection lost")},
		},
	}

	remoteAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "fail-agent",
		Description: "failing agent",
		AgentCard:   cfg.Card,
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return NewFakeA2AClient(cfg), nil
		},
	})
	if err != nil {
		t.Fatalf("NewRemoteAgent failed: %v", err)
	}

	ctx := &mockInvocationContext{agent: remoteAgent}
	_, err = remoteAgent.Execute(ctx)
	if err == nil {
		t.Error("expected error from stream error")
	}
}

func TestRemoteAgent_ClientCreateError(t *testing.T) {
	remoteAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "fail-agent",
		Description: "failing agent",
		AgentCard:   AgentCard{Name: "remote"},
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return nil, errors.New("cannot connect")
		},
	})
	if err != nil {
		t.Fatalf("NewRemoteAgent failed: %v", err)
	}

	ctx := &mockInvocationContext{agent: remoteAgent}
	_, err = remoteAgent.Execute(ctx)
	if err == nil {
		t.Error("expected error from client creation")
	}
}

func TestRemoteAgent_ValidationErrors(t *testing.T) {
	_, err := NewRemoteAgent(RemoteAgentConfig{
		Name: "",
	})
	if err == nil {
		t.Error("expected error for empty name")
	}

	_, err = NewRemoteAgent(RemoteAgentConfig{
		Name:        "agent",
		Description: "test",
	})
	if err == nil {
		t.Error("expected error for missing ClientProvider")
	}
}

func TestNewRemoteAgent_Fields(t *testing.T) {
	a, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "test",
		Description: "desc",
		AgentCard:   AgentCard{Name: "card"},
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return NewFakeA2AClient(FakeA2AClientConfig{}), nil
		},
	})
	if err != nil {
		t.Fatalf("NewRemoteAgent failed: %v", err)
	}
	if a.Name() != "test" {
		t.Errorf("Name = %q, want 'test'", a.Name())
	}
	if a.Description() != "desc" {
		t.Errorf("Description = %q, want 'desc'", a.Description())
	}
}

// ---------------------------------------------------------------------------
// Test: TaskState.IsTerminal
// ---------------------------------------------------------------------------

func TestTaskState_IsTerminal(t *testing.T) {
	tests := []struct {
		state    TaskState
		terminal bool
	}{
		{TaskStateSubmitted, false},
		{TaskStateWorking, false},
		{TaskStateInputRequired, false},
		{TaskStateCompleted, true},
		{TaskStateFailed, true},
		{TaskStateCancelled, true},
	}
	for _, tt := range tests {
		if got := tt.state.IsTerminal(); got != tt.terminal {
			t.Errorf("%s.IsTerminal() = %v, want %v", tt.state, got, tt.terminal)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Streaming partial aggregation via RemoteAgent
// ---------------------------------------------------------------------------

func TestRemoteAgent_PartialAggregationFlow(t *testing.T) {
	cfg := FakeA2AClientConfig{
		Card: AgentCard{Name: "streamer", StreamingSupported: true},
		Events: []StreamEvent{
			// Chunk 1: partial append
			{Event: &RemoteEvent{
				Type:      RemoteEventTaskArtifactUpdate,
				TaskID:    "t1",
				Parts:     []RemotePart{{Text: "Hello "}},
				Append:    true,
				LastChunk: false,
			}},
			// Chunk 2: partial append
			{Event: &RemoteEvent{
				Type:      RemoteEventTaskArtifactUpdate,
				TaskID:    "t1",
				Parts:     []RemotePart{{Text: "World"}},
				Append:    true,
				LastChunk: false,
			}},
			// Chunk 3: final append
			{Event: &RemoteEvent{
				Type:      RemoteEventTaskArtifactUpdate,
				TaskID:    "t1",
				Parts:     []RemotePart{{Text: "!"}},
				Append:    true,
				LastChunk: true,
			}},
			// Terminal
			{Event: &RemoteEvent{
				Type:   RemoteEventTaskStatusUpdate,
				TaskID: "t1",
				State:  TaskStateCompleted,
			}},
		},
	}

	remoteAgent, err := NewRemoteAgent(RemoteAgentConfig{
		Name:        "streamer-agent",
		Description: "streaming agent",
		AgentCard:   cfg.Card,
		ClientProvider: func(card AgentCard) (A2AClient, error) {
			return NewFakeA2AClient(cfg), nil
		},
	})
	if err != nil {
		t.Fatalf("NewRemoteAgent failed: %v", err)
	}

	ctx := &mockInvocationContext{agent: remoteAgent}
	events, err := remoteAgent.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Should have: aggregated text event + terminal status event
	if len(events) < 1 {
		t.Fatal("expected at least 1 event from aggregation")
	}

	// Find the aggregated text event.
	var textEvent *event.Event
	for _, ev := range events {
		if ev.Content != nil && len(ev.Content.Parts) > 0 && ev.Content.Parts[0].Text != "" {
			textEvent = ev
			break
		}
	}
	if textEvent == nil {
		t.Fatal("expected an aggregated text event")
	}
	if textEvent.Partial {
		t.Error("aggregated event should not be partial")
	}

	combined := textEvent.Content.Parts[0].Text
	if !strings.Contains(combined, "Hello") {
		t.Errorf("aggregated text should contain 'Hello', got %q", combined)
	}
	if !strings.Contains(combined, "World") {
		t.Errorf("aggregated text should contain 'World', got %q", combined)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mockInvocationContext implements agent.InvocationContext for tests.
type mockInvocationContext struct {
	agent agent.Agent
	ended bool
}

func (m *mockInvocationContext) Agent() agent.Agent { return m.agent }
func (m *mockInvocationContext) EndInvocation()      { m.ended = true }
func (m *mockInvocationContext) Ended() bool         { return m.ended }

// collectStream drains a stream channel and returns all remote events.
func collectStream(t *testing.T, stream <-chan StreamEvent) []*RemoteEvent {
	t.Helper()
	var events []*RemoteEvent
	for se := range stream {
		if se.Err != nil {
			t.Fatalf("stream error: %v", se.Err)
		}
		events = append(events, se.Event)
	}
	return events
}
