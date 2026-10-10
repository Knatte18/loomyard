// donecheck_test.go covers doneCheckVerdicts, the pure half of DoneChecks: the three done rules applied to a hand-built answer index, plus the coverage guard that refuses an answer set which does not cover a target the caller asked about.
// It reads no repository at all, only the candidate files an ambiguous row writes under a temp root — DoneChecks' own resolve-backed half is covered by donecheck_resolve_test.go instead.

package planglyph

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// TestDoneCheckVerdicts_UncoveredTargetIsInfrastructureNotAPass is the sabotage-proof for R5-6: an
// answer index missing a target the entries name must NOT let that entry's blocking check pass
// silently. Every key reaching doneCheckVerdicts was put into the resolve's own target list by
// DoneChecks, so a missing answer is quarry's positional contract not holding — infrastructure,
// never a verdict on the plan.
func TestDoneCheckVerdicts_UncoveredTargetIsInfrastructureNotAPass(t *testing.T) {
	t.Parallel()

	entries := []doneCheckEntry{{
		card:    planparser.Card{Number: 3, Slug: "add-thing"},
		checkID: "create-not-done",
		key:     "internal/greet#Farewell",
		display: "internal/greet#Farewell",
	}}

	findings, err := doneCheckVerdicts("", entries, map[string]quarry.ResolveResult{})

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
// A blocking row also pins the detail's lookup: the key, quarry's status, the unit status of a missing member, and each ambiguous candidate's file and build constraint.
// An ambiguous row writes its candidate files under a temp root, where a pair partitioned by build constraints counts as resolved and so lands a Create and a Rename new side but still blocks a Delete and a Rename old side.
//
//testtiming:keep pins the ambiguous rows and the per-arm rule table, which the resolve-backed DoneChecks tests reach only for found and not_found
func TestDoneCheckVerdicts_Rules(t *testing.T) {
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
			[]string{key, "quarry answered not_found", "the unit exists and the member is missing"}},
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
			[]string{key, "quarry answered not_found", "the unit exists and the member is missing"}},
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

			findings, err := doneCheckVerdicts(root, entries, index)
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
