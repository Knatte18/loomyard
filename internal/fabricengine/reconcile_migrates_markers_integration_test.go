// reconcile_migrates_markers_integration_test.go covers Reconcile's per-pair judging of the weft anchor markers:
// a weft with no committed marker gets the one its content implies.
// The same derivation is shown for a weft branch Add adopts.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.
// The tests are serial: they swap the package push seam with SwapPushRecorderForTest.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// migrationShape is the hub every scenario starts from: the "." anchor and one pair.
var migrationShape = hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}}

// dropCommittedMarker commits the removal of the marker in the weft worktree at weft.
func dropCommittedMarker(t *testing.T, weft string) {
	t.Helper()

	gitkit.Git(t, weft, "rm", "--quiet", lymcwd.AnchorFileName)
	gitkit.Git(t, weft, "commit", "--quiet", "-m", "drop the marker")
}

// committedMarkerAt returns the marker committed at rev in the repository at dir.
func committedMarkerAt(t *testing.T, dir, rev string) string {
	t.Helper()

	return gitkit.Git(t, dir, "show", rev+":"+lymcwd.AnchorFileName)
}

// reconcileWithoutPairErrors runs a reconcile pass and fails the test on a pass error or on any pair carrying an Error.
func reconcileWithoutPairErrors(t *testing.T, h *hubforge.Hub) fabricengine.ReconcileResult {
	t.Helper()

	res, err := h.Topology.Reconcile(h.Location)
	if err != nil {
		t.Fatalf("Reconcile() = %v; want nil", err)
	}
	for _, pair := range res.Pairs {
		if pair.Error != "" {
			t.Errorf("pair %s Error = %q; want none", pair.CodeWorktree, pair.Error)
		}
	}
	return res
}

