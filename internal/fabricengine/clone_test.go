// clone_test.go — unit tests for URL-derivation helpers, cloneRepo's error path, and CloneHub's
// hub-level <hub>/.lyx materialisation.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/reedengine"
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
	if strings.Contains(err.Error(), "fatal:") {
		t.Errorf("cloneRepo(%q, %q) error = %q; want no %q substring (raw git stderr leak)", url, dest, err.Error(), "fatal:")
	}
}

// initTinyRepo initializes a minimal single-commit git repository at dir on branch "main", suitable
// as a CloneHub source: cloneRepo works against any git repo, not only a bare one.
func initTinyRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	lyxtest.MustRun(t, dir, "git", "init", "-b", "main")
	lyxtest.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, dir, "git", "config", "user.name", "Test")
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# "+filepath.Base(dir)), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	lyxtest.MustRun(t, dir, "git", "add", "README.md")
	lyxtest.MustRun(t, dir, "git", "commit", "-m", "init")
}

// TestCloneHub_CreatesHubDotLyx asserts that CloneHub's hub-materialisation step creates <hub>/.lyx:
// a fabric-recognised hub-level geometry element alongside <hub>/_board, a real directory rather than
// a junction, since the hub itself is not a git repo and has nothing to exclude and no weft to point
// at.
func TestCloneHub_CreatesHubDotLyx(t *testing.T) {
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

	dotLyx := filepath.Join(res.HubPath, lyxdirs.DotLyxDirName)
	info, statErr := os.Stat(dotLyx)
	if statErr != nil {
		t.Fatalf("stat %s: %v; want CloneHub to have created it", dotLyx, statErr)
	}
	if !info.IsDir() {
		t.Errorf("%s exists but is not a directory", dotLyx)
	}
}

// TestReedHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx asserts the second half of card
// 50's coverage: reedengine's own idempotent os.MkdirAll(HubLogsDir(...)) boot-path call still
// succeeds, twice in a row, when <hub>/.lyx already exists as a real directory — the shape CloneHub
// now produces. No import cycle exists between fabricengine and reedengine (verified: reedengine does
// not depend on fabricengine), so this half lives here rather than in the integration file.
func TestReedHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx(t *testing.T) {
	hubPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hubPath, lyxdirs.DotLyxDirName), 0o755); err != nil {
		t.Fatalf("mkdir <hub>/.lyx: %v", err)
	}

	l := &lyxcwd.Location{HubPath: hubPath}
	logsDir := reedengine.HubLogsDir(l)
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("first MkdirAll(%s) = %v; want nil", logsDir, err)
	}
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("second MkdirAll(%s) = %v; want nil (idempotent)", logsDir, err)
	}
}
