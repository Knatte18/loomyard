// recordspaths_test.go covers RecordsPathsAt, IsRecordsOnlyCommit and IsFabricMergeCommit against hermetic repositories.
// The tests rely on the package TestMain in testmain_test.go, which runs gitkit.HermeticGitEnv().

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// commitPaths writes files into dir, stages exactly those paths and commits them under message.
// It returns the new HEAD.
func commitPaths(t *testing.T, dir, message string, files map[string]string) string {
	t.Helper()

	var paths []string
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
		paths = append(paths, rel)
	}
	gitkit.Git(t, dir, slices.Concat([]string{"add", "--"}, paths)...)
	gitkit.Git(t, dir, "commit", "-q", "-m", message)
	return gitkit.RevParse(t, dir, "HEAD")
}

// commitParentless commits files as a parentless commit on a new orphan branch.
// It then returns to the branch checked out before.
func commitParentless(t *testing.T, dir, orphanBranch string, files map[string]string) string {
	t.Helper()

	previous := gitkit.CurrentBranch(t, dir)
	gitkit.Git(t, dir, "checkout", "-q", "--orphan", orphanBranch)
	gitkit.Git(t, dir, "rm", "-rf", "-q", ".")
	sha := commitPaths(t, dir, "parentless", files)
	gitkit.Git(t, dir, "checkout", "-q", "-f", previous)
	return sha
}

func TestRecordsPathsAt(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir := fabricengine.NewPlainWarpRepoForTest(t)
	noMarker := commitPaths(t, dir, "code only", map[string]string{"code.txt": "v1"})
	rootAnchor := commitPaths(t, dir, "root anchor", map[string]string{".lym-anchor": ".\n", "_lym/plan.md": "v1"})
	anchorChanged := commitPaths(t, dir, "move the anchor", map[string]string{".lym-anchor": "backend\n", "backend/_lym/plan.md": "v1"})
	subpathAnchor := commitPaths(t, dir, "edit under the subpath", map[string]string{"backend/_lym/plan.md": "v2"})
	rootCommit := commitParentless(t, dir, "orphan", map[string]string{".lym-anchor": ".\n", "_lym/plan.md": "v1"})

	tests := []struct {
		name string
		rev  string
		want []string
	}{
		{"RootAnchor", rootAnchor, []string{".lym-anchor", "_lym"}},
		{"SubpathAnchor", subpathAnchor, []string{".lym-anchor", "backend/_lym"}},
		{"AnchorChangedInTheCommit", anchorChanged, []string{".lym-anchor", "_lym", "backend/_lym"}},
		{"NoMarker", noMarker, []string{".lym-anchor"}},
		{"RootCommit", rootCommit, []string{".lym-anchor", "_lym"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := fabricengine.RecordsPathsAt(dir, tt.rev)
			if err != nil {
				t.Fatalf("RecordsPathsAt(%s) error = %v; want nil", tt.rev, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("RecordsPathsAt(%s) = %v; want %v", tt.rev, got, tt.want)
			}
		})
	}
}

// mergeKind says how mergeSide builds the merge commit of a side branch into main.
type mergeKind struct {
	// trailer adds the fabric merge trailer to the merge message.
	trailer bool
	// carrySourceRecords keeps the side's records in the merge instead of restoring the receiver's.
	carrySourceRecords bool
	// conflictOnCode makes both branches edit one code file, so the merge is resolved by hand.
	conflictOnCode bool
}

// mergeSide forks a side branch at main's tip and gives the side a code file and a records file.
// It gives main a records edit, then merges the side into main as kind says.
// It returns the merge commit.
func mergeSide(t *testing.T, dir, name string, kind mergeKind) string {
	t.Helper()

	gitkit.Git(t, dir, "checkout", "-q", "-b", "side-"+name)
	sideFiles := map[string]string{"side-" + name + ".txt": "side", "_lym/side-" + name + ".md": "side"}
	if kind.conflictOnCode {
		sideFiles["conflict.txt"] = "side"
	}
	commitPaths(t, dir, "side "+name, sideFiles)
	gitkit.Git(t, dir, "checkout", "-q", "main")
	mainFiles := map[string]string{"_lym/plan.md": "receiver-" + name}
	if kind.conflictOnCode {
		mainFiles["conflict.txt"] = "main"
	}
	commitPaths(t, dir, "main "+name, mainFiles)

	// A conflicted merge exits non-zero and leaves the merge open, which is the state the test resolves next.
	_, _ = gitexec.Run([]string{"merge", "--no-commit", "--no-ff", "side-" + name}, dir)
	if kind.conflictOnCode {
		if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("resolved by hand"), 0o644); err != nil {
			t.Fatalf("write the hand resolution: %v", err)
		}
		gitkit.Git(t, dir, "add", "--", "conflict.txt")
	}
	if !kind.carrySourceRecords {
		gitkit.Git(t, dir, "rm", "-rf", "-q", "--ignore-unmatch", "_lym")
		gitkit.Git(t, dir, "checkout", "HEAD", "--", "_lym", ".lym-anchor")
	}
	message := "merge " + name
	if kind.trailer {
		message += "\n\n" + fabricengine.MergeTrailerKey + ": " + name
	}
	gitkit.Git(t, dir, "commit", "-q", "-m", message)
	return gitkit.RevParse(t, dir, "HEAD")
}

