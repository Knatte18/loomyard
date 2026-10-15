// scope_test.go covers ScopeGuard against fake deltas, never a real quarry.Repo, since the
// function's own contract is pure comparison over an already-computed answer.

package planglyph

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// TestScopeGuard_SymbolCoverage covers the symbol check.
// A symbol is in scope when a member target or handle names it, when a rename has a side named, or when a file or package target covers every file it lives in.
// Each row is one card's targets and one delta symbol, and asserts the reported findings' files.
func TestScopeGuard_SymbolCoverage(t *testing.T) {
	sym := func(id, file string) quarry.Symbol { return quarry.Symbol{ID: id, File: file} }
	created := func(s quarry.Symbol) quarry.DeltaAnswer { return quarry.DeltaAnswer{Created: []quarry.Symbol{s}} }
	modified := func(beforeFile, afterFile string) quarry.DeltaAnswer {
		return quarry.DeltaAnswer{Modified: []quarry.ModifiedSymbol{{
			ID:     "sub#Foo",
			Before: []quarry.SymbolLocation{{File: beforeFile}},
			After:  []quarry.Symbol{sym("sub#Foo", afterFile)},
		}}}
	}
	renamed := func(fromFile, toFile string) quarry.DeltaAnswer {
		return quarry.DeltaAnswer{Renamed: []quarry.RenamedPair{{From: sym("sub#Foo", fromFile), To: sym("sub#Bar", toFile)}}}
	}

	for _, tc := range []struct {
		name    string
		targets []string
		delta   quarry.DeltaAnswer
		want    []string
	}{
		{name: "member glyph covers itself", targets: []string{"sub#Foo"}, delta: created(sym("sub#Foo", "sub/a.go"))},
		{name: "member glyph never covers its package's other members", targets: []string{"sub#Foo"}, delta: created(sym("sub#Bar", "sub/b.go")), want: []string{"sub/b.go"}},
		{name: "member handle covers the symbol it stands for", targets: []string{"plan:sub#Foo"}, delta: created(sym("sub#Foo", "sub/a.go"))},
		{name: "file self glyph covers a symbol in its file", targets: []string{"sub/a.go#"}, delta: created(sym("sub#Foo", "sub/a.go"))},
		{name: "plain file path covers a symbol in its file", targets: []string{"sub/a.go"}, delta: created(sym("sub#Foo", "sub/a.go"))},
		{name: "file target never covers another file", targets: []string{"sub/a.go#"}, delta: created(sym("sub#Bar", "sub/b.go")), want: []string{"sub/b.go"}},
		{name: "package self glyph covers a modified symbol in its directory", targets: []string{"sub#"}, delta: modified("sub/a.go", "sub/a.go")},
		{name: "directory path covers a deleted symbol in its directory", targets: []string{"sub"}, delta: quarry.DeltaAnswer{Deleted: []quarry.Symbol{sym("sub#Gone", "sub/c.go")}}},
		{name: "package self glyph never covers a subdirectory", targets: []string{"sub#"}, delta: created(sym("sub/inner#Foo", "sub/inner/a.go")), want: []string{"sub/inner/a.go"}},
		{name: "directory path never covers a subdirectory", targets: []string{"sub"}, delta: created(sym("sub/inner#Foo", "sub/inner/a.go")), want: []string{"sub/inner/a.go"}},
		{name: "modified symbol moved out of a file target stays reported", targets: []string{"sub/a.go#"}, delta: modified("sub/a.go", "sub/b.go"), want: []string{"sub/b.go"}},
		{name: "modified symbol moved into a file target stays reported", targets: []string{"sub/a.go#"}, delta: modified("sub/b.go", "sub/a.go"), want: []string{"sub/a.go"}},
		{name: "modified symbol with both files targeted is covered", targets: []string{"sub/a.go#", "sub/b.go#"}, delta: modified("sub/a.go", "sub/b.go")},
		{name: "renamed pair leaving a file target is reported", targets: []string{"sub/a.go#"}, delta: renamed("sub/a.go", "sub/b.go"), want: []string{"sub/b.go"}},
		{name: "renamed pair entering a file target is reported", targets: []string{"sub/a.go#"}, delta: renamed("sub/b.go", "sub/a.go"), want: []string{"sub/a.go"}},
		{name: "renamed pair inside a package target is covered", targets: []string{"sub#"}, delta: renamed("sub/a.go", "sub/b.go")},
		{name: "file-unit member glyph never covers the file's other symbols", targets: []string{"sub/a.go#Foo"}, delta: created(sym("sub#Bar", "sub/a.go")), want: []string{"sub/a.go"}},
		{name: "member handle never covers its package's other members", targets: []string{"plan:sub#Foo"}, delta: created(sym("sub#Bar", "sub/a.go")), want: []string{"sub/a.go"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cards := []planparser.Card{{Number: 1, Slug: "one", Targets: tc.targets}}

			var got []string
			for _, f := range ScopeGuard(cards, quarry.GitDeltaAnswer{DeltaAnswer: tc.delta}) {
				if f.Check != "scope-outside-plan" || f.Severity != SeverityInformational {
					t.Errorf("finding = %+v; want an informational scope-outside-plan", f)
				}
				got = append(got, f.File)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ScopeGuard files = %v; want %v", got, tc.want)
			}
		})
	}
}

