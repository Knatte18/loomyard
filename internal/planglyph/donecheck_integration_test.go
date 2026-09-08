//go:build integration

// donecheck_integration_test.go covers DoneChecks against real fixture repositories, since it
// resolves against the actual on-disk tree rather than a hand-built []quarry.ResolveResult.

package planglyph

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// TestDoneChecks_CreateSymbolLanded covers a Create target whose symbol landed on disk: no
// finding.
func TestDoneChecks_CreateSymbolLanded(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	cards := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"sub#Foo"}}},
	}}

	got, err := DoneChecks(&planparser.Plan{}, cards, root)
	if err != nil {
		t.Fatalf("DoneChecks(...) returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("DoneChecks(landed Create) = %+v; want no findings", got)
	}
}

// TestDoneChecks_CreateSymbolMissing covers a Create target whose symbol never landed: the
// create-not-done finding.
func TestDoneChecks_CreateSymbolMissing(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n"})
	cards := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"sub#Foo"}}},
	}}

	got, err := DoneChecks(&planparser.Plan{}, cards, root)
	if err != nil {
		t.Fatalf("DoneChecks(...) returned error: %v", err)
	}
	if len(got) != 1 || got[0].Check != "create-not-done" || got[0].Card != "1-one" {
		t.Fatalf("DoneChecks(missing Create) = %+v; want exactly one create-not-done finding on card 1-one", got)
	}
}

// TestDoneChecks_CreateHandleStripped covers a Create target spelled as a canonical plan: handle,
// which resolves via its stripped expected-glyph half rather than the literal token.
func TestDoneChecks_CreateHandleStripped(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	cards := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"plan:sub#Foo"}}},
	}}

	got, err := DoneChecks(&planparser.Plan{}, cards, root)
	if err != nil {
		t.Fatalf("DoneChecks(...) returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("DoneChecks(handle-shaped landed Create) = %+v; want no findings", got)
	}
}

// TestDoneChecks_DeleteSymbolGone covers a Delete target whose symbol is gone: no finding.
func TestDoneChecks_DeleteSymbolGone(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n"})
	cards := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeDelete, Refs: []string{"sub#Foo"}}},
	}}

	got, err := DoneChecks(&planparser.Plan{}, cards, root)
	if err != nil {
		t.Fatalf("DoneChecks(...) returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("DoneChecks(gone Delete) = %+v; want no findings", got)
	}
}

// TestDoneChecks_DeleteSymbolSurvives covers a Delete target whose symbol still resolves: the
// delete-not-done finding.
func TestDoneChecks_DeleteSymbolSurvives(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	cards := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeDelete, Refs: []string{"sub#Foo"}}},
	}}

	got, err := DoneChecks(&planparser.Plan{}, cards, root)
	if err != nil {
		t.Fatalf("DoneChecks(...) returned error: %v", err)
	}
	if len(got) != 1 || got[0].Check != "delete-not-done" || got[0].Card != "1-one" {
		t.Fatalf("DoneChecks(surviving Delete) = %+v; want exactly one delete-not-done finding on card 1-one", got)
	}
}

// TestDoneChecks_QuarryUnavailable covers a quarry-unavailable root: the infrastructure error
// blocks the done-checks rather than passing them.
func TestDoneChecks_QuarryUnavailable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	cards := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"sub#Foo"}}},
	}}

	_, err := DoneChecks(&planparser.Plan{}, cards, missing)
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("DoneChecks(...) error = %v; want errors.Is(err, ErrQuarryUnavailable)", err)
	}
}

// TestDoneChecks_RenameLanded covers a Rename pair whose rename happened: the old side no longer
// resolves, the new side does, no finding. The New side carries the canonical plan: handle
// spelling a validated plan's symbol rename always has, proving resolveKeyFor strips it.
func TestDoneChecks_RenameLanded(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Bar() {}\n"})
	cards := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{
			Type:  planparser.CardTypeRename,
			Refs:  []string{"sub#Foo", "plan:sub#Bar"},
			Pairs: []planparser.MovePair{{Old: "sub#Foo", New: "plan:sub#Bar"}},
		}},
	}}

	got, err := DoneChecks(&planparser.Plan{}, cards, root)
	if err != nil {
		t.Fatalf("DoneChecks(...) returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("DoneChecks(landed Rename) = %+v; want no findings", got)
	}
}

