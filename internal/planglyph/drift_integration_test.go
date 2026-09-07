//go:build integration

// drift_integration_test.go covers DetectDrift against real fixture repositories, since it
// revalidates the exact-tier repair through a genuine quarry.Repo.Resolve call.

package planglyph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// TestDetectDrift_ExactTierRenameAutoRepairsAndAmends covers an exact-tier rename rewriting the
// plan, revalidating clean, and appending exactly one amendment carrying all six fields.
func TestDetectDrift_ExactTierRenameAutoRepairsAndAmends(t *testing.T) {
	worktree := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc New() {}\n"})

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Uses:**\n- `sub#Old`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** one\n",
	})

	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Renamed: []quarry.RenamedPair{{From: quarry.Symbol{ID: "sub#Old"}, To: quarry.Symbol{ID: "sub#New"}}},
	}}

	findings, err := DetectDrift(plan, plan, dir, worktree, delta, "deadbeef", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("DetectDrift(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none for an auto-repaired exact-tier rename", findings)
	}

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
	for _, want := range []string{"2026-01-01T00:00:00Z", "1-card1", "sub#Old", "sub#New", "exact", "deadbeef"} {
		if !strings.Contains(amendText, want) {
			t.Errorf("amendments file = %q; want it to contain %q", amendText, want)
		}
	}
	if strings.Count(amendText, "OldGlyph:") != 1 {
		t.Errorf("amendments file = %q; want exactly one amendment entry", amendText)
	}
}

// TestDetectDrift_RenameMatchingCardPairProducesNoFindingNoAmendment covers a rename matching a
// declared Rename card producing no drift finding and no amendment.
//
// The pair's New side is a plan: handle, not a bare glyph: the plan format REQUIRES that of a
// symbol Rename pair (rename-to-not-handle), so a bare-glyph fixture here would test a shape no
// real plan can carry -- which is exactly how gate one shipped unable to match anything.
func TestDetectDrift_RenameMatchingCardPairProducesNoFindingNoAmendment(t *testing.T) {
	worktree := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc New() {}\n"})

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#Old` -> `plan:sub#New`\n\n**Intent:** one\n\n## Rename mechanic\n",
	})
	before := readCardFile(t, dir, 1, "card1")

	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Renamed: []quarry.RenamedPair{{From: quarry.Symbol{ID: "sub#Old"}, To: quarry.Symbol{ID: "sub#New"}}},
	}}

	findings, err := DetectDrift(plan, plan, dir, worktree, delta, "deadbeef", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("DetectDrift(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none — this rename is the Rename card's own expected outcome", findings)
	}

	after := readCardFile(t, dir, 1, "card1")
	if before != after {
		t.Errorf("card was rewritten despite matching its own declared Rename pair:\nbefore: %s\nafter: %s", before, after)
	}
	if _, statErr := os.Stat(filepath.Join(dir, planparser.AmendmentsFileName)); statErr == nil {
		t.Error("amendments file was created despite no repair happening")
	}
}

// TestDetectDrift_RenamedSymbolNothingReferencesLogsOnlyNoRewrite covers a renamed symbol nothing
// references producing a log-only outcome with no rewrite.
func TestDetectDrift_RenamedSymbolNothingReferencesLogsOnlyNoRewrite(t *testing.T) {
	worktree := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc New() {}\n"})

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Edit:**\n- `sub/other.go`\n\n**Intent:** one\n",
	})
	before := readCardFile(t, dir, 1, "card1")

	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Renamed: []quarry.RenamedPair{{From: quarry.Symbol{ID: "sub#Old"}, To: quarry.Symbol{ID: "sub#New"}}},
	}}

	findings, err := DetectDrift(plan, plan, dir, worktree, delta, "deadbeef", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("DetectDrift(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none", findings)
	}
	after := readCardFile(t, dir, 1, "card1")
	if before != after {
		t.Errorf("card was rewritten despite referencing neither side of the rename:\nbefore: %s\nafter: %s", before, after)
	}
}

// TestDetectDrift_DeletedAndStillReferencedProducesBlockingFinding covers a deleted-and-still-
// referenced symbol producing plan-references-deleted-symbol.
func TestDetectDrift_DeletedAndStillReferencedProducesBlockingFinding(t *testing.T) {
	worktree := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n"})

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Uses:**\n- `sub#Gone`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** one\n",
	})

	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Deleted: []quarry.Symbol{{ID: "sub#Gone"}},
	}}

	findings, err := DetectDrift(plan, plan, dir, worktree, delta, "deadbeef", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("DetectDrift(...) returned error: %v", err)
	}
	if len(findings) != 1 || findings[0].Check != "plan-references-deleted-symbol" || findings[0].Severity != SeverityBlocking {
		t.Fatalf("findings = %+v; want exactly one blocking plan-references-deleted-symbol finding", findings)
	}
}

