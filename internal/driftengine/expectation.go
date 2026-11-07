// expectation.go parses `_lym/drift/expected.yaml`, the expectation the expect agent writes.
// It is the only parser of that file and models which cells the expectation covers.

package driftengine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Expectation is the changes the expect agent expects a task to make to the captures.
// The zero Expectation is the empty expectation, which is valid and covers nothing.
type Expectation struct {
	// Cases maps a case name to what is expected of it.
	Cases map[string]ExpectedCase
	// Added lists the cases only the current capture is expected to hold.
	Added []CaseEntry
	// Removed lists the cases only the baseline capture is expected to hold.
	Removed []CaseEntry
}

// ExpectedCase is what is expected of one case.
// Under DiffLines it holds Keys and Lines and no Why; under DiffCommand it holds a Why and nothing else.
type ExpectedCase struct {
	// Keys are the keyed values expected to change.
	Keys []KeyEntry
	// Lines are the unkeyed line ranges expected to change.
	Lines []LineEntry
	// Why is why the case's compare is expected to differ, under DiffCommand only.
	Why string
}

// KeyEntry expects a keyed value to change.
type KeyEntry struct {
	// Key is the key of the value.
	Key string
	// N is the occurrence of the key among the file's lines holding it, counted from 1; zero covers every occurrence.
	N int
	// Added marks a key the baseline does not hold, which the expectation expects the current capture to add.
	Added bool
	// Why is why the change is expected.
	Why string
}

// LineEntry expects unkeyed lines to change.
type LineEntry struct {
	// Range is `12`, `12-20` or the open `12-`, positions among the file's unkeyed lines counted from 1.
	Range string `yaml:"range"`
	// Why is why the change is expected.
	Why string `yaml:"why"`
}

// CaseEntry expects a whole case to appear or disappear.
type CaseEntry struct {
	// Case is the case's name.
	Case string `yaml:"case"`
	// Why is why the case is expected to appear or disappear.
	Why string `yaml:"why"`
}

// Coverage is what covers a cell: the covering entry's why, and whether that entry is an open range.
type Coverage struct {
	Why  string
	Open bool
}

// rawKeyEntry is a key entry as expected.yaml spells it; N is a pointer so an absent `n` reads apart from `n: 0`.
type rawKeyEntry struct {
	Key   string `yaml:"key"`
	N     *int   `yaml:"n"`
	Added bool   `yaml:"added"`
	Why   string `yaml:"why"`
}

// rawCase is an expected case as expected.yaml spells it.
type rawCase struct {
	Keys  []rawKeyEntry `yaml:"keys"`
	Lines []LineEntry   `yaml:"lines"`
	Why   string        `yaml:"why"`
}

// rawExpectation is expected.yaml as it is spelled, before validation.
type rawExpectation struct {
	Cases   map[string]rawCase `yaml:"cases"`
	Added   []CaseEntry        `yaml:"added"`
	Removed []CaseEntry        `yaml:"removed"`
}

// lineRange is a parsed range of unkeyed line positions; an open range has no upper bound.
type lineRange struct {
	low, high int
	open      bool
}

// parseLineRange parses `N`, `N-M` with 1 <= N <= M, or the open `N-`.
func parseLineRange(text string) (lineRange, error) {
	low, high, hasDash := strings.Cut(text, "-")
	first, err := strconv.Atoi(low)
	if err != nil || first < 1 {
		return lineRange{}, fmt.Errorf("range %q is not N, N-M or N- with N at least 1", text)
	}
	switch {
	case !hasDash:
		return lineRange{low: first, high: first}, nil
	case high == "":
		return lineRange{low: first, open: true}, nil
	}
	last, err := strconv.Atoi(high)
	if err != nil || last < first {
		return lineRange{}, fmt.Errorf("range %q is not N, N-M or N- with N at most M", text)
	}
	return lineRange{low: first, high: last}, nil
}

