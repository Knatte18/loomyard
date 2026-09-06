// planglyph_test.go covers ValidateFormat/Validate's composition over planparser's own checks,
// resolvePass's language: none short-circuit, and collectGlyphTargets' deduplication.

package planglyph

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
)

// minimalPlan returns a *planparser.Plan carrying just enough to exercise ValidateFormat/Validate
// without going through ParsePlan: Format 5, language none (so resolvePass opens no repository),
// and no cards, so every card-scoped check reports clean.
func minimalPlan(t *testing.T, dir string) *planparser.Plan {
	t.Helper()
	return &planparser.Plan{
		Dir:      dir,
		Format:   5,
		Language: "none",
		Approved: false,
	}
}

// TestValidateFormat_ConvertsFindingsUnchanged asserts ValidateFormat returns every
// planparser.ValidateFormat finding unchanged apart from the SeverityBlocking stamp, with a nil
// error under language: none.
func TestValidateFormat_ConvertsFindingsUnchanged(t *testing.T) {
	dir := t.TempDir()
	plan := minimalPlan(t, dir)
	plan.Format = 4 // deliberately wrong, to force a format-unrecognized finding.

	want := planparser.ValidateFormat(plan, dir)
	got, err := ValidateFormat(plan, dir)
	if err != nil {
		t.Fatalf("ValidateFormat(...) returned error: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("len(ValidateFormat(...)) = %d; want %d (matching planparser.ValidateFormat)", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Check != w.Check || got[i].Card != w.Card || got[i].Detail != w.Detail {
			t.Errorf("finding %d = %+v; want Check/Card/Detail matching %+v", i, got[i], w)
		}
		if got[i].Severity != SeverityBlocking {
			t.Errorf("finding %d Severity = %q; want %q", i, got[i].Severity, SeverityBlocking)
		}
	}
}

// TestValidate_AddsPlanUnapproved asserts Validate returns everything ValidateFormat does plus the
// plan-unapproved finding.
func TestValidate_AddsPlanUnapproved(t *testing.T) {
	dir := t.TempDir()
	plan := minimalPlan(t, dir)

	formatFindings, err := ValidateFormat(plan, dir)
	if err != nil {
		t.Fatalf("ValidateFormat(...) returned error: %v", err)
	}
	validateFindings, err := Validate(plan, dir)
	if err != nil {
		t.Fatalf("Validate(...) returned error: %v", err)
	}

	if len(validateFindings) != len(formatFindings)+1 {
		t.Fatalf("len(Validate(...)) = %d; want len(ValidateFormat(...))+1 = %d", len(validateFindings), len(formatFindings)+1)
	}

	foundUnapproved := false
	for _, f := range validateFindings {
		if f.Check == "plan-unapproved" {
			foundUnapproved = true
		}
	}
	if !foundUnapproved {
		t.Errorf("Validate(...) = %+v; want a plan-unapproved finding", validateFindings)
	}
}

// TestResolvePass_LanguageNoneOpensNoRepository asserts resolvePass returns cleanly, with no
// panic, no findings, and a nil error, when plan.Language is "none" even against a directory that
// is not a quarry repository at all — proving no quarry call is made.
func TestResolvePass_LanguageNoneOpensNoRepository(t *testing.T) {
	plan := minimalPlan(t, t.TempDir())
	nonRepo := t.TempDir() + "/does-not-exist"

	got, err := resolvePass(plan, nonRepo)
	if got != nil {
		t.Errorf("resolvePass(...) findings = %+v; want nil under language: none", got)
	}
	if err != nil {
		t.Errorf("resolvePass(...) error = %v; want nil under language: none", err)
	}
}

// TestValidate_QuarryUnavailableReturnsPureFindingsAlongsideTheError asserts Validate returns
// ErrQuarryUnavailable together with the pure findings it had already collected, when the
// worktree root does not name a quarry repository at all.
func TestValidate_QuarryUnavailableReturnsPureFindingsAlongsideTheError(t *testing.T) {
	dir := t.TempDir()
	plan := &planparser.Plan{Dir: dir, Format: 4, Language: "go", Approved: false} // format 4 forces a pure finding too.
	nonRepo := filepath.Join(t.TempDir(), "does-not-exist")

	got, err := Validate(plan, nonRepo)
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Fatalf("Validate(...) error = %v; want errors.Is(err, ErrQuarryUnavailable)", err)
	}
	if len(got) == 0 {
		t.Errorf("Validate(...) findings = %+v; want the pure findings collected before the quarry failure", got)
	}
}

// TestCollectGlyphTargets_DeduplicatesAcrossCards asserts a glyph referenced by two cards
// collapses into one target, and that non-glyph-shaped refs (a path, a bare symbol, a plan:
// handle) are excluded.
func TestCollectGlyphTargets_DeduplicatesAcrossCards(t *testing.T) {
	plan := &planparser.Plan{
		Cards: []planparser.Card{
			{Number: 1, Slug: "one", Targets: []string{"sub#Foo", "sub/a.go", "plan:sub#Bar"}},
			{Number: 2, Slug: "two", Targets: []string{"sub#Foo"}, Uses: []string{"sub#Foo"}},
		},
	}

	got := collectGlyphTargets(plan, glyph.Go)
	if len(got) != 1 || got[0] != "sub#Foo" {
		t.Errorf("collectGlyphTargets(...) = %v; want exactly [%q]", got, "sub#Foo")
	}
}
