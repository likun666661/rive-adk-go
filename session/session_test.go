package session

import (
	"testing"

	"github.com/likun666661/rive-adk-go/event"
)

// ---------------------------------------------------------------------------
// Existing tests — keep as-is
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// ExtractStateDeltas tests
// ---------------------------------------------------------------------------

func TestExtractStateDeltas(t *testing.T) {
	appDelta, userDelta, sessionDelta := ExtractStateDeltas(map[string]any{
		"app:conf":      "app_val",
		"app:threshold": 42,
		"user:pref":     "dark_mode",
		"user:lang":     "en",
		"session_key":   "s_val",
		"temp:scratch":  "tmp",
		"temp:counter":  999,
	})

	if len(appDelta) != 2 || appDelta["conf"] != "app_val" || appDelta["threshold"] != 42 {
		t.Errorf("appDelta = %v, want {conf:app_val, threshold:42}", appDelta)
	}
	if len(userDelta) != 2 || userDelta["pref"] != "dark_mode" || userDelta["lang"] != "en" {
		t.Errorf("userDelta = %v, want {pref:dark_mode, lang:en}", userDelta)
	}
	if len(sessionDelta) != 1 || sessionDelta["session_key"] != "s_val" {
		t.Errorf("sessionDelta = %v, want {session_key:s_val} (temp keys must be excluded)", sessionDelta)
	}
}

func TestExtractStateDeltasNil(t *testing.T) {
	appDelta, userDelta, sessionDelta := ExtractStateDeltas(nil)
	if len(appDelta) != 0 || len(userDelta) != 0 || len(sessionDelta) != 0 {
		t.Error("expected empty deltas for nil input")
	}
}

func TestExtractStateDeltasEmpty(t *testing.T) {
	appDelta, userDelta, sessionDelta := ExtractStateDeltas(map[string]any{})
	if len(appDelta) != 0 || len(userDelta) != 0 || len(sessionDelta) != 0 {
		t.Error("expected empty deltas for empty input")
	}
}

// ---------------------------------------------------------------------------
// MergeStates tests
// ---------------------------------------------------------------------------

func TestMergeStatesBasicOverlay(t *testing.T) {
	appState := map[string]any{"env": "prod", "version": "1.0"}
	userState := map[string]any{"pref": "dark", "version": "2.0"}
	sessionState := map[string]any{"session_key": "s1", "pref": "light"}

	merged := MergeStates(appState, userState, sessionState)

	if merged["app:env"] != "prod" {
		t.Errorf("app:env = %v, want 'prod'", merged["app:env"])
	}
	if merged["app:version"] != "1.0" {
		t.Errorf("app:version = %v, want '1.0'", merged["app:version"])
	}
	if merged["user:pref"] != "dark" {
		t.Errorf("user:pref = %v, want 'dark'", merged["user:pref"])
	}
	if merged["user:version"] != "2.0" {
		t.Errorf("user:version = %v, want '2.0'", merged["user:version"])
	}
	if merged["session_key"] != "s1" {
		t.Errorf("session_key = %v, want 's1'", merged["session_key"])
	}
	if merged["pref"] != "light" {
		t.Errorf("pref (session overlay) = %v, want 'light'", merged["pref"])
	}
}

func TestMergeStatesTombstoneHidesLowerLayers(t *testing.T) {
	appState := map[string]any{"env": "prod", "deleted": "from_app"}
	userState := map[string]any{"env": "staging", "deleted": "from_user"}
	sessionState := map[string]any{
		"env":     "session_env",
		"deleted": TombstoneValue,
	}

	merged := MergeStates(appState, userState, sessionState)

	if merged["app:env"] != "prod" {
		t.Errorf("app:env = %v, want 'prod' (not tombstoned)", merged["app:env"])
	}
	if _, ok := merged["app:deleted"]; ok {
		t.Errorf("app:deleted should be hidden by session tombstone")
	}
	if _, ok := merged["user:deleted"]; ok {
		t.Errorf("user:deleted should be hidden by session tombstone")
	}
	if merged["deleted"] != TombstoneValue {
		t.Errorf("deleted = %v, want TombstoneValue in session layer", merged["deleted"])
	}
}

