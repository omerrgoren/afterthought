package adapter

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Cursor reads chats from Cursor's global state DB (table cursorDiskKV).
// The layout (bubbleId:<composer>:<bubble> rows with type 1=user, 2=assistant,
// composerData:<composer> rows with createdAt) is UNVERIFIED against current
// Cursor releases; the adapter fails soft and returns no sessions on mismatch.
type Cursor struct{ DB string }

func NewCursor() Cursor {
	h, _ := os.UserHomeDir()
	var p string
	switch runtime.GOOS {
	case "darwin":
		p = filepath.Join(h, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb")
	case "windows":
		p = filepath.Join(os.Getenv("APPDATA"), "Cursor", "User", "globalStorage", "state.vscdb")
	default:
		p = filepath.Join(h, ".config", "Cursor", "User", "globalStorage", "state.vscdb")
	}
	return Cursor{DB: p}
}

func (Cursor) Name() string { return "cursor" }

func (c Cursor) FetchSessions(since time.Time) ([]Session, error) {
	if _, err := os.Stat(c.DB); err != nil {
		return nil, nil // Cursor not installed
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&immutable=1", c.DB))
	if err != nil {
		return nil, nil
	}
	defer db.Close()

	created := map[string]time.Time{}
	if rows, err := db.Query(`SELECT key, value FROM cursorDiskKV WHERE key LIKE 'composerData:%'`); err == nil {
		for rows.Next() {
			var k string
			var v []byte
			if rows.Scan(&k, &v) != nil {
				continue
			}
			var d struct {
				CreatedAt int64 `json:"createdAt"`
			}
			if json.Unmarshal(v, &d) == nil && d.CreatedAt > 0 {
				created[strings.TrimPrefix(k, "composerData:")] = time.UnixMilli(d.CreatedAt)
			}
		}
		rows.Close()
	}

	rows, err := db.Query(`SELECT key, value FROM cursorDiskKV WHERE key LIKE 'bubbleId:%' ORDER BY rowid`)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	byComposer := map[string]*Session{}
	var order []string
	for rows.Next() {
		var k string
		var v []byte
		if rows.Scan(&k, &v) != nil {
			continue
		}
		parts := strings.Split(k, ":")
		if len(parts) < 3 {
			continue
		}
		var b struct {
			Type int    `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(v, &b) != nil || strings.TrimSpace(b.Text) == "" {
			continue
		}
		role := map[int]string{1: "user", 2: "assistant"}[b.Type]
		if role == "" {
			continue
		}
		id := parts[1]
		s := byComposer[id]
		if s == nil {
			s = &Session{ID: id, Source: "cursor", Started: created[id], Updated: created[id]}
			byComposer[id] = s
			order = append(order, id)
		}
		s.Turns = append(s.Turns, Turn{Role: role, Text: b.Text})
	}
	var out []Session
	for _, id := range order {
		s := byComposer[id]
		if !s.Started.IsZero() && s.Started.Before(since) {
			continue
		}
		if strings.HasPrefix(s.Turns[0].Text, InternalMarker) {
			continue
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.Before(out[j].Started) })
	return out, nil
}
