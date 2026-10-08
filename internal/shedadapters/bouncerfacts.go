// bouncerfacts.go computes and renders the per-round facts file the judge reads in place of the review and fixer artifacts:
// counts per round, severity and class, and the finding keys that recur across the ledgers.
// The counts come from burlerengine.ParseReview, never a second parser,
// and the file records no review's own top-level verdict, so the judge decides from the findings alone.
// The file is an input to the judge and never one of its declared output files.

package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// recurringKey is a ledger key open in two or more distinct ledger rounds.
type recurringKey struct {
	Key      string
	Rounds   []int
	Reopened bool
}

// noOriginLabel is the label under which findings that carry no origin are counted in the lens section.
const noOriginLabel = "(no origin)"

// originCounts is the latest review's finding count for one origin value.
type originCounts struct {
	Origin   string
	Findings int
	Severity map[burlerengine.Severity]int
	Class    map[burlerengine.Class]int
}

// lensFacts is the lens section of a fanned segment's facts file.
// UsageErr is set when the round's usage record is missing or unreadable, leaving Fan, Lenses and Excluded empty;
// ReviewErr is set when the latest review does not parse, leaving Origins empty.
type lensFacts struct {
	Fan       string
	Lenses    []string
	UsageErr  string
	ReviewErr string
	Origins   []originCounts
	Excluded  []string
}

// roundFacts is everything the facts file for Round records.
// EarlierOpen lists every key open in at least one ledger before Round, with the ledger rounds it was open in,
// and EarlierLedgerErrs names each earlier ledger that is missing or fails to parse, so a degraded read never empties the list silently.
// Lenses is set only for a fanned segment.
type roundFacts struct {
	Round             int
	Rows              []roundFactsRow
	Recurring         []recurringKey
	EarlierOpen       []recurringKey
	EarlierLedgerErrs []string
	Lenses            *lensFacts
}

// computeLensFacts reads the fan and the lenses round ran from its usage record, counts round's review by origin through burlerengine.ParseReview,
// and names the lenses already excluded for the round's next judgment.
// A missing usage record or an unparseable review is recorded on the result and never fails the function.
// Every lens the round ran has an origin row, even with no findings.
func computeLensFacts(name, runDir string, round int, reportName func(int) string) lensFacts {
	var facts lensFacts
	usage, err := readRoundUsage(runDir, round)
	if err != nil {
		facts.UsageErr = escapeCell(err.Error())
	} else {
		facts.Fan = usage.Fan
		facts.Lenses = usage.Lenses
		facts.Excluded, _ = excludedLenses(name, runDir, round, usage.Fan)
	}

	byOrigin := map[string]*originCounts{}
	countsFor := func(origin string) *originCounts {
		if byOrigin[origin] == nil {
			byOrigin[origin] = &originCounts{Origin: origin, Severity: map[burlerengine.Severity]int{}, Class: map[burlerengine.Class]int{}}
		}
		return byOrigin[origin]
	}
	for _, lens := range facts.Lenses {
		countsFor("lens:" + lens)
	}

	raw, err := os.ReadFile(filepath.Join(runDir, reportName(round)))
	if err == nil {
		var findings []burlerengine.Finding
		_, findings, err = burlerengine.ParseReview(raw)
		for _, f := range findings {
			origin := f.Origin
			if origin == "" {
				origin = noOriginLabel
			}
			counts := countsFor(origin)
			counts.Findings++
			counts.Severity[f.Severity]++
			counts.Class[f.Class]++
		}
	}
	if err != nil {
		facts.ReviewErr = escapeCell(err.Error())
		return facts
	}
	for _, counts := range byOrigin {
		facts.Origins = append(facts.Origins, *counts)
	}
	sort.Slice(facts.Origins, func(i, j int) bool { return facts.Origins[i].Origin < facts.Origins[j].Origin })
	return facts
}

