// Package launcher defines stable entrypoint abstractions that expose
// the runtime through composable sublaunchers.
//
// Design (Chapter 06):
//   - Config carries all service dependencies (session, artifact, memory,
//     agent loader, plugin manager).
//   - Launcher is the top-level entrypoint; it parses argv and delegates
//     to a chosen sublauncher.
//   - SubLauncher is a composable capability unit identified by a keyword.
//   - The universal router matches the first argv token to a sublauncher
//     keyword; the first registered sublauncher is the default.
//   - The console sublauncher drives runner.Run from scripted input,
//     persisting events through the session.
//
// This keeps the runtime unmodified; the launcher layer only manages
// lifecycle, routing, and I/O.
package launcher

import (
	"context"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/artifact"
	"github.com/likun666661/rive-adk-go/memory"
	"github.com/likun666661/rive-adk-go/plugin"
	"github.com/likun666661/rive-adk-go/runner"
)

// AgentLoader loads the root agent for a launcher entrypoint.
type AgentLoader interface {
	RootAgent() agent.Agent
}

// Config carries session/artifact/memory/agent loader/plugin
// dependencies for launchers and sublaunchers.
type Config struct {
	SessionService  runner.SessionService
	ArtifactService artifact.Service
	MemoryService   memory.Service
	AgentLoader     AgentLoader
	PluginManager   *plugin.Manager
}

// Launcher is the main interface for running an application entrypoint.
// It is responsible for parsing command-line arguments and executing the
// corresponding logic.
type Launcher interface {
	Execute(ctx context.Context, config *Config, args []string) error
	CommandLineSyntax() string
}

// SubLauncher is a composable capability unit that can be routed to
// by keyword within a universal launcher. Each SubLauncher corresponds
// to a specific mode of operation (e.g. "console").
type SubLauncher interface {
	Keyword() string
	Parse(args []string) ([]string, error)
	Run(ctx context.Context, config *Config) error
	CommandLineSyntax() string
	SimpleDescription() string
}