// holds reports whether position lies in the range.
func (r lineRange) holds(position int) bool {
	return position >= r.low && (r.open || position <= r.high)
}

// ParseExpectation parses expected.yaml's bytes under mode, DiffLines or DiffCommand.
// It checks them against the baseline's case listings.
// The file is agent-written, so an unknown key is refused.
// An empty file and an empty document parse as the empty expectation.
//
// Under DiffLines it refuses a case in neither baseline nor `added`, and a `removed` case absent from baseline.
// Under DiffLines it also refuses a key entry the baseline's case listing lacks unless it carries `added: true`.
// Under DiffLines it also refuses a range that is not `N`, `N-M` or `N-`, and a case-level `why`.
// Under DiffCommand it refuses a `keys` or `lines` entry and checks no case name against baseline.
// Under both it refuses a blank `why`, an `n` below 1 and an unknown key.
// Each refusal names the entry and the rule.
func ParseExpectation(data []byte, mode string, baseline []CaseListing) (Expectation, error) {
	if mode != DiffLines && mode != DiffCommand {
		return Expectation{}, fmt.Errorf("expected.yaml: mode %q is neither %q nor %q", mode, DiffLines, DiffCommand)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var raw rawExpectation
	if err := decoder.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return Expectation{}, fmt.Errorf("expected.yaml: %w", err)
	}

	listings := make(map[string]CaseListing, len(baseline))
	for _, listing := range baseline {
		listings[listing.Case] = listing
	}
	added, err := checkCaseEntries("added", raw.Added, nil)
	if err != nil {
		return Expectation{}, err
	}
	var removedAgainst map[string]CaseListing
	if mode == DiffLines {
		removedAgainst = listings
	}
	if _, err := checkCaseEntries("removed", raw.Removed, removedAgainst); err != nil {
		return Expectation{}, err
	}

	expectation := Expectation{Added: raw.Added, Removed: raw.Removed}
	names := make([]string, 0, len(raw.Cases))
	for name := range raw.Cases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		parsed, err := checkCase(name, raw.Cases[name], mode, listings, added)
		if err != nil {
			return Expectation{}, err
		}
		if expectation.Cases == nil {
			expectation.Cases = map[string]ExpectedCase{}
		}
		expectation.Cases[name] = parsed
	}
	return expectation, nil
}

// checkCaseEntries checks each entry of an `added` or `removed` list and returns the case names it holds.
// Its errors name the list.
// When against is not nil, every named case must be one of its listings.
func checkCaseEntries(list string, entries []CaseEntry, against map[string]CaseListing) (map[string]bool, error) {
	names := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.Case == "" {
			return nil, fmt.Errorf("expected.yaml: an entry under %q has no case", list)
		}
		if strings.TrimSpace(entry.Why) == "" {
			return nil, fmt.Errorf("expected.yaml: %s case %q has an empty why", list, entry.Case)
		}
		if against != nil {
			if _, inBaseline := against[entry.Case]; !inBaseline {
				return nil, fmt.Errorf("expected.yaml: %s case %q is not a case of the baseline", list, entry.Case)
			}
		}
		names[entry.Case] = true
	}
	return names, nil
}

