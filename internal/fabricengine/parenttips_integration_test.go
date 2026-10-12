//go:build integration

// parenttips_integration_test.go proves the code-side reads a parent merge needs — ReadParentTips, ChangedPaths and HoldsWork — and FastForwardPair's fast-forward-only move and refusals.
// Package fabricengine_test; it reuses pull_integration_test.go's buildReconcileFixture and rewriteWarpRemoteHistory.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// pushFromSecondClone commits one new file on main in a fresh clone of bareDir, pushes it and returns the new upstream tip.
func pushFromSecondClone(t *testing.T, fixturesDir, bareDir, name string) string {
	t.Helper()

	clone := filepath.Join(fixturesDir, "clone-"+name)
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.Git(t, clone, "config", "user.email", "test@test.com")
	gitkit.Git(t, clone, "config", "user.name", "Test")
	sha := gitkit.CommitFile(t, clone, name+".txt", name, name)
	gitkit.MustRun(t, clone, "git", "push")
	return sha
}

// TestReadParentTips covers every relation, and that an upstream push is seen only when the read fetches.
func TestReadParentTips(t *testing.T) {
	t.Parallel()

	type env struct {
		f           *fabricengine.Fabric
		warpPath    string
		fixturesDir string
		bareDir     string
	}
	tests := []struct {
		name         string
		branch       string
		fetch        bool
		setup        func(t *testing.T, e env) (wantLocal, wantUpstream string)
		wantRelation fabricengine.ParentRelation
	}{
		{
			name:   "equal",
			branch: "main",
			setup: func(t *testing.T, e env) (string, string) {
				head := fabricengine.CurrentSHAForTest(t, e.warpPath)
				return head, head
			},
			wantRelation: fabricengine.ParentEqual,
		},
		{
			name:   "an upstream push is unseen without a fetch",
			branch: "main",
			setup: func(t *testing.T, e env) (string, string) {
				head := fabricengine.CurrentSHAForTest(t, e.warpPath)
				pushFromSecondClone(t, e.fixturesDir, e.bareDir, "other")
				return head, head
			},
			wantRelation: fabricengine.ParentEqual,
		},
		{
			name:   "behind after an upstream push",
			branch: "main",
			fetch:  true,
			setup: func(t *testing.T, e env) (string, string) {
				head := fabricengine.CurrentSHAForTest(t, e.warpPath)
				return head, pushFromSecondClone(t, e.fixturesDir, e.bareDir, "other")
			},
			wantRelation: fabricengine.ParentBehind,
		},
		{
			name:   "ahead after a local commit",
			branch: "main",
			fetch:  true,
			setup: func(t *testing.T, e env) (string, string) {
				upstream := fabricengine.CurrentSHAForTest(t, e.warpPath)
				return gitkit.CommitFile(t, e.warpPath, "local.txt", "local", "local"), upstream
			},
			wantRelation: fabricengine.ParentAhead,
		},
		{
			name:   "diverged after a commit on each side",
			branch: "main",
			fetch:  true,
			setup: func(t *testing.T, e env) (string, string) {
				upstream := pushFromSecondClone(t, e.fixturesDir, e.bareDir, "other")
				return gitkit.CommitFile(t, e.warpPath, "local.txt", "local", "local"), upstream
			},
			wantRelation: fabricengine.ParentDiverged,
		},
		{
			name:   "no upstream for a local-only branch",
			branch: "local-only",
			fetch:  true,
			setup: func(t *testing.T, e env) (string, string) {
				gitkit.Git(t, e.warpPath, "branch", "local-only")
				return fabricengine.CurrentSHAForTest(t, e.warpPath), ""
			},
			wantRelation: fabricengine.ParentNoUpstream,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixturesDir := t.TempDir()
			f, warpPath, bareDir, _, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)
			wantLocal, wantUpstream := tc.setup(t, env{f: f, warpPath: warpPath, fixturesDir: fixturesDir, bareDir: bareDir})

			got, err := f.ReadParentTips(tc.branch, tc.fetch)
			if err != nil {
				t.Fatalf("ReadParentTips(%q, %v) error = %v", tc.branch, tc.fetch, err)
			}
			want := fabricengine.ParentTips{Local: wantLocal, Upstream: wantUpstream, Relation: tc.wantRelation}
			if got != want {
				t.Errorf("ReadParentTips(%q, %v) = %+v; want %+v", tc.branch, tc.fetch, got, want)
			}
		})
	}
}

