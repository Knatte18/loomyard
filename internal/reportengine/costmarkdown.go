// costmarkdown.go renders a CostReport as markdown: the selection, then a table of cost cells.

package reportengine

import (
	"fmt"
	"strings"
	"time"
)

// HeadingCostCells is the level-two heading of the cost table in a rendered cost report.
const HeadingCostCells = "## Cost per recipe, profile and stored record"

// RenderCostMarkdown renders report as one document: the selection it was computed over, then one table row per cell.
// A figure the report holds as nil renders as no data, and a cell under the minimum shows its note in place of every figure.
func RenderCostMarkdown(report CostReport) string {
	var b strings.Builder
	b.WriteString("# Cost report\n\n")
	s := report.Selection
	fmt.Fprintf(&b, "- records branch: %s at %s\n", s.RecordsBranch, s.RecordsTip)
	fmt.Fprintf(&b, "- since: %s\n", s.Since.UTC().Format("2006-01-02"))
	fmt.Fprintf(&b, "- minimum runs per cell: %d\n", s.MinRuns)
	fmt.Fprintf(&b, "- snapshot: %s\n", s.SnapshotAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- cells: %d, findings: %d\n", len(report.Cells), len(report.Findings))

	b.WriteString("\n" + HeadingCostCells + "\n\n")
	header := []string{"Recipe", "Profile", "Stored record", "Runs", "Mean USD", "Mean hours", "USD fixed", "USD per 100 lines", "USD correlation", "Hours fixed", "Hours per 100 lines", "Hours correlation", "Note"}
	table(&b, header, costRows(report.Cells))
	return b.String()
}

func costRows(cells []CostCell) [][]string {
	rows := make([][]string, len(cells))
	for i, cell := range cells {
		row := []string{cell.Recipe, cell.Profile, fmt.Sprint(cell.StoredRecord), fmt.Sprint(cell.Runs), formatFigure(cell.MeanUSD), formatFigure(cell.MeanHours)}
		row = append(row, fitColumns(cell.USDFit)...)
		row = append(row, fitColumns(cell.HoursFit)...)
		note := cell.Note
		if note == "" {
			note = noData
		}
		rows[i] = append(row, note)
	}
	return rows
}

// fitColumns spells a fit as its fixed cost, cost per 100 lines and correlation, each no data when the fit is nil.
func fitColumns(fit *CostFit) []string {
	if fit == nil {
		return []string{noData, noData, noData}
	}
	return []string{formatFigure(&fit.Fixed), formatFigure(&fit.Per100Lines), formatFigure(&fit.Correlation)}
}

// formatFigure spells a figure to two decimals, no data when nil.
func formatFigure(figure *float64) string {
	if figure == nil {
		return noData
	}
	return fmt.Sprintf("%.2f", *figure)
}
