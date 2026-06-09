// Package flow implements the core execution loop that orchestrates model calls,
// tool execution, and event production.
//
// Flow.Run runs the multi‑step loop:
//
//	for { runOneStep → if lastEvent.IsFinalResponse() → return }
//
// Each runOneStep:
//  1. Preprocess (request processor hooks)
//  2. Call model (with before/after model callbacks)
//  3. Postprocess (response processor hooks)
//  4. Build and yield a model response event
//  5. Handle function calls (parallel tool execution, state delta merge)
//
// Flow also supports tool callbacks (before/after) and error handling so that
// tool errors become proper event fields instead of silent successes.
package flow

import (
	"fmt"
	"strings"
	"sync"

	"github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/session"
	"github.com/likun666661/rive-adk-go/tool"
)

// RequestProcessor can inspect or mutate the LLM request before the model call.
// If it returns a non‑nil event the step short‑circuits and yields that event.
type RequestProcessor func(ctx context.InvocationContext, req *model.LLMRequest) (*event.Event, error)

// ResponseProcessor can inspect or mutate the LLM response after the model call.
type ResponseProcessor func(ctx context.InvocationContext, req *model.LLMRequest, resp *model.LLMResponse) error

// BeforeModelCallback is invoked before the model call. Returning a non‑nil
// response short‑circuits the call.
type BeforeModelCallback func(ctx context.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error)

// AfterModelCallback is invoked after the model call.
type AfterModelCallback func(ctx context.InvocationContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error)

// BeforeToolCallback is invoked before a single tool executes. Returning a
// non‑nil result short‑circuits the actual tool call.
type BeforeToolCallback func(ctx context.InvocationContext, toolName string, args map[string]any) (map[string]any, error)

// AfterToolCallback is invoked after a single tool executes.
type AfterToolCallback func(ctx context.InvocationContext, toolName string, args, result map[string]any, runErr error) (map[string]any, error)

// Flow orchestrates the model‑and‑tool execution loop for a single agent.
type Flow struct {
	Model                model.LLM
	Tools                map[string]tool.FunctionTool
	Toolsets             []tool.Toolset
	RequestProcessors    []RequestProcessor
	ResponseProcessors   []ResponseProcessor
	BeforeModelCallbacks []BeforeModelCallback
	AfterModelCallbacks  []AfterModelCallback
	BeforeToolCallbacks  []BeforeToolCallback
	AfterToolCallbacks   []AfterToolCallback

	resolvedTools    map[string]tool.Tool
	resolvedToolList []tool.Tool
}

// Run executes the full multi‑step loop until a final response is reached.
func (f *Flow) Run(ctx context.InvocationContext) ([]*event.Event, error) {
	if f.Model == nil {
		return nil, fmt.Errorf("flow: model not configured for agent %q", ctx.Agent().Name())
	}

	var allEvents []*event.Event
	for step := 1; ; step++ {
		if ctx.Ended() {
			return allEvents, nil
		}

		stepEvents, err := f.runOneStep(ctx, step)
		if err != nil {
			return allEvents, err
		}
		if len(stepEvents) == 0 {
			return allEvents, nil
		}
		allEvents = append(allEvents, stepEvents...)

		modelEvent := stepEvents[0]
		if modelEvent.IsFinalResponse() {
			return allEvents, nil
		}
		if modelEvent.Partial {
			return allEvents, fmt.Errorf("flow: model event is partial (streaming limit reached)")
		}
	}
}

// runOneStep executes a single iteration: preprocess → callModel → postprocess
// → finalizeEvent → yield → handleFunctionCalls.
func (f *Flow) runOneStep(ctx context.InvocationContext, step int) ([]*event.Event, error) {
	req := &model.LLMRequest{
		Model: f.Model.Name(),
	}

	ev, err := f.preprocess(ctx, req)
	if err != nil {
		return nil, err
	}
	if ev != nil {
		return []*event.Event{ev}, nil
	}
	if ctx.Ended() {
		return nil, nil
	}

	f.injectToolDeclarations(req)

	var resp *model.LLMResponse
	for {
		var modelErr error
		resp, modelErr = f.callModel(ctx, req)
		if modelErr != nil {
			return nil, modelErr
		}
		if resp == nil {
			return nil, fmt.Errorf("flow: model %q returned nil response", f.Model.Name())
		}

		if err := f.postprocess(ctx, req, resp); err != nil {
			return nil, err
		}

		if resp.Content == nil && resp.ErrorCode == "" && !resp.Interrupted {
			continue
		}
		break
	}

	modelEvent := f.finalizeModelResponseEvent(ctx, step, resp)
	if modelEvent == nil {
		return nil, nil
	}

	events := []*event.Event{modelEvent}

	if resp.Partial {
		return events, nil
	}

	toolEvent, err := f.handleFunctionCalls(ctx, step, modelEvent)
	if err != nil {
		return events, err
	}
	if toolEvent != nil {
		events = append(events, toolEvent)
	}

	return events, nil
}

