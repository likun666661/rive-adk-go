// Command demo demonstrates the complete Runner → Agent → Flow → Event → Session
// chain using a fake model and a simple weather tool.
//
// Chapter 01 — runtime flow:
//
//	User: "What's the weather in Tokyo?"
//	  → Model: function call (get_weather)
//	  → Tool:  returns weather data
//	  → Model: final text response
//	  → Session persists user, model, tool, and final events
//
// Chapter 02 — state lifecycle:
//
//	Two sessions for the same user demonstrate:
//	  - app: state shared across all users and sessions
//	  - user: state shared across a user's sessions
//	  - session state (no prefix) isolated to one session
//	  - temp: state visible during invocation, trimmed on persist
//	  - artifact save/load with versioning
//	  - memory add/search across sessions
package main

import (
	stdctx "context"
	"fmt"
	"os"

	"github.com/likun666661/rive-adk-go/artifact"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/flow"
	"github.com/likun666661/rive-adk-go/llmagent"
	"github.com/likun666661/rive-adk-go/memory"
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

	if code := runChapter01(); code != 0 {
		return code
	}
	fmt.Println()

	if code := runChapter02(); code != 0 {
		return code
	}

	fmt.Println()
	fmt.Println("=== All demos complete ===")
	return 0
}

// ---------------------------------------------------------------------------
// Chapter 01 — runtime flow (weather demo)
// ---------------------------------------------------------------------------

func runChapter01() int {
	fmt.Println("--- Chapter 01: Runtime Flow ---")
	fmt.Println()

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

	f := &flow.Flow{
		Model: fakeModel,
		Tools: map[string]tool.FunctionTool{
			"get_weather": weatherTool,
		},
	}

	ag, err := llmagent.New("weather_bot", "A bot that answers weather questions.", f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent: %v\n", err)
		return 1
	}

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

	fmt.Println("[User] What's the weather in Tokyo?")
	sess, events, err := r.Run(stdctx.Background(), "user-1", "sess-1", "What's the weather in Tokyo?")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
		return 1
	}

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

	return 0
}

// ---------------------------------------------------------------------------
// Chapter 02 — state lifecycle
// ---------------------------------------------------------------------------