func TestCommitPredicates(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir := fabricengine.NewPlainWarpRepoForTest(t)
	commitPaths(t, dir, "records and code", map[string]string{".lym-anchor": ".\n", "_lym/plan.md": "v1", "code.txt": "v1"})
	recordsEdit := commitPaths(t, dir, "records edit", map[string]string{"_lym/plan.md": "v2"})
	markerEdit := commitPaths(t, dir, "marker edit", map[string]string{".lym-anchor": ".\n\n"})
	codeEdit := commitPaths(t, dir, "code edit", map[string]string{"code.txt": "v2"})
	codeEditWithTrailer := commitPaths(t, dir, "code edit\n\n"+fabricengine.MergeTrailerKey+": claimed", map[string]string{"code.txt": "v3"})
	commitPaths(t, dir, "shared file", map[string]string{"conflict.txt": "base"})
	rootCommit := commitParentless(t, dir, "orphan", map[string]string{".lym-anchor": ".\n", "_lym/plan.md": "v1"})

	merged := mergeSide(t, dir, "clean", mergeKind{trailer: true})
	withoutTrailer := mergeSide(t, dir, "plain", mergeKind{})
	carrying := mergeSide(t, dir, "carry", mergeKind{trailer: true, carrySourceRecords: true})
	handResolved := mergeSide(t, dir, "conflict", mergeKind{trailer: true, conflictOnCode: true})

	gitkit.Git(t, dir, "checkout", "-q", "-b", "side-squash")
	commitPaths(t, dir, "task work", map[string]string{"task.txt": "task"})
	gitkit.Git(t, dir, "checkout", "-q", "main")
	gitkit.Git(t, dir, "merge", "--squash", "side-squash")
	gitkit.Git(t, dir, "commit", "-q", "-m", "squash")
	squash := gitkit.RevParse(t, dir, "HEAD")

	tests := []struct {
		name            string
		sha             string
		wantRecordsOnly bool
		wantFabricMerge bool
	}{
		{"RecordsOnlyEdit", recordsEdit, true, false},
		{"MarkerEditOnly", markerEdit, true, false},
		{"CodeEdit", codeEdit, false, false},
		{"CodeEditCarryingTheMergeTrailer", codeEditWithTrailer, false, false},
		{"MergeWithTrailerAndReceiverRecords", merged, false, true},
		{"MergeWithoutTrailer", withoutTrailer, false, false},
		{"MergeCarryingSourceRecords", carrying, false, false},
		{"MergeWithHandResolvedCodeConflict", handResolved, false, false},
		{"SquashOfATask", squash, false, false},
		{"RootCommitOfRecordsOnly", rootCommit, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotRecordsOnly, err := fabricengine.IsRecordsOnlyCommit(dir, tt.sha)
			if err != nil {
				t.Fatalf("IsRecordsOnlyCommit(%s) error = %v; want nil", tt.sha, err)
			}
			if gotRecordsOnly != tt.wantRecordsOnly {
				t.Errorf("IsRecordsOnlyCommit(%s) = %v; want %v", tt.sha, gotRecordsOnly, tt.wantRecordsOnly)
			}
			gotFabricMerge, err := fabricengine.IsFabricMergeCommit(dir, tt.sha)
			if err != nil {
				t.Fatalf("IsFabricMergeCommit(%s) error = %v; want nil", tt.sha, err)
			}
			if gotFabricMerge != tt.wantFabricMerge {
				t.Errorf("IsFabricMergeCommit(%s) = %v; want %v", tt.sha, gotFabricMerge, tt.wantFabricMerge)
			}
		})
	}
}
