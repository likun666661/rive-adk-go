package deploy

import (
	"fmt"
	"sort"
	"strings"
)

// CloudRunPlan is a deterministic dry-run snapshot of a Cloud Run deploy.
// It captures the build target, Dockerfile command, enabled protocols,
// secrets, and proxy URL hints without executing any actual build or
// network operation.
type CloudRunPlan struct {
	EntryPoint   string     `json:"entry_point"`
	ExecFile     string     `json:"exec_file"`
	Project      string     `json:"project"`
	Region       string     `json:"region"`
	ServiceName  string     `json:"service_name"`
	ServerPort   int        `json:"server_port"`
	ProxyPort    int        `json:"proxy_port"`
	Protocols    []Protocol `json:"protocols"`
	A2AAgentURL  string     `json:"a2a_agent_url,omitempty"`
	WebUIAddress string     `json:"webui_address,omitempty"`

	dockerfile string
	buildCmd   string
	proxyCmd   string
	lines      []string
}

// CloudRunConfig holds parameters for a Cloud Run dry-run plan.
type CloudRunConfig struct {
	EntryPoint   string
	Project      string
	Region       string
	ServiceName  string
	ServerPort   int
	ProxyPort    int
	Protocols    []Protocol
	A2AAgentURL  string
	WebUIAddress string
}

var defaultCloudRunConfig = CloudRunConfig{
	ServerPort:   8080,
	ProxyPort:    8081,
	A2AAgentURL:  "http://127.0.0.1:8081",
	WebUIAddress: "http://127.0.0.1:8081/api",
}

// PlanCloudRun produces a Cloud Run dry-run deploy plan.
// It validates inputs and returns a deterministic plan. No external
// commands are invoked.
func PlanCloudRun(cfg CloudRunConfig) (*CloudRunPlan, error) {
	if cfg.ServerPort == 0 {
		cfg.ServerPort = defaultCloudRunConfig.ServerPort
	}
	if cfg.ProxyPort == 0 {
		cfg.ProxyPort = defaultCloudRunConfig.ProxyPort
	}
	if cfg.A2AAgentURL == "" {
		cfg.A2AAgentURL = defaultCloudRunConfig.A2AAgentURL
	}
	if cfg.WebUIAddress == "" {
		cfg.WebUIAddress = defaultCloudRunConfig.WebUIAddress
	}

	if err := ValidateAll(
		func() error { return ValidateEntryPoint(cfg.EntryPoint) },
		func() error { return ValidateProjectName(cfg.Project) },
		func() error { return ValidateRegion(cfg.Region) },
		func() error { return ValidateServiceName(cfg.ServiceName) },
		func() error { return ValidateServerPort(cfg.ServerPort) },
		func() error { return ValidateProtocols(cfg.Protocols) },
	); err != nil {
		return nil, err
	}

	execFile, err := StripExtension(cfg.EntryPoint, ".go")
	if err != nil {
		return nil, fmt.Errorf("cloudrun: %w", err)
	}

	plan := &CloudRunPlan{
		EntryPoint:   cfg.EntryPoint,
		ExecFile:     execFile,
		Project:      cfg.Project,
		Region:       cfg.Region,
		ServiceName:  cfg.ServiceName,
		ServerPort:   cfg.ServerPort,
		ProxyPort:    cfg.ProxyPort,
		Protocols:    cfg.Protocols,
		A2AAgentURL:  cfg.A2AAgentURL,
		WebUIAddress: cfg.WebUIAddress,
	}

	plan.dockerfile = cloudRunDockerfile(plan)
	plan.buildCmd = cloudRunBuildCmd(plan)
	plan.proxyCmd = cloudRunProxyCmd(plan)
	plan.lines = cloudRunLines(plan)
	return plan, nil
}

// --- Dockerfile ---