// computeRoundFacts reads rounds 1..round's review files and rounds 1..round-1's ledgers inside runDir.
// A review that is missing or fails to parse yields a row carrying the error text,
// and a ledger that fails to parse is skipped from the key lists and named in EarlierLedgerErrs, so the function never fails.
func computeRoundFacts(runDir string, round int, reportName func(int) string) roundFacts {
	facts := roundFacts{Round: round}
	for n := 1; n <= round; n++ {
		facts.Rows = append(facts.Rows, reviewFactsRow(runDir, n, reportName))
	}
	facts.Recurring = recurringKeys(runDir, round-1)
	for key, h := range ledgerHistories(runDir, round-1) {
		if len(h.openRounds) > 0 {
			facts.EarlierOpen = append(facts.EarlierOpen, recurringKey{Key: key, Rounds: h.openRounds, Reopened: h.reopened})
		}
	}
	sort.Slice(facts.EarlierOpen, func(i, j int) bool { return facts.EarlierOpen[i].Key < facts.EarlierOpen[j].Key })
	for n := 1; n < round; n++ {
		raw, err := os.ReadFile(ledgerPath(runDir, n))
		if err == nil {
			_, err = parseLedger(raw)
		}
		if err != nil {
			facts.EarlierLedgerErrs = append(facts.EarlierLedgerErrs, fmt.Sprintf("ledger round %d: %s", n, escapeCell(err.Error())))
		}
	}
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

// keyHistory is what ledgers 1..last say about one key:
// the ledger rounds in which its entry was open, ascending, and whether an open entry followed a resolved one.
type keyHistory struct {
	openRounds []int
	resolved   bool
	reopened   bool
}

// ledgerHistories reads ledgers 1..last inside runDir and returns each key's keyHistory.
// The rounds are the ledgers' own round numbers, never the entry's judge-written rounds list.
// An unreadable or unparseable ledger contributes nothing.
func ledgerHistories(runDir string, last int) map[string]*keyHistory {
	histories := map[string]*keyHistory{}
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
			h := histories[e.Key]
			if h == nil {
				h = &keyHistory{}
				histories[e.Key] = h
			}
			switch e.Status {
			case "resolved":
				h.resolved = true
			case "open":
				if h.resolved {
					h.reopened = true
				}
				if len(h.openRounds) == 0 || h.openRounds[len(h.openRounds)-1] != n {
					h.openRounds = append(h.openRounds, n)
				}
			}
		}
	}
	return histories
}

