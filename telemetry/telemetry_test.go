package telemetry

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Recorder — span recording
// ---------------------------------------------------------------------------

func TestRecorderSpanRecording(t *testing.T) {
	r := NewRecorder()
	ctx := context.Background()

	s1 := StartInvokeAgentSpan(ctx, r, "my-agent", "does things", "sess-1", "inv-1")
	s1.End("OK")

	s2 := StartGenerateContentSpan(ctx, r, "gemini-2.5-flash", "inv-1")
	SetEventID(s2, "ev-1")
	SetTokenUsage(s2, 100, 50, 10, 5)
	s2.End("OK")

	s3 := StartExecuteToolSpan(ctx, r, "get_weather", map[string]any{"city": "Tokyo"})
	s3.End("OK")

	spans := r.Spans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	// Span 1: invoke_agent.
	if !strings.Contains(spans[0].Name, "invoke_agent") {
		t.Errorf("span[0].Name = %q, want 'invoke_agent ...'", spans[0].Name)
	}
	if spans[0].Status != "OK" {
		t.Errorf("span[0].Status = %q", spans[0].Status)
	}
	if spans[0].Attributes["gen_ai.agent.name"] != "my-agent" {
		t.Errorf("span[0] agent.name = %v", spans[0].Attributes["gen_ai.agent.name"])
	}
	if spans[0].Attributes["gcp.vertex.agent.invocation_id"] != "inv-1" {
		t.Errorf("span[0] invocation_id = %v", spans[0].Attributes["gcp.vertex.agent.invocation_id"])
	}
	if spans[0].Attributes["gen_ai.conversation.id"] != "sess-1" {
		t.Errorf("span[0] conversation.id = %v", spans[0].Attributes["gen_ai.conversation.id"])
	}

	// Span 2: generate_content.
	if !strings.Contains(spans[1].Name, "generate_content") {
		t.Errorf("span[1].Name = %q", spans[1].Name)
	}
	if spans[1].Attributes["gen_ai.request.model"] != "gemini-2.5-flash" {
		t.Errorf("span[1] model = %v", spans[1].Attributes["gen_ai.request.model"])
	}
	if spans[1].Attributes["gcp.vertex.agent.event_id"] != "ev-1" {
		t.Errorf("span[1] event_id = %v", spans[1].Attributes["gcp.vertex.agent.event_id"])
	}
	if spans[1].Attributes["gen_ai.usage.input_tokens"] != int64(100) {
		t.Errorf("span[1] input_tokens = %v", spans[1].Attributes["gen_ai.usage.input_tokens"])
	}
	if spans[1].Attributes["gen_ai.usage.output_tokens"] != int64(50) {
		t.Errorf("span[1] output_tokens = %v", spans[1].Attributes["gen_ai.usage.output_tokens"])
	}
	if spans[1].Attributes["gen_ai.usage.cache_read.input_tokens"] != int64(10) {
		t.Errorf("span[1] cache_read = %v", spans[1].Attributes["gen_ai.usage.cache_read.input_tokens"])
	}
	if spans[1].Attributes["gen_ai.usage.reasoning.output_tokens"] != int64(5) {
		t.Errorf("span[1] reasoning_tokens = %v", spans[1].Attributes["gen_ai.usage.reasoning.output_tokens"])
	}

	// Span 3: execute_tool.
	if !strings.Contains(spans[2].Name, "execute_tool") {
		t.Errorf("span[2].Name = %q", spans[2].Name)
	}
	if spans[2].Attributes["gen_ai.tool.name"] != "get_weather" {
		t.Errorf("span[2] tool.name = %v", spans[2].Attributes["gen_ai.tool.name"])
	}
	args := spans[2].Attributes["gcp.vertex.agent.tool_call_args"].(string)
	if !strings.Contains(args, "Tokyo") {
		t.Errorf("span[2] tool_call_args missing 'Tokyo': %s", args)
	}
}

func TestSpanErrorRecording(t *testing.T) {
	r := NewRecorder()
	ctx := context.Background()

	s := StartInvokeAgentSpan(ctx, r, "failing-agent", "fails", "sess-1", "inv-1")
	s.EndWithError("ERROR", "invocation timeout")

	spans := r.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status != "ERROR" {
		t.Errorf("Status = %q, want ERROR", spans[0].Status)
	}
	if spans[0].Error != "invocation timeout" {
		t.Errorf("Error = %q", spans[0].Error)
	}
}