// TestScopeGuard_FileLevelCoverage covers the file-level pass: a changed file the delta gives no symbols for is reported unless a card target's file or package directory covers it.
// A file carrying an out-of-target symbol is reported by that symbol's finding alone.
func TestScopeGuard_FileLevelCoverage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		targets []string
		created []quarry.Symbol
		path    string
		want    []string
	}{
		{name: "import-only change in a target's package", targets: []string{"sub#Foo"}, path: "sub/a.go"},
		{name: "import-only change in a subdirectory of a target's package", targets: []string{"sub#Foo"}, path: "sub/inner/a.go", want: []string{"sub/inner/a.go"}},
		{name: "external test file of an external test package target", targets: []string{"sub_test#TestFoo"}, path: "sub/a_test.go"},
		{name: "file equal to a file target", targets: []string{"docs/notes.md"}, path: "docs/notes.md"},
		{name: "other file beside a file target", targets: []string{"docs/notes.md"}, path: "docs/other.md", want: []string{"docs/other.md"}},
		{name: "non-Go file in a package target's directory", targets: []string{"sub#"}, path: "sub/README.md"},
		{name: "file in another package", targets: []string{"sub#Foo"}, path: "other/x.go", want: []string{"other/x.go"}},
		{name: "plan handle covers its package", targets: []string{"plan:sub#Bar"}, path: "sub/new.go"},
		{
			name:    "file with an out-of-target symbol gets only the symbol finding",
			targets: []string{"other#Z"},
			created: []quarry.Symbol{{ID: "sub#Bar", File: "sub/b.go"}},
			path:    "sub/b.go",
			want:    []string{"sub/b.go"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cards := []planparser.Card{{Number: 1, Slug: "one", Targets: tc.targets}}
			delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
				Created: tc.created,
				Files:   []quarry.DeltaFile{{Path: tc.path, Disposition: quarry.DispositionError}},
			}}

			var got []string
			for _, f := range ScopeGuard(cards, delta) {
				got = append(got, f.File)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ScopeGuard files = %v; want %v", got, tc.want)
			}
		})
	}
}

// TestScopeGuard_DeletedAndModifiedAlsoChecked covers a Deleted and a Modified symbol outside the
// union, each producing its own informational finding.
func TestScopeGuard_DeletedAndModifiedAlsoChecked(t *testing.T) {
	cards := []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}}}
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Deleted:  []quarry.Symbol{{ID: "sub#Gone", File: "sub/c.go"}},
		Modified: []quarry.ModifiedSymbol{{ID: "sub#Changed", After: []quarry.Symbol{{ID: "sub#Changed", File: "sub/d.go"}}}},
	}}

	got := ScopeGuard(cards, delta)
	if len(got) != 2 {
		t.Fatalf("ScopeGuard(deleted+modified outside) = %+v; want exactly two findings", got)
	}
}

// TestScopeGuard_EmptyDeltaNoFindingsNoPanic covers the caller's degradation path input: a
// zero-value delta produces no findings and no panic.
func TestScopeGuard_EmptyDeltaNoFindingsNoPanic(t *testing.T) {
	cards := []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}}}

	got := ScopeGuard(cards, quarry.GitDeltaAnswer{})
	if len(got) != 0 {
		t.Errorf("ScopeGuard(zero-value delta) = %+v; want none", got)
	}
}

// TestScopeGuard_RenamedPairOutsideTargetsIsReported covers the round-4 review's R4-21. Quarry
// removes an exact-tier pair's constituents from Created and Deleted entirely, so a renamed symbol
// reaches neither of those loops — a fork that renamed a symbol outside its batch's declared
// targets was the one kind of touch this guard never reported, and DetectDrift's gate two logs an
// unreferenced rename at Debug only, so nothing operator-visible named it.
func TestScopeGuard_RenamedPairOutsideTargetsIsReported(t *testing.T) {
	cards := []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}}}
	delta := quarry.GitDeltaAnswer{}
	delta.Renamed = []quarry.RenamedPair{{
		From: quarry.Symbol{ID: "sub#Unrelated", File: "sub/e.go"},
		To:   quarry.Symbol{ID: "sub#Renamed", File: "sub/e.go"},
	}}

	got := ScopeGuard(cards, delta)
	if len(got) != 1 {
		t.Fatalf("ScopeGuard(renamed outside) = %+v; want exactly one finding", got)
	}
	if got[0].Check != "scope-outside-plan" || got[0].Severity != SeverityInformational {
		t.Errorf("finding = %+v; want an informational scope-outside-plan", got[0])
	}
}

// TestScopeGuard_RenamedPairInsideTargetsIsNotReported covers the other half: a pair either of
// whose endpoints the batch declared is the card doing the renaming, or a card that asked for the
// new name, and is in scope on either side.
func TestScopeGuard_RenamedPairInsideTargetsIsNotReported(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target string
	}{
		{name: "old side declared", target: "sub#Foo"},
		{name: "new side declared", target: "sub#Bar"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cards := []planparser.Card{{Number: 1, Slug: "one", Targets: []string{tc.target}}}
			delta := quarry.GitDeltaAnswer{}
			delta.Renamed = []quarry.RenamedPair{{
				From: quarry.Symbol{ID: "sub#Foo", File: "sub/e.go"},
				To:   quarry.Symbol{ID: "sub#Bar", File: "sub/e.go"},
			}}

			if got := ScopeGuard(cards, delta); len(got) != 0 {
				t.Fatalf("ScopeGuard(renamed inside, %s) = %+v; want no findings", tc.name, got)
			}
		})
	}
}
