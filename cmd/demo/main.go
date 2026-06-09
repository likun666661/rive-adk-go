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
	"strings"

	"github.com/likun666661/rive-adk-go/artifact"
	invctx "github.com/likun666661/rive-adk-go/context"
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

	if code := runChapter03(); code != 0 {
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

// ---------------------------------------------------------------------------
// Chapter 03 — tool system integration
// ---------------------------------------------------------------------------

func runChapter03() int {
	fmt.Println("--- Chapter 03: Tool System Integration ---")
	fmt.Println()

	if code := demoFilteredTools(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoConfirmedToolCall(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoRejectedConfirmation(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoStreamingToolNonLive(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoLongRunningTool(); code != 0 {
		return code
	}

	return 0
}

// ---------------------------------------------------------------------------
// Demo 3.1 — Allowed tool filtering via FilterToolset
// ---------------------------------------------------------------------------

func demoFilteredTools() int {
	fmt.Println("[Demo 3.1] Allowed Tool Filtering via FilterToolset")

	allowedTool := tool.NewFunctionToolWithDeclaration("get_weather", "Get weather",
		tool.NewDeclaration("get_weather", "Get weather for a city",
			map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}},
			nil,
		),
		func(args map[string]any) (map[string]any, error) {
			city, _ := args["city"].(string)
			return map[string]any{"city": city, "temp": 22}, nil
		},
	)

	blockedTool := tool.NewFunctionToolWithDeclaration("delete_data", "Delete data",
		tool.NewDeclaration("delete_data", "Delete all user data",
			map[string]any{"type": "object"},
			nil,
		),
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"deleted": true}, nil
		},
	)

	fullTs := tool.NewStaticToolset("all_tools", []tool.Tool{
		allowedTool.(tool.Tool),
		blockedTool.(tool.Tool),
	})
	filteredTs := tool.NewFilterToolset("safe_tools", fullTs,
		tool.AllowedToolsPredicate("get_weather"),
	)

	// Capture declarations to verify only get_weather is visible.
	var capturedNames []string
	seen := map[string]bool{}
	f := &flow.Flow{
		Model: model.NewFakeModel("demo-model",
			model.FunctionCallResponse("Let me check weather.",
				event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
			),
			model.TextResponse("Tokyo is 22°C."),
		),
		Tools:    map[string]tool.FunctionTool{},
		Toolsets: []tool.Toolset{filteredTs},
		BeforeModelCallbacks: []flow.BeforeModelCallback{
			func(ctx invctx.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				for _, d := range req.ToolDeclarations {
					if dec, ok := d.(tool.Declaration); ok {
						if !seen[dec.Name] {
							seen[dec.Name] = true
							capturedNames = append(capturedNames, dec.Name)
						}
					}
				}
				return nil, nil
			},
		},
	}

	ag, _ := llmagent.New("filter_bot", "A bot with filtered tools.", f)
	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "filter_app",
		Agent:          ag.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	_, _, err = r.Run(stdctx.Background(), "user-1", "sess-filter", "Weather in Tokyo?")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
		return 1
	}

	fmt.Printf("  Declarations visible to model: %v\n", capturedNames)
	fmt.Println("  => Only 'get_weather' is declared; 'delete_data' is filtered out.")
	return 0
}

// ---------------------------------------------------------------------------
// Demo 3.2 — Confirmed tool call
// ---------------------------------------------------------------------------

func demoConfirmedToolCall() int {
	fmt.Println("[Demo 3.2] Confirmed Tool Call (Approve)")

	inner := tool.NewFunctionTool("deploy_app", "Deploy the application to production",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"deployed": true, "version": args["version"]}, nil
		},
	)

	confirmedTool := tool.WithConfirmation(inner, true, nil)

	// First call — should request confirmation.
	result, err := confirmedTool.Run(map[string]any{"version": "v2.0"})
	if err == nil {
		fmt.Println("  ERROR: expected confirmation required")
		return 1
	}
	fmt.Printf("  First call: requires_confirmation=%v, hint=%v\n",
		result["confirmation_required"], result["hint"])
	fmt.Printf("  Error: %v\n", err)

	// User approves the call.
	confirmedTool.Run(map[string]any{"version": "v2.0"}) // force confirmation required
	if cc, ok := confirmedTool.(tool.ConfirmationControl); ok {
		cc.SetConfirmed(true)
	}

	// Second call — should execute.
	result, err = confirmedTool.Run(map[string]any{"version": "v2.0"})
	if err != nil {
		fmt.Printf("  ERROR: unexpected error after approval: %v\n", err)
		return 1
	}
	fmt.Printf("  After approval: deployed=%v, version=%v\n",
		result["deployed"], result["version"])

	return 0
}

