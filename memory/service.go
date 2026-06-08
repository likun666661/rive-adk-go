// Package memory defines a long-term memory service that ingests session
// events and supports keyword-based search across sessions.
//
// Memory is scoped to (app, user) and survives beyond individual sessions.
// This separation from session state prevents temporary conversational
// details from polluting long-term recall.
package memory

import (
	"context"
	"time"

	"github.com/likun666661/rive-adk-go/event"
	"github.com/likun666661/rive-adk-go/session"
)

// Service ingests sessions into memory and enables keyword search
// across user-scoped sessions.
type Service interface {
	AddSessionToMemory(ctx context.Context, s session.Session) error
	SearchMemory(ctx context.Context, req *SearchRequest) (*SearchResponse, error)
}

// SearchRequest represents a memory search query.
type SearchRequest struct {
	Query   string
	UserID  string
	AppName string
}

// SearchResponse contains matching memory entries.
type SearchResponse struct {
	Memories []Entry
}

// Entry represents a single memory entry extracted from session events.
type Entry struct {
	ID             string
	Content        *event.Content
	Author         string
	Timestamp      time.Time
	CustomMetadata map[string]any
}
