package telemetry

import (
	"context"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Span helpers — mirroring internal/telemetry/telemetry.go
// ---------------------------------------------------------------------------

// StartInvokeAgentSpan starts a span for an agent invocation.
// It records agent name, description, session ID, and invocation ID.
func StartInvokeAgentSpan(ctx context.Context, r *Recorder, agentName, agentDesc, sessionID, invocationID string) *StartedSpan {
	now := time.Now()
	s := &StartedSpan{
		recorder: r,
		name:     fmt.Sprintf("invoke_agent %s", agentName),
		start:    now,
		attrs: map[string]any{
			"gcp.vertex.agent.invocation_id": invocationID,
			"gen_ai.operation.name":          "invoke_agent",
			"gen_ai.agent.description":       agentDesc,
			"gen_ai.agent.name":              agentName,
			"gen_ai.conversation.id":         sessionID,
		},
	}
	return s
}

// StartGenerateContentSpan starts a span for a model generation call.
func StartGenerateContentSpan(ctx context.Context, r *Recorder, modelName, invocationID string) *StartedSpan {
	now := time.Now()
	s := &StartedSpan{
		recorder: r,
		name:     fmt.Sprintf("generate_content %s", modelName),
		start:    now,
		attrs: map[string]any{
			"gcp.vertex.agent.invocation_id": invocationID,
			"gen_ai.operation.name":          "generate_content",
			"gen_ai.request.model":           modelName,
		},
	}
	return s
}

// StartExecuteToolSpan starts a span for a tool execution.
func StartExecuteToolSpan(ctx context.Context, r *Recorder, toolName string, args map[string]any) *StartedSpan {
	now := time.Now()
	s := &StartedSpan{
		recorder: r,
		name:     fmt.Sprintf("execute_tool %s", toolName),
		start:    now,
		attrs: map[string]any{
			"gen_ai.operation.name":           "execute_tool",
			"gen_ai.tool.name":                toolName,
			"gcp.vertex.agent.tool_call_args": safeJSON(args),
		},
	}
	return s
}

// StartServerEventSpan starts a span for a server-side event (e.g. HTTP
// request handling).
func StartServerEventSpan(ctx context.Context, r *Recorder, operation, path string) *StartedSpan {
	now := time.Now()
	s := &StartedSpan{
		recorder: r,
		name:     fmt.Sprintf("server %s %s", operation, path),
		start:    now,
		attrs: map[string]any{
			"server.operation": operation,
			"server.path":      path,
		},
	}
	return s
}

// StartedSpan represents an in-progress span. Call End or EndWithError
// to record it.
type StartedSpan struct {
	recorder *Recorder
	name     string
	start    time.Time
	attrs    map[string]any
}

// SetAttribute adds or overwrites a span attribute.
func (s *StartedSpan) SetAttribute(key string, value any) {
	if s.attrs == nil {
		s.attrs = make(map[string]any)
	}
	s.attrs[key] = value
}

// End finalizes the span with status OK.
func (s *StartedSpan) End(status string) {
	now := time.Now()
	sr := SpanRecord{
		Name:       s.name,
		StartTime:  s.start,
		EndTime:    now,
		Attributes: s.attrs,
		Status:     status,
	}
	s.recorder.addSpan(sr)
}

// EndWithError finalizes the span with status ERROR.
func (s *StartedSpan) EndWithError(status, errMsg string) {
	now := time.Now()
	sr := SpanRecord{
		Name:       s.name,
		StartTime:  s.start,
		EndTime:    now,
		Attributes: s.attrs,
		Status:     status,
		Error:      errMsg,
	}
	s.recorder.addSpan(sr)
}

// SetTokenUsage records token usage attributes on a span.
func SetTokenUsage(s *StartedSpan, promptTokens, candidatesTokens, cachedTokens, thoughtsTokens int64) {
	s.SetAttribute("gen_ai.usage.input_tokens", promptTokens)
	s.SetAttribute("gen_ai.usage.output_tokens", candidatesTokens)
	s.SetAttribute("gen_ai.usage.cache_read.input_tokens", cachedTokens)
	s.SetAttribute("gen_ai.usage.reasoning.output_tokens", thoughtsTokens)
}

// SetEventID records the event ID that produced this span.
func SetEventID(s *StartedSpan, eventID string) {
	s.SetAttribute("gcp.vertex.agent.event_id", eventID)
}

// ---------------------------------------------------------------------------
// Log helpers — mirroring internal/telemetry/logger.go
// ---------------------------------------------------------------------------

// LogRequest records a model request log event.
// If captureMessageContent is false, the message body is elided.
func LogRequest(ctx context.Context, r *Recorder, systemMessage string, userMessages ...string) {
	capture := r.CaptureMessageContent()

	// System message log.
	if systemMessage != "" {
		content := "<elided>"
		if capture {
			content = systemMessage
		}
		r.addLog(LogRecord{
			EventName: "gen_ai.system.message",
			Timestamp: time.Now(),
			Body: map[string]any{
				"content": content,
			},
		})
	}

	// User message logs.
	for _, msg := range userMessages {
		content := "<elided>"
		if capture {
			content = msg
		}
		r.addLog(LogRecord{
			EventName: "gen_ai.user.message",
			Timestamp: time.Now(),
			Body: map[string]any{
				"content": content,
			},
		})
	}
}

// LogResponse records a model response log event.
func LogResponse(ctx context.Context, r *Recorder, finishReason string, content string, toolCalls []string) {
	capture := r.CaptureMessageContent()

	body := map[string]any{
		"index": 0,
	}
	if capture {
		body["content"] = content
	} else {
		body["content"] = "<elided>"
	}

	if finishReason != "" {
		body["finish_reason"] = finishReason
	}
	if len(toolCalls) > 0 {
		tcs := make([]map[string]any, len(toolCalls))
		for i, tc := range toolCalls {
			tcs[i] = map[string]any{
				"name": tc,
			}
		}
		body["tool_calls"] = tcs
	}

	r.addLog(LogRecord{
		EventName: "gen_ai.choice",
		Timestamp: time.Now(),
		Body:      body,
	})
}

// LogServerEvent records a server-side event (e.g. HTTP request/reply).
func LogServerEvent(ctx context.Context, r *Recorder, method, path string, statusCode int, duration time.Duration) {
	r.addLog(LogRecord{
		EventName: "server.request",
		Timestamp: time.Now(),
		Attributes: map[string]any{
			"http.method":        method,
			"http.path":          path,
			"http.status_code":   statusCode,
			"http.duration_ms":   duration.Milliseconds(),
		},
	})
}

// ---------------------------------------------------------------------------
// Flush helpers
// ---------------------------------------------------------------------------

// FlushSpanCount returns the number of spans, which can be used after
// Shutdown to verify proper flush semantics.
func FlushSpanCount(r *Recorder) int {
	return r.SpanCount()
}

// FlushLogCount returns the number of logs after flush.
func FlushLogCount(r *Recorder) int {
	return r.LogCount()
}

// Drain clears all recorded data (used in tests to clean state).
func Drain(r *Recorder) {
	r.Reset()
}