func TestMergeStatesEmptyMaps(t *testing.T) {
	merged := MergeStates(nil, nil, nil)
	if len(merged) != 0 {
		t.Errorf("expected empty merge, got %d entries", len(merged))
	}
}

func TestMergeStatesSessionPriority(t *testing.T) {
	appState := map[string]any{"conflict": "app_wins"}
	userState := map[string]any{"conflict": "user_wins"}
	sessionState := map[string]any{"conflict": "session_wins"}

	merged := MergeStates(appState, userState, sessionState)

	if merged["conflict"] != "session_wins" {
		t.Errorf("conflict = %v, want 'session_wins'", merged["conflict"])
	}
	if merged["app:conflict"] != "app_wins" {
		t.Errorf("app:conflict = %v, want 'app_wins'", merged["app:conflict"])
	}
	if merged["user:conflict"] != "user_wins" {
		t.Errorf("user:conflict = %v, want 'user_wins'", merged["user:conflict"])
	}
}

// ---------------------------------------------------------------------------
// TrimTempDeltaState tests
// ---------------------------------------------------------------------------

func TestTrimTempDeltaState(t *testing.T) {
	delta := map[string]any{
		"app:keep":     "a",
		"user:keep":    "u",
		"session:keep": "s",
		"temp:remove1": "t1",
		"temp:remove2": "t2",
	}

	trimmed := trimTempDeltaState(delta)

	if len(trimmed) != 3 {
		t.Errorf("len = %d, want 3", len(trimmed))
	}
	if trimmed["app:keep"] != "a" {
		t.Errorf("app:keep = %v, want 'a'", trimmed["app:keep"])
	}
	if trimmed["user:keep"] != "u" {
		t.Errorf("user:keep = %v, want 'u'", trimmed["user:keep"])
	}
	if trimmed["session:keep"] != "s" {
		t.Errorf("session:keep = %v, want 's'", trimmed["session:keep"])
	}
	if _, ok := trimmed["temp:remove1"]; ok {
		t.Error("temp:remove1 should be removed")
	}
	if _, ok := trimmed["temp:remove2"]; ok {
		t.Error("temp:remove2 should be removed")
	}
}

func TestTrimTempDeltaStateNoTemp(t *testing.T) {
	delta := map[string]any{"key1": "val1", "key2": "val2"}
	trimmed := trimTempDeltaState(delta)
	if len(trimmed) != 2 {
		t.Errorf("len = %d, want 2 (no temp keys to remove)", len(trimmed))
	}
}

func TestTrimTempDeltaStateEmpty(t *testing.T) {
	delta := map[string]any{}
	trimmed := trimTempDeltaState(delta)
	if len(trimmed) != 0 {
		t.Errorf("len = %d, want 0", len(trimmed))
	}
}

// ---------------------------------------------------------------------------
// Service: app-state sharing
// ---------------------------------------------------------------------------

func TestServiceAppStateShared(t *testing.T) {
	svc := NewService()

	s1, _ := svc.Create("myapp", "user_a", "s1", nil)
	_, _ = svc.Create("myapp", "user_b", "s2", nil)

	ev := event.NewEvent("ev1", "agent", event.RoleModel)
	ev.Actions.StateDelta = map[string]any{"app:theme": "corp_blue"}

	svc.AppendEvent(s1, ev)

	merged1, _ := svc.GetMergedState("myapp", "user_a", "s1")
	merged2, _ := svc.GetMergedState("myapp", "user_b", "s2")

	if merged1["app:theme"] != "corp_blue" {
		t.Errorf("user_a merged: app:theme = %v, want 'corp_blue'", merged1["app:theme"])
	}
	if merged2["app:theme"] != "corp_blue" {
		t.Errorf("user_b merged: app:theme = %v, want 'corp_blue' (shared across users)", merged2["app:theme"])
	}
}

// ---------------------------------------------------------------------------
// Service: user-state scoping
// ---------------------------------------------------------------------------

