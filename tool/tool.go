// Package tool defines function tools that can be called by an agent via the flow.
//
// The package provides a layered abstraction:
//
//	Tool           — minimal public interface (Name, Description)
//	FunctionTool   — extends Tool with Run execution
//	Declaration    — tool metadata for LLM requests (name, description, schemas)
//	Toolset        — dynamic collection of tools with Name and Tools()
//	RequestProcessor — injects tool declarations into an LLMRequest
//
// Tool errors are surfaced as result entries with an "error" key rather
// than being swallowed, so the flow can propagate them to events.
package tool

import (
	"errors"
	"fmt"
	"sort"

	"github.com/likun666661/rive-adk-go/model"
)

// ErrConfirmationRequired indicates that the tool requires user confirmation.
var ErrConfirmationRequired = errors.New("requires confirmation, please approve or reject")

// ErrConfirmationRejected indicates that the tool call confirmation was rejected.
var ErrConfirmationRejected = errors.New("call is rejected")

// ConfirmationProvider defines a function that dynamically determines whether
// a specific tool execution requires user confirmation.
type ConfirmationProvider func(toolName string, toolInput map[string]any) bool

// Tool is the minimal public interface for a tool.
// Every tool must have a Name and Description for identification by the LLM.
type Tool interface {
	Name() string
	Description() string

	// IsLongRunning indicates whether the tool is a long-running operation,
	// which typically returns a resource ID first and finishes later.
	// Long-running tools are annotated in their declaration to help the
	// LLM avoid redundant calls.
	IsLongRunning() bool
}

// FunctionTool extends Tool with a Run method for local execution.
type FunctionTool interface {
	Tool
	Run(args map[string]any) (map[string]any, error)
}

// Declaration holds the metadata needed to present a tool to an LLM.
// InputSchema and OutputSchema are JSON‑Schema–like maps describing the
// parameters and return value shapes respectively.
type Declaration struct {
	Name         string
	Description  string
	InputSchema  map[string]any
	OutputSchema map[string]any
}

// NewDeclaration creates a Declaration with the required fields.
// Schemas are optional and default to nil.
func NewDeclaration(name, description string, inputSchema, outputSchema map[string]any) Declaration {
	return Declaration{
		Name:         name,
		Description:  description,
		InputSchema:  cloneSchema(inputSchema),
		OutputSchema: cloneSchema(outputSchema),
	}
}

// DeclarationProvider is optionally implemented by tools that can
// provide a stable Declaration for LLM requests.
type DeclarationProvider interface {
	Declaration() Declaration
}

// FuncTool is a helper that wraps a plain function into a FunctionTool.
// It optionally carries a Declaration for LLM‑facing metadata.
type FuncTool struct {
	name        string
	description string
	decl        Declaration
	run         func(args map[string]any) (map[string]any, error)
	longRunning bool
}

func (f *FuncTool) Name() string                                    { return f.name }
func (f *FuncTool) Description() string                             { return f.description }
func (f *FuncTool) IsLongRunning() bool                             { return f.longRunning }
func (f *FuncTool) Run(args map[string]any) (map[string]any, error) { return f.run(args) }
func (f *FuncTool) Declaration() Declaration                        { return cloneDeclaration(f.decl) }

// NewFunctionTool creates a FunctionTool from a name, description, and run function.
// The resulting tool has an empty Declaration and is not long-running.
func NewFunctionTool(name, description string, run func(args map[string]any) (map[string]any, error)) FunctionTool {
	return &FuncTool{
		name:        name,
		description: description,
		decl:        Declaration{},
		run:         run,
	}
}

// NewFunctionToolWithDeclaration creates a FunctionTool with an explicit
// Declaration that can be exposed to the LLM.
func NewFunctionToolWithDeclaration(name, description string, decl Declaration, run func(args map[string]any) (map[string]any, error)) FunctionTool {
	return &FuncTool{
		name:        name,
		description: description,
		decl:        cloneDeclaration(decl),
		run:         run,
	}
}

