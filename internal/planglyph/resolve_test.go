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