func TestServiceUserStateScoped(t *testing.T) {
	svc := NewService()

	s1, _ := svc.Create("myapp", "user_a", "s1", nil)
	s2, _ := svc.Create("myapp", "user_b", "s2", nil)

	// Set user: preference for user_a
	ev1 := event.NewEvent("ev1", "agent", event.RoleModel)
	ev1.Actions.StateDelta = map[string]any{"user:lang": "fr"}
	svc.AppendEvent(s1, ev1)

	// Set user: preference for user_b
	ev2 := event.NewEvent("ev2", "agent", event.RoleModel)
	ev2.Actions.StateDelta = map[string]any{"user:lang": "de"}
	svc.AppendEvent(s2, ev2)

	mergedA, _ := svc.GetMergedState("myapp", "user_a", "s1")
	mergedB, _ := svc.GetMergedState("myapp", "user_b", "s2")

	if mergedA["user:lang"] != "fr" {
		t.Errorf("user_a user:lang = %v, want 'fr'", mergedA["user:lang"])
	}
	if mergedB["user:lang"] != "de" {
		t.Errorf("user_b user:lang = %v, want 'de'", mergedB["user:lang"])
	}
}

func TestServiceUserStateSharedAcrossSessions(t *testing.T) {
	svc := NewService()

	s1, _ := svc.Create("myapp", "user_a", "s1", nil)
	_, _ = svc.Create("myapp", "user_a", "s2", nil)

	ev := event.NewEvent("ev1", "agent", event.RoleModel)
	ev.Actions.StateDelta = map[string]any{"user:pref": "dark"}
	svc.AppendEvent(s1, ev)

	merged2, _ := svc.GetMergedState("myapp", "user_a", "s2")
	if merged2["user:pref"] != "dark" {
		t.Errorf("session s2 user:pref = %v, want 'dark' (shared across user's sessions)", merged2["user:pref"])
	}
}

// ---------------------------------------------------------------------------
// Service: session-state is local
// ---------------------------------------------------------------------------

func TestServiceSessionStateLocal(t *testing.T) {
	svc := NewService()

	s1, _ := svc.Create("myapp", "user_a", "s1", nil)
	_, _ = svc.Create("myapp", "user_a", "s2", nil)

	ev := event.NewEvent("ev1", "agent", event.RoleModel)
	ev.Actions.StateDelta = map[string]any{"local_key": "val1"}
	svc.AppendEvent(s1, ev)

	merged1, _ := svc.GetMergedState("myapp", "user_a", "s1")
	merged2, _ := svc.GetMergedState("myapp", "user_a", "s2")

	if merged1["local_key"] != "val1" {
		t.Errorf("s1 local_key = %v, want 'val1'", merged1["local_key"])
	}
	if _, ok := merged2["local_key"]; ok {
		t.Error("s2 should not have local_key (session scoped)")
	}
}

// ---------------------------------------------------------------------------
// Service: temp state lifecycle — visible during invocation, cleaned after persist
// ---------------------------------------------------------------------------

func TestServiceTempStateCleanupAfterAppend(t *testing.T) {
	svc := NewService()
	sess, _ := svc.Create("myapp", "user_a", "s1", nil)

	// Build an event with a temp: key in the StateDelta.
	ev := event.NewEvent("ev1", "agent", event.RoleModel)
	ev.Actions.StateDelta = map[string]any{
		"temp:scratch": "visible_now",
		"durable":      "stays",
	}

	svc.AppendEvent(sess, ev)

	// Check session state directly — temp key should NOT be present after AppendEvent.
	if _, ok := sess.State().Get("temp:scratch"); ok {
		t.Error("temp:scratch should be cleaned from durable session state after AppendEvent")
	}

	// Durable key should still be present.
	if v, ok := sess.State().Get("durable"); !ok || v != "stays" {
		t.Errorf("durable = %v, ok=%v, want 'stays', true", v, ok)
	}

	// Check the persisted event — temp key should NOT be in StateDelta.
	events := sess.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if _, ok := events[0].Actions.StateDelta["temp:scratch"]; ok {
		t.Error("temp:scratch should NOT be in the persisted event's StateDelta")
	}
	if events[0].Actions.StateDelta["durable"] != "stays" {
		t.Error("durable key should remain in persisted event's StateDelta")
	}
}

