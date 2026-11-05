// donecheck_test.go covers doneCheckVerdicts, the pure half of DoneChecks: the three done rules applied to a hand-built answer index, plus the coverage guard that refuses an answer set which does not cover a target the caller asked about.
// It reads no repository at all, only the candidate files an ambiguous row writes under a temp root — DoneChecks' own resolve-backed half is covered by donecheck_resolve_test.go instead.

package planglyph

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// TestDoneCheckVerdicts_UncoveredTargetIsInfrastructureNotAPass is the sabotage-proof for R5-6: an
// answer index missing a target the entries name must NOT let that entry's blocking check pass
// silently. Every key reaching doneCheckVerdicts was put into the resolve's own target list by
// DoneChecks, so a missing answer is quarry's positional contract not holding — infrastructure,
// never a verdict on the plan.
func TestDoneCheckVerdicts_UncoveredTargetIsInfrastructureNotAPass(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	entries := []doneCheckEntry{{
		card:    planparser.Card{Number: 3, Slug: "add-thing"},
		checkID: "create-not-done",
		key:     "internal/greet#Farewell",
		display: "internal/greet#Farewell",
	}}

	findings, err := doneCheckVerdicts(&planparser.Plan{}, "", entries, map[string]quarry.ResolveResult{}, nil)

	if err == nil {
		t.Fatalf("doneCheckVerdicts() with an uncovered target = (%v, nil); want a wrapped ErrQuarryUnavailable — passing the check would be a false success", findings)
	}
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("doneCheckVerdicts() error = %v; want it to wrap ErrQuarryUnavailable so a caller can tell an outage from a plan defect", err)
	}
	if !strings.Contains(err.Error(), "internal/greet#Farewell") {
		t.Errorf("doneCheckVerdicts() error = %v; want it to name the uncovered target", err)
	}
	if len(findings) != 0 {
		t.Errorf("doneCheckVerdicts() findings = %v; want none — the uncovered target produced no verdict either way", findings)
	}
}

