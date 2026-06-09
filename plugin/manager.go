package plugin

import (
	"fmt"

	"github.com/likun666661/rive-adk-go/callbackctx"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/model"
)

// Manager holds an ordered list of Plugin instances and runs their hooks
// with consistent semantics:
//
//   - Preserves registration order — first registered runs first.
//   - Skips nil hooks — if a plugin doesn't implement a hook, it's ignored.
//   - Stops at first non-nil result — early exit for shortcuts/overrides.
//   - Returns errors immediately — any hook error aborts the chain.
//
// Use NewManager to create an empty Manager, then Register plugins in the
// desired priority order.
type Manager struct {
	plugins []*Plugin
}

// NewManager creates an empty plugin Manager.
func NewManager() *Manager {
	return &Manager{}
}

// Register adds a plugin to the end of the execution chain. Plugins run
// in registration order.
func (m *Manager) Register(p *Plugin) {
	if p == nil {
		return
	}
	m.plugins = append(m.plugins, p)
}

// Len returns the number of registered plugins.
func (m *Manager) Len() int {
	return len(m.plugins)
}

// ==========================================================================
// Agent hooks
// ==========================================================================

// RunBeforeAgentCallback executes all before-agent hooks in registration
// order. Early exit on first non-nil event; immediate return on error.
func (m *Manager) RunBeforeAgentCallback(cctx callbackctx.CallbackContext) (*event.Event, error) {
	for i, p := range m.plugins {
		cb := p.BeforeAgentCallback()
		if cb == nil {
			continue
		}
		ev, err := cb(cctx)
		if err != nil {
			return nil, fmt.Errorf("plugin %q RunBeforeAgentCallback[%d]: %w", p.Name(), i, err)
		}
		if ev != nil {
			return ev, nil
		}
	}
	return nil, nil
}

// RunAfterAgentCallback executes all after-agent hooks in registration
// order. Early exit on first non-nil event; immediate return on error.
func (m *Manager) RunAfterAgentCallback(cctx callbackctx.CallbackContext, events []*event.Event) (*event.Event, error) {
	for i, p := range m.plugins {
		cb := p.AfterAgentCallback()
		if cb == nil {
			continue
		}
		ev, err := cb(cctx, events)
		if err != nil {
			return nil, fmt.Errorf("plugin %q RunAfterAgentCallback[%d]: %w", p.Name(), i, err)
		}
		if ev != nil {
			return ev, nil
		}
	}
	return nil, nil
}

// ==========================================================================
// Model hooks
// ==========================================================================

// RunBeforeModelCallback executes all before-model hooks in registration
// order. Returns (nil, nil) if no plugin intervenes. Early exit on first
// non-nil response; immediate return on error.
func (m *Manager) RunBeforeModelCallback(cctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
	for i, p := range m.plugins {
		cb := p.BeforeModelCallback()
		if cb == nil {
			continue
		}
		resp, err := cb(cctx, req)
		if err != nil {
			return nil, fmt.Errorf("plugin %q RunBeforeModelCallback[%d]: %w", p.Name(), i, err)
		}
		if resp != nil {
			return resp, nil
		}
	}
	return nil, nil
}

// RunAfterModelCallback executes all after-model hooks in registration
// order. Returns (nil, nil) if no plugin intervenes. Each callback can
// replace the response. Early exit on first non-nil override.
func (m *Manager) RunAfterModelCallback(cctx callbackctx.CallbackContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
	for i, p := range m.plugins {
		cb := p.AfterModelCallback()
		if cb == nil {
			continue
		}
		newResp, err := cb(cctx, req, resp, callErr)
		if err != nil {
			return nil, fmt.Errorf("plugin %q RunAfterModelCallback[%d]: %w", p.Name(), i, err)
		}
		if newResp != nil {
			return newResp, nil
		}
	}
	return nil, nil
}

// RunOnModelErrorCallback executes all model-error hooks in registration
// order. Can replace the error with a successful response (recovery).
// Returns (nil, nil) if no plugin intervenes.
func (m *Manager) RunOnModelErrorCallback(cctx callbackctx.CallbackContext, req *model.LLMRequest, originalErr error) (*model.LLMResponse, error) {
	for i, p := range m.plugins {
		cb := p.OnModelErrorCallback()
		if cb == nil {
			continue
		}
		resp, err := cb(cctx, req, originalErr)
		if err != nil {
			return nil, fmt.Errorf("plugin %q RunOnModelErrorCallback[%d]: %w", p.Name(), i, err)
		}
		if resp != nil {
			return resp, nil
		}
	}
	return nil, nil
}

// ==========================================================================
// Tool hooks
// ==========================================================================

// RunBeforeToolCallback executes all before-tool hooks in registration
// order. Early exit on first non-nil result; immediate return on error.
func (m *Manager) RunBeforeToolCallback(tctx callbackctx.ToolContext, toolName string, args map[string]any) (map[string]any, error) {
	for i, p := range m.plugins {
		cb := p.BeforeToolCallback()
		if cb == nil {
			continue
		}
		result, err := cb(tctx, toolName, args)
		if err != nil {
			return nil, fmt.Errorf("plugin %q RunBeforeToolCallback[%d]: %w", p.Name(), i, err)
		}
		if result != nil {
			return result, nil
		}
	}
	return nil, nil
}

// RunAfterToolCallback executes all after-tool hooks in registration
// order. Early exit on first non-nil result; immediate return on error.
func (m *Manager) RunAfterToolCallback(tctx callbackctx.ToolContext, toolName string, args, result map[string]any, runErr error) (map[string]any, error) {
	for i, p := range m.plugins {
		cb := p.AfterToolCallback()
		if cb == nil {
			continue
		}
		newResult, err := cb(tctx, toolName, args, result, runErr)
		if err != nil {
			return nil, fmt.Errorf("plugin %q RunAfterToolCallback[%d]: %w", p.Name(), i, err)
		}
		if newResult != nil {
			return newResult, nil
		}
	}
	return nil, nil
}

// RunOnToolErrorCallback executes all tool-error hooks in registration
// order. Can replace the error with a successful result (recovery).
// Returns (nil, nil) if no plugin intervenes.
func (m *Manager) RunOnToolErrorCallback(tctx callbackctx.ToolContext, toolName string, args map[string]any, originalErr error) (map[string]any, error) {
	for i, p := range m.plugins {
		cb := p.OnToolErrorCallback()
		if cb == nil {
			continue
		}
		result, err := cb(tctx, toolName, args, originalErr)
		if err != nil {
			return nil, fmt.Errorf("plugin %q RunOnToolErrorCallback[%d]: %w", p.Name(), i, err)
		}
		if result != nil {
			return result, nil
		}
	}
	return nil, nil
}