func (f *Flow) preprocess(ctx context.InvocationContext, req *model.LLMRequest) (*event.Event, error) {
	for _, p := range f.RequestProcessors {
		if p == nil {
			continue
		}
		ev, err := p(ctx, req)
		if err != nil {
			return nil, err
		}
		if ev != nil {
			return ev, nil
		}
	}
	return nil, nil
}

func (f *Flow) callModel(ctx context.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
	for _, cb := range f.BeforeModelCallbacks {
		if cb == nil {
			continue
		}
		resp, err := cb(ctx, req)
		if resp != nil || err != nil {
			return resp, err
		}
	}

	resp, err := f.Model.GenerateContent(req)

	for _, cb := range f.AfterModelCallbacks {
		if cb == nil {
			continue
		}
		overrideResp, overrideErr := cb(ctx, req, resp, err)
		if overrideResp != nil || overrideErr != nil {
			resp = overrideResp
			err = overrideErr
			break
		}
	}

	return resp, err
}

func (f *Flow) postprocess(ctx context.InvocationContext, req *model.LLMRequest, resp *model.LLMResponse) error {
	for _, p := range f.ResponseProcessors {
		if p == nil {
			continue
		}
		if err := p(ctx, req, resp); err != nil {
			return err
		}
	}
	return nil
}

func (f *Flow) finalizeModelResponseEvent(ctx context.InvocationContext, step int, resp *model.LLMResponse) *event.Event {
	ev := event.NewEvent(
		fmt.Sprintf("%s-step-%d", ctx.InvocationID(), step),
		ctx.Agent().Name(),
		event.RoleModel,
	)
	ev.Branch = ctx.Branch()

	if resp.Content != nil {
		content := &event.Content{
			Role: event.Role(resp.Content.Role),
		}
		for _, p := range resp.Content.Parts {
			part := event.Part{
				Text:             p.Text,
				FunctionCall:     p.FunctionCall,
				FunctionResponse: p.FunctionResponse,
			}
			content.Parts = append(content.Parts, part)
		}
		ev.Content = content
	}
	if resp.Partial {
		ev.Partial = true
	}
	if resp.TurnComplete {
		ev.TurnComplete = true
	}
	if resp.Interrupted {
		ev.Interrupted = true
	}
	if resp.ErrorCode != "" {
		ev.ErrorCode = resp.ErrorCode
	}
	if resp.ErrorMessage != "" {
		ev.ErrorMessage = resp.ErrorMessage
	}
	return ev
}

func (f *Flow) handleFunctionCalls(ctx context.InvocationContext, step int, modelEvent *event.Event) (*event.Event, error) {
	fnCalls := modelEvent.FunctionCalls()
	if len(fnCalls) == 0 {
		return nil, nil
	}

	results := make([]tool.CallResult, len(fnCalls))
	var wg sync.WaitGroup

	for i, fnCall := range fnCalls {
		wg.Add(1)
		go func(idx int, fc *event.FunctionCall) {
			defer wg.Done()
			results[idx] = f.executeToolCall(ctx, fc)
		}(i, fnCall)
	}
	wg.Wait()

	merged := mergeResultsToEvent(ctx, step, results)
	if merged != nil && merged.Actions.StateDelta != nil {
		session.MergeStateDelta(ctx.Session().State(), merged.Actions.StateDelta)
	}

	return merged, nil
}

