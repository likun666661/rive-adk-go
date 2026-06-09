package tool

import (
	"errors"
	"sort"
	"testing"

	"github.com/likun666661/rive-adk-go/model"
)

// ---------------------------------------------------------------------------
// 1. Tool interface + FunctionTool backward compatibility
// ---------------------------------------------------------------------------

func TestFunctionToolSatisfiesTool(t *testing.T) {
	ft := NewFunctionTool("echo", "Echoes input",
		func(args map[string]any) (map[string]any, error) {
			return args, nil
		},
	)

	// FunctionTool must satisfy the Tool interface.
	var _ Tool = ft
	if ft.Name() != "echo" {
		t.Errorf("Name = %q", ft.Name())
	}
	if ft.Description() != "Echoes input" {
		t.Errorf("Description = %q", ft.Description())
	}

	// run still works
	result, err := ft.Run(map[string]any{"x": 1})
	if err != nil {
		t.Fatal(err)
	}
	if result["x"] != 1 {
		t.Errorf("result[x] = %v", result["x"])
	}
}

func TestFunctionToolAsTool(t *testing.T) {
	ft := NewFunctionTool("search", "Search the web",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"results": []string{"a", "b"}}, nil
		},
	)
	tl := FunctionToolAsTool(ft)
	if tl.Name() != "search" {
		t.Errorf("Name = %q", tl.Name())
	}
}

// ---------------------------------------------------------------------------
// 2. Stable declarations from function tools
// ---------------------------------------------------------------------------

func TestFunctionToolStableDeclaration(t *testing.T) {
	inputSchema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"city": map[string]any{"type": "string"}},
		"required":   []any{"city"},
	}
	outputSchema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"temperature": map[string]any{"type": "number"}},
	}
	decl := NewDeclaration("get_weather", "Get weather for a city",
		inputSchema,
		outputSchema,
	)
	inputSchema["type"] = "array"
	inputSchema["properties"].(map[string]any)["city"] = map[string]any{"type": "number"}
	outputSchema["type"] = "array"

	ft := NewFunctionToolWithDeclaration("get_weather", "Get weather for a city", decl,
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"temperature": 22}, nil
		},
	)
	decl.InputSchema["type"] = "boolean"
	decl.OutputSchema["type"] = "boolean"

	dp, ok := ft.(DeclarationProvider)
	if !ok {
		t.Fatal("FunctionTool should implement DeclarationProvider")
	}

	d := dp.Declaration()

	// Stable declaration: same every call, even if caller mutates returned maps.
	d.InputSchema["type"] = "mutated"
	d.InputSchema["properties"].(map[string]any)["city"] = map[string]any{"type": "integer"}
	d2 := dp.Declaration()
	if d2.InputSchema["type"] != "object" {
		t.Errorf("declaration input schema mutated across calls: %v", d2.InputSchema["type"])
	}
	if got := d2.InputSchema["properties"].(map[string]any)["city"].(map[string]any)["type"]; got != "string" {
		t.Errorf("city schema type = %v, want string", got)
	}

	if d.Name != d2.Name || d.Description != d2.Description {
		t.Error("declaration is not stable across calls")
	}

	if d.Name != "get_weather" {
		t.Errorf("decl.Name = %q, want 'get_weather'", d.Name)
	}
	if d.Description != "Get weather for a city" {
		t.Errorf("decl.Description = %q", d.Description)
	}
	if d.InputSchema == nil {
		t.Error("InputSchema should not be nil")
	}
	if d.OutputSchema == nil {
		t.Error("OutputSchema should not be nil")
	}
	if d.InputSchema["properties"] == nil {
		t.Error("InputSchema should have properties")
	}
}

func TestConfirmationWrapperPreservesDeclaration(t *testing.T) {
	decl := NewDeclaration("delete_data", "Delete data",
		map[string]any{
			"type":       "object",
			"properties": map[string]any{"id": map[string]any{"type": "string"}},
		},
		nil,
	)

	inner := NewFunctionToolWithDeclaration("delete_data", "Delete data", decl,
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"deleted": true}, nil
		},
	)
	wrapped := WithConfirmation(inner, true, nil)
	dp, ok := wrapped.(DeclarationProvider)
	if !ok {
		t.Fatal("confirmation wrapper should preserve DeclarationProvider")
	}

	d := dp.Declaration()
	if d.Name != "delete_data" {
		t.Errorf("declaration name = %q, want delete_data", d.Name)
	}
}

