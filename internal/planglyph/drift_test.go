// drift_test.go covers DetectDrift with hand-built deltas: the gates decide before any git call,
// and the repair revalidates through quarry reading plain fixture files, so these tests stay in the
// untagged tier — the half driving a delta from a real git repository lives in
// delta_integration_test.go.

package planglyph

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

const driftTestTimestamp = "2026-01-01T00:00:00Z"

// requireExactTierRepair asserts an exact-tier rename sub#Old -> sub#New was auto-repaired in card
// 1 of the plan under dir: the card names the new glyph only, and the amendments file carries
// exactly one entry holding every string of wantAmendment.
func requireExactTierRepair(t *testing.T, dir string, wantAmendment []string) {
	t.Helper()

	got := readCardFile(t, dir, 1, "card1")
	if strings.Contains(got, "sub#Old") {
		t.Errorf("card still references the old glyph after auto-repair: %s", got)
	}
	if !strings.Contains(got, "sub#New") {
		t.Errorf("card was not rewritten to the new glyph: %s", got)
	}

	amendData, readErr := os.ReadFile(filepath.Join(dir, planparser.AmendmentsFileName))
	if readErr != nil {
		t.Fatalf("read amendments file: %v", readErr)
	}
	amendText := string(amendData)
	for _, want := range wantAmendment {
		if !strings.Contains(amendText, want) {
			t.Errorf("amendments file = %q; want it to contain %q", amendText, want)
		}
	}
	if strings.Count(amendText, "OldGlyph:") != 1 {
		t.Errorf("amendments file = %q; want exactly one amendment entry", amendText)
	}
}

// TestDetectDrift_ExactTierRenameAutoRepairsAndAmends covers an exact-tier rename rewriting the
// plan, revalidating clean, and appending exactly one amendment carrying all six fields.
func TestDetectDrift_ExactTierRenameAutoRepairsAndAmends(t *testing.T) {
	t.Parallel()

	worktree := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc New() {}\n"})

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Uses:**\n- `sub#Old`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** one\n",
	})

	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Renamed: []quarry.RenamedPair{{From: quarry.Symbol{ID: "sub#Old"}, To: quarry.Symbol{ID: "sub#New"}}},
	}}

	findings, err := DetectDrift(plan, plan, dir, worktree, delta, "deadbeef", driftTestTimestamp)
	if err != nil {
		t.Fatalf("DetectDrift(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none for an auto-repaired exact-tier rename", findings)
	}

	requireExactTierRepair(t, dir, []string{driftTestTimestamp, "1-card1", "sub#Old", "sub#New", "exact", "deadbeef"})
}

