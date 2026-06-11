package model

import "github.com/likun666661/rive-adk-go/event"

// ContentsFromEvents converts persisted/session events into model-facing
// contents. It is intentionally lossless for text, function calls, and function
// responses so a real LLM can see the same conversation state that the fake
// model-based flow tests exercise structurally.
func ContentsFromEvents(events []*event.Event) []LLMContent {
	contents := make([]LLMContent, 0, len(events))
	for _, ev := range events {
		if ev == nil || ev.Content == nil || len(ev.Content.Parts) == 0 {
			continue
		}
		content := LLMContent{
			Role:  string(ev.Content.Role),
			Parts: make([]LLMPart, 0, len(ev.Content.Parts)),
		}
		for _, part := range ev.Content.Parts {
			content.Parts = append(content.Parts, LLMPart{
				Text:             part.Text,
				FunctionCall:     cloneFunctionCall(part.FunctionCall),
				FunctionResponse: cloneFunctionResponse(part.FunctionResponse),
			})
		}
		contents = append(contents, content)
	}
	return contents
}

func cloneFunctionCall(fc *event.FunctionCall) *event.FunctionCall {
	if fc == nil {
		return nil
	}
	return &event.FunctionCall{
		ID:   fc.ID,
		Name: fc.Name,
		Args: cloneMap(fc.Args),
	}
}

func cloneFunctionResponse(fr *event.FunctionResponse) *event.FunctionResponse {
	if fr == nil {
		return nil
	}
	return &event.FunctionResponse{
		ID:     fr.ID,
		Name:   fr.Name,
		Result: cloneMap(fr.Result),
		Error:  fr.Error,
	}
}

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = cloneAny(v)
	}
	return dst
}

func cloneAny(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		dst := make([]any, len(typed))
		for i, item := range typed {
			dst[i] = cloneAny(item)
		}
		return dst
	default:
		return typed
	}
}
