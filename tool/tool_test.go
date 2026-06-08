package tool

import (
	"errors"
	"testing"
)

func TestFunctionTool(t *testing.T) {
	tool := NewFunctionTool("echo", "Echoes input",
		func(args map[string]any) (map[string]any, error) {
			return args, nil
		},
	)
	if tool.Name() != "echo" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() != "Echoes input" {
		t.Errorf("Description = %q", tool.Description())
	}

	result, err := tool.Run(map[string]any{"msg": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if result["msg"] != "hello" {
		t.Errorf("result[msg] = %v", result["msg"])
	}
}

func TestExecute(t *testing.T) {
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

func TestExecuteToolError(t *testing.T) {
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

func TestExecuteToolNotFound(t *testing.T) {
	cr := Execute("call1", "missing", map[string]any{}, nil)
	if cr.Error == "" {
		t.Error("expected error for nil tool")
	}
	if _, ok := cr.Result["error"].(string); !ok {
		t.Error("expected Result[error] to be set")
	}
}

func TestExecuteToolErrorWithExistingResult(t *testing.T) {
	tool := NewFunctionTool("partial", "Returns partial result with error",
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"partial": "data"}, errors.New("timeout")
		},
	)

	cr := Execute("call1", "partial", map[string]any{}, tool)
	if cr.Error != "timeout" {
		t.Errorf("Error = %q", cr.Error)
	}
	if _, ok := cr.Result["partial"]; !ok {
		t.Error("expected partial data in result")
	}
	if _, ok := cr.Result["error"]; !ok {
		t.Error("expected error in result")
	}
}

func TestMergeResults(t *testing.T) {
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

	s, ok := merged["fc2"]
	if !ok {
		t.Fatal("expected 'fc2' key")
	}
	sm := s.(map[string]any)
	if sm["name"] != "search" {
		t.Errorf("name = %q", sm["name"])
	}
}

func TestMergeResultsWithError(t *testing.T) {
	results := []CallResult{
		{CallID: "fc1", Name: "unreliable", Result: map[string]any{"error": "broken"}, Error: "broken"},
	}

	merged := MergeResults(results)
	entry := merged["fc1"].(map[string]any)
	if entry["error"] != "broken" {
		t.Errorf("error = %q", entry["error"])
	}
}