func TestFunctionToolEmptyDeclaration(t *testing.T) {
	ft := NewFunctionTool("echo", "Echo input",
		func(args map[string]any) (map[string]any, error) {
			return args, nil
		},
	)

	dp, ok := ft.(DeclarationProvider)
	if !ok {
		t.Fatal("FunctionTool should implement DeclarationProvider")
	}
	d := dp.Declaration()
	if d.Name != "" {
		t.Errorf("expected empty declaration name, got %q", d.Name)
	}
}

func TestDeclarationNotCollectedWhenEmpty(t *testing.T) {
	ft := NewFunctionTool("no_decl", "No declaration",
		func(args map[string]any) (map[string]any, error) {
			return nil, nil
		},
	)

	decls := CollectDeclarations([]Tool{ft})
	if len(decls) != 0 {
		t.Errorf("expected 0 declarations for tool with empty declaration, got %d", len(decls))
	}
}

// ---------------------------------------------------------------------------
// 3. Toolset collections
// ---------------------------------------------------------------------------

func TestStaticToolset(t *testing.T) {
	t1 := NewFunctionTool("tool_a", "First tool",
		func(args map[string]any) (map[string]any, error) {
			return nil, nil
		},
	)
	t2 := NewFunctionTool("tool_b", "Second tool",
		func(args map[string]any) (map[string]any, error) {
			return nil, nil
		},
	)

	ts := NewStaticToolset("my_tools", []Tool{FunctionToolAsTool(t1), FunctionToolAsTool(t2)})

	if ts.Name() != "my_tools" {
		t.Errorf("Name = %q, want 'my_tools'", ts.Name())
	}

	tools, err := ts.Tools()
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	if !names["tool_a"] || !names["tool_b"] {
		t.Errorf("missing tool names: %v", names)
	}
}

