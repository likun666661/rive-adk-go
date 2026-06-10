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
//
// Chapter 05 — multi-agent composition:
//
//	Sequential, parallel, and loop workflows; AgentTool delegation;
//	and remote A2A streaming aggregation.
//
// Chapter 06 — entrypoint, deploy, and telemetry:
//
//	Launcher console/web routing, REST JSON/SSE protocols,
//	dry-run deploy plans (Cloud Run + Agent Engine),
//	and telemetry capture around a runner invocation.
//
// Chapter 07 — configurable agent flow:
//
//	ReAct function-call loop, agent transfer with transfer_to_agent,
//	ExitLoop/Reflection/HiddenArg policy extensions, and configurable
//	agent tree construction from a JSON config file.
package main

import (
	stdctx "context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/likun666661/rive-adk-go/agent"
	"github.com/likun666661/rive-adk-go/agent/agentconfig"
	"github.com/likun666661/rive-adk-go/agent/remoteagent"
	"github.com/likun666661/rive-adk-go/artifact"
	"github.com/likun666661/rive-adk-go/callbackctx"
	invctx "github.com/likun666661/rive-adk-go/context"
	"github.com/likun666661/rive-adk-go/deploy"
	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/flow"
	"github.com/likun666661/rive-adk-go/llmagent"
	"github.com/likun666661/rive-adk-go/memory"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/plugin"
	"github.com/likun666661/rive-adk-go/plugin/functionmodifier"
	"github.com/likun666661/rive-adk-go/plugin/retryreflect"
	"github.com/likun666661/rive-adk-go/runner"
	"github.com/likun666661/rive-adk-go/telemetry"
	"github.com/likun666661/rive-adk-go/tool"
	"github.com/likun666661/rive-adk-go/tool/agenttool"
	"github.com/likun666661/rive-adk-go/tool/exitloop"
	"github.com/likun666661/rive-adk-go/workflow"
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

	if code := runChapter04(); code != 0 {
		return code
	}
	fmt.Println()

	if code := runChapter05(); code != 0 {
		return code
	}
	fmt.Println()

	if code := runChapter06(); code != 0 {
		return code
	}
	fmt.Println()

	if code := runChapter07(); code != 0 {
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

// ---------------------------------------------------------------------------
// Chapter 04 — callback / plugin / instruction integration
// ---------------------------------------------------------------------------

func runChapter04() int {
	fmt.Println("--- Chapter 04: Callback / Plugin / Instruction Integration ---")
	fmt.Println()

	if code := demoPluginLogging(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoBeforeModelCache(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoInstructionInterpolation(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoPluginOrdering(); code != 0 {
		return code
	}

	return 0
}

// ---------------------------------------------------------------------------
// Demo 4.1 — Plugin logging / observability
// ---------------------------------------------------------------------------

func demoPluginLogging() int {
	fmt.Println("[Demo 4.1] Plugin Logging / Observability")

	var logLines []string
	logPrefix := func(stage string) func(format string, args ...any) {
		return func(format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			logLines = append(logLines, fmt.Sprintf("[%s] %s", stage, msg))
		}
	}

	logBeforeModel := logPrefix("before-model")
	logAfterModel := logPrefix("after-model")
	logBeforeTool := logPrefix("before-tool")
	logAfterTool := logPrefix("after-tool")

	mgr := plugin.NewManager()
	mgr.Register(plugin.New(plugin.Config{
		Name: "logging-plugin",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			logBeforeModel("model=%q instructions=%q", req.Model, truncate(req.SystemInstruction, 50))
			return nil, nil
		},
		AfterModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
			if resp != nil && resp.Content != nil && len(resp.Content.Parts) > 0 {
				logAfterModel("response_text=%q", truncate(resp.Content.Parts[0].Text, 50))
			}
			return nil, nil
		},
		BeforeTool: func(ctx callbackctx.ToolContext, toolName string, args map[string]any) (map[string]any, error) {
			logBeforeTool("tool=%q args=%v", toolName, args)
			return nil, nil
		},
		AfterTool: func(ctx callbackctx.ToolContext, toolName string, args, result map[string]any, runErr error) (map[string]any, error) {
			if runErr != nil {
				logAfterTool("tool=%q error=%v", toolName, runErr)
			} else {
				logAfterTool("tool=%q result=%v", toolName, result)
			}
			return nil, nil
		},
	}))

	echoTool := tool.NewFunctionTool("echo", "Echo back the message",
		func(args map[string]any) (map[string]any, error) {
			msg, _ := args["msg"].(string)
			return map[string]any{"echo": msg}, nil
		},
	)

	f := &flow.Flow{
		Model: model.NewFakeModel("demo-model",
			model.FunctionCallResponse("Let me echo that.",
				event.FunctionCall{ID: "fc-echo", Name: "echo", Args: map[string]any{"msg": "hello world"}},
			),
			model.TextResponse("I've echoed your message."),
		),
		Tools: map[string]tool.FunctionTool{
			"echo": echoTool,
		},
		PluginManager: mgr,
	}

	ag, err := llmagent.New("log_demo_agent", "A bot with logging plugin.", f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent: %v\n", err)
		return 1
	}

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "log_app",
		Agent:          ag.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	_, _, err = r.Run(stdctx.Background(), "user-1", "sess-log", "Echo 'hello world'")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
		return 1
	}

	for _, line := range logLines {
		fmt.Printf("  %s\n", line)
	}
	fmt.Printf("  => Logging plugin captured %d events (pure observer, no control flow change).\n", len(logLines))
	return 0
}

// ---------------------------------------------------------------------------
// Demo 4.2 — Before-model cache / mock response early-exit
// ---------------------------------------------------------------------------

func demoBeforeModelCache() int {
	fmt.Println("[Demo 4.2] Before-Model Cache / Mock Response Early-Exit")

	cache := map[string]string{
		"What's the weather?": "The weather is sunny and 22°C (cached).",
	}

	mgr := plugin.NewManager()
	mgr.Register(plugin.New(plugin.Config{
		Name: "cache-plugin",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			userMsg := ctx.UserContent()
			if cached, ok := cache[userMsg]; ok {
				return model.TextResponse(cached), nil
			}
			return nil, nil
		},
	}))

	weatherTool := tool.NewFunctionTool("get_weather", "Get weather",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"temp": 22, "condition": "sunny"}, nil
		},
	)

	f := &flow.Flow{
		Model: model.NewFakeModel("demo-model",
			model.FunctionCallResponse("Let me check weather.",
				event.FunctionCall{ID: "fc1", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
			),
			model.TextResponse("Tokyo is 22°C and sunny."),
		),
		Tools: map[string]tool.FunctionTool{
			"get_weather": weatherTool,
		},
		PluginManager: mgr,
	}

	ag, err := llmagent.New("cache_agent", "A bot with cache plugin.", f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent: %v\n", err)
		return 1
	}

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "cache_app",
		Agent:          ag.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-cache", "What's the weather?")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
		return 1
	}

	fmt.Printf("  Events produced: %d\n", len(events))
	for _, ev := range events {
		if ev.Content != nil {
			for _, p := range ev.Content.Parts {
				if p.Text != "" {
					fmt.Printf("  Model response: %q\n", p.Text)
				}
			}
		}
	}
	fmt.Println("  => Cache plugin returned a mock response before LLM was called (early exit).")
	return 0
}

