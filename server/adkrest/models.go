// Package adkrest provides an HTTP JSON and SSE server layer for the
// ADK Go runtime, exposing runner.Run through REST endpoints.
package adkrest

import (
	"github.com/likun666661/rive-adk-go/event"
)

// RunRequest is the JSON body for /run and /run_sse endpoints.
type RunRequest struct {
	AppName   string `json:"appName"`
	UserID    string `json:"userId"`
	SessionID string `json:"sessionId"`
	Message   string `json:"newMessage"`
	Streaming bool   `json:"streaming,omitempty"`
}

// PartResponse is a JSON-friendly representation of event.Part.
type PartResponse struct {
	Text             string                 `json:"text,omitempty"`
	FunctionCall     *FunctionCallResponse  `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponseResp  `json:"functionResponse,omitempty"`
	Thought          bool                   `json:"thought,omitempty"`
}

// FunctionCallResponse mirrors event.FunctionCall for JSON.
type FunctionCallResponse struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// FunctionResponseResp mirrors event.FunctionResponse for JSON.
type FunctionResponseResp struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Result map[string]any `json:"result"`
	Error  string         `json:"error,omitempty"`
}

// ContentResponse mirrors event.Content for JSON.
type ContentResponse struct {
	Role  string         `json:"role"`
	Parts []PartResponse `json:"parts,omitempty"`
}

// ActionsResponse mirrors event.EventActions for JSON.
type ActionsResponse struct {
	StateDelta                  map[string]any                       `json:"stateDelta,omitempty"`
	ArtifactDelta               map[string]int64                    `json:"artifactDelta,omitempty"`
	TransferToAgent             string                              `json:"transferToAgent,omitempty"`
	EndInvocation               bool                                `json:"endInvocation,omitempty"`
	Escalate                    bool                                `json:"escalate,omitempty"`
	SkipSummarization           bool                                `json:"skipSummarization,omitempty"`
	RequestedToolConfirmations  map[string]ToolConfirmationResponse `json:"requestedToolConfirmations,omitempty"`
}

// ToolConfirmationResponse mirrors event.ToolConfirmation for JSON.
type ToolConfirmationResponse struct {
	Hint      string `json:"hint"`
	Confirmed bool   `json:"confirmed"`
	Payload   any    `json:"payload,omitempty"`
}

// EventResponse is the JSON representation of an event.Event.
type EventResponse struct {
	ID           string            `json:"id"`
	Author       string            `json:"author"`
	Role         string            `json:"role"`
	Content      *ContentResponse  `json:"content,omitempty"`
	Actions      ActionsResponse   `json:"actions"`
	Partial      bool              `json:"partial,omitempty"`
	Timestamp    int64             `json:"timestamp"`
	Branch       string            `json:"branch,omitempty"`
	ErrorCode    string            `json:"errorCode,omitempty"`
	ErrorMessage string            `json:"errorMessage,omitempty"`
	Interrupted  bool              `json:"interrupted,omitempty"`
	TurnComplete bool              `json:"turnComplete,omitempty"`
}

// EventToResponse converts an event.Event to an EventResponse.
func EventToResponse(ev *event.Event) EventResponse {
	if ev == nil {
		return EventResponse{}
	}
	resp := EventResponse{
		ID:           ev.ID,
		Author:       ev.Author,
		Role:         string(ev.Role),
		Partial:      ev.Partial,
		Timestamp:    ev.Timestamp.UnixMilli(),
		Branch:       ev.Branch,
		ErrorCode:    ev.ErrorCode,
		ErrorMessage: ev.ErrorMessage,
		Interrupted:  ev.Interrupted,
		TurnComplete: ev.TurnComplete,
		Actions: ActionsResponse{
			StateDelta:                 ev.Actions.StateDelta,
			ArtifactDelta:             ev.Actions.ArtifactDelta,
			TransferToAgent:            ev.Actions.TransferToAgent,
			EndInvocation:              ev.Actions.EndInvocation,
			Escalate:                   ev.Actions.Escalate,
			SkipSummarization:          ev.Actions.SkipSummarization,
		},
	}

	if ev.Actions.RequestedToolConfirmations != nil {
		resp.Actions.RequestedToolConfirmations = make(map[string]ToolConfirmationResponse)
		for k, v := range ev.Actions.RequestedToolConfirmations {
			resp.Actions.RequestedToolConfirmations[k] = ToolConfirmationResponse{
				Hint:      v.Hint,
				Confirmed: v.Confirmed,
				Payload:   v.Payload,
			}
		}
	}

	if ev.Content != nil {
		resp.Content = &ContentResponse{
			Role:  string(ev.Content.Role),
			Parts: make([]PartResponse, len(ev.Content.Parts)),
		}
		for i, p := range ev.Content.Parts {
			pr := PartResponse{
				Text:    p.Text,
				Thought: p.Thought,
			}
			if p.FunctionCall != nil {
				pr.FunctionCall = &FunctionCallResponse{
					ID:   p.FunctionCall.ID,
					Name: p.FunctionCall.Name,
					Args: p.FunctionCall.Args,
				}
			}
			if p.FunctionResponse != nil {
				pr.FunctionResponse = &FunctionResponseResp{
					ID:     p.FunctionResponse.ID,
					Name:   p.FunctionResponse.Name,
					Result: p.FunctionResponse.Result,
					Error:  p.FunctionResponse.Error,
				}
			}
			resp.Content.Parts[i] = pr
		}
	}

	return resp
}