func TestEmptyStaticToolset(t *testing.T) {
	ts := NewStaticToolset("empty", []Tool{})
	tools, err := ts.Tools()
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

// ---------------------------------------------------------------------------
// 4. Filtering / AllowedTools
// ---------------------------------------------------------------------------

func TestAllowedToolsPredicate(t *testing.T) {
	t1 := newTestFuncTool("get_weather")
	t2 := newTestFuncTool("search")
	t3 := newTestFuncTool("calculator")

	allow := AllowedToolsPredicate("get_weather", "calculator")

	if !allow(t1) {
		t.Error("get_weather should be allowed")
	}
	if allow(t2) {
		t.Error("search should NOT be allowed")
	}
	if !allow(t3) {
		t.Error("calculator should be allowed")
	}
}

func TestFilterToolsetByName(t *testing.T) {
	t1 := newTestFuncTool("get_weather")
	t2 := newTestFuncTool("search")
	t3 := newTestFuncTool("calculator")

	inner := NewStaticToolset("full", []Tool{t1, t2, t3})
	filtered := NewFilterToolset("filtered", inner, AllowedToolsPredicate("get_weather", "calculator"))

	tools, err := filtered.Tools()
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools after filtering, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	if !names["get_weather"] || !names["calculator"] {
		t.Errorf("unexpected tools after filtering: %v", names)
	}
	if names["search"] {
		t.Error("search should have been filtered out")
	}
}

func TestFilterToolsetAllBlocked(t *testing.T) {
	t1 := newTestFuncTool("get_weather")
	inner := NewStaticToolset("inner", []Tool{t1})
	filtered := NewFilterToolset("none", inner, AllowedToolsPredicate("search"))

	tools, err := filtered.Tools()
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestFilterToolsetAllowAll(t *testing.T) {
	t1 := newTestFuncTool("get_weather")
	t2 := newTestFuncTool("search")
	inner := NewStaticToolset("inner", []Tool{t1, t2})

	allowAll := func(t Tool) bool { return true }
	filtered := NewFilterToolset("all", inner, allowAll)

	tools, err := filtered.Tools()
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

func TestFilterToolsetPreservesName(t *testing.T) {
	t1 := newTestFuncTool("get_weather")
	inner := NewStaticToolset("inner", []Tool{t1})
	filtered := NewFilterToolset("my_filtered", inner, AllowedToolsPredicate("get_weather"))

	if filtered.Name() != "my_filtered" {
		t.Errorf("Name = %q, want 'my_filtered'", filtered.Name())
	}
}

// ---------------------------------------------------------------------------
// 5. Declaration injection into LLMRequest — deterministic & ordered
// ---------------------------------------------------------------------------

func TestInjectDeclarationsDeterministicAndOrdered(t *testing.T) {
	declC := NewDeclaration("calculator", "Perform math",
		map[string]any{"type": "object", "properties": map[string]any{"expr": map[string]any{"type": "string"}}},
		nil,
	)
	declA := NewDeclaration("get_weather", "Get weather",
		map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}},
		nil,
	)
	declB := NewDeclaration("search", "Search the web",
		map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}},
		nil,
	)

	t1 := NewFunctionToolWithDeclaration("calculator", "math", declC, func(args map[string]any) (map[string]any, error) { return nil, nil })
	t2 := NewFunctionToolWithDeclaration("get_weather", "weather", declA, func(args map[string]any) (map[string]any, error) { return nil, nil })
	t3 := NewFunctionToolWithDeclaration("search", "search", declB, func(args map[string]any) (map[string]any, error) { return nil, nil })

	tools := []Tool{
		FunctionToolAsTool(t1),
		FunctionToolAsTool(t2),
		FunctionToolAsTool(t3),
	}

	// Run multiple times — must be deterministic (sorted by name).
	for i := 0; i < 5; i++ {
		req := &model.LLMRequest{}
		InjectDeclarations(req, tools)

		if len(req.ToolDeclarations) != 3 {
			t.Fatalf("iteration %d: expected 3 tool declarations, got %d", i, len(req.ToolDeclarations))
		}

		// Sorted by name: calculator < get_weather < search
		names := make([]string, len(req.ToolDeclarations))
		for j, td := range req.ToolDeclarations {
			d, ok := td.(Declaration)
			if !ok {
				t.Fatalf("iteration %d: ToolDeclarations[%d] is not Declaration, got %T", i, j, td)
			}
			names[j] = d.Name
		}

		if names[0] != "calculator" || names[1] != "get_weather" || names[2] != "search" {
			t.Errorf("iteration %d: wrong order: %v (expected [calculator get_weather search])", i, names)
		}
	}
}

func TestInjectDeclarationsHandlesNilRequest(t *testing.T) {
	t1 := newTestFuncTool("search")
	// Should not panic.
	InjectDeclarations(nil, []Tool{t1})
}

func TestInjectDeclarationsSkipsToolsWithoutDeclaration(t *testing.T) {
	t1 := NewFunctionTool("no_decl", "No declaration tool",
		func(args map[string]any) (map[string]any, error) { return nil, nil },
	)
	decl := NewDeclaration("has_decl", "Has declaration", nil, nil)
	t2 := NewFunctionToolWithDeclaration("has_decl", "tool2", decl,
		func(args map[string]any) (map[string]any, error) { return nil, nil },
	)

	req := &model.LLMRequest{}
	InjectDeclarations(req, []Tool{FunctionToolAsTool(t1), FunctionToolAsTool(t2)})

	if len(req.ToolDeclarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(req.ToolDeclarations))
	}
	d := req.ToolDeclarations[0].(Declaration)
	if d.Name != "has_decl" {
		t.Errorf("Name = %q, want 'has_decl'", d.Name)
	}
}

func TestCollectDeclarationsEmptyList(t *testing.T) {
	decls := CollectDeclarations([]Tool{})
	if len(decls) != 0 {
		t.Errorf("expected 0 declarations, got %d", len(decls))
	}
}

