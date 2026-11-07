// gitsha_integration_test.go holds the fixture helpers that read commit SHAs and commit messages from git.
// They live here rather than in export_test.go because each spawns git directly through os/exec.Command,
// and every caller is integration-tagged.
// An untagged export_test.go with a raw exec.Command call would trip the Test Tier Purity Invariant.
// That holds whatever build tag the callers carry.

package fabricengine

import (
	"os/exec"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// CurrentSHAForTest returns dir's HEAD commit SHA for the external test package.
func CurrentSHAForTest(t *testing.T, dir string) string {
	t.Helper()
	return gitkit.RevParse(t, dir, "HEAD")
}

// CommitWarpForTest commits content as the README of warpPath, using content as the commit message.
// It returns the new HEAD SHA.
func CommitWarpForTest(t *testing.T, warpPath, content string) string {
	t.Helper()
	return gitkit.CommitFile(t, warpPath, "README", content, content)
}

// BareBranchSHAForTest returns the SHA that branch points to inside the bare repo at bareDir.
func BareBranchSHAForTest(t *testing.T, bareDir, branch string) string {
	t.Helper()
	return gitkit.RevParse(t, bareDir, branch)
}

// CommitMessageAtForTest returns a revision's full raw commit message for the external test package.
var CommitMessageAtForTest = commitMessageAt

// commitMessageAt returns rev's full raw commit message (subject, body and trailers) in repoPath.
// It reads the message with `git log --format=%B`.
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
