// create_test.go covers createFindings' inversion of the resolve verdict for Create groups.

package planglyph

import (
	"errors"
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

// TestCreateFindings_UnreadableStatusFailsClosed is R9-6's regression: the Create inversion must
// fail CLOSED on an answer it cannot read.
//
// A Create target is excluded from statusFindings by resolvePass, and statusFindings owns the only
// other reader of a result carrying no Status — quarry's pre-resolution rejection of the target
// string itself, which crucible round opus-high-r9 confirmed live is reachable for a path-shaped
// ref — so without a default arm here such a result had no reader at all and passed the inversion
// silently, which under the inversion reads as "the target does not exist yet, carry on".
// TestCreateFindings_AmbiguousIsAlreadyExists drives a REAL ambiguous answer — two declarations of
// the same name, in two files of one package, which quarry resolves ambiguous — and pins both
// halves of F3's fix (crucible round opus5-high-r1): the disposition stays blocking, and the check
// ID and detail now name the hazard that actually occurred.
//
// Before the fix this arm fell through to default/glyph-rejected and rendered via
// unreadableStatusDetail as `Create target "sub#Dup" answered the unrecognized resolve status
// "ambiguous"` — false, since ambiguous is one of quarry's four documented statuses, and it pointed
// the operator at quarry rather than at the colliding declarations in their own tree.
func TestCreateFindings_AmbiguousIsAlreadyExists(t *testing.T) {
	root := writeFixtureRepo(t, map[string]string{
		"sub/a.go": "package sub\n\nfunc Dup() {}\n",
		"sub/b.go": "package sub\n\nfunc Dup() {}\n",
	})
	repo, err := openRepo(root)
	if err != nil {
		t.Fatalf("openRepo(%q) returned error: %v", root, err)
	}
	results, err := resolveTargets(repo, []string{"sub#Dup"})
	if err != nil {
		t.Fatalf("resolveTargets(...) returned error: %v", err)
	}
	if got := results[0].Status; got != quarry.StatusAmbiguous {
		t.Fatalf("fixture did not produce the status under test: resolve(sub#Dup).Status = %q; want %q", got, quarry.StatusAmbiguous)
	}

	got := createFindings(createPlan("sub#Dup"), resultByTarget(results))
	if len(got) != 1 {
		t.Fatalf("createFindings(ambiguous) = %+v; want exactly one finding", got)
	}
	if got[0].Check != "create-already-exists" || got[0].Severity != SeverityBlocking {
		t.Errorf("createFindings(ambiguous) = %+v; want a blocking create-already-exists finding", got[0])
	}
	if strings.Contains(got[0].Detail, "unrecognized") {
		t.Errorf("finding detail = %q; ambiguous is a status quarry documents and must never be reported as unrecognized", got[0].Detail)
	}
	if !strings.Contains(got[0].Detail, "ambiguous") || !strings.Contains(got[0].Detail, "sub#Dup") {
		t.Errorf("finding detail = %q; want it to name both the ambiguity and the colliding candidates", got[0].Detail)
	}
	// Every candidate's declaring FILE must be named too: the constructible Go ambiguity is the
	// same name declared twice in one unit, where every candidate shares one glyph ID, so an
	// ID-only detail read "ambiguous among: X, X" and located neither declaration (crucible round
	// fable5-high-r2, F-R2-2).
	for _, cand := range results[0].Candidates {
		if cand.File == "" {
			t.Fatalf("fixture candidate %+v carries no File; the fixture assumption behind this assertion broke", cand)
		}
		if !strings.Contains(got[0].Detail, cand.File) {
			t.Errorf("finding detail = %q; want it to locate candidate %q via its file %q", got[0].Detail, cand.ID, cand.File)
		}
	}
}

func TestCreateFindings_UnreadableStatusFailsClosed(t *testing.T) {
	t.Run("pre-resolution rejection is blocking glyph-rejected", func(t *testing.T) {
		index := map[string]quarry.ResolveResult{
			"plan:sub#Bar": {Target: "sub#Bar", Error: "a glyph needs a \"#\"", Reason: "no_separator"},
		}
		got := createFindings(createPlan("plan:sub#Bar"), index)
		if len(got) != 1 || got[0].Check != "glyph-rejected" || got[0].Severity != SeverityBlocking {
			t.Fatalf("createFindings(pre-resolution rejection) = %+v; want one blocking glyph-rejected finding", got)
		}
		if !strings.Contains(got[0].Detail, "no_separator") {
			t.Errorf("finding detail = %q; want it to carry quarry's own reason", got[0].Detail)
		}
	})

	t.Run("a status outside quarry's vocabulary is blocking glyph-rejected", func(t *testing.T) {
		index := map[string]quarry.ResolveResult{
			"plan:sub#Bar": {Target: "sub#Bar", Status: "partially_found"},
		}
		got := createFindings(createPlan("plan:sub#Bar"), index)
		if len(got) != 1 || got[0].Check != "glyph-rejected" || got[0].Severity != SeverityBlocking {
			t.Fatalf("createFindings(unrecognized status) = %+v; want one blocking glyph-rejected finding", got)
		}
		if !strings.Contains(got[0].Detail, "partially_found") {
			t.Errorf("finding detail = %q; want it to name the unrecognized status", got[0].Detail)
		}
	})
}

// TestMatchHandleResults pins F3's per-key guard (crucible round fable-high-r10): a Create handle
// whose expected glyph got no answer is ErrQuarryUnavailable, never a silent drop — createFindings
// reads an absent index entry as "not a glyph target" and would skip the Create inversion entirely,
// which under the inversion means "does not exist yet, carry on".
func TestMatchHandleResults(t *testing.T) {
	expected := map[string]string{"plan:sub#New": "sub#New"}

	byHandle, err := matchHandleResults(expected, []quarry.ResolveResult{{Target: "sub#New", Status: quarry.StatusNotFound}})
	if err != nil {
		t.Fatalf("matchHandleResults() with a covering answer returned error: %v", err)
	}
	if _, ok := byHandle["plan:sub#New"]; !ok {
		t.Errorf("matchHandleResults() = %v; want the answer re-keyed by the handle %q", byHandle, "plan:sub#New")
	}

	_, err = matchHandleResults(expected, nil)
	if err == nil {
		t.Fatalf("matchHandleResults() with no answer = nil error; want a wrapped ErrQuarryUnavailable — the handle's Create inversion would otherwise silently pass")
	}
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("matchHandleResults() error = %v; want it to wrap ErrQuarryUnavailable", err)
	}
	if !strings.Contains(err.Error(), "plan:sub#New") {
		t.Errorf("matchHandleResults() error = %v; want it to name the unanswered handle", err)
	}
}
