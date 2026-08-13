package execute

import "regexp"

type Redactor struct {
	patterns []*regexp.Regexp
}

func NewRedactor(values []string) Redactor {
	patterns := make([]*regexp.Regexp, 0, len(values)+4)
	for _, value := range values {
		if value != "" {
			patterns = append(patterns, regexp.MustCompile(regexp.QuoteMeta(value)))
		}
	}
	patterns = append(patterns,
		regexp.MustCompile(`(?i)(?:ghp_|github_pat_)[A-Za-z0-9_]+`),
		regexp.MustCompile(`(?i)npm_[A-Za-z0-9]+`),
		regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=-]+`),
		regexp.MustCompile(`-----BEGIN [^-]+-----[\s\S]*?-----END [^-]+-----`),
	)
	return Redactor{patterns: patterns}
}

func (r Redactor) Redact(value string) string {
	for _, pattern := range r.patterns {
		value = pattern.ReplaceAllString(value, "[REDACTED]")
	}
	return value
}
