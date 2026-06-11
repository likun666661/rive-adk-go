package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/likun666661/rive-adk-go/event"
)

const defaultOpenAICompatibleTimeout = 60 * time.Second

// OpenAICompatibleConfig configures an OpenAI-compatible chat/completions model.
// DeepSeek can be used with BaseURL "https://api.deepseek.com/v1" and model
// "deepseek-chat".
type OpenAICompatibleConfig struct {
	Name       string
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// OpenAICompatibleModel adapts OpenAI-compatible chat/completions APIs to the
// repository's minimal LLM interface.
type OpenAICompatibleModel struct {
	name       string
	endpoint   string
	apiKey     string
	httpClient *http.Client
	timeout    time.Duration
}

// NewOpenAICompatibleModel creates an OpenAI-compatible chat model.
func NewOpenAICompatibleModel(cfg OpenAICompatibleConfig) (*OpenAICompatibleModel, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("openai-compatible model: Name is required")
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("openai-compatible model: BaseURL is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai-compatible model: APIKey is required")
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultOpenAICompatibleTimeout
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &OpenAICompatibleModel{
		name:       cfg.Name,
		endpoint:   chatCompletionsEndpoint(cfg.BaseURL),
		apiKey:     cfg.APIKey,
		httpClient: client,
		timeout:    timeout,
	}, nil
}

// NewDeepSeekModel creates a DeepSeek chat model using the OpenAI-compatible
// endpoint. Empty baseURL defaults to https://api.deepseek.com/v1.
func NewDeepSeekModel(apiKey, modelName, baseURL string) (*OpenAICompatibleModel, error) {
	if modelName == "" {
		modelName = "deepseek-chat"
	}
	if baseURL == "" {
		baseURL = "https://api.deepseek.com/v1"
	}
	return NewOpenAICompatibleModel(OpenAICompatibleConfig{
		Name:    modelName,
		BaseURL: baseURL,
		APIKey:  apiKey,
	})
}

func (m *OpenAICompatibleModel) Name() string { return m.name }

func (m *OpenAICompatibleModel) GenerateContent(req *LLMRequest) (*LLMResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("openai-compatible model %q: nil request", m.name)
	}
	payload := openAIChatRequest{
		Model:    m.name,
		Messages: openAIMessagesFromRequest(req),
		Tools:    openAIToolsFromDeclarations(req.ToolDeclarations),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible model %q: encode request: %w", m.name, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai-compatible model %q: build request: %w", m.name, err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible model %q: request failed: %w", m.name, err)
	}
	defer httpResp.Body.Close()
	respBody, readErr := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if readErr != nil {
		return nil, fmt.Errorf("openai-compatible model %q: read response: %w", m.name, readErr)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai-compatible model %q: status %d: %s", m.name, httpResp.StatusCode, truncateResponseBody(respBody, 4096))
	}

	var decoded openAIChatResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, fmt.Errorf("openai-compatible model %q: decode response: %w", m.name, err)
	}
	if len(decoded.Choices) == 0 {
		return nil, fmt.Errorf("openai-compatible model %q: response has no choices", m.name)
	}
	return llmResponseFromOpenAIChoice(decoded.Choices[0])
}

type openAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	Tools    []openAITool        `json:"tools,omitempty"`
}

type openAIChatMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type openAITool struct {
	Type     string                 `json:"type"`
	Function openAIFunctionToolSpec `json:"function"`
}

type openAIFunctionToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatResponse struct {
	Choices []openAIChoice `json:"choices"`
}