// TestDoneChecks_RenameSkipped is F3's (round fable5-high-r3) regression test: a fork that never
// performed its declared Rename leaves the old side still resolving and the new side still
// unresolvable, and BOTH halves must report rename-not-done — against pre-fix source this card
// recorded clean, since DoneChecks inspected only Create and Delete groups.
func TestDoneChecks_RenameSkipped(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	cards := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{
			Type:  planparser.CardTypeRename,
			Refs:  []string{"sub#Foo", "plan:sub#Bar"},
			Pairs: []planparser.MovePair{{Old: "sub#Foo", New: "plan:sub#Bar"}},
		}},
	}}

	got, err := DoneChecks(&planparser.Plan{}, cards, root)
	if err != nil {
		t.Fatalf("DoneChecks(...) returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("DoneChecks(skipped Rename) = %+v; want two rename-not-done findings (old still resolves, new still missing)", got)
	}
	for _, f := range got {
		if f.Check != "rename-not-done" || f.Card != "1-one" {
			t.Errorf("finding %+v; want Check rename-not-done on card 1-one", f)
		}
	}
}

// TestDoneChecks_FileRenameLanded covers a file-rename pair (both sides self glyphs) whose git mv
// happened: no finding.
func TestDoneChecks_FileRenameLanded(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/b.go": "package sub\n\nfunc Foo() {}\n"})
	cards := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{
			Type:  planparser.CardTypeRename,
			Refs:  []string{"sub/a.go#", "sub/b.go#"},
			Pairs: []planparser.MovePair{{Old: "sub/a.go#", New: "sub/b.go#"}},
		}},
	}}

	got, err := DoneChecks(&planparser.Plan{}, cards, root)
	if err != nil {
		t.Fatalf("DoneChecks(...) returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("DoneChecks(landed file rename) = %+v; want no findings", got)
	}
}

// TestDoneChecks_RootFilenameCreateLanded is R9-1's planglyph-side regression: a Create group
// target naming a repository-root extensionless file must reach DoneChecks already canonicalized
// to its self glyph, which quarry resolves found once the file exists.
//
// Spelled as the bare token "LICENSE" — which is what the parser produced before R9-1's fix, and
// which classifyRef rule 4 explicitly admits as a legal card ref — quarry rejects the target
// BEFORE resolution ("a glyph needs a \"#\""), doneCheckVerdicts reads that rejection as "did not
// resolve", and the card is blocked by create-not-done forever, on every retry, even though it
// created exactly what it said it would. This test pins both halves: the glyph spelling passes,
// and the bare token is still the false failure it always was, so the parser-side canonicalization
// is the thing keeping this correct.
func TestDoneChecks_RootFilenameCreateLanded(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{
		"sub/a.go": "package sub\n",
		"LICENSE":  "licence text\n",
	})

	landed := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"LICENSE#"}}},
	}}
	got, err := DoneChecks(&planparser.Plan{}, landed, root)
	if err != nil {
		t.Fatalf("DoneChecks(canonicalized root filename) returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("DoneChecks(canonicalized root filename) = %+v; want no findings", got)
	}

	bare := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"LICENSE"}}},
	}}
	stale, err := DoneChecks(&planparser.Plan{}, bare, root)
	if err != nil {
		t.Fatalf("DoneChecks(bare root filename) returned error: %v", err)
	}
	if len(stale) != 1 || stale[0].Check != "create-not-done" {
		t.Fatalf("DoneChecks(bare root filename) = %+v; want exactly one create-not-done — this is the failure canonicalization now prevents", stale)
	}
}
