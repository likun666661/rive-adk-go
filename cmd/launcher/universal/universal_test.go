package universal

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/likun666661/rive-adk-go/cmd/launcher"
)

type spySubLauncher struct {
	keyword string
	desc    string
	syntax  string
	runErr  error

	parseCalled bool
	parseArgs   []string
	parseRet    []string

	runCalled bool
}

func newSpy(keyword, desc string) *spySubLauncher {
	return &spySubLauncher{keyword: keyword, desc: desc, syntax: keyword + " flags"}
}

func (s *spySubLauncher) Keyword() string                                { return s.keyword }
func (s *spySubLauncher) SimpleDescription() string                      { return s.desc }
func (s *spySubLauncher) CommandLineSyntax() string                      { return s.syntax }
func (s *spySubLauncher) Parse(args []string) ([]string, error)          { s.parseCalled = true; s.parseArgs = args; return s.parseRet, nil }
func (s *spySubLauncher) Run(ctx context.Context, config *launcher.Config) error {
	s.runCalled = true
	return s.runErr
}

func TestDefaultSubLauncherSelection(t *testing.T) {
	console := newSpy("console", "console mode")
	web := newSpy("web", "web mode")

	uni := New(console, web)
	err := uni.Execute(context.Background(), &launcher.Config{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !console.runCalled {
		t.Error("default (first) sublauncher should be called")
	}
	if web.runCalled {
		t.Error("web sublauncher should not be called")
	}
}

func TestKeywordBasedRouting(t *testing.T) {
	console := newSpy("console", "console mode")
	web := newSpy("web", "web mode")

	uni := New(console, web)
	err := uni.Execute(context.Background(), &launcher.Config{}, []string{"web"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if console.runCalled {
		t.Error("console should not be called when 'web' keyword is used")
	}
	if !web.runCalled {
		t.Error("web sublauncher should be called for 'web' keyword")
	}
	if !web.parseCalled {
		t.Error("web.Parse should be called")
	}
}

func TestKeywordRoutingWithArgs(t *testing.T) {
	console := newSpy("console", "console mode")
	web := newSpy("web", "web mode")
	web.parseRet = nil

	uni := New(console, web)
	err := uni.Execute(context.Background(), &launcher.Config{}, []string{"web", "--port=8080"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !web.parseCalled {
		t.Error("web.Parse should be called")
	}
	if len(web.parseArgs) != 1 || web.parseArgs[0] != "--port=8080" {
		t.Errorf("web.Parse args = %v, want [--port=8080]", web.parseArgs)
	}
}

func TestUnknownCommandError(t *testing.T) {
	console := newSpy("console", "console mode")
	web := newSpy("web", "web mode")

	uni := New(console, web)
	err := uni.Execute(context.Background(), &launcher.Config{}, []string{"unknown"})

	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error should mention 'unknown': %v", err)
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("error should mention 'unknown command': %v", err)
	}
}

func TestNoSubLaunchersError(t *testing.T) {
	uni := New()
	err := uni.Execute(context.Background(), &launcher.Config{}, nil)
	if err == nil {
		t.Fatal("expected error with no sublaunchers")
	}
}

func TestDuplicateKeywordsError(t *testing.T) {
	a := newSpy("dup", "first")
	b := newSpy("dup", "second")

	uni := New(a, b)
	err := uni.Execute(context.Background(), &launcher.Config{}, nil)
	if err == nil {
		t.Fatal("expected error for duplicate keywords")
	}
}

func TestSubLauncherRunErrorPropagation(t *testing.T) {
	s := newSpy("console", "console mode")
	s.runErr = errors.New("console crash")

	uni := New(s)
	err := uni.Execute(context.Background(), &launcher.Config{}, nil)
	if err == nil {
		t.Fatal("expected error propagation from sublauncher")
	}
	if err.Error() != "console crash" {
		t.Errorf("error = %q, want 'console crash'", err.Error())
	}
}

func TestCommandLineSyntax(t *testing.T) {
	console := newSpy("console", "console mode")
	web := newSpy("web", "web mode")

	uni := New(console, web)
	syntax := uni.CommandLineSyntax()

	if !strings.Contains(syntax, "console") {
		t.Error("syntax should mention 'console'")
	}
	if !strings.Contains(syntax, "web") {
		t.Error("syntax should mention 'web'")
	}
	if !strings.Contains(syntax, "console mode") {
		t.Error("syntax should include description")
	}
}

func TestDefaultWithFullArgsNoKeywordMatch(t *testing.T) {
	console := newSpy("console", "console mode")
	web := newSpy("web", "web mode")

	uni := New(console, web)
	err := uni.Execute(context.Background(), &launcher.Config{}, []string{"--verbose"})
	if err == nil {
		t.Fatal("expected error for unknown command (not a keyword)")
	}
}

func TestParseRoutesToFirstWhenNoArgs(t *testing.T) {
	console := newSpy("console", "console mode")
	web := newSpy("web", "web mode")

	uni := New(console, web)
	err := uni.Execute(context.Background(), &launcher.Config{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !console.parseCalled {
		t.Error("default sublauncher Parse should be called even with nil args")
	}
}