// TestDetectDrift_EvidenceTierCandidatesInformationalWithSignals covers a candidate block
// producing informational findings whose details carry every signal field, the plan bytes staying
// byte-identical, and no amendment entry appended.
func TestDetectDrift_EvidenceTierCandidatesInformationalWithSignals(t *testing.T) {
	worktree := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n"})

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Uses:**\n- `sub#Gone`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** one\n",
	})
	before := readCardFile(t, dir, 1, "card1")

	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		RenameCandidates: []quarry.RenameCandidateEntry{
			{
				ID: "sub#Gone",
				Candidates: []quarry.RenameCandidate{
					{
						ID:   "sub#Renamed",
						File: "sub/b.go",
						Signals: quarry.RenameSignals{
							SignatureIdenticalModuloName: true,
							BodyTokenSimilarity:          0.875,
							BodyTokensBefore:             10,
							BodyTokensAfter:              11,
							DocIdentical:                 false,
						},
					},
				},
			},
		},
	}}

	findings, err := DetectDrift(plan, plan, dir, worktree, delta, "deadbeef", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("DetectDrift(...) returned error: %v", err)
	}
	if len(findings) != 1 || findings[0].Check != "rename-candidate" || findings[0].Severity != SeverityInformational {
		t.Fatalf("findings = %+v; want exactly one informational rename-candidate finding", findings)
	}
	for _, want := range []string{"sub#Renamed", "sub/b.go", "signature_identical_modulo_name=true", "body_token_similarity=0.8750", "body_tokens_before=10", "body_tokens_after=11", "doc_identical=false"} {
		if !strings.Contains(findings[0].Detail, want) {
			t.Errorf("finding detail = %q; want it to contain %q", findings[0].Detail, want)
		}
	}

	after := readCardFile(t, dir, 1, "card1")
	if before != after {
		t.Errorf("plan bytes changed for an evidence-tier candidate:\nbefore: %s\nafter: %s", before, after)
	}
	if _, statErr := os.Stat(filepath.Join(dir, planparser.AmendmentsFileName)); statErr == nil {
		t.Error("amendments file was created for an evidence-tier candidate; want none")
	}
}

// TestDetectDrift_EvidenceTierCandidateSuppressesTheDeletedSymbolFinding covers the collision the
// evidence tier has with the deleted-symbol sweep: quarry leaves an evidence-tier candidate's
// endpoints in Deleted, so the same symbol would otherwise report a blocking
// plan-references-deleted-symbol beside the informational candidate that contradicts it — and the
// blocking half would kill the batch before any reviewer saw the evidence.
func TestDetectDrift_EvidenceTierCandidateSuppressesTheDeletedSymbolFinding(t *testing.T) {
	worktree := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n"})

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Uses:**\n- `sub#Gone`\n\n**Edit:**\n- `sub/other.go`\n\n**Intent:** one\n",
	})

	// The same ID in both arrays, exactly as quarry emits it for an unresolved candidate.
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Deleted: []quarry.Symbol{{ID: "sub#Gone"}},
		RenameCandidates: []quarry.RenameCandidateEntry{
			{ID: "sub#Gone", Candidates: []quarry.RenameCandidate{{ID: "sub#Renamed", File: "sub/b.go"}}},
		},
	}}

	findings, err := DetectDrift(plan, plan, dir, worktree, delta, "deadbeef", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("DetectDrift(...) returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %+v; want exactly one — the informational candidate alone", findings)
	}
	if findings[0].Check != "rename-candidate" || findings[0].Severity != SeverityInformational {
		t.Fatalf("findings[0] = %+v; want the informational rename-candidate, never a blocking deleted-symbol finding beside it", findings[0])
	}
}

// TestDetectDrift_EvidenceTierCandidateUnreferencedProducesNoFinding covers a candidate for a
// symbol nothing in the remaining plan references producing no finding at all — gate two applies
// to this tier too.
func TestDetectDrift_EvidenceTierCandidateUnreferencedProducesNoFinding(t *testing.T) {
	worktree := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n"})

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Edit:**\n- `sub/other.go`\n\n**Intent:** one\n",
	})

	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		RenameCandidates: []quarry.RenameCandidateEntry{
			{ID: "sub#Gone", Candidates: []quarry.RenameCandidate{{ID: "sub#Renamed", File: "sub/b.go"}}},
		},
	}}

	findings, err := DetectDrift(plan, plan, dir, worktree, delta, "deadbeef", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("DetectDrift(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v; want none — nothing references the deleted symbol", findings)
	}
}