// NewLongRunningFunctionTool creates a FunctionTool marked as long-running.
// The Declaration description is automatically annotated to inform the LLM
// not to repeat calls.
func NewLongRunningFunctionTool(name, description string, decl Declaration, run func(args map[string]any) (map[string]any, error)) FunctionTool {
	decl = cloneDeclaration(decl)
	if decl.Description != "" {
		decl.Description += "\n\nNOTE: This is a long-running operation. Do not call this tool again if it has already returned some intermediate or pending status."
	} else {
		decl.Description = "NOTE: This is a long-running operation. Do not call this tool again if it has already returned some intermediate or pending status."
	}
	return &FuncTool{
		name:        name,
		description: description,
		decl:        decl,
		run:         run,
		longRunning: true,
	}
}

// CallResult captures the result of executing a single tool call.
type CallResult struct {
	CallID string
	Name   string
	Result map[string]any
	Error  string
}

// Execute runs a single tool call and returns a structured result.
// Tool errors are captured in the Error field and Result["error"] rather
// than being returned as a Go error, so the flow always has a CallResult.
func Execute(callID, name string, args map[string]any, t FunctionTool) CallResult {
	cr := CallResult{CallID: callID, Name: name}
	if t == nil {
		cr.Error = fmt.Sprintf("tool %q not found", name)
		cr.Result = map[string]any{"error": cr.Error}
		return cr
	}
	result, err := t.Run(args)
	if err != nil {
		cr.Error = err.Error()
		if result == nil {
			result = map[string]any{"error": err.Error()}
		} else if _, ok := result["error"]; !ok {
			result["error"] = err.Error()
		}
	}
	cr.Result = result
	return cr
}

// MergeResults combines a list of parallel tool call results into a single
// result map. Keys are the tool call IDs to allow deterministic lookups.
func MergeResults(results []CallResult) map[string]any {
	merged := make(map[string]any, len(results))
	for _, r := range results {
		entry := map[string]any{
			"name":   r.Name,
			"result": r.Result,
		}
		if r.Error != "" {
			entry["error"] = r.Error
		}
		merged[r.CallID] = entry
	}
	return merged
}

// ---------------------------------------------------------------------------
// Toolset — dynamic tool collections
// ---------------------------------------------------------------------------

// Toolset is a dynamic collection of tools that can be queried by name.
// The Tools() method returns the current set of tools, which may vary
// across invocations (e.g. when the agent state changes).
type Toolset interface {
	Name() string
	Tools() ([]Tool, error)
}

// StaticToolset is a Toolset backed by a static slice of tools.
type StaticToolset struct {
	name  string
	tools []Tool
}

func (s *StaticToolset) Name() string           { return s.name }
func (s *StaticToolset) Tools() ([]Tool, error) { return s.tools, nil }

// NewStaticToolset creates a StaticToolset with the given name and tools.
func NewStaticToolset(name string, tools []Tool) *StaticToolset {
	return &StaticToolset{name: name, tools: tools}
}

// ---------------------------------------------------------------------------
// Filtering
// ---------------------------------------------------------------------------

// Predicate is a function that decides whether a Tool should be included.
type Predicate func(tool Tool) bool

// AllowedToolsPredicate returns a Predicate that allows only tools whose
// names appear in the given allow‑list.
func AllowedToolsPredicate(names ...string) Predicate {
	allow := make(map[string]bool, len(names))
	for _, n := range names {
		allow[n] = true
	}
	return func(t Tool) bool {
		return allow[t.Name()]
	}
}

// FilterToolset wraps a Toolset and applies a Predicate to each tool
// returned by the inner Toolset.Tools(). Only tools for which the
// Predicate returns true are included.
type FilterToolset struct {
	name      string
	inner     Toolset
	predicate Predicate
}

func (f *FilterToolset) Name() string { return f.name }