// recurringKeys collects the keys that ledgers 1..last have open in two or more distinct rounds.
// A key is reopened when an earlier ledger has it resolved and a later ledger has it open.
func recurringKeys(runDir string, last int) []recurringKey {
	var out []recurringKey
	for key, h := range ledgerHistories(runDir, last) {
		if len(h.openRounds) < 2 {
			continue
		}
		out = append(out, recurringKey{Key: key, Rounds: h.openRounds, Reopened: h.reopened})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// isGatingEntry reports whether e is gating by the judge stencil's definition:
// a design finding at MEDIUM or BLOCKING, or any BLOCKING.
// An unlabelled entry is not gating.
func isGatingEntry(e ledgerEntry) bool {
	if e.Severity == burlerengine.SeverityBlocking {
		return true
	}
	return e.Class == burlerengine.GatingClass && e.Severity == burlerengine.SeverityMedium
}

// circlingEvidence returns, sorted, the keys that are gating and open in ledger round and were open in an earlier ledger too.
// Only a key open in two rounds counts, so a rising finding count or new keys alone yield nothing.
// An unreadable or unparseable ledger round yields no evidence.
func circlingEvidence(runDir string, round int) []string {
	raw, err := os.ReadFile(ledgerPath(runDir, round))
	if err != nil {
		return nil
	}
	ledger, err := parseLedger(raw)
	if err != nil {
		return nil
	}
	earlier := ledgerHistories(runDir, round-1)
	var out []string
	for _, e := range ledger.Entries {
		if e.Status != "open" || !isGatingEntry(e) {
			continue
		}
		if h := earlier[e.Key]; h != nil && len(h.openRounds) > 0 && !slices.Contains(out, e.Key) {
			out = append(out, e.Key)
		}
	}
	sort.Strings(out)
	return out
}

// renderRoundFacts renders f as deterministic Markdown: a heading, one table with a row per round, the recurring-keys list, and the list of keys open in an earlier round with a parse-error line per unreadable earlier ledger.
// Severities and classes appear in their constant order and keys sorted, so two renders of the same facts are byte-identical.
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

	b.WriteString("\n## Recurring keys (ledger rounds each key was open in)\n\n")
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

	b.WriteString("\n## Keys open in an earlier round (ledger rounds each key was open in)\n\n")
	if len(f.EarlierOpen) == 0 {
		b.WriteString("No ledger key was open in an earlier round.\n")
	}
	for _, k := range f.EarlierOpen {
		rounds := make([]string, len(k.Rounds))
		for i, r := range k.Rounds {
			rounds[i] = fmt.Sprint(r)
		}
		fmt.Fprintf(&b, "- `%s`: rounds %s\n", k.Key, strings.Join(rounds, ", "))
	}
	for _, errLine := range f.EarlierLedgerErrs {
		fmt.Fprintf(&b, "- parse error: %s\n", errLine)
	}
	if f.Lenses != nil {
		renderLensFacts(&b, *f.Lenses)
	}
	return []byte(b.String())
}

// renderLensFacts writes the "## Lenses" section: the fan, the lenses run, the lenses already excluded and a table of the latest review's findings per origin.
// A missing usage record or an unparseable review renders as one line saying so in place of what it would have supplied.
func renderLensFacts(b *strings.Builder, l lensFacts) {
	b.WriteString("\n## Lenses (the latest round's findings per origin)\n\n")
	if l.UsageErr != "" {
		fmt.Fprintf(b, "Usage record unreadable, so the fan and lenses run are unknown: %s\n", l.UsageErr)
	} else {
		fmt.Fprintf(b, "Fan: `%s`\n\n", l.Fan)
		fmt.Fprintf(b, "Lenses run: %s\n\n", joinOrNone(l.Lenses))
		fmt.Fprintf(b, "Lenses already excluded for the next round: %s\n", joinOrNone(l.Excluded))
	}
	if l.ReviewErr != "" {
		fmt.Fprintf(b, "\nReview unparseable, so findings per origin are unknown: %s\n", l.ReviewErr)
		return
	}
	b.WriteString("\n")
	header := []string{"Origin", "Findings"}
	for _, s := range severityOrder {
		header = append(header, string(s))
	}
	for _, c := range classOrder {
		header = append(header, string(c))
	}
	writeTableRow(b, header)
	sep := make([]string, len(header))
	for i := range sep {
		sep[i] = "---"
	}
	writeTableRow(b, sep)
	for _, o := range l.Origins {
		cells := []string{"`" + o.Origin + "`", fmt.Sprint(o.Findings)}
		for _, s := range severityOrder {
			cells = append(cells, fmt.Sprint(o.Severity[s]))
		}
		for _, c := range classOrder {
			cells = append(cells, fmt.Sprint(o.Class[c]))
		}
		writeTableRow(b, cells)
	}
}

// joinOrNone joins names with ", " and renders none as "none".
func joinOrNone(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
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

// writeRoundFacts computes and renders round's facts file and writes it at factsPath, overwriting any earlier render.
// The lens section is computed, identifying itself as name in its warnings, only when clusterExcludes is true, so a solo segment's file carries none.
func writeRoundFacts(name, runDir string, round int, reportName func(int) string, clusterExcludes bool) error {
	facts := computeRoundFacts(runDir, round, reportName)
	if clusterExcludes {
		lenses := computeLensFacts(name, runDir, round, reportName)
		facts.Lenses = &lenses
	}
	content := renderRoundFacts(facts)
	path := factsPath(runDir, round)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("bouncer: write facts file %s: %w", path, err)
	}
	return nil
}
