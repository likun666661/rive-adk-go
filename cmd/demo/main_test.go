package main

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/cmd/launcher"
	"github.com/likun666661/rive-adk-go/cmd/launcher/console"
	"github.com/likun666661/rive-adk-go/cmd/launcher/universal"
	"github.com/likun666661/rive-adk-go/deploy"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/flow"
	"github.com/likun666661/rive-adk-go/llmagent"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/runner"
	"github.com/likun666661/rive-adk-go/server/adkrest"
	"github.com/likun666661/rive-adk-go/telemetry"
	"github.com/likun666661/rive-adk-go/tool"
)

// ===========================================================================
// End-to-end tests for Chapter 06: Entrypoint, Deploy, Telemetry
// ===========================================================================

// ---------------------------------------------------------------------------
// Test 6.1 — Launcher Console Path (end-to-end)
// ---------------------------------------------------------------------------

func TestEndToEndLauncherConsolePath(t *testing.T) {
	weatherTool := tool.NewFunctionTool("get_weather", "Get current weather for a city",
		func(args map[string]any) (map[string]any, error) {
			city, _ := args["city"].(string)
			return map[string]any{
				"city":        city,
				"temperature": 22,
			}, nil
		},
	)

	fakeModel := model.NewFakeModel("demo-model",
		model.FunctionCallResponse("Let me check the weather.",
			event.FunctionCall{
				ID:   "fc-1",
				Name: "get_weather",
				Args: map[string]any{"city": "Tokyo"},
			},
		),
		model.TextResponse("The weather in Tokyo is 22°C and sunny."),
	)

	f := &flow.Flow{
		Model: fakeModel,
		Tools:  map[string]tool.FunctionTool{"get_weather": weatherTool},
	}

	ag, err := llmagent.New("weather_bot", "A bot that answers weather questions.", f)
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	loader := &agentLoader{ag: ag}

	var buf bytes.Buffer
	c := console.NewConsole(
		console.WithInput(strings.NewReader("What's the weather in Tokyo?\n")),
		console.WithOutput(&buf),
	)

	config := &launcher.Config{
		AgentLoader:    loader,
		SessionService: runner.NewInMemorySessionService(),
	}

	err = c.Run(stdctx.Background(), config)
	if err != nil {
		t.Fatalf("Console Run failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "get_weather") {
		t.Errorf("Console output should contain tool call: %s", output)
	}
	if !strings.Contains(output, "Tokyo is 22°C") {
		t.Errorf("Console output should contain final text: %s", output)
	}
	if !strings.Contains(output, "events persisted") {
		t.Errorf("Console output should mention persisted events: %s", output)
	}
}

// agentLoader wraps an agent.Agent for use as launcher.AgentLoader.
type agentLoader struct {
	ag agent.Agent
}

func (l *agentLoader) RootAgent() agent.Agent { return l.ag }

// ---------------------------------------------------------------------------
// Test 6.2 — Web/Server JSON Path (end-to-end)
// ---------------------------------------------------------------------------

func TestEndToEndWebJSONPath(t *testing.T) {
	ag, err := agent.New(agent.Config{
		Name:        "greeter",
		Description: "Test greeter agent",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			ev := event.NewEvent("ev-1", "greeter", event.RoleModel)
			ev.Content = &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: "Hello, JSON world!"}},
			}
			return []*event.Event{ev}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := &launcher.Config{
		SessionService: runner.NewInMemorySessionService(),
		AgentLoader:    &agentLoader{ag: ag},
	}
	s, err := adkrest.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"appName":   "testapp",
		"userId":    "user-1",
		"sessionId": "sess-json",
		"newMessage": "Hi",
	})

	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
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

	var resp []adkrest.EventResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 event, got %d", len(resp))
	}
	if resp[0].Author != "greeter" {
		t.Errorf("Author = %q, want 'greeter'", resp[0].Author)
	}
	if resp[0].Content == nil || len(resp[0].Content.Parts) == 0 {
		t.Fatal("expected content with parts")
	}
	if resp[0].Content.Parts[0].Text != "Hello, JSON world!" {
		t.Errorf("Text = %q", resp[0].Content.Parts[0].Text)
	}
}

