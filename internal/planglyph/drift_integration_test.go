//go:build integration

// drift_integration_test.go covers DetectDrift against real fixture repositories, since it
// revalidates the exact-tier repair through a genuine quarry.Repo.Resolve call.
//
// Most tests here hand DetectDrift a quarry.GitDeltaAnswer built in the test body, which is right
// for pinning the detector's own logic: a synthetic delta is the only way to construct the corner
// cases (a repair into an unresolvable glyph, a coverage breach) that a real repository cannot
// produce. But it leaves one thing structurally untestable — whether quarry's OWN emitted
// Symbol.ID spelling still matches what resolveKeyFor derives from a plan: handle, which is the
// comparison gate one is built on. A test that authors both sides of a comparison cannot check
// that the two sides agree in production.
//
// The two TestDetectDrift_RealDelta* tests at the bottom close that gap: they build a real git
// rename commit, call the real Delta, and drive the resulting answer through DetectDrift from both
// sides of gate one (crucible round opus5-high-r1, F4).

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

// TestDetectDrift_RepairIntoAnUnresolvableGlyphIsReported pins R6-11: the post-repair resolve's
// ANSWERS are read, not just its transport error. Before this, a repair that rewrote a ref into a
// glyph quarry answers not_found for passed the step described as "revalidated" in silence, and the
// amendment was appended as if the repair had worked — so the plan carried webster's own edit to a
// symbol that is not there, with nothing reported.
func TestDetectDrift_RepairIntoAnUnresolvableGlyphIsReported(t *testing.T) {
	// The fixture repo carries neither sub#Old nor sub#Gone: the rename quarry reports as exact names
	// a destination that does not exist in the tree the repair is validated against.
	worktree := writeFixtureRepo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Kept() {}\n"})

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Uses:**\n- `sub#Old`\n\n**Edit:**\n- `sub#Kept`\n\n**Intent:** one\n",
	})

	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Renamed: []quarry.RenamedPair{{From: quarry.Symbol{ID: "sub#Old"}, To: quarry.Symbol{ID: "sub#Gone"}}},
	}}

	findings, err := DetectDrift(plan, plan, dir, worktree, delta, "deadbeef", "2026-01-01T00:00:00Z")
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

// realRenameDelta builds a real two-commit git fixture whose second commit renames one Go
// declaration, calls the real Delta (spawning git through (*quarry.Repo).DeltaGit), and returns the
// repository root alongside quarry's own answer.
//
// It FAILS rather than skips when the answer carries no exact-tier rename pair for oldID -> newID.
// That is the point of the helper: every other DetectDrift test hands the detector a hand-written
// quarry.GitDeltaAnswer, so the ID spelling under test is supplied by the test itself and a
// spelling mismatch between quarry and this package could never surface. A silent skip here would
// restore exactly that blind spot.
func realRenameDelta(t *testing.T, oldID, newID string) (string, quarry.GitDeltaAnswer) {
	t.Helper()

	f := newDeltaFixtureRepo(t)
	from := f.writeAndCommit("sub/a.go", "package sub\n\nfunc Old() {}\n\nfunc Kept() {}\n", "first commit")
	to := f.writeAndCommit("sub/a.go", "package sub\n\nfunc New() {}\n\nfunc Kept() {}\n", "rename Old to New")

	delta, err := Delta(f.root, from, to)
	if err != nil {
		t.Fatalf("Delta(%q, %q, %q) returned error: %v", f.root, from, to, err)
	}

	var matched bool
	for _, pair := range delta.Renamed {
		if pair.From.ID == oldID && pair.To.ID == newID {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("real Delta answer carries no exact-tier rename %s -> %s; Renamed = %+v. Either quarry no longer classifies this edit as an exact rename, or its Symbol.ID spelling has moved — both break gate one, which compares these IDs against resolveKeyFor's output", oldID, newID, delta.Renamed)
	}
	return f.root, delta
}

// TestDetectDrift_RealDeltaGateOneRecognizesDeclaredRename is the test that closes F4 (crucible
// round opus5-high-r1): the ONLY one that drives a real quarry.DeltaGit answer into DetectDrift's
// gate one.
//
// Gate one asks "is this rename some declared Rename card's own expected outcome?" by comparing
// quarry's RenamedPair.To.ID against resolveKeyFor(pair.New) — the plan format requires a symbol
// Rename's New side to be a plan: handle, while quarry reports the new symbol under its bare glyph,
// so the two spellings must be brought together or every declared rename is misread as drift and
// auto-"repaired": a plan-wide RewriteRefs plus an amendment recording work the plan had asked for.
//
// Every other DetectDrift test constructs the RenamedPair by hand, so both sides of that comparison
// were previously supplied by the test. Here quarry supplies one of them.
func TestDetectDrift_RealDeltaGateOneRecognizesDeclaredRename(t *testing.T) {
	worktree, delta := realRenameDelta(t, "sub#Old", "sub#New")

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#Old` -> `plan:sub#New`\n\n**Intent:** one\n\n## Rename mechanic\n",
		2: "**Edit:**\n- `sub#Kept`\n\n**Uses:**\n- `plan:sub#New`\n\n**Intent:** two\n",
	})
	before := readCardFile(t, dir, 2, "card2")

	findings, err := DetectDrift(plan, plan, dir, worktree, delta, "deadbeef", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("DetectDrift(...) returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %+v; want none — the delta's rename is card 1's own declared outcome, not drift", findings)
	}

	if got := readCardFile(t, dir, 2, "card2"); got != before {
		t.Errorf("card 2 was rewritten:\n got %q\nwant %q (unchanged) — gate one must suppress the exact-tier repair for a declared rename", got, before)
	}
	if _, err := os.Stat(filepath.Join(dir, planparser.AmendmentsFileName)); !os.IsNotExist(err) {
		t.Errorf("an amendments file exists after a DECLARED rename; gate one must record no repair (stat err = %v)", err)
	}
}

// TestDetectDrift_RealDeltaExactTierRepairsUndeclaredRename is the companion: the same real
// quarry.DeltaGit answer, against a plan that declares NO Rename card, must auto-repair.
//
// Together with the test above it pins gate one from both sides against a real delta — declared
// renames are left alone, undeclared ones are repaired — so a change to either quarry's Symbol.ID
// spelling or to resolveKeyFor breaks exactly one of the two rather than silently passing both.
func TestDetectDrift_RealDeltaExactTierRepairsUndeclaredRename(t *testing.T) {
	worktree, delta := realRenameDelta(t, "sub#Old", "sub#New")

	dir, plan := writePlanFixture(t, map[int]string{
		1: "**Edit:**\n- `sub#Kept`\n\n**Uses:**\n- `sub#Old`\n\n**Intent:** one\n",
	})

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
	for _, want := range []string{"1-card1", "sub#Old", "sub#New", "exact", "deadbeef"} {
		if !strings.Contains(amendText, want) {
			t.Errorf("amendments file = %q; want it to contain %q", amendText, want)
		}
	}
	if strings.Count(amendText, "OldGlyph:") != 1 {
		t.Errorf("amendments file = %q; want exactly one amendment entry", amendText)
	}
}
