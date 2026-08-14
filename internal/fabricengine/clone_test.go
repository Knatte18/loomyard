// clone_test.go — unit tests for URL-derivation helpers, cloneRepo's error path, and CloneHub's
// hub-scratch materialisation at <hub>/_board/.lyx.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

func TestDeriveWarpName(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "https with .git",
			url:  "https://github.com/u/repo.git",
			want: "repo",
		},
		{
			name: "https without .git",
			url:  "https://github.com/u/repo",
			want: "repo",
		},
		{
			name: "SCP form with .git",
			url:  "git@github.com:u/repo.git",
			want: "repo",
		},
		{
			name: "trailing slash",
			url:  "https://github.com/u/repo/",
			want: "repo",
		},
		{
			name: "Windows file path",
			url:  "C:\\path\\to\\repo.git",
			want: "repo",
		},
		{
			name: "empty string",
			url:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveWarpName(tt.url)
			if got != tt.want {
				t.Errorf("DeriveWarpName(%q) = %q; want %q", tt.url, got, tt.want)
			}
		})
	}
}

// TestCloneRepo_InvalidURLFails asserts that cloneRepo's error on a bogus/nonexistent source URL is
// composed from local context (the attempted URL and destination, plus the git exit code) rather
// than git's own stderr text.
// No real git fixture is needed: a nonexistent source path is enough to make `git clone` fail
// immediately.
func TestCloneRepo_InvalidURLFails(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "cloned-repo")
	const url = "/does/not/exist/nonexistent-repo.git"

	err := cloneRepo(url, dest)
	if err == nil {
		t.Fatalf("cloneRepo(%q, %q) error = nil; want failure for a nonexistent source", url, dest)
	}
	if !strings.Contains(err.Error(), url) {
		t.Errorf("cloneRepo(%q, %q) error = %q; want substring %q (attempted URL)", url, dest, err.Error(), url)
	}
	// Compare against filepath.Base(dest) rather than the raw dest string: %q escapes
	// backslashes on Windows, so the literal OS-native dest path would never appear
	// unescaped in err.Error() even though the destination is faithfully reported.
	if destName := filepath.Base(dest); !strings.Contains(err.Error(), destName) {
		t.Errorf("cloneRepo(%q, %q) error = %q; want substring %q (destination)", url, dest, err.Error(), destName)
	}
	// git's own explanation must survive alongside the local context: an exit code by itself does
	// not tell the operator whether the URL was wrong, unreachable, or unauthorised.
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("cloneRepo(%q, %q) error = %q; want git's own explanation included, not just an exit code",
			url, dest, err.Error())
	}
}

// initTinyRepo initializes a minimal single-commit git repository at dir on branch "main", suitable
// as a CloneHub source: cloneRepo works against any git repo, not only a bare one.
func initTinyRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	gitkit.MustRun(t, dir, "git", "init", "-b", "main")
	gitkit.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, dir, "git", "config", "user.name", "Test")
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# "+filepath.Base(dir)), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	gitkit.MustRun(t, dir, "git", "add", "README.md")
	gitkit.MustRun(t, dir, "git", "commit", "-m", "init")
}

// TestCloneHub_CreatesHubScratchDir asserts that CloneHub's hub-materialisation step creates
// HubScratchDir(res.HubPath) (<hub>/_board/.lyx): the hub-wide ephemeral sibling of
// <hub>/_board/_lyx, created only after the board worktree exists, a real directory rather than a
// junction.
func TestCloneHub_CreatesHubScratchDir(t *testing.T) {
	fixtures := t.TempDir()
	warpSrc := filepath.Join(fixtures, "warp-src")
	weftSrc := filepath.Join(fixtures, "weft-src")
	initTinyRepo(t, warpSrc)
	initTinyRepo(t, weftSrc)

	cloneParent := t.TempDir()
	// ForceBootstrap: true — the weft fixture here is a non-bare working repo built by
	// initTinyRepo, an ordinary seeded repo standing in for a weft, not a repo that has ever
	// been one, so it carries no .lyx-anchor and would otherwise trip the old-order guard.
	res, err := CloneHub(cloneParent, CloneOptions{
		WeftURL:        filepath.ToSlash(weftSrc),
		WarpURL:        filepath.ToSlash(warpSrc),
		Subpath:        ".",
		ForceBootstrap: true,
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	scratchDir := HubScratchDir(res.HubPath)
	info, statErr := os.Stat(scratchDir)
	if statErr != nil {
		t.Fatalf("stat %s: %v; want CloneHub to have created it", scratchDir, statErr)
	}
	if !info.IsDir() {
		t.Errorf("%s exists but is not a directory", scratchDir)
	}
}
