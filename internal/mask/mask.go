// Package mask hides secrets before any text reaches an AI.
package mask

import "regexp"

const Redacted = "[REDACTED]"

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`sk-[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`xox[abprs]-[A-Za-z0-9\-]{10,}`),
	regexp.MustCompile(`AIza[0-9A-Za-z_\-]{30,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{16,}`),
	regexp.MustCompile(`(?i)(://[^/\s:@]+:)[^@\s/]+(@)`),
}

// key=value style assignments keep the key and hide the value.
var assign = regexp.MustCompile(`(?i)((?:password|passwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key)["']?\s*[:=]\s*["']?)[^\s"',;]{6,}`)

func Text(s string) string {
	for i, p := range patterns {
		if i == len(patterns)-1 {
			s = p.ReplaceAllString(s, "${1}"+Redacted+"${2}")
			continue
		}
		s = p.ReplaceAllString(s, Redacted)
	}
	return assign.ReplaceAllString(s, "${1}"+Redacted)
}
