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
//  6. Handle agent transfer (execute target agent inline)
//
// Flow also supports tool callbacks (before/after) and error handling so that
// tool errors become proper event fields instead of silent successes.
package flow

import (
	"fmt"
	"strings"
	"sync"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/callbackctx"
	"github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/plugin"
	"github.com/likun666661/rive-adk-go/session"
	"github.com/likun666661/rive-adk-go/tool"
	"github.com/likun666661/rive-adk-go/tool/transfer"
)

const maxTransferDepth = 10

// executableAgent mirrors runner.ExecutableAgent to avoid an import cycle
// between flow and runner.
type executableAgent interface {
	agent.Agent
	Execute(ctx agent.InvocationContext) ([]*event.Event, error)
}

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

// BeforeModelCallbackCtx is a context‑aware before‑model callback.
type BeforeModelCallbackCtx func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error)

// AfterModelCallbackCtx is a context‑aware after‑model callback.
type AfterModelCallbackCtx func(ctx callbackctx.CallbackContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error)

// BeforeToolCallbackCtx is a context‑aware before‑tool callback.
type BeforeToolCallbackCtx func(ctx callbackctx.ToolContext, toolName string, args map[string]any) (map[string]any, error)

// AfterToolCallbackCtx is a context‑aware after‑tool callback.
type AfterToolCallbackCtx func(ctx callbackctx.ToolContext, toolName string, args, result map[string]any, runErr error) (map[string]any, error)

// Flow orchestrates the model‑and‑tool execution loop for a single agent.
type Flow struct {
	Model                model.LLM
	Tools                map[string]tool.FunctionTool
	Toolsets             []tool.Toolset
	PluginManager        *plugin.Manager
	RequestProcessors    []RequestProcessor
	ResponseProcessors   []ResponseProcessor
	BeforeModelCallbacks []BeforeModelCallback
	AfterModelCallbacks  []AfterModelCallback
	BeforeToolCallbacks  []BeforeToolCallback
	AfterToolCallbacks   []AfterToolCallback

	// Context‑aware callback lists (record side effects via EventActions).
	BeforeModelCallbacksCtx []BeforeModelCallbackCtx
	AfterModelCallbacksCtx  []AfterModelCallbackCtx
	BeforeToolCallbacksCtx  []BeforeToolCallbackCtx
	AfterToolCallbacksCtx   []AfterToolCallbackCtx

	resolvedTools      map[string]tool.Tool
	resolvedToolList   []tool.Tool
	activeTransferTool *transfer.TransferToAgentTool
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

		stepEvents, err := f.runOneStep(ctx, step, allEvents)
		if err != nil {
			return allEvents, err
		}
		if len(stepEvents) == 0 {
			return allEvents, nil
		}
		allEvents = append(allEvents, stepEvents...)

		for _, ev := range stepEvents {
			if ev != nil && ev.Actions.EndInvocation {
				ctx.EndInvocation()
				return allEvents, nil
			}
		}

		modelEvent := stepEvents[0]
		if modelEvent.IsFinalResponse() {
			return allEvents, nil
		}
		if modelEvent.Partial {
			return allEvents, fmt.Errorf("flow: model event is partial (streaming limit reached)")
		}

		if len(stepEvents) > 1 {
			for _, ev := range stepEvents[1:] {
				if ev != nil && ev.Actions.TransferToAgent != "" {
					return allEvents, nil
				}
			}
		}
	}
}

