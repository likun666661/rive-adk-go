package memory

import (
	"testing"
	"time"

	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/session"
)

func makeSession(app, user, sid string, events ...*event.Event) session.Session {
	s := session.NewInMemorySession(sid, app, user)
	for _, ev := range events {
		_ = s.AppendEvent(ev)
	}
	return s
}

func makeTextEvent(id, author string, role event.Role, text string) *event.Event {
	ev := event.NewEvent(id, author, role)
	ev.Content = &event.Content{Role: role, Parts: []event.Part{{Text: text}}}
	return ev
}

func TestInMemoryService_AddAndSearchMemory(t *testing.T) {
	svc := InMemoryService()

	sess1 := makeSession("myapp", "user1", "sess1",
		makeTextEvent("ev1", "user1", event.RoleUser, "The quick brown fox"),
		makeTextEvent("ev2", "model", event.RoleModel, "jumps over the lazy dog"),
	)
	sess2 := makeSession("myapp", "user1", "sess2",
		makeTextEvent("ev3", "user1", event.RoleUser, "hello world"),
	)

	if err := svc.AddSessionToMemory(t.Context(), sess1); err != nil {
		t.Fatalf("AddSessionToMemory(sess1): %v", err)
	}
	if err := svc.AddSessionToMemory(t.Context(), sess2); err != nil {
		t.Fatalf("AddSessionToMemory(sess2): %v", err)
	}

	resp, err := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user1",
		Query:   "quick hello",
	})
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}

	if len(resp.Memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(resp.Memories))
	}

	hasFox := false
	hasHello := false
	for _, m := range resp.Memories {
		if m.ID == "ev1" {
			hasFox = true
		}
		if m.ID == "ev3" {
			hasHello = true
		}
	}
	if !hasFox || !hasHello {
		t.Errorf("missing expected memories; found: %+v", resp.Memories)
	}
}

func TestInMemoryService_MemorySurvivesAcrossSessions(t *testing.T) {
	svc := InMemoryService()

	sess1 := makeSession("myapp", "user1", "sess1",
		makeTextEvent("ev1", "user1", event.RoleUser, "I like Python"),
	)

	if err := svc.AddSessionToMemory(t.Context(), sess1); err != nil {
		t.Fatalf("AddSessionToMemory: %v", err)
	}

	resp, err := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user1",
		Query:   "Python",
	})
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if len(resp.Memories) != 1 {
		t.Fatalf("expected 1 memory from sess1, got %d", len(resp.Memories))
	}

	// sess1 is gone but memory persists
	sess2 := makeSession("myapp", "user1", "sess2",
		makeTextEvent("ev2", "user1", event.RoleUser, "I use Go now"),
	)
	if err := svc.AddSessionToMemory(t.Context(), sess2); err != nil {
		t.Fatalf("AddSessionToMemory(sess2): %v", err)
	}

	// Search for "Python" — still findable from sess1
	resp2, err := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user1",
		Query:   "Python",
	})
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if len(resp2.Memories) != 1 {
		t.Errorf("memory from sess1 should survive across sessions; got %d memories", len(resp2.Memories))
	}

	// Go is findable too (from sess2)
	resp3, err := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user1",
		Query:   "Go",
	})
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if len(resp3.Memories) != 1 || resp3.Memories[0].ID != "ev2" {
		t.Errorf("expected Go memory from sess2; got %+v", resp3.Memories)
	}
}

func TestInMemoryService_AppScopedIsolation(t *testing.T) {
	svc := InMemoryService()

	sess1 := makeSession("app_a", "user1", "s1",
		makeTextEvent("ev1", "user1", event.RoleUser, "secret project alpha"),
	)
	svc.AddSessionToMemory(t.Context(), sess1)

	resp, err := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "app_b",
		UserID:  "user1",
		Query:   "secret",
	})
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if len(resp.Memories) != 0 {
		t.Error("memory from app_a should not leak to app_b")
	}
}

func TestInMemoryService_UserScopedIsolation(t *testing.T) {
	svc := InMemoryService()

	sess1 := makeSession("myapp", "user_a", "s1",
		makeTextEvent("ev1", "user_a", event.RoleUser, "private data"),
	)
	svc.AddSessionToMemory(t.Context(), sess1)

	resp, err := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user_b",
		Query:   "private",
	})
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if len(resp.Memories) != 0 {
		t.Error("memory from user_a should not leak to user_b")
	}
}

