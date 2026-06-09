package adkrest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/cmd/launcher"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/runner"
)

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

func newTestAgent(name string, text string) agent.Agent {
	ag, err := agent.New(agent.Config{
		Name: name,
		Description: "Test agent",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			ev := event.NewEvent("ev-1", name, event.RoleModel)
			ev.Content = &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: text}},
			}
			return []*event.Event{ev}, nil
		},
	})
	if err != nil {
		panic(err)
	}
	return ag
}

func newTestAgentWithEvents(name string, events []*event.Event) agent.Agent {
	ag, err := agent.New(agent.Config{
		Name: name,
		Description: "Test agent",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return events, nil
		},
	})
	if err != nil {
		panic(err)
	}
	return ag
}

type testAgentLoader struct {
	ag agent.Agent
}

func (l *testAgentLoader) RootAgent() agent.Agent { return l.ag }

func newTestServer(t *testing.T, ag agent.Agent) *Server {
	t.Helper()
	cfg := &launcher.Config{
		SessionService: runner.NewInMemorySessionService(),
		AgentLoader:    &testAgentLoader{ag: ag},
	}
	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s.SetAppName("testapp")
	return s
}

func makeJSONBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

func mustUnmarshal[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return v
}

// ---------------------------------------------------------------------------
// Test 1: JSON run happy path
// ---------------------------------------------------------------------------

func TestRunHandlerHappyPath(t *testing.T) {
	ag := newTestAgent("greeter", "Hello, world!")
	s := newTestServer(t, ag)

	req := httptest.NewRequest(http.MethodPost, "/run", makeJSONBody(RunRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: "sess-1",
		Message:   "Hi",
	}))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	events := mustUnmarshal[[]EventResponse](t, rec.Body.Bytes())
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Author != "greeter" {
		t.Errorf("Author = %q, want 'greeter'", events[0].Author)
	}
	if events[0].Role != "model" {
		t.Errorf("Role = %q, want 'model'", events[0].Role)
	}
	if events[0].Content == nil || len(events[0].Content.Parts) == 0 {
		t.Fatal("expected content with parts")
	}
	if events[0].Content.Parts[0].Text != "Hello, world!" {
		t.Errorf("Text = %q, want 'Hello, world!'", events[0].Content.Parts[0].Text)
	}
}

// ---------------------------------------------------------------------------
// Test 2: SSE framing
// ---------------------------------------------------------------------------

func TestRunSSEHandlerHappyPath(t *testing.T) {
	ag := newTestAgent("greeter", "Hello from SSE!")
	s := newTestServer(t, ag)

	req := httptest.NewRequest(http.MethodPost, "/run_sse", makeJSONBody(RunRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: "sess-sse",
		Message:   "Hi",
	}))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "data: ") {
		t.Errorf("SSE body missing 'data: ' prefix: %s", body)
	}
	if !strings.Contains(body, "Hello from SSE!") {
		t.Errorf("SSE body missing expected text: %s", body)
	}
	// SSE format: "data: <json>\n\n"
	if !strings.Contains(body, "\n\n") {
		t.Errorf("SSE body should contain double newline separator: %q", body)
	}

	// Parse SSE frames — each frame is "data: <json>\n\n"
	frames := strings.Split(strings.TrimSpace(body), "\n\n")
	found := false
	for _, frame := range frames {
		frame = strings.TrimSpace(frame)
		if !strings.HasPrefix(frame, "data:") {
			continue
		}
		jsonStr := strings.TrimPrefix(frame, "data:")
		var ev EventResponse
		if err := json.Unmarshal([]byte(jsonStr), &ev); err != nil {
			continue
		}
		if ev.Content != nil && len(ev.Content.Parts) > 0 && ev.Content.Parts[0].Text == "Hello from SSE!" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find expected event in SSE frames")
	}
}

// ---------------------------------------------------------------------------
// Test 3: multiple events in SSE
// ---------------------------------------------------------------------------

