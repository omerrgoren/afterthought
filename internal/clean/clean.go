// Package clean reduces a session to user questions and assistant explanations.
package clean

import (
	"regexp"
	"strings"

	"github.com/omerrgoren/afterthought/internal/adapter"
	"github.com/omerrgoren/afterthought/internal/mask"
)

const (
	maxCodeLines   = 6
	maxAssistChars = 1500
)

var (
	fence   = regexp.MustCompile("(?s)```.*?```")
	sysTags = regexp.MustCompile(`(?s)<(system-reminder|command-[a-z]+|local-command-[a-z]+|pasted_content)[^>]*>.*?</[a-z_\-]+>`)
	multiNL = regexp.MustCompile(`\n{3,}`)
)

// EstimateTokens is a cheap chars/4 estimate.
func EstimateTokens(s string) int { return (len(s) + 3) / 4 }

func dropLongCode(s string) string {
	return fence.ReplaceAllStringFunc(s, func(b string) string {
		if strings.Count(b, "\n") > maxCodeLines {
			return "[code omitted]"
		}
		return b
	})
}

// Session returns masked, cleaned text capped at capTokens (oldest turns dropped first).
func Session(s adapter.Session, capTokens int) string {
	var turns []string
	for _, t := range s.Turns {
		txt := sysTags.ReplaceAllString(t.Text, "")
		txt = dropLongCode(txt)
		txt = multiNL.ReplaceAllString(strings.TrimSpace(txt), "\n\n")
		if txt == "" {
			continue
		}
		if t.Role == "assistant" && len(txt) > maxAssistChars {
			txt = txt[:maxAssistChars] + " [...]"
		}
		label := "USER"
		if t.Role == "assistant" {
			label = "ASSISTANT"
		}
		turns = append(turns, label+": "+mask.Text(txt))
	}
	for len(turns) > 1 && EstimateTokens(strings.Join(turns, "\n\n")) > capTokens {
		turns = turns[1:]
	}
	out := strings.Join(turns, "\n\n")
	if max := capTokens * 4; len(out) > max {
		out = out[len(out)-max:]
	}
	return out
}

// UserTurns returns just the user questions (masked, code removed).
func UserTurns(s adapter.Session) []string {
	var out []string
	for _, t := range s.Turns {
		if t.Role != "user" {
			continue
		}
		txt := strings.TrimSpace(dropLongCode(sysTags.ReplaceAllString(t.Text, "")))
		if txt != "" {
			out = append(out, mask.Text(txt))
		}
	}
	return out
}
