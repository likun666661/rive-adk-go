// Package console provides a sublauncher that drives the runner from
// scripted input and persists events through sessions.
//
// The console launcher:
//   - Creates a session via the session service (or uses InMemoryService
//     if none is provided).
//   - Loads the root agent via the AgentLoader.
//   - Creates a runner.
//   - Reads lines from a configurable input reader (defaults to os.Stdin).
//   - For each line, calls runner.Run and writes events to an output
//     writer (defaults to os.Stdout).
//   - Persists non-partial events into the session.
package console

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/likun666661/rive-adk-go/cmd/launcher"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/runner"
)

// defaultAppName and defaultUserID are used when Config.SessionService
// creates sessions with an identity.
const (
	defaultAppName = "console_app"
	defaultUserID  = "console_user"
)

// Console reads input from an io.Reader (default os.Stdin), writes output
// to an io.Writer (default os.Stdout), and drives runner.Run for each
// input line.
type Console struct {
	in  io.Reader
	out io.Writer
}

// ConsoleOption configures a Console.
type ConsoleOption func(*Console)

// WithInput sets the input reader.
func WithInput(r io.Reader) ConsoleOption {
	return func(c *Console) { c.in = r }
}

// WithOutput sets the output writer.
func WithOutput(w io.Writer) ConsoleOption {
	return func(c *Console) { c.out = w }
}

// NewConsole creates a Console with the given options.
func NewConsole(opts ...ConsoleOption) *Console {
	c := &Console{
		in:  os.Stdin,
		out: os.Stdout,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Keyword implements launcher.SubLauncher.
func (c *Console) Keyword() string {
	return "console"
}

// Parse implements launcher.SubLauncher. The console sublauncher
// accepts no additional flags in this compact implementation.
func (c *Console) Parse(args []string) ([]string, error) {
	return args, nil
}

// CommandLineSyntax implements launcher.SubLauncher.
func (c *Console) CommandLineSyntax() string {
	return "console mode accepts no additional flags"
}

// SimpleDescription implements launcher.SubLauncher.
func (c *Console) SimpleDescription() string {
	return "runs an agent in interactive console mode"
}

// Run implements launcher.SubLauncher. It creates a session, creates a
// runner, and processes each input line through runner.Run.
func (c *Console) Run(ctx context.Context, config *launcher.Config) error {
	if config.AgentLoader == nil {
		return fmt.Errorf("console: AgentLoader is required")
	}

	sessionSvc := config.SessionService
	if sessionSvc == nil {
		sessionSvc = runner.NewInMemorySessionService()
	}

	rootAgent := config.AgentLoader.RootAgent()
	ea, ok := rootAgent.(runner.ExecutableAgent)
	if !ok {
		return fmt.Errorf("console: root agent does not implement runner.ExecutableAgent")
	}

	r, err := runner.New(runner.Config{
		AppName:         defaultAppName,
		Agent:           ea,
		SessionService:  sessionSvc,
		MemoryService:   config.MemoryService,
		ArtifactService: config.ArtifactService,
	})
	if err != nil {
		return fmt.Errorf("console: failed to create runner: %w", err)
	}

	userID := defaultUserID
	sessionID := "console-session-1"

	scanner := bufio.NewScanner(c.in)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if err := c.processLine(ctx, r, userID, sessionID, line); err != nil {
			fmt.Fprintf(c.out, "ERROR: %v\n", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("console: input read error: %w", err)
	}
	return nil
}

func (c *Console) processLine(ctx context.Context, r *runner.Runner, userID, sessionID, line string) error {
	sess, events, err := r.Run(ctx, userID, sessionID, line)
	if err != nil {
		return fmt.Errorf("runner.Run: %w", err)
	}

	fmt.Fprintf(c.out, "--- session: %s (events persisted: %d) ---\n", sess.ID(), sess.EventCount())

	for i, ev := range events {
		c.printEvent(i, ev)
	}
	return nil
}

func (c *Console) printEvent(idx int, ev *event.Event) {
	if ev.Content == nil {
		return
	}
	prefix := ""
	switch ev.Role {
	case event.RoleModel:
		prefix = "model"
	case event.RoleTool:
		prefix = "tool"
	case event.RoleUser:
		prefix = "user"
	}
	partial := ""
	if ev.Partial {
		partial = " [partial]"
	}
	for _, p := range ev.Content.Parts {
		switch {
		case p.Text != "":
			fmt.Fprintf(c.out, "  [%d] %s%s: %s\n", idx, prefix, partial, p.Text)
		case p.FunctionCall != nil:
			fmt.Fprintf(c.out, "  [%d] %s%s: call %s(%v)\n", idx, prefix, partial, p.FunctionCall.Name, p.FunctionCall.Args)
		case p.FunctionResponse != nil:
			fmt.Fprintf(c.out, "  [%d] %s%s: result from %s => %v\n", idx, prefix, partial, p.FunctionResponse.Name, p.FunctionResponse.Result)
		}
	}
}

// Compile-time check that Console satisfies SubLauncher.
var _ launcher.SubLauncher = (*Console)(nil)