// TestReconcile_RecordsMarkersOfMarkerlessWefts covers the marker a reconcile pass derives for a markerless weft.
// It also covers the same derivation on Add's adopt path.
func TestReconcile_RecordsMarkersOfMarkerlessWefts(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	pair := hubforge.TemplatePairSlug

	t.Run("MarkerlessWeftsGetTheirContentsAnchorPrimeFirst", func(t *testing.T) {
		h := hubforge.CopyHub(t, migrationShape)
		recorder := fabricengine.SwapPushRecorderForTest(t)
		prime := h.Location.WorktreeName
		primeWeft, pairWeft := h.PrimeRecords(), h.PairRecordsSibling(pair)

		// The prime moves to "sub" after the pair forked from it, so the pair's content stays at ".".
		hubforge.ReanchorPair(t, h, prime, "sub")
		// A fresh hub's weft tracks no wired directory, so each weft tracks one file for its content to imply the anchor.
		gitkit.CommitFile(t, primeWeft, "sub/"+lymdirs.LymDirName+"/keep.txt", "x", "track content at sub")
		gitkit.CommitFile(t, pairWeft, lymdirs.LymDirName+"/keep.txt", "x", "track content at the root")
		dropCommittedMarker(t, primeWeft)
		dropCommittedMarker(t, pairWeft)

		res := reconcileWithoutPairErrors(t, h)

		if got := committedMarkerAt(t, primeWeft, "HEAD"); got != "sub" {
			t.Errorf("prime weft marker = %q; want %q, the anchor its content sits at", got, "sub")
		}
		if got := committedMarkerAt(t, pairWeft, "HEAD"); got != "." {
			t.Errorf("pair weft marker = %q; want %q, the anchor its content sits at", got, ".")
		}
		for weft, want := range map[string]string{primeWeft: "fabric reconcile: record anchor sub", pairWeft: "fabric reconcile: record anchor ."} {
			if got := gitkit.Git(t, weft, "log", "-1", "--format=%s"); got != want {
				t.Errorf("last commit subject of %s = %q; want %q", weft, got, want)
			}
		}
		for _, slug := range []string{prime, pair} {
			if detail := pairResultFor(t, h, res, slug).Detail; !strings.Contains(detail, "recorded anchor") {
				t.Errorf("pair %s Detail = %q; want it to name the recorded anchor", slug, detail)
			}
		}

		calls := recorder.Calls()
		if len(calls) != 2 {
			t.Fatalf("push recorder calls = %d; want one per recorded weft", len(calls))
		}
		if calls[0].WeftPath != primeWeft || calls[0].WarpPath != "" {
			t.Errorf("first push = %+v; want the prime's records worktree %s alone", calls[0], primeWeft)
		}
		if calls[1].WeftPath != pairWeft || calls[1].WarpPath != h.PairCodeWorktree(pair) {
			t.Errorf("second push = %+v; want the pair's records worktree %s with its code worktree", calls[1], pairWeft)
		}

		for slug, anchor := range map[string]string{prime: "sub", pair: "."} {
			wired := filepath.Join(fabricengine.WorktreePath(h.Location, slug), anchor, lymdirs.LymDirName)
			if _, err := os.Lstat(wired); err != nil {
				t.Errorf("junction %s of %s is missing: %v", wired, slug, err)
			}
		}
		if _, err := os.Lstat(filepath.Join(h.PrimeWorktree(), lymdirs.LymDirName)); err == nil {
			t.Error("the prime was wired at the warp root; want its junctions at the anchor read back from the recorded marker")
		}
	})

	t.Run("UnwireKeepsTheRecordedMarker", func(t *testing.T) {
		h := hubforge.CopyHub(t, migrationShape)
		fabricengine.SwapPushRecorderForTest(t)
		pairWeft := h.PairRecordsSibling(pair)

		dropCommittedMarker(t, pairWeft)
		reconcileWithoutPairErrors(t, h)
		if _, err := fabricengine.Unwire(h.PairCodeWorktree(pair), true); err != nil {
			t.Fatalf("Unwire() = %v; want nil", err)
		}

		if got := committedMarkerAt(t, pairWeft, "HEAD"); got != "." {
			t.Errorf("committed marker after unwire = %q; want %q", got, ".")
		}
		if data, err := os.ReadFile(filepath.Join(pairWeft, lymcwd.AnchorFileName)); err != nil || strings.TrimSpace(string(data)) != "." {
			t.Errorf("marker file after unwire = %q, %v; want %q in place", data, err, ".")
		}
		if dirt := gitkit.Git(t, pairWeft, "status", "--porcelain", "--", lymcwd.AnchorFileName); dirt != "" {
			t.Errorf("marker status after unwire = %q; want clean", dirt)
		}
	})

	t.Run("AddAdoptsABranchWithNoUsableMarkerAtItsContentsAnchor", func(t *testing.T) {
		tests := []struct {
			name string
			// rewrite plants the pushed branch's state on a clone of the weft bare.
			rewrite func(clone string)
		}{
			{"Markerless", func(clone string) { mustGit(clone, "rm", "--quiet", lymcwd.AnchorFileName) }},
			{"BlankMarker", func(clone string) {
				if err := os.WriteFile(filepath.Join(clone, lymcwd.AnchorFileName), []byte("\n"), 0o644); err != nil {
					panic(err)
				}
				mustGit(clone, "add", "--", lymcwd.AnchorFileName)
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				const slug = "adopted"
				h := removedPair(t, slug)
				weftBranch := fabricengine.RecordsBranchName(slug)

				clone := t.TempDir()
				mustGit(clone, "clone", "--quiet", h.RecordsBare, ".")
				mustGit(clone, "checkout", "--quiet", weftBranch)
				tt.rewrite(clone)
				mustGit(clone, "commit", "--quiet", "-m", "plant the unusable marker")
				mustGit(clone, "push", "--quiet", "origin", weftBranch)

				// The acting worktree is anchored elsewhere, so only the branch's own tree can supply the anchor.
				hubforge.ReanchorPair(t, h, h.Location.WorktreeName, "sub")
				if _, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{}); err != nil {
					t.Fatalf("Add: %v", err)
				}

				weft := fabricengine.RecordsWorktreePath(h.Location, slug)
				if got := committedMarkerAt(t, weft, "HEAD"); got != "." {
					t.Errorf("adopted weft marker = %q; want %q, the anchor its content sits at", got, ".")
				}
				if got := gitkit.Git(t, weft, "log", "-1", "--format=%s"); got != "fabric add: record anchor ." {
					t.Errorf("last commit subject = %q; want the recorded anchor", got)
				}
				adopted, err := lymcwd.NamedWorktree(h.Path, slug)
				if err != nil {
					t.Fatalf("NamedWorktree(%s): %v", slug, err)
				}
				if adopted.AnchorRel != "." {
					t.Errorf("adopted pair AnchorRel = %q; want %q", adopted.AnchorRel, ".")
				}
				if _, err := os.Lstat(filepath.Join(fabricengine.WorktreePath(h.Location, slug), lymdirs.LymDirName)); err != nil {
					t.Errorf("adopted pair is not wired at its own anchor: %v", err)
				}
			})
		}
	})
}