// checkCase validates one expected case and returns it as the model.
func checkCase(name string, raw rawCase, mode string, listings map[string]CaseListing, added map[string]bool) (ExpectedCase, error) {
	if mode == DiffCommand {
		if len(raw.Keys) > 0 || len(raw.Lines) > 0 {
			return ExpectedCase{}, fmt.Errorf("expected.yaml: case %q holds keys or lines, which only diff %q takes; under diff %q a case holds one why", name, DiffLines, DiffCommand)
		}
		if strings.TrimSpace(raw.Why) == "" {
			return ExpectedCase{}, fmt.Errorf("expected.yaml: case %q has an empty why", name)
		}
		return ExpectedCase{Why: raw.Why}, nil
	}

	listing, inBaseline := listings[name]
	if !inBaseline && !added[name] {
		return ExpectedCase{}, fmt.Errorf("expected.yaml: case %q is neither a case of the baseline nor listed under added", name)
	}
	if raw.Why != "" {
		return ExpectedCase{}, fmt.Errorf("expected.yaml: case %q has a case-level why, which only diff %q takes; give each key and line entry its own", name, DiffCommand)
	}

	occurrences := map[string]int{}
	for _, line := range listing.Lines {
		if line.Identity.Key != "" {
			occurrences[line.Identity.Key]++
		}
	}
	parsed := ExpectedCase{}
	for _, entry := range raw.Keys {
		keyEntry, err := checkKeyEntry(name, entry, occurrences)
		if err != nil {
			return ExpectedCase{}, err
		}
		parsed.Keys = append(parsed.Keys, keyEntry)
	}
	for _, entry := range raw.Lines {
		if _, err := parseLineRange(entry.Range); err != nil {
			return ExpectedCase{}, fmt.Errorf("expected.yaml: case %q lines entry: %w", name, err)
		}
		if strings.TrimSpace(entry.Why) == "" {
			return ExpectedCase{}, fmt.Errorf("expected.yaml: case %q lines entry %q has an empty why", name, entry.Range)
		}
		parsed.Lines = append(parsed.Lines, entry)
	}
	return parsed, nil
}

// checkKeyEntry validates one key entry of caseName against the key counts of its baseline listing.
func checkKeyEntry(caseName string, entry rawKeyEntry, occurrences map[string]int) (KeyEntry, error) {
	label := fmt.Sprintf("case %q keys entry %q", caseName, entry.Key)
	if entry.Key == "" {
		return KeyEntry{}, fmt.Errorf("expected.yaml: case %q has a keys entry with no key", caseName)
	}
	occurrence := 0
	if entry.N != nil {
		if *entry.N < 1 {
			return KeyEntry{}, fmt.Errorf("expected.yaml: %s has n %d, which is below 1", label, *entry.N)
		}
		occurrence = *entry.N
	}
	if strings.TrimSpace(entry.Why) == "" {
		return KeyEntry{}, fmt.Errorf("expected.yaml: %s has an empty why", label)
	}
	if !entry.Added && occurrences[entry.Key] < max(occurrence, 1) {
		if occurrence > 0 && occurrences[entry.Key] > 0 {
			return KeyEntry{}, fmt.Errorf("expected.yaml: %s names occurrence %d, but the baseline holds %d; mark it added: true if the current capture adds it", label, occurrence, occurrences[entry.Key])
		}
		return KeyEntry{}, fmt.Errorf("expected.yaml: %s is not a key of the baseline's case; mark it added: true if the current capture adds it", label)
	}
	return KeyEntry{Key: entry.Key, N: occurrence, Added: entry.Added, Why: entry.Why}, nil
}

// Merge returns the receiver's entries followed by more's.
// It lets containment read the sealed file and every amendment as one expectation.
// A case in both keeps the receiver's why when it has one.
func (e Expectation) Merge(more Expectation) Expectation {
	merged := Expectation{
		Added:   append(append([]CaseEntry(nil), e.Added...), more.Added...),
		Removed: append(append([]CaseEntry(nil), e.Removed...), more.Removed...),
	}
	if len(e.Cases)+len(more.Cases) > 0 {
		merged.Cases = map[string]ExpectedCase{}
	}
	for _, source := range []map[string]ExpectedCase{e.Cases, more.Cases} {
		for name, expected := range source {
			into := merged.Cases[name]
			into.Keys = append(into.Keys, expected.Keys...)
			into.Lines = append(into.Lines, expected.Lines...)
			if into.Why == "" {
				into.Why = expected.Why
			}
			merged.Cases[name] = into
		}
	}
	return merged
}

