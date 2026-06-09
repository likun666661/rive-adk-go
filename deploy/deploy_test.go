package deploy

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Cloud Run plan – deterministic content
// ---------------------------------------------------------------------------

func TestCloudRunPlanDeterministic(t *testing.T) {
	plan, err := PlanCloudRun(CloudRunConfig{
		EntryPoint:  "cmd/myserver/main.go",
		Project:     "my-project",
		Region:      "us-central1",
		ServiceName: "my-service",
		ServerPort:  8080,
		ProxyPort:   8081,
		Protocols:   []Protocol{ProtocolAPI, ProtocolWebUI},
	})
	if err != nil {
		t.Fatalf("PlanCloudRun: %v", err)
	}

	// Check key fields.
	if plan.EntryPoint != "cmd/myserver/main.go" {
		t.Errorf("EntryPoint = %q, want 'cmd/myserver/main.go'", plan.EntryPoint)
	}
	if plan.ExecFile != "cmd/myserver/main" {
		t.Errorf("ExecFile = %q, want 'cmd/myserver/main'", plan.ExecFile)
	}
	if plan.Project != "my-project" {
		t.Errorf("Project = %q", plan.Project)
	}
	if plan.Region != "us-central1" {
		t.Errorf("Region = %q", plan.Region)
	}
	if plan.ServiceName != "my-service" {
		t.Errorf("ServiceName = %q", plan.ServiceName)
	}
	if plan.ServerPort != 8080 {
		t.Errorf("ServerPort = %d", plan.ServerPort)
	}
	if plan.ProxyPort != 8081 {
		t.Errorf("ProxyPort = %d", plan.ProxyPort)
	}
	if len(plan.Protocols) != 2 {
		t.Errorf("Protocols len = %d, want 2", len(plan.Protocols))
	}

	// Dockerfile must contain expected lines.
	df := plan.Dockerfile()
	for _, want := range []string{
		"FROM gcr.io/distroless/static-debian11",
		"COPY cmd/myserver/main",
		"EXPOSE 8080",
		`"api"`,
		`"webui"`,
	} {
		if !strings.Contains(df, want) {
			t.Errorf("Dockerfile missing %q:\n%s", want, df)
		}
	}

	// Dockerfile must NOT contain protocol flags that weren't enabled.
	if strings.Contains(df, `"a2a"`) {
		t.Error("Dockerfile should not contain 'a2a' protocol")
	}
	if strings.Contains(df, `"agentengine"`) {
		t.Error("Dockerfile should not contain 'agentengine' protocol")
	}

	// Build command.
	bc := plan.BuildCmd()
	if bc != `go build -ldflags "-s -w" -o cmd/myserver/main cmd/myserver/main.go` {
		t.Errorf("BuildCmd = %q", bc)
	}

	// Proxy command.
	pc := plan.ProxyCmd()
	if !strings.Contains(pc, "gcloud run services proxy") {
		t.Errorf("ProxyCmd missing 'gcloud run services proxy': %q", pc)
	}
	if !strings.Contains(pc, "my-service") {
		t.Errorf("ProxyCmd missing service name: %q", pc)
	}

	// Lines()
	lines := plan.Lines()
	if len(lines) < 10 {
		t.Fatalf("expected at least 10 lines, got %d", len(lines))
	}

	// Verify specific markers in lines.
	joined := "\n" + strings.Join(lines, "\n") + "\n"
	for _, marker := range []string{
		"=== Cloud Run Dry-Run Plan: my-service ===",
		"Entry point:",
		"cmd/myserver/main.go",
		"--- Build ---",
		"CGO_ENABLED=0 GOOS=linux GOARCH=amd64",
		"--- Dockerfile ---",
		"--- Deploy ---",
		"--- Local Proxy ---",
		"Access:",
	} {
		if !strings.Contains(joined, marker) {
			t.Errorf("lines missing marker %q", marker)
		}
	}
}