// ---------------------------------------------------------------------------
// Demo 3.3 — Rejected confirmation path
// ---------------------------------------------------------------------------

func demoRejectedConfirmation() int {
	fmt.Println("[Demo 3.3] Rejected Confirmation Path")

	inner := tool.NewFunctionTool("drop_table", "Drop a database table",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"dropped": true}, nil
		},
	)

	ct := tool.WithConfirmation(inner, true, nil)

	// First call — should request confirmation.
	result, err := ct.Run(map[string]any{"table": "users"})
	if err == nil {
		fmt.Println("  ERROR: expected confirmation required")
		return 1
	}
	fmt.Printf("  First call: requires_confirmation=%v\n", result["confirmation_required"])

	// User rejects.
	if cc, ok := ct.(tool.ConfirmationControl); ok {
		cc.SetConfirmed(false)
	}

	// Second call — should be rejected.
	result, err = ct.Run(map[string]any{"table": "users"})
	if err == nil {
		fmt.Println("  ERROR: expected confirmation rejected")
		return 1
	}
	fmt.Printf("  After rejection: confirmation_rejected=%v, error=%v\n",
		result["confirmation_rejected"], result["error"])
	fmt.Println("  => Tool call was blocked by user rejection.")

	return 0
}

// ---------------------------------------------------------------------------
// Demo 3.4 — Streaming tool collected in non-live mode
// ---------------------------------------------------------------------------

func demoStreamingToolNonLive() int {
	fmt.Println("[Demo 3.4] Streaming Tool Collected in Non-Live Mode")

	st := tool.NewStreamingFunctionTool("generate_report", "Generate a report in chunks",
		func(args map[string]any) ([]tool.StreamChunk, error) {
			sections := []string{"Introduction\n", "Analysis\n", "Conclusion\n"}
			var chunks []tool.StreamChunk
			for i, s := range sections {
				chunks = append(chunks, tool.StreamChunk{
					Text:  s,
					Final: i == len(sections)-1,
				})
			}
			return chunks, nil
		},
	)

	cr := tool.ExecuteStream("fc-001", "generate_report", map[string]any{}, st)
	if cr.Error != "" {
		fmt.Printf("  ERROR: %s\n", cr.Error)
		return 1
	}

	result, _ := cr.Result["result"].(string)
	fmt.Printf("  Collected report:\n")
	fmt.Printf("  %s\n", indent(result, "  "))
	fmt.Println("  => Streaming chunks were collected into a single result in non-live mode.")

	return 0
}

// ---------------------------------------------------------------------------
// Demo 3.5 — Long-running tool metadata
// ---------------------------------------------------------------------------

func demoLongRunningTool() int {
	fmt.Println("[Demo 3.5] Long-Running Tool Metadata")

	decl := tool.NewDeclaration("train_model", "Train a machine learning model",
		map[string]any{
			"type":       "object",
			"properties": map[string]any{"dataset": map[string]any{"type": "string"}},
		},
		map[string]any{
			"type":       "object",
			"properties": map[string]any{"job_id": map[string]any{"type": "string"}, "status": map[string]any{"type": "string"}},
		},
	)

	lr := tool.NewLongRunningFunctionTool("train_model", "Train a machine learning model", decl,
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{
				"job_id":  "train-abc-123",
				"status":  "pending",
				"message": "Training job submitted. Check back later for results.",
			}, nil
		},
	)

	// Check IsLongRunning flag.
	fmt.Printf("  IsLongRunning: %v\n", lr.IsLongRunning())

	// Check the declaration annotation.
	dp := lr.(tool.DeclarationProvider)
	d := dp.Declaration()
	fmt.Printf("  Declaration description (first 100 chars):\n")
	fmt.Printf("  %s\n", indent(truncate(d.Description, 100), "  "))

	// Execute the tool — returns pending status with job_id.
	result, err := lr.Run(map[string]any{"dataset": "training_data.csv"})
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return 1
	}
	fmt.Printf("  Result: job_id=%v, status=%v\n", result["job_id"], result["status"])
	fmt.Println("  => Long-running tool returns job metadata; LLM is warned not to repeat calls.")

	return 0
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" || i < len(lines)-1 {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}
