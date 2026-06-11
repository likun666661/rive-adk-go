// Command realllm validates the replica runtime against a real
// OpenAI-compatible model backend. It is intentionally tiny: one weather tool,
// one runner invocation, and a strict check that the model calls the tool before
// producing the final answer.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	invctx "github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/flow"
	"github.com/likun666661/rive-adk-go/llmagent"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/runner"
	"github.com/likun666661/rive-adk-go/tool"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "real LLM smoke failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("DEEPSEEK_API_KEY is required")
	}
	modelName := envOrDefault("DEEPSEEK_MODEL", "deepseek-chat")
	baseURL := envOrDefault("DEEPSEEK_BASE_URL", "https://api.deepseek.com/v1")

	llm, err := model.NewDeepSeekModel(apiKey, modelName, baseURL)
	if err != nil {
		return err
	}

	weatherTool := tool.NewFunctionToolWithDeclaration(
		"get_weather",
		"Get deterministic current weather for one city.",
		tool.NewDeclaration(
			"get_weather",
			"Get deterministic current weather for one city.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{
						"type":        "string",
						"description": "City name, for example Tokyo.",
					},
				},
				"required": []any{"city"},
			},
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city":        map[string]any{"type": "string"},
					"temperature": map[string]any{"type": "number"},
					"condition":   map[string]any{"type": "string"},
				},
			},
		),
		func(args map[string]any) (map[string]any, error) {
			city, _ := args["city"].(string)
			if strings.TrimSpace(city) == "" {
				city = "Tokyo"
			}
			return map[string]any{
				"city":        city,
				"temperature": 22,
				"condition":   "sunny",
				"source":      "deterministic smoke tool",
			}, nil
		},
	)

	f := &flow.Flow{
		Model: llm,
		Tools: map[string]tool.FunctionTool{
			"get_weather": weatherTool,
		},
		RequestProcessors: []flow.RequestProcessor{
			func(ctx invctx.InvocationContext, req *model.LLMRequest) (*event.Event, error) {
				req.SystemInstruction = strings.Join([]string{
					"You are validating a Go ADK runtime replica.",
					"You must call the get_weather tool exactly once before final answer.",
					"After the tool result arrives, answer in one concise English sentence and mention the city, temperature, and condition.",
				}, " ")
				return nil, nil
			},
		},
	}

	ag, err := llmagent.New("real_llm_weather_bot", "Validates a real LLM tool-calling loop.", f)
	if err != nil {
		return err
	}
	execAgent, ok := ag.(runner.ExecutableAgent)
	if !ok {
		return fmt.Errorf("agent does not implement runner.ExecutableAgent")
	}
	r, err := runner.New(runner.Config{
		AppName:        "real_llm_smoke",
		Agent:          execAgent,
		SessionService: runner.NewInMemorySessionService(),
	})
	if err != nil {
		return err
	}

	prompt := envOrDefault("ADKGO_REAL_LLM_PROMPT", "Use get_weather to answer: what is the weather in Tokyo?")
	sess, events, err := r.Run(context.Background(), "real-user", "real-session", prompt)
	if err != nil {
		return err
	}

	var functionCalls, functionResponses int
	var finalText string
	for _, ev := range events {
		if ev == nil || ev.Content == nil {
			continue
		}
		for _, part := range ev.Content.Parts {
			if part.FunctionCall != nil {
				functionCalls++
				fmt.Printf("model_call: %s args=%v\n", part.FunctionCall.Name, part.FunctionCall.Args)
			}
			if part.FunctionResponse != nil {
				functionResponses++
				fmt.Printf("tool_result: %s result=%v\n", part.FunctionResponse.Name, part.FunctionResponse.Result)
			}
			if ev.Role == event.RoleModel && part.Text != "" && !ev.HasFunctionCalls() {
				finalText = part.Text
			}
		}
	}
	if functionCalls == 0 || functionResponses == 0 {
		return fmt.Errorf("expected at least one tool call and tool result, got calls=%d responses=%d", functionCalls, functionResponses)
	}
	if strings.TrimSpace(finalText) == "" {
		return fmt.Errorf("expected final model text after tool result")
	}

	fmt.Printf("final_answer: %s\n", finalText)
	fmt.Printf("persisted_events: %d\n", sess.EventCount())
	fmt.Println("real_llm_smoke: ok")
	return nil
}

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