// ---------------------------------------------------------------------------
// Demo 4.3 — Callback state mutation and instruction interpolation
// ---------------------------------------------------------------------------

func demoInstructionInterpolation() int {
	fmt.Println("[Demo 4.3] Callback State Mutation & Instruction Interpolation")

	var capturedInstruction string
	var capturedState map[string]any

	f := &flow.Flow{
		Model: model.NewFakeModel("demo-model",
			model.TextResponse("Hello Alice! I'll help you with data analysis as an admin."),
		),
		// A request processor that builds a system instruction from session state.
		RequestProcessors: []flow.RequestProcessor{
			func(ic invctx.InvocationContext, req *model.LLMRequest) (*event.Event, error) {
				name := ""
				role := ""
				task := ""

				if v, ok := ic.Session().State().Get("user_name"); ok {
					name = fmt.Sprintf("%v", v)
				}
				if v, ok := ic.Session().State().Get("user_role"); ok {
					role = fmt.Sprintf("%v", v)
				}
				if v, ok := ic.Session().State().Get("current_task"); ok {
					task = fmt.Sprintf("%v", v)
				}

				req.SystemInstruction = fmt.Sprintf(
					"You are assisting %s. Their role is %s. Current task: %s.",
					name, role, task,
				)
				capturedInstruction = req.SystemInstruction
				capturedState = ic.Session().State().All()
				return nil, nil
			},
		},
	}

	ag, err := llmagent.New("instruction_agent", "A bot with instruction interpolation.", f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent: %v\n", err)
		return 1
	}

	sessionSvc := runner.NewInMemorySessionService()
	_, err = runner.New(runner.Config{
		AppName:        "instr_app",
		Agent:          ag.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	// Pre-populate session state before running (simulating plugin/callback state mutation).
	sess, err := sessionSvc.Create(stdctx.Background(), "instr_app", "user-1", "sess-instr")
	if err != nil {
		sess, _ = sessionSvc.Get(stdctx.Background(), "instr_app", "user-1", "sess-instr")
	}
	sess.State().Set("user_name", "Alice")
	sess.State().Set("user_role", "admin")
	sess.State().Set("current_task", "data analysis")

	// Use manual Run to show the captured instruction.
	nextOrdinal := sess.EventCount() + 1
	invocationID := fmt.Sprintf("%s-inv-%d", sess.ID(), nextOrdinal)
	userEvent := event.NewEvent(
		fmt.Sprintf("%s-user-%d", sess.ID(), nextOrdinal),
		"user",
		event.RoleUser,
	)
	userEvent.Branch = ag.Name()
	userEvent.Content = &event.Content{
		Role: event.RoleUser,
		Parts: []event.Part{
			{Text: "Help me with data analysis"},
		},
	}
	sess.AppendEvent(userEvent)

	ic := invctx.NewInvocationContext(invctx.Params{
		Ctx:          stdctx.Background(),
		Agent:        ag,
		Session:      sess,
		InvocationID: invocationID,
		Branch:       ag.Name(),
		UserContent:  "Help me with data analysis",
	})

	events, err := ag.(runner.ExecutableAgent).Execute(ic)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Execute error: %v\n", err)
		return 1
	}

	fmt.Printf("  Session state keys: ")
	for k := range capturedState {
		fmt.Printf("%s ", k)
	}
	fmt.Println()
	fmt.Printf("  Captured system instruction: %q\n", capturedInstruction)

	for _, ev := range events {
		if ev.Content != nil {
			for _, p := range ev.Content.Parts {
				if p.Text != "" {
					fmt.Printf("  Model response: %q\n", p.Text)
				}
			}
		}
	}
	fmt.Println("  => Instruction was interpolated from session state before LLM call.")
	return 0
}

// ---------------------------------------------------------------------------
// Demo 4.4 — Plugin ordering relative to direct callbacks
// ---------------------------------------------------------------------------

func demoPluginOrdering() int {
	fmt.Println("[Demo 4.4] Plugin Ordering Relative to Direct Callbacks")

	var executionOrder []string
	record := func(name string) {
		executionOrder = append(executionOrder, name)
	}

	mgr := plugin.NewManager()
	mgr.Register(plugin.New(plugin.Config{
		Name: "plugin-a",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			record("plugin-a:beforeModel")
			return nil, nil
		},
		AfterModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
			record("plugin-a:afterModel")
			return nil, nil
		},
	}))
	mgr.Register(plugin.New(plugin.Config{
		Name: "plugin-b",
		BeforeModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			record("plugin-b:beforeModel")
			return nil, nil
		},
		AfterModel: func(ctx callbackctx.CallbackContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
			record("plugin-b:afterModel")
			return nil, nil
		},
	}))

	f := &flow.Flow{
		Model: model.NewFakeModel("demo-model",
			model.TextResponse("Ordering confirmed."),
		),
		BeforeModelCallbacks: []flow.BeforeModelCallback{
			func(ctx invctx.InvocationContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				record("direct:beforeModel-1")
				return nil, nil
			},
		},
		AfterModelCallbacks: []flow.AfterModelCallback{
			func(ctx invctx.InvocationContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
				record("direct:afterModel-1")
				return nil, nil
			},
		},
		PluginManager: mgr,
	}

	ag, err := llmagent.New("order_agent", "A bot demonstrating hook ordering.", f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent: %v\n", err)
		return 1
	}

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "order_app",
		Agent:          ag.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	_, _, err = r.Run(stdctx.Background(), "user-1", "sess-order", "Show ordering")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
		return 1
	}

	fmt.Println("  Execution order (before model):")
	for _, step := range executionOrder {
		fmt.Printf("    %s\n", step)
	}
	fmt.Println("  => Plugins always run before direct callbacks (Chapter 04 teaching model).")
	return 0
}

