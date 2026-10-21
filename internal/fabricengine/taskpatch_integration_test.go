//go:build integration

// taskpatch_integration_test.go proves the code-identity reads of taskpatch.go over real git checkouts:
// ReadTaskPatch's patch, hashes and skips, and MergeInOnly's verdict on each way a task's history can move.

package fabricengine_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// taskPatchFixture is a warp checkout on a clean task branch forked from main, with its weft side beside it.
type taskPatchFixture struct {
	fabric   *fabricengine.Fabric
	warpDir  string
	weftDir  string
	baseSHA  string
	baseFile string
}

// newTaskPatchFixture returns a fixture whose task branch stands at main's tip, which holds a twenty-line lib.txt.
func newTaskPatchFixture(t *testing.T) taskPatchFixture {
	t.Helper()

	warpDir := fabricengine.NewPlainWarpRepoForTest(t)
	var lines strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&lines, "line %d\n", i)
	}
	baseSHA := gitkit.CommitFile(t, warpDir, "lib.txt", lines.String(), "add lib")
	gitkit.Git(t, warpDir, "checkout", "-q", "-b", "task")

	weftDir := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."}).PrimeRecords()
	return taskPatchFixture{
		fabric:   fabricengine.NewFabricForTest(t, warpDir, weftDir),
		warpDir:  warpDir,
		weftDir:  weftDir,
		baseSHA:  baseSHA,
		baseFile: lines.String(),
	}
}