// TestDoneCheckVerdicts_Rules pins the three done rules against a fully covering answer index, so the guard above cannot be satisfied by a version that simply always errors.
// A blocking row also pins the detail's lookup: the key, quarry's status, the unit status of a missing member, each ambiguous candidate's file and build constraint, and for a Create or Rename new side answered not_found the unit's files and the name's declarations elsewhere.
// An ambiguous row writes its candidate files under a temp root, where a pair partitioned by build constraints counts as resolved and so lands a Create and a Rename new side but still blocks a Delete and a Rename old side.
//
//testtiming:keep pins the ambiguous rows and the per-arm rule table, which the resolve-backed DoneChecks tests reach only for found and not_found
func TestDoneCheckVerdicts_Rules(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const key = "internal/greet#Thing"
	card := planparser.Card{Number: 1, Slug: "card"}
	const declaration = "\n\npackage greet\n\nfunc Thing() {}\n"
	partitionedFiles := map[string]string{
		"internal/greet/a.go": "//go:build linux" + declaration,
		"internal/greet/b.go": "//go:build !linux" + declaration,
	}
	overlappingFiles := map[string]string{
		"internal/greet/a.go": "//go:build linux" + declaration,
		"internal/greet/b.go": "//go:build amd64" + declaration,
	}
	candidateFiles := []string{"internal/greet/a.go", "internal/greet/b.go"}
	// declarations is the worktree listing every row is handed: one member of the key's unit, and the key's name declared in another unit.
	declarations := []quarry.Symbol{
		{Glyph: glyph.Glyph{Lang: glyph.Go, Unit: "internal/greet", Name: "Other"}, ID: "internal/greet#Other", File: "internal/greet/greet.go"},
		{Glyph: glyph.Glyph{Lang: glyph.Go, Unit: "internal/other", Name: "Thing"}, ID: "internal/other#Thing", File: "internal/other/thing.go"},
	}
	const searched = "quarry searched 1 file of unit internal/greet; Thing is declared as internal/other#Thing in internal/other/thing.go"

	tests := []struct {
		name string
		// checkID is the arm under test.
		checkID string
		status  quarry.Status
		// unit is the answer's unit status, read for not_found only.
		unit quarry.Status
		// files are written under the row's temp root, and candidates name the ambiguous answer's candidate files.
		files      map[string]string
		candidates []string
		wantCheck  string
		// wantDetail holds substrings of the one finding's detail.
		wantDetail []string
	}{
		{"create landed", "create-not-done", quarry.StatusFound, "", nil, nil, "", nil},
		{"create missing, member missing", "create-not-done", quarry.StatusNotFound, quarry.StatusFound, nil, nil, "create-not-done",
			[]string{key, "quarry answered not_found", "the unit exists and the member is missing", searched}},
		{"create missing, unit missing", "create-not-done", quarry.StatusNotFound, quarry.StatusNotFound, nil, nil, "create-not-done",
			[]string{key, "quarry answered not_found", "the unit is missing"}},
		{"delete gone", "delete-not-done", quarry.StatusNotFound, "", nil, nil, "", nil},
		{"delete still there", "delete-not-done", quarry.StatusFound, "", nil, nil, "delete-not-done",
			[]string{key, "quarry answered found"}},
		{"rename old gone", "rename-not-done-old", quarry.StatusNotFound, "", nil, nil, "", nil},
		{"rename old still there", "rename-not-done-old", quarry.StatusFound, "", nil, nil, "rename-not-done",
			[]string{key, "quarry answered found"}},
		{"rename new landed", "rename-not-done-new", quarry.StatusFound, "", nil, nil, "", nil},
		{"rename new missing", "rename-not-done-new", quarry.StatusNotFound, quarry.StatusFound, nil, nil, "rename-not-done",
			[]string{key, "quarry answered not_found", "the unit exists and the member is missing", searched}},
		{"multipart counts as resolved", "create-not-done", quarry.StatusMultipart, "", nil, nil, "", nil},
		// Ambiguous means declarations with that name still exist, so the Delete direction blocks
		// on it — the old single "resolved" boolean read ambiguous as "gone" and passed both of
		// these (crucible round fable-high-r10, F1).
		{"delete still ambiguous", "delete-not-done", quarry.StatusAmbiguous, "", nil, nil, "delete-not-done",
			[]string{key, "quarry answered ambiguous"}},
		{"rename old still ambiguous", "rename-not-done-old", quarry.StatusAmbiguous, "", nil, nil, "rename-not-done", nil},
		{"create ambiguous is not landed", "create-not-done", quarry.StatusAmbiguous, "", overlappingFiles, candidateFiles, "create-not-done",
			[]string{"internal/greet/a.go, //go:build linux", "internal/greet/b.go, //go:build amd64"}},
		{"rename new ambiguous is not landed", "rename-not-done-new", quarry.StatusAmbiguous, "", nil, nil, "rename-not-done", nil},
		{"create over a partitioned member lands", "create-not-done", quarry.StatusAmbiguous, "", partitionedFiles, candidateFiles, "", nil},
		{"rename new over a partitioned member lands", "rename-not-done-new", quarry.StatusAmbiguous, "", partitionedFiles, candidateFiles, "", nil},
		{"delete still blocks on a partitioned member", "delete-not-done", quarry.StatusAmbiguous, "", partitionedFiles, candidateFiles, "delete-not-done",
			[]string{"internal/greet/a.go, //go:build linux", "internal/greet/b.go, //go:build !linux"}},
		{"rename old still blocks on a partitioned member", "rename-not-done-old", quarry.StatusAmbiguous, "", partitionedFiles, candidateFiles, "rename-not-done",
			[]string{"internal/greet/a.go, //go:build linux", "internal/greet/b.go, //go:build !linux"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeFixtureFiles(t, root, tt.files)
			var candidates []quarry.Symbol
			for _, file := range tt.candidates {
				candidates = append(candidates, quarry.Symbol{ID: key, File: file})
			}
			entries := []doneCheckEntry{{card: card, checkID: tt.checkID, key: key, display: key}}
			index := map[string]quarry.ResolveResult{key: {Target: key, Status: tt.status, Unit: tt.unit, Candidates: candidates}}

			findings, err := doneCheckVerdicts(&planparser.Plan{}, root, entries, index, declarations)
			if err != nil {
				t.Fatalf("doneCheckVerdicts() error = %v; want nil", err)
			}
			if tt.wantCheck == "" {
				if len(findings) != 0 {
					t.Fatalf("doneCheckVerdicts() = %v; want no finding", findings)
				}
				return
			}
			if len(findings) != 1 {
				t.Fatalf("doneCheckVerdicts() = %v; want exactly one %s finding", findings, tt.wantCheck)
			}
			if findings[0].Check != tt.wantCheck {
				t.Errorf("finding check = %q; want %q", findings[0].Check, tt.wantCheck)
			}
			if findings[0].Severity != SeverityBlocking {
				t.Errorf("finding severity = %q; want %q", findings[0].Severity, SeverityBlocking)
			}
			for _, want := range tt.wantDetail {
				if !strings.Contains(findings[0].Detail, want) {
					t.Errorf("finding detail = %q; want it to contain %q", findings[0].Detail, want)
				}
			}
		})
	}
}

