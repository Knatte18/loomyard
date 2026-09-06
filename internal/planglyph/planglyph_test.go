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

// TestValidateFormat_UnreadablePlanAfterCanonicalizationIsAnInfrastructureError asserts the
// post-canonicalization reload's failure is reported rather than swallowed. It used to degrade
// silently to the stale in-memory plan, so the three resolve-backed passes ran against bytes that
// were no longer on disk and the gate reported a clean-looking verdict over them.
func TestValidateFormat_UnreadablePlanAfterCanonicalizationIsAnInfrastructureError(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	// A plan carrying no handles at all, so CanonicalizeHandles returns before it rewrites anything
	// and the reload below is the only thing that can fail.
	planDir := filepath.Join(t.TempDir(), "plan")
	plan := &planparser.Plan{
		Dir:      planDir,
		Format:   5,
		Language: "go",
		Approved: true,
		Cards:    []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}}},
	}

	// planDir was never created, so ParsePlan cannot read an overview there.
	got, err := ValidateFormat(plan, root)
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Fatalf("ValidateFormat(...) error = %v; want errors.Is(err, ErrQuarryUnavailable) for an unreadable plan directory", err)
	}
	// The pure findings already collected are still returned alongside the error, per this package's
	// documented contract; what must NOT appear is any resolve-backed finding, since those passes
	// would have run against the stale in-memory plan.
	for _, f := range got {
		switch f.Check {
		case "glyph-not-found", "glyph-ambiguous", "glyph-rejected", "create-already-exists", "create-new-unit", "containment-file-overlap":
			t.Errorf("ValidateFormat(...) reported resolve-backed finding %+v; the passes must not run against a stale plan", f)
		}
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
