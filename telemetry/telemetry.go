// Package telemetry provides an in-memory span and log recorder that
// explains how ADK Go instruments runner, model, tool, and server events
// (Chapter 06). No external OpenTelemetry or GCP dependencies are required;
// all data is held in-recorder for inspection and testing.
//
// Design:
//   - Recorder is the central accumulator. It stores spans and logs in-memory.
//   - Providers wraps the recorder and manages lifecycle (Init, Shutdown).
//   - Options configure capture behavior (e.g. capture message content).
//   - Instrumentation helpers mirror semantic span/log patterns from
//     google.golang.org/adk/internal/telemetry.
//
// The recorder is intended for educational use and test verification.
package telemetry

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Span / Log records
// ---------------------------------------------------------------------------

// SpanRecord represents a recorded span with name, attributes, status, and
// timing. It mirrors the shape of an OpenTelemetry span for educational
// inspection.
type SpanRecord struct {
	Name       string
	StartTime  time.Time
	EndTime    time.Time
	Attributes map[string]any
	Status     string // "", "OK", "ERROR"
	Error      string
}

// LogRecord represents a recorded log event.
type LogRecord struct {
	EventName  string
	Timestamp  time.Time
	Attributes map[string]any
	Body       map[string]any
}

// ---------------------------------------------------------------------------
// Recorder
// ---------------------------------------------------------------------------

// Recorder is an in-memory span and log accumulator.
// It is safe for concurrent use.
type Recorder struct {
	mu     sync.Mutex
	spans  []SpanRecord
	logs   []LogRecord

	captureMessageContent bool
}

// NewRecorder creates a new empty Recorder with the given options.
func NewRecorder(opts ...Option) *Recorder {
	r := &Recorder{}
	for _, o := range opts {
		o.apply(r)
	}
	return r
}

func (r *Recorder) addSpan(s SpanRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spans = append(r.spans, s)
}

func (r *Recorder) addLog(l LogRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, l)
}

// Spans returns a snapshot of all recorded spans.
func (r *Recorder) Spans() []SpanRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]SpanRecord, len(r.spans))
	copy(out, r.spans)
	return out
}

// Logs returns a snapshot of all recorded logs.
func (r *Recorder) Logs() []LogRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]LogRecord, len(r.logs))
	copy(out, r.logs)
	return out
}

// SpanCount returns the number of recorded spans.
func (r *Recorder) SpanCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.spans)
}

// LogCount returns the number of recorded log records.
func (r *Recorder) LogCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.logs)
}

// Reset clears all spans and logs.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spans = r.spans[:0]
	r.logs = r.logs[:0]
}

// CaptureMessageContent returns whether message content capture is enabled.
func (r *Recorder) CaptureMessageContent() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.captureMessageContent
}

// ---------------------------------------------------------------------------
// Option model
// ---------------------------------------------------------------------------

// Option configures a Recorder.
type Option interface {
	apply(r *Recorder)
}

type optionFunc func(r *Recorder)

func (f optionFunc) apply(r *Recorder) {
	f(r)
}

// WithCaptureMessageContent enables/disables capturing message content
// in log records. When false (default), message bodies are elided as
// "<elided>" to avoid recording PII or secrets.
func WithCaptureMessageContent(capture bool) Option {
	return optionFunc(func(r *Recorder) {
		r.captureMessageContent = capture
	})
}

// ---------------------------------------------------------------------------
// Provider model
// ---------------------------------------------------------------------------

// Providers wraps a Recorder and exposes Init / Shutdown semantics
// that mirror ADK Go's telemetry.Providers.
type Providers struct {
	recorder *Recorder
}

// NewProviders creates a new Providers with a Recorder configured
// by the given options.
func NewProviders(opts ...Option) *Providers {
	return &Providers{
		recorder: NewRecorder(opts...),
	}
}

// Init initializes telemetry. This is a no-op for the in-memory recorder
// but satisfies the lifecycle contract.
func (p *Providers) Init(ctx context.Context) error {
	p.recorder.Reset()
	return nil
}

// Shutdown flushes and clears the recorder. For the in-memory recorder,
// this keeps the data available for inspection until explicitly Reset.
func (p *Providers) Shutdown(ctx context.Context) error {
	return nil
}

// Recorder returns the underlying Recorder for test inspection.
func (p *Providers) Recorder() *Recorder {
	return p.recorder
}

// ---------------------------------------------------------------------------
// Default instance (for use without manual Providers lifecycle)
// ---------------------------------------------------------------------------

var (
	defaultRecorderMu sync.Mutex
	defaultRecorder   *Recorder
)

// DefaultRecorder returns a package-level shared recorder.
func DefaultRecorder() *Recorder {
	defaultRecorderMu.Lock()
	defer defaultRecorderMu.Unlock()
	if defaultRecorder == nil {
		defaultRecorder = NewRecorder()
	}
	return defaultRecorder
}

// SetDefaultRecorder overrides the default recorder.
func SetDefaultRecorder(r *Recorder) {
	defaultRecorderMu.Lock()
	defer defaultRecorderMu.Unlock()
	defaultRecorder = r
}

// ---------------------------------------------------------------------------
// JSON-safe serialization of nested values
// ---------------------------------------------------------------------------

func safeJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "<not serializable>"
	}
	return string(b)
}
