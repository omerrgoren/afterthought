package extract

import (
	"context"
	"testing"
)

func TestParseToleratesChatter(t *testing.T) {
	cs, err := Parse("Sure!\n```json\n[{\"concept\":\" iptables NAT \",\"signal\":\"x\",\"importance\":9,\"difficulty\":\"weird\",\"understanding_signal\":\"asked\"},{\"concept\":\"\"}]\n```")
	if err != nil || len(cs) != 1 {
		t.Fatal(cs, err)
	}
	c := cs[0]
	if c.Name != "iptables NAT" || c.Importance != 3 || c.Difficulty != "mid" || c.TimesAsked != 1 {
		t.Fatalf("%+v", c)
	}
	if _, err := Parse("no array"); err == nil {
		t.Fatal("expected error")
	}
}

type seqRunner struct {
	replies []string
	calls   int
}

func (s *seqRunner) Run(_ context.Context, _ string, _ bool) (string, error) {
	r := s.replies[s.calls]
	s.calls++
	return r, nil
}

func TestConceptsRetriesOnProse(t *testing.T) {
	r := &seqRunner{replies: []string{"I looked at the session and...", `[{"concept":"preStop hook","understanding_signal":"asked"}]`}}
	cs, err := Concepts(context.Background(), r, "USER: x", nil, "")
	if err != nil || len(cs) != 1 || r.calls != 2 {
		t.Fatal(cs, err, r.calls)
	}
}
