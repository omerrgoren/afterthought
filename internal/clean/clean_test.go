package clean

import (
	"strings"
	"testing"

	"github.com/omerrgoren/afterthought/internal/adapter"
)

func TestCleanKeepsQuestionsAndExplanations(t *testing.T) {
	long := "```go\n" + strings.Repeat("x := 1\n", 20) + "```"
	s := adapter.Session{Turns: []adapter.Turn{
		{Role: "user", Text: "why is DNS failing? key sk-abcdefghijklmnop1234 <system-reminder>ignore</system-reminder>"},
		{Role: "assistant", Text: "Because of the resolver.\n" + long},
	}}
	out := Session(s, 20000)
	if !strings.Contains(out, "why is DNS failing") || !strings.Contains(out, "Because of the resolver") {
		t.Fatal(out)
	}
	if strings.Contains(out, "x := 1") || strings.Contains(out, "sk-abc") || strings.Contains(out, "ignore") {
		t.Fatal("not cleaned: " + out)
	}
}

func TestTokenCapDropsOldestTurns(t *testing.T) {
	var turns []adapter.Turn
	for i := 0; i < 50; i++ {
		turns = append(turns, adapter.Turn{Role: "user", Text: strings.Repeat("word ", 100)})
	}
	turns = append(turns, adapter.Turn{Role: "user", Text: "LASTQUESTION"})
	out := Session(adapter.Session{Turns: turns}, 500)
	if EstimateTokens(out) > 500 || !strings.Contains(out, "LASTQUESTION") {
		t.Fatalf("cap failed: %d tokens", EstimateTokens(out))
	}
}
