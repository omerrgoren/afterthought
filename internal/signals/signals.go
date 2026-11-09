// Package signals holds cheap, LLM-free don't-know detection.
package signals

import (
	"regexp"
	"strings"
)

var word = regexp.MustCompile(`[\p{L}\p{N}]{3,}`)

func tokens(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range word.FindAllString(strings.ToLower(s), -1) {
		m[w] = true
	}
	return m
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	return float64(inter) / float64(len(a)+len(b)-inter)
}

// Repeated groups similar user questions and returns the groups of size >= 2.
func Repeated(questions []string) [][]string {
	toks := make([]map[string]bool, len(questions))
	for i, q := range questions {
		toks[i] = tokens(q)
	}
	used := make([]bool, len(questions))
	var groups [][]string
	for i := range questions {
		if used[i] {
			continue
		}
		g := []string{questions[i]}
		for j := i + 1; j < len(questions); j++ {
			if !used[j] && jaccard(toks[i], toks[j]) >= 0.4 {
				g = append(g, questions[j])
				used[j] = true
			}
		}
		if len(g) >= 2 {
			groups = append(groups, g)
		}
	}
	return groups
}