// TestReadParentTips_MissingBranchIsAnError covers a missing local branch, which the error names.
func TestReadParentTips_MissingBranchIsAnError(t *testing.T) {
	t.Parallel()

	f, _, _, _, _, _, _ := buildReconcileFixture(t, t.TempDir(), 1)
	got, err := f.ReadParentTips("no-such-branch", false)
	if err == nil {
		t.Fatalf("ReadParentTips(no-such-branch) = %+v, nil; want an error naming the branch", got)
	}
	if !strings.Contains(err.Error(), "no-such-branch") {
		t.Errorf("ReadParentTips(no-such-branch) error = %q; want it to name the branch", err)
	}
}

// TestChangedPaths_NamesExactlyTheCommittedFiles asserts the sorted paths between two commits.
func TestChangedPaths_NamesExactlyTheCommittedFiles(t *testing.T) {
	t.Parallel()

	f, warpPath, _, _, _, _, _ := buildReconcileFixture(t, t.TempDir(), 1)
	before := fabricengine.CurrentSHAForTest(t, warpPath)
	gitkit.CommitFile(t, warpPath, "z/last.txt", "z", "z")
	after := gitkit.CommitFile(t, warpPath, "a/first.txt", "a", "a")

	got, err := f.ChangedPaths(before, after)
	if err != nil {
		t.Fatalf("ChangedPaths() error = %v", err)
	}
	if want := []string{"a/first.txt", "z/last.txt"}; !slices.Equal(got, want) {
		t.Errorf("ChangedPaths() = %v; want %v", got, want)
	}

	none, err := f.ChangedPaths(after, after)
	if err != nil {
		t.Fatalf("ChangedPaths(same) error = %v", err)
	}
	if len(none) != 0 {
		t.Errorf("ChangedPaths(same) = %v; want none", none)
	}
}

// TestHoldsWork covers a source the base lacks, one the base holds after a squash, and a conflicting one.
func TestHoldsWork(t *testing.T) {
	t.Parallel()

	f, warpPath, _, _, _, _, _ := buildReconcileFixture(t, t.TempDir(), 1)
	gitkit.Git(t, warpPath, "checkout", "-b", "feature")
	featureTip := gitkit.CommitFile(t, warpPath, "feature.txt", "feature", "feature work")
	gitkit.Git(t, warpPath, "checkout", "main")

	check := func(step, base, source string, want bool) {
		t.Helper()
		got, err := f.HoldsWork(base, source)
		if err != nil {
			t.Fatalf("%s: HoldsWork() error = %v", step, err)
		}
		if got != want {
			t.Errorf("%s: HoldsWork(%s, %s) = %v; want %v", step, base, source, got, want)
		}
	}

	check("before the squash", "main", featureTip, false)

	gitkit.Git(t, warpPath, "merge", "--squash", "feature")
	gitkit.Git(t, warpPath, "commit", "-m", "squash feature")
	check("after the squash", "main", featureTip, true)

	gitkit.Git(t, warpPath, "checkout", "-b", "clash", "feature")
	clashTip := gitkit.CommitFile(t, warpPath, "feature.txt", "clash", "clash work")
	gitkit.Git(t, warpPath, "checkout", "main")
	check("conflicting source", "main", clashTip, false)
}

// TestFastForwardPair_MovesBehindPairToUpstream asserts the code side lands on the upstream tip, and that a weft whose upstream also moved is advanced and recorded.
func TestFastForwardPair_MovesBehindPairToUpstream(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	f, warpPath, bareDir, weftFixture, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)

	weftBranch := fabricengine.RecordsBranchName("main")
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "push", "-u", "origin", weftBranch)
	cloneB := filepath.Join(fixturesDir, "weft-cloneB")
	gitkit.MustRun(t, fixturesDir, "git", "clone", "-q", "-b", weftBranch, weftFixture.RecordsBare, cloneB)
	gitkit.Git(t, cloneB, "config", "user.email", "test@test.com")
	gitkit.Git(t, cloneB, "config", "user.name", "Test")
	weftTip := gitkit.CommitFile(t, cloneB, "from-clone-b.txt", "b", "b")
	gitkit.MustRun(t, cloneB, "git", "push", "-q")

	upstream := pushFromSecondClone(t, fixturesDir, bareDir, "other")

	res, err := f.FastForwardPair(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("FastForwardPair() error = %v", err)
	}
	if res.Refusal != "" || res.NewHead != upstream {
		t.Errorf("FastForwardPair() Refusal = %q, NewHead = %q; want no refusal and %q", res.Refusal, res.NewHead, upstream)
	}
	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != upstream {
		t.Errorf("code HEAD = %q; want the upstream tip %q", got, upstream)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords()); got != weftTip {
		t.Errorf("weft HEAD = %q; want its upstream tip %q", got, weftTip)
	}

	var kinds []fabricengine.Kind
	var details []string
	for _, entry := range res.Mutated().Entries() {
		kinds = append(kinds, entry.Kind)
		details = append(details, entry.Detail)
	}
	if !slices.Contains(kinds, fabricengine.KindWorktreeReset) {
		t.Errorf("record kinds = %v; want the reset entry", kinds)
	}
	for _, advance := range []string{weftTip, upstream} {
		if !slices.Contains(details, advance) {
			t.Errorf("record details = %v; want an advance entry carrying %q", details, advance)
		}
	}
}

