// resolve_test.go covers statusFindings, table-driven against a small fixture repository built
// under t.TempDir().

package planglyph

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// resolveFixture is the fixture source every test in this file resolves against: Foo is a free
// function, Thing is a type with one member method.
const resolveFixture = "package sub\n\nfunc Foo() {}\n\ntype Thing struct{}\n\nfunc (t Thing) Method() {}\n"

func TestStatusFindings(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": resolveFixture})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}

	card := planparser.Card{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}}
	plan := &planparser.Plan{Cards: []planparser.Card{card}}

	t.Run("Found", func(t *testing.T) {
		results, err := resolveTargets(repo, []string{"sub#Foo"})
		if err != nil {
			t.Fatalf("resolveTargets(...) returned error: %v", err)
		}
		got := statusFindings(plan, results)
		if len(got) != 0 {
			t.Errorf("statusFindings(found) = %+v; want no findings", got)
		}
	})

	t.Run("Multipart", func(t *testing.T) {
		multiFixture := map[string]string{
			"sub/a.go": "package sub\n\nfunc init() {}\n",
			"sub/b.go": "package sub\n\nfunc init() {}\n",
		}
		multiRoot := writeFixtureRepo(t, multiFixture)
		multiRepo, err := openRepo(multiRoot)
		if err != nil {
			t.Fatalf("openRepo(%q) returned error: %v", multiRoot, err)
		}
		results, err := resolveTargets(multiRepo, []string{"sub#init"})
		if err != nil {
			t.Fatalf("resolveTargets(...) returned error: %v", err)
		}
		if results[0].Status != quarry.StatusMultipart {
			t.Fatalf("Status = %q; want %q (fixture assumption broken)", results[0].Status, quarry.StatusMultipart)
		}
		multiPlan := &planparser.Plan{Cards: []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"sub#init"}}}}
		got := statusFindings(multiPlan, results)
		if len(got) != 0 {
			t.Errorf("statusFindings(multipart) = %+v; want no findings", got)
		}
	})

	t.Run("Ambiguous", func(t *testing.T) {
		ambRoot := writeFixtureRepo(t, map[string]string{
			"amb/a.go": "package amb\n\nfunc Foo() {}\n",
			"amb/b.go": "package amb\n\nfunc Foo() {}\n",
		})
		ambRepo, err := openRepo(ambRoot)
		if err != nil {
			t.Fatalf("openRepo(%q) returned error: %v", ambRoot, err)
		}
		results, err := resolveTargets(ambRepo, []string{"amb#Foo"})
		if err != nil {
			t.Fatalf("resolveTargets(...) returned error: %v", err)
		}
		if results[0].Status != quarry.StatusAmbiguous {
			t.Fatalf("Status = %q; want %q (fixture assumption broken)", results[0].Status, quarry.StatusAmbiguous)
		}
		ambPlan := &planparser.Plan{Cards: []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"amb#Foo"}}}}
		got := statusFindings(ambPlan, results)
		if len(got) != 1 || got[0].Check != "glyph-ambiguous" {
			t.Fatalf("statusFindings(ambiguous) = %+v; want one glyph-ambiguous finding", got)
		}
		for _, cand := range results[0].Candidates {
			if !strings.Contains(got[0].Detail, cand.ID) {
				t.Errorf("Detail = %q; want it to list candidate %q", got[0].Detail, cand.ID)
			}
			// The declaring file is required alongside the ID: both fixture candidates share the
			// glyph ID amb#Foo, so an ID-only detail named the collision twice without locating
			// either declaration (crucible round fable5-high-r2, F-R2-2).
			if cand.File == "" {
				t.Fatalf("fixture candidate %+v carries no File; the fixture assumption behind this assertion broke", cand)
			}
			if !strings.Contains(got[0].Detail, cand.File) {
				t.Errorf("Detail = %q; want it to locate candidate %q via its file %q", got[0].Detail, cand.ID, cand.File)
			}
		}
	})

	t.Run("NotFoundUnitFound", func(t *testing.T) {
		results, err := resolveTargets(repo, []string{"sub#DoesNotExist"})
		if err != nil {
			t.Fatalf("resolveTargets(...) returned error: %v", err)
		}
		if results[0].Status != quarry.StatusNotFound || results[0].Unit != quarry.StatusFound {
			t.Fatalf("results[0] = %+v; want not_found with unit: found", results[0])
		}
		nfPlan := &planparser.Plan{Cards: []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"sub#DoesNotExist"}}}}
		got := statusFindings(nfPlan, results)
		if len(got) != 1 || got[0].Check != "glyph-not-found" {
			t.Fatalf("statusFindings(not_found, unit: found) = %+v; want one glyph-not-found finding", got)
		}
		if !strings.Contains(got[0].Detail, "member") {
			t.Errorf("Detail = %q; want it to name a misspelled member", got[0].Detail)
		}
	})

	t.Run("NotFoundUnitNotFound", func(t *testing.T) {
		results, err := resolveTargets(repo, []string{"missing#Foo"})
		if err != nil {
			t.Fatalf("resolveTargets(...) returned error: %v", err)
		}
		if results[0].Status != quarry.StatusNotFound || results[0].Unit != quarry.StatusNotFound {
			t.Fatalf("results[0] = %+v; want not_found with unit: not_found", results[0])
		}
		nfPlan := &planparser.Plan{Cards: []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"missing#Foo"}}}}
		got := statusFindings(nfPlan, results)
		if len(got) != 1 || got[0].Check != "glyph-not-found" {
			t.Fatalf("statusFindings(not_found, unit: not_found) = %+v; want one glyph-not-found finding", got)
		}
		if !strings.Contains(got[0].Detail, "unit") {
			t.Errorf("Detail = %q; want it to name a misspelled unit", got[0].Detail)
		}
	})

	t.Run("Rejected", func(t *testing.T) {
		results, err := resolveTargets(repo, []string{"/absolute#Foo"})
		if err != nil {
			t.Fatalf("resolveTargets(...) returned error: %v", err)
		}
		if results[0].Status != "" || results[0].Error == "" {
			t.Fatalf("results[0] = %+v; want a pre-resolution rejection", results[0])
		}
		rejPlan := &planparser.Plan{Cards: []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"/absolute#Foo"}}}}
		got := statusFindings(rejPlan, results)
		if len(got) != 1 || got[0].Check != "glyph-rejected" {
			t.Fatalf("statusFindings(rejected) = %+v; want one glyph-rejected finding", got)
		}
		if !strings.Contains(got[0].Detail, results[0].Error) {
			t.Errorf("Detail = %q; want it to carry Error %q", got[0].Detail, results[0].Error)
		}
	})
}

