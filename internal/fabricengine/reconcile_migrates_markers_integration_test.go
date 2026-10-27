//go:build tier1

// reconcile_migrates_markers_integration_test.go covers Reconcile's per-pair judging of the weft anchor markers:
// a weft with no committed marker gets the one its content implies, and the hub-wide marker on the board is retired once every weft carries its own.
// The same migration is shown for a weft branch Add adopts.
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

// seedBoardMarkers commits the legacy hub-wide marker and a leftover pre-rename marker beside it on the board, and pushes them.
// It also pushes the prime's weft branch, which a fixture hub leaves local while its warp sibling is on origin, as a live hub's is.
func seedBoardMarkers(t *testing.T, h *hubforge.Hub) {
	t.Helper()

	gitkit.Git(t, h.PrimeRecords(), "push", "--quiet", "origin", "HEAD")
	board := h.BoardDir()
	gitkit.CommitFile(t, board, lymcwd.AnchorFileName, ".\n", "legacy hub-wide marker")
	gitkit.CommitFile(t, board, lymcwd.StaleAnchorFileName, ".\n", "leftover pre-rename marker")
	gitkit.Git(t, board, "push", "--quiet", "origin", "HEAD")
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

// TestReconcile_MigratesMarkerlessWefts covers the migration of markerless wefts, the census behind the Bolt marker's retirement, and the same migration on Add's adopt path.
func TestReconcile_MigratesMarkerlessWefts(t *testing.T) {
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	pair := hubforge.TemplatePairSlug

	t.Run("MarkerlessWeftsGetTheirContentsAnchorPrimeFirst", func(t *testing.T) {
		h := hubforge.CopyHub(t, migrationShape)
		recorder := fabricengine.SwapPushRecorderForTest(t)
		prime := h.Location.WorktreeName
		primeWeft, pairWeft := h.PrimeRecords(), h.PairRecordsSibling(pair)

		// The prime moves to "sub" after the pair forked from it, so the pair's content stays at ".".
		hubforge.ReanchorPair(t, h, prime, "sub")
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
			t.Fatalf("push recorder calls = %d; want one per migrated weft", len(calls))
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
			t.Error("the prime was wired at the warp root; want its junctions at the anchor read back from the migrated marker")
		}
	})

	t.Run("UnwireKeepsTheMigratedMarker", func(t *testing.T) {
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

	t.Run("BoltMarkerRetiredInOneBoardCommitOnceEveryWeftCarriesOne", func(t *testing.T) {
		h := hubforge.CopyHub(t, migrationShape)
		fabricengine.SwapPushRecorderForTest(t)
		seedBoardMarkers(t, h)
		board := h.BoardDir()
		before := gitkit.RevListCount(t, board, "HEAD")

		res := reconcileWithoutPairErrors(t, h)

		if res.BoltAnchor != "removed" {
			t.Fatalf("BoltAnchor = %q; want %q", res.BoltAnchor, "removed")
		}
		if got := gitkit.RevListCount(t, board, "HEAD"); got != before+1 {
			t.Errorf("board commits = %d; want %d, the one removal commit", got, before+1)
		}
		removed := strings.Fields(gitkit.Git(t, board, "show", "--format=", "--name-only", "HEAD"))
		if len(removed) != 2 {
			t.Errorf("files in the removal commit = %v; want both markers", removed)
		}
		for _, name := range []string{lymcwd.AnchorFileName, lymcwd.StaleAnchorFileName} {
			if _, err := os.Lstat(filepath.Join(board, name)); err == nil {
				t.Errorf("board still holds %s", name)
			}
		}
		if head, upstream := gitkit.RevParse(t, board, "HEAD"), gitkit.RevParse(t, board, "@{upstream}"); head != upstream {
			t.Errorf("board HEAD %s is not pushed; upstream is %s", head, upstream)
		}
	})

	t.Run("HubWithNoBoltMarkerReportsNothing", func(t *testing.T) {
		h := hubforge.CopyHub(t, migrationShape)
		fabricengine.SwapPushRecorderForTest(t)

		if res := reconcileWithoutPairErrors(t, h); res.BoltAnchor != "" {
			t.Errorf("BoltAnchor = %q; want none for a hub with no Bolt marker", res.BoltAnchor)
		}
	})

	t.Run("BoltMarkerKeptWhileACensusMemberLacksOne", func(t *testing.T) {
		tests := []struct {
			name string
			// arrange breaks the census in one way and returns what the verdict must name.
			arrange func(t *testing.T, h *hubforge.Hub) string
		}{
			{
				// The pair's weft is pushed markerless; the pass commits its marker, but the fetched ref still lacks it.
				"MarkerCommitNotPushed",
				func(t *testing.T, h *hubforge.Hub) string {
					weft := h.PairRecordsSibling(pair)
					dropCommittedMarker(t, weft)
					gitkit.Git(t, weft, "push", "--quiet", "origin", "HEAD")
					return "origin/" + fabricengine.RecordsBranchName(pair)
				},
			},
			{
				"FetchedWeftRefMissing",
				func(t *testing.T, h *hubforge.Hub) string {
					gitkit.Git(t, h.PrimeRecords(), "update-ref", "-d", "refs/remotes/origin/"+fabricengine.RecordsBranchName(pair))
					return fabricengine.RecordsBranchName(pair)
				},
			},
			{
				"CommittedMarkerBlank",
				func(t *testing.T, h *hubforge.Hub) string {
					weft := h.PairRecordsSibling(pair)
					gitkit.CommitFile(t, weft, lymcwd.AnchorFileName, "\n", "blank the marker")
					return weft
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				h := hubforge.CopyHub(t, migrationShape)
				fabricengine.SwapPushRecorderForTest(t)
				seedBoardMarkers(t, h)
				want := tt.arrange(t, h)

				res, err := h.Topology.Reconcile(h.Location)
				if err != nil {
					t.Fatalf("Reconcile() = %v; want nil", err)
				}
				if !strings.HasPrefix(res.BoltAnchor, "kept: ") || !strings.Contains(res.BoltAnchor, want) {
					t.Errorf("BoltAnchor = %q; want a kept verdict naming %q", res.BoltAnchor, want)
				}
				if _, err := os.Lstat(filepath.Join(h.BoardDir(), lymcwd.AnchorFileName)); err != nil {
					t.Errorf("board marker went missing: %v", err)
				}
			})
		}
	})

	t.Run("BoltMarkerRetiredOnceTheMigratedMarkerIsPushed", func(t *testing.T) {
		h := hubforge.CopyHub(t, migrationShape)
		fabricengine.SwapPushRecorderForTest(t)
		seedBoardMarkers(t, h)
		weft := h.PairRecordsSibling(pair)
		dropCommittedMarker(t, weft)
		gitkit.Git(t, weft, "push", "--quiet", "origin", "HEAD")

		if res := reconcileWithoutPairErrors(t, h); !strings.HasPrefix(res.BoltAnchor, "kept: ") {
			t.Fatalf("first pass BoltAnchor = %q; want kept while the migrated marker is unpushed", res.BoltAnchor)
		}
		gitkit.Git(t, weft, "push", "--quiet", "origin", "HEAD")

		if res := reconcileWithoutPairErrors(t, h); res.BoltAnchor != "removed" {
			t.Errorf("second pass BoltAnchor = %q; want %q", res.BoltAnchor, "removed")
		}
	})

	t.Run("OrphanRemoteWeftBranchIsNotCounted", func(t *testing.T) {
		h := hubforge.CopyHub(t, migrationShape)
		fabricengine.SwapPushRecorderForTest(t)
		seedBoardMarkers(t, h)

		// A markerless weft branch on origin whose warp sibling never reached origin.
		weft := h.PairRecordsSibling(pair)
		pairBranch := fabricengine.RecordsBranchName(pair)
		gitkit.Git(t, weft, "checkout", "--quiet", "-b", "orphan-weft")
		dropCommittedMarker(t, weft)
		gitkit.Git(t, weft, "push", "--quiet", "origin", "orphan-weft")
		gitkit.Git(t, weft, "checkout", "--quiet", pairBranch)

		if res := reconcileWithoutPairErrors(t, h); res.BoltAnchor != "removed" {
			t.Errorf("BoltAnchor = %q; want %q, the orphan not counted", res.BoltAnchor, "removed")
		}
	})

	t.Run("CensusMakesNoNetworkCall", func(t *testing.T) {
		tests := []struct {
			name string
			// arrange runs with the remote reachable and returns what the verdict must contain.
			arrange func(t *testing.T, h *hubforge.Hub) string
		}{
			{"EveryMarkerPresent", func(*testing.T, *hubforge.Hub) string { return "fetch_failed" }},
			{
				"FetchedRefLacksMarker",
				func(t *testing.T, h *hubforge.Hub) string {
					weft := h.PairRecordsSibling(pair)
					dropCommittedMarker(t, weft)
					gitkit.Git(t, weft, "push", "--quiet", "origin", "HEAD")
					return "origin/" + fabricengine.RecordsBranchName(pair)
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				h := hubforge.CopyHub(t, migrationShape)
				fabricengine.SwapPushRecorderForTest(t)
				seedBoardMarkers(t, h)
				want := tt.arrange(t, h)
				gitkit.Git(t, h.PrimeRecords(), "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing"))

				res := reconcileWithoutPairErrors(t, h)
				if !strings.HasPrefix(res.BoltAnchor, "kept: ") || !strings.Contains(res.BoltAnchor, want) {
					t.Errorf("BoltAnchor = %q; want a kept verdict containing %q", res.BoltAnchor, want)
				}
			})
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