func TestServerEventSpan(t *testing.T) {
	r := NewRecorder()
	ctx := context.Background()

	s := StartServerEventSpan(ctx, r, "POST", "/run_sse")
	s.End("OK")

	spans := r.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Attributes["server.operation"] != "POST" {
		t.Errorf("operation = %v", spans[0].Attributes["server.operation"])
	}
	if spans[0].Attributes["server.path"] != "/run_sse" {
		t.Errorf("path = %v", spans[0].Attributes["server.path"])
	}
}

// ---------------------------------------------------------------------------
// Log recording
// ---------------------------------------------------------------------------

func TestLogRequest(t *testing.T) {
	r := NewRecorder(WithCaptureMessageContent(true))
	ctx := context.Background()

	LogRequest(ctx, r, "You are a helpful assistant.", "What is the weather?")
	logs := r.Logs()

	if len(logs) != 2 {
		t.Fatalf("expected 2 log records (system + user), got %d", len(logs))
	}

	// System message.
	if logs[0].EventName != "gen_ai.system.message" {
		t.Errorf("log[0].EventName = %q", logs[0].EventName)
	}
	if logs[0].Body["content"] != "You are a helpful assistant." {
		t.Errorf("log[0] content = %v", logs[0].Body["content"])
	}

	// User message.
	if logs[1].EventName != "gen_ai.user.message" {
		t.Errorf("log[1].EventName = %q", logs[1].EventName)
	}
}

func TestLogRequestElided(t *testing.T) {
	r := NewRecorder() // capture is off by default.
	ctx := context.Background()

	LogRequest(ctx, r, "secret system prompt", "secret user message")
	logs := r.Logs()

	if len(logs) != 2 {
		t.Fatalf("expected 2 log records, got %d", len(logs))
	}
	if logs[0].Body["content"] != "<elided>" {
		t.Errorf("system content should be elided: %v", logs[0].Body["content"])
	}
	if logs[1].Body["content"] != "<elided>" {
		t.Errorf("user content should be elided: %v", logs[1].Body["content"])
	}
}

func TestLogResponse(t *testing.T) {
	r := NewRecorder(WithCaptureMessageContent(true))
	ctx := context.Background()

	LogResponse(ctx, r, "STOP", "The weather is sunny.", []string{"get_weather"})
	logs := r.Logs()

	if len(logs) != 1 {
		t.Fatalf("expected 1 log record, got %d", len(logs))
	}
	if logs[0].EventName != "gen_ai.choice" {
		t.Errorf("EventName = %q", logs[0].EventName)
	}
	if logs[0].Body["finish_reason"] != "STOP" {
		t.Errorf("finish_reason = %v", logs[0].Body["finish_reason"])
	}
	if logs[0].Body["content"] != "The weather is sunny." {
		t.Errorf("content = %v", logs[0].Body["content"])
	}

	tcVal := logs[0].Body["tool_calls"]
	tcs, ok := tcVal.([]map[string]any)
	if !ok {
		t.Fatalf("tool_calls is not []map[string]any: %T", tcVal)
	}
	if len(tcs) != 1 || tcs[0]["name"] != "get_weather" {
		t.Errorf("tool_calls = %v", tcs)
	}
}

func TestLogResponseElided(t *testing.T) {
	r := NewRecorder() // capture off.
	ctx := context.Background()

	LogResponse(ctx, r, "STOP", "secret response", nil)
	logs := r.Logs()

	if logs[0].Body["content"] != "<elided>" {
		t.Errorf("content should be elided: %v", logs[0].Body["content"])
	}
}

func TestLogServerEvent(t *testing.T) {
	r := NewRecorder()
	ctx := context.Background()

	LogServerEvent(ctx, r, "POST", "/run", 200, 15*time.Millisecond)
	logs := r.Logs()

	if len(logs) != 1 {
		t.Fatalf("expected 1 log record, got %d", len(logs))
	}
	if logs[0].EventName != "server.request" {
		t.Errorf("EventName = %q", logs[0].EventName)
	}
	if logs[0].Attributes["http.method"] != "POST" {
		t.Errorf("method = %v", logs[0].Attributes["http.method"])
	}
	if logs[0].Attributes["http.status_code"] != 200 {
		t.Errorf("status = %v", logs[0].Attributes["http.status_code"])
	}
	dur := logs[0].Attributes["http.duration_ms"]
	if dur.(int64) != 15 {
		t.Errorf("duration_ms = %v", dur)
	}
}

