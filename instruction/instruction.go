// Package instruction provides instruction providers and template injection
// for the agent runtime.
//
// Three instruction sources are supported:
//
//   - Static instruction: a literal string set in agent config.
//   - Dynamic instruction provider: a function (ReadonlyContext) -> (string, error)
//     called at each LLM request to produce context-sensitive instructions.
//   - Global instruction: a literal string applied to the root agent only.
//   - Global instruction provider: like dynamic but for root agent only.
//
// Template injection supports {placeholder} syntax with these patterns:
//
//	{varName}            — read from session/user/app state (merged view).
//	{varName?}           — optional; resolves to "" if key is absent.
//	{app:key}            — read from app-scoped state.
//	{user:key}           — read from user-scoped state.
//	{temp:key}           — read from invocation-local temp state.
//
// Placeholders are resolved via session.ReadonlyState which should be a
// fully merged view of app + user + session state.
package instruction

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/session"
)

// Provider is a dynamic instruction provider.
// It receives readonly context and returns an instruction string.
// Returning ("", nil) means no instruction should be injected.
type Provider func(ctx ReadonlyContext) (string, error)

// placeholderRE matches {placeholder} tokens inside instruction templates.
// Supported forms:
//
//	{name}    — required state variable
//	{name?}   — optional (resolves to "" if absent)
//	{prefix:key} — scoped state via prefix (app:, user:, temp:)
var placeholderRE = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*(?:\:[a-zA-Z_][a-zA-Z0-9_]*)?)(\?)?\}`)

// InjectSessionState replaces {placeholder} tokens in template with values
// from the provided ReadonlyState. The state should be the fully merged
// (app + user + session) view so that all scope prefixes resolve correctly.
func InjectSessionState(template string, state session.ReadonlyState) (string, error) {
	if template == "" {
		return "", nil
	}

	var lastErr error
	result := placeholderRE.ReplaceAllStringFunc(template, func(match string) string {
		sub := placeholderRE.FindStringSubmatch(match)
		if sub == nil {
			return match
		}
		key := sub[1]
		optional := sub[2] == "?"

		val, ok := state.Get(key)
		if !ok {
			if optional {
				return ""
			}
			lastErr = fmt.Errorf("instruction: placeholder %q not found in state", match)
			return match
		}

		return fmt.Sprintf("%v", val)
	})

	if lastErr != nil {
		return "", lastErr
	}
	return result, nil
}

// Config holds all instruction-related parameters for an agent.
type Config struct {
	// Static instruction template (may contain {placeholders}).
	Instruction string

	// Dynamic instruction provider, called before each LLM request.
	InstructionProvider Provider

	// Global instruction — applied only when the agent is the root agent.
	GlobalInstruction string

	// Global dynamic instruction provider — root-agent only.
	GlobalInstructionProvider Provider

	// IsRootAgent returns true when the agent executing is the root agent.
	// If nil, global instructions are never applied.
	IsRootAgent func() bool

	// ReadonlyState returns a fully merged (app+user+session) readonly state
	// for placeholder injection. If nil, no placeholder substitution occurs.
	ReadonlyState func() session.ReadonlyState
}

// NewRequestProcessor creates an instruction processor that injects
// instructions into the LLMRequest.SystemInstruction field.
//
// Execution order per request:
//  1. GlobalInstruction (static) — appended first, root-agent only.
//  2. GlobalInstructionProvider (dynamic) — appended next, root-agent only.
//  3. Instruction (static) — the agent's own instruction template.
//  4. InstructionProvider (dynamic) — the agent's own dynamic provider.
//  5. Placeholder injection against the merged session/user/app state.
//
// The final result is written to req.SystemInstruction. Multiple sources
// are joined with "\n\n".
//
// Use ToRequestProcessor to wrap the result into a flow.RequestProcessor.
func NewRequestProcessor(cfg Config) func(ctx ReadonlyContext, req *model.LLMRequest) (*event.Event, error) {
	return func(ctx ReadonlyContext, req *model.LLMRequest) (*event.Event, error) {
		var parts []string

		if cfg.IsRootAgent != nil && cfg.IsRootAgent() {
			if cfg.GlobalInstruction != "" {
				parts = append(parts, cfg.GlobalInstruction)
			}
			if cfg.GlobalInstructionProvider != nil {
				gi, err := cfg.GlobalInstructionProvider(ctx)
				if err != nil {
					return nil, fmt.Errorf("instruction: global provider: %w", err)
				}
				if gi != "" {
					parts = append(parts, gi)
				}
			}
		}

		if cfg.Instruction != "" {
			parts = append(parts, cfg.Instruction)
		}

		if cfg.InstructionProvider != nil {
			di, err := cfg.InstructionProvider(ctx)
			if err != nil {
				return nil, fmt.Errorf("instruction: provider: %w", err)
			}
			if di != "" {
				parts = append(parts, di)
			}
		}

		if len(parts) == 0 {
			return nil, nil
		}

		template := strings.Join(parts, "\n\n")

		if cfg.ReadonlyState != nil {
			state := cfg.ReadonlyState()
			if state != nil {
				injected, err := InjectSessionState(template, state)
				if err != nil {
					return nil, err
				}
				req.SystemInstruction = injected
				return nil, nil
			}
		}

		req.SystemInstruction = template
		return nil, nil
	}
}

// MergeStateView merges app-state, user-state, and session-state maps into
// a single ReadonlyState for use with InjectSessionState.
// App keys get "app:" prefix, user keys get "user:" prefix.
func MergeStateView(appState, userState, sessionState map[string]any) session.ReadonlyState {
	merged := make(map[string]any, len(appState)+len(userState)+len(sessionState))
	for k, v := range appState {
		merged[session.KeyPrefixApp+k] = v
	}
	for k, v := range userState {
		merged[session.KeyPrefixUser+k] = v
	}
	for k, v := range sessionState {
		merged[k] = v
	}
	return &staticState{data: merged}
}

// staticState is a trivial session.ReadonlyState backed by a map.
type staticState struct {
	data map[string]any
}

func (s *staticState) Get(key string) (any, bool) {
	v, ok := s.data[key]
	return v, ok
}

func (s *staticState) All() map[string]any {
	cpy := make(map[string]any, len(s.data))
	for k, v := range s.data {
		cpy[k] = v
	}
	return cpy
}