func TestInMemoryService_NoMatches(t *testing.T) {
	svc := InMemoryService()
	sess := makeSession("myapp", "user1", "s1",
		makeTextEvent("ev1", "user1", event.RoleUser, "hello world"),
	)
	svc.AddSessionToMemory(t.Context(), sess)

	resp, err := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user1",
		Query:   "something entirely different",
	})
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if len(resp.Memories) != 0 {
		t.Errorf("expected 0 matches; got %d", len(resp.Memories))
	}
}

func TestInMemoryService_EmptyStore(t *testing.T) {
	svc := InMemoryService()
	resp, err := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user1",
		Query:   "anything",
	})
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if len(resp.Memories) != 0 {
		t.Errorf("expected 0 results from empty store; got %d", len(resp.Memories))
	}
}

func TestInMemoryService_SkipsPartialEvents(t *testing.T) {
	svc := InMemoryService()

	partialEv := &event.Event{
		ID:      "partial1",
		Author:  "model",
		Role:    event.RoleModel,
		Partial: true,
		Content: &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "streaming chunk"}}},
	}

	sess := makeSession("myapp", "user1", "s1", partialEv)
	svc.AddSessionToMemory(t.Context(), sess)

	resp, err := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user1",
		Query:   "streaming",
	})
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if len(resp.Memories) != 0 {
		t.Error("partial events should not be ingested into memory")
	}
}

func TestInMemoryService_UpdatesSessionEntries(t *testing.T) {
	svc := InMemoryService()

	// First insertion
	sess := makeSession("myapp", "user1", "s1",
		makeTextEvent("ev1", "user1", event.RoleUser, "first message"),
	)
	svc.AddSessionToMemory(t.Context(), sess)

	resp, _ := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user1",
		Query:   "first",
	})
	if len(resp.Memories) != 1 {
		t.Fatalf("expected 1 memory; got %d", len(resp.Memories))
	}

	// Second insertion overwrites the same session's entries
	sess2 := makeSession("myapp", "user1", "s1",
		makeTextEvent("ev2", "user1", event.RoleUser, "updated message"),
	)
	svc.AddSessionToMemory(t.Context(), sess2)

	resp2, _ := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user1",
		Query:   "first",
	})
	if len(resp2.Memories) != 0 {
		t.Error("first message should be overwritten by updated session entries")
	}
}

func TestInMemoryService_EntryContentIsCopied(t *testing.T) {
	svc := InMemoryService()

	ev := makeTextEvent("ev1", "user1", event.RoleUser, "original text")
	sess := makeSession("myapp", "user1", "s1", ev)
	svc.AddSessionToMemory(t.Context(), sess)

	ev.Content.Parts[0].Text = "modified text"

	resp, _ := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user1",
		Query:   "original",
	})
	if len(resp.Memories) != 1 {
		t.Error("memory entry should not be affected by subsequent mutation of source event")
	}
}

func TestInMemoryService_EntryCarriesAuthorAndTimestamp(t *testing.T) {
	svc := InMemoryService()

	ev := event.NewEvent("ev1", "agent-bot", event.RoleModel)
	ev.Content = &event.Content{Role: event.RoleModel, Parts: []event.Part{{Text: "bot message"}}}
	ev.Timestamp = time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	sess := makeSession("myapp", "user1", "s1", ev)
	svc.AddSessionToMemory(t.Context(), sess)

	resp, _ := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user1",
		Query:   "bot",
	})
	if len(resp.Memories) != 1 {
		t.Fatal("expected 1 memory")
	}
	m := resp.Memories[0]
	if m.Author != "agent-bot" {
		t.Errorf("Author = %q, want 'agent-bot'", m.Author)
	}
	if m.Timestamp.Year() != 2025 || m.Timestamp.Month() != 6 {
		t.Errorf("Timestamp = %v, want June 2025", m.Timestamp)
	}
}

func TestInMemoryService_SkipsNilContent(t *testing.T) {
	svc := InMemoryService()

	noContentEv := event.NewEvent("ev1", "model", event.RoleModel)
	sess := makeSession("myapp", "user1", "s1", noContentEv)
	svc.AddSessionToMemory(t.Context(), sess)

	resp, _ := svc.SearchMemory(t.Context(), &SearchRequest{
		AppName: "myapp",
		UserID:  "user1",
		Query:   "anything",
	})
	if len(resp.Memories) != 0 {
		t.Error("events with nil content should be skipped")
	}
}