// TestDoneCheckVerdicts_NotFoundDetail pins the finding for a Create target quarry answered not_found, read against the worktree's declarations.
// A member its unit declares only with another parameter list is a signature-mismatch finding that leads with the declared signature and its file, for a Create target and a Rename new side alike.
// A target with no parameter list, or whose same-named member sits under another owner or unit, stays create-not-done, as does a Go target.
// A name declared in many other units names the nearest few and counts the rest, and the unit's files are counted, never listed.
func TestDoneCheckVerdicts_NotFoundDetail(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	symbol := func(g glyph.Glyph, file string) quarry.Symbol {
		return quarry.Symbol{Glyph: g, ID: g.String(), File: file}
	}
	goSymbol := func(unit, name, file string) quarry.Symbol {
		return symbol(glyph.Glyph{Lang: glyph.Go, Unit: unit, Name: name}, file)
	}
	csharpMethod := func(unit, owner, name string, params []string, file string) quarry.Symbol {
		return symbol(glyph.Glyph{Lang: glyph.CSharp, Unit: unit, Owner: []string{owner}, Name: name, Params: params}, file)
	}
	createDetail := func(key, searched string) string {
		return fmt.Sprintf("Create target %q still does not resolve: looked up %q, quarry answered not_found (the unit exists and the member is missing); %s", key, key, searched)
	}

	tests := []struct {
		name     string
		language string
		// checkID is the done-check arm of the entry; empty means create-not-done.
		checkID      string
		key          string
		declarations []quarry.Symbol
		wantCheck    string
		wantDetail   string
	}{
		{
			name:         "SignatureMismatch",
			language:     "csharp",
			key:          "Shop#CartTests.Adds(int)",
			declarations: []quarry.Symbol{csharpMethod("Shop", "CartTests", "Adds", []string{"string"}, "tests/CartTests.cs"), csharpMethod("Shop", "CartTests", "Other", []string{}, "tests/CartTests.cs")},
			wantCheck:    "signature-mismatch",
			wantDetail:   `declared as Shop#CartTests.Adds(string) in tests/CartTests.cs, not with the parameter list Create target "Shop#CartTests.Adds(int)" names`,
		},
		{
			name:         "SignatureMismatchOnRenameNewSide",
			language:     "csharp",
			checkID:      "rename-not-done-new",
			key:          "Shop#CartTests.Adds(int)",
			declarations: []quarry.Symbol{csharpMethod("Shop", "CartTests", "Adds", []string{"string"}, "tests/CartTests.cs")},
			wantCheck:    "signature-mismatch",
			wantDetail:   `declared as Shop#CartTests.Adds(string) in tests/CartTests.cs, not with the parameter list Rename pair's new side "Shop#CartTests.Adds(int)" names`,
		},
		{
			name:         "TargetWithoutParameterListIsNotAMismatch",
			language:     "csharp",
			key:          "Shop#CartTests.Adds",
			declarations: []quarry.Symbol{csharpMethod("Shop", "CartTests", "Adds", []string{"string"}, "tests/CartTests.cs")},
			wantCheck:    "create-not-done",
			wantDetail:   createDetail("Shop#CartTests.Adds", "quarry searched 1 file of unit Shop; Adds is declared as Shop#CartTests.Adds(string) in tests/CartTests.cs"),
		},
		{
			name:         "OtherOwnerIsNotAMismatch",
			language:     "csharp",
			key:          "Shop#CartTests.Adds(int)",
			declarations: []quarry.Symbol{csharpMethod("Shop", "OtherTests", "Adds", []string{"string"}, "tests/OtherTests.cs")},
			wantCheck:    "create-not-done",
			wantDetail:   createDetail("Shop#CartTests.Adds(int)", "quarry searched 1 file of unit Shop; Adds is declared as Shop#OtherTests.Adds(string) in tests/OtherTests.cs"),
		},
		{
			name:         "OtherUnitIsNotAMismatch",
			language:     "csharp",
			key:          "Shop#CartTests.Adds(int)",
			declarations: []quarry.Symbol{csharpMethod("Billing", "CartTests", "Adds", []string{"string"}, "tests/BillingCartTests.cs")},
			wantCheck:    "create-not-done",
			wantDetail:   createDetail("Shop#CartTests.Adds(int)", "Adds is declared as Billing#CartTests.Adds(string) in tests/BillingCartTests.cs"),
		},
		{
			name:         "GoTargetIsNotAMismatch",
			language:     "go",
			key:          "internal/greet#Thing",
			declarations: []quarry.Symbol{goSymbol("internal/greet", "Other", "internal/greet/greet.go")},
			wantCheck:    "create-not-done",
			wantDetail:   createDetail("internal/greet#Thing", "quarry searched 1 file of unit internal/greet; no declaration named Thing exists anywhere in the worktree"),
		},
		{
			name:     "DeclaringSitesCappedNearestFirst",
			language: "go",
			key:      "internal/a/b#Load",
			declarations: []quarry.Symbol{
				goSymbol("internal/a/b", "Other", "internal/a/b/one.go"),
				goSymbol("internal/a/b", "More", "internal/a/b/two.go"),
				goSymbol("pkg/y", "Load", "pkg/y/y.go"),
				goSymbol("cmd/x", "Load", "cmd/x/x.go"),
				goSymbol("internal/z", "Load", "internal/z/z.go"),
				goSymbol("internal/a/c", "Load", "internal/a/c/c.go"),
			},
			wantCheck:  "create-not-done",
			wantDetail: createDetail("internal/a/b#Load", "quarry searched 2 files of unit internal/a/b; Load is declared as internal/a/c#Load in internal/a/c/c.go, internal/z#Load in internal/z/z.go, cmd/x#Load in cmd/x/x.go and 1 more"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			checkID := tt.checkID
			if checkID == "" {
				checkID = "create-not-done"
			}
			entries := []doneCheckEntry{{card: planparser.Card{Number: 1, Slug: "card"}, checkID: checkID, key: tt.key, display: tt.key}}
			index := map[string]quarry.ResolveResult{tt.key: {Target: tt.key, Status: quarry.StatusNotFound, Unit: quarry.StatusFound}}

			findings, err := doneCheckVerdicts(&planparser.Plan{Language: tt.language}, t.TempDir(), entries, index, tt.declarations)
			if err != nil {
				t.Fatalf("doneCheckVerdicts() error = %v; want nil", err)
			}
			if len(findings) != 1 {
				t.Fatalf("doneCheckVerdicts() = %v; want exactly one %s finding", findings, tt.wantCheck)
			}
			if findings[0].Check != tt.wantCheck || findings[0].Severity != SeverityBlocking {
				t.Errorf("finding = %+v; want a blocking %s finding", findings[0], tt.wantCheck)
			}
			if findings[0].Detail != tt.wantDetail {
				t.Errorf("finding detail = %q; want %q", findings[0].Detail, tt.wantDetail)
			}
		})
	}
}
