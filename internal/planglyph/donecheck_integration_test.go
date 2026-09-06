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
