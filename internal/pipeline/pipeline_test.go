package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/omerrgoren/afterthought/internal/adapter"
	"github.com/omerrgoren/afterthought/internal/config"
	"github.com/omerrgoren/afterthought/internal/ledger"
	"github.com/omerrgoren/afterthought/internal/roadmap"
)

type fakeLLM struct{ calls int }

func (f *fakeLLM) Run(_ context.Context, prompt string, web bool) (string, error) {
	f.calls++
	if web {
		return "=== TOPIC: readiness vs liveness ===\n### What is X\nx\n### What is Y\ny\n### Where to use each\nz\n### Best practices\nb\n### Interesting articles and official docs\n- https://kubernetes.io/docs/\n- https://dead.invalid/x\n", nil
	}
	return `[{"concept":"readiness vs liveness","category":"k8s","context":"pod restarts","why_it_matters":"avoid outages","understanding_signal":"auto_accepted","importance":4,"difficulty":"hard","times_asked":1,"recap":"r"}]`, nil
}

type fakeAdapter struct{ ss []adapter.Session }

func (fakeAdapter) Name() string                                         { return "fake" }
func (f fakeAdapter) FetchSessions(time.Time) ([]adapter.Session, error) { return f.ss, nil }

func setup(t *testing.T, sessions ...adapter.Session) (Deps, *fakeLLM, *[]string) {
	roadmap.LinkOK = func(u string) bool { return !strings.Contains(u, "dead.invalid") }
	l, err := ledger.Open(filepath.Join(t.TempDir(), "l.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	f := &fakeLLM{}
	var notes []string
	// Friday 2026-11-06 14:00 local
	now := time.Date(2026, 11, 6, 14, 0, 0, 0, time.Local)
	return Deps{Cfg: config.Default(), L: l, R: f, Adapters: []adapter.Adapter{fakeAdapter{sessions}}, Now: now,
		RoadmapDir: t.TempDir(), Notify: func(a, b string) { notes = append(notes, b) }}, f, &notes
}

func sess(id, project string) adapter.Session {
	return adapter.Session{ID: id, Source: "fake", Project: project, Turns: []adapter.Turn{
		{Role: "user", Text: "why do pods restart?"}, {Role: "assistant", Text: "liveness probe failed"}}}
}

func TestFridayGateOneFileNeverTwiceAndTwoNotifications(t *testing.T) {
	d, _, notes := setup(t, sess("s1", "/a"))
	res, _ := Run(context.Background(), d)
	if !res.Created || res.Concepts != 1 {
		t.Fatalf("%+v", res)
	}
	b, _ := os.ReadFile(res.Roadmap)
	md := string(b)
	if !strings.Contains(md, "This week's hot topics") || !strings.Contains(md, "readiness vs liveness") || !strings.Contains(md, "kubernetes.io") {
		t.Fatal(md)
	}
	if strings.Contains(md, "dead.invalid") {
		t.Fatal("dead link shipped")
	}
	res2, _ := Run(context.Background(), d)
	if res2.Created || len(*notes) != 1 {
		t.Fatalf("second run created=%v notes=%d", res2.Created, len(*notes))
	}
	d.Now = d.Now.Add(2 * time.Hour)
	Run(context.Background(), d)
	Run(context.Background(), d)
	if len(*notes) != 2 {
		t.Fatalf("want exactly 2 notifications, got %d", len(*notes))
	}
}

func TestBeforeFridayNoFileForNewWeekBoundary(t *testing.T) {
	d, _, _ := setup(t)
	d.Now = time.Date(2026, 11, 6, 12, 0, 0, 0, time.Local) // Friday 12:00: boundary is previous Friday
	if got := roadmap.WeekKey(roadmap.Boundary(d.Now)); got != "2026-10-30" {
		t.Fatal(got)
	}
}

func TestQuietWeekIsNeverEmpty(t *testing.T) {
	d, _, _ := setup(t)
	d.Cfg.Topics = []string{"networking"} // everything below is out of scope -> parked
	d.Adapters = []adapter.Adapter{fakeAdapter{[]adapter.Session{sess("s1", "/a")}}}
	res, _ := Run(context.Background(), d)
	b, _ := os.ReadFile(res.Roadmap)
	if !strings.Contains(string(b), "parking lot") || !strings.Contains(string(b), "never asked about") {
		t.Fatal(string(b))
	}
}

func TestEmptyLedgerStillWritesFile(t *testing.T) {
	d, _, _ := setup(t)
	res, _ := Run(context.Background(), d)
	b, err := os.ReadFile(res.Roadmap)
	if err != nil || !strings.Contains(string(b), "hot topics") {
		t.Fatal(err)
	}
}

func TestIgnoredProjectProducesNothing(t *testing.T) {
	d, f, _ := setup(t, sess("s1", "/secret/proj"))
	d.Cfg.Ignore = []string{"/secret"}
	Run(context.Background(), d)
	if c, _, _ := d.L.Counts(); c != 0 || f.calls > 0 {
		t.Fatalf("ignored project leaked: concepts=%d llmcalls=%d", c, f.calls)
	}
}

func TestPausedDoesNothing(t *testing.T) {
	d, f, _ := setup(t, sess("s1", "/a"))
	d.Cfg.Paused = true
	res, _ := Run(context.Background(), d)
	if res.Created || f.calls > 0 {
		t.Fatal("paused run did work")
	}
}

func TestPreviewNeverCallsAI(t *testing.T) {
	s := sess("s1", "/a")
	s.Turns[0].Text = "my key sk-abcdefghijklmnop1234"
	d, f, _ := setup(t, s)
	p, _ := Preview(d, time.Time{})
	if f.calls != 0 || len(p) != 1 {
		t.Fatal("preview called AI")
	}
	for _, v := range p {
		if strings.Contains(v, "sk-abc") {
			t.Fatal("secret in preview")
		}
	}
}
