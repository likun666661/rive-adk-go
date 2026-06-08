// Command demo demonstrates the complete Runner → Agent → Flow → Event → Session
// chain using a fake model and a simple weather tool.
//
// Flow:
//
//	User: "What's the weather in Tokyo?"
//	  → Model: function call (get_weather)
//	  → Tool:  returns weather data
//	  → Model: final text response
//	  → Session persists user, model, tool, and final events
package main

import (
	stdctx "context"
	"fmt"
	"os"

	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/flow"
	"github.com/likun666661/rive-adk-go/llmagent"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/runner"
	"github.com/likun666661/rive-adk-go/tool"
)

func main() {
	os.Exit(run())
}

func run() int {
	fmt.Println("=== ADK Go Runtime Demo ===")
	fmt.Println()

	// 1. Create a weather tool.
	weatherTool := tool.NewFunctionTool("get_weather", "Get current weather for a city",
		func(args map[string]any) (map[string]any, error) {
			city, _ := args["city"].(string)
			return map[string]any{
				"city":        city,
				"temperature": 22,
				"condition":   "sunny",
				"humidity":    "45%",
			}, nil
		},
	)

	// 2. Create a fake model with a two-step response queue:
	//    Step 1: function call for weather
	//    Step 2: final text response
	fakeModel := model.NewFakeModel("demo-model",
		model.FunctionCallResponse("Let me check the weather.",
			event.FunctionCall{
				ID:   "fc-1",
				Name: "get_weather",
				Args: map[string]any{"city": "Tokyo"},
			},
		),
		model.TextResponse("The weather in Tokyo is 22°C and sunny with 45% humidity."),
	)

	// 3. Wire up the Flow.
	f := &flow.Flow{
		Model: fakeModel,
		Tools: map[string]tool.FunctionTool{
			"get_weather": weatherTool,
		},
	}

	// 4. Create the LLM agent.
	ag, err := llmagent.New("weather_bot", "A bot that answers weather questions.", f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent: %v\n", err)
		return 1
	}

	// 5. Create a session service and runner.
	sessionSvc := runner.NewInMemorySessionService()
	execAgent := ag.(runner.ExecutableAgent)

	r, err := runner.New(runner.Config{
		AppName:        "weather_app",
		Agent:          execAgent,
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	// 6. Run the user message.
	fmt.Println("[User] What's the weather in Tokyo?")
	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-1", "What's the weather in Tokyo?")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
		return 1
	}

	// 7. Display results.
	fmt.Printf("\nProduced %d events:\n", len(events))
	for i, ev := range events {
		fmt.Printf("  [%d] ID=%s Author=%s Role=%s Partial=%v\n", i+1, ev.ID, ev.Author, ev.Role, ev.Partial)
		if ev.Content != nil {
			for j, p := range ev.Content.Parts {
				if p.Text != "" {
					fmt.Printf("       Part[%d] Text: %s\n", j, p.Text)
				}
				if p.FunctionCall != nil {
					fmt.Printf("       Part[%d] FunctionCall: %s(%v)\n", j, p.FunctionCall.Name, p.FunctionCall.Args)
				}
				if p.FunctionResponse != nil {
					fmt.Printf("       Part[%d] FunctionResponse: %s => %v\n", j, p.FunctionResponse.Name, p.FunctionResponse.Result)
				}
			}
		}
	}

	// 8. Verify session persistence.
	fmt.Printf("\nSession %q has %d persisted events:\n", sess.ID(), sess.EventCount())
	for i, ev := range sess.Events() {
		role := ev.Role
		summary := ""
		if ev.Content != nil && len(ev.Content.Parts) > 0 {
			p := ev.Content.Parts[0]
			switch {
			case p.Text != "":
				summary = p.Text
			case p.FunctionCall != nil:
				summary = fmt.Sprintf("call %s", p.FunctionCall.Name)
			case p.FunctionResponse != nil:
				summary = fmt.Sprintf("result from %s", p.FunctionResponse.Name)
			}
		}
		fmt.Printf("  [%d] %s | %s\n", i+1, role, summary)
	}

	fmt.Println()
	fmt.Println("=== Demo complete ===")
	return 0
}
