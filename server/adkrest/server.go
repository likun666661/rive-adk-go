package adkrest

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/likun666661/rive-adk-go/cmd/launcher"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/runner"
)

// Server exposes the runtime via HTTP JSON and SSE endpoints.
// It uses no global singletons — all services come from the launcher config.
type Server struct {
	config  *launcher.Config
	mux     *http.ServeMux
	appName string
}

// NewServer creates a new Server using services from the launcher config.
// The appName is used as the runner's AppName for all requests.
func NewServer(cfg *launcher.Config) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("adkrest: config is nil")
	}
	if cfg.AgentLoader == nil {
		return nil, fmt.Errorf("adkrest: AgentLoader is required")
	}

	s := &Server{
		config:  cfg,
		mux:     http.NewServeMux(),
		appName: "adkrest_app",
	}

	s.mux.HandleFunc("/run", s.runHandler)
	s.mux.HandleFunc("/run_sse", s.runSSEHandler)
	return s, nil
}

// SetAppName overrides the default runner app name.
func (s *Server) SetAppName(name string) {
	s.appName = name
}

// ServeHTTP implements http.Handler so Server can be used directly
// or mounted under a path prefix.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// Handler returns the underlying http.Handler for mounting.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// ---------------------------------------------------------------------------
// JSON handler — decodes request, runs agent, returns collected events as JSON
// ---------------------------------------------------------------------------

func (s *Server) runHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed, use POST")
		return
	}

	req, err := decodeRunRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	events, srvErr := s.runAgent(r.Context(), req)
	if srvErr != nil {
		writeError(w, srvErr.code, srvErr.message)
		return
	}

	resp := make([]EventResponse, len(events))
	for i, ev := range events {
		resp[i] = EventToResponse(ev)
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// SSE handler — streams events as data: <json>\n\n
// ---------------------------------------------------------------------------

func (s *Server) runSSEHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed, use POST")
		return
	}

	req, err := decodeRunRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	events, srvErr := s.runAgent(r.Context(), req)
	if srvErr != nil {
		writeSSEError(w, flusher, srvErr.message)
		return
	}

	for _, ev := range events {
		payload, err := json.Marshal(EventToResponse(ev))
		if err != nil {
			writeSSEError(w, flusher, fmt.Sprintf("marshal event: %v", err))
			return
		}
		if err := writeSSEData(w, flusher, string(payload)); err != nil {
			log.Printf("adkrest: SSE write error: %v", err)
			return
		}
	}
}

// ---------------------------------------------------------------------------
// runAgent — shared logic: load agent, create runner, execute run
// ---------------------------------------------------------------------------

type serverError struct {
	code    int
	message string
}

func (e *serverError) Error() string {
	return e.message
}

func (s *Server) runAgent(ctx stdctx.Context, req RunRequest) ([]*event.Event, *serverError) {
	rootAgent := s.config.AgentLoader.RootAgent()
	if rootAgent == nil {
		return nil, &serverError{
			code:    http.StatusNotFound,
			message: "agent not found",
		}
	}

	ea, ok := rootAgent.(runner.ExecutableAgent)
	if !ok {
		return nil, &serverError{
			code:    http.StatusInternalServerError,
			message: "agent does not implement ExecutableAgent",
		}
	}

	sessionSvc := s.config.SessionService
	if sessionSvc == nil {
		sessionSvc = runner.NewInMemorySessionService()
	}

	r, err := runner.New(runner.Config{
		AppName:         s.appName,
		Agent:           ea,
		SessionService:  sessionSvc,
		MemoryService:   s.config.MemoryService,
		ArtifactService: s.config.ArtifactService,
	})
	if err != nil {
		return nil, &serverError{
			code:    http.StatusInternalServerError,
			message: fmt.Sprintf("failed to create runner: %v", err),
		}
	}

	_, events, runErr := r.Run(ctx, req.UserID, req.SessionID, req.Message)
	if runErr != nil {
		return nil, &serverError{
			code:    http.StatusInternalServerError,
			message: fmt.Sprintf("run error: %v", runErr),
		}
	}

	return events, nil
}

// ---------------------------------------------------------------------------
// request decoding with strict field checking
// ---------------------------------------------------------------------------

func decodeRunRequest(r *http.Request) (RunRequest, *serverError) {
	var req RunRequest
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&req); err != nil {
		return req, &serverError{
			code:    http.StatusBadRequest,
			message: fmt.Sprintf("failed to decode request body: %v", err),
		}
	}
	if err := validateRunRequest(req); err != nil {
		return req, &serverError{
			code:    http.StatusBadRequest,
			message: err.Error(),
		}
	}
	return req, nil
}

func validateRunRequest(req RunRequest) error {
	if req.AppName == "" {
		return fmt.Errorf("appName is required")
	}
	if req.UserID == "" {
		return fmt.Errorf("userId is required")
	}
	if req.SessionID == "" {
		return fmt.Errorf("sessionId is required")
	}
	if req.Message == "" {
		return fmt.Errorf("newMessage is required")
	}
	return nil
}

// ---------------------------------------------------------------------------
// response helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(status)
	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			log.Printf("adkrest: JSON encode error: %v", err)
		}
	}
}

type errorBody struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: msg})
}

func writeSSEData(w http.ResponseWriter, flusher http.Flusher, data string) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", data)
	if err != nil {
		return fmt.Errorf("write SSE data: %w", err)
	}
	flusher.Flush()
	return nil
}

func writeSSEError(w http.ResponseWriter, flusher http.Flusher, msg string) {
	_, _ = fmt.Fprintf(w, "event: error\n")
	safeJSON, err := json.Marshal(errorBody{Error: msg})
	if err != nil {
		safeJSON = []byte(`{"error":"internal error"}`)
	}
	_ = writeSSEData(w, flusher, string(safeJSON))
}