func TestCollectDeclarationsSortOrder(t *testing.T) {
	// Bulk test: ensure CollectDeclarations always returns sorted-by-name.
	makeTool := func(name string) Tool {
		d := NewDeclaration(name, name+" description", nil, nil)
		ft := NewFunctionToolWithDeclaration(name, name+" desc", d,
			func(args map[string]any) (map[string]any, error) { return nil, nil },
		)
		return FunctionToolAsTool(ft)
	}

	tools := []Tool{
		makeTool("zulu"),
		makeTool("alpha"),
		makeTool("mike"),
		makeTool("beta"),
		makeTool("kilo"),
	}

	decls := CollectDeclarations(tools)
	if len(decls) != 5 {
		t.Fatalf("expected 5 declarations, got %d", len(decls))
	}

	expected := []string{"alpha", "beta", "kilo", "mike", "zulu"}
	for i, d := range decls {
		if d.Name != expected[i] {
			t.Errorf("decls[%d].Name = %q, want %q", i, d.Name, expected[i])
		}
	}

	// Also verify the input slice was not mutated.
	if tools[0].Name() != "zulu" {
		t.Error("input slice was mutated")
	}
}

// ---------------------------------------------------------------------------
// 6. RequestProcessor interface compliance
// ---------------------------------------------------------------------------

type testRequestProcessor struct {
	name     string
	decls    []Declaration
	reqCount int
}

func (p *testRequestProcessor) Name() string { return p.name }

func (p *testRequestProcessor) ProcessRequest(req *model.LLMRequest) error {
	p.reqCount++
	InjectDeclarations(req, p.asTools())
	return nil
}

func (p *testRequestProcessor) asTools() []Tool {
	tools := make([]Tool, len(p.decls))
	for i, d := range p.decls {
		ft := NewFunctionToolWithDeclaration(d.Name, d.Description, d,
			func(args map[string]any) (map[string]any, error) { return nil, nil },
		)
		tools[i] = FunctionToolAsTool(ft)
	}
	return tools
}

func newTestRP(name string, decls ...Declaration) *testRequestProcessor {
	return &testRequestProcessor{name: name, decls: decls}
}

func TestRequestProcessorInjectsDeclarations(t *testing.T) {
	rp := newTestRP("weather_rp",
		NewDeclaration("get_weather", "Get weather",
			map[string]any{"type": "object"},
			nil,
		),
		NewDeclaration("get_forecast", "Get forecast",
			map[string]any{"type": "object"},
			nil,
		),
	)

	req1 := &model.LLMRequest{}
	err := rp.ProcessRequest(req1)
	if err != nil {
		t.Fatal(err)
	}
	if len(req1.ToolDeclarations) != 2 {
		t.Fatalf("expected 2 tool declarations, got %d", len(req1.ToolDeclarations))
	}

	// Ensure deterministic ordering: get_forecast < get_weather
	d0 := req1.ToolDeclarations[0].(Declaration)
	d1 := req1.ToolDeclarations[1].(Declaration)
	if d0.Name != "get_forecast" || d1.Name != "get_weather" {
		t.Errorf("wrong order: [%s, %s]", d0.Name, d1.Name)
	}
}

func TestRequestProcessorMultipleCalls(t *testing.T) {
	rp := newTestRP("rp",
		NewDeclaration("tool_x", "X", nil, nil),
	)

	req := &model.LLMRequest{}
	rp.ProcessRequest(req)
	rp.ProcessRequest(req)
	rp.ProcessRequest(req)

	if rp.reqCount != 3 {
		t.Errorf("reqCount = %d, want 3", rp.reqCount)
	}
}

// ---------------------------------------------------------------------------
// 7. Toolset + RequestProcessor combined: collect tools from toolsets, filter, inject
// ---------------------------------------------------------------------------

func TestToolsetCollectFilterInject(t *testing.T) {
	declA := NewDeclaration("get_weather", "weather", nil, nil)
	declB := NewDeclaration("search", "search web", nil, nil)
	declC := NewDeclaration("calculator", "math", nil, nil)

	ftA := NewFunctionToolWithDeclaration("get_weather", "w", declA, nilRun)
	ftB := NewFunctionToolWithDeclaration("search", "s", declB, nilRun)
	ftC := NewFunctionToolWithDeclaration("calculator", "c", declC, nilRun)

	allToolset := NewStaticToolset("all", []Tool{
		FunctionToolAsTool(ftA),
		FunctionToolAsTool(ftB),
		FunctionToolAsTool(ftC),
	})

	// Filter to only get_weather and calculator
	filtered := NewFilterToolset("filtered", allToolset, AllowedToolsPredicate("get_weather", "calculator"))

	tools, err := filtered.Tools()
	if err != nil {
		t.Fatal(err)
	}

	req := &model.LLMRequest{}
	InjectDeclarations(req, tools)

	if len(req.ToolDeclarations) != 2 {
		t.Fatalf("expected 2 declarations, got %d", len(req.ToolDeclarations))
	}
	names := []string{
		req.ToolDeclarations[0].(Declaration).Name,
		req.ToolDeclarations[1].(Declaration).Name,
	}
	if names[0] != "calculator" || names[1] != "get_weather" {
		t.Errorf("wrong filtered declarations: %v", names)
	}
}