// TestFastForwardPair_WeftWithoutUpstreamStillMovesCodeSide asserts a weft with no upstream is skipped, not an error.
func TestFastForwardPair_WeftWithoutUpstreamStillMovesCodeSide(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	bareDir := addWarpBareRemote(t, fixturesDir, warpPath)

	weftPath := t.TempDir()
	gitkit.Git(t, weftPath, "init", "-q", "-b", "main-weft")
	gitkit.Git(t, weftPath, "config", "user.email", "test@test.com")
	gitkit.Git(t, weftPath, "config", "user.name", "Test")
	gitkit.CommitFile(t, weftPath, "seed.txt", "weft", "init")
	f := fabricengine.NewFabricForTest(t, warpPath, weftPath)

	upstream := pushFromSecondClone(t, fixturesDir, bareDir, "other")

	res, err := f.FastForwardPair(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("FastForwardPair() error = %v; want the weft skipped", err)
	}
	if res.NewHead != upstream {
		t.Errorf("FastForwardPair() NewHead = %q; want %q", res.NewHead, upstream)
	}
	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != upstream {
		t.Errorf("code HEAD = %q; want the upstream tip %q", got, upstream)
	}
}

// TestFastForwardPair_RefusalsLeavePairUnmovedWithNilError covers each refusal: nil error, no movement, an empty record.
func TestFastForwardPair_RefusalsLeavePairUnmovedWithNilError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// arrange leaves the pair in the refused state; the upstream is already ahead of the pair when it runs.
		arrange func(t *testing.T, f *fabricengine.Fabric, warpPath, fixturesDir, bareDir string)
		want    fabricengine.FastForwardRefusal
	}{
		{
			name: "a modified tracked file",
			arrange: func(t *testing.T, _ *fabricengine.Fabric, warpPath, _, _ string) {
				if err := os.WriteFile(filepath.Join(warpPath, "README"), []byte("uncommitted edit"), 0o644); err != nil {
					t.Fatalf("dirty tracked file: %v", err)
				}
			},
			want: fabricengine.FastForwardDirty,
		},
		{
			name: "a fabric merge record on the pair",
			arrange: func(t *testing.T, f *fabricengine.Fabric, warpPath, _, _ string) {
				head := fabricengine.CurrentSHAForTest(t, warpPath)
				if err := fabricengine.SaveMergeStateForTest(f, fabricengine.MergeStateForTest{
					Verb:        "merge-in",
					Source:      "feature",
					WarpStart:   head,
					WarpOutcome: "staged",
					StartedAt:   time.Now(),
				}); err != nil {
					t.Fatalf("SaveMergeStateForTest() error = %v", err)
				}
			},
			want: fabricengine.FastForwardMergeInProgress,
		},
		{
			name: "an upstream force-pushed after the caller's read",
			arrange: func(t *testing.T, f *fabricengine.Fabric, warpPath, fixturesDir, bareDir string) {
				if _, err := f.ReadParentTips("main", true); err != nil {
					t.Fatalf("ReadParentTips() error = %v", err)
				}
				rewriteWarpRemoteHistory(t, fixturesDir, bareDir, fabricengine.CurrentSHAForTest(t, warpPath)+"~1")
			},
			want: fabricengine.FastForwardNotFastForward,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixturesDir := t.TempDir()
			f, warpPath, bareDir, _, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)
			pushFromSecondClone(t, fixturesDir, bareDir, "other")
			tc.arrange(t, f, warpPath, fixturesDir, bareDir)
			before := fabricengine.CurrentSHAForTest(t, warpPath)

			res, err := f.FastForwardPair(fabricengine.SyncOptions{})
			if err != nil {
				t.Fatalf("FastForwardPair() error = %v; want a refusal with a nil error", err)
			}
			if res.Refusal != tc.want {
				t.Errorf("FastForwardPair() Refusal = %q; want %q", res.Refusal, tc.want)
			}
			if res.NewHead != "" {
				t.Errorf("FastForwardPair() NewHead = %q; want empty on a refusal", res.NewHead)
			}
			if got := fabricengine.CurrentSHAForTest(t, warpPath); got != before {
				t.Errorf("code HEAD = %q after a refusal; want unmoved %q", got, before)
			}
			if n := res.Mutated().Len(); n != 0 {
				t.Errorf("record length = %d after a refusal; want 0", n)
			}
		})
	}
}
