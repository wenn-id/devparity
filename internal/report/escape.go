package report

import (
	"strconv"
	"strings"
)

// textField makes untrusted values safe for one-line terminal output while
// preserving printable text. strconv.Quote escapes control bytes, quotes,
// backslashes, invalid UTF-8, and embedded line breaks; the surrounding quotes
// are intentionally removed because the report already supplies delimiters.
func textField(value string) string {
	quoted := strconv.Quote(value)
	return quoted[1 : len(quoted)-1]
}

// markdownCell keeps an untrusted value inside one GitHub-flavored Markdown
// table cell. Backslash is escaped first so subsequent escapes cannot be
// interpreted as a single user-controlled Markdown escape sequence.
func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "|", `\|`)
	value = strings.ReplaceAll(value, "`", "\\`")
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