// coversKey reports whether a key entry of caseName covers the keyed cell.
func coversKey(caseName string, entry KeyEntry, cell Cell) bool {
	return cell.Case == caseName && cell.Identity.Key == entry.Key && (entry.N == 0 || entry.N == cell.Identity.N)
}

// coversLine reports whether a line entry of caseName covers the unkeyed cell.
// An entry whose range does not parse covers nothing.
func coversLine(caseName string, entry LineEntry, cell Cell) (covers, open bool) {
	if cell.Case != caseName || cell.Identity.Key != "" || cell.Identity.Line < 1 {
		return false, false
	}
	span, err := parseLineRange(entry.Range)
	if err != nil {
		return false, false
	}
	return span.holds(cell.Identity.Line), span.open
}

// Cover returns the first entry that covers the cell, and false when none does.
// An added case cell is covered by an `added` entry, a removed case cell by a `removed` entry.
// A keyed cell is covered by a key entry of its case with its key and either no `n` or its occurrence.
// An unkeyed cell is covered by a line entry whose range holds its position.
// A line entry never covers a keyed cell.
func (e Expectation) Cover(c Cell) (Coverage, bool) {
	switch c.Kind {
	case CellAddedCase:
		return coverCase(e.Added, c.Case)
	case CellRemovedCase:
		return coverCase(e.Removed, c.Case)
	}
	expected := e.Cases[c.Case]
	if c.Identity.Key != "" {
		for _, entry := range expected.Keys {
			if coversKey(c.Case, entry, c) {
				return Coverage{Why: entry.Why}, true
			}
		}
		return Coverage{}, false
	}
	for _, entry := range expected.Lines {
		if covers, open := coversLine(c.Case, entry, c); covers {
			return Coverage{Why: entry.Why, Open: open}, true
		}
	}
	return Coverage{}, false
}

// coverCase returns the first entry naming caseName.
func coverCase(entries []CaseEntry, caseName string) (Coverage, bool) {
	for _, entry := range entries {
		if entry.Case == caseName {
			return Coverage{Why: entry.Why}, true
		}
	}
	return Coverage{}, false
}

// Uncovering names every entry that covers none of cells.
// Each name reads as `added case "x"`, `removed case "x"`, `key "k" (n 2) of case "x"` or `lines 12-20 of case "x"`.
// The names are in a fixed order: added, removed, then each case by name with its keys before its lines.
func (e Expectation) Uncovering(cells []Cell) []string {
	var uncovered []string
	for _, entry := range e.Added {
		if !anyCell(cells, func(cell Cell) bool { return cell.Kind == CellAddedCase && cell.Case == entry.Case }) {
			uncovered = append(uncovered, fmt.Sprintf("added case %q", entry.Case))
		}
	}
	for _, entry := range e.Removed {
		if !anyCell(cells, func(cell Cell) bool { return cell.Kind == CellRemovedCase && cell.Case == entry.Case }) {
			uncovered = append(uncovered, fmt.Sprintf("removed case %q", entry.Case))
		}
	}
	names := make([]string, 0, len(e.Cases))
	for name := range e.Cases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, entry := range e.Cases[name].Keys {
			if !anyCell(cells, func(cell Cell) bool { return coversKey(name, entry, cell) }) {
				label := fmt.Sprintf("key %q", entry.Key)
				if entry.N > 0 {
					label += fmt.Sprintf(" (n %d)", entry.N)
				}
				uncovered = append(uncovered, fmt.Sprintf("%s of case %q", label, name))
			}
		}
		for _, entry := range e.Cases[name].Lines {
			if !anyCell(cells, func(cell Cell) bool { covers, _ := coversLine(name, entry, cell); return covers }) {
				uncovered = append(uncovered, fmt.Sprintf("lines %s of case %q", entry.Range, name))
			}
		}
	}
	return uncovered
}

// anyCell reports whether match holds for any of cells.
func anyCell(cells []Cell, match func(Cell) bool) bool {
	for _, cell := range cells {
		if match(cell) {
			return true
		}
	}
	return false
}
