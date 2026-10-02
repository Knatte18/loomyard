// text.go renders the compact one-line-per-entry listing that "list --text" and "find --text" print instead of JSON.

package boardcli

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// columnGap separates the listing's columns.
const columnGap = "  "

// RenderCompact returns one line per entry, in the order given: tier, type, slug and title, each padded to the widest value in its column, then "[<status>]" when the entry's status is set.
// Empty input renders as the empty string.
func RenderCompact(tasks []boardengine.BriefTask) string {
	var typeWidth, slugWidth, titleWidth int
	for _, t := range tasks {
		typeWidth = max(typeWidth, utf8.RuneCountInString(t.Type))
		slugWidth = max(slugWidth, utf8.RuneCountInString(t.Slug))
		titleWidth = max(titleWidth, utf8.RuneCountInString(t.Title))
	}

	var sb strings.Builder
	for _, t := range tasks {
		line := strconv.Itoa(t.Tier) + columnGap +
			pad(t.Type, typeWidth) + columnGap +
			pad(t.Slug, slugWidth) + columnGap +
			pad(t.Title, titleWidth)
		if t.Status != nil && *t.Status != "" {
			line += columnGap + "[" + *t.Status + "]"
		}
		sb.WriteString(strings.TrimRight(line, " "))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// pad right-pads s with spaces to width characters, so a multi-byte character counts as one column.
func pad(s string, width int) string {
	return s + strings.Repeat(" ", width-utf8.RuneCountInString(s))
}
