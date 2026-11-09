// Package roadmap writes the weekly learning roadmap as a local .md file.
package roadmap

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/omerrgoren/afterthought/internal/config"
	"github.com/omerrgoren/afterthought/internal/ledger"
	"github.com/omerrgoren/afterthought/internal/llm"
)

// Boundary is the most recent Friday 13:00 (local) at or before now.
func Boundary(now time.Time) time.Time {
	d := now
	for d.Weekday() != time.Friday {
		d = d.AddDate(0, 0, -1)
	}
	b := time.Date(d.Year(), d.Month(), d.Day(), 13, 0, 0, 0, now.Location())
	if b.After(now) {
		b = b.AddDate(0, 0, -7)
	}
	return b
}

func WeekKey(b time.Time) string { return b.Format("2006-01-02") }

// LinkOK reports whether a URL resolves; replaced in tests.
var LinkOK = func(u string) bool {
	c := &http.Client{Timeout: 8 * time.Second}
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		req, err := http.NewRequest(method, u, nil)
		if err != nil {
			return false
		}
		req.Header.Set("User-Agent", "afterthought-linkcheck")
		resp, err := c.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode < 400 || resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 429 {
			return true
		}
	}
	return false
}

type topic struct {
	c    ledger.Concept
	kind string // fresh | parked | listed
}

const sectionPrompt = `Write learning notes for a developer. For EACH topic below output a block that starts with a line "=== TOPIC: <exact topic name> ===" followed by markdown with exactly these parts, in order:
### What is X
### What is Y
### Where to use each
### Best practices
### Interesting articles and official docs
(If the topic is not a comparison, use "What it is", "Why it matters" in place of the first two headings and keep the other three.)
Be concise and concrete. In the last part, use web search and list 2-4 REAL links as markdown bullets (official docs first). Never invent URLs.
Topics:
%s`

func Generate(ctx context.Context, r llm.Runner, l *ledger.Ledger, cfg config.Config, dir string, now time.Time, force bool) (string, bool, error) {
	b := Boundary(now)
	week := WeekKey(b)
	if ex, err := l.Roadmap(week); err != nil {
		return "", false, err
	} else if ex != nil && !force {
		return ex.Path, false, nil
	}
	topics, err := pick(l, cfg, now)
	if err != nil {
		return "", false, err
	}
	var llmTopics []ledger.Concept
	for _, t := range topics {
		if t.kind == "fresh" {
			llmTopics = append(llmTopics, t.c)
		}
	}
	blocks := map[string]string{}
	if len(llmTopics) > 0 && r != nil {
		var lines []string
		for _, c := range llmTopics {
			lines = append(lines, fmt.Sprintf("- %s (context: %s)", c.Name, c.Context))
		}
		if out, err := r.Run(ctx, fmt.Sprintf(sectionPrompt, strings.Join(lines, "\n")), true); err == nil {
			blocks = splitBlocks(out)
		}
	}
	md := render(topics, blocks, now)
	md = DropDeadLinks(md)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	path := filepath.Join(dir, week+"-roadmap.md")
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		return "", false, err
	}
	var ids []int64
	for _, t := range topics {
		ids = append(ids, t.c.ID)
	}
	if err := l.MarkListed(ids, week); err != nil {
		return "", false, err
	}
	if err := l.AddRoadmap(week, path, now); err != nil {
		return "", false, err
	}
	return path, true, nil
}

// pick chooses at most cfg.MaxTopics: fresh gaps first, then the parking lot, then earlier topics for recap.
func pick(l *ledger.Ledger, cfg config.Config, now time.Time) ([]topic, error) {
	var out []topic
	add := func(cs []ledger.Concept, kind string) {
		for _, c := range cs {
			if len(out) >= cfg.MaxTopics {
				return
			}
			out = append(out, topic{c, kind})
		}
	}
	fresh, err := l.Fresh(now)
	if err != nil {
		return nil, err
	}
	add(fresh, "fresh")
	if len(out) < cfg.MaxTopics {
		parked, err := l.Parked(now)
		if err != nil {
			return nil, err
		}
		add(parked, "parked")
	}
	if len(out) < cfg.MaxTopics {
		listed, err := l.Listed(now)
		if err != nil {
			return nil, err
		}
		for _, c := range listed {
			dup := false
			for _, t := range out {
				dup = dup || t.c.ID == c.ID
			}
			if !dup {
				add([]ledger.Concept{c}, "listed")
			}
		}
	}
	return out, nil
}