func (f *Flow) executeToolCall(ctx context.InvocationContext, fc *event.FunctionCall) tool.CallResult {
	args := fc.Args
	if args == nil {
		args = map[string]any{}
	}

	for _, cb := range f.BeforeToolCallbacks {
		if cb == nil {
			continue
		}
		result, err := cb(ctx, fc.Name, args)
		if result != nil || err != nil {
			if err != nil {
				return tool.CallResult{
					CallID: fc.ID,
					Name:   fc.Name,
					Result: map[string]any{"error": err.Error()},
					Error:  err.Error(),
				}
			}
			return tool.CallResult{CallID: fc.ID, Name: fc.Name, Result: result}
		}
	}

	t := f.lookupTool(fc.Name)
	if t == nil {
		errMsg := fmt.Sprintf("tool %q not found", fc.Name)
		return tool.CallResult{
			CallID: fc.ID,
			Name:   fc.Name,
			Result: map[string]any{"error": errMsg},
			Error:  errMsg,
		}
	}

	var cr tool.CallResult
	switch executable := t.(type) {
	case tool.StreamingFunctionTool:
		cr = tool.ExecuteStream(fc.ID, fc.Name, args, executable)
	case tool.FunctionTool:
		cr = tool.Execute(fc.ID, fc.Name, args, executable)
	default:
		errMsg := fmt.Sprintf("tool %q is not executable", fc.Name)
		cr = tool.CallResult{
			CallID: fc.ID,
			Name:   fc.Name,
			Result: map[string]any{"error": errMsg},
			Error:  errMsg,
		}
	}

	for _, cb := range f.AfterToolCallbacks {
		if cb == nil {
			continue
		}
		var runErr error
		if cr.Error != "" {
			runErr = fmt.Errorf("%s", cr.Error)
		}
		overrideResult, overrideErr := cb(ctx, fc.Name, args, cr.Result, runErr)
		if overrideResult != nil || overrideErr != nil {
			if overrideErr != nil {
				cr.Error = overrideErr.Error()
				if overrideResult != nil {
					overrideResult["error"] = overrideErr.Error()
					cr.Result = overrideResult
				} else {
					cr.Result = map[string]any{"error": overrideErr.Error()}
				}
			} else {
				cr.Result = overrideResult
				cr.Error = ""
			}
			break
		}
	}

	return cr
}

func mergeResultsToEvent(ctx context.InvocationContext, step int, results []tool.CallResult) *event.Event {
	ev := event.NewEvent(
		fmt.Sprintf("%s-step-%d-toolresults", ctx.InvocationID(), step),
		ctx.Agent().Name(),
		event.RoleTool,
	)
	ev.Branch = ctx.Branch()

	content := &event.Content{Role: event.RoleTool}
	var stateDelta map[string]any
	var errParts []string

	for _, r := range results {
		part := event.Part{
			FunctionResponse: &event.FunctionResponse{
				ID:     r.CallID,
				Name:   r.Name,
				Result: r.Result,
			},
		}
		if r.Error != "" {
			part.FunctionResponse.Error = r.Error
			errParts = append(errParts, fmt.Sprintf("%s: %s", r.Name, r.Error))
		}
		content.Parts = append(content.Parts, part)

		if sd, ok := r.Result["state_delta"]; ok {
			if sdMap, ok := sd.(map[string]any); ok {
				if stateDelta == nil {
					stateDelta = make(map[string]any)
				}
				for k, v := range sdMap {
					stateDelta[k] = v
				}
			}
		}
	}

	ev.Content = content
	if stateDelta != nil {
		ev.Actions.StateDelta = stateDelta
	}
	if len(errParts) > 0 {
		ev.ErrorMessage = strings.Join(errParts, "; ")
	}

	return ev
}

func (f *Flow) injectToolDeclarations(req *model.LLMRequest) {
	if req == nil {
		return
	}
	f.resolveToolsets()
	tool.InjectDeclarations(req, f.resolvedToolList)
}

func (f *Flow) resolveToolsets() {
	if f.resolvedTools != nil {
		return
	}
	f.resolvedTools = make(map[string]tool.Tool, len(f.Tools))
	for k, v := range f.Tools {
		f.resolvedTools[k] = v
	}
	for _, ts := range f.Toolsets {
		if ts == nil {
			continue
		}
		tsTools, err := ts.Tools()
		if err != nil {
			continue
		}
		for _, t := range tsTools {
			if _, exists := f.resolvedTools[t.Name()]; !exists {
				f.resolvedTools[t.Name()] = t
			}
		}
	}
	var tools []tool.Tool
	for _, ft := range f.resolvedTools {
		tools = append(tools, ft)
	}
	f.resolvedToolList = tools
}

func (f *Flow) lookupTool(name string) tool.Tool {
	if t, ok := f.Tools[name]; ok {
		return t
	}
	if f.resolvedTools == nil {
		f.resolveToolsets()
	}
	return f.resolvedTools[name]
}
