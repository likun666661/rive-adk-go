package retryreflect

import (
	"errors"
	"testing"
)

func TestPluginName(t *testing.T) {
	p := New(Config{Name: "my_retry", MaxRetries: 2})
	if p.Name() != "my_retry" {
		t.Errorf("name = %q, want 'my_retry'", p.Name())
	}
}

func TestPluginDefaultName(t *testing.T) {
	p := New(Config{MaxRetries: 2})
	if p.Name() != "retry_reflect" {
		t.Errorf("default name = %q, want 'retry_reflect'", p.Name())
	}
}

func TestPluginOnToolErrorAddsReflection(t *testing.T) {
	p := New(Config{Name: "test", MaxRetries: 2})

	origErr := errors.New("connection timeout")
	result, err := p.OnToolErrorCallback()(nil, "test_tool", map[string]any{"key": "val"}, origErr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["error"] != "connection timeout" {
		t.Errorf("result[error] = %q", result["error"])
	}
	reflection, ok := result["reflection"].(string)
	if !ok || reflection == "" {
		t.Error("expected reflection in result")
	}
	if p.FailureCount("test_tool") != 1 {
		t.Errorf("failure count = %d, want 1", p.FailureCount("test_tool"))
	}
}

func TestPluginOnToolErrorExceedsMaxRetries(t *testing.T) {
	p := New(Config{Name: "test", MaxRetries: 2})

	origErr := errors.New("disk full")
	for i := 0; i < 3; i++ {
		result, err := p.OnToolErrorCallback()(nil, "disk_tool", nil, origErr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	}

	result, err := p.OnToolErrorCallback()(nil, "disk_tool", nil, origErr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["reflection_exceeded"] == nil {
		t.Error("expected reflection_exceeded after max retries")
	}
	if p.FailureCount("disk_tool") != 4 {
		t.Errorf("failure count = %d, want 4", p.FailureCount("disk_tool"))
	}
}

func TestPluginAfterToolResetsCounter(t *testing.T) {
	p := New(Config{Name: "test", MaxRetries: 2})

	origErr := errors.New("timeout")
	for i := 0; i < 2; i++ {
		result, err := p.OnToolErrorCallback()(nil, "reset_tool", nil, origErr)
		if err != nil {
			t.Fatalf("OnToolError returned unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("OnToolError returned nil result")
		}
	}
	if p.FailureCount("reset_tool") != 2 {
		t.Errorf("failure count = %d, want 2", p.FailureCount("reset_tool"))
	}

	// Successful tool execution — result has no "error" key
	overrideResult, overrideErr := p.AfterToolCallback()(nil, "reset_tool", nil, map[string]any{"ok": true}, nil)
	if overrideResult != nil || overrideErr != nil {
		t.Errorf("AfterTool = (%v, %v), want (nil, nil)", overrideResult, overrideErr)
	}
	if p.FailureCount("reset_tool") != 0 {
		t.Errorf("failure count after success = %d, want 0", p.FailureCount("reset_tool"))
	}
}

func TestPluginAfterToolDoesNotResetOnError(t *testing.T) {
	p := New(Config{Name: "test", MaxRetries: 2})

	origErr := errors.New("error")
	for i := 0; i < 2; i++ {
		result, err := p.OnToolErrorCallback()(nil, "err_tool", nil, origErr)
		if err != nil {
			t.Fatalf("OnToolError returned unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("OnToolError returned nil result")
		}
	}

	// Result has "error" key — AfterTool should NOT reset counter
	runErr := errors.New("still failing")
	overrideResult, overrideErr := p.AfterToolCallback()(nil, "err_tool", nil, map[string]any{"error": "still failing"}, runErr)
	if overrideResult != nil || overrideErr != nil {
		t.Errorf("AfterTool = (%v, %v), want (nil, nil)", overrideResult, overrideErr)
	}
	if p.FailureCount("err_tool") != 2 {
		t.Errorf("failure count = %d, want 2 (not reset on error)", p.FailureCount("err_tool"))
	}
}
