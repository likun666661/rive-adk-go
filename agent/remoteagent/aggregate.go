package remoteagent

import (
	"github.com/likun666661/rive-adk-go/event"
)

type aggregateItem struct {
	eventType RemoteEventType
	text      string
	thought   bool
}

// aggregator manages partial-to-full event aggregation across a stream.
//
// The core semantics mirror the ADK Go A2A processor:
//
//   - TaskStatusUpdate (terminal): flush all accumulated partials as non-partial
//     events, then emit the terminal status.
//   - TaskArtifactUpdate (non-Append): reset the aggregation buffer and emit a
//     standalone event for this artifact.
//   - TaskArtifactUpdate (Append, !LastChunk): accumulate text into the buffer
//     and emit nothing (or emit a partial event).
//   - TaskArtifactUpdate (Append, LastChunk): append, flush the buffer as a
//     single non-partial event, reset.
type aggregator struct {
	pending []aggregateItem
}

// newAggregator creates a new aggregator with an empty buffer.
func newAggregator() *aggregator {
	return &aggregator{
		pending: make([]aggregateItem, 0),
	}
}

// process takes a converted session event and the originating remote event,
// and returns zero or more events to emit. The aggregator may hold partial
// events until a terminal flush.
//
// Parameters:
//   - remote: the originating RemoteEvent (controls append/last-chunk semantics).
//   - converted: the session events produced by ConvertToSessionEvent.
//
// Returns the list of events that should be yielded to the consumer.
func (a *aggregator) process(remote *RemoteEvent, converted []*event.Event) []*event.Event {
	if remote == nil {
		return converted
	}

	switch remote.Type {
	case RemoteEventTaskStatusUpdate:
		if remote.State.IsTerminal() {
			return a.terminalFlush(converted)
		}
		// Non-terminal status update: emit converted events directly.
		return converted

	case RemoteEventTaskArtifactUpdate:
		return a.handleArtifactUpdate(remote, converted)

	case RemoteEventMessage:
		if remote.Append && !remote.LastChunk {
			a.accumulate(remote, converted)
			return nil
		}
		if remote.Append && remote.LastChunk {
			a.accumulate(remote, converted)
			return a.flush()
		}
		// Non-append message: emit converted events directly after clearing pending.
		events := a.flushWithoutReset()
		a.reset()
		events = append(events, converted...)
		return events

	default:
		return converted
	}
}

// flush returns all accumulated data as non-partial events and resets the buffer.
func (a *aggregator) flush() []*event.Event {
	events := a.flushWithoutReset()
	a.reset()
	return events
}

// flushWithoutReset returns accumulated data as non-partial events without
// clearing the buffer (allows the caller to decide when to reset).
func (a *aggregator) flushWithoutReset() []*event.Event {
	if len(a.pending) == 0 {
		return nil
	}

	var events []*event.Event
	var currentText string
	var currentThought bool

	for _, item := range a.pending {
		if item.text == "" {
			continue
		}

		// Merge consecutive text with same thought flag.
		if currentText != "" && item.thought == currentThought {
			currentText += item.text
		} else {
			// Emit previous merged text.
			if currentText != "" {
				ev := buildAggregatedEvent(currentText, currentThought)
				events = append(events, ev)
			}
			currentText = item.text
			currentThought = item.thought
		}
	}

	// Emit final merged text.
	if currentText != "" {
		ev := buildAggregatedEvent(currentText, currentThought)
		events = append(events, ev)
	}

	return events
}

func (a *aggregator) reset() {
	a.pending = a.pending[:0]
}

func (a *aggregator) accumulate(remote *RemoteEvent, converted []*event.Event) {
	if len(converted) == 0 || converted[0] == nil || converted[0].Content == nil {
		return
	}

	ev := converted[0]
	for _, p := range ev.Content.Parts {
		if p.Text == "" {
			continue
		}
		a.pending = append(a.pending, aggregateItem{
			eventType: remote.Type,
			text:      p.Text,
			thought:   p.Thought,
		})
	}
}

func (a *aggregator) handleArtifactUpdate(remote *RemoteEvent, converted []*event.Event) []*event.Event {
	if !remote.Append {
		// Non-append artifact: reset aggregation and emit converted event.
		events := a.flushWithoutReset()
		a.reset()
		events = append(events, converted...)
		return events
	}

	if !remote.LastChunk {
		// Append chunk, not last: accumulate and suppress emission.
		a.accumulate(remote, converted)
		return nil
	}

	// Append + LastChunk: accumulate, flush, reset.
	a.accumulate(remote, converted)
	return a.flush()
}

// terminalFlush flushes all accumulated partials as non-partial events,
// then appends the terminal status events.
func (a *aggregator) terminalFlush(converted []*event.Event) []*event.Event {
	events := a.flush()
	events = append(events, converted...)
	return events
}

func buildAggregatedEvent(text string, thought bool) *event.Event {
	ev := event.NewEvent("aggregated", "remoteagent", event.RoleModel)
	ev.Content = &event.Content{
		Role: event.RoleModel,
		Parts: []event.Part{
			{
				Text:    text,
				Thought: thought,
			},
		},
	}
	ev.Partial = false
	ev.TurnComplete = true
	return ev
}
