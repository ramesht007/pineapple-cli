// Package scan flags likely secrets in lines about to be committed.
package scan

import (
	"path"
	"regexp"
)

// AddedLine matches repo.AddedLine (identical type).
type AddedLine = struct{ File, Line string }

type Finding struct{ File, Line, Rule string }

var rules = []struct {
	name string
	re   *regexp.Regexp
}{
	{"aws-access-key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"github-token", regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`)},
	{"github-pat", regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`)},
	{"anthropic-key", regexp.MustCompile(`sk-ant-[A-Za-z0-9\-_]{20,}`)},
	{"openai-project", regexp.MustCompile(`sk-proj-[A-Za-z0-9_\-]{20,}`)},
	{"slack", regexp.MustCompile(`xox[bpoas]-[A-Za-z0-9\-]{10,}`)},
	{"google-api", regexp.MustCompile(`AIza[0-9A-Za-z_\-]{35}`)},
	{"private-key", regexp.MustCompile(`-----BEGIN( RSA| EC| OPENSSH)? PRIVATE KEY-----`)},
	{"generic-secret", regexp.MustCompile(`(?i)(api[_-]?key|secret|token|passwd|password)["']?\s*[:=]\s*["']?[A-Za-z0-9_\-/+]{16,}`)},
}

// Scan returns one finding per (line, matching rule). secrets.env.age is never flagged.
func Scan(lines []AddedLine) []Finding {
	var out []Finding
	for _, l := range lines {
		if path.Base(l.File) == "secrets.env.age" {
			continue
		}
		for _, r := range rules {
			if r.re.MatchString(l.Line) {
				out = append(out, Finding{l.File, l.Line, r.name})
			}
		}
	}
	return out
}
