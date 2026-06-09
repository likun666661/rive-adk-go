// Package plugin defines a compact plugin layer that composes existing
// callback hook points with ordered execution and early-exit semantics.
//
// Plugin hooks are invoked before direct callbacks, matching the Chapter 04
// teaching model. The Manager preserves registration order, skips nil hooks,
// stops at the first non-nil result, and returns errors immediately.
package plugin

import (
	"github.com/likun666661/rive-adk-go/callbackctx"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/model"
)

// Plugin is a collection of optional callback hooks that can be registered
// with a Manager. Each hook field may be nil; the Manager skips nil hooks
// during execution.
//
// Hooks are organised by lifecycle stage:
//   - Agent: before/after agent execution
//   - Model: before/after model call, plus on-error recovery
//   - Tool:  before/after tool call, plus on-error recovery
type Plugin struct {
	name string

	beforeAgent func(callbackctx.CallbackContext) (*event.Event, error)
	afterAgent  func(callbackctx.CallbackContext, []*event.Event) (*event.Event, error)

	beforeModel  func(callbackctx.CallbackContext, *model.LLMRequest) (*model.LLMResponse, error)
	afterModel   func(callbackctx.CallbackContext, *model.LLMRequest, *model.LLMResponse, error) (*model.LLMResponse, error)
	onModelError func(callbackctx.CallbackContext, *model.LLMRequest, error) (*model.LLMResponse, error)

	beforeTool  func(callbackctx.ToolContext, string, map[string]any) (map[string]any, error)
	afterTool   func(callbackctx.ToolContext, string, map[string]any, map[string]any, error) (map[string]any, error)
	onToolError func(callbackctx.ToolContext, string, map[string]any, error) (map[string]any, error)
}

// Config configures a Plugin. Only the Name field is required; all hook
// fields are optional. Set a hook to nil to skip it.
type Config struct {
	Name string

	BeforeAgent func(callbackctx.CallbackContext) (*event.Event, error)
	AfterAgent  func(callbackctx.CallbackContext, []*event.Event) (*event.Event, error)

	BeforeModel  func(callbackctx.CallbackContext, *model.LLMRequest) (*model.LLMResponse, error)
	AfterModel   func(callbackctx.CallbackContext, *model.LLMRequest, *model.LLMResponse, error) (*model.LLMResponse, error)
	OnModelError func(callbackctx.CallbackContext, *model.LLMRequest, error) (*model.LLMResponse, error)

	BeforeTool  func(callbackctx.ToolContext, string, map[string]any) (map[string]any, error)
	AfterTool   func(callbackctx.ToolContext, string, map[string]any, map[string]any, error) (map[string]any, error)
	OnToolError func(callbackctx.ToolContext, string, map[string]any, error) (map[string]any, error)
}

// New creates a Plugin from its Config.
func New(cfg Config) *Plugin {
	return &Plugin{
		name:           cfg.Name,
		beforeAgent:    cfg.BeforeAgent,
		afterAgent:     cfg.AfterAgent,
		beforeModel:    cfg.BeforeModel,
		afterModel:     cfg.AfterModel,
		onModelError:   cfg.OnModelError,
		beforeTool:     cfg.BeforeTool,
		afterTool:      cfg.AfterTool,
		onToolError:    cfg.OnToolError,
	}
}

// Name returns the plugin's name.
func (p *Plugin) Name() string { return p.name }

// BeforeAgentCallback returns the before-agent hook, or nil.
func (p *Plugin) BeforeAgentCallback() func(callbackctx.CallbackContext) (*event.Event, error) {
	return p.beforeAgent
}

// AfterAgentCallback returns the after-agent hook, or nil.
func (p *Plugin) AfterAgentCallback() func(callbackctx.CallbackContext, []*event.Event) (*event.Event, error) {
	return p.afterAgent
}

// BeforeModelCallback returns the before-model hook, or nil.
func (p *Plugin) BeforeModelCallback() func(callbackctx.CallbackContext, *model.LLMRequest) (*model.LLMResponse, error) {
	return p.beforeModel
}

// AfterModelCallback returns the after-model hook, or nil.
func (p *Plugin) AfterModelCallback() func(callbackctx.CallbackContext, *model.LLMRequest, *model.LLMResponse, error) (*model.LLMResponse, error) {
	return p.afterModel
}

// OnModelErrorCallback returns the model-error-recovery hook, or nil.
func (p *Plugin) OnModelErrorCallback() func(callbackctx.CallbackContext, *model.LLMRequest, error) (*model.LLMResponse, error) {
	return p.onModelError
}

// BeforeToolCallback returns the before-tool hook, or nil.
func (p *Plugin) BeforeToolCallback() func(callbackctx.ToolContext, string, map[string]any) (map[string]any, error) {
	return p.beforeTool
}

// AfterToolCallback returns the after-tool hook, or nil.
func (p *Plugin) AfterToolCallback() func(callbackctx.ToolContext, string, map[string]any, map[string]any, error) (map[string]any, error) {
	return p.afterTool
}

// OnToolErrorCallback returns the tool-error-recovery hook, or nil.
func (p *Plugin) OnToolErrorCallback() func(callbackctx.ToolContext, string, map[string]any, error) (map[string]any, error) {
	return p.onToolError
}
