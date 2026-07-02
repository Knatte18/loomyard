//go:build integration

// unjunction_test.go covers UnwireJunctions, the mirror-image reversal of
// WireJunctions: happy-path removal, the never-wired no-op, and the two hard-error
// guards (a real directory in place of the junction, and a junction pointing at an
// unexpected target).

package warpengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// readExcludeContent resolves and reads the host worktree's .git/info/exclude
// file, mirroring the resolution logic in seedGitExclude/unseedGitExclude so
// tests observe the same path the production code writes to.
func readExcludeContent(t *testing.T, l *hubgeometry.Layout, slug string) string {
	t.Helper()

	worktreePath := l.WorktreePath(slug)
	stdout, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
	if exitCode != 0 {
		t.Fatalf("git rev-parse --git-path info/exclude failed")
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}

	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude file: %v", err)
	}
	return string(content)
}

// excludeContainsLine reports whether content contains name as a line-exact match.
func excludeContainsLine(content, name string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

// TestUnwireJunctions_HappyPath verifies that UnwireJunctions reverses a prior
// WireJunctions call: the host junction is removed and its exclude line is gone.
func TestUnwireJunctions_HappyPath(t *testing.T) {
	t.Parallel()

	const slug = "unwire-happy-path"

	f := lyxtest.CopyPairedLocal(t)

	w := New(Config{})
	if _, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions(%q): %v", slug, err)
	}

	result, err := UnwireJunctions(f.Layout, slug)
	if err != nil {
		t.Fatalf("UnwireJunctions(%q): %v", slug, err)
	}
	if !result.JunctionRemoved {
		t.Errorf("UnwireJunctions(%q).JunctionRemoved = false; want true", slug)
	}
	if !result.ExcludeChanged {
		t.Errorf("UnwireJunctions(%q).ExcludeChanged = false; want true", slug)
	}

	hostLink := f.Layout.HostLyxLink(slug)
	if _, statErr := os.Lstat(hostLink); !os.IsNotExist(statErr) {
		t.Errorf("UnwireJunctions(%q): host junction %s still exists", slug, hostLink)
	}

	content := readExcludeContent(t, f.Layout, slug)
	if excludeContainsLine(content, hubgeometry.LyxDirName) {
		t.Errorf("UnwireJunctions(%q): exclude file still contains %q line", slug, hubgeometry.LyxDirName)
	}
}

// TestUnwireJunctions_NeverWired verifies that UnwireJunctions is a clean no-op
// when WireJunctions was never called for the slug.
func TestUnwireJunctions_NeverWired(t *testing.T) {
	t.Parallel()

	const slug = "unwire-never-wired"

	f := lyxtest.CopyPairedLocal(t)

	w := New(Config{})
	if _, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	result, err := UnwireJunctions(f.Layout, slug)
	if err != nil {
		t.Fatalf("UnwireJunctions(%q): %v", slug, err)
	}
	if result != (UnwireResult{}) {
		t.Errorf("UnwireJunctions(%q) = %+v; want zero UnwireResult", slug, result)
	}
}

// TestUnwireJunctions_RealDirectoryGuard verifies that UnwireJunctions refuses to
// remove a real (non-junction) directory sitting at the host _lyx path, and leaves
// it and the exclude file untouched.
func TestUnwireJunctions_RealDirectoryGuard(t *testing.T) {
	t.Parallel()

	const slug = "unwire-real-directory-guard"

	f := lyxtest.CopyPairedLocal(t)

	w := New(Config{})
	if _, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// unseedLyxJunction resolves the weft-side target via filepath.EvalSymlinks
	// before checking fslink.IsLink, so the target must exist first or this test
	// would hit the "missing target" branch instead of the intended "real
	// directory" branch.
	if err := os.MkdirAll(f.Layout.WeftLyxDirFor(slug), 0o755); err != nil {
		t.Fatalf("mkdir weft target: %v", err)
	}

	hostLink := f.Layout.HostLyxLink(slug)
	if err := os.MkdirAll(hostLink, 0o755); err != nil {
		t.Fatalf("mkdir host real directory: %v", err)
	}
	marker := filepath.Join(hostLink, "marker.txt")
	if err := os.WriteFile(marker, []byte("real content"), 0o644); err != nil {
		t.Fatalf("write marker file: %v", err)
	}

	before := readExcludeContent(t, f.Layout, slug)

	result, err := UnwireJunctions(f.Layout, slug)
	if err == nil {
		t.Fatalf("UnwireJunctions(%q) error = nil; want error", slug)
	}
	if result != (UnwireResult{}) {
		t.Errorf("UnwireJunctions(%q) = %+v; want zero UnwireResult on error", slug, result)
	}

	// The real directory and its content must be untouched.
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Errorf("UnwireJunctions(%q): marker file missing; want untouched: %v", slug, statErr)
	}

	after := readExcludeContent(t, f.Layout, slug)
	if before != after {
		t.Errorf("UnwireJunctions(%q): exclude file changed; want untouched", slug)
	}
}

