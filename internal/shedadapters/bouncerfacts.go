// bouncerfacts.go computes and renders the per-round facts file the judge reads in place of the
// review and fixer artifacts: counts per round, severity and class, and the finding keys that recur
// across the ledgers.
// The counts come from burlerengine.ParseReview, never a second parser, and the file records no
// review's own top-level verdict, so the judge decides from the findings alone.
// The file is an input to the judge and never one of its declared output files.

package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// severityOrder is the fixed order severities appear in, most severe first.
var severityOrder = []burlerengine.Severity{
	burlerengine.SeverityBlocking,
	burlerengine.SeverityMedium,
	burlerengine.SeverityLow,
	burlerengine.SeverityNit,
}

// classOrder is the fixed order classes appear in.
var classOrder = []burlerengine.Class{
	burlerengine.ClassDesign,
	burlerengine.ClassScope,
	burlerengine.ClassDecision,
	burlerengine.ClassConsistency,
}

// roundFactsRow is one round's counts, or the error that kept them from being computed.
type roundFactsRow struct {
	Round    int
	Err      string
	Findings int
	Severity map[burlerengine.Severity]int
	Highest  burlerengine.Severity
	Class    map[burlerengine.Class]int
	Gating   int
}

// recurringKey is a ledger key seen in two or more distinct rounds.
type recurringKey struct {
	Key      string
	Rounds   []int
	Reopened bool
}

// roundFacts is everything the facts file for Round records.
type roundFacts struct {
	Round     int
	Rows      []roundFactsRow
	Recurring []recurringKey
}

// computeRoundFacts reads rounds 1..round's review files and rounds 1..round-1's ledgers inside
// runDir.
// A review that is missing or fails to parse yields a row carrying the error text, and a ledger
// that fails to parse is skipped, so the function never fails.
func computeRoundFacts(runDir string, round int, reportName func(int) string) roundFacts {
	facts := roundFacts{Round: round}
	for n := 1; n <= round; n++ {
		facts.Rows = append(facts.Rows, reviewFactsRow(runDir, n, reportName))
	}
	facts.Recurring = recurringKeys(runDir, round-1)
	return facts
}

// reviewFactsRow counts one round's review file, or records why it could not.
func reviewFactsRow(runDir string, round int, reportName func(int) string) roundFactsRow {
	row := roundFactsRow{Round: round}
	raw, err := os.ReadFile(filepath.Join(runDir, reportName(round)))
	if err != nil {
		row.Err = err.Error()
		return row
	}
	_, findings, err := burlerengine.ParseReview(raw)
	if err != nil {
		row.Err = err.Error()
		return row
	}

	row.Findings = len(findings)
	row.Severity = map[burlerengine.Severity]int{}
	row.Class = map[burlerengine.Class]int{}
	for _, f := range findings {
		row.Severity[f.Severity]++
		row.Class[f.Class]++
		if f.Class == burlerengine.GatingClass && (f.Severity == burlerengine.SeverityBlocking || f.Severity == burlerengine.SeverityMedium) {
			row.Gating++
		}
	}
	for _, s := range severityOrder {
		if row.Severity[s] > 0 {
			row.Highest = s
			break
		}
	}
	return row
}

// recurringKeys collects the keys of ledgers 1..last whose rounds hold two or more distinct
// rounds.
// A key is reopened when an earlier ledger has it resolved and a later ledger has it open.
func recurringKeys(runDir string, last int) []recurringKey {
	type keyState struct {
		rounds   map[int]bool
		resolved bool
		reopened bool
	}
	states := map[string]*keyState{}
	for n := 1; n <= last; n++ {
		raw, err := os.ReadFile(ledgerPath(runDir, n))
		if err != nil {
			continue
		}
		ledger, err := parseLedger(raw)
		if err != nil {
			continue
		}
		for _, e := range ledger.Entries {
			st := states[e.Key]
			if st == nil {
				st = &keyState{rounds: map[int]bool{}}
				states[e.Key] = st
			}
			for _, r := range e.Rounds {
				st.rounds[r] = true
			}
			switch e.Status {
			case "resolved":
				st.resolved = true
			case "open":
				if st.resolved {
					st.reopened = true
				}
			}
		}
	}

	var out []recurringKey
	for key, st := range states {
		if len(st.rounds) < 2 {
			continue
		}
		rounds := make([]int, 0, len(st.rounds))
		for r := range st.rounds {
			rounds = append(rounds, r)
		}
		sort.Ints(rounds)
		out = append(out, recurringKey{Key: key, Rounds: rounds, Reopened: st.reopened})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// renderRoundFacts renders f as deterministic Markdown: a heading, one table with a row per round,
// and the recurring-keys list.
// Severities and classes appear in their constant order and keys sorted, so two renders of the same
// facts are byte-identical.
func renderRoundFacts(f roundFacts) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Round %d facts\n\n", f.Round)

	header := []string{"Round", "Findings"}
	for _, s := range severityOrder {
		header = append(header, string(s))
	}
	header = append(header, "Highest")
	for _, c := range classOrder {
		header = append(header, string(c))
	}
	header = append(header, "Gating")
	writeTableRow(&b, header)
	sep := make([]string, len(header))
	for i := range sep {
		sep[i] = "---"
	}
	writeTableRow(&b, sep)

	for _, row := range f.Rows {
		cells := []string{fmt.Sprint(row.Round)}
		if row.Err != "" {
			cells = append(cells, "parse error: "+escapeCell(row.Err))
			for len(cells) < len(header) {
				cells = append(cells, "")
			}
			writeTableRow(&b, cells)
			continue
		}
		cells = append(cells, fmt.Sprint(row.Findings))
		for _, s := range severityOrder {
			cells = append(cells, fmt.Sprint(row.Severity[s]))
		}
		highest := string(row.Highest)
		if highest == "" {
			highest = "-"
		}
		cells = append(cells, highest)
		for _, c := range classOrder {
			cells = append(cells, fmt.Sprint(row.Class[c]))
		}
		cells = append(cells, fmt.Sprint(row.Gating))
		writeTableRow(&b, cells)
	}

	b.WriteString("\n## Recurring keys\n\n")
	if len(f.Recurring) == 0 {
		b.WriteString("No ledger key recurs across rounds.\n")
	}
	for _, k := range f.Recurring {
		rounds := make([]string, len(k.Rounds))
		for i, r := range k.Rounds {
			rounds[i] = fmt.Sprint(r)
		}
		fmt.Fprintf(&b, "- `%s`: rounds %s", k.Key, strings.Join(rounds, ", "))
		if k.Reopened {
			b.WriteString(" (reopened)")
		}
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// writeTableRow writes one Markdown table row.
func writeTableRow(b *strings.Builder, cells []string) {
	b.WriteString("| ")
	b.WriteString(strings.Join(cells, " | "))
	b.WriteString(" |\n")
}

// escapeCell keeps an error text on one table line.
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", `\|`)
}

// writeRoundFacts computes and renders round's facts file and writes it at factsPath, overwriting
// any earlier render.
func writeRoundFacts(runDir string, round int, reportName func(int) string) error {
	content := renderRoundFacts(computeRoundFacts(runDir, round, reportName))
	path := factsPath(runDir, round)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("bouncer: write facts file %s: %w", path, err)
	}
	return nil
}