func TestCloudRunPlanAllProtocols(t *testing.T) {
	plan, err := PlanCloudRun(CloudRunConfig{
		EntryPoint:  "server/main.go",
		Project:     "p",
		Region:      "r",
		ServiceName: "s",
		Protocols:   []Protocol{ProtocolAPI, ProtocolA2A, ProtocolWebUI, ProtocolPubSub, ProtocolEventarc},
	})
	if err != nil {
		t.Fatalf("PlanCloudRun: %v", err)
	}

	df := plan.Dockerfile()
	for _, want := range []string{`"api"`, `"a2a"`, `"webui"`, `"pubsub"`, `"eventarc"`} {
		if !strings.Contains(df, want) {
			t.Errorf("Dockerfile missing protocol %q:\n%s", want, df)
		}
	}
}

func TestCloudRunPlanDefaults(t *testing.T) {
	plan, err := PlanCloudRun(CloudRunConfig{
		EntryPoint:  "app/main.go",
		Project:     "proj",
		Region:      "reg",
		ServiceName: "svc",
	})
	if err != nil {
		t.Fatalf("PlanCloudRun: %v", err)
	}
	if plan.ServerPort != 8080 {
		t.Errorf("default ServerPort = %d, want 8080", plan.ServerPort)
	}
	if plan.ProxyPort != 8081 {
		t.Errorf("default ProxyPort = %d, want 8081", plan.ProxyPort)
	}
}

func TestCloudRunPlanStringIsDeterministic(t *testing.T) {
	cfg := CloudRunConfig{
		EntryPoint:  "cmd/myserver/main.go",
		Project:     "my-project",
		Region:      "us-central1",
		ServiceName: "my-service",
		Protocols:   []Protocol{ProtocolAPI, ProtocolWebUI},
	}

	plan1, err := PlanCloudRun(cfg)
	if err != nil {
		t.Fatalf("PlanCloudRun 1: %v", err)
	}
	plan2, err := PlanCloudRun(cfg)
	if err != nil {
		t.Fatalf("PlanCloudRun 2: %v", err)
	}

	if plan1.String() != plan2.String() {
		t.Errorf("Cloud Run plan output should be deterministic:\n--- first ---\n%s\n--- second ---\n%s",
			plan1.String(), plan2.String())
	}
}

// ---------------------------------------------------------------------------
// Agent Engine plan – deterministic content
// ---------------------------------------------------------------------------

func TestAgentEnginePlanDeterministic(t *testing.T) {
	plan, err := PlanAgentEngine(AgentEngineConfig{
		EntryPoint: "cmd/agent/main.go",
		Project:    "my-project",
		Region:     "us-central1",
		Name:       "my-agent-engine",
		ClassMethods: []ClassMethod{
			{Name: "async_stream_query", Path: "/stream_query", Method: "POST", Streaming: true},
			{Name: "async_create_session", Path: "/sessions", Method: "POST"},
		},
	})
	if err != nil {
		t.Fatalf("PlanAgentEngine: %v", err)
	}

	if plan.EntryPoint != "cmd/agent/main.go" {
		t.Errorf("EntryPoint = %q", plan.EntryPoint)
	}
	if plan.ExecFile != "cmd/agent/main" {
		t.Errorf("ExecFile = %q", plan.ExecFile)
	}
	if plan.Project != "my-project" {
		t.Errorf("Project = %q", plan.Project)
	}
	if plan.Region != "us-central1" {
		t.Errorf("Region = %q", plan.Region)
	}
	if plan.Name != "my-agent-engine" {
		t.Errorf("Name = %q", plan.Name)
	}

	// Dockerfile must contain expected lines.
	df := plan.Dockerfile()
	for _, want := range []string{
		"FROM golang:1.24 as builder",
		"FROM gcr.io/distroless/static-debian11",
		"cmd/agent/main",
		`"agentengine"`,
		"EXPOSE 8080",
	} {
		if !strings.Contains(df, want) {
			t.Errorf("Dockerfile missing %q:\n%s", want, df)
		}
	}

	// Build command.
	bc := plan.BuildCmd()
	if !strings.Contains(bc, "CGO_ENABLED=0") {
		t.Errorf("BuildCmd missing CGO_ENABLED: %q", bc)
	}

	// Stream URL.
	su := plan.StreamURL()
	if !strings.Contains(su, "us-central1-aiplatform.googleapis.com") {
		t.Errorf("StreamURL missing region: %q", su)
	}
	if !strings.Contains(su, "my-project") {
		t.Errorf("StreamURL missing project: %q", su)
	}
	if !strings.Contains(su, "my-agent-engine") {
		t.Errorf("StreamURL missing name: %q", su)
	}
	if !strings.Contains(su, "streamQuery") {
		t.Errorf("StreamURL missing streamQuery: %q", su)
	}

	// Lines.
	joined := "\n" + plan.String() + "\n"
	for _, marker := range []string{
		"=== Agent Engine Dry-Run Plan: my-agent-engine ===",
		"--- Source Archive ---",
		"--- Dockerfile ---",
		"--- Class Methods ---",
		"async_stream_query",
		"async_create_session",
		"--- Env / Secrets ---",
		"--- Deploy ---",
		"--- Stream Query Endpoint ---",
	} {
		if !strings.Contains(joined, marker) {
			t.Errorf("lines missing marker %q", marker)
		}
	}
}