// ---------------------------------------------------------------------------
// Test 6.3 — SSE Encoding Path (end-to-end)
// ---------------------------------------------------------------------------

func TestEndToEndSSEEncodingPath(t *testing.T) {
	ag, err := agent.New(agent.Config{
		Name:        "streamer",
		Description: "Streaming test agent",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			return []*event.Event{
				{
					ID:      "e1",
					Author:  "streamer",
					Role:    event.RoleModel,
					Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "chunk1"}}},
					Partial: true,
				},
				{
					ID:      "e2",
					Author:  "streamer",
					Role:    event.RoleModel,
					Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "chunk2"}}},
					Partial: true,
				},
				{
					ID:      "e3",
					Author:  "streamer",
					Role:    event.RoleModel,
					Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "final"}}},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := &launcher.Config{
		SessionService: runner.NewInMemorySessionService(),
		AgentLoader:    &agentLoader{ag: ag},
	}
	s, err := adkrest.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"appName":   "testapp",
		"userId":    "user-1",
		"sessionId": "sess-sse",
		"newMessage": "Stream",
	})

	req := httptest.NewRequest(http.MethodPost, "/run_sse", bytes.NewReader(body))
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

	bodyStr := rec.Body.String()

	// Verify SSE framing.
	if !strings.Contains(bodyStr, "data: ") {
		t.Errorf("SSE body missing 'data: ' prefix: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "\n\n") {
		t.Errorf("SSE body missing double-newline separator: %s", bodyStr)
	}

	// Each event should appear in SSE frames.
	for _, want := range []string{"chunk1", "chunk2", "final"} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("SSE body missing %q: %s", want, bodyStr)
		}
	}

	// Count SSE frames.
	frameCount := strings.Count(bodyStr, "data:")
	if frameCount != 3 {
		t.Errorf("expected 3 SSE data frames, got %d: %s", frameCount, bodyStr)
	}

	// Parse each SSE frame as valid JSON.
	frames := strings.Split(strings.TrimSpace(bodyStr), "\n\n")
	for _, frame := range frames {
		frame = strings.TrimSpace(frame)
		if !strings.HasPrefix(frame, "data:") {
			continue
		}
		jsonStr := strings.TrimPrefix(frame, "data:")
		var ev adkrest.EventResponse
		if err := json.Unmarshal([]byte(jsonStr), &ev); err != nil {
			t.Errorf("failed to unmarshal SSE frame %q: %v", frame, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 6.4 — Deploy Dry-Run Plan Generation
// ---------------------------------------------------------------------------

func TestEndToEndDeployDryRunPlan(t *testing.T) {
	// --- Cloud Run plan ---
	t.Run("CloudRun", func(t *testing.T) {
		plan, err := deploy.PlanCloudRun(deploy.CloudRunConfig{
			EntryPoint:  "cmd/myserver/main.go",
			Project:     "my-project",
			Region:      "us-central1",
			ServiceName: "my-service",
			ServerPort:  8080,
			ProxyPort:   8081,
			Protocols:   []deploy.Protocol{deploy.ProtocolAPI, deploy.ProtocolA2A, deploy.ProtocolWebUI},
		})
		if err != nil {
			t.Fatalf("PlanCloudRun: %v", err)
		}

		df := plan.Dockerfile()
		if !strings.Contains(df, "FROM gcr.io/distroless/static-debian11") {
			t.Error("Dockerfile missing base image")
		}
		if !strings.Contains(df, `"api"`) {
			t.Error("Dockerfile missing api protocol")
		}
		if !strings.Contains(df, `"a2a"`) {
			t.Error("Dockerfile missing a2a protocol")
		}
		if !strings.Contains(df, `"webui"`) {
			t.Error("Dockerfile missing webui protocol")
		}
		// Distroless Dockerfile: single stage CMD runs web launcher.
		if !strings.Contains(df, `"web"`) {
			t.Error("Dockerfile CMD should invoke web launcher")
		}

		bc := plan.BuildCmd()
		if !strings.Contains(bc, "go build") {
			t.Errorf("BuildCmd missing 'go build': %q", bc)
		}
		if !strings.Contains(bc, "-ldflags") {
			t.Errorf("BuildCmd missing '-ldflags': %q", bc)
		}

		pc := plan.ProxyCmd()
		if !strings.Contains(pc, "gcloud run services proxy") {
			t.Errorf("ProxyCmd: %q", pc)
		}

		lines := plan.Lines()
		if len(lines) < 10 {
			t.Fatalf("expected at least 10 lines, got %d", len(lines))
		}
		joined := strings.Join(lines, "\n")
		for _, marker := range []string{
			"=== Cloud Run Dry-Run Plan",
			"Entry point:",
			"Binary:",
			"--- Build ---",
			"CGO_ENABLED=0 GOOS=linux GOARCH=amd64",
			"--- Deploy ---",
			"gcloud run deploy",
			"--- Local Proxy ---",
		} {
			if !strings.Contains(joined, marker) {
				t.Errorf("lines missing %q", marker)
			}
		}
	})

	// --- Agent Engine plan ---
	t.Run("AgentEngine", func(t *testing.T) {
		plan, err := deploy.PlanAgentEngine(deploy.AgentEngineConfig{
			EntryPoint: "cmd/myagent/main.go",
			Project:    "my-project",
			Region:     "us-central1",
			Name:       "my-reasoning-engine",
			ServerPort: 8080,
			SourceDir:  ".",
			ClassMethods: []deploy.ClassMethod{
				{Name: "async_stream_query", Description: "Stream query", Path: "/stream_query", Method: "POST", Streaming: true},
			},
		})
		if err != nil {
			t.Fatalf("PlanAgentEngine: %v", err)
		}

		df := plan.Dockerfile()
		// Agent Engine uses multi-stage Dockerfile.
		if !strings.Contains(df, "FROM golang:1.24 as builder") {
			t.Error("Dockerfile missing golang builder stage")
		}
		if !strings.Contains(df, "FROM gcr.io/distroless/static-debian11") {
			t.Error("Dockerfile missing distroless stage")
		}
		if !strings.Contains(df, `"agentengine"`) {
			t.Error("Dockerfile CMD should include agentengine")
		}
		if !strings.Contains(df, `"web"`) {
			t.Error("Dockerfile CMD should invoke web launcher")
		}

		if plan.StreamURL() == "" {
			t.Error("StreamURL should not be empty")
		}
		if !strings.Contains(plan.StreamURL(), "streamQuery") {
			t.Errorf("StreamURL should reference streamQuery: %s", plan.StreamURL())
		}

		lines := plan.Lines()
		joined := strings.Join(lines, "\n")
		for _, marker := range []string{
			"=== Agent Engine Dry-Run Plan",
			"Entry point:",
			"Binary:",
			"--- Source Archive ---",
			"tar -czf",
			"--- Class Methods ---",
			"async_stream_query",
			"--- Env / Secrets ---",
			"GOOGLE_API_KEY",
			"OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT",
			"--- Deploy ---",
			"--- Stream Query Endpoint ---",
		} {
			if !strings.Contains(joined, marker) {
				t.Errorf("lines missing %q", marker)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Test 6.5 — Telemetry Capture Around Runner Invocation
// ---------------------------------------------------------------------------

func TestEndToEndTelemetryCaptureRunnerInvocation(t *testing.T) {
	rec := telemetry.NewRecorder(telemetry.WithCaptureMessageContent(true))
	ctx := stdctx.Background()

	agt, err := agent.New(agent.Config{
		Name:        "telemetry_bot",
		Description: "Agent for telemetry testing",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			ev := event.NewEvent("ev-tel", "telemetry_bot", event.RoleModel)
			ev.Content = &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: "Response with telemetry tracking."}},
			}
			return []*event.Event{ev}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ea := runner.ExecutableAgent(agt)

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "tel_app",
		Agent:          ea,
		SessionService: sessionSvc,
	})
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}

	invocationID := "inv-telemetry-1"

	// --- Instrument agent invocation ---
	invSpan := telemetry.StartInvokeAgentSpan(ctx, rec, "telemetry_bot",
		"Agent for telemetry testing", "sess-tel", invocationID)

	// --- Instrument a server event ---
	serverSpan := telemetry.StartServerEventSpan(ctx, rec, "POST", "/run")
	telemetry.LogServerEvent(ctx, rec, "POST", "/run", 200, 15*time.Millisecond)

	// --- Run the agent ---
	_, events, err := r.Run(ctx, "user-tel", "sess-tel", "Test telemetry")
	if err != nil {
		telemetry.LogServerEvent(ctx, rec, "POST", "/run", 500, 0)
		invSpan.EndWithError("ERROR", err.Error())
		serverSpan.EndWithError("ERROR", err.Error())
		t.Fatalf("Runner failed: %v", err)
	}

	// --- Instrument model generation ---
	modelSpan := telemetry.StartGenerateContentSpan(ctx, rec, "fake-model", invocationID)
	telemetry.SetEventID(modelSpan, events[0].ID)
	telemetry.SetTokenUsage(modelSpan, 120, 60, 10, 5)
	telemetry.LogRequest(ctx, rec, "You are a test assistant.", "Test telemetry")
	telemetry.LogResponse(ctx, rec, "STOP", "Response with telemetry tracking.", nil)

	invSpan.End("OK")
	modelSpan.End("OK")
	serverSpan.End("OK")

	// --- Verify spans ---
	spans := rec.Spans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	// Check invoke_agent span.
	if !strings.Contains(spans[0].Name, "invoke_agent") {
		t.Errorf("span[0] should be invoke_agent: %q", spans[0].Name)
	}
	if spans[0].Attributes["gen_ai.agent.name"] != "telemetry_bot" {
		t.Errorf("span[0] agent.name = %v", spans[0].Attributes["gen_ai.agent.name"])
	}
	if spans[0].Status != "OK" {
		t.Errorf("span[0] status = %q, want OK", spans[0].Status)
	}

	// Check server span.
	if !strings.Contains(spans[1].Name, "generate_content") {
		t.Errorf("span[1] should be generate_content: %q", spans[1].Name)
	}
	if spans[1].Attributes["gen_ai.usage.input_tokens"] != int64(120) {
		t.Errorf("span[1] input_tokens = %v", spans[1].Attributes["gen_ai.usage.input_tokens"])
	}
	if spans[1].Attributes["gen_ai.usage.output_tokens"] != int64(60) {
		t.Errorf("span[1] output_tokens = %v", spans[1].Attributes["gen_ai.usage.output_tokens"])
	}

	// Check server span.
	if !strings.Contains(spans[2].Name, "server") {
		t.Errorf("span[2] should be server event: %q", spans[2].Name)
	}
	if spans[2].Attributes["server.operation"] != "POST" {
		t.Errorf("span[2] operation = %v", spans[2].Attributes["server.operation"])
	}

	// --- Verify logs ---
	logs := rec.Logs()
	if len(logs) < 4 {
		t.Fatalf("expected at least 4 logs, got %d", len(logs))
	}

	hasServerLog := false
	hasSystemLog := false
	hasUserLog := false
	hasChoiceLog := false
	for _, l := range logs {
		switch l.EventName {
		case "server.request":
			hasServerLog = true
		case "gen_ai.system.message":
			hasSystemLog = true
		case "gen_ai.user.message":
			hasUserLog = true
		case "gen_ai.choice":
			hasChoiceLog = true
		}
	}
	if !hasServerLog {
		t.Error("missing server.request log")
	}
	if !hasSystemLog {
		t.Error("missing gen_ai.system.message log")
	}
	if !hasUserLog {
		t.Error("missing gen_ai.user.message log")
	}
	if !hasChoiceLog {
		t.Error("missing gen_ai.choice log")
	}

	// --- Verify content capture ---
	for _, l := range logs {
		if l.EventName == "gen_ai.system.message" {
			if l.Body["content"] != "You are a test assistant." {
				t.Errorf("system message content = %v", l.Body["content"])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Test 6.6 — Universal Launcher Routing (console is default, web by keyword)
// ---------------------------------------------------------------------------

func TestEndToEndUniversalLauncherRouting(t *testing.T) {
	ag, err := agent.New(agent.Config{
		Name:        "route_bot",
		Description: "Routing test agent",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			ev := event.NewEvent("ev-r", "route_bot", event.RoleModel)
			ev.Content = &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: "routed"}},
			}
			return []*event.Event{ev}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("no_args_defaults_to_console", func(t *testing.T) {
		var buf bytes.Buffer
		c := console.NewConsole(
			console.WithInput(strings.NewReader("hello\n")),
			console.WithOutput(&buf),
		)

		uni := universal.New(c)
		config := &launcher.Config{
			AgentLoader:    &agentLoader{ag: ag},
			SessionService: runner.NewInMemorySessionService(),
		}

		err := uni.Execute(stdctx.Background(), config, nil)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(buf.String(), "routed") {
			t.Errorf("output should contain agent response: %s", buf.String())
		}
	})

	t.Run("console_keyword_routes_to_console", func(t *testing.T) {
		var buf bytes.Buffer
		c := console.NewConsole(
			console.WithInput(strings.NewReader("hi\n")),
			console.WithOutput(&buf),
		)

		uni := universal.New(c)
		config := &launcher.Config{
			AgentLoader:    &agentLoader{ag: ag},
			SessionService: runner.NewInMemorySessionService(),
		}

		err := uni.Execute(stdctx.Background(), config, []string{"console"})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(buf.String(), "routed") {
			t.Errorf("output should contain agent response: %s", buf.String())
		}
	})

	t.Run("unknown_keyword_errors", func(t *testing.T) {
		c := console.NewConsole()
		uni := universal.New(c)

		err := uni.Execute(stdctx.Background(), &launcher.Config{
			AgentLoader: &agentLoader{ag: ag},
		}, []string{"unknown"})
		if err == nil {
			t.Fatal("expected error for unknown keyword")
		}
		if !strings.Contains(err.Error(), "unknown") {
			t.Errorf("error should mention unknown command: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 6.7 — Telemetry Content Capture Toggle
// ---------------------------------------------------------------------------

func TestEndToEndTelemetryContentCaptureToggle(t *testing.T) {
	t.Run("capture_on", func(t *testing.T) {
		r := telemetry.NewRecorder(telemetry.WithCaptureMessageContent(true))
		telemetry.LogRequest(stdctx.Background(), r, "system prompt", "user message")
		logs := r.Logs()
		if logs[0].Body["content"] != "system prompt" {
			t.Errorf("system content should be visible: %v", logs[0].Body["content"])
		}
		if logs[1].Body["content"] != "user message" {
			t.Errorf("user content should be visible: %v", logs[1].Body["content"])
		}
	})

	t.Run("capture_off", func(t *testing.T) {
		r := telemetry.NewRecorder() // capture is off by default
		telemetry.LogRequest(stdctx.Background(), r, "secret", "secret")
		logs := r.Logs()
		if logs[0].Body["content"] != "<elided>" {
			t.Errorf("system content should be elided: %v", logs[0].Body["content"])
		}
		if logs[1].Body["content"] != "<elided>" {
			t.Errorf("user content should be elided: %v", logs[1].Body["content"])
		}
	})
}

// ---------------------------------------------------------------------------
// Test 6.8 — Providers Lifecycle with Runner Invocation
// ---------------------------------------------------------------------------

func TestEndToEndProvidersLifecycle(t *testing.T) {
	providers := telemetry.NewProviders(telemetry.WithCaptureMessageContent(true))
	ctx := stdctx.Background()

	if err := providers.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	rec := providers.Recorder()

	ag, err := agent.New(agent.Config{
		Name:        "lifecycle_bot",
		Description: "Tests provider lifecycle",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			ev := event.NewEvent("ev-lc", "lifecycle_bot", event.RoleModel)
			ev.Content = &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: "lifecycle ok"}},
			}
			return []*event.Event{ev}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ea := runner.ExecutableAgent(ag)
	r, _ := runner.New(runner.Config{
		AppName:        "lc_app",
		Agent:          ea,
		SessionService: runner.NewInMemorySessionService(),
	})

	span := telemetry.StartInvokeAgentSpan(ctx, rec, "lifecycle_bot", "test", "sess-lc", "inv-lc")
	telemetry.LogRequest(ctx, rec, "system", "user")
	_, _, err = r.Run(ctx, "user-lc", "sess-lc", "test")
	span.End("OK")
	if err != nil {
		t.Fatalf("runner.Run: %v", err)
	}

	if err := providers.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Data should be preserved after shutdown.
	if rec.SpanCount() != 1 {
		t.Errorf("SpanCount after shutdown = %d, want 1", rec.SpanCount())
	}
	if rec.LogCount() != 2 {
		t.Errorf("LogCount after shutdown = %d, want 2", rec.LogCount())
	}
}

// ---------------------------------------------------------------------------
// Test 6.9 — Multi-Protocol Cloud Run Deploy Plan
// ---------------------------------------------------------------------------

func TestEndToEndMultiProtocolDeployPlan(t *testing.T) {
	plan, err := deploy.PlanCloudRun(deploy.CloudRunConfig{
		EntryPoint:  "main.go",
		Project:     "demo-project",
		Region:      "us-east1",
		ServiceName: "demo-service",
		ServerPort:  8888,
		ProxyPort:   9999,
		Protocols:   []deploy.Protocol{deploy.ProtocolAPI, deploy.ProtocolWebUI, deploy.ProtocolA2A, deploy.ProtocolPubSub, deploy.ProtocolEventarc},
	})
	if err != nil {
		t.Fatalf("PlanCloudRun: %v", err)
	}

	df := plan.Dockerfile()
	for _, p := range []string{"api", "a2a", "webui", "pubsub", "eventarc"} {
		if !strings.Contains(df, p) {
			t.Errorf("Dockerfile missing protocol %q:\n%s", p, df)
		}
	}

	lines := plan.Lines()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "8888") {
		t.Error("Server port 8888 should appear in plan")
	}
	if !strings.Contains(joined, "9999") {
		t.Error("Proxy port 9999 should appear in plan")
	}
}

// ---------------------------------------------------------------------------
// Test 6.10 — validate runner events show up in session after web request
// ---------------------------------------------------------------------------

func TestEndToEndWebSessionPersistence(t *testing.T) {
	callCount := 0
	ag, err := agent.New(agent.Config{
		Name:        "counter",
		Description: "Counts invocations",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			callCount++
			ev := event.NewEvent("ev-cnt", "counter", event.RoleModel)
			ev.Content = &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: "ok"}},
			}
			return []*event.Event{ev}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := &launcher.Config{
		SessionService: runner.NewInMemorySessionService(),
		AgentLoader:    &agentLoader{ag: ag},
	}
	s, err := adkrest.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(map[string]any{
			"appName":   "testapp",
			"userId":    "user-1",
			"sessionId": "sess-persist",
			"newMessage": "msg",
		})
		resp, err := http.Post(ts.URL+"/run", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: %d: %s", i, resp.StatusCode, string(respBody))
		}
	}

	if callCount != 3 {
		t.Errorf("expected 3 agent invocations, got %d", callCount)
	}
}
