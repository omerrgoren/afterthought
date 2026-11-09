package adapter

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClaudeCodeParsing(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "-proj")
	os.MkdirAll(dir, 0o755)
	jsonl := `{"type":"permission-mode"}
{"type":"user","sessionId":"s1","cwd":"/work/app","timestamp":"2026-11-09T10:00:00Z","message":{"content":"what is a preStop hook?"}}
{"type":"assistant","timestamp":"2026-11-09T10:00:05Z","message":{"content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"It runs before SIGTERM."},{"type":"tool_use","name":"Bash"}]}}
{"type":"user","timestamp":"2026-11-09T10:00:06Z","message":{"content":[{"type":"tool_result","content":"HUGE OUTPUT"}]}}
not json
{"type":"user","isMeta":true,"message":{"content":"meta"}}
`
	os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(jsonl), 0o644)
	os.WriteFile(filepath.Join(dir, "own.jsonl"), []byte(`{"type":"user","message":{"content":"`+InternalMarker+` extract"}}`+"\n"), 0o644)
	ss, err := ClaudeCode{Root: root}.FetchSessions(time.Time{})
	if err != nil || len(ss) != 1 {
		t.Fatalf("got %d sessions, err %v", len(ss), err)
	}
	s := ss[0]
	if s.Project != "/work/app" || len(s.Turns) != 2 || s.Turns[1].Text != "It runs before SIGTERM." {
		t.Fatalf("%+v", s)
	}
}

func TestCursorFixture(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.vscdb")
	db, _ := sql.Open("sqlite", p)
	db.Exec(`CREATE TABLE cursorDiskKV(key TEXT, value BLOB)`)
	db.Exec(`INSERT INTO cursorDiskKV VALUES('composerData:c1','{"createdAt":1762680000000}')`)
	db.Exec(`INSERT INTO cursorDiskKV VALUES('bubbleId:c1:b1','{"type":1,"text":"explain iptables NAT"}')`)
	db.Exec(`INSERT INTO cursorDiskKV VALUES('bubbleId:c1:b2','{"type":2,"text":"NAT rewrites addresses."}')`)
	db.Close()
	ss, err := Cursor{DB: p}.FetchSessions(time.Time{})
	if err != nil || len(ss) != 1 || len(ss[0].Turns) != 2 || ss[0].Turns[0].Role != "user" {
		t.Fatalf("%v %+v", err, ss)
	}
	if ss2, _ := (Cursor{DB: filepath.Join(t.TempDir(), "missing")}).FetchSessions(time.Time{}); len(ss2) != 0 {
		t.Fatal("missing db should yield nothing")
	}
}
