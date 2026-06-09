package deploy

import (
	"fmt"
	"strings"
	"time"
)

// ClassMethod represents a Reasoning Engine class method signature.
type ClassMethod struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path,omitempty"`
	Method      string `json:"method,omitempty"`
	Streaming   bool   `json:"streaming,omitempty"`
}

// AgentEnginePlan is a deterministic dry-run snapshot of an Agent Engine
// deploy. It captures the source archive plan, Dockerfile command, class
// methods, environment variables, and runtime command without executing
// any actual tar, build, or Vertex AI API calls.
type AgentEnginePlan struct {
	EntryPoint   string        `json:"entry_point"`
	ExecFile     string        `json:"exec_file"`
	Project      string        `json:"project"`
	Region       string        `json:"region"`
	Name         string        `json:"name"`
	DisplayName  string        `json:"display_name"`
	ServerPort   int           `json:"server_port"`
	SourceDir    string        `json:"source_dir"`
	ClassMethods []ClassMethod `json:"class_methods"`
	MemoryBank   bool          `json:"memory_bank,omitempty"`
	MemoryModel  string        `json:"memory_model,omitempty"`
	MemoryTTL    time.Duration `json:"memory_ttl,omitempty"`

	dockerfile string
	buildCmd   string
	streamURL  string
	lines      []string
}

// AgentEngineConfig holds parameters for an Agent Engine dry-run plan.
type AgentEngineConfig struct {
	EntryPoint   string
	Project      string
	Region       string
	Name         string
	ServerPort   int
	SourceDir    string
	ClassMethods []ClassMethod
	MemoryBank   bool
	MemoryModel  string
	MemoryTTL    time.Duration
}

var defaultAgentEngineConfig = AgentEngineConfig{
	ServerPort: 8080,
	SourceDir:  ".",
}

// PlanAgentEngine produces an Agent Engine dry-run deploy plan.
func PlanAgentEngine(cfg AgentEngineConfig) (*AgentEnginePlan, error) {
	if cfg.ServerPort == 0 {
		cfg.ServerPort = defaultAgentEngineConfig.ServerPort
	}
	if cfg.SourceDir == "" {
		cfg.SourceDir = defaultAgentEngineConfig.SourceDir
	}

	if err := ValidateAll(
		func() error { return ValidateEntryPoint(cfg.EntryPoint) },
		func() error { return ValidateProjectName(cfg.Project) },
		func() error { return ValidateRegion(cfg.Region) },
		func() error { return ValidateServiceName(cfg.Name) },
		func() error { return ValidateServerPort(cfg.ServerPort) },
	); err != nil {
		return nil, err
	}

	execFile, err := StripExtension(cfg.EntryPoint, ".go")
	if err != nil {
		return nil, fmt.Errorf("agentengine: %w", err)
	}

	plan := &AgentEnginePlan{
		EntryPoint:   cfg.EntryPoint,
		ExecFile:     execFile,
		Project:      cfg.Project,
		Region:       cfg.Region,
		Name:         cfg.Name,
		DisplayName:  cfg.Name,
		ServerPort:   cfg.ServerPort,
		SourceDir:    cfg.SourceDir,
		ClassMethods: cfg.ClassMethods,
		MemoryBank:   cfg.MemoryBank,
		MemoryModel:  cfg.MemoryModel,
		MemoryTTL:    cfg.MemoryTTL,
	}

	plan.dockerfile = agentEngineDockerfile(plan)
	plan.buildCmd = agentEngineBuildCmd(plan)
	plan.streamURL = agentEngineStreamURL(plan)
	plan.lines = agentEngineLines(plan)
	return plan, nil
}

// --- Dockerfile ---

func agentEngineDockerfile(p *AgentEnginePlan) string {
	var b strings.Builder
	b.WriteString("FROM golang:1.24 as builder\n")
	b.WriteString("WORKDIR /app\n")
	b.WriteString("COPY . .\n")
	b.WriteString(fmt.Sprintf("RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags \"-s -w\" -o %s %s\n\n",
		p.ExecFile, p.EntryPoint))
	b.WriteString("FROM gcr.io/distroless/static-debian11\n")
	b.WriteString(fmt.Sprintf("COPY --from=builder /app/%s  /app/%s\n", p.ExecFile, p.ExecFile))
	b.WriteString(fmt.Sprintf("EXPOSE %d\n", p.ServerPort))
	b.WriteString(fmt.Sprintf("CMD [\"/app/%s\", \"web\", \"-port\", \"%d\", \"agentengine\"]\n",
		p.ExecFile, p.ServerPort))
	return b.String()
}

// --- build command (in-docker) ---

