// Package session defines session state and append-only event history.
//
// A session ties a user conversation to an agent tree over time.
// Events are appended as the invocation progresses; partial events
// are consumed in-flight but not persisted.
//
// State is a flat key-value store with deep-merge semantics for deltas.
package session

import (
	"fmt"
	"sync"

	"github.com/likun666661/rive-adk-go/event"
)

// State is a mutable key-value store scoped to a session.
// Keys use prefixes: "app:", "user:", "temp:".

// TODO: later node — implement prefix-scoped state access and template injection.
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

type inMemorySession struct {
	mu      sync.RWMutex
	id      string
	appName string
	userID  string
	state   *stateImpl
	events  []*event.Event
}

func (s *inMemorySession) ID() string     { return s.id }
func (s *inMemorySession) AppName() string  { return s.appName }
func (s *inMemorySession) UserID() string   { return s.userID }
func (s *inMemorySession) State() State     { return s.state }
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return nil
}

// MergeStateDelta applies event actions' state deltas to the session state.
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