// ---------------------------------------------------------------------------
// Chapter 07 — configurable agent flow
// ---------------------------------------------------------------------------

func runChapter07() int {
	fmt.Println("--- Chapter 07: Configurable Agent Flow ---")
	fmt.Println()

	if code := demoReActLoop(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoAgentTransfer(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoPolicyExtensions(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoConfigurableConstruction(); code != 0 {
		return code
	}

	return 0
}

// ---------------------------------------------------------------------------
// Demo 7.1 — ReAct function-call loop
// ---------------------------------------------------------------------------

func demoReActLoop() int {
	fmt.Println("[Demo 7.1] ReAct Function-Call Loop")

	weatherTool := tool.NewFunctionTool("get_weather", "Get current weather for a city",
		func(args map[string]any) (map[string]any, error) {
			city, _ := args["city"].(string)
			return map[string]any{
				"city":        city,
				"temperature": 22,
				"condition":   "sunny",
			}, nil
		},
	)

	model := model.NewFakeModel("react-demo",
		model.FunctionCallResponse("Let me check the weather.",
			event.FunctionCall{ID: "fc-1", Name: "get_weather", Args: map[string]any{"city": "Tokyo"}},
		),
		model.TextResponse("The weather in Tokyo is 22°C and sunny."),
	)

	f := &flow.Flow{
		Model: model,
		Tools: map[string]tool.FunctionTool{"get_weather": weatherTool},
	}

	ag, err := llmagent.New("weather_bot", "Answers weather questions.", f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Failed to create agent: %v\n", err)
		return 1
	}

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "react_demo",
		Agent:          ag.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Failed to create runner: %v\n", err)
		return 1
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-react", "What's the weather in Tokyo?")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Run error: %v\n", err)
		return 1
	}

	fmt.Printf("  %d events produced across ReAct loop:\n", len(events))
	for i, ev := range events {
		author := ev.Author
		var parts []string
		if ev.Content != nil {
			for _, p := range ev.Content.Parts {
				switch {
				case p.Text != "":
					parts = append(parts, fmt.Sprintf("text=%q", truncate(p.Text, 50)))
				case p.FunctionCall != nil:
					parts = append(parts, fmt.Sprintf("call %s(%v)", p.FunctionCall.Name, p.FunctionCall.Args))
				case p.FunctionResponse != nil:
					parts = append(parts, fmt.Sprintf("result %s => %v", p.FunctionResponse.Name, p.FunctionResponse.Result))
				}
			}
		}
		fmt.Printf("    [%d] %s: %s\n", i+1, author, strings.Join(parts, ", "))
	}
	fmt.Println("  => ReAct loop: user -> model (fc) -> tool (result) -> model (final).")

	return 0
}

// ---------------------------------------------------------------------------
// Demo 7.2 — Agent transfer (host agent delegates to specialist)
// ---------------------------------------------------------------------------

func demoAgentTransfer() int {
	fmt.Println("[Demo 7.2] Agent Transfer — Host to Specialist")

	specialistTool := tool.NewFunctionTool("calculator", "Perform a calculation",
		func(args map[string]any) (map[string]any, error) {
			expr, _ := args["expression"].(string)
			return map[string]any{"expression": expr, "result": "42"}, nil
		},
	)

	specialistFlow := &flow.Flow{
		Model: model.NewFakeModel("specialist",
			model.FunctionCallResponse("Let me calculate that.",
				event.FunctionCall{ID: "fc-calc", Name: "calculator", Args: map[string]any{"expression": "6*7"}},
			),
			model.TextResponse("The result of 6*7 is 42."),
		),
		Tools: map[string]tool.FunctionTool{"calculator": specialistTool},
	}

	specialist, err := llmagent.New("math_agent", "Solves math problems.", specialistFlow)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Failed to create specialist: %v\n", err)
		return 1
	}

	if err := agent.SetParent(specialist, nil); err != nil {
		fmt.Fprintf(os.Stderr, "  SetParent(specialist): %v\n", err)
		return 1
	}

	hostModel := model.NewFakeModel("host",
		model.FunctionCallResponse("Let me transfer to the math specialist.",
			event.FunctionCall{ID: "fc-tr", Name: "transfer_to_agent", Args: map[string]any{"agent_name": "math_agent"}},
		),
	)

	hostFlow := &flow.Flow{
		Model: hostModel,
	}

	host, err := llmagent.New("host_agent", "Routes queries to specialists.", hostFlow)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Failed to create host: %v\n", err)
		return 1
	}

	if err := agent.SetSubAgents(host, []agent.Agent{specialist}); err != nil {
		fmt.Fprintf(os.Stderr, "  SetSubAgents: %v\n", err)
		return 1
	}
	if err := agent.SetParent(specialist, host); err != nil {
		fmt.Fprintf(os.Stderr, "  SetParent: %v\n", err)
		return 1
	}

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "transfer_demo",
		Agent:          host.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Failed to create runner: %v\n", err)
		return 1
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-transfer", "What is 6*7?")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Run error: %v\n", err)
		return 1
	}

	fmt.Printf("  %d events produced (host -> specialist):\n", len(events))
	for i, ev := range events {
		author := ev.Author
		var parts []string
		if ev.Content != nil {
			for _, p := range ev.Content.Parts {
				switch {
				case p.Text != "":
					parts = append(parts, fmt.Sprintf("text=%q", truncate(p.Text, 50)))
				case p.FunctionCall != nil:
					parts = append(parts, fmt.Sprintf("call %s(%v)", p.FunctionCall.Name, p.FunctionCall.Args))
				case p.FunctionResponse != nil:
					parts = append(parts, fmt.Sprintf("result %s => %v", p.FunctionResponse.Name, p.FunctionResponse.Result))
				}
			}
		}
		if ev.Actions.TransferToAgent != "" {
			parts = append(parts, fmt.Sprintf("[transfer_to=%s]", ev.Actions.TransferToAgent))
		}
		fmt.Printf("    [%d] %s: %s\n", i+1, author, strings.Join(parts, ", "))
	}
	fmt.Println("  => Host agent transfers control to specialist via transfer_to_agent.")

	return 0
}

