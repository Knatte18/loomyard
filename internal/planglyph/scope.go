// scope.go implements ScopeGuard, the glyph-granular replacement for the informational
// changed-files union: it compares the record-batch delta's touched symbols against the completed
// batch's own declared targets, keeping the check's informational posture exactly as it was before
// the glyph alphabet — plan-predicted impact is frequently incomplete, so a deviation alone never
// fails a fork.

package planglyph

import (
	"fmt"
	pathpkg "path"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// targetGlyphUnion returns the union of cards' own flat Targets, the comparison set ScopeGuard
// checks every touched symbol against.
//
// Each target is normalized through resolveKeyFor (donecheck.go), so a plan: handle contributes the
// expected glyph it stands for rather than its literal handle text. Without that, a Create card's
// handle-shaped target never matched the bare glyph quarry's delta reports for the symbol that card
// just created, and ScopeGuard flagged every handle-created symbol as touched outside the plan --
// the exact opposite of what the check is for.
func targetGlyphUnion(cards []planparser.Card) map[string]bool {
	union := make(map[string]bool)
	for _, c := range cards {
		for _, t := range c.Targets {
			union[resolveKeyFor(t)] = true
		}
	}
	return union
}

// ScopeGuard compares delta's Renamed, Created, Deleted and Modified symbol IDs against the union
// of cards' own target glyphs, emitting one informational finding per symbol touched outside that
// union, check ID scope-outside-plan, naming the symbol and the file it lives in.
// The finding's File is that file.
//
// It also reports, at file level, each entry of delta.Files that no symbol of the delta lives in, every disposition included, unless some card target covers it.
// A target covers a path when its file equals the path or its package directory equals the path's own directory.
// The bound is file-granular: a symbol-less change in a target's file or package directory passes, and every other one is reported, a file in a subdirectory of a target's package included.
// A file that carries an out-of-target symbol yields that symbol's finding alone.
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
			File:     file,
		})
	}

	// Renamed pairs need their own loop rather than riding the two below: quarry removes an exact
	// pair's constituents from Created and Deleted entirely (see drift.go's own note on why that
	// keeps plan-references-deleted-symbol from colliding with the exact tier), so a renamed symbol
	// reaches neither of them. Without this loop a fork that renamed a symbol outside its batch's
	// declared targets was the one kind of touch this guard never reported — and DetectDrift's gate
	// two logs an unreferenced rename at Debug only, so nothing operator-visible named it at all.
	// A pair is in scope when EITHER endpoint is a declared target: the card that declared the old
	// symbol is the card doing the renaming, and a card that declared the new one asked for it.
	for _, rp := range delta.Renamed {
		if union[rp.From.ID] || union[rp.To.ID] {
			continue
		}
		report(rp.To.ID, rp.To.File)
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

	symbolFiles := filesWithSymbols(delta)
	for _, f := range delta.Files {
		if symbolFiles[f.Path] || targetsCover(cards, f.Path) {
			continue
		}
		findings = append(findings, Finding{
			Check:    "scope-outside-plan",
			Detail:   fmt.Sprintf("file %q was changed outside the completed batch's own targets with no symbol in the delta", f.Path),
			Severity: SeverityInformational,
			File:     f.Path,
		})
	}

	return findings
}

// filesWithSymbols returns the files delta gives at least one symbol for, either side of a modified or renamed symbol included.
func filesWithSymbols(delta quarry.GitDeltaAnswer) map[string]bool {
	files := make(map[string]bool)
	for _, s := range delta.Created {
		files[s.File] = true
	}
	for _, s := range delta.Deleted {
		files[s.File] = true
	}
	for _, m := range delta.Modified {
		for _, s := range m.Before {
			files[s.File] = true
		}
		for _, s := range m.After {
			files[s.File] = true
		}
	}
	for _, rp := range delta.Renamed {
		files[rp.From.File] = true
		files[rp.To.File] = true
	}
	return files
}

// targetsCover reports whether some target of cards covers path: a file scope equal to path, or a package directory scope equal to the directory path lives in.
func targetsCover(cards []planparser.Card, path string) bool {
	dir := pathpkg.Dir(path)
	for _, c := range cards {
		for _, t := range c.Targets {
			scope, isFile, ok := planparser.RefScope(nil, t)
			if !ok {
				continue
			}
			if isFile && scope == path || !isFile && scope == dir {
				return true
			}
		}
	}
	return false
}