func TestServiceTempStateNotInMergedView(t *testing.T) {
	svc := NewService()
	sess, _ := svc.Create("myapp", "user_a", "s1", nil)

	ev := event.NewEvent("ev1", "agent", event.RoleModel)
	ev.Actions.StateDelta = map[string]any{
		"temp:cache": "tmp",
	}
	svc.AppendEvent(sess, ev)

	// Merged state should NOT have temp: key (GetMergedState reads from app/user/session stores)
	merged, _ := svc.GetMergedState("myapp", "user_a", "s1")
	if _, ok := merged["temp:cache"]; ok {
		t.Error("temp:cache should not appear in merged state (not in app/user stores)")
	}
}

// ---------------------------------------------------------------------------
// Service: partial event guard
// ---------------------------------------------------------------------------

func TestServicePartialEventDoesNotPersist(t *testing.T) {
	svc := NewService()
	sess, _ := svc.Create("myapp", "user_a", "s1", nil)

	// Partial event with state delta — should NOT mutate state.
	partialEv := &event.Event{
		ID:      "pe1",
		Partial: true,
		Actions: event.EventActions{
			StateDelta: map[string]any{"app:conf": "should_not_stick"},
		},
	}
	if err := svc.AppendEvent(sess, partialEv); err != nil {
		t.Fatalf("AppendEvent(partial) error = %v (should be silently dropped)", err)
	}

	if sess.EventCount() != 0 {
		t.Errorf("EventCount = %d, want 0 (partial not persisted)", sess.EventCount())
	}

	merged, _ := svc.GetMergedState("myapp", "user_a", "s1")
	if _, ok := merged["app:conf"]; ok {
		t.Error("partial event's state delta should not mutate durable state")
	}
}

func TestServicePartialEventCannotMutateState(t *testing.T) {
	svc := NewService()
	sess, _ := svc.Create("myapp", "user_a", "s1", nil)

	// First, set some durable state.
	durableEv := event.NewEvent("ev1", "agent", event.RoleModel)
	durableEv.Actions.StateDelta = map[string]any{"app:version": "1.0", "user:pref": "light"}
	svc.AppendEvent(sess, durableEv)

	// Partial event tries to overwrite — should not persist.
	partialEv := &event.Event{
		ID:      "pe1",
		Partial: true,
		Actions: event.EventActions{
			StateDelta: map[string]any{"app:version": "666"},
		},
	}
	svc.AppendEvent(sess, partialEv)

	merged, _ := svc.GetMergedState("myapp", "user_a", "s1")
	if merged["app:version"] != "1.0" {
		t.Errorf("app:version = %v, want '1.0' (partial event must not mutate)", merged["app:version"])
	}
}

// ---------------------------------------------------------------------------
// Service: delete/tombstone semantics
// ---------------------------------------------------------------------------

func TestServiceTombstoneDeleteHidesSharedState(t *testing.T) {
	svc := NewService()
	sess, _ := svc.Create("myapp", "user_a", "s1", nil)

	// Set app-level and user-level state.
	ev := event.NewEvent("ev1", "agent", event.RoleModel)
	ev.Actions.StateDelta = map[string]any{
		"app:env":  "prod",
		"user:env": "user_env",
	}
	svc.AppendEvent(sess, ev)

	// Tombstone the env key in session state.
	ev2 := event.NewEvent("ev2", "agent", event.RoleModel)
	ev2.Actions.StateDelta = map[string]any{"env": TombstoneValue}
	svc.AppendEvent(sess, ev2)

	merged, _ := svc.GetMergedState("myapp", "user_a", "s1")
	if _, ok := merged["app:env"]; ok {
		t.Error("app:env should be hidden by session tombstone")
	}
	if _, ok := merged["user:env"]; ok {
		t.Error("user:env should be hidden by session tombstone")
	}
	if merged["env"] != TombstoneValue {
		t.Errorf("env = %v, want TombstoneValue in session layer", merged["env"])
	}
}

