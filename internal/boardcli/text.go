// text.go renders the compact one-line-per-entry listing that "list --text" and "find --text" print instead of JSON.

package boardcli

import (
	"strings"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// columnGap separates the listing's columns.
const columnGap = "  "

// RenderCompact returns one line per entry, in the order given: kind, priority, slug, title and the labels joined by commas, each padded to the widest value in its column, then "[<status>]" when the entry's status is set.
// The priority column holds high or low and is blank for a normal entry, and it is left out when every entry is normal.
// Empty input renders as the empty string.
func RenderCompact(tasks []boardengine.BriefTask) string {
	var kindWidth, priorityWidth, slugWidth, titleWidth, labelsWidth int
	for _, t := range tasks {
		kindWidth = max(kindWidth, utf8.RuneCountInString(t.Kind))
		priorityWidth = max(priorityWidth, utf8.RuneCountInString(t.Priority))
		slugWidth = max(slugWidth, utf8.RuneCountInString(t.Slug))
		titleWidth = max(titleWidth, utf8.RuneCountInString(t.Title))
		labelsWidth = max(labelsWidth, utf8.RuneCountInString(strings.Join(t.Labels, ",")))
	}

	var sb strings.Builder
	for _, t := range tasks {
		line := pad(t.Kind, kindWidth) + columnGap
		if priorityWidth > 0 {
			line += pad(t.Priority, priorityWidth) + columnGap
		}
		line += pad(t.Slug, slugWidth) + columnGap +
			pad(t.Title, titleWidth) + columnGap +
			pad(strings.Join(t.Labels, ","), labelsWidth)
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
