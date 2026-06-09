package launcher

import (
	"context"
	"errors"
	"testing"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/runner"
)

type stubAgentLoader struct {
	agent agent.Agent
}

func (l *stubAgentLoader) RootAgent() agent.Agent { return l.agent }

func TestConfigDefaults(t *testing.T) {
	cfg := &Config{}
	if cfg.SessionService != nil {
		t.Error("session service should be nil by default")
	}
	if cfg.ArtifactService != nil {
		t.Error("artifact service should be nil by default")
	}
	if cfg.MemoryService != nil {
		t.Error("memory service should be nil by default")
	}
	if cfg.AgentLoader != nil {
		t.Error("agent loader should be nil by default")
	}
}

func TestConfigWithServices(t *testing.T) {
	ss := runner.NewInMemorySessionService()
	cfg := &Config{
		SessionService: ss,
	}
	if cfg.SessionService != ss {
		t.Error("session service not set")
	}
}

type safeLauncher struct{}

func (s *safeLauncher) Execute(ctx context.Context, config *Config, args []string) error { return nil }
func (s *safeLauncher) CommandLineSyntax() string                                        { return "safe" }

type safeSubLauncher struct {
	keyword string
	desc    string
	syntax  string
	parsed  []string
	runErr  error
	ran     bool
}

func (s *safeSubLauncher) Keyword() string                                { return s.keyword }
func (s *safeSubLauncher) SimpleDescription() string                      { return s.desc }
func (s *safeSubLauncher) CommandLineSyntax() string                      { return s.syntax }
func (s *safeSubLauncher) Parse(args []string) ([]string, error)          { s.parsed = args; return args, nil }
func (s *safeSubLauncher) Run(ctx context.Context, config *Config) error {
	s.ran = true
	return s.runErr
}

func TestLauncherInterface(t *testing.T) {
	var l Launcher = &safeLauncher{}
	if l == nil {
		t.Fatal("Launcher interface not satisfied")
	}
}

func TestSubLauncherInterface(t *testing.T) {
	var s SubLauncher = &safeSubLauncher{keyword: "test"}
	if s == nil {
		t.Fatal("SubLauncher interface not satisfied")
	}
	if s.Keyword() != "test" {
		t.Errorf("keyword = %q, want 'test'", s.Keyword())
	}
}

func TestSubLauncherParseCalled(t *testing.T) {
	sl := &safeSubLauncher{keyword: "hello"}
	remaining, err := sl.Parse([]string{"--flag", "value"})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 {
		t.Errorf("remaining = %v, want 2 args", remaining)
	}
	if sl.parsed[0] != "--flag" {
		t.Errorf("parsed[0] = %q, want '--flag'", sl.parsed[0])
	}
}

func TestSubLauncherRunError(t *testing.T) {
	sl := &safeSubLauncher{keyword: "bad", runErr: errors.New("boom")}
	err := sl.Run(context.Background(), &Config{})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "boom" {
		t.Errorf("error = %q, want 'boom'", err.Error())
	}
	if !sl.ran {
		t.Error("expected Run to be called")
	}
}
