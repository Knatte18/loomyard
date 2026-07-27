//go:build integration || smoke

// gitfixture_test.go holds the scratch-git-repo helpers shared by the
// integration tier (poll_test.go, spawnbatch_test.go,
// weft_integration_test.go) and the smoke tier (smoke_test.go): both tiers
// spawn real git, so the helpers carry a dual build tag rather than living
// in a single-tier file -- smoke_test.go referencing them inside an
// integration-only file is exactly the mismatch that once made
// `go test -tags smoke` fail to compile for this package.

package buildercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// newScratchRepo initializes a fresh git repo at t.TempDir(), configures a
// test identity, and returns its path -- the same minimal recipe
// builderengine's own gitquery_test.go uses, reimplemented here since it is
// package-private there.
func newScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.name", "Test User")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	return dir
}

// mustGit runs a git command in dir via gitexec.RunGit, failing the test on
// any spawn error or non-zero exit.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	stdout, stderr, exitCode, err := gitexec.RunGit(args, dir)
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	if exitCode != 0 {
		t.Fatalf("git %v in %s exited %d: %s", args, dir, exitCode, stderr)
	}
	return stdout
}

// commitFile writes name/content in dir and commits it, returning the new
// HEAD SHA.
func commitFile(t *testing.T, dir, name, content, message string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	mustGit(t, dir, "add", name)
	mustGit(t, dir, "commit", "-m", message)
	return strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))
}
