// Package deploy produces dry-run deploy plans for Cloud Run and Agent Engine
// without invoking real cloud CLIs. Plans explain how entrypoints become
// product/server deployments following ADK Go's deploy architecture
// (Chapter 06).
//
// Design:
//   - Plan is an immutable snapshot describing all steps of a deploy.
//   - CloudRunPlan and AgentEnginePlan are concrete plan types.
//   - Validate performs pre-flight checks (entrypoint path, names, protocols).
//   - No gcloud, Docker, or network calls are made.
package deploy

import (
	"fmt"
	"strings"
)

// Protocol represents a protocol option enabled for a deploy.
type Protocol string

const (
	ProtocolAPI       Protocol = "api"
	ProtocolA2A       Protocol = "a2a"
	ProtocolWebUI     Protocol = "webui"
	ProtocolAgentEngine Protocol = "agentengine"
	ProtocolPubSub     Protocol = "pubsub"
	ProtocolEventarc   Protocol = "eventarc"
)

// ValidProtocols is the set of known protocols.
var ValidProtocols = map[Protocol]bool{
	ProtocolAPI:       true,
	ProtocolA2A:       true,
	ProtocolWebUI:     true,
	ProtocolAgentEngine: true,
	ProtocolPubSub:     true,
	ProtocolEventarc:   true,
}

// Plan is the common interface for all dry-run deploy plans.
type Plan interface {
	// String returns a human-readable summary of the plan.
	String() string
	// Lines returns the plan as a slice of lines for inspection.
	Lines() []string
}

// ---------------------------------------------------------------------------
// validation
// ---------------------------------------------------------------------------

// ValidationError collects one or more validation failures.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0]
	}
	return fmt.Sprintf("validation errors: %s", strings.Join(e.Errors, "; "))
}

func (e *ValidationError) add(msg string) {
	e.Errors = append(e.Errors, msg)
}

func (e *ValidationError) valid() bool {
	return len(e.Errors) == 0
}

// ValidateEntryPoint checks that the entry point path is usable.
// It must be non-empty and end with .go.
func ValidateEntryPoint(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("entry_point_path is required")
	}
	if !strings.HasSuffix(path, ".go") {
		return fmt.Errorf("entry_point_path must end with .go, got %q", path)
	}
	return nil
}

// ValidateProjectName checks that the GCP project name is non-empty.
func ValidateProjectName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("project_name is required")
	}
	return nil
}

// ValidateRegion checks that the GCP region is non-empty.
func ValidateRegion(region string) error {
	if strings.TrimSpace(region) == "" {
		return fmt.Errorf("region is required")
	}
	return nil
}

// ValidateServiceName checks that the service name is non-empty.
func ValidateServiceName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("service_name is required")
	}
	return nil
}

// ValidateProtocols checks that every protocol is known.
func ValidateProtocols(protocols []Protocol) error {
	var verr ValidationError
	for _, p := range protocols {
		if !ValidProtocols[p] {
			verr.add(fmt.Sprintf("unknown protocol %q", p))
		}
	}
	if !verr.valid() {
		return &verr
	}
	return nil
}

// ValidateServerPort checks the port is in the valid range.
func ValidateServerPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("server_port must be between 1 and 65535, got %d", port)
	}
	return nil
}

// ValidateAll runs multiple validators and returns an aggregate error.
func ValidateAll(validators ...func() error) error {
	var verr ValidationError
	for _, v := range validators {
		if err := v(); err != nil {
			if ve, ok := err.(*ValidationError); ok {
				verr.Errors = append(verr.Errors, ve.Errors...)
			} else {
				verr.add(err.Error())
			}
		}
	}
	if !verr.valid() {
		return &verr
	}
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// StripExtension removes a suffix from a filename.
func StripExtension(name, ext string) (string, error) {
	if !strings.HasSuffix(name, ext) {
		return "", fmt.Errorf("name %q does not have extension %q", name, ext)
	}
	return strings.TrimSuffix(name, ext), nil
}
