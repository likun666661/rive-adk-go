package session

import (
	"testing"

	"github.com/likun666661/rive-adk-go/event"
)

func TestSessionAppend(t *testing.T) {
	s := NewInMemorySession("s1", "myapp", "user1")

	if s.ID() != "s1" {
		t.Errorf("ID = %q, want %q", s.ID(), "s1")
	}
	if s.EventCount() != 0 {
		t.Errorf("EventCount = %d, want 0", s.EventCount())
	}

	ev := event.NewEvent("ev1", "agent1", event.RoleModel)
	ev.Content = &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "hello"}}}

	if err := s.AppendEvent(ev); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	if s.EventCount() != 1 {
		t.Errorf("EventCount = %d, want 1", s.EventCount())
	}
	if len(s.Events()) != 1 {
		t.Errorf("len(Events()) = %d, want 1", len(s.Events()))
	}
}

func TestSessionAppendNilEvent(t *testing.T) {
	s := NewInMemorySession("s1", "myapp", "user1")
	if err := s.AppendEvent(nil); err == nil {
		t.Error("expected error for nil event")
	}
}

func TestSessionAppendPartialEvent(t *testing.T) {
	s := NewInMemorySession("s1", "myapp", "user1")
	ev := &event.Event{ID: "ev1", Partial: true}
	if err := s.AppendEvent(ev); err == nil {
		t.Error("expected error for partial event")
	}
	if s.EventCount() != 0 {
		t.Errorf("EventCount = %d, want 0 (partial not persisted)", s.EventCount())
	}
}

func TestSessionStateBasicOps(t *testing.T) {
	s := NewInMemorySession("s1", "myapp", "user1")
	st := s.State()

	v, ok := st.Get("key1")
	if ok || v != nil {
		t.Error("expected key1 to not exist")
	}

	st.Set("key1", "value1")
	v, ok = st.Get("key1")
	if !ok || v != "value1" {
		t.Errorf("Get(key1) = %v, %v, want 'value1', true", v, ok)
	}

	st.Delete("key1")
	_, ok = st.Get("key1")
	if ok {
		t.Error("expected key1 to be deleted")
	}
}

func TestMergeStateDelta(t *testing.T) {
	s := NewInMemorySession("s1", "myapp", "user1")
	s.State().Set("a", "1")
	s.State().Set("b", "2")

	MergeStateDelta(s.State(), map[string]any{
		"a": "updated",
		"c": "3",
	})

	if v, _ := s.State().Get("a"); v != "updated" {
		t.Errorf("a = %v, want 'updated'", v)
	}
	if v, _ := s.State().Get("b"); v != "2" {
		t.Errorf("b = %v, want '2'", v)
	}
	if v, _ := s.State().Get("c"); v != "3" {
		t.Errorf("c = %v, want '3'", v)
	}
}

func TestMergeStateDeltaDeepMerge(t *testing.T) {
	s := NewInMemorySession("s1", "myapp", "user1")
	s.State().Set("nested", map[string]any{
		"x": "1",
		"y": map[string]any{
			"z": "2",
		},
	})

	MergeStateDelta(s.State(), map[string]any{
		"nested": map[string]any{
			"y": map[string]any{
				"w": "3",
			},
		},
	})

	nested, ok := s.State().Get("nested")
	if !ok {
		t.Fatal("nested key not found")
	}
	m := nested.(map[string]any)
	if m["x"] != "1" {
		t.Errorf("x = %v, want '1'", m["x"])
	}
	y := m["y"].(map[string]any)
	if y["z"] != "2" {
		t.Errorf("z = %v, want '2'", y["z"])
	}
	if y["w"] != "3" {
		t.Errorf("w = %v, want '3'", y["w"])
	}
}

func TestMergeStateDeltaNil(t *testing.T) {
	s := NewInMemorySession("s1", "myapp", "user1")
	s.State().Set("x", "1")
	MergeStateDelta(s.State(), nil)
	if v, _ := s.State().Get("x"); v != "1" {
		t.Errorf("x = %v, want '1'", v)
	}
}
