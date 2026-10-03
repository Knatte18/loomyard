//go:build integration

// gitsha_integration_test.go holds the SHA/commit-message-reading fixture helpers export_test.go
// used to carry: CurrentSHAForTest, commitWarp, BareBranchSHAForTest, and commitMessageAt, plus their ForTest
// re-exports. They live here rather than in export_test.go because each spawns git directly via
// os/exec.Command to capture its output, and every one of their callers is itself
// integration-tagged; an untagged export_test.go carrying a raw exec.Command call would trip the
// Test Tier Purity Invariant regardless of which build tag its callers carry.

package fabricengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// CurrentSHAForTest returns dir's HEAD commit SHA for the external test package.
func CurrentSHAForTest(t *testing.T, dir string) string {
	t.Helper()
	return gitkit.RevParse(t, dir, "HEAD")
}

// CommitWarpForTest re-exports commitWarp (relocated fixture helper, formerly
// index_integration_test.go): several of the nine relocating files need it before
// index_integration_test.go's own migration card lands.
var CommitWarpForTest = commitWarp

// commitWarp creates a new commit in warpPath carrying content, returning the new HEAD SHA.
func commitWarp(t *testing.T, warpPath, content string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(warpPath, "README"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, warpPath, "git", "add", ".")
	gitkit.MustRun(t, warpPath, "git", "commit", "-q", "-m", content)
	return gitkit.RevParse(t, warpPath, "HEAD")
}

// BareBranchSHAForTest returns the SHA that branch points to inside the bare repo at bareDir.
func BareBranchSHAForTest(t *testing.T, bareDir, branch string) string {
	t.Helper()
	return gitkit.RevParse(t, bareDir, branch)
}

// CommitMessageAtForTest re-exports commitMessageAt (relocated fixture helper, formerly
// syncweft_integration_test.go): commitweftat_test.go (package fabricengine, never migrating) calls it
// unqualified, and commit_integration_test.go needs it before syncweft_integration_test.go's own
// migration card lands.
var CommitMessageAtForTest = commitMessageAt

// commitMessageAt returns rev's full raw commit message (subject + body + trailers) in repoPath, via
// `git log --format=%B`.
func commitMessageAt(t *testing.T, repoPath, rev string) string {
	t.Helper()

	cmd := exec.Command("git", "log", "-1", "--format=%B", rev)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log -1 --format=%%B %s in %s: %v", rev, repoPath, err)
	}
	return string(out)
}