// TestDetectDrift_HandBuiltDelta covers the findings and side effects of DetectDrift's gates over a
// hand-built delta.
// Gate one reads the FULL plan's declared Rename pairs, not the pending view: the card whose rename
// the delta reports is exactly the one record-batch excludes from the pending view, and an index
// built over pending let the declared rename fall through to the repair path, rewritten plan-wide
// as exact-tier drift with an amendment.
// Two cards declaring a rename of the SAME old symbol are legal under every plan-format check, and
// gate one must recognize either card's own declared destination.
// The pair's New side is a plan: handle, not a bare glyph: the plan format REQUIRES that of a
// symbol Rename pair (rename-to-not-handle), so a bare-glyph fixture would test a shape no real
// plan can carry.
// An evidence-tier candidate is informational with every signal field in its detail, leaves the
// plan bytes identical and appends no amendment; quarry leaves its endpoints in Deleted, so the
// deleted-symbol sweep must not report a blocking finding beside the informational candidate that
// contradicts it, or the batch dies before any reviewer sees the evidence.
func TestDetectDrift_HandBuiltDelta(t *testing.T) {
	t.Parallel()

	type wantFinding struct {
		check    string
		severity Severity
	}
	const editOther = "**Edit:**\n- `sub/other.go`\n\n**Intent:** one\n"
	const usesGoneEditOther = "**Uses:**\n- `sub#Gone`\n\n" + editOther
	newOnly := map[string]string{"sub/a.go": "package sub\n\nfunc New() {}\n"}
	emptyOnly := map[string]string{"sub/a.go": "package sub\n"}

	cases := []struct {
		name  string
		files map[string]string
		cards map[int]string
		// pendingFirstCard drops every card but card 1 from the pending view, as record-batch does for the card it records.
		pendingFirstCard bool
		delta            quarry.DeltaAnswer
		wantFindings     []wantFinding
		// detailContains are substrings of the first finding's detail.
		detailContains []string
		// unchangedCard, when set, is the card whose file must stay byte-identical.
		unchangedCard int
		// noAmendment requires that no amendments file exists afterwards.
		noAmendment bool
	}{
		{
			name:  "gate one sees the recording batch's own pair",
			files: nil,
			cards: map[int]string{
				1: "**Rename:**\n- `sub#Foo` -> `plan:sub#Bar`\n\n**Intent:** rename Foo\n",
				2: "**Edit:**\n- `sub#Foo`\n\n**Intent:** two\n\n**ImpactSummary:** none\n",
			},
			pendingFirstCard: true,
			delta: quarry.DeltaAnswer{Renamed: []quarry.RenamedPair{
				{From: quarry.Symbol{ID: "sub#Foo"}, To: quarry.Symbol{ID: "sub#Bar"}},
			}},
			unchangedCard: 2,
			noAmendment:   true,
		},
		{
			name:  "gate one sees every declared destination for one old side",
			files: nil,
			cards: map[int]string{
				1: "**Rename:**\n- `sub#Foo` -> `plan:sub#Bar`\n\n**Intent:** rename Foo to Bar\n",
				2: "**Rename:**\n- `sub#Foo` -> `plan:sub#Baz`\n\n**Intent:** rename Foo to Baz\n",
				3: "**Edit:**\n- `sub#Foo`\n\n**Intent:** three\n\n**ImpactSummary:** none\n",
			},
			pendingFirstCard: true,
			delta: quarry.DeltaAnswer{Renamed: []quarry.RenamedPair{
				{From: quarry.Symbol{ID: "sub#Foo"}, To: quarry.Symbol{ID: "sub#Bar"}},
			}},
			unchangedCard: 3,
			noAmendment:   true,
		},
		{
			name:  "rename matching a card's own pair produces no finding and no amendment",
			files: newOnly,
			cards: map[int]string{
				1: "**Rename:**\n- `sub#Old` -> `plan:sub#New`\n\n**Intent:** one\n\n## Rename mechanic\n",
			},
			delta: quarry.DeltaAnswer{Renamed: []quarry.RenamedPair{
				{From: quarry.Symbol{ID: "sub#Old"}, To: quarry.Symbol{ID: "sub#New"}},
			}},
			unchangedCard: 1,
			noAmendment:   true,
		},
		{
			name:  "renamed symbol nothing references is logged only",
			files: newOnly,
			cards: map[int]string{1: editOther},
			delta: quarry.DeltaAnswer{Renamed: []quarry.RenamedPair{
				{From: quarry.Symbol{ID: "sub#Old"}, To: quarry.Symbol{ID: "sub#New"}},
			}},
			unchangedCard: 1,
		},
		{
			name:         "deleted and still referenced is blocking",
			files:        emptyOnly,
			cards:        map[int]string{1: usesGoneEditOther},
			delta:        quarry.DeltaAnswer{Deleted: []quarry.Symbol{{ID: "sub#Gone"}}},
			wantFindings: []wantFinding{{"plan-references-deleted-symbol", SeverityBlocking}},
		},
		{
			name:  "evidence-tier candidate is informational with its signals",
			files: emptyOnly,
			cards: map[int]string{1: usesGoneEditOther},
			delta: quarry.DeltaAnswer{RenameCandidates: []quarry.RenameCandidateEntry{{
				ID: "sub#Gone",
				Candidates: []quarry.RenameCandidate{{
					ID:   "sub#Renamed",
					File: "sub/b.go",
					Signals: quarry.RenameSignals{
						SignatureIdenticalModuloName: true,
						BodyTokenSimilarity:          0.875,
						BodyTokensBefore:             10,
						BodyTokensAfter:              11,
						DocIdentical:                 false,
					},
				}},
			}}},
			wantFindings:   []wantFinding{{"rename-candidate", SeverityInformational}},
			detailContains: []string{"sub#Renamed", "sub/b.go", "signature_identical_modulo_name=true", "body_token_similarity=0.8750", "body_tokens_before=10", "body_tokens_after=11", "doc_identical=false"},
			unchangedCard:  1,
			noAmendment:    true,
		},
		{
			// The same ID in both arrays, exactly as quarry emits it for an unresolved candidate.
			name:  "evidence-tier candidate suppresses the deleted-symbol finding",
			files: emptyOnly,
			cards: map[int]string{1: usesGoneEditOther},
			delta: quarry.DeltaAnswer{
				Deleted: []quarry.Symbol{{ID: "sub#Gone"}},
				RenameCandidates: []quarry.RenameCandidateEntry{
					{ID: "sub#Gone", Candidates: []quarry.RenameCandidate{{ID: "sub#Renamed", File: "sub/b.go"}}},
				},
			},
			wantFindings: []wantFinding{{"rename-candidate", SeverityInformational}},
		},
		{
			// Gate two applies to the evidence tier too: a candidate for a symbol nothing references is silent.
			name:  "evidence-tier candidate nothing references produces no finding",
			files: emptyOnly,
			cards: map[int]string{1: editOther},
			delta: quarry.DeltaAnswer{RenameCandidates: []quarry.RenameCandidateEntry{
				{ID: "sub#Gone", Candidates: []quarry.RenameCandidate{{ID: "sub#Renamed", File: "sub/b.go"}}},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			worktree := writeFixtureRepo(t, tc.files)
			dir, fullPlan := writePlanFixture(t, tc.cards)
			pending := fullPlan
			if tc.pendingFirstCard {
				pending = PendingPlan(fullPlan, []planparser.Card{fullPlan.Cards[0]})
			}
			var before string
			if tc.unchangedCard != 0 {
				before = readCardFile(t, dir, tc.unchangedCard, fmt.Sprintf("card%d", tc.unchangedCard))
			}

			findings, err := DetectDrift(fullPlan, pending, dir, worktree, quarry.GitDeltaAnswer{DeltaAnswer: tc.delta}, "deadbeef", driftTestTimestamp)
			if err != nil {
				t.Fatalf("DetectDrift(...) returned error: %v", err)
			}

			if len(findings) != len(tc.wantFindings) {
				t.Fatalf("findings = %+v; want %v", findings, tc.wantFindings)
			}
			for i, want := range tc.wantFindings {
				if findings[i].Check != want.check || findings[i].Severity != want.severity {
					t.Errorf("findings[%d] = %+v; want %s at severity %s", i, findings[i], want.check, want.severity)
				}
			}
			for _, want := range tc.detailContains {
				if !strings.Contains(findings[0].Detail, want) {
					t.Errorf("finding detail = %q; want it to contain %q", findings[0].Detail, want)
				}
			}
			if tc.unchangedCard != 0 {
				if got := readCardFile(t, dir, tc.unchangedCard, fmt.Sprintf("card%d", tc.unchangedCard)); got != before {
					t.Errorf("card %d was rewritten:\nbefore: %q\nafter:  %q", tc.unchangedCard, before, got)
				}
			}
			if _, statErr := os.Stat(filepath.Join(dir, planparser.AmendmentsFileName)); tc.noAmendment && !os.IsNotExist(statErr) {
				t.Errorf("amendments file exists (stat err = %v); want none — no repair happened", statErr)
			}
		})
	}
}

// TestDetectDrift_RepairIntoAnUnresolvableGlyphIsReported pins R6-11: the post-repair resolve's
// ANSWERS are read, not just its transport error. Before this, a repair that rewrote a ref into a
// glyph quarry answers not_found for passed the step described as "revalidated" in silence, and the
// amendment was appended as if the repair had worked — so the plan carried webster's own edit to a
// symbol that is not there, with nothing reported.
func TestDetectDrift_RepairIntoAnUnresolvableGlyphIsReported(t *testing.T) {
	t.Parallel()

	// The fixture repo carries neither sub#Old nor sub#Gone: the rename quarry reports as exact names
	// a destination that does not exist in the tree the repair is validated against.
	worktree := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Kept() {}\n"})

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Uses:**\n- `sub#Old`\n\n**Edit:**\n- `sub#Kept`\n\n**Intent:** one\n",
	})

	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Renamed: []quarry.RenamedPair{{From: quarry.Symbol{ID: "sub#Old"}, To: quarry.Symbol{ID: "sub#Gone"}}},
	}}

	findings, err := DetectDrift(plan, plan, dir, worktree, delta, "deadbeef", driftTestTimestamp)
	if err != nil {
		t.Fatalf("DetectDrift(...) returned error: %v", err)
	}

	var reported bool
	for _, f := range findings {
		if f.Check == "glyph-not-found" && strings.Contains(f.Detail, "sub#Gone") {
			reported = true
		}
	}
	if !reported {
		t.Errorf("findings = %+v; want a glyph-not-found naming sub#Gone — the repair rewrote the plan to a symbol that is not in the tree", findings)
	}
}