func (f *FilterToolset) Tools() ([]Tool, error) {
	all, err := f.inner.Tools()
	if err != nil {
		return nil, err
	}
	filtered := make([]Tool, 0, len(all))
	for _, t := range all {
		if f.predicate(t) {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

// NewFilterToolset creates a FilterToolset wrapping inner with the given Predicate.
func NewFilterToolset(name string, inner Toolset, predicate Predicate) *FilterToolset {
	return &FilterToolset{name: name, inner: inner, predicate: predicate}
}

// ---------------------------------------------------------------------------
// Request processing — expose declarations to model.LLMRequest
// ---------------------------------------------------------------------------

// RequestProcessor can inspect or mutate an LLMRequest.
// Tool‑focused implementations inject function declarations into the request
// so the LLM knows which tools are available.
type RequestProcessor interface {
	ProcessRequest(req *model.LLMRequest) error
}

// InjectDeclarations collects Declarations from every tool in the list
// and sets them on req.ToolDeclarations. Tools that implement
// DeclarationProvider contribute their declaration; others are skipped.
//
// Declarations are sorted by name to guarantee deterministic ordering
// regardless of the input slice order.
func InjectDeclarations(req *model.LLMRequest, tools []Tool) {
	if req == nil {
		return
	}
	decls := CollectDeclarations(tools)
	result := make([]any, len(decls))
	for i, d := range decls {
		result[i] = d
	}
	req.ToolDeclarations = result
}

// CollectDeclarations gathers Declarations from tools that implement
// DeclarationProvider, returning them sorted by name.
func CollectDeclarations(tools []Tool) []Declaration {
	var decls []Declaration
	for _, t := range tools {
		if dp, ok := t.(DeclarationProvider); ok {
			d := cloneDeclaration(dp.Declaration())
			if d.Name != "" {
				decls = append(decls, d)
			}
		}
	}
	sort.Slice(decls, func(i, j int) bool {
		return decls[i].Name < decls[j].Name
	})
	return decls
}

// FunctionToolAsTool adapts a FunctionTool to the Tool interface.
// This is a convenience for placing FunctionTool values into a Toolset.
func FunctionToolAsTool(ft FunctionTool) Tool {
	return ft
}

// ---------------------------------------------------------------------------
// Context-aware tool interfaces
// ---------------------------------------------------------------------------

// ContextFunctionTool extends FunctionTool with a Run method that
// receives a ToolContext, giving the tool access to invocation state,
// confirmation status, and session identity.
type ContextFunctionTool interface {
	FunctionTool
	RunWithContext(ctx ToolContext, args map[string]any) (map[string]any, error)
}

// StreamingFunctionTool is a tool that returns results as a sequence
// of string chunks rather than a single result map.
type StreamingFunctionTool interface {
	Tool
	Declaration() Declaration
	RunStream(args map[string]any) ([]StreamChunk, error)
}

// StreamChunk represents a single chunk from a streaming tool result.
type StreamChunk struct {
	Text  string
	Error string
	Final bool
}

// CollectStreamChunks concatenates all stream chunks into a normal
// function response map. This is the non-live-mode fallback.
func CollectStreamChunks(chunks []StreamChunk) (map[string]any, error) {
	var text string
	var errMsg string
	for _, c := range chunks {
		if c.Error != "" {
			errMsg = c.Error
		}
		text += c.Text
	}
	if errMsg != "" {
		return map[string]any{"result": text, "error": errMsg}, fmt.Errorf("%s", errMsg)
	}
	return map[string]any{"result": text}, nil
}

// ---------------------------------------------------------------------------
// Confirmation wrapper
// ---------------------------------------------------------------------------

// WithConfirmation wraps a FunctionTool to add Human-in-the-Loop
// confirmation logic. When requireConfirmation is true or the
// ConfirmationProvider returns true, the tool will:
//   - On the first call: produce a confirmation-required result
//   - On a confirmed call: execute the tool
//   - On a rejected call: produce a rejected result
func WithConfirmation(ft FunctionTool, requireConfirmation bool, provider ConfirmationProvider) FunctionTool {
	return &confirmationTool{
		inner:               ft,
		requireConfirmation: requireConfirmation,
		provider:            provider,
	}
}

// ConfirmationControl provides access to the SetConfirmed method
// on a confirmation-wrapped tool.
type ConfirmationControl interface {
	SetConfirmed(approved bool)
}

var _ ConfirmationControl = (*confirmationTool)(nil)

type confirmationTool struct {
	inner               FunctionTool
	requireConfirmation bool
	provider            ConfirmationProvider
	confirmed           bool
	confirmedCall       bool
}

func (c *confirmationTool) Name() string        { return c.inner.Name() }
func (c *confirmationTool) Description() string { return c.inner.Description() }
func (c *confirmationTool) IsLongRunning() bool { return c.inner.IsLongRunning() }
func (c *confirmationTool) Declaration() Declaration {
	if dp, ok := c.inner.(DeclarationProvider); ok {
		return cloneDeclaration(dp.Declaration())
	}
	return Declaration{}
}

func (c *confirmationTool) Run(args map[string]any) (map[string]any, error) {
	// If we've been externally confirmed (via SetConfirmed), execute.
	if c.confirmedCall {
		c.confirmedCall = false
		return c.inner.Run(args)
	}

	// Check if confirmation was explicitly rejected.
	if c.confirmed {
		return c.rejectedResult(), fmt.Errorf("tool %q %w", c.Name(), ErrConfirmationRejected)
	}

	// Determine if confirmation is needed.
	needsConfirmation := c.requireConfirmation
	if c.provider != nil {
		needsConfirmation = c.provider(c.Name(), args)
	}

	if !needsConfirmation {
		return c.inner.Run(args)
	}

	return c.confirmationRequiredResult(), fmt.Errorf("tool %q %w", c.Name(), ErrConfirmationRequired)
}

func (c *confirmationTool) SetConfirmed(approved bool) {
	c.confirmedCall = approved
	c.confirmed = !approved
}

func (c *confirmationTool) confirmationRequiredResult() map[string]any {
	return map[string]any{
		"confirmation_required": true,
		"hint":                  fmt.Sprintf("Please approve or reject the tool call %s()", c.Name()),
	}
}

func (c *confirmationTool) rejectedResult() map[string]any {
	return map[string]any{
		"confirmation_rejected": true,
		"error":                 fmt.Sprintf("tool call %q was rejected by user", c.Name()),
	}
}

// ContextExecute runs a single tool call with a ToolContext, preferring
// ContextFunctionTool.RunWithContext over FunctionTool.Run when available.
func ContextExecute(ctx ToolContext, callID, name string, args map[string]any, t FunctionTool) CallResult {
	cr := CallResult{CallID: callID, Name: name}
	if t == nil {
		cr.Error = fmt.Sprintf("tool %q not found", name)
		cr.Result = map[string]any{"error": cr.Error}
		return cr
	}

	var result map[string]any
	var err error

	if cft, ok := t.(ContextFunctionTool); ok {
		result, err = cft.RunWithContext(ctx, args)
	} else {
		result, err = t.Run(args)
	}

	if err != nil {
		if errors.Is(err, ErrConfirmationRequired) {
			cr.Error = err.Error()
			cr.Result = result
			if cr.Result == nil {
				cr.Result = map[string]any{
					"error":                 err.Error(),
					"confirmation_required": true,
				}
			}
			return cr
		}
		if errors.Is(err, ErrConfirmationRejected) {
			cr.Error = err.Error()
			cr.Result = result
			if cr.Result == nil {
				cr.Result = map[string]any{
					"error":                 err.Error(),
					"confirmation_rejected": true,
				}
			}
			return cr
		}
		cr.Error = err.Error()
		if result == nil {
			result = map[string]any{"error": err.Error()}
		} else if _, ok := result["error"]; !ok {
			result["error"] = err.Error()
		}
	}
	cr.Result = result
	return cr
}

func cloneDeclaration(d Declaration) Declaration {
	return Declaration{
		Name:         d.Name,
		Description:  d.Description,
		InputSchema:  cloneSchema(d.InputSchema),
		OutputSchema: cloneSchema(d.OutputSchema),
	}
}

func cloneSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	copied := make(map[string]any, len(schema))
	for k, v := range schema {
		copied[k] = cloneSchemaValue(v)
	}
	return copied
}

func cloneSchemaValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		return cloneSchema(typed)
	case []any:
		copied := make([]any, len(typed))
		for i, item := range typed {
			copied[i] = cloneSchemaValue(item)
		}
		return copied
	default:
		return typed
	}
}