// writeTaskPatchFile writes content to rel under dir without staging it.
func writeTaskPatchFile(t *testing.T, dir, rel, content string) {
	t.Helper()

	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// diffHeader returns the header git opens path's diff with.
func diffHeader(path string) string {
	return "diff --git a/" + path + " b/" + path
}

func readTaskPatch(t *testing.T, fx taskPatchFixture, base string, skip []string) fabricengine.TaskPatch {
	t.Helper()

	patch, err := fx.fabric.ReadTaskPatch(base, skip)
	if err != nil {
		t.Fatalf("ReadTaskPatch(%q, %v) error = %v", base, skip, err)
	}
	return patch
}

// TestFabric_ReadTaskPatch covers which changes the patch holds, how skip exempts untracked paths, and what its hashes follow.
func TestFabric_ReadTaskPatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(t *testing.T, fx taskPatchFixture)
		skip      []string
		wantPaths []string
		wantNot   []string
	}{
		{
			name: "CommittedAndUncommittedChanges",
			mutate: func(t *testing.T, fx taskPatchFixture) {
				gitkit.CommitFile(t, fx.warpDir, "task.txt", "committed\n", "task change")
				writeTaskPatchFile(t, fx.warpDir, "README", "edited")
			},
			wantPaths: []string{"task.txt", "README"},
		},
		{
			name: "UntrackedFileAppears",
			mutate: func(t *testing.T, fx taskPatchFixture) {
				writeTaskPatchFile(t, fx.warpDir, "untracked.txt", "new\n")
				writeTaskPatchFile(t, fx.warpDir, "empty.txt", "")
			},
			wantPaths: []string{"untracked.txt", "empty.txt"},
		},
		{
			name: "IgnoredFileIsLeftOut",
			mutate: func(t *testing.T, fx taskPatchFixture) {
				gitkit.CommitFile(t, fx.warpDir, ".gitignore", "ignored.txt\n", "ignore it")
				writeTaskPatchFile(t, fx.warpDir, "ignored.txt", "x\n")
			},
			wantNot: []string{"ignored.txt"},
		},
		{
			name: "SkippedUntrackedFileIsLeftOut",
			mutate: func(t *testing.T, fx taskPatchFixture) {
				writeTaskPatchFile(t, fx.warpDir, "skipme.txt", "x\n")
				writeTaskPatchFile(t, fx.warpDir, "kept.txt", "y\n")
			},
			skip:      []string{"skipme.txt"},
			wantPaths: []string{"kept.txt"},
			wantNot:   []string{"skipme.txt"},
		},
		{
			name: "UntrackedFileUnderSkippedDirectoryIsLeftOut",
			mutate: func(t *testing.T, fx taskPatchFixture) {
				writeTaskPatchFile(t, fx.warpDir, "gen/out.txt", "x\n")
				writeTaskPatchFile(t, fx.warpDir, "genuine.txt", "y\n")
			},
			skip:      []string{"gen/"},
			wantPaths: []string{"genuine.txt"},
			wantNot:   []string{"gen/out.txt"},
		},
		{
			name: "TrackedFileOfSkippedNameCounts",
			mutate: func(t *testing.T, fx taskPatchFixture) {
				gitkit.CommitFile(t, fx.warpDir, "keep.txt", "one\n", "add keep")
				writeTaskPatchFile(t, fx.warpDir, "keep.txt", "two\n")
			},
			skip:      []string{"keep.txt"},
			wantPaths: []string{"keep.txt"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newTaskPatchFixture(t)
			tt.mutate(t, fx)

			patch := readTaskPatch(t, fx, fx.baseSHA, tt.skip)

			for _, path := range tt.wantPaths {
				if header := diffHeader(path); !bytes.Contains(patch.Full, []byte(header)) {
					t.Errorf("ReadTaskPatch Full lacks %q:\n%s", header, patch.Full)
				}
			}
			for _, path := range tt.wantNot {
				if header := diffHeader(path); bytes.Contains(patch.Full, []byte(header)) {
					t.Errorf("ReadTaskPatch Full holds %q, want it left out:\n%s", header, patch.Full)
				}
			}
			sum := sha256.Sum256(patch.Full)
			if want := hex.EncodeToString(sum[:]); patch.FullHash != want {
				t.Errorf("ReadTaskPatch FullHash = %q; want the SHA-256 of Full, %q", patch.FullHash, want)
			}
			if want := gitkit.RevParse(t, fx.warpDir, "HEAD"); patch.Head != want {
				t.Errorf("ReadTaskPatch Head = %q; want %q", patch.Head, want)
			}
		})
	}

	t.Run("HashesFollowTheChange", func(t *testing.T) {
		t.Parallel()
		fx := newTaskPatchFixture(t)

		clean := readTaskPatch(t, fx, fx.baseSHA, nil)
		if len(clean.Full) != 0 || clean.PatchID != "" {
			t.Errorf("ReadTaskPatch of an unchanged tree = %d bytes, PatchID %q; want an empty patch", len(clean.Full), clean.PatchID)
		}

		writeTaskPatchFile(t, fx.warpDir, "untracked.txt", "new\n")
		withUntracked := readTaskPatch(t, fx, fx.baseSHA, nil)
		if withUntracked.FullHash == clean.FullHash || withUntracked.PatchID == "" {
			t.Errorf("an untracked file left FullHash %q (clean %q) and PatchID %q; want a changed hash and a patch-id", withUntracked.FullHash, clean.FullHash, withUntracked.PatchID)
		}

		fabricengine.WriteWeftConfigContentForTest(t, fx.weftDir, "status: changed\n")
		afterWeft := readTaskPatch(t, fx, fx.baseSHA, nil)
		if afterWeft.FullHash != withUntracked.FullHash {
			t.Errorf("a weft-side change moved FullHash from %q to %q; want it equal", withUntracked.FullHash, afterWeft.FullHash)
		}
	})

	t.Run("PatchIDSurvivesARebaseOntoAMovedBase", func(t *testing.T) {
		t.Parallel()
		fx := newTaskPatchFixture(t)

		edited := strings.Replace(fx.baseFile, "line 15\n", "line 15 changed\n", 1)
		gitkit.CommitFile(t, fx.warpDir, "lib.txt", edited, "task edits line 15")
		before := readTaskPatch(t, fx, fx.baseSHA, nil)

		// The parent adds a line at the top, which shifts every line number and changes the file's blob.
		movedBase := gitkit.CommitFileOnBranch(t, fx.warpDir, "main", "lib.txt", "line 0\n"+fx.baseFile, "parent adds a top line")
		gitkit.Git(t, fx.warpDir, "checkout", "-q", "task")
		gitkit.Git(t, fx.warpDir, "rebase", "-q", "main")
		after := readTaskPatch(t, fx, movedBase, nil)

		if after.PatchID == "" || after.PatchID != before.PatchID {
			t.Errorf("PatchID after the rebase = %q; want it equal to the one before, %q", after.PatchID, before.PatchID)
		}
		if after.FullHash == before.FullHash {
			t.Errorf("FullHash after the rebase = %q, unchanged; want the shifted context to change it", after.FullHash)
		}

		// The task moves its edit from line 15 to line 3.
		writeTaskPatchFile(t, fx.warpDir, "lib.txt", strings.Replace("line 0\n"+fx.baseFile, "line 3\n", "line 3 changed\n", 1))
		if moved := readTaskPatch(t, fx, movedBase, nil); moved.FullHash == after.FullHash {
			t.Errorf("FullHash after the task moved its edit = %q, unchanged; want it changed", moved.FullHash)
		}
	})
}