// ---------------------------------------------------------------------------
// Demo 7.3 — Policy extensions: ExitLoop, retry/reflect, hidden args
// ---------------------------------------------------------------------------

func demoPolicyExtensions() int {
	fmt.Println("[Demo 7.3] Policy Extensions — ExitLoop, Retry/Reflect, Hidden Args")
	fmt.Println()

	if code := demoExitLoop(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoRetryReflect(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoHiddenArgs(); code != 0 {
		return code
	}

	return 0
}

func demoExitLoop() int {
	fmt.Println("  [Sub-Demo 7.3a] ExitLoop — Agent signals early loop termination")

	echoTool := tool.NewFunctionTool("echo", "Echo back a message",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"echoed": args["msg"]}, nil
		},
	)

	pMgr := plugin.NewManager()
	pMgr.Register(plugin.New(plugin.Config{
		Name: "exit-loop-demo",
	}))

	exitTool := exitloop.NewExitLoopTool()

	model := model.NewFakeModel("exit-demo",
		model.FunctionCallResponse("Let me exit now.",
			event.FunctionCall{ID: "fc-exit", Name: "exit_loop", Args: map[string]any{}},
		),
	)

	f := &flow.Flow{
		Model:         model,
		Tools:         map[string]tool.FunctionTool{"echo": echoTool, "exit_loop": exitTool},
		PluginManager: pMgr,
	}

	ag, err := llmagent.New("exit_agent", "Agent that can exit early.", f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    Failed to create agent: %v\n", err)
		return 1
	}

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "exit_demo",
		Agent:          ag.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "    Failed to create runner: %v\n", err)
		return 1
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-exit", "Exit the loop")
	if err != nil {
		fmt.Fprintf(os.Stderr, "    Run error: %v\n", err)
		return 1
	}

	for _, ev := range events {
		if ev.Actions.EndInvocation {
			fmt.Println("    => ExitLoop detected: EndInvocation=true, loop terminated.")
		}
	}
	fmt.Printf("    %d events produced (exited after first step).\n", len(events))
	return 0
}

func demoRetryReflect() int {
	fmt.Println("  [Sub-Demo 7.3b] Retry/Reflect — Tool error recovery with reflection")

	alwaysFails := tool.NewFunctionTool("fail_tool", "Always fails",
		func(args map[string]any) (map[string]any, error) {
			return nil, fmt.Errorf("internal error: connection refused")
		},
	)

	pMgr := plugin.NewManager()
	rrPlugin := retryreflect.New(retryreflect.Config{
		Name:       "retry-reflect-demo",
		MaxRetries: 2,
	})
	pMgr.Register(rrPlugin.Plugin)

	model := model.NewFakeModel("reflect-demo",
		model.FunctionCallResponse("Let me try the tool.",
			event.FunctionCall{ID: "fc-fail", Name: "fail_tool", Args: map[string]any{}},
		),
		model.TextResponse("The tool failed but I reflected on the error and adjusted my approach."),
	)

	f := &flow.Flow{
		Model:         model,
		Tools:         map[string]tool.FunctionTool{"fail_tool": alwaysFails},
		PluginManager: pMgr,
	}

	ag, err := llmagent.New("reflect_agent", "Agent with retry/reflect plugin.", f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    Failed to create agent: %v\n", err)
		return 1
	}

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "reflect_demo",
		Agent:          ag.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "    Failed to create runner: %v\n", err)
		return 1
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-reflect", "Try the tool")
	if err != nil {
		fmt.Fprintf(os.Stderr, "    Run error: %v\n", err)
		return 1
	}

	for _, ev := range events {
		if ev.Content != nil {
			for _, p := range ev.Content.Parts {
				if p.FunctionResponse != nil {
					if refl, ok := p.FunctionResponse.Result["reflection"]; ok {
						fmt.Printf("    Reflection: %s\n", truncate(fmt.Sprintf("%v", refl), 80))
					}
				}
			}
		}
	}
	fmt.Printf("    %d events produced (error caught, reflection added).\n", len(events))
	return 0
}

func demoHiddenArgs() int {
	fmt.Println("  [Sub-Demo 7.3c] Hidden Args — Protect internal parameters from LLM")

	processTool := tool.NewFunctionTool("process_data", "Process user data",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"status": "processed", "user_id": args["user_id"]}, nil
		},
	)

	pMgr := plugin.NewManager()
	fmPlugin := functionmodifier.New(functionmodifier.Config{
		Name: "hidden-args-demo",
		Predicate: func(toolName string) bool {
			return toolName == "process_data"
		},
		HiddenArgs: map[string]any{
			"user_id": map[string]any{"type": "string", "description": "Internal user ID (hidden)"},
		},
	})
	pMgr.Register(fmPlugin.Plugin)

	model := model.NewFakeModel("hidden-demo",
		model.FunctionCallResponse("Processing data.",
			event.FunctionCall{ID: "fc-proc", Name: "process_data", Args: map[string]any{}},
		),
		model.TextResponse("Data processing complete."),
	)

	f := &flow.Flow{
		Model:         model,
		Tools:         map[string]tool.FunctionTool{"process_data": processTool},
		PluginManager: pMgr,
	}

	ag, err := llmagent.New("hidden_agent", "Agent with hidden args plugin.", f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    Failed to create agent: %v\n", err)
		return 1
	}

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "hidden_demo",
		Agent:          ag.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "    Failed to create runner: %v\n", err)
		return 1
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-hidden", "Process my data")
	if err != nil {
		fmt.Fprintf(os.Stderr, "    Run error: %v\n", err)
		return 1
	}

	fmt.Printf("    %d events produced (hidden args handled by FunctionCallModifier).\n", len(events))
	fmt.Println("    => The user_id parameter was injected/removed transparently by the plugin.")
	return 0
}

// ---------------------------------------------------------------------------
// Demo 7.4 — Configurable agent tree construction
// ---------------------------------------------------------------------------

