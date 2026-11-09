// Package adapter reads sessions from AI coding tools behind one interface.
package adapter

import "time"

type Turn struct {
	Role string // "user" or "assistant"
	Text string
	At   time.Time
}

type Session struct {
	ID      string
	Source  string
	Project string
	Started time.Time
	Updated time.Time
	Turns   []Turn
}

type Adapter interface {
	Name() string
	FetchSessions(since time.Time) ([]Session, error)
}

// InternalMarker prefixes prompts afterthought sends to an AI so those sessions are never ingested.
const InternalMarker = "[afterthought-internal]"
