package report

import (
	"strconv"
	"strings"
	"unicode"
)

// textField makes untrusted values safe for one-line terminal output without
// altering printable text. Control characters and other non-printable runes
// are rendered as Go-style escapes (e.g. \n, \x1b); printable characters —
// including quotes and backslashes — are preserved verbatim so that ordinary
// report values such as C:\repo and "quoted" text stay human-readable.
func textField(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsPrint(r) {
			b.WriteRune(r)
			continue
		}
		esc := strconv.QuoteRune(r) // e.g. '\n', '\x1b', '\u2028'
		b.WriteString(esc[1 : len(esc)-1])
	}
	return b.String()
}

// markdownCell keeps an untrusted value inside one GitHub-flavored Markdown
// table cell as literal text. Backslash is escaped first so later escapes
// cannot combine into a single user-controlled Markdown escape sequence; then
// every Markdown-significant delimiter — links, images, HTML, emphasis, code
// spans, and the table cell separator — is backslash-escaped and line breaks
// are collapsed to spaces.
func markdownCell(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	for _, c := range []string{`|`, "`", `[`, `]`, `(`, `)`, `!`, `<`, `>`, `*`, `_`, `~`} {
		value = strings.ReplaceAll(value, c, `\`+c)
	}
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
