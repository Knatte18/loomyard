// haltfindings.go words the findings of a Stuck over judge rulings.
// It groups the rulings that share a case, key, kind and reason, and caps the text the halt carries.
// It writes every finding to the findings file the halt names.

package driftengine

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	// haltFindingsLimit is the most characters of findings a halt reason carries, separators and omission note included;
	// the findings file holds the rest.
	haltFindingsLimit = 1500
	// findingsSeparator joins the findings of a halt reason.
	findingsSeparator = "; "
	// findingsEllipsis ends a first finding cut to the limit.
	findingsEllipsis = "…"
)

// ruledCell is one cell a judge ruled, with the ruling's reason.
type ruledCell struct {
	cell   ClassifiedCell
	reason string
}

// describeRuledCell words one ruled cell with its reason.
func describeRuledCell(ruled ruledCell) string {
	return fmt.Sprintf("%s: %s", describeCell(ruled.cell), ruled.reason)
}

// rulingGroupKey is what the cells of one group share.
type rulingGroupKey struct {
	caseName string
	key      string
	kind     CellKind
	reason   string
}

// groupRuledCells words the ruled cells as one finding per group of cells that share a case, key, kind and reason.
// Groups come in the order of their first cell.
// A group of one cell is worded as that cell.
// A larger group names its row range, its row count and the old and new values of its first and last rows.
// The rows of a keyed group are the key's occurrences, and those of an unkeyed group are its line positions.
// The first and last rows are the lowest and the highest, whatever order the cells come in.
func groupRuledCells(ruled []ruledCell) []string {
	var order []rulingGroupKey
	groups := map[rulingGroupKey][]ruledCell{}
	for _, item := range ruled {
		cell := item.cell.Cell
		key := rulingGroupKey{caseName: cell.Case, key: cell.Identity.Key, kind: cell.Kind, reason: item.reason}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], item)
	}

	findings := make([]string, 0, len(order))
	for _, key := range order {
		group := groups[key]
		if len(group) == 1 || key.kind == CellAddedCase || key.kind == CellRemovedCase {
			for _, item := range group {
				findings = append(findings, describeRuledCell(item))
			}
			continue
		}
		slices.SortStableFunc(group, func(a, b ruledCell) int { return cmp.Compare(rowOf(a.cell.Cell), rowOf(b.cell.Cell)) })
		first, last := group[0].cell.Cell, group[len(group)-1].cell.Cell
		low, high := rowOf(first), rowOf(last)
		at := fmt.Sprintf("lines #%d to #%d", low, high)
		if key.key != "" {
			at = fmt.Sprintf("key %q n %d to %d", key.key, low, high)
		}
		findings = append(findings, fmt.Sprintf("%s in case %q at %s, %d rows, first old %q, new %q, last old %q, new %q: %s",
			key.kind, key.caseName, at, len(group), first.Old, first.New, last.Old, last.New, key.reason))
	}
	return findings
}

// rowOf returns the row a cell sits at: its key's occurrence for a keyed line, and its line position otherwise.
func rowOf(cell Cell) int {
	if cell.Identity.Key != "" {
		return cell.Identity.N
	}
	return cell.Identity.Line
}

// omittedFindingsNote words how many findings a capped halt reason left out.
func omittedFindingsNote(count int) string {
	return fmt.Sprintf("and %d more", count)
}

// cappedFindings joins findings for a halt reason, keeping whole findings so the text stays within haltFindingsLimit.
// The separators and the note of how many findings it left out count toward the limit.
// It keeps the first finding always, cut to the limit with an ellipsis when it alone passes it.
func cappedFindings(findings []string) string {
	var kept []string
	length := 0
	for i, finding := range findings {
		reserved := 0
		if rest := len(findings) - i - 1; rest > 0 {
			reserved = len(findingsSeparator) + len(omittedFindingsNote(rest))
		}
		used := length
		if i > 0 {
			used += len(findingsSeparator)
		}
		if used+len(finding)+reserved <= haltFindingsLimit {
			kept = append(kept, finding)
			length = used + len(finding)
			continue
		}

		dropped := i
		if i == 0 {
			cut := finding[:haltFindingsLimit-reserved-len(findingsEllipsis)]
			kept = append(kept, strings.ToValidUTF8(cut, "")+findingsEllipsis)
			dropped = 1
		}
		if dropped < len(findings) {
			kept = append(kept, omittedFindingsNote(len(findings)-dropped))
		}
		break
	}
	return strings.Join(kept, findingsSeparator)
}

// writeFindingsFile writes every finding, one per line, to the findings file Paths.Findings names, replacing it.
// It returns the file's path relative to the worktree root.
func writeFindingsFile(d Deps, lead string, findings []string) (string, error) {
	var text strings.Builder
	fmt.Fprintf(&text, "# %s\n\n", lead)
	for _, finding := range findings {
		fmt.Fprintf(&text, "- %s\n", finding)
	}
	if err := os.MkdirAll(filepath.Dir(d.Paths.Findings), 0o755); err != nil {
		return "", fmt.Errorf("drift: create the findings file's directory: %w", err)
	}
	if err := os.WriteFile(d.Paths.Findings, []byte(text.String()), 0o644); err != nil {
		return "", fmt.Errorf("drift: write the findings file: %w", err)
	}
	return worktreeRel(d, d.Paths.Findings)
}
