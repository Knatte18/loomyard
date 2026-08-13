//go:build integration

// gitsha_integration_test.go holds the SHA/commit-message-reading fixture helpers export_test.go
// used to carry: currentSHA, commitWarp, bareBranchSHA, and commitMessageAt, plus their ForTest
// re-exports. They live here rather than in export_test.go because each spawns git directly via
// os/exec.Command to capture its output, and every one of their callers is itself
// integration-tagged; an untagged export_test.go carrying a raw exec.Command call would trip the
// Test Tier Purity Invariant regardless of which build tag its callers carry.

package fabricengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// CurrentSHAForTest re-exports currentSHA (relocated fixture helper, formerly
// index_integration_test.go): several of the nine relocating files need it before
// index_integration_test.go's own migration card lands.
var CurrentSHAForTest = currentSHA

// currentSHA returns dir's HEAD commit SHA.
func currentSHA(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
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
	return currentSHA(t, warpPath)
}

// BareBranchSHAForTest re-exports bareBranchSHA (relocated fixture helper, formerly
// coalesce_integration_test.go): bolt_integration_test.go (package fabricengine, never migrating)
// calls it unqualified.
var BareBranchSHAForTest = bareBranchSHA

// bareBranchSHA returns the SHA that branch points to inside the bare repo at bareDir.
func bareBranchSHA(t *testing.T, bareDir, branch string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", branch)
	cmd.Dir = bareDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s in %s: %v", branch, bareDir, err)
	}
	return strings.TrimSpace(string(out))
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
