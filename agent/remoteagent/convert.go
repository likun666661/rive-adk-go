package remoteagent

import (
	"fmt"
	"time"

	"github.com/likun666661/rive-adk-go/event"
)

// DefaultConvertToSessionEvent is the default remote-to-local event converter.
//
// Mapping rules:
//   - TaskStatusUpdate: produces a session event with task state metadata in
//     the event Actions.StateDelta (under key "remote_task_state").
//     Terminal states (completed/failed/cancelled) produce non-partial events.
//   - TaskArtifactUpdate / Message: text/function parts are mapped 1:1.
//     Append + !LastChunk → Partial = true.
//     !Append or Append + LastChunk → Partial = false.
func DefaultConvertToSessionEvent(remote *RemoteEvent) ([]*event.Event, error) {
	if remote == nil {
		return nil, fmt.Errorf("remoteagent: cannot convert nil remote event")
	}

	switch remote.Type {
	case RemoteEventTaskStatusUpdate:
		return convertTaskStatusUpdate(remote)
	case RemoteEventTaskArtifactUpdate, RemoteEventMessage:
		return convertContentEvent(remote)
	default:
		return nil, fmt.Errorf("remoteagent: unknown remote event type %q", remote.Type)
	}
}

func convertTaskStatusUpdate(remote *RemoteEvent) ([]*event.Event, error) {
	ev := event.NewEvent(
		remote.TaskID+"-status",
		"remoteagent",
		event.RoleModel,
	)
	ev.Timestamp = time.Now()

	ev.Actions.StateDelta = map[string]any{
		"remote_task_state":    string(remote.State),
		"remote_task_id":       remote.TaskID,
	}

	// Terminal states produce non-partial events; non-terminal produce partial.
	if remote.State.IsTerminal() {
		ev.Partial = false
		ev.TurnComplete = true
	} else {
		ev.Partial = true
	}

	// Attach error info if present.
	if remote.Error != nil {
		ev.Error = remote.Error
		ev.ErrorCode = remote.ErrorCode
		ev.ErrorMessage = remote.ErrorMessage
	}

	return []*event.Event{ev}, nil
}

func convertContentEvent(remote *RemoteEvent) ([]*event.Event, error) {
	ev := event.NewEvent(
		remote.TaskID+"-content",
		"remoteagent",
		event.RoleModel,
	)
	ev.Timestamp = time.Now()
	ev.Content = &event.Content{
		Role:  event.RoleModel,
		Parts: convertParts(remote.Parts),
	}

	// Partial flag: Append without LastChunk → partial.
	// Non-append or Append+LastChunk → final.
	if remote.Append && !remote.LastChunk {
		ev.Partial = true
	} else {
		ev.Partial = false
		ev.TurnComplete = true
	}

	// Attach error info if present.
	if remote.Error != nil {
		ev.Error = remote.Error
		ev.ErrorCode = remote.ErrorCode
		ev.ErrorMessage = remote.ErrorMessage
	}

	return []*event.Event{ev}, nil
}

func convertParts(parts []RemotePart) []event.Part {
	out := make([]event.Part, 0, len(parts))
	for _, p := range parts {
		ep := event.Part{
			Text:    p.Text,
			Thought: p.Thought,
		}
		if p.FunctionCall != nil {
			ep.FunctionCall = &event.FunctionCall{
				ID:   p.FunctionCall.ID,
				Name: p.FunctionCall.Name,
				Args: p.FunctionCall.Args,
			}
		}
		if p.FunctionResponse != nil {
			ep.FunctionResponse = &event.FunctionResponse{
				ID:     p.FunctionResponse.ID,
				Name:   p.FunctionResponse.Name,
				Result: p.FunctionResponse.Result,
				Error:  p.FunctionResponse.Error,
			}
		}
		out = append(out, ep)
	}
	return out
}

// ConvertSessionEventToRemote converts a local session event to one or more
// remote events. This is useful for constructing SendMessageRequest parts
// from session history.
func ConvertSessionEventToRemote(ev *event.Event) ([]RemotePart, error) {
	if ev == nil {
		return nil, fmt.Errorf("remoteagent: cannot convert nil session event")
	}
	if ev.Content == nil {
		return nil, nil
	}

	var parts []RemotePart
	for _, p := range ev.Content.Parts {
		rp := RemotePart{
			Text:    p.Text,
			Thought: p.Thought,
		}
		if p.FunctionCall != nil {
			rp.FunctionCall = &RemoteFunctionCall{
				ID:   p.FunctionCall.ID,
				Name: p.FunctionCall.Name,
				Args: p.FunctionCall.Args,
			}
		}
		if p.FunctionResponse != nil {
			rp.FunctionResponse = &RemoteFunctionResponse{
				ID:     p.FunctionResponse.ID,
				Name:   p.FunctionResponse.Name,
				Result: p.FunctionResponse.Result,
				Error:  p.FunctionResponse.Error,
			}
		}
		parts = append(parts, rp)
	}
	return parts, nil
}
