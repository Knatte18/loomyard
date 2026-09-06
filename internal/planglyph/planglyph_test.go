// planglyph_test.go covers ValidateFormat/Validate's composition over planparser's own checks,
// resolvePass's language: none short-circuit, and collectGlyphTargets' deduplication.

package planglyph

import (
	"errors"
	"path/filepath"
	"strings"
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

	got, err := resolvePass(plan, nonRepo, nil)
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

// TestCanonicalizeHandles_ReportsWhetherItRewrote pins the signal resolvePass hangs its reload on.
// resolvePass must re-read the plan exactly when canonicalization changed it on disk, and must
// treat a failure of that re-read as an infrastructure error rather than silently falling back to
// the stale in-memory copy — which would run the resolve-backed passes against bytes no longer on
// disk and report a clean verdict over them. Both halves depend on this second return being
// truthful.
func TestCanonicalizeHandles_ReportsWhetherItRewrote(t *testing.T) {
	t.Run("a plan carrying no handle rewrites nothing", func(t *testing.T) {
		dir, plan := writePlanFixture(t, map[int]string{
			1: "**Edit:**\n- `sub/other.go`\n\n**Intent:** one\n\n**ImpactSummary:** none\n",
		})

		_, rewrote, err := CanonicalizeHandles(plan, dir, nil)
		if err != nil {
			t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
		}
		if rewrote {
			t.Error("CanonicalizeHandles reported a rewrite for a plan carrying no handle")
		}
	})

	t.Run("a plan carrying a handle rewrites", func(t *testing.T) {
		dir, plan := writePlanFixture(t, map[int]string{
			1: "**Create:**\n- `plan:sub#Draft` -> `func Actual() {}`\n\n**Intent:** one\n",
			2: "**Uses:**\n- `plan:sub#Draft`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** two\n\n**ImpactSummary:** none\n",
		})

		_, rewrote, err := CanonicalizeHandles(plan, dir, nil)
		if err != nil {
			t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
		}
		if !rewrote {
			t.Fatal("CanonicalizeHandles reported no rewrite despite canonicalizing a draft handle")
		}
		if got := readCardFile(t, dir, 1, "card1"); !strings.Contains(got, "plan:sub#Actual") {
			t.Errorf("card 1 = %q; want the canonical handle plan:sub#Actual", got)
		}
	})
}

// TestValidate_UnparseablePlanDirectoryIsAnInfrastructureError asserts a plan directory that cannot
// be read reports as a gate/infrastructure failure, never as a plan finding — the gate could not
// read the artifact, it did not find a defect in it.
func TestValidate_UnparseablePlanDirectoryIsAnInfrastructureError(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	planDir := filepath.Join(t.TempDir(), "never-created")
	plan := &planparser.Plan{
		Dir:      planDir,
		Format:   5,
		Language: "go",
		Approved: true,
		Cards: []planparser.Card{{
			Number:       1,
			Slug:         "one",
			Targets:      []string{"plan:sub#Draft"},
			Declarations: []planparser.CardDeclaration{{Handle: "plan:sub#Draft", Decl: "func Actual() {}"}},
		}},
	}

	got, err := ValidateFormat(plan, root)
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Fatalf("ValidateFormat(...) error = %v; want errors.Is(err, ErrQuarryUnavailable) for an unreadable plan directory", err)
	}
	// The pure findings already collected are still returned alongside the error, per this package's
	// documented contract; what must NOT appear is any resolve-backed finding, since those passes
	// would have had to run against a plan the gate could not confirm.
	for _, f := range got {
		switch f.Check {
		case "glyph-not-found", "glyph-ambiguous", "glyph-rejected", "create-already-exists", "create-new-unit", "containment-file-overlap":
			t.Errorf("ValidateFormat(...) reported resolve-backed finding %+v; want none when the plan could not be read", f)
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
