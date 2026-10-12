// markdown.go renders a Report as markdown: the selection, then one section per board question, each a table over the report's weeks.

package reportengine

import (
	"fmt"
	"sort"
	"strings"
)

// noData is what a figure no run or fix carries renders as.
const noData = "no data"

// The headings of RenderMarkdown's four sections, one per board question.
const (
	HeadingWakes      = "## Do orch wakes per run fall?"
	HeadingStops      = "## Do the same stop and failure classes recur?"
	HeadingFreshFixes = "## Does the fresh-code fix share fall?"
	HeadingRounds     = "## Do review rounds per segment fall?"
)

// RenderMarkdown renders r as one document: the selection it was computed over, then one section per board question.
// A figure the report holds as nil renders as no data.
func RenderMarkdown(r Report) string {
	var b strings.Builder
	b.WriteString("# Convergence report\n\n")
	s := r.Selection
	fmt.Fprintf(&b, "- parent branch: %s at %s\n", s.ParentBranch, s.ParentTip)
	fmt.Fprintf(&b, "- records branch: %s at %s\n", s.RecordsBranch, s.RecordsTip)
	fmt.Fprintf(&b, "- board branch: %s at %s\n", s.BoardBranch, s.BoardTip)
	fmt.Fprintf(&b, "- since: %s\n", s.Since.UTC().Format("2006-01-02"))
	fmt.Fprintf(&b, "- fresh days: %d\n", s.FreshDays)
	fmt.Fprintf(&b, "- runs: %d, partial runs: %d, landings: %d, findings: %d\n", len(r.Runs), len(r.Partial), len(r.Landings), len(r.Findings))

	b.WriteString("\n" + HeadingWakes + "\n\n")
	table(&b, []string{"Week", "Runs", "Runs counted", "Wakes per run", "Wakes by kind", "Automatic steps", "Paused"}, wakeRows(r.Weeks))

	b.WriteString("\n" + HeadingStops + "\n\n")
	table(&b, []string{"Week", "Kind", "Class", "Stops", "Runs recurred in"}, stopRows(r.Weeks))

	b.WriteString("\n" + HeadingFreshFixes + "\n\n")
	table(&b, []string{"Week", "Fix", "Feature", "Other", "Unmapped", "Hand", "Fix share", "Fix fresh / aged / no pre-image", "Unmapped share", "Unmapped fresh / aged / no pre-image"}, freshRows(r.Weeks))

	b.WriteString("\n" + HeadingRounds + "\n\n")
	table(&b, []string{"Week", "Segment", "Mean rounds", "Runs", "Lacking", "Outcomes"}, roundRows(r.Weeks))
	return b.String()
}

// table writes a markdown table, or no data under the heading when it has no rows.
func table(b *strings.Builder, header []string, rows [][]string) {
	if len(rows) == 0 {
		b.WriteString(noData + "\n")
		return
	}
	b.WriteString("| " + strings.Join(header, " | ") + " |\n")
	b.WriteString("|" + strings.Repeat(" --- |", len(header)) + "\n")
	for _, row := range rows {
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
}

// formatFloat spells a mean or share, no data when nil.
func formatFloat(value *float64) string {
	if value == nil {
		return noData
	}
	return fmt.Sprintf("%.2f", *value)
}

// formatInt spells a count, no data when nil.
func formatInt(value *int) string {
	if value == nil {
		return noData
	}
	return fmt.Sprint(*value)
}

// formatCounts spells a map of counts as sorted `key n` pairs; nil is no data and an empty map is none.
func formatCounts(counts map[string]int) string {
	if counts == nil {
		return noData
	}
	if len(counts) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, len(keys))
	for i, key := range keys {
		pairs[i] = fmt.Sprintf("%s %d", key, counts[key])
	}
	return strings.Join(pairs, ", ")
}

// formatWakes spells the wake kinds of every source as `source: kind n` pairs.
func formatWakes(wakes map[string]map[string]int) string {
	if wakes == nil {
		return noData
	}
	sources := make([]string, 0, len(wakes))
	for source := range wakes {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	var parts []string
	for _, source := range sources {
		parts = append(parts, source+": "+formatCounts(wakes[source]))
	}
	return strings.Join(parts, "; ")
}

func wakeRows(weeks []Week) [][]string {
	rows := make([][]string, len(weeks))
	for i, w := range weeks {
		rows[i] = []string{w.Week, fmt.Sprint(w.Runs), fmt.Sprint(w.WakeRuns), formatFloat(w.WakesPerRun), formatWakes(w.Wakes), formatCounts(w.Automatic), formatInt(w.Paused)}
	}
	return rows
}

func stopRows(weeks []Week) [][]string {
	var rows [][]string
	for _, w := range weeks {
		for _, set := range []struct {
			kind    string
			classes []ClassCount
		}{{"stop", w.StopClasses}, {"failure", w.FailureClasses}} {
			if set.classes == nil {
				rows = append(rows, []string{w.Week, set.kind, noData, noData, noData})
				continue
			}
			for _, class := range set.classes {
				rows = append(rows, []string{w.Week, set.kind, class.Class, fmt.Sprint(class.Stops), fmt.Sprint(class.Runs)})
			}
		}
	}
	return rows
}

func freshCountsCell(c FreshCounts) string {
	return fmt.Sprintf("%d / %d / %d", c.Fresh, c.Aged, c.NoPreImage)
}

func freshRows(weeks []Week) [][]string {
	rows := make([][]string, len(weeks))
	for i, w := range weeks {
		rows[i] = []string{
			w.Week,
			fmt.Sprint(w.Landings[LandingKindFix]), fmt.Sprint(w.Landings[LandingKindFeature]), fmt.Sprint(w.Landings[LandingKindOther]), fmt.Sprint(w.Landings[LandingKindUnmapped]),
			fmt.Sprint(w.Hand),
			formatFloat(w.FixShare), freshCountsCell(w.Fix),
			formatFloat(w.UnmappedShare), freshCountsCell(w.Unmapped),
		}
	}
	return rows
}

func roundRows(weeks []Week) [][]string {
	var rows [][]string
	for _, w := range weeks {
		segments := make([]string, 0, len(w.Rounds))
		for segment := range w.Rounds {
			segments = append(segments, segment)
		}
		sort.Strings(segments)
		for _, segment := range segments {
			held := w.Rounds[segment]
			rows = append(rows, []string{w.Week, segment, formatFloat(held.Mean), fmt.Sprint(held.Runs), fmt.Sprint(held.Lacking), formatCounts(held.Outcomes)})
		}
	}
	return rows
}