type openAIChoice struct {
	Message      openAIChatMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

func chatCompletionsEndpoint(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

func openAIMessagesFromRequest(req *LLMRequest) []openAIChatMessage {
	messages := make([]openAIChatMessage, 0, len(req.Contents)+1)
	if strings.TrimSpace(req.SystemInstruction) != "" {
		messages = append(messages, openAIChatMessage{
			Role:    "system",
			Content: req.SystemInstruction,
		})
	}
	for _, content := range req.Contents {
		messages = append(messages, openAIMessagesFromContent(content)...)
	}
	return messages
}

func openAIMessagesFromContent(content LLMContent) []openAIChatMessage {
	role := openAIRole(content.Role)
	if role == "tool" {
		return openAIToolMessagesFromContent(content)
	}

	msg := openAIChatMessage{Role: role}
	for _, part := range content.Parts {
		if part.Text != "" {
			if msg.Content != "" {
				msg.Content += "\n"
			}
			msg.Content += part.Text
		}
		if part.FunctionCall != nil {
			msg.ToolCalls = append(msg.ToolCalls, openAIToolCall{
				ID:   part.FunctionCall.ID,
				Type: "function",
				Function: openAIToolFunction{
					Name:      part.FunctionCall.Name,
					Arguments: mustJSON(part.FunctionCall.Args),
				},
			})
		}
	}
	if msg.Content == "" && len(msg.ToolCalls) == 0 {
		return nil
	}
	return []openAIChatMessage{msg}
}

func openAIToolMessagesFromContent(content LLMContent) []openAIChatMessage {
	var messages []openAIChatMessage
	var fallbackText string
	for _, part := range content.Parts {
		if part.Text != "" {
			if fallbackText != "" {
				fallbackText += "\n"
			}
			fallbackText += part.Text
		}
		if part.FunctionResponse == nil {
			continue
		}
		messages = append(messages, openAIChatMessage{
			Role:       "tool",
			ToolCallID: part.FunctionResponse.ID,
			Name:       part.FunctionResponse.Name,
			Content:    mustJSON(functionResponsePayload(part.FunctionResponse)),
		})
	}
	if len(messages) == 0 && fallbackText != "" {
		messages = append(messages, openAIChatMessage{Role: "tool", Content: fallbackText})
	}
	return messages
}

func openAIRole(role string) string {
	switch role {
	case "", "model":
		return "assistant"
	default:
		return role
	}
}

func functionResponsePayload(fr *event.FunctionResponse) map[string]any {
	payload := map[string]any{"result": fr.Result}
	if fr.Error != "" {
		payload["error"] = fr.Error
	}
	return payload
}

func openAIToolsFromDeclarations(decls []any) []openAITool {
	tools := make([]openAITool, 0, len(decls))
	for _, raw := range decls {
		name, description, parameters := unpackToolDeclaration(raw)
		if name == "" {
			continue
		}
		if parameters == nil {
			parameters = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		tools = append(tools, openAITool{
			Type: "function",
			Function: openAIFunctionToolSpec{
				Name:        name,
				Description: description,
				Parameters:  parameters,
			},
		})
	}
	return tools
}

func unpackToolDeclaration(raw any) (name string, description string, inputSchema map[string]any) {
	switch typed := raw.(type) {
	case nil:
		return "", "", nil
	case map[string]any:
		name = firstString(typed, "name", "Name")
		description = firstString(typed, "description", "Description")
		inputSchema = firstMap(typed, "inputSchema", "InputSchema", "parameters", "Parameters")
		return name, description, inputSchema
	default:
		v := reflect.ValueOf(raw)
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return "", "", nil
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return "", "", nil
		}
		name = stringField(v, "Name")
		description = stringField(v, "Description")
		inputSchema = mapField(v, "InputSchema")
		return name, description, inputSchema
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok {
			return v
		}
	}
	return ""
}

func firstMap(m map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if v, ok := m[key].(map[string]any); ok {
			return v
		}
	}
	return nil
}

func stringField(v reflect.Value, name string) string {
	f := v.FieldByName(name)
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}

func mapField(v reflect.Value, name string) map[string]any {
	f := v.FieldByName(name)
	if !f.IsValid() || f.IsNil() {
		return nil
	}
	if m, ok := f.Interface().(map[string]any); ok {
		return m
	}
	return nil
}

func llmResponseFromOpenAIChoice(choice openAIChoice) (*LLMResponse, error) {
	content := &LLMContent{Role: "model"}
	if choice.Message.Content != "" {
		content.Parts = append(content.Parts, LLMPart{Text: choice.Message.Content})
	}
	for _, toolCall := range choice.Message.ToolCalls {
		args := map[string]any{}
		if strings.TrimSpace(toolCall.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
				return nil, fmt.Errorf("decode tool call %q arguments: %w", toolCall.Function.Name, err)
			}
		}
		id := toolCall.ID
		if id == "" {
			id = "call_" + toolCall.Function.Name
		}
		content.Parts = append(content.Parts, LLMPart{
			FunctionCall: &event.FunctionCall{
				ID:   id,
				Name: toolCall.Function.Name,
				Args: args,
			},
		})
	}
	return &LLMResponse{
		Content:      content,
		TurnComplete: choice.FinishReason != "length",
	}, nil
}

func mustJSON(v any) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":"json encode failed: %s"}`, err.Error())
	}
	return string(b)
}

func truncateResponseBody(body []byte, max int) string {
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "...(truncated)"
}