func TestServiceTombstoneDeleteSessionOnly(t *testing.T) {
	svc := NewService()
	sess, _ := svc.Create("myapp", "user_a", "s1", nil)

	ev := event.NewEvent("ev1", "agent", event.RoleModel)
	ev.Actions.StateDelta = map[string]any{"key": "val"}
	svc.AppendEvent(sess, ev)

	ev2 := event.NewEvent("ev2", "agent", event.RoleModel)
	ev2.Actions.StateDelta = map[string]any{"key": TombstoneValue}
	svc.AppendEvent(sess, ev2)

	merged, _ := svc.GetMergedState("myapp", "user_a", "s1")
	if merged["key"] != TombstoneValue {
		t.Errorf("key = %v, want TombstoneValue", merged["key"])
	}
}

// ---------------------------------------------------------------------------
// Service: Create with initial state
// ---------------------------------------------------------------------------

func TestServiceCreateWithInitialState(t *testing.T) {
	svc := NewService()
	sess, err := svc.Create("myapp", "user_a", "s1", map[string]any{
		"app:env":    "init_prod",
		"user:pref":  "init_dark",
		"session_key": "init_s",
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	merged, _ := svc.GetMergedState("myapp", "user_a", "s1")
	if merged["app:env"] != "init_prod" {
		t.Errorf("app:env = %v, want 'init_prod'", merged["app:env"])
	}
	if merged["user:pref"] != "init_dark" {
		t.Errorf("user:pref = %v, want 'init_dark'", merged["user:pref"])
	}
	if v, ok := sess.State().Get("session_key"); !ok || v != "init_s" {
		t.Errorf("session_key = %v, ok=%v, want 'init_s', true", v, ok)
	}
}

// ---------------------------------------------------------------------------
// Service: duplicate session create fails
// ---------------------------------------------------------------------------

func TestServiceCreateDuplicateFails(t *testing.T) {
	svc := NewService()
	_, err := svc.Create("myapp", "user_a", "s1", nil)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err = svc.Create("myapp", "user_a", "s1", nil)
	if err == nil {
		t.Error("expected error for duplicate session ID")
	}
}

// ---------------------------------------------------------------------------
// Service: GetOrCreate
// ---------------------------------------------------------------------------

func TestServiceGetOrCreate(t *testing.T) {
	svc := NewService()

	s1, err := svc.GetOrCreate("myapp", "user_a", "s1")
	if err != nil {
		t.Fatalf("first GetOrCreate: %v", err)
	}
	if s1.ID() != "s1" {
		t.Errorf("ID = %q, want 's1'", s1.ID())
	}

	s2, err := svc.GetOrCreate("myapp", "user_a", "s1")
	if err != nil {
		t.Fatalf("second GetOrCreate: %v", err)
	}
	if s2.ID() != "s1" {
		t.Errorf("ID = %q, want same session 's1'", s2.ID())
	}
}

// ---------------------------------------------------------------------------
// Service: app-state isolation between apps
// ---------------------------------------------------------------------------

func TestServiceAppStateIsolation(t *testing.T) {
	svc := NewService()

	s1, _ := svc.Create("app_a", "user_x", "s1", nil)
	_, _ = svc.Create("app_b", "user_x", "s2", nil)

	ev := event.NewEvent("ev1", "agent", event.RoleModel)
	ev.Actions.StateDelta = map[string]any{"app:secret": "app_a_secret"}
	svc.AppendEvent(s1, ev)

	merged2, _ := svc.GetMergedState("app_b", "user_x", "s2")
	if _, ok := merged2["app:secret"]; ok {
		t.Error("app_b session should not see app_a's app:secret")
	}
}

// ---------------------------------------------------------------------------
// Service: DeleteSession
// ---------------------------------------------------------------------------

func TestServiceDeleteSession(t *testing.T) {
	svc := NewService()
	svc.Create("myapp", "user_a", "s1", nil)

	svc.DeleteSession("myapp", "user_a", "s1")

	_, err := svc.Get("myapp", "user_a", "s1")
	if err == nil {
		t.Error("expected error after session deletion")
	}
}

// ---------------------------------------------------------------------------
// Service: state merge overlay order (app < user < session)
// ---------------------------------------------------------------------------

func TestServiceStateMergeOverlayOrder(t *testing.T) {
	svc := NewService()
	s1, _ := svc.Create("myapp", "user_a", "s1", nil)
	s2, _ := svc.Create("myapp", "user_a", "s2", nil)

	// Set app-level value via s1.
	ev1 := event.NewEvent("ev1", "agent", event.RoleModel)
	ev1.Actions.StateDelta = map[string]any{"app:env": "app_env"}
	svc.AppendEvent(s1, ev1)

	// Set user-level with same key via s1.
	ev2 := event.NewEvent("ev2", "agent", event.RoleModel)
	ev2.Actions.StateDelta = map[string]any{"user:env": "user_env"}
	svc.AppendEvent(s1, ev2)

	// Set session-level with same key via s2.
	ev3 := event.NewEvent("ev3", "agent", event.RoleModel)
	ev3.Actions.StateDelta = map[string]any{"env": "session_env"}
	svc.AppendEvent(s2, ev3)

	// s2 should see: session env wins, app:env and user:env still exist.
	merged2, _ := svc.GetMergedState("myapp", "user_a", "s2")
	if merged2["env"] != "session_env" {
		t.Errorf("env = %v, want 'session_env' (session priority)", merged2["env"])
	}
	if merged2["app:env"] != "app_env" {
		t.Errorf("app:env = %v, want 'app_env'", merged2["app:env"])
	}
	if merged2["user:env"] != "user_env" {
		t.Errorf("user:env = %v, want 'user_env'", merged2["user:env"])
	}

	// s1 should see: no session env, app:env and user:env present.
	merged1, _ := svc.GetMergedState("myapp", "user_a", "s1")
	if _, ok := merged1["env"]; ok {
		t.Error("s1 should not have session-level 'env'")
	}
}

// ---------------------------------------------------------------------------
// Standalone session (no service): temp state handling
// ---------------------------------------------------------------------------

func TestStandaloneSessionTempStateHandling(t *testing.T) {
	sess := NewInMemorySession("s1", "myapp", "user1")

	ev := event.NewEvent("ev1", "agent", event.RoleModel)
	ev.Actions.StateDelta = map[string]any{
		"temp:scratch": "tmp_val",
		"real":         "real_val",
	}

	if err := sess.AppendEvent(ev); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	// Temp should NOT be visible in session state after AppendEvent.
	if _, ok := sess.State().Get("temp:scratch"); ok {
		t.Error("temp:scratch should be cleaned from durable session state after AppendEvent")
	}

	// But NOT in the persisted event's StateDelta.
	events := sess.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if _, ok := events[0].Actions.StateDelta["temp:scratch"]; ok {
		t.Error("temp:scratch should not be in persisted StateDelta")
	}
	if events[0].Actions.StateDelta["real"] != "real_val" {
		t.Error("real should remain in persisted StateDelta")
	}
}

// ---------------------------------------------------------------------------
// Standalone session (no service): partial event guard
// ---------------------------------------------------------------------------

func TestStandaloneSessionPartialEventStateGuard(t *testing.T) {
	sess := NewInMemorySession("s1", "myapp", "user1")
	sess.State().Set("key", "original")

	partialEv := &event.Event{
		ID:      "pe1",
		Partial: true,
		Actions: event.EventActions{
			StateDelta: map[string]any{"key": "overwritten"},
		},
	}

	// Standalone sessions reject partial events with an error.
	_ = sess.AppendEvent(partialEv) // error expected

	v, _ := sess.State().Get("key")
	if v != "original" {
		t.Errorf("key = %v, want 'original' (partial must not mutate state)", v)
	}
	if sess.EventCount() != 0 {
		t.Error("EventCount should be 0; partial events are not persisted")
	}
}
