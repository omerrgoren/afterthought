// Package extract turns a cleaned session into concepts via the user's AI CLI.
package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/omerrgoren/afterthought/internal/llm"
)

type Concept struct {
	Name       string `json:"concept"`
	Category   string `json:"category"`
	Context    string `json:"context"`
	Why        string `json:"why_it_matters"`
	Signal     string `json:"understanding_signal"` // asked | explained_only | auto_accepted | knows
	Importance int    `json:"importance"`           // 1-5
	Difficulty string `json:"difficulty"`           // easy | mid | hard
	TimesAsked int    `json:"times_asked"`
	Recap      string `json:"recap"`
}

const promptTmpl = `You find knowledge gaps in an AI coding session. Below is a cleaned transcript (USER questions and ASSISTANT explanations).
List concepts the user probably does NOT fully understand but would benefit from learning: things the assistant fixed or used without the user showing they understood. Skip trivia and things the user clearly knows.
Phrase concepts as a comparison or question when natural (e.g. "readiness vs liveness probes").
Already known concepts (do not repeat these): %s
%s
Reply with ONLY a JSON array. Each item: {"concept":string,"category":string,"context":"the problem it came up in, one sentence","why_it_matters":"one sentence","understanding_signal":"asked|explained_only|auto_accepted|knows","importance":1-5,"difficulty":"easy|mid|hard","times_asked":number of user turns about it,"recap":"2-3 line summary"}.
Use "auto_accepted" when a hard concept was applied and the user just moved on. Use [] if there are no gaps.

TRANSCRIPT:
%s`

func Concepts(ctx context.Context, r llm.Runner, transcript string, known []string, repeatedHint string) ([]Concept, error) {
	k := "(none)"
	if len(known) > 0 {
		k = strings.Join(known, "; ")
	}
	hint := ""
	if repeatedHint != "" {
		hint = "These user questions look repeated, so they are likely gaps:\n" + repeatedHint
	}
	prompt := fmt.Sprintf(promptTmpl, k, hint, transcript)
	out, err := r.Run(ctx, prompt, false)
	if err != nil {
		return nil, err
	}
	cs, err := Parse(out)
	if err != nil { // models occasionally answer in prose: retry once, stricter
		out, rerr := r.Run(ctx, prompt+"\n\nYour reply must be ONLY a JSON array, no prose, no code fences. Use [] if nothing.", false)
		if rerr != nil {
			return nil, rerr
		}
		cs, err = Parse(out)
	}
	if err != nil {
		return nil, fmt.Errorf("%w (reply started: %q)", err, snippet(out))
	}
	return cs, nil
}

func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// Parse pulls the first JSON array out of the model reply.
func Parse(s string) ([]Concept, error) {
	i, j := strings.Index(s, "["), strings.LastIndex(s, "]")
	if i < 0 || j < i {
		return nil, fmt.Errorf("no JSON array in reply")
	}
	var cs []Concept
	if err := json.Unmarshal([]byte(s[i:j+1]), &cs); err != nil {
		return nil, err
	}
	var out []Concept
	for _, c := range cs {
		c.Name = strings.TrimSpace(c.Name)
		if c.Name == "" {
			continue
		}
		if c.Importance < 1 || c.Importance > 5 {
			c.Importance = 3
		}
		switch c.Difficulty {
		case "easy", "mid", "hard":
		default:
			c.Difficulty = "mid"
		}
		if c.TimesAsked < 1 && c.Signal == "asked" {
			c.TimesAsked = 1
		}
		out = append(out, c)
	}
	return out, nil
}