func agentEngineBuildCmd(p *AgentEnginePlan) string {
	return fmt.Sprintf("CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags \"-s -w\" -o %s %s",
		p.ExecFile, p.EntryPoint)
}

// --- stream query URL ---

func agentEngineStreamURL(p *AgentEnginePlan) string {
	return fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1beta1/projects/%s/locations/%s/reasoningEngines/%s:streamQuery",
		p.Region, p.Project, p.Region, p.Name)
}

// --- lines ---

func agentEngineLines(p *AgentEnginePlan) []string {
	lines := []string{
		fmt.Sprintf("=== Agent Engine Dry-Run Plan: %s ===", p.Name),
		fmt.Sprintf(""),
		fmt.Sprintf("Entry point:       %s", p.EntryPoint),
		fmt.Sprintf("Binary:            %s", p.ExecFile),
		fmt.Sprintf("Project:           %s", p.Project),
		fmt.Sprintf("Region:            %s", p.Region),
		fmt.Sprintf("Name:              %s", p.Name),
		fmt.Sprintf("Display name:      %s", p.DisplayName),
		fmt.Sprintf("Server port:       %d", p.ServerPort),
		fmt.Sprintf("Source dir:        %s", p.SourceDir),
		fmt.Sprintf(""),
		fmt.Sprintf("--- Source Archive ---"),
		fmt.Sprintf("$ tar -czf archive.tgz -C %s --exclude=.git --exclude=adkgo . -C <tmpdir> Dockerfile", p.SourceDir),
		fmt.Sprintf(""),
		fmt.Sprintf("--- Dockerfile ---"),
		fmt.Sprintf("%s", strings.TrimRight(p.dockerfile, "\n")),
		fmt.Sprintf(""),
		fmt.Sprintf("--- Class Methods ---"),
	}

	if len(p.ClassMethods) == 0 {
		lines = append(lines, "  (default Agent Engine class methods)")
		lines = append(lines, "  - async_create_session   (POST /sessions)")
		lines = append(lines, "  - async_get_session      (GET /sessions/{id})")
		lines = append(lines, "  - async_list_sessions    (GET /sessions)")
		lines = append(lines, "  - async_delete_session   (DELETE /sessions/{id})")
		lines = append(lines, "  - async_stream_query     (POST /stream_query)")
	} else {
		for _, cm := range p.ClassMethods {
			stream := ""
			if cm.Streaming {
				stream = " [streaming]"
			}
			lines = append(lines, fmt.Sprintf("  - %-24s %s %s", cm.Name, cm.Path, stream))
		}
	}

	lines = append(lines, "")
	lines = append(lines, "--- Env / Secrets ---")
	lines = append(lines, "  GOOGLE_CLOUD_REGION:                           "+p.Region)
	lines = append(lines, "  NUM_WORKERS:                                   1")
	lines = append(lines, "  GOOGLE_CLOUD_AGENT_ENGINE_ENABLE_TELEMETRY:    true")
	lines = append(lines, "  OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT: true")
	lines = append(lines, "  GOOGLE_API_KEY (secret):                       GOOGLE_API_KEY:latest")
	lines = append(lines, "")

	lines = append(lines, "--- Deploy ---")
	lines = append(lines, fmt.Sprintf("API: POST https://%s-aiplatform.googleapis.com/v1beta1/", p.Region))
	lines = append(lines, fmt.Sprintf("     projects/%s/locations/%s/reasoningEngines", p.Project, p.Region))
	lines = append(lines, fmt.Sprintf("Agent framework:  google-adk"))
	lines = append(lines, "")

	if p.MemoryBank {
		lines = append(lines, "--- Memory Bank ---")
		lines = append(lines, fmt.Sprintf("  Model:            %s", p.MemoryModel))
		lines = append(lines, fmt.Sprintf("  TTL:              %s", p.MemoryTTL))
		lines = append(lines, "")
	}

	lines = append(lines, "--- Stream Query Endpoint ---")
	lines = append(lines, p.streamURL)

	return lines
}

// --- Plan interface ---

func (p *AgentEnginePlan) String() string {
	return strings.Join(p.lines, "\n")
}

func (p *AgentEnginePlan) Lines() []string {
	return p.lines
}

// Dockerfile returns the computed Dockerfile content.
func (p *AgentEnginePlan) Dockerfile() string {
	return p.dockerfile
}

// BuildCmd returns the build command.
func (p *AgentEnginePlan) BuildCmd() string {
	return p.buildCmd
}

// StreamURL returns the stream query endpoint URL.
func (p *AgentEnginePlan) StreamURL() string {
	return p.streamURL
}
