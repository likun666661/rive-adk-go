// Package web provides a web sublauncher that mounts the adkrest server
// routes inside an HTTP server, enabling REST and SSE access to the runtime.
package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/likun666661/rive-adk-go/cmd/launcher"
	"github.com/likun666661/rive-adk-go/server/adkrest"
)

// defaultPort is the TCP port the HTTP server listens on.
const defaultPort = 8080

// Web exposes the runtime via HTTP JSON and SSE endpoints.
type Web struct {
	port         int
	readTimeout  time.Duration
	writeTimeout time.Duration
}

// New creates a new Web sublauncher with defaults.
func New() *Web {
	return &Web{
		port:         defaultPort,
		readTimeout:  10 * time.Second,
		writeTimeout: 10 * time.Second,
	}
}

// WithPort sets the listen port.
func (w *Web) WithPort(port int) *Web {
	w.port = port
	return w
}

// WithReadTimeout sets the server read timeout.
func (w *Web) WithReadTimeout(d time.Duration) *Web {
	w.readTimeout = d
	return w
}

// WithWriteTimeout sets the server write timeout.
func (w *Web) WithWriteTimeout(d time.Duration) *Web {
	w.writeTimeout = d
	return w
}

// Keyword implements launcher.SubLauncher.
func (w *Web) Keyword() string {
	return "web"
}

// SimpleDescription implements launcher.SubLauncher.
func (w *Web) SimpleDescription() string {
	return "starts an HTTP server with REST and SSE endpoints"
}

// CommandLineSyntax implements launcher.SubLauncher.
func (w *Web) CommandLineSyntax() string {
	return fmt.Sprintf("web [-port %d]", defaultPort)
}

// Parse implements launcher.SubLauncher. It accepts a -port flag.
func (w *Web) Parse(args []string) ([]string, error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-port":
			i++
			if i < len(args) {
				_, err := fmt.Sscanf(args[i], "%d", &w.port)
				if err != nil {
					return nil, fmt.Errorf("web: invalid port value %q", args[i])
				}
			}
		default:
			return args[i:], nil
		}
	}
	return nil, nil
}

// Run implements launcher.SubLauncher. It creates an adkrest server,
// mounts it on an http.Server, and blocks until the context is cancelled.
func (w *Web) Run(ctx context.Context, config *launcher.Config) error {
	restServer, err := adkrest.NewServer(config)
	if err != nil {
		return fmt.Errorf("web: failed to create adkrest server: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", restServer)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", w.port),
		Handler:      mux,
		ReadTimeout:  w.readTimeout,
		WriteTimeout: w.writeTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("[web] ADK REST server listening on http://localhost:%d\n", w.port)
		fmt.Printf("[web]   POST /run       — JSON run\n")
		fmt.Printf("[web]   POST /run_sse   — SSE event stream\n")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		fmt.Println("[web] shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("web: server error: %w", err)
	}
}

// Compile-time check.
var _ launcher.SubLauncher = (*Web)(nil)