func runChapter02() int {
	fmt.Println("--- Chapter 02: State Lifecycle ---")
	fmt.Println()

	ctx := stdctx.Background()

	// ---- Services ----
	sessionSvc := runner.NewInMemorySessionService()
	memorySvc := memory.InMemoryService()
	artifactSvc := artifact.InMemoryService()

	// ---- Tools ----
	// state_setter accepts a state_delta and returns it, so the flow
	// merges it into session state with proper scope routing.
	stateSetter := tool.NewFunctionTool("state_setter", "Set state with scope-prefixed keys",
		func(args map[string]any) (map[string]any, error) {
			delta, _ := args["delta"].(map[string]any)
			if delta == nil {
				delta = map[string]any{}
			}
			// Return the delta via state_delta so the flow merges it.
			return map[string]any{
				"status":      "ok",
				"state_delta": delta,
			}, nil
		},
	)

	// read_state returns current session state for a given key.
	readState := tool.NewFunctionTool("read_state", "Read a key from session state",
		func(args map[string]any) (map[string]any, error) {
			key, _ := args["key"].(string)
			return map[string]any{
				"key":   key,
				"found": false,
			}, nil
		},
	)

	// ---- Model ----
	fakeModel := model.NewFakeModel(
		"state-model",
		model.FunctionCallResponse("Setting app config and user theme.",
			event.FunctionCall{
				ID:   "fc-app",
				Name: "state_setter",
				Args: map[string]any{"delta": map[string]any{
					"app:env":    "production",
					"app:region": "us-east-1",
					"user:theme": "dark",
					"user:lang":  "en",
					"topic":      "state-lifecycle",
				}},
			},
		),
		model.TextResponse("App and user state configured for this session."),
		model.FunctionCallResponse("Reading state back.",
			event.FunctionCall{
				ID:   "fc-read",
				Name: "read_state",
				Args: map[string]any{"key": "topic"},
			},
		),
		model.TextResponse("State read confirmed."),
	)

	f := &flow.Flow{
		Model: fakeModel,
		Tools: map[string]tool.FunctionTool{
			"state_setter": stateSetter,
			"read_state":   readState,
		},
	}

	ag, err := llmagent.New("state_bot", "A bot that manages state lifecycle.", f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent: %v\n", err)
		return 1
	}

	execAgent := ag.(runner.ExecutableAgent)
	r, err := runner.New(runner.Config{
		AppName:         "state_demo",
		Agent:           execAgent,
		SessionService:  sessionSvc,
		MemoryService:   memorySvc,
		ArtifactService: artifactSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	// ==========================================
	// Scenario 1: Set state from session 1
	// ==========================================
	fmt.Println("[Scenario 1] Session 'user-a/sess-1' sets app:, user:, and session state via event actions.")
	sess1, _, err := r.Run(ctx, "user-a", "sess-1", "Set app:env=production, user:theme=dark, topic=state-lifecycle")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run 1 error: %v\n", err)
		return 1
	}

	// Check sess-1 local state.
	if v, ok := sess1.State().Get("topic"); ok {
		fmt.Printf("  session state 'topic' = %v\n", v)
	}
	if v, ok := sess1.State().Get("app:env"); ok {
		fmt.Printf("  session raw 'app:env' = %v (routed to app store)\n", v)
	}

	// Get merged state (full overlay: app + user + session).
	merged1, _ := sessionSvc.GetMergedState("state_demo", "user-a", "sess-1")

	fmt.Printf("  merged: app:env=%v, app:region=%v, user:theme=%v, user:lang=%v, topic=%v\n",
		merged1["app:env"], merged1["app:region"], merged1["user:theme"], merged1["user:lang"], merged1["topic"])

	// ==========================================
	// Scenario 2: Same user, different session
	// ==========================================
	fmt.Println()
	fmt.Println("[Scenario 2] Session 'user-a/sess-2' (same user) — sees app: and user: state but not session state.")

	// Second model for sess-2 with different tool calls.
	fakeModel2 := model.NewFakeModel(
		"state-model-2",
		model.FunctionCallResponse("Setting additional user state from sess-2.",
			event.FunctionCall{
				ID:   "fc-new",
				Name: "state_setter",
				Args: map[string]any{"delta": map[string]any{
					"user:font_size": "14",
					"temp:scratch":   "in-progress",
					"new_topic":      "from-sess-2",
				}},
			},
		),
		model.TextResponse("Session 2 state set."),
	)

	f2 := &flow.Flow{
		Model: fakeModel2,
		Tools: map[string]tool.FunctionTool{
			"state_setter": stateSetter,
			"read_state":   readState,
		},
	}

	ag2, _ := llmagent.New("state_bot", "State bot for sess-2.", f2)
	execAgent2 := ag2.(runner.ExecutableAgent)
	r2, err := runner.New(runner.Config{
		AppName:         "state_demo",
		Agent:           execAgent2,
		SessionService:  sessionSvc,
		MemoryService:   memorySvc,
		ArtifactService: artifactSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner 2: %v\n", err)
		return 1
	}

	sess2, _, _ := r2.Run(ctx, "user-a", "sess-2", "Set more state from different session")
	merged2, _ := sessionSvc.GetMergedState("state_demo", "user-a", "sess-2")

	fmt.Printf("  merged: app:env=%v, app:region=%v, user:theme=%v, user:lang=%v, user:font_size=%v\n",
		merged2["app:env"], merged2["app:region"], merged2["user:theme"], merged2["user:lang"], merged2["user:font_size"])
	fmt.Printf("  session local 'new_topic' = %v (only in sess-2)\n", merged2["new_topic"])
	fmt.Printf("  'topic' from sess-1 NOT in sess-2: %v\n", merged2["topic"])

	// temp state should NOT appear in merged view.
	if _, ok := merged2["temp:scratch"]; ok {
		fmt.Println("  WARNING: temp:scratch leaked into merged state!")
	} else {
		fmt.Println("  temp:scratch correctly absent from merged state (trimmed on persist)")
	}

	// temp state should also be cleaned from durable session state after invocation.
	if _, ok := sess2.State().Get("temp:scratch"); ok {
		fmt.Println("  WARNING: temp:scratch leaked into durable session state!")
	} else {
		fmt.Println("  temp:scratch correctly absent from durable session state (cleaned after persist)")
	}

	// ==========================================
	// Scenario 3: Artifact save and load
	// ==========================================
	fmt.Println()
	fmt.Println("[Scenario 3] Save and load an artifact.")

	saveResp, err := artifactSvc.Save(ctx, &artifact.SaveRequest{
		AppName:   "state_demo",
		UserID:    "user-a",
		SessionID: "sess-1",
		FileName:  "report.txt",
		Part:      &artifact.ArtifactPart{Text: "Session 1 state report: topic=state-lifecycle, env=production"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Artifact Save error: %v\n", err)
		return 1
	}
	fmt.Printf("  Saved artifact version %d\n", saveResp.Version)

	// Save again — version increments
	saveResp2, _ := artifactSvc.Save(ctx, &artifact.SaveRequest{
		AppName:   "state_demo",
		UserID:    "user-a",
		SessionID: "sess-1",
		FileName:  "report.txt",
		Part:      &artifact.ArtifactPart{Text: "Session 1 state report v2: updated report"},
	})
	fmt.Printf("  Saved artifact version %d (incremented)\n", saveResp2.Version)

	// Load latest
	loadResp, _ := artifactSvc.Load(ctx, &artifact.LoadRequest{
		AppName:   "state_demo",
		UserID:    "user-a",
		SessionID: "sess-1",
		FileName:  "report.txt",
	})
	fmt.Printf("  Loaded latest: %q\n", loadResp.Part.Text)

	// Load specific version
	loadV1, _ := artifactSvc.Load(ctx, &artifact.LoadRequest{
		AppName:   "state_demo",
		UserID:    "user-a",
		SessionID: "sess-1",
		FileName:  "report.txt",
		Version:   1,
	})
	fmt.Printf("  Loaded version 1: %q\n", loadV1.Part.Text)

	// List artifacts for the session
	listResp, _ := artifactSvc.List(ctx, &artifact.ListRequest{
		AppName:   "state_demo",
		UserID:    "user-a",
		SessionID: "sess-1",
	})
	fmt.Printf("  Artifact files in session: %v\n", listResp.FileNames)

	// ==========================================
	// Scenario 4: Memory add and search
	// ==========================================
	fmt.Println()
	fmt.Println("[Scenario 4] Add session to memory and search across sessions.")

	// Add both sessions to memory.
	if err := memorySvc.AddSessionToMemory(ctx, sess1); err != nil {
		fmt.Fprintf(os.Stderr, "AddSessionToMemory(sess1): %v\n", err)
		return 1
	}
	if err := memorySvc.AddSessionToMemory(ctx, sess2); err != nil {
		fmt.Fprintf(os.Stderr, "AddSessionToMemory(sess2): %v\n", err)
		return 1
	}

	// Search for "state" across all sessions.
	searchResp, _ := memorySvc.SearchMemory(ctx, &memory.SearchRequest{
		AppName: "state_demo",
		UserID:  "user-a",
		Query:   "state config",
	})
	fmt.Printf("  Found %d memory entries matching 'state config':\n", len(searchResp.Memories))
	for _, m := range searchResp.Memories {
		fmt.Printf("    [%s] author=%s\n", m.ID, m.Author)
		if m.Content != nil && len(m.Content.Parts) > 0 {
			fmt.Printf("         text: %s\n", truncate(m.Content.Parts[0].Text, 80))
		}
	}

	// Search for something not in memory
	emptyResp, _ := memorySvc.SearchMemory(ctx, &memory.SearchRequest{
		AppName: "state_demo",
		UserID:  "user-a",
		Query:   "nonexistent_keyword",
	})
	fmt.Printf("  Search for 'nonexistent_keyword': %d results\n", len(emptyResp.Memories))

	// Confirm memory is scoped to user
	otherUserResp, _ := memorySvc.SearchMemory(ctx, &memory.SearchRequest{
		AppName: "state_demo",
		UserID:  "user-b",
		Query:   "state",
	})
	fmt.Printf("  Search for 'state' as user-b: %d results (should be 0 — user-scoped)\n", len(otherUserResp.Memories))

	return 0
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
