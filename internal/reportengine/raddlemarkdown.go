// raddlemarkdown.go renders a RaddleReport as markdown: the selection, then a table of Sketches.

package reportengine

import (
	"fmt"
	"strings"
	"time"
)

// HeadingRaddleRows is the heading of RenderRaddleMarkdown's one section.
const HeadingRaddleRows = "## Predicted against actual cost per Sketch"

// RenderRaddleMarkdown renders report as one document: the selection it was computed over, then one table row per Sketch.
// A figure the report holds as nil renders as no data.
func RenderRaddleMarkdown(report RaddleReport) string {
	var b strings.Builder
	b.WriteString("# Raddle report\n\n")
	s := report.Selection
	fmt.Fprintf(&b, "- board branch: %s at %s\n", s.BoardBranch, s.BoardTip)
	fmt.Fprintf(&b, "- records branch: %s at %s\n", s.RecordsBranch, s.RecordsTip)
	fmt.Fprintf(&b, "- snapshot: %s\n", s.SnapshotAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- sketches: %d, findings: %d\n", len(report.Rows), len(report.Findings))

	b.WriteString("\n" + HeadingRaddleRows + "\n\n")
	header := []string{"Sketch", "Predicted USD", "Actual USD", "Actual partial", "Unlanded parts", "Proposal changed"}
	rows := make([][]string, len(report.Rows))
	for i, row := range report.Rows {
		unlanded := "none"
		if len(row.Unlanded) > 0 {
			unlanded = strings.Join(row.Unlanded, ", ")
		}
		rows[i] = []string{row.Sketch, formatFigure(row.PredictedUSD), formatFigure(row.ActualUSD), fmt.Sprint(row.ActualPartial), unlanded, fmt.Sprint(row.ProposalChanged)}
	}
	table(&b, header, rows)
	return b.String()
}
