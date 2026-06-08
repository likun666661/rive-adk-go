// Package tool defines function tools that can be called by an agent via the flow.
//
// A FunctionTool has a Name, Description, and a Run method that receives
// a map of arguments and returns a result map or an error.
//
// Tool errors are surfaced as result entries with an "error" key rather
// than being swallowed, so the flow can propagate them to events.
package tool

import (
	"fmt"
)

// FunctionTool is a callable tool that performs a specific task.
type FunctionTool interface {
	Name() string
	Description() string
	Run(args map[string]any) (map[string]any, error)
}

// FuncTool is a helper that wraps a plain function into a FunctionTool.
type FuncTool struct {
	name        string
	description string
	run         func(args map[string]any) (map[string]any, error)
}

func (f *FuncTool) Name() string                                  { return f.name }
func (f *FuncTool) Description() string                           { return f.description }
func (f *FuncTool) Run(args map[string]any) (map[string]any, error) { return f.run(args) }

// NewFunctionTool creates a FunctionTool from a name, description, and run function.
func NewFunctionTool(name, description string, run func(args map[string]any) (map[string]any, error)) FunctionTool {
	return &FuncTool{
		name:        name,
		description: description,
		run:         run,
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
