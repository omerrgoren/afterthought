package ledger

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/omerrgoren/afterthought/internal/config"
	"github.com/omerrgoren/afterthought/internal/extract"
)

func open(t *testing.T) *Ledger {
	l, err := Open(filepath.Join(t.TempDir(), "l.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

var now = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

func TestReaskedConceptIsPromoted(t *testing.T) {
	l := open(t)
	cfg := config.Default()
	cfg.Topics = []string{"networking"} // concept below is out of scope
	c := extract.Concept{Name: "Interpolation vs exact", Category: "db", Signal: "asked", Difficulty: "hard", Importance: 3, TimesAsked: 1}
	l.Upsert(cfg, c, "s1", now)
	if p, _ := l.Parked(now); len(p) != 1 {
		t.Fatal("out-of-scope concept should be parked")
	}
	c.TimesAsked = 3
	l.Upsert(cfg, c, "s2", now)
	f, _ := l.Fresh(now)
	if len(f) != 1 || f[0].Reasked < 2 {
		t.Fatalf("expected promotion, got %+v", f)
	}
}

func TestDepthParksEasyConcepts(t *testing.T) {
	l := open(t)
	cfg := config.Default()
	cfg.Depth = "hard"
	l.Upsert(cfg, extract.Concept{Name: "ls flags", Signal: "asked", Difficulty: "easy", Importance: 1}, "s", now)
	if p, _ := l.Parked(now); len(p) != 1 {
		t.Fatal("below-depth concept should be parked")
	}
}

func TestKnownCanFadeAndResurface(t *testing.T) {
	l := open(t)
	cfg := config.Default()
	c := extract.Concept{Name: "DNS resolver", Signal: "knows", Difficulty: "mid", Importance: 3}
	l.Upsert(cfg, c, "s1", now)
	if f, _ := l.Fresh(now); len(f) != 0 {
		t.Fatal("known concept should not be listed")
	}
	c.Signal = "asked"
	l.Upsert(cfg, c, "s2", now.Add(time.Hour))
	if f, _ := l.Fresh(now); len(f) != 1 {
		t.Fatal("re-asked known concept should come back")
	}
}

func TestListedConceptsNotRepeatedUntilResurfaced(t *testing.T) {
	l := open(t)
	cfg := config.Default()
	c := extract.Concept{Name: "race condition", Signal: "asked", Difficulty: "mid", Importance: 4, TimesAsked: 1}
	l.Upsert(cfg, c, "s1", now)
	f, _ := l.Fresh(now)
	l.MarkListed([]int64{f[0].ID}, "2026-11-06")
	if f, _ := l.Fresh(now); len(f) != 0 {
		t.Fatal("listed concept repeated")
	}
	l.Upsert(cfg, c, "s2", now)
	if f, _ := l.Fresh(now); len(f) != 1 || !f[0].Resurfaced {
		t.Fatal("resurfaced concept should return flagged")
	}
}