// TestUnwireJunctions_TargetMismatch verifies that UnwireJunctions refuses to
// remove a junction that resolves to an unexpected target, leaving both the
// junction and the exclude file untouched.
func TestUnwireJunctions_TargetMismatch(t *testing.T) {
	t.Parallel()

	const slug = "unwire-target-mismatch"

	f := lyxtest.CopyPairedLocal(t)

	w := New(Config{})
	if _, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions(%q): %v", slug, err)
	}

	// Replace the valid junction with one pointing at an unrelated directory.
	hostLink := f.Layout.HostLyxLink(slug)
	if err := fslink.Remove(hostLink); err != nil {
		t.Fatalf("remove junction to replace it: %v", err)
	}
	wrongTarget := filepath.Join(f.Hub, "unrelated-target")
	if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
		t.Fatalf("mkdir unrelated target: %v", err)
	}
	if err := fslink.CreateDirLink(hostLink, wrongTarget); err != nil {
		t.Fatalf("CreateDirLink(%s, %s): %v", hostLink, wrongTarget, err)
	}

	before := readExcludeContent(t, f.Layout, slug)

	result, err := UnwireJunctions(f.Layout, slug)
	if err == nil {
		t.Fatalf("UnwireJunctions(%q) error = nil; want error", slug)
	}
	if result != (UnwireResult{}) {
		t.Errorf("UnwireJunctions(%q) = %+v; want zero UnwireResult on error", slug, result)
	}

	// The mismatched junction must still exist afterward.
	isLink, err := fslink.IsLink(hostLink)
	if err != nil {
		t.Fatalf("fslink.IsLink(%s): %v", hostLink, err)
	}
	if !isLink {
		t.Errorf("UnwireJunctions(%q): mismatched junction %s no longer a link", slug, hostLink)
	}

	after := readExcludeContent(t, f.Layout, slug)
	if before != after {
		t.Errorf("UnwireJunctions(%q): exclude file changed; want untouched", slug)
	}
}

// TestUnwireJunctions_Subpath mirrors TestRemoveSubpathJunction's fixture setup to
// verify UnwireJunctions correctly resolves and removes a nested junction when the
// Layout's RelPath is non-trivial (cwd inside a subdirectory of the hub).
func TestUnwireJunctions_Subpath(t *testing.T) {
	const slug = "unwire-subpath-junction-test"
	const subpath = "sub/path"

	f := lyxtest.CopyPairedLocal(t)

	subpathDir := filepath.Join(f.Hub, subpath)
	if err := os.MkdirAll(subpathDir, 0755); err != nil {
		t.Fatalf("mkdir subpath: %v", err)
	}

	t.Chdir(subpathDir)

	l, err := hubgeometry.Resolve(subpathDir)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve: %v", err)
	}
	if l.RelPath == "." {
		t.Skip("this test requires RelPath != \".\"; got: " + l.RelPath)
	}

	// The weft repo's Prime template only checks out a root-level _lyx directory,
	// not one nested under the subpath, so the nested weft-side target does not
	// exist purely from Add. unseedLyxJunction resolves that target via
	// filepath.EvalSymlinks before removing the link (same guard as
	// TestUnwireJunctions_RealDirectoryGuard exercises), so create it directly here
	// to exercise the legitimate removal path rather than the missing-target guard.
	w := New(Config{})
	if _, err := w.Add(l, slug, AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q) at subpath: %v", slug, err)
	}
	if err := os.MkdirAll(l.WeftLyxDirFor(slug), 0o755); err != nil {
		t.Fatalf("mkdir nested weft target: %v", err)
	}
	if err := WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions(%q) at subpath: %v", slug, err)
	}

	hostLink := l.HostLyxLink(slug)
	if _, err := os.Lstat(hostLink); err != nil {
		t.Fatalf("nested host junction missing before unwire: %v", err)
	}

	result, err := UnwireJunctions(l, slug)
	if err != nil {
		t.Fatalf("UnwireJunctions(%q) at subpath: %v", slug, err)
	}
	if !result.JunctionRemoved {
		t.Errorf("UnwireJunctions(%q) at subpath: JunctionRemoved = false; want true", slug)
	}

	if _, err := os.Lstat(hostLink); !os.IsNotExist(err) {
		t.Errorf("UnwireJunctions(%q) at subpath failed to remove nested junction at %s", slug, hostLink)
	}
}
