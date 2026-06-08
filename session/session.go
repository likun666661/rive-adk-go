// Package session defines session state and append-only event history.
//
// A session ties a user conversation to an agent tree over time.
// Events are appended as the invocation progresses; partial events
// are consumed in-flight but not persisted.
//
// State supports four scopes via key prefixes:
//
//	"app:"  — shared by all users and sessions within the same app.
//	"user:" — shared across all sessions for the same user within the app.
//	"temp:" — visible only during the current invocation, never persisted.
//	(no prefix) — scoped to the individual session.
//
// When reading state the layers are overlaid in order: app < user < session,
// so session-level keys take priority.
package session

import (
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/likun666661/rive-adk-go/event"
)

// Key prefixes for state scope routing.
const (
	KeyPrefixApp  = "app:"
	KeyPrefixUser = "user:"
	KeyPrefixTemp = "temp:"
)

// TombstoneValue is a sentinel used to mark a key as deleted in a state delta.
// When merged, a tombstone in a higher layer hides the key from lower layers.
const TombstoneValue = "__STATE_TOMBSTONE__"

// State is a mutable key-value store scoped to a session.
type State interface {
	Get(key string) (any, bool)
	Set(key string, value any)
	Delete(key string)
	All() map[string]any
}

// ReadonlyState provides read-only access to session state.
type ReadonlyState interface {
	Get(key string) (any, bool)
	All() map[string]any
}

type stateImpl struct {
	mu   sync.RWMutex
	data map[string]any
}

func newState() *stateImpl {
	return &stateImpl{data: make(map[string]any)}
}

func (s *stateImpl) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *stateImpl) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

func (s *stateImpl) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

func (s *stateImpl) All() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cpy := make(map[string]any, len(s.data))
	for k, v := range s.data {
		cpy[k] = v
	}
	return cpy
}

// Session ties together a user conversation with an agent tree over time.
type Session interface {
	ID() string
	AppName() string
	UserID() string
	State() State
	Events() []*event.Event
	AppendEvent(ev *event.Event) error
	EventCount() int
}

// NewInMemorySession creates a session backed by in-memory storage.
func NewInMemorySession(id, appName, userID string) Session {
	return &inMemorySession{
		id:      id,
		appName: appName,
		userID:  userID,
		state:   newState(),
		events:  make([]*event.Event, 0),
	}
}

// newInMemorySessionWithService creates a session owned by a Service.
// The service reference enables cross-session app/user state sharing.
func newInMemorySessionWithService(id, appName, userID string, svc *Service) *inMemorySession {
	return &inMemorySession{
		id:      id,
		appName: appName,
		userID:  userID,
		state:   newState(),
		events:  make([]*event.Event, 0),
		svc:     svc,
	}
}

type inMemorySession struct {
	mu      sync.RWMutex
	id      string
	appName string
	userID  string
	state   *stateImpl
	events  []*event.Event
	svc     *Service
}

func (s *inMemorySession) ID() string    { return s.id }
func (s *inMemorySession) AppName() string { return s.appName }
func (s *inMemorySession) UserID() string  { return s.userID }
func (s *inMemorySession) State() State    { return s.state }
func (s *inMemorySession) EventCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}

func (s *inMemorySession) Events() []*event.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cpy := make([]*event.Event, len(s.events))
	copy(cpy, s.events)
	return cpy
}

