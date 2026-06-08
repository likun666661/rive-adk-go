package memory

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/session"
)

// InMemoryService returns a new thread-safe in-memory memory service.
func InMemoryService() Service {
	return &inMemoryService{
		store: make(map[appUserKey]map[sessionID][]entryValue),
	}
}

type appUserKey struct {
	appName, userID string
}

type sessionID string

type entryValue struct {
	id             string
	content        *event.Content
	author         string
	timestamp      string
	customMetadata map[string]any
	words          map[string]struct{}
}

type inMemoryService struct {
	mu    sync.RWMutex
	store map[appUserKey]map[sessionID][]entryValue
}

func (s *inMemoryService) AddSessionToMemory(ctx context.Context, curSession session.Session) error {
	var values []entryValue

	for _, ev := range curSession.Events() {
		if ev.Partial {
			continue
		}
		if ev.Content == nil {
			continue
		}

		words := make(map[string]struct{})
		for _, part := range ev.Content.Parts {
			if part.Text == "" {
				continue
			}
			for _, w := range strings.Fields(part.Text) {
				words[strings.ToLower(w)] = struct{}{}
			}
		}

		if len(words) == 0 {
			continue
		}

		values = append(values, entryValue{
			id:             ev.ID,
			content:        ev.Content,
			author:         ev.Author,
			timestamp:      ev.Timestamp.Format("2006-01-02T15:04:05Z"),
			customMetadata: nil,
			words:          words,
		})
	}

	key := appUserKey{
		appName: curSession.AppName(),
		userID:  curSession.UserID(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.store[key]
	if !ok {
		v = make(map[sessionID][]entryValue)
		s.store[key] = v
	}

	v[sessionID(curSession.ID())] = values
	return nil
}

func (s *inMemoryService) SearchMemory(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	queryWords := make(map[string]struct{})
	for _, w := range strings.Fields(req.Query) {
		queryWords[strings.ToLower(w)] = struct{}{}
	}

	key := appUserKey{
		appName: req.AppName,
		userID:  req.UserID,
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionsByKey, ok := s.store[key]
	if !ok {
		return &SearchResponse{}, nil
	}

	var memories []Entry
	for _, events := range sessionsByKey {
		for _, e := range events {
			if wordsIntersect(e.words, queryWords) {
				memories = append(memories, Entry{
					ID:             e.id,
					Content:        e.content,
					Author:         e.author,
					Timestamp:      curSessionTimestamp(e.timestamp),
					CustomMetadata: e.customMetadata,
				})
			}
		}
	}

	return &SearchResponse{Memories: memories}, nil
}

func wordsIntersect(m1, m2 map[string]struct{}) bool {
	if len(m1) == 0 || len(m2) == 0 {
		return false
	}
	if len(m1) > len(m2) {
		m1, m2 = m2, m1
	}
	for k := range m1 {
		if _, ok := m2[k]; ok {
			return true
		}
	}
	return false
}

func curSessionTimestamp(s string) time.Time {
	t, _ := time.Parse("2006-01-02T15:04:05Z", s)
	return t
}