// runOneStep executes a single iteration: preprocess → callModel → postprocess
// → finalizeEvent → yield → handleFunctionCalls → handleTransfer.
func (f *Flow) runOneStep(ctx context.InvocationContext, step int, priorEvents []*event.Event) ([]*event.Event, error) {
	currentAgent := ctx.Agent()

	history := append([]*event.Event{}, ctx.Session().Events()...)
	history = append(history, priorEvents...)
	req := &model.LLMRequest{
		Model:    f.Model.Name(),
		Contents: model.ContentsFromEvents(history),
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
	f.injectTransferTool(currentAgent, req)

	var resp *model.LLMResponse
	modelActions := &event.EventActions{}
	for {
		var modelErr error
		resp, modelErr = f.callModel(ctx, req, modelActions)
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

	modelEvent := f.finalizeModelResponseEvent(ctx, step, resp, modelActions)
	if modelEvent == nil {
		return nil, nil
	}

	events := []*event.Event{modelEvent}

	if resp.Partial {
		return events, nil
	}

	toolEvent, tt := f.handleFunctionCalls(ctx, step, modelEvent)
	if toolEvent != nil {
		events = append(events, toolEvent)

		if toolEvent.Actions.TransferToAgent != "" {
			transferEvents, transferErr := f.executeTransfer(ctx, step, toolEvent.Actions.TransferToAgent)
			if transferErr != nil {
				return events, transferErr
			}
			events = append(events, transferEvents...)
		}
	}

	if tt != nil {
		f.injectContextTransfer(tt, ctx)
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

func (f *Flow) callModel(ctx context.InvocationContext, req *model.LLMRequest, actions *event.EventActions) (*model.LLMResponse, error) {
	cctx := context.NewCallbackContextWithArtifactTracking(ctx, actions)

	if f.PluginManager != nil {
		resp, err := f.PluginManager.RunBeforeModelCallback(cctx, req)
		if resp != nil || err != nil {
			return resp, err
		}
	}

	for _, cb := range f.BeforeModelCallbacksCtx {
		if cb == nil {
			continue
		}
		resp, err := cb(cctx, req)
		if resp != nil || err != nil {
			return resp, err
		}
	}

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

	if err != nil && f.PluginManager != nil {
		recoveryResp, recoveryErr := f.PluginManager.RunOnModelErrorCallback(cctx, req, err)
		if recoveryResp != nil || recoveryErr != nil {
			return recoveryResp, recoveryErr
		}
	}

	if f.PluginManager != nil {
		afterResp, afterErr := f.PluginManager.RunAfterModelCallback(cctx, req, resp, err)
		if afterResp != nil || afterErr != nil {
			resp = afterResp
			err = afterErr
		}
	}

	for _, cb := range f.AfterModelCallbacksCtx {
		if cb == nil {
			continue
		}
		overrideResp, overrideErr := cb(cctx, req, resp, err)
		if overrideResp != nil || overrideErr != nil {
			resp = overrideResp
			err = overrideErr
			break
		}
	}

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

func (f *Flow) finalizeModelResponseEvent(ctx context.InvocationContext, step int, resp *model.LLMResponse, actions *event.EventActions) *event.Event {
	ev := event.NewEvent(
		fmt.Sprintf("%s-step-%d", ctx.InvocationID(), step),
		ctx.Agent().Name(),
		event.RoleModel,
	)
	ev.Branch = ctx.Branch()
	ev.Actions = compactEventActions(actions)

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

func (f *Flow) handleFunctionCalls(ctx context.InvocationContext, step int, modelEvent *event.Event) (*event.Event, *transfer.TransferToAgentTool) {
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

	var tt *transfer.TransferToAgentTool
	if f.activeTransferTool != nil && merged != nil && merged.Actions.TransferToAgent != "" {
		tt = f.activeTransferTool
	}

	return merged, tt
}

func (f *Flow) executeToolCall(ctx context.InvocationContext, fc *event.FunctionCall) tool.CallResult {
	args := fc.Args
	if args == nil {
		args = map[string]any{}
	}
	actions := &event.EventActions{}
	tctx := context.NewToolContext(ctx, fc.ID, actions)

	if f.PluginManager != nil {
		result, err := f.PluginManager.RunBeforeToolCallback(tctx, fc.Name, args)
		if result != nil || err != nil {
			if err != nil {
				return tool.CallResult{
					CallID:  fc.ID,
					Name:    fc.Name,
					Result:  map[string]any{"error": err.Error()},
					Error:   err.Error(),
					Actions: compactEventActions(actions),
				}
			}
			return tool.CallResult{CallID: fc.ID, Name: fc.Name, Result: result, Actions: compactEventActions(actions)}
		}
	}

	for _, cb := range f.BeforeToolCallbacksCtx {
		if cb == nil {
			continue
		}
		result, err := cb(tctx, fc.Name, args)
		if result != nil || err != nil {
			if err != nil {
				return tool.CallResult{
					CallID:  fc.ID,
					Name:    fc.Name,
					Result:  map[string]any{"error": err.Error()},
					Error:   err.Error(),
					Actions: compactEventActions(actions),
				}
			}
			return tool.CallResult{CallID: fc.ID, Name: fc.Name, Result: result, Actions: compactEventActions(actions)}
		}
	}

	for _, cb := range f.BeforeToolCallbacks {
		if cb == nil {
			continue
		}
		result, err := cb(ctx, fc.Name, args)
		if result != nil || err != nil {
			if err != nil {
				return tool.CallResult{
					CallID:  fc.ID,
					Name:    fc.Name,
					Result:  map[string]any{"error": err.Error()},
					Error:   err.Error(),
					Actions: compactEventActions(actions),
				}
			}
			return tool.CallResult{CallID: fc.ID, Name: fc.Name, Result: result, Actions: compactEventActions(actions)}
		}
	}

	t := f.lookupTool(fc.Name)
	if t == nil {
		errMsg := fmt.Sprintf("tool %q not found", fc.Name)
		cr := tool.CallResult{
			CallID:  fc.ID,
			Name:    fc.Name,
			Result:  map[string]any{"error": errMsg},
			Error:   errMsg,
			Actions: compactEventActions(actions),
		}
		return f.applyAfterToolPluginAndCallbacks(ctx, tctx, fc, args, cr)
	}

	var cr tool.CallResult
	switch executable := t.(type) {
	case tool.StreamingFunctionTool:
		cr = tool.ExecuteStream(fc.ID, fc.Name, args, executable)
	case tool.ContextFunctionTool:
		ttctx := tool.NewToolContext(ctx, fc.ID, actions, nil)
		cr = tool.ContextExecute(ttctx, fc.ID, fc.Name, args, executable)
	case tool.FunctionTool:
		cr = tool.Execute(fc.ID, fc.Name, args, executable)
	default:
		errMsg := fmt.Sprintf("tool %q is not executable", fc.Name)
		cr = tool.CallResult{
			CallID:  fc.ID,
			Name:    fc.Name,
			Result:  map[string]any{"error": errMsg},
			Error:   errMsg,
			Actions: compactEventActions(actions),
		}
	}

	cr.Actions = compactEventActions(actions)
	return f.applyAfterToolPluginAndCallbacks(ctx, tctx, fc, args, cr)
}

func (f *Flow) applyAfterToolPluginAndCallbacks(ctx context.InvocationContext, tctx callbackctx.ToolContext, fc *event.FunctionCall, args map[string]any, cr tool.CallResult) tool.CallResult {
	var runErr error
	if cr.Error != "" {
		runErr = fmt.Errorf("%s", cr.Error)
	}

	if runErr != nil && f.PluginManager != nil {
		recoveryResult, recoveryErr := f.PluginManager.RunOnToolErrorCallback(tctx, fc.Name, args, runErr)
		if recoveryResult != nil || recoveryErr != nil {
			if recoveryErr != nil {
				cr.Error = recoveryErr.Error()
				if recoveryResult != nil {
					recoveryResult["error"] = recoveryErr.Error()
					cr.Result = recoveryResult
				} else {
					cr.Result = map[string]any{"error": recoveryErr.Error()}
				}
			} else {
				cr.Result = recoveryResult
				cr.Error = ""
				runErr = nil
			}
		}
	}

	if f.PluginManager != nil {
		overrideResult, overrideErr := f.PluginManager.RunAfterToolCallback(tctx, fc.Name, args, cr.Result, runErr)
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
			cr.Actions = compactEventActions(tctx.Actions())
			return cr
		}
	}

	for _, cb := range f.AfterToolCallbacksCtx {
		if cb == nil {
			continue
		}
		overrideResult, overrideErr := cb(tctx, fc.Name, args, cr.Result, runErr)
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

	for _, cb := range f.AfterToolCallbacks {
		if cb == nil {
			continue
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

	cr.Actions = compactEventActions(tctx.Actions())
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
	for _, r := range results {
		ev.Actions = mergeEventActions(ev.Actions, r.Actions)
	}
	if len(errParts) > 0 {
		ev.ErrorMessage = strings.Join(errParts, "; ")
	}

	return ev
}

func mergeEventActions(dst, src event.EventActions) event.EventActions {
	if len(src.StateDelta) > 0 {
		if dst.StateDelta == nil {
			dst.StateDelta = make(map[string]any, len(src.StateDelta))
		}
		for k, v := range src.StateDelta {
			dst.StateDelta[k] = v
		}
	}
	if len(src.ArtifactDelta) > 0 {
		if dst.ArtifactDelta == nil {
			dst.ArtifactDelta = make(map[string]int64, len(src.ArtifactDelta))
		}
		for k, v := range src.ArtifactDelta {
			dst.ArtifactDelta[k] = v
		}
	}
	if src.TransferToAgent != "" {
		dst.TransferToAgent = src.TransferToAgent
	}
	if src.EndInvocation {
		dst.EndInvocation = true
	}
	if src.Escalate {
		dst.Escalate = true
	}
	if src.SkipSummarization {
		dst.SkipSummarization = true
	}
	if len(src.RequestedToolConfirmations) > 0 {
		if dst.RequestedToolConfirmations == nil {
			dst.RequestedToolConfirmations = make(map[string]event.ToolConfirmation, len(src.RequestedToolConfirmations))
		}
		for k, v := range src.RequestedToolConfirmations {
			dst.RequestedToolConfirmations[k] = v
		}
	}
	return compactEventActions(&dst)
}

func compactEventActions(actions *event.EventActions) event.EventActions {
	if actions == nil {
		return event.EventActions{}
	}
	compact := *actions
	if len(compact.StateDelta) == 0 {
		compact.StateDelta = nil
	}
	if len(compact.ArtifactDelta) == 0 {
		compact.ArtifactDelta = nil
	}
	if len(compact.RequestedToolConfirmations) == 0 {
		compact.RequestedToolConfirmations = nil
	}
	return compact
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
	if f.activeTransferTool != nil && f.activeTransferTool.Name() == name {
		return f.activeTransferTool
	}
	if f.resolvedTools == nil {
		f.resolveToolsets()
	}
	return f.resolvedTools[name]
}

// injectTransferTool checks if the current agent has transfer targets and,
// if so, injects the transfer_to_agent tool declaration and system
// instructions into the request.
func (f *Flow) injectTransferTool(currentAgent agent.Agent, req *model.LLMRequest) {
	f.activeTransferTool = transfer.InjectTransferTool(currentAgent, req)
}

// injectContextTransfer registers the transfer tool for custom execution
// via the invocation context so context-aware tool execution can set
// TransferToAgent actions.
func (f *Flow) injectContextTransfer(tt *transfer.TransferToAgentTool, ctx context.InvocationContext) {
	_ = tt
	_ = ctx
}

// executeTransfer finds the target agent by name and executes it inline,
// returning the events produced by the target.
func (f *Flow) executeTransfer(ctx context.InvocationContext, step int, targetName string) ([]*event.Event, error) {
	rootAgent := ctx.RootAgent()
	if rootAgent == nil {
		return nil, fmt.Errorf("flow: no root agent available for transfer to %q", targetName)
	}

	targetAgent := rootAgent.FindAgent(targetName)
	if targetAgent == nil {
		errEvent := event.NewEvent(
			fmt.Sprintf("%s-transfer-error-%d", ctx.InvocationID(), step),
			ctx.Agent().Name(),
			event.RoleTool,
		)
		errEvent.Content = &event.Content{
			Role: event.RoleTool,
			Parts: []event.Part{
				{
					FunctionResponse: &event.FunctionResponse{
						ID:     fmt.Sprintf("transfer-%d", step),
						Name:   "transfer_to_agent",
						Result: map[string]any{"error": fmt.Sprintf("invalid transfer target %q", targetName)},
						Error:  fmt.Sprintf("invalid transfer target %q", targetName),
					},
				},
			},
		}
		errEvent.ErrorMessage = fmt.Sprintf("invalid transfer target %q", targetName)
		return []*event.Event{errEvent}, nil
	}

	ea, ok := targetAgent.(executableAgent)
	if !ok {
		return nil, fmt.Errorf("flow: agent %q does not implement executableAgent", targetAgent.Name())
	}

	depth := transferDepth(ctx)
	if depth >= maxTransferDepth {
		return nil, fmt.Errorf(
			"flow: transfer loop detected: max depth %d reached at agent %q",
			maxTransferDepth, targetAgent.Name(),
		)
	}

	branch := targetAgent.Name()
	targetCtx := &transferContext{InvocationContext: ctx, targetAgent: targetAgent, branch: branch, depth: depth + 1}

	targetEvents, err := ea.Execute(targetCtx)
	if err != nil {
		return nil, fmt.Errorf("flow: transfer to %q failed: %w", targetName, err)
	}

	var allEvents []*event.Event
	allEvents = append(allEvents, targetEvents...)

	for _, ev := range targetEvents {
		if ev != nil && ev.Actions.TransferToAgent != "" {
			chainedEvents, chainedErr := f.executeTransfer(targetCtx, step, ev.Actions.TransferToAgent)
			if chainedErr != nil {
				return allEvents, chainedErr
			}
			allEvents = append(allEvents, chainedEvents...)
		}
	}

	return allEvents, nil
}

// transferDepth extracts the transfer depth from the context, or 0.
func transferDepth(ctx context.InvocationContext) int {
	if tc, ok := ctx.(*transferContext); ok {
		return tc.depth
	}
	return 0
}

// transferContext wraps an InvocationContext and overrides Agent()/AgentName()
// so the target agent sees itself as the current agent.
type transferContext struct {
	context.InvocationContext
	targetAgent agent.Agent
	branch      string
	depth       int
}

func (c *transferContext) Agent() agent.Agent { return c.targetAgent }
func (c *transferContext) AgentName() string  { return c.targetAgent.Name() }
func (c *transferContext) Branch() string     { return c.branch }