func (s *inMemorySession) AppendEvent(ev *event.Event) error {
	if ev == nil {
		return fmt.Errorf("session: cannot append nil event")
	}
	if ev.Partial {
		return fmt.Errorf("session: cannot append partial event")
	}

	if s.svc != nil {
		return s.svc.AppendEvent(s, ev)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(ev.Actions.StateDelta) > 0 {
		mergeStateDeltaIntoState(s.state, ev.Actions.StateDelta)
		ev.Actions.StateDelta = trimTempDeltaState(ev.Actions.StateDelta)
		removeTempKeysFromState(s.state)
	}

	s.events = append(s.events, ev)
	return nil
}

// MergeStateDelta applies event actions' state deltas to the given State.
//
// Values are deep-merged: if both existing and incoming values are
// map[string]any, the maps are merged recursively. Otherwise the incoming
// value overwrites.
func MergeStateDelta(s State, delta map[string]any) {
	if delta == nil {
		return
	}
	for k, v := range delta {
		if existing, ok := s.Get(k); ok {
			existingMap, existingIsMap := existing.(map[string]any)
			incomingMap, incomingIsMap := v.(map[string]any)
			if existingIsMap && incomingIsMap {
				deepMergeMap(existingMap, incomingMap)
				continue
			}
		}
		s.Set(k, v)
	}
}

func deepMergeMap(dst, src map[string]any) {
	for k, v := range src {
		if dstMap, dstIsMap := dst[k].(map[string]any); dstIsMap {
			if srcMap, srcIsMap := v.(map[string]any); srcIsMap {
				deepMergeMap(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
}

// mergeStateDeltaIntoState applies a delta directly to a stateImpl.
func mergeStateDeltaIntoState(st *stateImpl, delta map[string]any) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for k, v := range delta {
		if existing, ok := st.data[k]; ok {
			existingMap, existingIsMap := existing.(map[string]any)
			incomingMap, incomingIsMap := v.(map[string]any)
			if existingIsMap && incomingIsMap {
				deepMergeMap(existingMap, incomingMap)
				continue
			}
		}
		st.data[k] = v
	}
}

// ExtractStateDeltas splits a state delta by scope prefix.
//
// Keys with "app:" prefix → appDelta (prefix stripped).
// Keys with "user:" prefix → userDelta (prefix stripped).
// Keys with "temp:" prefix are silently dropped.
// All other keys → sessionDelta.
func ExtractStateDeltas(delta map[string]any) (appDelta, userDelta, sessionDelta map[string]any) {
	appDelta = make(map[string]any)
	userDelta = make(map[string]any)
	sessionDelta = make(map[string]any)

	if delta == nil {
		return
	}

	for key, value := range delta {
		if clean, ok := strings.CutPrefix(key, KeyPrefixApp); ok {
			appDelta[clean] = value
		} else if clean, ok := strings.CutPrefix(key, KeyPrefixUser); ok {
			userDelta[clean] = value
		} else if !strings.HasPrefix(key, KeyPrefixTemp) {
			sessionDelta[key] = value
		}
	}
	return
}

// MergeStates overlays app, user, and session state into a single map.
//
// Overlay order: session (highest priority) → user → app (lowest).
// App-level keys are prefixed with "app:", user-level with "user:".
// Session-level keys are stored without a prefix.
// Tombstone values in session state hide the corresponding
// app: and user: keys from the merged result.
func MergeStates(appState, userState, sessionState map[string]any) map[string]any {
	totalSize := len(appState) + len(userState) + len(sessionState)
	merged := make(map[string]any, totalSize)

	tombstoned := make(map[string]bool)
	for k, v := range sessionState {
		if isTombstone(v) {
			tombstoned[k] = true
		}
	}

	for k, v := range appState {
		if !tombstoned[k] && !isTombstone(v) {
			merged[KeyPrefixApp+k] = v
		}
	}

	for k, v := range userState {
		if !tombstoned[k] && !isTombstone(v) {
			merged[KeyPrefixUser+k] = v
		}
	}

	for k, v := range sessionState {
		merged[k] = v
	}

	return merged
}

func isTombstone(v any) bool {
	s, ok := v.(string)
	return ok && s == TombstoneValue
}

// removeTempKeysFromState removes all "temp:" prefixed keys from a stateImpl.
// This ensures invocation-local temp keys do not leak into durable session state
// after the invocation has completed.
func removeTempKeysFromState(st *stateImpl) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for k := range st.data {
		if strings.HasPrefix(k, KeyPrefixTemp) {
			delete(st.data, k)
		}
	}
}

// trimTempDeltaState removes "temp:" prefixed keys from a state delta map
// and returns a new map. The original is not mutated.
func trimTempDeltaState(delta map[string]any) map[string]any {
	if len(delta) == 0 {
		return delta
	}

	hasTemp := false
	for k := range delta {
		if strings.HasPrefix(k, KeyPrefixTemp) {
			hasTemp = true
			break
		}
	}
	if !hasTemp {
		return delta
	}

	filtered := make(map[string]any, len(delta))
	for k, v := range delta {
		if !strings.HasPrefix(k, KeyPrefixTemp) {
			filtered[k] = v
		}
	}
	return filtered
}

// Service manages multiple sessions with cross-cutting (app/user) state.
//
// App-state keys are shared across all users and sessions within an app.
// User-state keys are shared across all sessions for the same user.
// Session-state keys are private to each session.
// Temp-state keys are trimmed during AppendEvent and never persisted.
//
// Thread-safe.
type Service struct {
	mu        sync.RWMutex
	sessions  map[string]*inMemorySession                     // key = app/user/sid
	appState  map[string]map[string]any                        // appName → state
	userState map[string]map[string]map[string]any             // appName → userID → state
}

// NewService creates a new in-memory session Service.
func NewService() *Service {
	return &Service{
		sessions:  make(map[string]*inMemorySession),
		appState:  make(map[string]map[string]any),
		userState: make(map[string]map[string]map[string]any),
	}
}

func (svc *Service) sessionKey(appName, userID, sessionID string) string {
	return appName + "/" + userID + "/" + sessionID
}

// Create creates a new session with optional initial state.
func (svc *Service) Create(appName, userID, sessionID string, initialState map[string]any) (Session, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	key := svc.sessionKey(appName, userID, sessionID)
	if _, exists := svc.sessions[key]; exists {
		return nil, fmt.Errorf("session: session %q already exists", sessionID)
	}

	sess := newInMemorySessionWithService(sessionID, appName, userID, svc)

	if len(initialState) > 0 {
		appDelta, userDelta, _ := ExtractStateDeltas(initialState)
		svc.updateAppState(appDelta, appName)
		svc.updateUserState(userDelta, appName, userID)

		for k, v := range initialState {
			if strings.HasPrefix(k, KeyPrefixTemp) {
				continue
			}
			sess.state.data[k] = v
		}
	}

	svc.sessions[key] = sess
	return sess, nil
}

// Get retrieves a session.
// The session's State() reflects only its local state.
// Call GetMergedState for the full app+user+session merged view.
func (svc *Service) Get(appName, userID, sessionID string) (Session, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	key := svc.sessionKey(appName, userID, sessionID)
	sess, ok := svc.sessions[key]
	if !ok {
		return nil, fmt.Errorf("session: session %q not found", sessionID)
	}

	return sess, nil
}

// GetOrCreate returns an existing session or creates a new one.
func (svc *Service) GetOrCreate(appName, userID, sessionID string) (Session, error) {
	sess, err := svc.Get(appName, userID, sessionID)
	if err == nil {
		return sess, nil
	}
	return svc.Create(appName, userID, sessionID, nil)
}

// AppendEvent persists an event and routes its state deltas by scope.
//
// Partial events are rejected (they are never persisted and cannot mutate
// durable state). Temp-prefix keys in the StateDelta are applied to the
// session state for the current invocation but stripped before the event
// is persisted.
//
// App-prefix and user-prefix keys are routed to the shared app/user state
// stores. Session-state keys (no prefix) are applied to the session state.
func (svc *Service) AppendEvent(sess Session, ev *event.Event) error {
	if sess == nil {
		return fmt.Errorf("session: session is nil")
	}
	if ev == nil {
		return fmt.Errorf("session: event is nil")
	}
	if ev.Partial {
		return nil
	}

	is, ok := sess.(*inMemorySession)
	if !ok {
		return fmt.Errorf("session: unexpected type %T", sess)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()

	key := svc.sessionKey(is.appName, is.userID, is.id)
	if _, exists := svc.sessions[key]; !exists {
		return fmt.Errorf("session: session %q not found in service", is.id)
	}

	if len(ev.Actions.StateDelta) > 0 {
		fullDelta := ev.Actions.StateDelta
		svc.applyStateDelta(is.appName, is.userID, is.state, fullDelta)
		ev.Actions.StateDelta = trimTempDeltaState(fullDelta)
		removeTempKeysFromState(is.state)
	}

	is.mu.Lock()
	is.events = append(is.events, ev)
	is.mu.Unlock()

	return nil
}

// DeleteSession removes a session from the service.
func (svc *Service) DeleteSession(appName, userID, sessionID string) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	delete(svc.sessions, svc.sessionKey(appName, userID, sessionID))
}

// applyStateDelta routes a full (pre-trim) state delta by scope prefix.
// All keys (including temp) are applied to the session state for invocation visibility.
// App and user prefixed keys are routed to shared stores.
// Caller must hold svc.mu.
func (svc *Service) applyStateDelta(appName, userID string, st *stateImpl, delta map[string]any) {
	st.mu.Lock()
	for k, v := range delta {
		st.data[k] = v
	}
	st.mu.Unlock()

	appDelta, userDelta, _ := ExtractStateDeltas(delta)
	svc.updateAppState(appDelta, appName)
	svc.updateUserState(userDelta, appName, userID)
}

func (svc *Service) updateAppState(delta map[string]any, appName string) {
	st, ok := svc.appState[appName]
	if !ok {
		st = make(map[string]any)
		svc.appState[appName] = st
	}
	maps.Copy(st, delta)
}

func (svc *Service) updateUserState(delta map[string]any, appName, userID string) {
	users, ok := svc.userState[appName]
	if !ok {
		users = make(map[string]map[string]any)
		svc.userState[appName] = users
	}
	st, ok := users[userID]
	if !ok {
		st = make(map[string]any)
		users[userID] = st
	}
	maps.Copy(st, delta)
}

func (svc *Service) mergeStatesForSession(sess *inMemorySession) map[string]any {
	appState := svc.appState[sess.appName]
	var userState map[string]any
	if users, ok := svc.userState[sess.appName]; ok {
		userState = users[sess.userID]
	}

	sess.state.mu.RLock()
	sessionState := make(map[string]any, len(sess.state.data))
	for k, v := range sess.state.data {
		if strings.HasPrefix(k, KeyPrefixApp) || strings.HasPrefix(k, KeyPrefixUser) || strings.HasPrefix(k, KeyPrefixTemp) {
			continue
		}
		sessionState[k] = v
	}
	sess.state.mu.RUnlock()

	return MergeStates(appState, userState, sessionState)
}

// GetMergedState returns the fully merged state (app + user + session)
// for a session without replacing the session's internal state.
func (svc *Service) GetMergedState(appName, userID, sessionID string) (map[string]any, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	key := svc.sessionKey(appName, userID, sessionID)
	sess, ok := svc.sessions[key]
	if !ok {
		return nil, fmt.Errorf("session: session %q not found", sessionID)
	}

	return svc.mergeStatesForSession(sess), nil
}
