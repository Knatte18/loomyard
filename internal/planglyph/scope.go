// scope.go implements ScopeGuard, the glyph-granular replacement for the informational
// changed-files union: it compares the record-batch delta's touched symbols against the completed
// batch's own declared targets, keeping the check's informational posture exactly as it was before
// the glyph alphabet — plan-predicted impact is frequently incomplete, so a deviation alone never
// fails a fork.

package planglyph

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// targetGlyphUnion returns the union of cards' own flat Targets, the comparison set ScopeGuard
// checks every touched symbol against.
func targetGlyphUnion(cards []planparser.Card) map[string]bool {
	union := make(map[string]bool)
	for _, c := range cards {
		for _, t := range c.Targets {
			union[t] = true
		}
	}
	return union
}

// ScopeGuard compares delta's Created, Deleted and Modified symbol IDs against the union of
// cards' own target glyphs, emitting one informational finding per symbol touched outside that
// union, check ID scope-outside-plan, naming the symbol and the file it lives in.
//
// It stays informational, never blocking, for two reasons that must both survive: it preserves the
// existing, deliberate posture that plan-predicted impact is frequently incomplete so deviation
// alone never fails a fork; and, when DeltaGit itself failed, this guard is exactly the consumer
// that is allowed to degrade — an unavailable diff costs visibility, not correctness. That
// degradation is the caller's own responsibility (recordbatch.go skips this function entirely on a
// DeltaGit infrastructure error and records its own guard-could-not-run notice instead), not
// something this function detects on its own, since a zero-value delta is indistinguishable from a
// real empty one.
func ScopeGuard(cards []planparser.Card, delta quarry.GitDeltaAnswer) []Finding {
	union := targetGlyphUnion(cards)

	var findings []Finding
	seen := make(map[string]bool)
	report := func(id, file string) {
		if union[id] || seen[id] {
			return
		}
		seen[id] = true
		findings = append(findings, Finding{
			Check:    "scope-outside-plan",
			Detail:   fmt.Sprintf("symbol %q in file %q was touched outside the completed batch's own target glyphs", id, file),
			Severity: SeverityInformational,
		})
	}

	for _, s := range delta.Created {
		report(s.ID, s.File)
	}
	for _, s := range delta.Deleted {
		report(s.ID, s.File)
	}
	for _, m := range delta.Modified {
		file := ""
		if len(m.After) > 0 {
			file = m.After[0].File
		} else if len(m.Before) > 0 {
			file = m.Before[0].File
		}
		report(m.ID, file)
	}

	return findings
}