// ---------------------------------------------------------------------------
// 8. Backward compatibility: existing Execute and MergeResults still work
// ---------------------------------------------------------------------------

func TestExecuteCompatibility(t *testing.T) {
	tool := NewFunctionTool("add", "Add two numbers",
		func(args map[string]any) (map[string]any, error) {
			a, _ := args["a"].(int)
			b, _ := args["b"].(int)
			return map[string]any{"sum": a + b}, nil
		},
	)

	cr := Execute("call1", "add", map[string]any{"a": 3, "b": 4}, tool)
	if cr.CallID != "call1" {
		t.Errorf("CallID = %q", cr.CallID)
	}
	if cr.Name != "add" {
		t.Errorf("Name = %q", cr.Name)
	}
	if cr.Error != "" {
		t.Errorf("expected no error, got %q", cr.Error)
	}
	if sum, _ := cr.Result["sum"].(int); sum != 7 {
		t.Errorf("sum = %d, want 7", sum)
	}
}

func TestExecuteToolErrorCompatibility(t *testing.T) {
	tool := NewFunctionTool("failing", "Always fails",
		func(args map[string]any) (map[string]any, error) {
			return nil, errors.New("connection refused")
		},
	)

	cr := Execute("call1", "failing", map[string]any{}, tool)
	if cr.Error != "connection refused" {
		t.Errorf("Error = %q", cr.Error)
	}
	if errStr, ok := cr.Result["error"].(string); !ok || errStr != "connection refused" {
		t.Errorf("Result[error] = %v", cr.Result["error"])
	}
}

func TestExecuteToolNotFoundCompatibility(t *testing.T) {
	cr := Execute("call1", "missing", map[string]any{}, nil)
	if cr.Error == "" {
		t.Error("expected error for nil tool")
	}
	if _, ok := cr.Result["error"].(string); !ok {
		t.Error("expected Result[error] to be set")
	}
}

func TestMergeResultsCompatibility(t *testing.T) {
	results := []CallResult{
		{CallID: "fc1", Name: "get_weather", Result: map[string]any{"temp": 22}},
		{CallID: "fc2", Name: "search", Result: map[string]any{"results": []string{"a", "b"}}},
	}

	merged := MergeResults(results)
	if len(merged) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(merged))
	}

	w, ok := merged["fc1"]
	if !ok {
		t.Fatal("expected 'fc1' key")
	}
	wm := w.(map[string]any)
	if wm["name"] != "get_weather" {
		t.Errorf("name = %q", wm["name"])
	}
}

// ---------------------------------------------------------------------------
// 9. Edge cases & nil handling
// ---------------------------------------------------------------------------

func TestFuncToolSatisfiesFunctionTool(t *testing.T) {
	ft := NewFunctionTool("test", "desc",
		func(args map[string]any) (map[string]any, error) { return nil, nil },
	)
	// Compile-time check: FunctionTool satisfies Tool
	var _ Tool = ft
	var _ FunctionTool = ft
}

func TestStaticToolsetSatisfiesToolset(t *testing.T) {
	var _ Toolset = NewStaticToolset("ts", nil)
}

func TestFilterToolsetSatisfiesToolset(t *testing.T) {
	inner := NewStaticToolset("inner", nil)
	var _ Toolset = NewFilterToolset("f", inner, nil)
}