func TestAgentEnginePlanDefaultClassMethods(t *testing.T) {
	plan, err := PlanAgentEngine(AgentEngineConfig{
		EntryPoint: "app/main.go",
		Project:    "p",
		Region:     "r",
		Name:       "engine",
	})
	if err != nil {
		t.Fatalf("PlanAgentEngine: %v", err)
	}

	lines := plan.Lines()
	joined := "\n" + strings.Join(lines, "\n") + "\n"

	for _, defaultMethod := range []string{
		"async_create_session",
		"async_get_session",
		"async_list_sessions",
		"async_delete_session",
		"async_stream_query",
	} {
		if !strings.Contains(joined, defaultMethod) {
			t.Errorf("lines missing default class method %q", defaultMethod)
		}
	}
}

func TestAgentEnginePlanWithMemoryBank(t *testing.T) {
	plan, err := PlanAgentEngine(AgentEngineConfig{
		EntryPoint:  "app/main.go",
		Project:     "p",
		Region:      "r",
		Name:        "engine",
		MemoryBank:  true,
		MemoryModel: "publishers/google/models/gemini-2.5-flash",
		MemoryTTL:   86400,
	})
	if err != nil {
		t.Fatalf("PlanAgentEngine: %v", err)
	}

	lines := plan.Lines()
	joined := "\n" + strings.Join(lines, "\n") + "\n"

	if !strings.Contains(joined, "--- Memory Bank ---") {
		t.Error("missing Memory Bank section")
	}
	if !strings.Contains(joined, "gemini-2.5-flash") {
		t.Error("missing memory model")
	}
}

func TestAgentEnginePlanStringIsDeterministic(t *testing.T) {
	cfg := AgentEngineConfig{
		EntryPoint: "cmd/agent/main.go",
		Project:    "my-project",
		Region:     "us-central1",
		Name:       "my-agent-engine",
		ClassMethods: []ClassMethod{
			{Name: "async_stream_query", Path: "/stream_query", Method: "POST", Streaming: true},
			{Name: "async_create_session", Path: "/sessions", Method: "POST"},
		},
	}

	plan1, err := PlanAgentEngine(cfg)
	if err != nil {
		t.Fatalf("PlanAgentEngine 1: %v", err)
	}
	plan2, err := PlanAgentEngine(cfg)
	if err != nil {
		t.Fatalf("PlanAgentEngine 2: %v", err)
	}

	if plan1.DisplayName != "my-agent-engine" {
		t.Errorf("DisplayName = %q, want deterministic service name", plan1.DisplayName)
	}
	if plan1.String() != plan2.String() {
		t.Errorf("Agent Engine plan output should be deterministic:\n--- first ---\n%s\n--- second ---\n%s",
			plan1.String(), plan2.String())
	}
}

// ---------------------------------------------------------------------------
// validation errors
// ---------------------------------------------------------------------------