func demoConfigurableConstruction() int {
	fmt.Println("[Demo 7.4] Configurable Agent Tree Construction")

	jsonCfg := `{
		"type": "llm_agent",
		"name": "root",
		"description": "Configurable root agent with sub-agents",
		"tools": ["get_weather"],
		"sub_agents": [
			{
				"type": "llm_agent",
				"name": "math_agent",
				"description": "Handles math problems"
			}
		]
	}`

	cfg, err := agentconfig.FromJSON([]byte(jsonCfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "  FromJSON error: %v\n", err)
		return 1
	}

	registry := agentconfig.ToolRegistry{
		"get_weather": tool.NewFunctionTool("get_weather", "Get current weather",
			func(args map[string]any) (map[string]any, error) {
				city, _ := args["city"].(string)
				return map[string]any{"city": city, "temperature": 25, "condition": "clear"}, nil
			},
		),
	}

	agt, err := agentconfig.Build(cfg, registry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Build error: %v\n", err)
		return 1
	}

	fmt.Printf("  Root agent: %s (%s)\n", agt.Name(), agt.Description())
	subs := agt.SubAgents()
	fmt.Printf("  Sub-agents: %d\n", len(subs))
	for _, s := range subs {
		fmt.Printf("    - %s: %s\n", s.Name(), s.Description())
	}
	fmt.Printf("  Parent chain: root=%v", agt.Parent() == nil)
	for _, s := range subs {
		fmt.Printf(", %s.parent=root->%v", s.Name(), s.Parent() != nil && s.Parent().Name() == "root")
	}
	fmt.Println()

	// Validate error paths
	fmt.Println()
	fmt.Println("  Validation tests:")

	// Duplicate name
	dupCfg := agentconfig.AgentConfig{
		Type: "llm_agent", Name: "dup",
		SubAgents: []agentconfig.AgentConfig{
			{Type: "llm_agent", Name: "child"},
			{Type: "llm_agent", Name: "child"},
		},
	}
	_, err = agentconfig.Build(dupCfg, nil)
	fmt.Printf("    Duplicate name error: %v\n", err)

	// Unknown tool ref
	badToolCfg := agentconfig.AgentConfig{
		Type: "llm_agent", Name: "bad", Tools: []string{"nonexistent"},
	}
	_, err = agentconfig.Build(badToolCfg, agentconfig.ToolRegistry{})
	fmt.Printf("    Unknown tool error: %v\n", err)

	// Unknown type
	unknownTypeCfg := agentconfig.AgentConfig{Type: "unknown", Name: "test"}
	_, err = agentconfig.Build(unknownTypeCfg, nil)
	fmt.Printf("    Unknown type error: %v\n", err)

	fmt.Println("  => Config loader validates all constraints deterministically.")
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

// ---------------------------------------------------------------------------
// Chapter 05 — workflow / AgentTool / remote A2A integration
// ---------------------------------------------------------------------------

func runChapter05() int {
	fmt.Println("--- Chapter 05: Workflow / AgentTool / Remote A2A Integration ---")
	fmt.Println()

	if code := demoSequentialWorkflow(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoParallelWorkflow(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoLoopWorkflow(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoAgentToolDelegation(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoRemoteA2AStreaming(); code != 0 {
		return code
	}

	return 0
}

// ---------------------------------------------------------------------------
// Demo 5.1 — Sequential workflow: two sub-agents execute in order
// ---------------------------------------------------------------------------

func demoSequentialWorkflow() int {
	fmt.Println("[Demo 5.1] Sequential Workflow — code generator → code reviewer")

	coder := newDemoAgent("coder", "Generates Go code.",
		model.TextResponse("func Add(a, b int) int { return a + b }"),
	)
	reviewer := newDemoAgent("reviewer", "Reviews Go code.",
		model.TextResponse("Review passed: function is correct and idiomatic."),
	)

	seq := workflow.NewSequentialAgent("pipeline", "code-gen → review pipeline",
		[]workflow.SubAgent{coder, reviewer},
	)

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "demo_seq",
		Agent:          seq,
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-seq", "Write an Add function")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
		return 1
	}

	fmt.Printf("  %d events produced (coder → reviewer):\n", len(events))
	for i, ev := range events {
		text := ""
		if ev.Content != nil && len(ev.Content.Parts) > 0 {
			text = ev.Content.Parts[0].Text
		}
		fmt.Printf("    [%d] %s: %q\n", i+1, ev.Author, truncate(text, 60))
	}
	fmt.Println("  => Coder output visible to reviewer via shared session state.")
	return 0
}

// ---------------------------------------------------------------------------
// Demo 5.2 — Parallel workflow with branch labels
// ---------------------------------------------------------------------------

func demoParallelWorkflow() int {
	fmt.Println("[Demo 5.2] Parallel Workflow — analyst, critic, evaluator run concurrently")

	analyst := newDemoAgent("analyst", "Market analyst.",
		model.TextResponse("Market trend: upward, growth 12% YoY."),
	)
	critic := newDemoAgent("critic", "Critical reviewer.",
		model.TextResponse("Critique: over-optimistic, missing risk factors."),
	)
	evaluator := newDemoAgent("evaluator", "Evaluator.",
		model.TextResponse("Score: 7/10. Solid analysis but needs risk assessment."),
	)

	par := workflow.NewParallelAgent("review-team", "parallel review",
		[]workflow.SubAgent{analyst, critic, evaluator},
	)

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "demo_par",
		Agent:          par,
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-par", "Analyze the market")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
		return 1
	}

	fmt.Printf("  %d events (concurrent, ordered by declaration):\n", len(events))
	for i, ev := range events {
		text := ""
		if ev.Content != nil && len(ev.Content.Parts) > 0 {
			text = ev.Content.Parts[0].Text
		}
		fmt.Printf("    [%d] %s (branch=%q): %q\n", i+1, ev.Author, ev.Branch, truncate(text, 50))
	}
	fmt.Println("  => Each event carries a branch label 'parent.child' for event grouping.")
	return 0
}

// ---------------------------------------------------------------------------
// Demo 5.3 — Loop workflow with early stop via Escalate
// ---------------------------------------------------------------------------

func demoLoopWorkflow() int {
	fmt.Println("[Demo 5.3] Loop Workflow — code fix loop with Escalate early stop")

	callCount := 0
	fixer := newRawDemoAgent("fixer", "Iterative code fixer.",
		func(ctx agent.InvocationContext) ([]*event.Event, error) {
			callCount++
			if callCount >= 3 {
				return []*event.Event{{
					ID:     "fix-done",
					Author: "fixer",
					Content: &event.Content{Role: event.RoleModel,
						Parts: []event.Part{{Text: "All tests pass! Stopping iteration."}}},
					Actions: event.EventActions{Escalate: true},
				}}, nil
			}
			return []*event.Event{{
				ID:     fmt.Sprintf("fix-%d", callCount),
				Author: "fixer",
				Content: &event.Content{Role: event.RoleModel,
					Parts: []event.Part{{Text: fmt.Sprintf("Fix round %d: tests failing, retrying...", callCount)}}},
			}}, nil
		},
	)

	loop := workflow.NewLoopAgent("fix-loop", "iterative code fix loop",
		[]workflow.SubAgent{fixer}, 10,
	)

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "demo_loop",
		Agent:          loop,
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-loop", "Fix the code")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
		return 1
	}

	fmt.Printf("  %d iterations before escalate (max=10):\n", callCount)
	for i, ev := range events {
		text := ""
		if ev.Content != nil && len(ev.Content.Parts) > 0 {
			text = ev.Content.Parts[0].Text
		}
		esc := ""
		if ev.Actions.Escalate {
			esc = " [ESCALATE]"
		}
		fmt.Printf("    [%d] %s: %q%s\n", i+1, ev.Author, text, esc)
	}
	fmt.Println("  => Agent signals loop termination via Actions.Escalate=true.")
	return 0
}

// ---------------------------------------------------------------------------
// Demo 5.4 — AgentTool delegation (agent as tool inside parent flow)
// ---------------------------------------------------------------------------

func demoAgentToolDelegation() int {
	fmt.Println("[Demo 5.4] AgentTool Delegation — parent agent delegates to math_agent tool")

	// Child agent: wrapped as a tool.
	childAgent, err := agent.New(agent.Config{
		Name:        "math_agent",
		Description: "Solves math problems. Input: a description of the problem.",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			ev := event.NewEvent("math-result", "math_agent", event.RoleModel)
			ev.Content = &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: "42"}},
			}
			return []*event.Event{ev}, nil
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create child agent: %v\n", err)
		return 1
	}

	at := agenttool.New(childAgent, nil)
	ft, ok := at.(tool.FunctionTool)
	if !ok {
		fmt.Fprintln(os.Stderr, "agenttool does not implement FunctionTool")
		return 1
	}

	// Parent agent with math_agent as a registered tool.
	fakeModel := model.NewFakeModel("orchestrator-model",
		model.FunctionCallResponse("Let me delegate to the math agent.",
			event.FunctionCall{ID: "fc-math", Name: "math_agent", Args: map[string]any{"request": "what is 6*7"}},
		),
		model.TextResponse("The math agent says the answer is 42."),
	)

	parentFlow := &flow.Flow{
		Model: fakeModel,
		Tools: map[string]tool.FunctionTool{
			"math_agent": ft,
		},
	}

	parentAgent, err := llmagent.New("orchestrator", "Parent agent with delegation capability.", parentFlow)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create parent agent: %v\n", err)
		return 1
	}

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "demo_agenttool",
		Agent:          parentAgent.(runner.ExecutableAgent),
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-at", "What is 6*7?")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
		return 1
	}

	for _, ev := range events {
		if ev.Content != nil {
			for _, p := range ev.Content.Parts {
				switch {
				case p.FunctionCall != nil:
					fmt.Printf("  [%s] FunctionCall → %s(args=%v)\n", ev.Author, p.FunctionCall.Name, p.FunctionCall.Args)
				case p.FunctionResponse != nil:
					fmt.Printf("  [%s] FunctionResponse ← %s => %v\n", ev.Author, p.FunctionResponse.Name, p.FunctionResponse.Result)
				case p.Text != "":
					fmt.Printf("  [%s] %q\n", ev.Author, truncate(p.Text, 60))
				}
			}
		}
	}
	fmt.Println("  => Child agent runs in isolated session; parent receives result via tool interface.")
	return 0
}