// TestEnsurePostRepairCoverage covers the per-key answer coverage guard: a target the post-repair
// batch actually requested, but which the resolve answer does not cover, must fail closed with
// ErrQuarryUnavailable naming the missing target — exactly as doneCheckVerdicts' own per-key guard
// does (donecheck.go) — rather than let the miss pass silently through DetectDrift's
// introduced-glyph filter.
// A result set covering every requested target returns nil even when the results ALSO carry an
// answer for a glyph outside the requested batch: the introduced-glyph-absent-from-batch case stays
// a silent pass, since the guard's scope is answer coverage of the requested targets only.
func TestEnsurePostRepairCoverage(t *testing.T) {
	t.Parallel()

	t.Run("unanswered target errors", func(t *testing.T) {
		t.Parallel()

		targets := []string{"sub#Bar", "sub#Baz"}
		results := []quarry.ResolveResult{
			{Target: "sub#Bar", Status: quarry.StatusFound},
			// sub#Baz requested but never answered.
		}

		err := ensurePostRepairCoverage(targets, results)
		if err == nil {
			t.Fatal("ensurePostRepairCoverage(unanswered target) = nil error; want an error naming the unanswered target")
		}
		if !errors.Is(err, ErrQuarryUnavailable) {
			t.Errorf("ensurePostRepairCoverage(unanswered target) error = %v; want errors.Is(err, ErrQuarryUnavailable)", err)
		}
		if !strings.Contains(err.Error(), "sub#Baz") {
			t.Errorf("ensurePostRepairCoverage(unanswered target) error = %v; want it to name %q", err, "sub#Baz")
		}
	})

	t.Run("full coverage passes", func(t *testing.T) {
		t.Parallel()

		targets := []string{"sub#Bar"}
		results := []quarry.ResolveResult{
			{Target: "sub#Bar", Status: quarry.StatusFound},
			{Target: "sub#Unrelated", Status: quarry.StatusFound},
		}

		if err := ensurePostRepairCoverage(targets, results); err != nil {
			t.Errorf("ensurePostRepairCoverage(full coverage) = %v; want nil", err)
		}
	})
}
