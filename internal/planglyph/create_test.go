// create_test.go covers createFindings' inversion of the resolve verdict for Create groups.

package planglyph

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// createPlan builds a one-card plan whose sole Create group targets target.
func createPlan(target string) *planparser.Plan {
	return &planparser.Plan{
		Cards: []planparser.Card{
			{
				Number: 1,
				Slug:   "one",
				TargetGroups: []planparser.TargetGroup{
					{Type: planparser.CardTypeCreate, Refs: []string{target}},
				},
			},
		},
	}
}

func TestCreateFindings_AlreadyExistsFound(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#Foo"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}

	got := createFindings(createPlan("sub#Foo"), resultByTarget(results))
	if len(got) != 1 || got[0].Check != "create-already-exists" || got[0].Severity != SeverityBlocking {
		t.Fatalf("createFindings(found) = %+v; want one blocking create-already-exists finding", got)
	}
}

func TestCreateFindings_AlreadyExistsMultipart(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{
		"sub/a.go": "package sub\n\nfunc init() {}\n",
		"sub/b.go": "package sub\n\nfunc init() {}\n",
	})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#init"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}
	if results[0].Status != quarry.StatusMultipart {
		t.Fatalf("Status = %q; want %q (fixture assumption broken)", results[0].Status, quarry.StatusMultipart)
	}

	got := createFindings(createPlan("sub#init"), resultByTarget(results))
	if len(got) != 1 || got[0].Check != "create-already-exists" {
		t.Fatalf("createFindings(multipart) = %+v; want one create-already-exists finding", got)
	}
}

func TestCreateFindings_NotFoundUnitFoundPasses(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#Bar"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}
	if results[0].Status != quarry.StatusNotFound || results[0].Unit != quarry.StatusFound {
		t.Fatalf("results[0] = %+v; want not_found with unit: found", results[0])
	}

	got := createFindings(createPlan("sub#Bar"), resultByTarget(results))
	if len(got) != 0 {
		t.Errorf("createFindings(not_found, unit: found) = %+v; want no findings", got)
	}
}

func TestCreateFindings_NotFoundUnitNotFoundIsInformational(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"newpkg#Bar"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}
	if results[0].Status != quarry.StatusNotFound || results[0].Unit != quarry.StatusNotFound {
		t.Fatalf("results[0] = %+v; want not_found with unit: not_found", results[0])
	}

	got := createFindings(createPlan("newpkg#Bar"), resultByTarget(results))
	if len(got) != 1 || got[0].Check != "create-new-unit" || got[0].Severity != SeverityInformational {
		t.Fatalf("createFindings(not_found, unit: not_found) = %+v; want one informational create-new-unit finding", got)
	}
}

// TestCreateFindings_HandleTargetWithNoAnswerProducesNoFinding covers the degenerate input: an
// index carrying no entry for the handle at all leaves createFindings running cleanly with no
// finding, rather than inventing a verdict it has no answer for.
func TestCreateFindings_HandleTargetWithNoAnswerProducesNoFinding(t *testing.T) {
	got := createFindings(createPlan("plan:sub#Bar"), nil)
	if len(got) != 0 {
		t.Errorf("createFindings(handle, no answer) = %+v; want no findings", got)
	}
}

// TestCreateFindings_HandleTargetIsInverted covers the Create inversion reaching a plan: handle,
// keyed by the handle the card itself spells. The handle is the shape the plan format prescribes
// for creating something genuinely new, so leaving it out left the inversion -- and its
// misspelled-unit protection -- inert in exactly the case it exists for.
func TestCreateFindings_HandleTargetIsInverted(t *testing.T) {
	t.Run("already exists", func(t *testing.T) {
		index := map[string]quarry.ResolveResult{
			"plan:sub#Bar": {Target: "sub#Bar", Status: quarry.StatusFound},
		}
		got := createFindings(createPlan("plan:sub#Bar"), index)
		if len(got) != 1 || got[0].Check != "create-already-exists" || got[0].Severity != SeverityBlocking {
			t.Fatalf("createFindings(handle, found) = %+v; want one blocking create-already-exists finding", got)
		}
	})

	t.Run("new symbol in an existing unit passes", func(t *testing.T) {
		index := map[string]quarry.ResolveResult{
			"plan:sub#Bar": {Target: "sub#Bar", Status: quarry.StatusNotFound, Unit: quarry.StatusFound},
		}
		got := createFindings(createPlan("plan:sub#Bar"), index)
		if len(got) != 0 {
			t.Errorf("createFindings(handle, not_found/unit: found) = %+v; want no findings", got)
		}
	})

	t.Run("new unit is informational", func(t *testing.T) {
		index := map[string]quarry.ResolveResult{
			"plan:newpkg#Bar": {Target: "newpkg#Bar", Status: quarry.StatusNotFound, Unit: quarry.StatusNotFound},
		}
		got := createFindings(createPlan("plan:newpkg#Bar"), index)
		if len(got) != 1 || got[0].Check != "create-new-unit" || got[0].Severity != SeverityInformational {
			t.Fatalf("createFindings(handle, not_found/unit: not_found) = %+v; want one informational create-new-unit finding", got)
		}
		if !strings.Contains(got[0].Detail, "newpkg") {
			t.Errorf("finding detail = %q; want it to name the new unit", got[0].Detail)
		}
	})
}