// ---------------------------------------------------------------------------
// Demo 5.5 — Remote A2A streaming aggregation
// ---------------------------------------------------------------------------

func demoRemoteA2AStreaming() int {
	fmt.Println("[Demo 5.5] Remote A2A Streaming — aggregated partial chunks from remote KB agent")

	fakeCfg := remoteagent.FakeA2AClientConfig{
		Card: remoteagent.AgentCard{
			Name:               "remote-kb",
			Description:        "Remote knowledge base with streaming support",
			StreamingSupported: true,
			Capabilities:       []string{"knowledge-retrieval", "streaming"},
		},
		Events: []remoteagent.StreamEvent{
			// Simulated streaming chunks from a remote A2A service.
			{Event: &remoteagent.RemoteEvent{
				Type:      remoteagent.RemoteEventTaskArtifactUpdate,
				TaskID:    "task-1",
				Parts:     []remoteagent.RemotePart{{Text: "According "}},
				Append:    true,
				LastChunk: false,
			}},
			{Event: &remoteagent.RemoteEvent{
				Type:      remoteagent.RemoteEventTaskArtifactUpdate,
				TaskID:    "task-1",
				Parts:     []remoteagent.RemotePart{{Text: "to the "}},
				Append:    true,
				LastChunk: false,
			}},
			{Event: &remoteagent.RemoteEvent{
				Type:      remoteagent.RemoteEventTaskArtifactUpdate,
				TaskID:    "task-1",
				Parts:     []remoteagent.RemotePart{{Text: "latest data, "}},
				Append:    true,
				LastChunk: false,
			}},
			{Event: &remoteagent.RemoteEvent{
				Type:      remoteagent.RemoteEventTaskArtifactUpdate,
				TaskID:    "task-1",
				Parts:     []remoteagent.RemotePart{{Text: "the capital is Tokyo."}},
				Append:    true,
				LastChunk: true,
			}},
			// Terminal status.
			{Event: &remoteagent.RemoteEvent{
				Type:   remoteagent.RemoteEventTaskStatusUpdate,
				TaskID: "task-1",
				State:  remoteagent.TaskStateCompleted,
			}},
		},
	}

	remoteAgent, err := remoteagent.NewRemoteAgent(remoteagent.RemoteAgentConfig{
		Name:        "kb-agent",
		Description: "Remote knowledge base bridge",
		AgentCard:   fakeCfg.Card,
		ClientProvider: func(card remoteagent.AgentCard) (remoteagent.A2AClient, error) {
			return remoteagent.NewFakeA2AClient(fakeCfg), nil
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create remote agent: %v\n", err)
		return 1
	}

	sessionSvc := runner.NewInMemorySessionService()
	r, err := runner.New(runner.Config{
		AppName:        "demo_a2a",
		Agent:          remoteAgent,
		SessionService: sessionSvc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	_, events, err := r.Run(stdctx.Background(), "user-1", "sess-a2a", "What is the capital?")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
		return 1
	}

	fmt.Printf("  %d raw events → aggregated into non-partial events:\n", 5)
	for _, ev := range events {
		text := ""
		if ev.Content != nil && len(ev.Content.Parts) > 0 {
			text = ev.Content.Parts[0].Text
		}
		partial := ""
		if ev.Partial {
			partial = " [PARTIAL-suppressed]"
		}
		status := ""
		if ev.Actions.StateDelta != nil {
			if s, ok := ev.Actions.StateDelta["remote_task_state"]; ok {
				status = fmt.Sprintf(" [status=%v]", s)
			}
		}
		if text != "" || status != "" {
			fmt.Printf("    %q%s%s\n", truncate(text, 60), partial, status)
		}
	}
	fmt.Println("  => Partial chunks are aggregated into complete events; terminal status flushes all.")
	return 0
}

// ---------------------------------------------------------------------------
// Demo agent helpers for Chapter 05
// ---------------------------------------------------------------------------

// newDemoAgent creates an LLM-backed demo sub-agent from a text response.
func newDemoAgent(name, desc string, responses ...*model.LLMResponse) workflow.SubAgent {
	f := &flow.Flow{Model: model.NewFakeModel("fake-"+name, responses...)}
	a, _ := llmagent.New(name, desc, f)
	return a.(workflow.SubAgent)
}

// newRawDemoAgent creates a sub-agent from a raw run function (for escalate demos).
func newRawDemoAgent(name, desc string, runFn func(ctx agent.InvocationContext) ([]*event.Event, error)) workflow.SubAgent {
	a, _ := agent.New(agent.Config{
		Name:        name,
		Description: desc,
		Run:         runFn,
	})
	return a
}

// ---------------------------------------------------------------------------
// Chapter 06 — entrypoint, deploy, and telemetry
// ---------------------------------------------------------------------------

func runChapter06() int {
	fmt.Println("--- Chapter 06: Entrypoint, Deploy, Telemetry ---")
	fmt.Println()

	if code := demoLauncherConfig(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoDeployPlans(); code != 0 {
		return code
	}
	fmt.Println()

	if code := demoTelemetryInstrumentation(); code != 0 {
		return code
	}

	return 0
}

// ---------------------------------------------------------------------------
// Demo 6.1 — Launcher Config keeps entrypoints stable
// ---------------------------------------------------------------------------

func demoLauncherConfig() int {
	fmt.Println("[Demo 6.1] Launcher Config — Stable Entrypoint Abstraction")
	fmt.Println("  The launcher.Config carries services (session, memory, artifact,")
	fmt.Println("  agent loader, plugin manager) so sublaunchers (console, web)")
	fmt.Println("  don't create their own global singletons.")
	fmt.Println()
	fmt.Println("  launcher.Config fields:")
	fmt.Println("    SessionService  — in-memory session store")
	fmt.Println("    ArtifactService — versioned file store")
	fmt.Println("    MemoryService   — cross-session long-term memory")
	fmt.Println("    AgentLoader     — loads agent by name")
	fmt.Println("    PluginManager   — composable hook bundles")
	fmt.Println()
	fmt.Println("  SubLauncher interface:")
	fmt.Println("    Keyword()           — routing keyword (e.g. 'console', 'web')")
	fmt.Println("    Parse(args)         — parse sublauncher flags")
	fmt.Println("    Run(ctx, config)    — execute with launcher config")
	fmt.Println("    CommandLineSyntax() — help text")
	fmt.Println("    SimpleDescription() — one-line description")
	fmt.Println()
	fmt.Println("  Universal launcher routes first argv token to sublauncher keyword.")
	fmt.Println("  First registered sublauncher is the default (console).")
	fmt.Println()

	// Demonstrate the universal launcher routing table.
	fmt.Println("  Routing table:")
	fmt.Println("    (no args)  → console (default)")
	fmt.Println("    console    → console sublauncher")
	fmt.Println("    web        → web sublauncher (REST + SSE)")
	fmt.Println()
	fmt.Println("  => Launcher config keeps agents decoupled from I/O transport.")
	return 0
}

// ---------------------------------------------------------------------------
// Demo 6.2 — Dry-run deploy plans (Cloud Run + Agent Engine)
// ---------------------------------------------------------------------------

func demoDeployPlans() int {
	fmt.Println("[Demo 6.2] Dry-Run Deploy Plans (Cloud Run + Agent Engine)")
	fmt.Println()

	// --- Cloud Run dry-run plan ---
	crPlan, err := deploy.PlanCloudRun(deploy.CloudRunConfig{
		EntryPoint:  "cmd/myserver/main.go",
		Project:     "my-gcp-project",
		Region:      "us-central1",
		ServiceName: "my-adk-service",
		ServerPort:  8080,
		ProxyPort:   8081,
		Protocols:   []deploy.Protocol{deploy.ProtocolAPI, deploy.ProtocolWebUI},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cloud Run plan error: %v\n", err)
		return 1
	}

	fmt.Println("  --- Cloud Run Dry-Run Plan ---")
	fmt.Printf("  Entry point:    %s\n", crPlan.EntryPoint)
	fmt.Printf("  Binary:         %s\n", crPlan.ExecFile)
	fmt.Printf("  GCP project:    %s\n", crPlan.Project)
	fmt.Printf("  Region:         %s\n", crPlan.Region)
	fmt.Printf("  Service:        %s\n", crPlan.ServiceName)
	fmt.Printf("  Server port:    %d\n", crPlan.ServerPort)
	fmt.Println()
	fmt.Println("  Build command:")
	fmt.Printf("    $ %s\n", crPlan.BuildCmd())
	fmt.Println()
	fmt.Println("  Dockerfile (distroless):")
	for _, line := range strings.Split(crPlan.Dockerfile(), "\n") {
		if strings.TrimSpace(line) != "" {
			fmt.Printf("    %s\n", line)
		}
	}
	fmt.Println()
	fmt.Println("  Deploy command:")
	fmt.Printf("    (gcloud run deploy with --source . --set-secrets)\n")
	fmt.Println()
	fmt.Println("  Local proxy:")
	fmt.Printf("    $ %s\n", crPlan.ProxyCmd())
	fmt.Println()

	// --- Agent Engine dry-run plan ---
	aePlan, err := deploy.PlanAgentEngine(deploy.AgentEngineConfig{
		EntryPoint: "cmd/myagent/main.go",
		Project:    "my-gcp-project",
		Region:     "us-central1",
		Name:       "my-reasoning-engine",
		ServerPort: 8080,
		SourceDir:  ".",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Agent Engine plan error: %v\n", err)
		return 1
	}

	fmt.Println("  --- Agent Engine Dry-Run Plan ---")
	fmt.Printf("  Entry point:    %s\n", aePlan.EntryPoint)
	fmt.Printf("  Binary:         %s\n", aePlan.ExecFile)
	fmt.Printf("  GCP project:    %s\n", aePlan.Project)
	fmt.Printf("  Region:         %s\n", aePlan.Region)
	fmt.Printf("  Name:           %s\n", aePlan.Name)
	fmt.Println()
	fmt.Println("  Dockerfile (multi-stage: golang builder + distroless):")
	for _, line := range strings.Split(aePlan.Dockerfile(), "\n") {
		if strings.TrimSpace(line) != "" {
			fmt.Printf("    %s\n", line)
		}
	}
	fmt.Println()
	fmt.Println("  Source archive command:")
	fmt.Println("    $ tar -czf archive.tgz --exclude=.git ...")
	fmt.Println()
	fmt.Printf("  Stream query endpoint: %s\n", aePlan.StreamURL())
	fmt.Println()
	fmt.Println("  => Plans are deterministic dry-run snapshots — no gcloud, Docker,")
	fmt.Println("     or network calls. All output is reproducible for the same input.")
	fmt.Println("     CMD always invokes the web launcher with the relevant protocol")
	fmt.Println("     flags (api, a2a, webui, agentengine).")

	return 0
}

// ---------------------------------------------------------------------------
// Demo 6.3 — Telemetry capture around a runner invocation
// ---------------------------------------------------------------------------

func demoTelemetryInstrumentation() int {
	fmt.Println("[Demo 6.3] Telemetry Capture Around a Runner Invocation")
	fmt.Println()

	rec := telemetry.NewRecorder(telemetry.WithCaptureMessageContent(true))
	ctx := stdctx.Background()

	agt, err := agent.New(agent.Config{
		Name:        "telemetry_bot",
		Description: "Agent for telemetry demo",
		Run: func(ctx agent.InvocationContext) ([]*event.Event, error) {
			ev := event.NewEvent("ev-tel", "telemetry_bot", event.RoleModel)
			ev.Content = &event.Content{
				Role:  event.RoleModel,
				Parts: []event.Part{{Text: "Telemetry capture is working."}},
			}
			return []*event.Event{ev}, nil
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent: %v\n", err)
		return 1
	}

	ea := runner.ExecutableAgent(agt)
	r, err := runner.New(runner.Config{
		AppName:        "telemetry_demo",
		Agent:          ea,
		SessionService: runner.NewInMemorySessionService(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create runner: %v\n", err)
		return 1
	}

	invocationID := "inv-demo-1"
	sessionID := "sess-telemetry"
	agentName := "telemetry_bot"

	// Start spans for a full invocation lifecycle.
	invSpan := telemetry.StartInvokeAgentSpan(ctx, rec, agentName,
		"Agent for telemetry demo", sessionID, invocationID)
	serverSpan := telemetry.StartServerEventSpan(ctx, rec, "POST", "/run_sse")
	telemetry.LogServerEvent(ctx, rec, "POST", "/run_sse", 200, 12*time.Millisecond)

	// Run the agent.
	_, events, err := r.Run(ctx, "user-tel", sessionID, "Show telemetry")
	if err != nil {
		invSpan.EndWithError("ERROR", err.Error())
		serverSpan.EndWithError("ERROR", err.Error())
		fmt.Fprintf(os.Stderr, "Runner.Run: %v\n", err)
		return 1
	}

	// Model span with token usage.
	modelSpan := telemetry.StartGenerateContentSpan(ctx, rec, "fake-model", invocationID)
	if len(events) > 0 {
		telemetry.SetEventID(modelSpan, events[0].ID)
	}
	telemetry.SetTokenUsage(modelSpan, 150, 80, 20, 10)
	telemetry.LogRequest(ctx, rec, "You are a helpful bot.", "Show telemetry")
	telemetry.LogResponse(ctx, rec, "STOP", "Telemetry capture is working.", nil)
	modelSpan.End("OK")

	invSpan.End("OK")
	serverSpan.End("OK")

	// Print span summary.
	spans := rec.Spans()
	fmt.Printf("  Recorded spans: %d\n", len(spans))
	for i, s := range spans {
		fmt.Printf("    [%d] %s (status=%s)\n", i+1, s.Name, s.Status)
		for k, v := range s.Attributes {
			fmt.Printf("         %s: %v\n", k, v)
		}
	}

	// Print log summary.
	logs := rec.Logs()
	fmt.Printf("\n  Recorded logs: %d\n", len(logs))
	for i, l := range logs {
		fmt.Printf("    [%d] %s", i+1, l.EventName)
		if l.Attributes != nil {
			for k, v := range l.Attributes {
				fmt.Printf(" (%s=%v)", k, v)
			}
		}
		fmt.Println()
	}

	fmt.Println()
	fmt.Println("  Span types recorded:")
	fmt.Println("    invoke_agent     — agent name, session ID, invocation ID")
	fmt.Println("    generate_content — model name, token usage, event ID")
	fmt.Println("    server POST /run_sse — operation and path")
	fmt.Println()
	fmt.Println("  Log types recorded:")
	fmt.Println("    server.request      — HTTP method, path, status, duration")
	fmt.Println("    gen_ai.system.message — system instruction (with capture toggle)")
	fmt.Println("    gen_ai.user.message   — user message (with capture toggle)")
	fmt.Println("    gen_ai.choice         — model response, finish reason, tool calls")
	fmt.Println()
	fmt.Println("  Content capture: enabled (WithCaptureMessageContent).")
	fmt.Println("    When disabled (default), message bodies are <elided>")
	fmt.Println("    to avoid recording PII or secrets. This mirrors the")
	fmt.Println("    OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT pattern")
	fmt.Println("    from ADK Go.")
	fmt.Println()
	fmt.Println("  => Telemetry recorder is in-memory and thread-safe;")
	fmt.Println("     Providers wrap the recorder with Init/Shutdown lifecycle.")

	return 0
}