func TestValidateEntryPoint(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"", true},
		{"   ", true},
		{"main.go", false},
		{"cmd/server/main.go", false},
		{"not_a_go_file", true},
		{"dir/", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			err := ValidateEntryPoint(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEntryPoint(%q) error = %v, wantErr = %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestValidateProjectName(t *testing.T) {
	if err := ValidateProjectName(""); err == nil {
		t.Error("empty project name should fail")
	}
	if err := ValidateProjectName("   "); err == nil {
		t.Error("whitespace-only project name should fail")
	}
	if err := ValidateProjectName("my-project"); err != nil {
		t.Errorf("valid project name should not fail: %v", err)
	}
}

func TestValidateRegion(t *testing.T) {
	if err := ValidateRegion(""); err == nil {
		t.Error("empty region should fail")
	}
	if err := ValidateRegion("us-central1"); err != nil {
		t.Errorf("valid region should not fail: %v", err)
	}
}

func TestValidateServiceName(t *testing.T) {
	if err := ValidateServiceName(""); err == nil {
		t.Error("empty service name should fail")
	}
	if err := ValidateServiceName("my-svc"); err != nil {
		t.Errorf("valid service name should not fail: %v", err)
	}
}

func TestValidateProtocols(t *testing.T) {
	if err := ValidateProtocols(nil); err != nil {
		t.Errorf("nil protocols should be valid: %v", err)
	}
	if err := ValidateProtocols([]Protocol{}); err != nil {
		t.Errorf("empty protocols should be valid: %v", err)
	}
	if err := ValidateProtocols([]Protocol{ProtocolAPI, ProtocolA2A}); err != nil {
		t.Errorf("valid protocols should not fail: %v", err)
	}
	if err := ValidateProtocols([]Protocol{"badprotocol"}); err == nil {
		t.Error("unknown protocol should fail")
	}
}

func TestValidateServerPort(t *testing.T) {
	tests := []struct {
		port    int
		wantErr bool
	}{
		{0, true},
		{-1, true},
		{65536, true},
		{1, false},
		{80, false},
		{8080, false},
		{65535, false},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			err := ValidateServerPort(tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateServerPort(%d) error = %v, wantErr = %v", tt.port, err, tt.wantErr)
			}
		})
	}
}

func TestValidateAll(t *testing.T) {
	err := ValidateAll(
		func() error { return nil },
		func() error { return ValidateEntryPoint("") },
		func() error { return ValidateProjectName("") },
	)
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "entry_point_path") || !strings.Contains(msg, "project_name") {
		t.Errorf("aggregated error should mention both failures: %s", msg)
	}
}

func TestValidateAllPasses(t *testing.T) {
	err := ValidateAll(
		func() error { return nil },
		func() error { return ValidateEntryPoint("main.go") },
		func() error { return ValidateProjectName("p") },
	)
	if err != nil {
		t.Errorf("all validators passed but got error: %v", err)
	}
}

func TestStripExtension(t *testing.T) {
	tests := []struct {
		name    string
		ext     string
		want    string
		wantErr bool
	}{
		{"main.go", ".go", "main", false},
		{"cmd/server/main.go", ".go", "cmd/server/main", false},
		{"main.go", ".py", "", true},
		{"noext", ".go", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := StripExtension(tt.name, tt.ext)
			if (err != nil) != tt.wantErr {
				t.Errorf("StripExtension(%q, %q) error = %v, wantErr = %v", tt.name, tt.ext, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("StripExtension(%q, %q) = %q, want %q", tt.name, tt.ext, got, tt.want)
			}
		})
	}
}

func TestCloudRunPlanValidationErrors(t *testing.T) {
	_, err := PlanCloudRun(CloudRunConfig{
		EntryPoint: "",
	})
	if err == nil {
		t.Fatal("expected validation errors for empty config")
	}
	msg := err.Error()
	for _, want := range []string{"entry_point", "project", "region", "service"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q: %s", want, msg)
		}
	}
}

func TestAgentEnginePlanValidationErrors(t *testing.T) {
	_, err := PlanAgentEngine(AgentEngineConfig{
		EntryPoint: "",
	})
	if err == nil {
		t.Fatal("expected validation errors for empty config")
	}
	msg := err.Error()
	for _, want := range []string{"entry_point", "project", "region", "service"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q: %s", want, msg)
		}
	}
}

func TestValidationErrorSingle(t *testing.T) {
	ve := &ValidationError{Errors: []string{"bad field"}}
	if ve.Error() != "bad field" {
		t.Errorf("single error = %q", ve.Error())
	}
}

func TestValidationErrorMultiple(t *testing.T) {
	ve := &ValidationError{Errors: []string{"a", "b"}}
	msg := ve.Error()
	if !strings.Contains(msg, "validation errors") {
		t.Errorf("multiple errors should say 'validation errors': %s", msg)
	}
}