// TestTargetCards_IndexesACardOncePerTarget is F1b's (round fable5-high-r3) regression test: both
// endpoints of every Pairs entry are also projected into Targets, so a Rename card references its
// own endpoints twice — and targetCards must still attribute one finding per card, not one per
// occurrence.
func TestTargetCards_IndexesACardOncePerTarget(t *testing.T) {
	card := planparser.Card{
		Number:  1,
		Slug:    "one",
		Targets: []string{"sub/a.go#", "sub/b.go#"},
		Pairs:   []planparser.MovePair{{Old: "sub/a.go#", New: "sub/b.go#"}},
	}
	plan := &planparser.Plan{Cards: []planparser.Card{card}}

	index := targetCards(plan)
	for _, ref := range []string{"sub/a.go#", "sub/b.go#"} {
		if got := len(index[ref]); got != 1 {
			t.Errorf("targetCards(...)[%q] has %d entries; want 1 — one attribution per card, not per occurrence", ref, got)
		}
	}
}

// TestStatusFindings_UnrecognizedStatusFailsClosed is R9-6's sibling regression on the status
// policy: quarry's four-value Status vocabulary is closed today, and a value outside it must fail
// closed rather than fall out of the switch with no finding, so widening that vocabulary can only
// ever be a deliberate change here.
func TestStatusFindings_UnrecognizedStatusFailsClosed(t *testing.T) {
	plan := &planparser.Plan{Cards: []planparser.Card{{
		Number: 1, Slug: "one",
		Targets: []string{"sub#Bar"},
	}}}

	got := statusFindings(plan, []quarry.ResolveResult{{Target: "sub#Bar", Status: "partially_found"}})
	if len(got) != 1 || got[0].Check != "glyph-rejected" || got[0].Severity != SeverityBlocking {
		t.Fatalf("statusFindings(unrecognized status) = %+v; want one blocking glyph-rejected finding", got)
	}
	if !strings.Contains(got[0].Detail, "partially_found") {
		t.Errorf("finding detail = %q; want it to name the unrecognized status", got[0].Detail)
	}
}

// TestCandidateList pins the shared ambiguous-candidate renderer both glyph-ambiguous
// (statusFindings) and create-already-exists (createFindings) delegate to: a candidate carrying a
// File is located as "ID (file)", one without keeps the bare ID, so identically-named declarations
// — which share one glyph ID — stay tellable apart by their files (crucible round fable5-high-r2,
// F-R2-2).
func TestCandidateList(t *testing.T) {
	tests := []struct {
		name       string
		candidates []quarry.Symbol
		want       string
	}{
		{
			"same ID in two files stays distinguishable",
			[]quarry.Symbol{
				{ID: "amb#Foo", File: "amb/a.go"},
				{ID: "amb#Foo", File: "amb/b.go"},
			},
			"amb#Foo (amb/a.go), amb#Foo (amb/b.go)",
		},
		{
			"a candidate with no file keeps the bare ID",
			[]quarry.Symbol{{ID: "amb#Foo"}},
			"amb#Foo",
		},
		{
			"mixed presence renders each candidate on its own terms",
			[]quarry.Symbol{
				{ID: "amb#Foo", File: "amb/a.go"},
				{ID: "amb#Bar"},
			},
			"amb#Foo (amb/a.go), amb#Bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := candidateList(tt.candidates); got != tt.want {
				t.Errorf("candidateList(...) = %q; want %q", got, tt.want)
			}
		})
	}
}