// TestFabric_MergeInOnly covers the verdict for each way the task's first-parent history can stand against the parent.
func TestFabric_MergeInOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// build moves the task branch and returns the since and parentTip arguments.
		build func(t *testing.T, fx taskPatchFixture) (since, parentTip string)
		want  bool
	}{
		{
			name: "SinceIsHead",
			build: func(t *testing.T, fx taskPatchFixture) (string, string) {
				return fx.baseSHA, fx.baseSHA
			},
			want: true,
		},
		{
			name: "AfterMergeOfTheParent",
			build: func(t *testing.T, fx taskPatchFixture) (string, string) {
				parentTip := gitkit.CommitFileOnBranch(t, fx.warpDir, "main", "parent.txt", "p\n", "parent change")
				gitkit.Git(t, fx.warpDir, "checkout", "-q", "task")
				gitkit.Git(t, fx.warpDir, "merge", "-q", "--no-ff", "-m", "merge main", "main")
				return fx.baseSHA, parentTip
			},
			want: true,
		},
		{
			name: "AfterFastForwardToTheParent",
			build: func(t *testing.T, fx taskPatchFixture) (string, string) {
				parentTip := gitkit.CommitFileOnBranch(t, fx.warpDir, "main", "parent.txt", "p\n", "parent change")
				gitkit.Git(t, fx.warpDir, "checkout", "-q", "task")
				gitkit.Git(t, fx.warpDir, "merge", "-q", "--ff-only", "main")
				return fx.baseSHA, parentTip
			},
			want: true,
		},
		{
			name: "AfterTaskCommit",
			build: func(t *testing.T, fx taskPatchFixture) (string, string) {
				gitkit.CommitFile(t, fx.warpDir, "task.txt", "t\n", "task change")
				return fx.baseSHA, fx.baseSHA
			},
			want: false,
		},
		{
			name: "AfterMergeOfASecondParentOutsideTheParent",
			build: func(t *testing.T, fx taskPatchFixture) (string, string) {
				gitkit.Git(t, fx.warpDir, "checkout", "-q", "-b", "other", fx.baseSHA)
				gitkit.CommitFile(t, fx.warpDir, "other.txt", "o\n", "other change")
				gitkit.Git(t, fx.warpDir, "checkout", "-q", "task")
				gitkit.Git(t, fx.warpDir, "merge", "-q", "--no-ff", "-m", "merge other", "other")
				return fx.baseSHA, fx.baseSHA
			},
			want: false,
		},
		{
			name: "SinceOffTheFirstParentHistory",
			build: func(t *testing.T, fx taskPatchFixture) (string, string) {
				parentTip := gitkit.CommitFileOnBranch(t, fx.warpDir, "main", "parent.txt", "p\n", "parent change")
				gitkit.Git(t, fx.warpDir, "checkout", "-q", "task")
				gitkit.Git(t, fx.warpDir, "merge", "-q", "--no-ff", "-m", "merge main", "main")
				return parentTip, parentTip
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newTaskPatchFixture(t)
			since, parentTip := tt.build(t, fx)

			got, err := fx.fabric.MergeInOnly(since, parentTip)
			if err != nil {
				t.Fatalf("MergeInOnly(%q, %q) error = %v", since, parentTip, err)
			}
			if got != tt.want {
				t.Errorf("MergeInOnly(%q, %q) = %v; want %v", since, parentTip, got, tt.want)
			}
		})
	}
}
