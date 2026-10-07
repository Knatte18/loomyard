// planglyph_test.go covers ValidateFormat/Validate's composition over planparser's own checks,
// resolvePass's language: none short-circuit, and collectGlyphTargets' deduplication.

package planglyph

import (
	"errors"
	"path/filepath"
	"slices"
	"sort"
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

	got, err := resolvePass(plan, nonRepo, nil, nil)
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
// resolvePass must re-read the plan exactly when canonicalization changed it on disk, and must treat a failure of that re-read as an infrastructure error rather than silently falling back to the stale in-memory copy — which would run the resolve-backed passes against bytes no longer on disk and report a clean verdict over them.
// Both halves depend on this second return being truthful.
//
//testtiming:keep pins the rewrote return value, which TestCanonicalizeHandles_Rewrites ignores
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

	// F4's (round fable5-high-r3) regression subtest: canonicalization is idempotent, so a plan
	// whose handles are already canonical must report rewrote=false and leave every card file's
	// bytes untouched — against pre-fix source the identity substitution rewrote the files with
	// identical bytes and reported rewrote=true, forcing a full plan re-parse on every validation
	// pass for the plan's whole life.
	t.Run("an already-canonical plan rewrites nothing", func(t *testing.T) {
		dir, plan := writePlanFixture(t, map[int]string{
			1: "**Create:**\n- `plan:sub#Actual` -> `func Actual() {}`\n\n**Intent:** one\n",
			2: "**Uses:**\n- `plan:sub#Actual`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** two\n\n**ImpactSummary:** none\n",
		})
		before := readCardFile(t, dir, 1, "card1")

		_, rewrote, err := CanonicalizeHandles(plan, dir, nil)
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
}

// TestValidate_UnparseablePlanDirectoryIsAnInfrastructureError asserts a plan directory that cannot
// be read reports as a gate/infrastructure failure, never as a plan finding — the gate could not
// read the artifact, it did not find a defect in it.
//
// R6-12: it must ALSO not be reported as a quarry outage. The failure comes from parsing and writing
// the plan, so wrapping it in ErrQuarryUnavailable made every caller print "quarry could not answer"
// for a read-only _lyx/plan and sent the operator at the wrong subsystem. Both callers' non-quarry
// branch already fails the gate with an accurate, plan-named message.
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
	if err == nil {
		t.Fatal("ValidateFormat(...) error = nil; want an infrastructure failure for an unreadable plan directory")
	}
	if errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("ValidateFormat(...) error = %v; want it NOT wrapped in ErrQuarryUnavailable — the PLAN could not be read, quarry answered fine", err)
	}
	if !strings.Contains(err.Error(), planDir) {
		t.Errorf("ValidateFormat(...) error = %v; want it to name the plan directory %q an operator can act on", err, planDir)
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

// TestResolvePass_FileRenameNewSideIsNotAFinding is F1's (round fable5-high-r3) regression test: a
// FILE-rename pair's New side canonicalizes to the self glyph of a file that only exists once the
// rename lands, so it resolves not_found against the pre-rename tree — and the status policy must
// treat that exactly as planparser's own path-missing check treats Pairs.New: never a finding.
// Against pre-fix source this reported blocking glyph-not-found (twice, per F1b) and wedged every
// plan carrying a file rename, including the plan spec's own worked example.
func TestResolvePass_FileRenameNewSideIsNotAFinding(t *testing.T) {
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

// TestValidateRework asserts ValidateRework returns ValidateFormat's findings, followed by a blocking rework-first-card finding only when first_card differs from the told number.
func TestValidateRework(t *testing.T) {
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

// forthcomingFindings runs resolvePass over a two-card plan where card 1 is completed and card 2 is pending, with card 1 forthcoming only when forthcoming is true.
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

// TestResolvePass_ForthcomingCreateTargetExcludedFromStatus asserts a pending card's Uses of a Create target declared by a completed card is excluded from the status check only when that card is forthcoming.
func TestResolvePass_ForthcomingCreateTargetExcludedFromStatus(t *testing.T) {
	card1 := "**Create:**\n- `sub#Gadget`\n\n**Intent:** add the gadget\n"
	card2 := "**Edit:**\n- `sub#Foo`\n\n**Uses:**\n- `sub#Gadget`\n\n**Intent:** use the gadget\n"

	if got := forthcomingFindings(t, card1, card2, true); hasCheck(got, "glyph-not-found") {
		t.Errorf("forthcoming Create target: findings = %+v; want no glyph-not-found", got)
	}
	if got := forthcomingFindings(t, card1, card2, false); !hasCheck(got, "glyph-not-found") {
		t.Errorf("completed, not forthcoming: findings = %+v; want glyph-not-found for the Uses", got)
	}
}

// TestResolvePass_ForthcomingRenameNewSideExcludedFromStatus asserts a Rename New side declared by a forthcoming card is excluded from the status check the same way.
func TestResolvePass_ForthcomingRenameNewSideExcludedFromStatus(t *testing.T) {
	card1 := "**Rename:**\n- `sub#Thing` -> `sub#Widget`\n\n**Intent:** rename the type\n"
	card2 := "**Edit:**\n- `sub#Foo`\n\n**Uses:**\n- `sub#Widget`\n\n**Intent:** use the renamed type\n"

	if got := forthcomingFindings(t, card1, card2, true); hasCheck(got, "glyph-not-found") {
		t.Errorf("forthcoming Rename New side: findings = %+v; want no glyph-not-found", got)
	}
	if got := forthcomingFindings(t, card1, card2, false); !hasCheck(got, "glyph-not-found") {
		t.Errorf("completed, not forthcoming: findings = %+v; want glyph-not-found for the Uses", got)
	}
}

// TestResolvePass_ForthcomingCardIsNotResolved asserts a forthcoming card's own Create target that already exists draws no create-already-exists, since the card is not resolved.
func TestResolvePass_ForthcomingCardIsNotResolved(t *testing.T) {
	card1 := "**Create:**\n- `sub#Foo`\n\n**Intent:** already landed\n"
	card2 := "**Edit:**\n- `sub#Thing`\n\n**Intent:** unrelated\n"

	if got := forthcomingFindings(t, card1, card2, true); hasCheck(got, "create-already-exists") {
		t.Errorf("forthcoming card: findings = %+v; want no create-already-exists", got)
	}
}

// TestValidateDispatch_DeleteTargetGone asserts a pending card's already-absent Delete target is the informational delete-target-gone only once a card is completed, and that a Uses or Edit of the same target keeps its blocking finding.
func TestValidateDispatch_DeleteTargetGone(t *testing.T) {
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
