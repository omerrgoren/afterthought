package adapter

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ClaudeCode reads JSONL sessions under ~/.claude/projects.
type ClaudeCode struct{ Root string }

func NewClaudeCode() ClaudeCode {
	h, _ := os.UserHomeDir()
	return ClaudeCode{Root: filepath.Join(h, ".claude", "projects")}
}

func (ClaudeCode) Name() string { return "claude-code" }

type ccLine struct {
	Type        string `json:"type"`
	IsMeta      bool   `json:"isMeta"`
	IsSidechain bool   `json:"isSidechain"`
	Timestamp   string `json:"timestamp"`
	Cwd         string `json:"cwd"`
	SessionID   string `json:"sessionId"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

func (c ClaudeCode) FetchSessions(since time.Time) ([]Session, error) {
	files, _ := filepath.Glob(filepath.Join(c.Root, "*", "*.jsonl"))
	var out []Session
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil || st.ModTime().Before(since) {
			continue
		}
		s, err := parseClaudeFile(f)
		if err != nil || len(s.Turns) == 0 {
			continue
		}
		s.Updated = st.ModTime()
		if len(s.Turns) > 0 && strings.HasPrefix(s.Turns[0].Text, InternalMarker) {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.Before(out[j].Started) })
	return out, nil
}

func parseClaudeFile(path string) (Session, error) {
	fh, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer fh.Close()
	s := Session{Source: "claude-code", ID: strings.TrimSuffix(filepath.Base(path), ".jsonl")}
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var l ccLine
		if json.Unmarshal(sc.Bytes(), &l) != nil || l.IsMeta || l.IsSidechain {
			continue
		}
		if l.Type != "user" && l.Type != "assistant" {
			continue
		}
		if l.Cwd != "" {
			s.Project = l.Cwd
		}
		text := textOf(l.Message.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		at, _ := time.Parse(time.RFC3339, l.Timestamp)
		if s.Started.IsZero() {
			s.Started = at
		}
		s.Turns = append(s.Turns, Turn{Role: l.Type, Text: text, At: at})
	}
	return s, sc.Err()
}

// textOf keeps plain text and text blocks; tool calls, results and thinking are dropped.
func textOf(raw json.RawMessage) string {
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}
