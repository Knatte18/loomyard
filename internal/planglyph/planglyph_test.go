// planglyph_test.go covers the composition of ValidateFormat and Validate over planparser's own checks.
// It also covers the language: none short-circuit of resolvePass and the deduplication in collectGlyphTargets.

package planglyph

import (
	"errors"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// minimalPlan returns a *planparser.Plan for tests of ValidateFormat and Validate that skips ParsePlan.
// It sets Format 5 and language none, so resolvePass opens no repository.
// It has no cards, so every card-scoped check reports clean.
func minimalPlan(t *testing.T, dir string) *planparser.Plan {
	t.Helper()
	return &planparser.Plan{
		Dir:      dir,
		Format:   5,
		Language: "none",
		Approved: false,
	}
}

// TestValidateFormat_ConvertsFindingsUnchanged asserts that ValidateFormat returns planparser's findings unchanged:
// each keeps its Check, Card and Detail, and its Severity is SeverityBlocking.
// Under language: none the returned error is nil.
func TestValidateFormat_ConvertsFindingsUnchanged(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestValidate_AddsPlanUnapproved asserts that Validate returns everything ValidateFormat returns.
// It also returns the plan-unapproved finding.
func TestValidate_AddsPlanUnapproved(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestResolvePass_LanguageNoneOpensNoRepository asserts that resolvePass returns cleanly when plan.Language is "none":
// no panic, no findings and a nil error, even against a directory that is not a quarry repository.
// That proves no quarry call is made.
func TestResolvePass_LanguageNoneOpensNoRepository(t *testing.T) {
	tiergate.Require(t, 0)
	plan := minimalPlan(t, t.TempDir())
	nonRepo := t.TempDir() + "/does-not-exist"

	got, err := resolvePass(plan, nonRepo, nil, nil)
	if got != nil {
		t.Errorf("resolvePass(...) findings = %+v; want nil under language: none", got)
	}
	if err != nil {
		t.Errorf("resolvePass(...) error = %v; want nil under language: none", err)
	}
}

// TestValidate_QuarryUnavailableReturnsPureFindingsAlongsideTheError asserts Validate returns ErrQuarryUnavailable:
// it also returns the pure findings it had already collected, when the worktree root does not name a quarry repository.
func TestValidate_QuarryUnavailableReturnsPureFindingsAlongsideTheError(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestCanonicalizeHandles_ReportsWhetherItRewrote pins the signal that resolvePass hangs its reload on.
// resolvePass must re-read the plan exactly when canonicalization changed it on disk.
// It must treat a failed re-read as an infrastructure error, not fall back silently to the stale in-memory copy:
// that would run the resolve-backed passes against bytes not on disk, and report a clean verdict over them.
// Both halves depend on the second return value being truthful.
//
//testtiming:keep pins the rewrote return value, which TestCanonicalizeHandles_Rewrites ignores
func TestCanonicalizeHandles_ReportsWhetherItRewrote(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("a plan carrying no handle rewrites nothing", func(t *testing.T) {
		_, plan := writePlanFixture(t, map[int]string{
			1: "**Edit:**\n- `sub/other.go`\n\n**Intent:** one\n\n**ImpactSummary:** none\n",
		})

		_, rewrote, err := CanonicalizeHandles(plan, "", nil)
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

		_, rewrote, err := CanonicalizeHandles(plan, "", nil)
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

	// Canonicalization is idempotent, so a plan whose handles are already canonical must report rewrote=false:
	// it must leave every card file's bytes untouched.
	// Reporting rewrote=true for identical bytes would force a full plan re-parse on every validation pass.
	t.Run("an already-canonical plan rewrites nothing", func(t *testing.T) {
		dir, plan := writePlanFixture(t, map[int]string{
			1: "**Create:**\n- `plan:sub#Actual` -> `func Actual() {}`\n\n**Intent:** one\n",
			2: "**Uses:**\n- `plan:sub#Actual`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** two\n\n**ImpactSummary:** none\n",
		})
		before := readCardFile(t, dir, 1, "card1")

		_, rewrote, err := CanonicalizeHandles(plan, "", nil)
		if err != nil {
			t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
		}
		if rewrote {
			t.Error("CanonicalizeHandles reported a rewrite for an already-canonical plan")
		}
		if got := readCardFile(t, dir, 1, "card1"); got != before {
			t.Errorf("card 1 bytes changed across an identity canonicalization:\nbefore: %q\nafter:  %q", before, got)
		}
	})

	t.Run("an overlaid plan is rewritten in memory and the plan directory stays byte-identical", func(t *testing.T) {
		dir, onDisk := writePlanFixture(t, map[int]string{
			1: "**Edit:**\n- `sub/other.go`\n\n**Intent:** one\n\n**ImpactSummary:** none\n",
		})
		before := readCardFile(t, dir, 1, "card1")
		overlay, err := planparser.NewOverlay(dir, map[string]string{
			"01-card1.md": "# Card 1 — card1\n\n**Create:**\n- `plan:sub#Draft` -> `func Actual() {}`\n\n**Intent:** one\n",
		})
		if err != nil {
			t.Fatalf("NewOverlay(...) returned error: %v", err)
		}
		plan, err := overlay.Parse()
		if err != nil {
			t.Fatalf("Overlay.Parse() returned error: %v", err)
		}
		if plan.Dir != onDisk.Dir {
			t.Fatalf("overlaid plan Dir = %q; want %q", plan.Dir, onDisk.Dir)
		}

		_, rewrote, err := CanonicalizeHandles(plan, "", nil)
		if err != nil {
			t.Fatalf("CanonicalizeHandles(...) returned error: %v", err)
		}
		if !rewrote {
			t.Error("CanonicalizeHandles reported no rewrite for an overlaid draft handle")
		}
		if got := readCardFile(t, dir, 1, "card1"); got != before {
			t.Errorf("card 1 on disk = %q; want it byte-identical to %q", got, before)
		}
		reparsed, err := plan.Reparse()
		if err != nil {
			t.Fatalf("Reparse() returned error: %v", err)
		}
		if got := reparsed.Cards[0].Declarations; len(got) != 1 || got[0].Handle != "plan:sub#Actual" {
			t.Errorf("reparsed card 1 declarations = %+v; want the canonical handle plan:sub#Actual", got)
		}
	})
}

// TestValidate_UnparseablePlanDirectoryIsAnInfrastructureError asserts an unreadable plan directory:
// it is reported as a gate or infrastructure failure, never as a plan finding.
// The gate could not read the artifact; it found no defect in it.
//
// It must also not be reported as a quarry outage.
// The failure comes from parsing and writing the plan.
// Such a report would send the operator to the wrong subsystem.
// The callers' non-quarry branch fails the gate with an accurate message that names the plan.
func TestValidate_UnparseablePlanDirectoryIsAnInfrastructureError(t *testing.T) {
	tiergate.Require(t, 0)
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
	if err == nil {
		t.Fatal("ValidateFormat(...) error = nil; want an infrastructure failure for an unreadable plan directory")
	}
	if errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("ValidateFormat(...) error = %v; want it NOT wrapped in ErrQuarryUnavailable — the PLAN could not be read, quarry answered fine", err)
	}
	if !strings.Contains(err.Error(), planDir) {
		t.Errorf("ValidateFormat(...) error = %v; want it to name the plan directory %q an operator can act on", err, planDir)
	}
	// The pure findings collected so far are still returned alongside the error.
	// Resolve-backed findings must not appear: those passes would run against a plan the gate could not confirm.
	for _, f := range got {
		switch f.Check {
		case "glyph-not-found", "glyph-ambiguous", "glyph-rejected", "create-already-exists", "create-new-unit":
			t.Errorf("ValidateFormat(...) reported resolve-backed finding %+v; want none when the plan could not be read", f)
		}
	}
}

// TestCollectGlyphTargets_DeduplicatesAcrossCards asserts a glyph referenced by two cards collapses into one target.
// Non-glyph-shaped refs (a path, a bare symbol, a plan: handle) are excluded.
func TestCollectGlyphTargets_DeduplicatesAcrossCards(t *testing.T) {
	tiergate.Require(t, 0)
	plan := &planparser.Plan{
		Cards: []planparser.Card{
			{Number: 1, Slug: "one", Targets: []string{"sub#Foo", "sub/a.go", "plan:sub#Bar"}},
			{Number: 2, Slug: "two", Targets: []string{"sub#Foo"}, Uses: []string{"sub#Foo"}},
		},
	}

	got := collectGlyphTargets(plan)
	if len(got) != 1 || got[0] != "sub#Foo" {
		t.Errorf("collectGlyphTargets(...) = %v; want exactly [%q]", got, "sub#Foo")
	}
}

// TestResolvePass_FileRenameNewSideIsNotAFinding asserts that a file rename's New side is never a finding.
// The New side canonicalizes to the self glyph of a file that exists only once the rename lands,
// so it resolves not_found against the pre-rename tree.
// The status policy must not report that not_found as a finding.
func TestResolvePass_FileRenameNewSideIsNotAFinding(t *testing.T) {
	tiergate.Require(t, 0)
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": resolveFixture})
	_, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub/a.go` -> `sub/b.go`\n\n**Intent:** rename the file\n",
	})

	got, err := resolvePass(plan, root, nil, nil)
	if err != nil {
		t.Fatalf("resolvePass(...) returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("resolvePass(file-rename plan) = %+v; want no findings — the pair's New side names the post-rename destination", got)
	}
}

// TestValidateRework asserts ValidateRework returns ValidateFormat's findings,
// and it adds a blocking rework-first-card finding only when first_card differs from the told number.
func TestValidateRework(t *testing.T) {
	tiergate.Require(t, 0)
	dir := t.TempDir()
	plan := minimalPlan(t, dir)
	plan.Format = 4 // forces a format-unrecognized finding so the prefix is non-empty.
	plan.FirstCard = 3

	base, err := ValidateFormat(plan, dir)
	if err != nil {
		t.Fatalf("ValidateFormat(...) returned error: %v", err)
	}

	t.Run("Match", func(t *testing.T) {
		got, err := ValidateRework(plan, dir, 3)
		if err != nil {
			t.Fatalf("ValidateRework(...) returned error: %v", err)
		}
		if len(got) != len(base) {
			t.Fatalf("ValidateRework(...) = %v; want exactly ValidateFormat's %v", got, base)
		}
		for i := range base {
			if got[i] != base[i] {
				t.Errorf("finding %d = %+v; want %+v", i, got[i], base[i])
			}
		}
	})

	t.Run("Mismatch", func(t *testing.T) {
		got, err := ValidateRework(plan, dir, 7)
		if err != nil {
			t.Fatalf("ValidateRework(...) returned error: %v", err)
		}
		if len(got) != len(base)+1 {
			t.Fatalf("ValidateRework(...) = %v; want ValidateFormat's findings plus one", got)
		}
		for i := range base {
			if got[i] != base[i] {
				t.Errorf("finding %d = %+v; want %+v", i, got[i], base[i])
			}
		}
		last := got[len(got)-1]
		if last.Check != "rework-first-card" || last.Severity != SeverityBlocking {
			t.Errorf("last finding = %+v; want a blocking rework-first-card", last)
		}
	})
}

// forthcomingFindings runs resolvePass over a two-card plan in which card 1 is completed and card 2 is pending.
// Card 1 is marked forthcoming only when the forthcoming argument is true.
func forthcomingFindings(t *testing.T, card1, card2 string, forthcoming bool) []Finding {
	t.Helper()
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": resolveFixture})
	_, plan := writePlanFixture(t, map[int]string{1: card1, 2: card2})
	done := map[string]bool{plan.Cards[0].ID(): true}
	var fc map[string]bool
	if forthcoming {
		fc = done
	}
	got, err := resolvePass(plan, root, done, fc)
	if err != nil {
		t.Fatalf("resolvePass(...) returned error: %v", err)
	}
	return got
}

func hasCheck(findings []Finding, check string) bool {
	for _, f := range findings {
		if f.Check == check {
			return true
		}
	}
	return false
}

// TestResolvePass_ForthcomingCreateTargetExcludedFromStatus asserts the status check excludes a Create target.
// A pending card's Uses of that target are excluded only when the completed card declaring it is forthcoming.
func TestResolvePass_ForthcomingCreateTargetExcludedFromStatus(t *testing.T) {
	tiergate.Require(t, 0)
	card1 := "**Create:**\n- `sub#Gadget`\n\n**Intent:** add the gadget\n"
	card2 := "**Edit:**\n- `sub#Foo`\n\n**Uses:**\n- `sub#Gadget`\n\n**Intent:** use the gadget\n"

	if got := forthcomingFindings(t, card1, card2, true); hasCheck(got, "glyph-not-found") {
		t.Errorf("forthcoming Create target: findings = %+v; want no glyph-not-found", got)
	}
	if got := forthcomingFindings(t, card1, card2, false); !hasCheck(got, "glyph-not-found") {
		t.Errorf("completed, not forthcoming: findings = %+v; want glyph-not-found for the Uses", got)
	}
}

// TestValidateFormat_PartScope asserts the plan-gate form keeps a file self glyph out of the status policy.
// The exclusion applies to a glyph the part scope's Paths hold;
// without the scope, it is reported as glyph-not-found.
func TestValidateFormat_PartScope(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := writeFixtureRepo(t, map[string]string{"sub/a.go": resolveFixture})
	card := "**Edit:**\n- `sub#Foo`\n\n**Uses:**\n- `sub/gadget.go#`\n\n**Intent:** use the file another part creates\n"

	for _, tt := range []struct {
		name string
		part *planparser.PartScope
		want bool
	}{
		{"scope holds the file", &planparser.PartScope{Name: "p", Paths: []string{"sub/gadget.go"}}, false},
		{"no scope", nil, true},
	} {
		_, plan := writePlanFixture(t, map[int]string{1: card})
		plan.Part = tt.part
		got, err := ValidateFormat(plan, root)
		if err != nil {
			t.Fatalf("%s: ValidateFormat(...) returned error: %v", tt.name, err)
		}
		if hasCheck(got, "glyph-not-found") != tt.want {
			t.Errorf("%s: findings = %+v; want glyph-not-found = %v", tt.name, got, tt.want)
		}
	}
}

// TestResolvePass_ForthcomingRenameNewSideExcludedFromStatus asserts the status check excludes a Rename New side.
// A pending card's Uses of the New side are excluded only when the completed card declaring it is forthcoming.
func TestResolvePass_ForthcomingRenameNewSideExcludedFromStatus(t *testing.T) {
	tiergate.Require(t, 0)
	card1 := "**Rename:**\n- `sub#Thing` -> `sub#Widget`\n\n**Intent:** rename the type\n"
	card2 := "**Edit:**\n- `sub#Foo`\n\n**Uses:**\n- `sub#Widget`\n\n**Intent:** use the renamed type\n"

	if got := forthcomingFindings(t, card1, card2, true); hasCheck(got, "glyph-not-found") {
		t.Errorf("forthcoming Rename New side: findings = %+v; want no glyph-not-found", got)
	}
	if got := forthcomingFindings(t, card1, card2, false); !hasCheck(got, "glyph-not-found") {
		t.Errorf("completed, not forthcoming: findings = %+v; want glyph-not-found for the Uses", got)
	}
}

// TestResolvePass_ForthcomingCardIsNotResolved asserts a forthcoming card draws no create-already-exists.
// The card is not resolved, so its own Create target, which already exists, draws no such finding.
func TestResolvePass_ForthcomingCardIsNotResolved(t *testing.T) {
	tiergate.Require(t, 0)
	card1 := "**Create:**\n- `sub#Foo`\n\n**Intent:** already landed\n"
	card2 := "**Edit:**\n- `sub#Thing`\n\n**Intent:** unrelated\n"

	if got := forthcomingFindings(t, card1, card2, true); hasCheck(got, "create-already-exists") {
		t.Errorf("forthcoming card: findings = %+v; want no create-already-exists", got)
	}
}

// TestValidateDispatch_DeleteTargetGone asserts a pending card's already-absent Delete target is informational.
// It is reported as delete-target-gone only once a card is completed.
// A Uses or Edit of the same target keeps its blocking finding.
func TestValidateDispatch_DeleteTargetGone(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const done = "**Edit:**\n- `sub#Foo`\n\n**Intent:** already landed\n"
	deleteGone := func(ref string) string {
		return "**Delete:**\n- `" + ref + "`\n\n**Intent:** remove it\n"
	}

	cases := []struct {
		name      string
		cards     map[int]string
		completed int
		want      []string
	}{
		{
			name:      "an absent member glyph is already deleted",
			cards:     map[int]string{1: done, 2: deleteGone("sub#Gone")},
			completed: 1,
			want:      []string{"delete-target-gone|2-card2|informational|sub#Gone"},
		},
		{
			name:      "an absent file path is already deleted",
			cards:     map[int]string{1: done, 2: deleteGone("sub/gone.go")},
			completed: 1,
			want:      []string{"delete-target-gone|2-card2|informational|sub/gone.go#"},
		},
		{
			name: "a Uses of the same target in another card keeps its blocking finding",
			cards: map[int]string{
				1: done,
				2: deleteGone("sub#Gone"),
				3: "**Edit:**\n- `sub#Foo`\n\n**Uses:**\n- `sub#Gone`\n\n**Intent:** read it\n",
			},
			completed: 1,
			want: []string{
				"delete-target-gone|2-card2|informational|sub#Gone",
				"glyph-not-found|3-card3|blocking|sub#Gone",
			},
		},
		{
			name: "an Edit of the same target in the deleting card keeps its blocking finding",
			cards: map[int]string{
				1: done,
				2: "**Edit:**\n- `sub#Gone`\n\n**Delete:**\n- `sub#Gone`\n\n**Intent:** both\n",
			},
			completed: 1,
			want:      []string{"glyph-not-found|2-card2|blocking|sub#Gone"},
		},
		{
			name:      "no completed card keeps the blocking finding",
			cards:     map[int]string{1: done, 2: deleteGone("sub#Gone")},
			completed: 0,
			want:      []string{"glyph-not-found|2-card2|blocking|sub#Gone"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := writeFixtureRepo(t, map[string]string{"sub/a.go": resolveFixture})
			_, plan := writePlanFixture(t, tc.cards)

			got, err := ValidateDispatch(plan, root, plan.Cards[:tc.completed], nil)
			if err != nil {
				t.Fatalf("ValidateDispatch(...) returned error: %v", err)
			}

			var gotKeys []string
			for _, f := range got {
				switch f.Check {
				case "delete-target-gone", "glyph-not-found", "path-missing":
					gotKeys = append(gotKeys, f.Check+"|"+f.Card+"|"+string(f.Severity)+"|"+f.Ref)
				}
			}
			sort.Strings(gotKeys)
			sort.Strings(tc.want)
			if !slices.Equal(gotKeys, tc.want) {
				t.Errorf("ValidateDispatch(...) findings = %v; want %v (all findings: %+v)", gotKeys, tc.want, got)
			}
		})
	}
}