func cloudRunDockerfile(p *CloudRunPlan) string {
	var b strings.Builder
	b.WriteString("FROM gcr.io/distroless/static-debian11\n")
	b.WriteString(fmt.Sprintf("COPY %s  /app/%s\n", p.ExecFile, p.ExecFile))
	b.WriteString(fmt.Sprintf("EXPOSE %d\n", p.ServerPort))
	b.WriteString(fmt.Sprintf("CMD [\"/app/%s\", \"web\", \"-port\", \"%d\"", p.ExecFile, p.ServerPort))

	for _, proto := range p.Protocols {
		switch proto {
		case ProtocolAPI:
			b.WriteString(fmt.Sprintf(`, "api", "-webui_address", "%s"`, p.WebUIAddress))
		case ProtocolA2A:
			b.WriteString(fmt.Sprintf(`, "a2a", "--a2a_agent_url", "%s"`, p.A2AAgentURL))
		case ProtocolWebUI:
			b.WriteString(fmt.Sprintf(`, "webui", "--api_server_address", "%s"`, p.WebUIAddress))
		case ProtocolPubSub:
			b.WriteString(`, "pubsub"`)
		case ProtocolEventarc:
			b.WriteString(`, "eventarc"`)
		}
	}
	b.WriteString("]\n")
	return b.String()
}

// --- build command ---

func cloudRunBuildCmd(p *CloudRunPlan) string {
	return fmt.Sprintf("go build -ldflags \"-s -w\" -o %s %s", p.ExecFile, p.EntryPoint)
}

// --- proxy command ---

func cloudRunProxyCmd(p *CloudRunPlan) string {
	return fmt.Sprintf("gcloud run services proxy %s --project %s --port %d --region %s",
		p.ServiceName, p.Project, p.ProxyPort, p.Region)
}

// --- lines ---

func cloudRunLines(p *CloudRunPlan) []string {
	return []string{
		fmt.Sprintf("=== Cloud Run Dry-Run Plan: %s ===", p.ServiceName),
		fmt.Sprintf(""),
		fmt.Sprintf("Entry point:       %s", p.EntryPoint),
		fmt.Sprintf("Binary:            %s", p.ExecFile),
		fmt.Sprintf("Project:           %s", p.Project),
		fmt.Sprintf("Region:            %s", p.Region),
		fmt.Sprintf("Service:           %s", p.ServiceName),
		fmt.Sprintf("Server port:       %d", p.ServerPort),
		fmt.Sprintf("Proxy port:        %d", p.ProxyPort),
		fmt.Sprintf("Protocols:         %s", formatProtocols(p.Protocols)),
		fmt.Sprintf("A2A agent URL:     %s", p.A2AAgentURL),
		fmt.Sprintf("WebUI address:     %s", p.WebUIAddress),
		fmt.Sprintf(""),
		fmt.Sprintf("--- Build ---"),
		fmt.Sprintf("CGO_ENABLED=0 GOOS=linux GOARCH=amd64"),
		fmt.Sprintf("$ %s", p.buildCmd),
		fmt.Sprintf(""),
		fmt.Sprintf("--- Dockerfile ---"),
		fmt.Sprintf("%s", strings.TrimRight(p.dockerfile, "\n")),
		fmt.Sprintf(""),
		fmt.Sprintf("--- Deploy ---"),
		fmt.Sprintf("$ gcloud run deploy %s --region %s --project %s \\", p.ServiceName, p.Region, p.Project),
		fmt.Sprintf("    --source . --set-secrets=GOOGLE_API_KEY=GOOGLE_API_KEY:latest \\"),
		fmt.Sprintf("    --ingress all --no-allow-unauthenticated"),
		fmt.Sprintf(""),
		fmt.Sprintf("--- Local Proxy ---"),
		fmt.Sprintf("$ %s", p.proxyCmd),
		fmt.Sprintf(""),
		fmt.Sprintf("Access:   http://127.0.0.1:%d/api/     (ADK REST API)", p.ProxyPort),
		fmt.Sprintf("          http://127.0.0.1:%d/ui/      (Web UI, if enabled)", p.ProxyPort),
	}
}

// --- Plan interface ---

func (p *CloudRunPlan) String() string {
	return strings.Join(p.lines, "\n")
}

func (p *CloudRunPlan) Lines() []string {
	return p.lines
}

// Dockerfile returns the computed Dockerfile content.
func (p *CloudRunPlan) Dockerfile() string {
	return p.dockerfile
}

// BuildCmd returns the build command.
func (p *CloudRunPlan) BuildCmd() string {
	return p.buildCmd
}

// ProxyCmd returns the proxy command.
func (p *CloudRunPlan) ProxyCmd() string {
	return p.proxyCmd
}

func formatProtocols(protocols []Protocol) string {
	if len(protocols) == 0 {
		return "(none)"
	}
	sorted := make([]string, len(protocols))
	for i, p := range protocols {
		sorted[i] = string(p)
	}
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}