func TestDeclarationNewDeclarationFields(t *testing.T) {
	d := NewDeclaration("my_tool", "My tool description",
		map[string]any{"type": "object", "required": []any{"x"}},
		map[string]any{"type": "object", "properties": map[string]any{"result": map[string]any{"type": "string"}}},
	)

	if d.Name != "my_tool" {
		t.Errorf("Name = %q", d.Name)
	}
	if d.Description != "My tool description" {
		t.Errorf("Description = %q", d.Description)
	}
	if d.InputSchema == nil {
		t.Error("InputSchema nil")
	}
	if d.OutputSchema == nil {
		t.Error("OutputSchema nil")
	}
}

func TestAllowedToolsPredicateEmpty(t *testing.T) {
	allow := AllowedToolsPredicate()
	tool := newTestFuncTool("any")
	if allow(tool) {
		t.Error("empty allow-list should reject all")
	}
}

func TestAllowedToolsPredicateNilTool(t *testing.T) {
	allow := AllowedToolsPredicate("x")
	// This would panic with nil, but Predicate is typed so it's safe.
	// Just verify the predicate handles an empty named tool.
	tool := newTestFuncTool("")
	if allow(tool) {
		t.Error("empty-named tool should not be in allow-list")
	}
}

// ---------------------------------------------------------------------------
// 10. Integration-style: full pipeline of Toolset → filter → inject
// ---------------------------------------------------------------------------

func TestFullPipelineDeterministicOutput(t *testing.T) {
	// Build 10 tools with declarations, filter to 5, inject, verify order.
	makeTool := func(name string) Tool {
		d := NewDeclaration(name, name,
			map[string]any{"type": "object"},
			nil,
		)
		ft := NewFunctionToolWithDeclaration(name, name, d, nilRun)
		return FunctionToolAsTool(ft)
	}

	names := []string{"z", "a", "m", "b", "k", "c", "x", "d", "y", "e"}
	var tools []Tool
	for _, n := range names {
		tools = append(tools, makeTool(n))
	}

	allTS := NewStaticToolset("all", tools)

	// Allow only: a, b, c, d, e
	filteredTS := NewFilterToolset("letters", allTS, AllowedToolsPredicate("a", "b", "c", "d", "e"))

	filteredTools, err := filteredTS.Tools()
	if err != nil {
		t.Fatal(err)
	}

	req := &model.LLMRequest{}
	InjectDeclarations(req, filteredTools)

	if len(req.ToolDeclarations) != 5 {
		t.Fatalf("expected 5 declarations, got %d", len(req.ToolDeclarations))
	}

	// Must be sorted: a, b, c, d, e
	expected := []string{"a", "b", "c", "d", "e"}
	for i, td := range req.ToolDeclarations {
		d := td.(Declaration)
		if d.Name != expected[i] {
			t.Errorf("declarations[%d] = %q, want %q", i, d.Name, expected[i])
		}
	}

	// Run again — must be identical.
	req2 := &model.LLMRequest{}
	InjectDeclarations(req2, filteredTools)
	if len(req2.ToolDeclarations) != 5 {
		t.Fatalf("second run: expected 5 declarations, got %d", len(req2.ToolDeclarations))
	}
	for i, td := range req2.ToolDeclarations {
		d := td.(Declaration)
		if d.Name != expected[i] {
			t.Errorf("second run: declarations[%d] = %q, want %q", i, d.Name, expected[i])
		}
	}
}

func TestInjectDeclarationsDoesNotMutateInput(t *testing.T) {
	t1 := newTestFuncTool("get_weather")
	t2 := newTestFuncTool("search")
	tools := []Tool{t1, t2}

	originalLen := len(tools)
	req := &model.LLMRequest{}
	InjectDeclarations(req, tools)

	if len(tools) != originalLen {
		t.Error("input tools slice was mutated")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestFuncTool(name string) *FuncTool {
	return &FuncTool{
		name:        name,
		description: name + " description",
		run:         nilRun,
	}
}

func nilRun(args map[string]any) (map[string]any, error) {
	return nil, nil
}

// verifySortOrder is a utility to check that a slice of Declarations is sorted.
func verifySortOrder(t *testing.T, decls []Declaration) {
	t.Helper()
	if !sort.SliceIsSorted(decls, func(i, j int) bool {
		return decls[i].Name < decls[j].Name
	}) {
		t.Error("declarations are not sorted by name")
	}
}
