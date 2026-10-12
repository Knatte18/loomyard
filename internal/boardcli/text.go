// text.go renders the compact one-line-per-entry listing that "list --text" and "find --text" print instead of JSON.

package boardcli

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// columnGap separates the listing's columns.
const columnGap = "  "

// RenderCompact returns one line per entry, in the order given: kind, priority, age, time since last revision, slug, title and the labels joined by commas, each padded to the widest value in its column, then "[<status>]" when the entry's status is set.
// The priority column holds high or low and is blank for a normal entry, and it is left out when every entry is normal.
// The ages are as of now, blank for an unknown date, and each age column is left out when no entry has that date.
// Empty input renders as the empty string.
func RenderCompact(tasks []boardengine.BriefTask, now time.Time) string {
	var kindWidth, priorityWidth, ageWidth, revisedWidth, slugWidth, titleWidth, labelsWidth int
	for _, t := range tasks {
		kindWidth = max(kindWidth, utf8.RuneCountInString(t.Kind))
		priorityWidth = max(priorityWidth, utf8.RuneCountInString(t.Priority))
		ageWidth = max(ageWidth, len(boardengine.FormatAge(t.Created, now)))
		revisedWidth = max(revisedWidth, len(boardengine.FormatAge(t.Revised, now)))
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
		if ageWidth > 0 {
			line += pad(boardengine.FormatAge(t.Created, now), ageWidth) + columnGap
		}
		if revisedWidth > 0 {
			line += pad(boardengine.FormatAge(t.Revised, now), revisedWidth) + columnGap
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
