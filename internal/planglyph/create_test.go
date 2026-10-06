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

// TestCreateFindings_ResolvedTargets covers createFindings' inversion of a resolve answer for a Create target: a found or multipart target already exists and blocks, a missing symbol in an existing unit passes, and a missing symbol in a missing unit is informational.
// A glyph target is answered by a real resolve of a fixture repository.
// A plan: handle target is answered by an index keyed by the handle the card itself spells: the handle is the shape the plan format prescribes for creating something genuinely new, so leaving it out left the inversion — and its misspelled-unit protection — inert in exactly the case it exists for; an index with no entry for the handle leaves createFindings running cleanly with no finding, rather than inventing a verdict it has no answer for.
func TestCreateFindings_ResolvedTargets(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		files  map[string]string
		target string
		// index, when non-nil, answers the target directly instead of a resolve of files.
		index map[string]quarry.ResolveResult
		// wantStatus and wantUnit are the fixture assumptions about the resolve answer; an empty wantUnit skips the unit check.
		wantStatus quarry.Status
		wantUnit   quarry.Status
		// wantCheck is the one finding's check; empty means no finding.
		wantCheck    string
		wantSeverity Severity
		// wantDetail, when set, is a substring of the finding's detail.
		wantDetail string
	}{
		{
			name:         "found already exists",
			files:        map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"},
			target:       "sub#Foo",
			wantStatus:   quarry.StatusFound,
			wantCheck:    "create-already-exists",
			wantSeverity: SeverityBlocking,
		},
		{
			name: "multipart already exists",
			files: map[string]string{
				"sub/a.go": "package sub\n\nfunc init() {}\n",
				"sub/b.go": "package sub\n\nfunc init() {}\n",
			},
			target:     "sub#init",
			wantStatus: quarry.StatusMultipart,
			wantCheck:  "create-already-exists",
		},
		{
			name:       "not found in an existing unit passes",
			files:      map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"},
			target:     "sub#Bar",
			wantStatus: quarry.StatusNotFound,
			wantUnit:   quarry.StatusFound,
		},
		{
			name:         "not found in a missing unit is informational",
			files:        map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"},
			target:       "newpkg#Bar",
			wantStatus:   quarry.StatusNotFound,
			wantUnit:     quarry.StatusNotFound,
			wantCheck:    "create-new-unit",
			wantSeverity: SeverityInformational,
		},
		{
			name:   "handle with no answer produces no finding",
			target: "plan:sub#Bar",
			index:  map[string]quarry.ResolveResult{},
		},
		{
			name:         "handle already exists",
			target:       "plan:sub#Bar",
			index:        map[string]quarry.ResolveResult{"plan:sub#Bar": {Target: "sub#Bar", Status: quarry.StatusFound}},
			wantCheck:    "create-already-exists",
			wantSeverity: SeverityBlocking,
		},
		{
			name:   "handle new symbol in an existing unit passes",
			target: "plan:sub#Bar",
			index: map[string]quarry.ResolveResult{
				"plan:sub#Bar": {Target: "sub#Bar", Status: quarry.StatusNotFound, Unit: quarry.StatusFound},
			},
		},
		{
			name:   "handle new unit is informational",
			target: "plan:newpkg#Bar",
			index: map[string]quarry.ResolveResult{
				"plan:newpkg#Bar": {Target: "newpkg#Bar", Status: quarry.StatusNotFound, Unit: quarry.StatusNotFound},
			},
			wantCheck:    "create-new-unit",
			wantSeverity: SeverityInformational,
			wantDetail:   "newpkg",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			index := tc.index
			if index == nil {
				root := writeFixtureRepo(t, tc.files)
				repo, err := openRepo(root)
				if err != nil {
					t.Fatalf("openRepo(%q) returned error: %v", root, err)
				}
				results, err := resolveTargets(repo, []string{tc.target})
				if err != nil {
					t.Fatalf("resolveTargets(...) returned error: %v", err)
				}
				if results[0].Status != tc.wantStatus || (tc.wantUnit != "" && results[0].Unit != tc.wantUnit) {
					t.Fatalf("results[0] = %+v; want status %q and unit %q (fixture assumption broken)", results[0], tc.wantStatus, tc.wantUnit)
				}
				index = resultByTarget(results)
			}

			got := createFindings(createPlan(tc.target), index)
			if tc.wantCheck == "" {
				if len(got) != 0 {
					t.Errorf("createFindings(%s) = %+v; want no findings", tc.name, got)
				}
				return
			}
			if len(got) != 1 || got[0].Check != tc.wantCheck {
				t.Fatalf("createFindings(%s) = %+v; want one %s finding", tc.name, got, tc.wantCheck)
			}
			if tc.wantSeverity != "" && got[0].Severity != tc.wantSeverity {
				t.Errorf("createFindings(%s) severity = %q; want %q", tc.name, got[0].Severity, tc.wantSeverity)
			}
			if !strings.Contains(got[0].Detail, tc.wantDetail) {
				t.Errorf("finding detail = %q; want it to contain %q", got[0].Detail, tc.wantDetail)
			}
		})
	}
}

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

// TestCreateFindings_UnreadableStatusFailsClosed is R9-6's regression: the Create inversion must
// fail CLOSED on an answer it cannot read.
//
// A Create target is excluded from statusFindings by resolvePass, and statusFindings owns the only
// other reader of a result carrying no Status — quarry's pre-resolution rejection of the target
// string itself, which crucible round opus-high-r9 confirmed live is reachable for a path-shaped
// ref — so without a default arm here such a result had no reader at all and passed the inversion
// silently, which under the inversion reads as "the target does not exist yet, carry on".
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