func TestRunSSEMultipleEvents(t *testing.T) {
	ag := newTestAgentWithEvents("multi", []*event.Event{
		{
			ID:      "e1",
			Author:  "multi",
			Role:    event.RoleModel,
			Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "chunk 1"}}},
			Partial: true,
		},
		{
			ID:      "e2",
			Author:  "multi",
			Role:    event.RoleModel,
			Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "chunk 2"}}},
		},
	})
	s := newTestServer(t, ag)

	req := httptest.NewRequest(http.MethodPost, "/run_sse", makeJSONBody(RunRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: "sess-multi",
		Message:   "Hi",
	}))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "chunk 1") {
		t.Errorf("body missing 'chunk 1': %s", body)
	}
	if !strings.Contains(body, "chunk 2") {
		t.Errorf("body missing 'chunk 2': %s", body)
	}

	frameCount := strings.Count(body, "data:")
	if frameCount < 2 {
		t.Errorf("expected at least 2 SSE data frames, got %d: %s", frameCount, body)
	}
}

// ---------------------------------------------------------------------------
// Test 4: missing agent returns 404
// ---------------------------------------------------------------------------

func TestRunHandlerMissingAgent(t *testing.T) {
	cfg := &launcher.Config{
		SessionService: runner.NewInMemorySessionService(),
		AgentLoader:    nil,
	}
	s, err := NewServer(cfg)
	if s == nil && err != nil {
		// Expected — NewServer validates config.
		if !strings.Contains(err.Error(), "AgentLoader") {
			t.Errorf("unexpected error: %v", err)
		}
		return
	}
	if s == nil {
		t.Fatal("expected error creating server without AgentLoader")
	}

	req := httptest.NewRequest(http.MethodPost, "/run", makeJSONBody(RunRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: "sess-1",
		Message:   "Hi",
	}))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing agent, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 5: malformed request does not corrupt later requests
// ---------------------------------------------------------------------------

func TestMalformedRequestDoesNotCorruptLaterRequests(t *testing.T) {
	ag := newTestAgent("echo", "I am working!")
	s := newTestServer(t, ag)

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Send malformed request first.
	malformedResp, err := http.Post(ts.URL+"/run", "application/json",
		strings.NewReader(`{"appName": "testapp"`)) // truncated JSON
	if err != nil {
		t.Fatalf("malformed request: %v", err)
	}
	malformedBody, _ := io.ReadAll(malformedResp.Body)
	malformedResp.Body.Close()

	if malformedResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed request, got %d: %s", malformedResp.StatusCode, string(malformedBody))
	}
	if !strings.Contains(string(malformedBody), "decode") {
		t.Errorf("expected decode error, got: %s", string(malformedBody))
	}

	// Now send a valid request — should work.
	validBody, _ := json.Marshal(RunRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: "sess-valid",
		Message:   "Hello",
	})
	validResp, err := http.Post(ts.URL+"/run", "application/json",
		bytes.NewReader(validBody))
	if err != nil {
		t.Fatalf("valid request after malformed: %v", err)
	}
	validBytes, _ := io.ReadAll(validResp.Body)
	validResp.Body.Close()

	if validResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for valid request after malformed, got %d: %s",
			validResp.StatusCode, string(validBytes))
	}

	events := mustUnmarshal[[]EventResponse](t, validBytes)
	if len(events) != 1 || events[0].Content.Parts[0].Text != "I am working!" {
		t.Error("valid request after malformed did not return expected data")
	}
}

// ---------------------------------------------------------------------------
// Test 6: missing required fields returns 400
// ---------------------------------------------------------------------------