var blockSplit = regexp.MustCompile(`(?m)^=== TOPIC: (.+?) ===\s*$`)

func splitBlocks(s string) map[string]string {
	out := map[string]string{}
	idx := blockSplit.FindAllStringSubmatchIndex(s, -1)
	for i, m := range idx {
		end := len(s)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		out[ledger.Key(s[m[2]:m[3]])] = strings.TrimSpace(s[m[1]:end])
	}
	return out
}

func render(topics []topic, blocks map[string]string, now time.Time) string {
	var sb strings.Builder
	sb.WriteString("# This week's hot topics you need to learn and cover\n\n")
	sb.WriteString(fmt.Sprintf("_Generated %s by afterthought._\n\n", now.Format("2006-01-02 15:04")))
	if len(topics) == 0 {
		sb.WriteString("Nothing has been captured yet. afterthought watches your AI coding sessions and, as gaps show up, this file fills with topics worth learning.\n\n")
		sb.WriteString("While you wait, a good first topic: **what did the last thing your AI fixed for you actually do?** Pick one change from your latest session and trace it by hand.\n")
		return sb.String()
	}
	for i, t := range topics {
		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, t.c.Name, tag(t)))
	}
	sb.WriteString("\n")
	for _, t := range topics {
		sb.WriteString("## " + t.c.Name + "\n\n")
		switch t.kind {
		case "fresh":
			if t.c.Resurfaced && t.c.Recap != "" {
				sb.WriteString("**Recap:** " + oneLine(t.c.Recap) + "\n\n")
			}
			if b := blocks[ledger.Key(t.c.Name)]; b != "" {
				sb.WriteString(b + "\n\n")
			} else {
				sb.WriteString(fallback(t.c))
			}
		case "parked":
			sb.WriteString(fmt.Sprintf("_From your parking lot: seen %d time(s)%s._\n\n", t.c.Seen, neverAsked(t.c)))
			sb.WriteString(compact(t.c))
		case "listed":
			sb.WriteString("_Revisit: listed before and it came up again or is still open._\n\n")
			sb.WriteString("**Recap:** " + oneLine(firstNonEmpty(t.c.Recap, t.c.Why)) + "\n\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func tag(t topic) string {
	switch t.kind {
	case "parked":
		return fmt.Sprintf(" _(parking lot, seen %d)_", t.c.Seen)
	case "listed":
		return " _(revisit)_"
	}
	return ""
}

func neverAsked(c ledger.Concept) string {
	if c.Reasked == 0 {
		return ", never asked about"
	}
	return fmt.Sprintf(", re-asked %d time(s)", c.Reasked)
}

func compact(c ledger.Concept) string {
	s := ""
	if c.Why != "" {
		s += "**Why it matters:** " + oneLine(c.Why) + "\n\n"
	}
	if c.Context != "" {
		s += "**Where it came up:** " + oneLine(c.Context) + "\n\n"
	}
	return s
}

func fallback(c ledger.Concept) string {
	return compact(c) + "_Full notes were unavailable this week (the AI CLI could not be reached). Ask your AI to explain this topic, or run `afterthought roadmap --force` later._\n\n"
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

var urlRe = regexp.MustCompile(`https?://[^\s)>\]"']+`)

// DropDeadLinks removes lines whose URL does not resolve, so every shipped link was alive when written.
func DropDeadLinks(md string) string {
	cache := map[string]bool{}
	var out []string
	for _, line := range strings.Split(md, "\n") {
		ok := true
		for _, u := range urlRe.FindAllString(line, -1) {
			u = strings.TrimRight(u, ".,;")
			alive, seen := cache[u]
			if !seen {
				alive = LinkOK(u)
				cache[u] = alive
			}
			if !alive {
				ok = false
			}
		}
		if ok {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
