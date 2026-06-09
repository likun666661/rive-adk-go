// Package remoteagent implements a lightweight educational Remote A2A bridge.
//
// It demonstrates how local agent events can be converted to a remote protocol
// stream and back into local session events. This is a teaching model, not a
// network-compatible ADK A2A implementation.
//
// Key concepts:
//   - AgentCard: identity / capability metadata for a remote agent.
//   - A2AClient: interface for streaming remote calls with cancel / cleanup.
//   - RemoteEvent: a simplified remote protocol event model.
//   - Convert: bidirectional conversion between session.Event and RemoteEvent.
//   - aggregatePartial: streaming partial aggregation with terminal flush.
//   - CleanupCallback: cleanup/cancel semantics for abandoned remote tasks.
package remoteagent

import (
	"github.com/likun666661/rive-adk-go/event"
)

// TaskState describes the lifecycle state of a remote task.
type TaskState string

const (
	TaskStateSubmitted   TaskState = "submitted"
	TaskStateWorking     TaskState = "working"
	TaskStateInputRequired TaskState = "input-required"
	TaskStateCompleted   TaskState = "completed"
	TaskStateFailed      TaskState = "failed"
	TaskStateCancelled   TaskState = "cancelled"
)

// IsTerminal returns true if the state is a terminal (final) state.
func (s TaskState) IsTerminal() bool {
	switch s {
	case TaskStateCompleted, TaskStateFailed, TaskStateCancelled:
		return true
	}
	return false
}

// RemoteEventType classifies remote protocol events.
type RemoteEventType string

const (
	RemoteEventTaskStatusUpdate  RemoteEventType = "task-status-update"
	RemoteEventTaskArtifactUpdate RemoteEventType = "task-artifact-update"
	RemoteEventMessage           RemoteEventType = "message"
)

// RemoteEvent is a simplified remote protocol event.
//
// It models the core event types from the A2A protocol:
//   - TaskStatusUpdate: carries task lifecycle transitions.
//   - TaskArtifactUpdate: carries artifact chunks (may be Append/LastChunk).
//   - Message: carries single-part or multi-part messages.
type RemoteEvent struct {
	// Type classifies the event.
	Type RemoteEventType

	// TaskID is the remote task identifier this event belongs to.
	TaskID string

	// State is the task state (for TaskStatusUpdate events).
	State TaskState

	// Parts holds text or function-call parts (for Message / TaskArtifactUpdate events).
	Parts []RemotePart

	// Append indicates this is an append chunk, not a standalone event.
	Append bool

	// LastChunk indicates this is the final chunk in an append chain.
	LastChunk bool

	// Metadata carries optional key-value metadata.
	Metadata map[string]string

	// Error is an optional error attached to this event.
	Error error

	// ErrorCode is a machine-readable error code.
	ErrorCode string

	// ErrorMessage is a human-readable error description.
	ErrorMessage string
}

// RemotePart is a single typed piece of content in a remote event.
type RemotePart struct {
	// Text is inline text content.
	Text string

	// Thought indicates this part is a model thought (not visible to end users).
	Thought bool

	// FunctionCall represents a tool call request.
	FunctionCall *RemoteFunctionCall

	// FunctionResponse represents a tool call result.
	FunctionResponse *RemoteFunctionResponse
}

// RemoteFunctionCall mirrors event.FunctionCall for the remote protocol.
type RemoteFunctionCall struct {
	ID   string
	Name string
	Args map[string]any
}

// RemoteFunctionResponse mirrors event.FunctionResponse for the remote protocol.
type RemoteFunctionResponse struct {
	ID     string
	Name   string
	Result map[string]any
	Error  string
}

// AgentCard describes a remote agent's identity and capabilities.
//
// In a real A2A implementation this would include skill declarations,
// protocol version negotiation, authentication metadata, etc. This
// teaching model keeps only the essential fields.
type AgentCard struct {
	// Name is the agent's display name.
	Name string

	// Description is a human-readable description of the agent.
	Description string

	// URL is the remote agent's endpoint (not used in fake clients).
	URL string

	// StreamingSupported indicates whether the remote agent supports streaming.
	StreamingSupported bool

	// Capabilities lists the agent's declared capabilities.
	Capabilities []string
}

// SendMessageRequest is the request sent to a remote agent.
type SendMessageRequest struct {
	// TaskID is an optional task identifier for stateful tasks.
	TaskID string

	// Parts are the content parts sent to the remote agent.
	Parts []RemotePart

	// Metadata carries optional request metadata.
	Metadata map[string]string

	// Streaming indicates whether the client wants a streaming response.
	Streaming bool
}

// StreamEvent pairs a remote event with an optional error.
type StreamEvent struct {
	Event *RemoteEvent
	Err   error
}

// A2AClient is the remote communication interface.
//
// Implementations can be network-based (REST, gRPC) or in-memory (for tests).
// The FakeA2AClient in this package provides an in-memory implementation
// suitable for testing.
type A2AClient interface {
	// SendStreamingMessage sends a request and returns a channel of stream events.
	// The returned channel is closed when the stream completes (success or error).
	SendStreamingMessage(req SendMessageRequest) <-chan StreamEvent

	// CancelTask requests cancellation of a remote task.
	// It returns an error if the cancellation fails (e.g., task not found).
	CancelTask(taskID string) error

	// Destroy releases any resources held by the client.
	Destroy() error
}

// CleanupCallback is invoked when a remote task needs cleanup.
//
// Parameters:
//   - taskID: the remote task to clean up.
//   - lastState: the last known task state ("" if unknown).
//   - err: the reason for cleanup (nil if clean shutdown).
//
// A typical implementation calls CancelTask on the remote service.
type CleanupCallback func(taskID string, lastState TaskState, err error) error

// ConvertToSessionEvent converts a remote event to a local session event.
//
// This is the default converter that handles all RemoteEventTypes:
//   - TaskStatusUpdate → session event with state metadata.
//   - TaskArtifactUpdate → session event with artifact content.
//   - Message → session event with model content.
//
// When Append is true and LastChunk is false the result has Partial=true.
// When Append and LastChunk are both true, the result is non-partial (final).
//
// Custom converters can be supplied via Config.Converter for protocol-specific
// transformations.
type Converter func(remote *RemoteEvent) ([]*event.Event, error)
