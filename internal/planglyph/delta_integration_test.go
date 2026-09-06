//go:build integration

// delta_integration_test.go covers Delta against a real two-commit fixture repository, spawning
// git through (*quarry.Repo).DeltaGit — the reason this file carries the integration build tag
// rather than running in the untagged tier.

package planglyph

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// deltaFixtureRepo is a throwaway git repository built fresh for one test under t.TempDir(), with
// a fixed identity and a fixed default branch name so no machine's global git configuration can
// change the fixture's behaviour.
type deltaFixtureRepo struct {
	t    *testing.T
	root string
}

// newDeltaFixtureRepo initializes a fresh git repository, skipping the whole test when no git
// binary is available on this machine.
func newDeltaFixtureRepo(t *testing.T) *deltaFixtureRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found on this machine")
	}

	f := &deltaFixtureRepo{t: t, root: t.TempDir()}
	f.git("init", "--quiet", "--initial-branch=main")
	f.git("config", "user.name", "planglyph-delta-fixture")
	f.git("config", "user.email", "planglyph-delta-fixture@example.com")
	return f
}

// git runs one git invocation against the fixture's root, failing the test immediately on error.
func (f *deltaFixtureRepo) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", f.root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// writeAndCommit writes content to path, repository-relative, and commits it, returning the
// resulting commit's SHA.
func (f *deltaFixtureRepo) writeAndCommit(path, content, message string) string {
	f.t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		f.t.Fatalf("mkdir %q: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		f.t.Fatalf("write %q: %v", full, err)
	}
	f.git("add", "-A")
	f.git("commit", "--quiet", "-m", message)
	return f.git("rev-parse", "HEAD")
}

// TestDelta_TwoCommitFixture builds a two-commit fixture repository, calls Delta across the two
// SHAs, and asserts the answer's From/To echo and that the symbol added in the second commit
// appears in Created.
func TestDelta_TwoCommitFixture(t *testing.T) {
	f := newDeltaFixtureRepo(t)
	from := f.writeAndCommit("sub/a.go", "package sub\n", "first commit")
	to := f.writeAndCommit("sub/a.go", "package sub\n\nfunc Foo() {}\n", "add Foo")

	answer, err := Delta(f.root, from, to)
	if err != nil {
		t.Fatalf("Delta(%q, %q, %q) returned error: %v", f.root, from, to, err)
	}
	if answer.From != from {
		t.Errorf("Delta(...).From = %q; want %q", answer.From, from)
	}
	if answer.To == nil || *answer.To != to {
		t.Errorf("Delta(...).To = %v; want a pointer to %q", answer.To, to)
	}

	found := false
	for _, s := range answer.Created {
		if s.ID == "sub#Foo" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Delta(...).Created = %+v; want %q present", answer.Created, "sub#Foo")
	}
}

// TestDelta_BadRevision asserts a bad revision returns an error satisfying
// errors.Is(err, ErrQuarryUnavailable).
func TestDelta_BadRevision(t *testing.T) {
	f := newDeltaFixtureRepo(t)
	from := f.writeAndCommit("sub/a.go", "package sub\n", "first commit")

	_, err := Delta(f.root, "does-not-exist-rev", from)
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("Delta(%q, %q, %q) error = %v; want errors.Is(err, ErrQuarryUnavailable)", f.root, "does-not-exist-rev", from, err)
	}
}