// ---------------------------------------------------------------------------
// Providers lifecycle
// ---------------------------------------------------------------------------

func TestProvidersInitShutdown(t *testing.T) {
	p := NewProviders(WithCaptureMessageContent(true))
	ctx := context.Background()

	if err := p.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	r := p.Recorder()
	LogRequest(ctx, r, "system", "user")
	if r.SpanCount() != 0 || r.LogCount() != 2 {
		t.Errorf("after Init: spans=%d logs=%d, want 0/2", r.SpanCount(), r.LogCount())
	}

	if err := p.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestProvidersShutdownKeepsData(t *testing.T) {
	p := NewProviders()
	ctx := context.Background()
	_ = p.Init(ctx)

	r := p.Recorder()
	LogRequest(ctx, r, "sys", "msg")
	_ = p.Shutdown(ctx)

	// Data should still be available after shutdown for inspection.
	if r.LogCount() != 2 {
		t.Errorf("logs after shutdown = %d, want 2", r.LogCount())
	}
}

// ---------------------------------------------------------------------------
// Default recorder
// ---------------------------------------------------------------------------

func TestDefaultRecorder(t *testing.T) {
	SetDefaultRecorder(NewRecorder(WithCaptureMessageContent(true)))
	r := DefaultRecorder()
	if r == nil {
		t.Fatal("DefaultRecorder is nil")
	}
	ctx := context.Background()
	StartInvokeAgentSpan(ctx, r, "a", "desc", "s", "i").End("OK")
	if r.SpanCount() != 1 {
		t.Errorf("default recorder span count = %d", r.SpanCount())
	}
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

func TestRecorderConcurrency(t *testing.T) {
	r := NewRecorder()
	ctx := context.Background()
	var wg sync.WaitGroup

	const goroutines = 10
	const iters = 100
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				StartInvokeAgentSpan(ctx, r, "a", "desc", "s", "inv").End("OK")
				LogRequest(ctx, r, "sys", "user")
			}
		}(g)
	}
	wg.Wait()

	if r.SpanCount() != goroutines*iters {
		t.Errorf("spans = %d, want %d", r.SpanCount(), goroutines*iters)
	}
	if r.LogCount() != goroutines*iters*2 { // system + user per iteration
		t.Errorf("logs = %d, want %d", r.LogCount(), goroutines*iters*2)
	}
}

// ---------------------------------------------------------------------------
// Drain / Reset
// ---------------------------------------------------------------------------

func TestDrain(t *testing.T) {
	r := NewRecorder()
	ctx := context.Background()

	StartInvokeAgentSpan(ctx, r, "a", "d", "s", "i").End("OK")
	LogRequest(ctx, r, "sys", "usr")

	if r.SpanCount() != 1 || r.LogCount() != 2 {
		t.Fatalf("before Drain: spans=%d logs=%d", r.SpanCount(), r.LogCount())
	}

	Drain(r)
	if r.SpanCount() != 0 || r.LogCount() != 0 {
		t.Errorf("after Drain: spans=%d logs=%d, want 0/0", r.SpanCount(), r.LogCount())
	}
}

// ---------------------------------------------------------------------------
// Span timestamp ordering
// ---------------------------------------------------------------------------

func TestSpanTimestamps(t *testing.T) {
	r := NewRecorder()
	ctx := context.Background()

	before := time.Now()
	s1 := StartInvokeAgentSpan(ctx, r, "a1", "d", "s", "i")
	time.Sleep(5 * time.Millisecond)
	s1.End("OK")

	s2 := StartGenerateContentSpan(ctx, r, "model", "inv")
	time.Sleep(5 * time.Millisecond)
	s2.End("OK")

	spans := r.Spans()

	// Verify start is before end within each span.
	if !spans[0].StartTime.Before(spans[0].EndTime) {
		t.Error("span[0] start not before end")
	}
	if !spans[1].StartTime.Before(spans[1].EndTime) {
		t.Error("span[1] start not before end")
	}

	// Verify ordering: span[0] started before span[1].
	if !spans[0].StartTime.Before(spans[1].StartTime) {
		t.Error("span[0] should start before span[1]")
	}

	// Both spans started after our baseline.
	if !spans[0].StartTime.After(before) {
		t.Error("span[0] started before our baseline")
	}
}