func TestRunHandlerMissingFields(t *testing.T) {
	ag := newTestAgent("echo", "OK")
	s := newTestServer(t, ag)

	tests := []struct {
		name string
		body any
		want string
	}{
		{
			name: "missing message",
			body: RunRequest{
				AppName:   "testapp",
				UserID:    "user-1",
				SessionID: "sess-1",
			},
			want: "newMessage",
		},
		{
			name: "missing userId",
			body: RunRequest{
				AppName:   "testapp",
				SessionID: "sess-1",
				Message:   "Hi",
			},
			want: "userId",
		},
		{
			name: "missing sessionId",
			body: RunRequest{
				AppName: "testapp",
				UserID:  "user-1",
				Message: "Hi",
			},
			want: "sessionId",
		},
		{
			name: "unknown field rejected",
			body: map[string]any{
				"appName":   "testapp",
				"userId":    "user-1",
				"sessionId": "sess-1",
				"newMessage": "Hi",
				"badField":  "should fail",
			},
			want: "decode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Errorf("body missing %q: %s", tt.want, rec.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test 7: method not allowed
// ---------------------------------------------------------------------------

func TestRunHandlerMethodNotAllowed(t *testing.T) {
	ag := newTestAgent("echo", "OK")
	s := newTestServer(t, ag)

	req := httptest.NewRequest(http.MethodGet, "/run", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Test 8: SSE error event for errors
// ---------------------------------------------------------------------------

func TestSSEErrorEvent(t *testing.T) {
	ag, err := agent.New(agent.Config{
		Name: "failing",
		Description: "Agent that always fails",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return nil, fmt.Errorf("simulated agent failure")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t, ag)

	req := httptest.NewRequest(http.MethodPost, "/run_sse", makeJSONBody(RunRequest{
		AppName:   "testapp",
		UserID:    "user-1",
		SessionID: "sess-fail",
		Message:   "Hi",
	}))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Errorf("expected 'event: error' in SSE response, got: %s", body)
	}
	if !strings.Contains(body, "simulated agent failure") {
		t.Errorf("expected error message in SSE response, got: %s", body)
	}
}

// ---------------------------------------------------------------------------
// Test 9: session reuse across requests
// ---------------------------------------------------------------------------

func TestSessionReuseAcrossRequests(t *testing.T) {
	callCount := 0
	ag, err := agent.New(agent.Config{
		Name: "counter",
		Description: "Counts calls",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			callCount++
			ev := event.NewEvent(fmt.Sprintf("ev-%d", callCount), "counter", event.RoleModel)
			ev.Content = &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: fmt.Sprintf("call %d", callCount)}},
			}
			return []*event.Event{ev}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t, ag)

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	for i := 1; i <= 3; i++ {
		body, _ := json.Marshal(RunRequest{
			AppName:   "testapp",
			UserID:    "user-1",
			SessionID: "sess-reuse",
			Message:   fmt.Sprintf("msg %d", i),
		})
		resp, err := http.Post(ts.URL+"/run", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d: %s", i, resp.StatusCode, string(respBody))
		}
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

// ---------------------------------------------------------------------------
// Test 10: event with function call and function response
// ---------------------------------------------------------------------------

func TestEventConversionFunctionCalls(t *testing.T) {
	ev := &event.Event{
		ID:     "ev-fc",
		Author: "toolbot",
		Role:   event.RoleModel,
		Content: &event.Content{
			Role: event.RoleModel,
			Parts: []event.Part{
				{
					FunctionCall: &event.FunctionCall{
						ID:   "fc-1",
						Name: "get_weather",
						Args: map[string]any{"city": "Tokyo"},
					},
				},
				{
					FunctionResponse: &event.FunctionResponse{
						ID:     "fc-1",
						Name:   "get_weather",
						Result: map[string]any{"temp": 22},
					},
				},
			},
		},
		Actions: event.EventActions{
			StateDelta: map[string]any{"key": "val"},
			Escalate:   true,
		},
	}

	resp := EventToResponse(ev)

	if resp.Content == nil {
		t.Fatal("content is nil")
	}
	if len(resp.Content.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(resp.Content.Parts))
	}
	if resp.Content.Parts[0].FunctionCall == nil {
		t.Error("part 0 function call is nil")
	} else {
		if resp.Content.Parts[0].FunctionCall.Name != "get_weather" {
			t.Errorf("fc name = %q", resp.Content.Parts[0].FunctionCall.Name)
		}
	}
	if resp.Content.Parts[1].FunctionResponse == nil {
		t.Error("part 1 function response is nil")
	} else {
		if resp.Content.Parts[1].FunctionResponse.Name != "get_weather" {
			t.Errorf("fr name = %q", resp.Content.Parts[1].FunctionResponse.Name)
		}
	}
	if resp.Actions.StateDelta["key"] != "val" {
		t.Errorf("stateDelta key = %v", resp.Actions.StateDelta["key"])
	}
	if !resp.Actions.Escalate {
		t.Error("Escalate should be true")
	}
}
